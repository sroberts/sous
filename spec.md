# Sous: Basics Design Spec

The foundation Sous is built on: how the service runs under Outpost, where its
state lives, who is calling, and the core recipe model with its REST and MCP
surface. Feature specs (UI, cook mode, meal planning, and the rest) build on
this and are listed in §13.

Conformance target: [`outpost-spec.md`](outpost-spec.md). Section references
like "Outpost §4" point there. §12 maps every item on its conformance
checklist to where this design satisfies it.

---

## 1. The process

One Go binary, `sous`. Running it with no subcommand serves. There are no
other subcommands in the basics: the Mela import is a separate script (§11).

- **Go 1.27**, standard library `net/http` routing, `log/slog` for logging.
- **SQLite via `modernc.org/sqlite`**: pure Go, so `CGO_ENABLED=0` and
  cross-compiled release binaries just work (Outpost §14).
- Foreground only. Never forks, writes a pidfile, or re-execs (Outpost §8).

### Startup sequence

1. Parse configuration (§2). Missing or invalid → log the reason, exit 1.
2. Listen on `127.0.0.1:<port>`. Failure → exit 1.
3. Start serving immediately with `ready = false`: `/up` returns 503, every
   other route returns 503 `{"error":{"code":"starting"}}`.
4. In the background, open storage (§3): create the layout, clear `tmp/`, open
   the database, run migrations, rebuild in-memory state.
   Failure → exit 1.
5. Set `ready = true`. Log one `ready` line with the port, storage path,
   version, schema version, and auth mode.

Expected time to ready is well under a second. Migrations that rebuild the
search index over the full library should still finish in a few seconds, inside
Outpost's default 30s `start_grace_seconds`. If a migration ever needs longer,
the release notes say to raise `start_grace_seconds`; Sous does not fake health.

### Shutdown sequence

On SIGTERM or SIGINT (Outpost §6, 10s before SIGKILL):

1. Set `ready = false`, stop accepting connections.
2. `http.Server.Shutdown` with a **6s** deadline, letting in-flight requests
   finish. MCP sessions and long requests are cut at the deadline.
3. Checkpoint the WAL into the main database file
   (`PRAGMA wal_checkpoint(TRUNCATE)`) and close it. This comes before anything
   else is torn down, because it is the thing that must reach disk. It also
   leaves `outpost backup`, which stops the service before archiving, a single
   self-contained `sous.db`.
4. Log `stopped` and exit 0. If the checkpoint or close fails, log it and
   exit 1.

SQLite with WAL is crash-safe without step 3. A SIGKILL loses nothing
committed, so restarts never need repair.

### HTTP server settings

`ReadHeaderTimeout` 10s, `IdleTimeout` 120s. No global write timeout, because
MCP streams. Per-route body limits: 1 MiB for JSON, 25 MiB for image uploads.

---

## 2. Configuration

Environment variables and flags only (Outpost §3). Nothing is read from a
well-known path, and nothing prompts.

| Setting | Source | Default | Notes |
|---|---|---|---|
| Port | `--port`, else `OUTPOST_PORT` | none, required | Flag wins. Never chosen dynamically. |
| Storage | `OUTPOST_STORAGE_DIR` | none, required | No flag: Outpost's value always wins, so Sous never takes it from anywhere else. |
| Auth mode | `OUTPOST_AUTH` | unset | `gateway` turns on token verification (§5). Any other value → exit 1. |
| Issuer | `OUTPOST_ISSUER` | none | Required when `OUTPOST_AUTH=gateway`. |
| JWKS URL | `OUTPOST_JWKS_URL` | none | Required when `OUTPOST_AUTH=gateway`. |
| Require gateway | `SOUS_REQUIRE_GATEWAY` | `false` | When `true` in gateway mode, requests without a valid identity token get 401 (§5). |
| Log level | `SOUS_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error`. |

Sous needs no secrets in the basics. If a later feature does (an API key for a
nutrition service, say), it takes a *path* via `SOUS_<NAME>_FILE` and reads
the file, per Outpost §3.

For local development the same variables apply:

```sh
OUTPOST_STORAGE_DIR=./.dev-data go run ./cmd/sous --port 8099
```

---

## 3. Storage

Everything Sous needs to survive a restart is under `OUTPOST_STORAGE_DIR`
(Outpost §4), and nothing is written anywhere else: not `$HOME`, not `/tmp`,
not beside the binary.

```
$OUTPOST_STORAGE_DIR/
  sous.db            SQLite database: all records, the search index, schema version
  sous.db-wal        present while running; folded in on clean shutdown
  sous.db-shm
  images/
    ab/abcdef….jpg   originals, content-addressed by SHA-256 of the bytes
  tmp/               staging for uploads; emptied at every start
```

- **The schema version lives only inside the database**, as
  `PRAGMA user_version`. Nothing about storage is cached outside it, because
  `outpost restore` can swap in older contents between runs.
- **Older database** (restored from backup) → migrations run forward from its
  `user_version`. Migrations are numbered, embedded in the binary, and each runs
  in its own transaction.
- **Newer database than the binary understands** (a downgrade) → refuse to
  touch it, log the two versions, exit 1. Opening it could corrupt it.
- **Empty directory** (first run, or a restore of nothing) → create the layout
  and an empty database. A fresh Sous is a valid Sous.
- **Images** are written to `tmp/`, fsynced, then renamed into `images/`. Same
  filesystem, so the rename is atomic and a crash never leaves half an image
  under its final name. Identical bytes deduplicate for free.
- **Orphaned images** (no recipe references them) are left alone in the
  basics. A sweep can come later; it is not needed for correctness.
- **No symlinks** are created anywhere in storage.

In-memory state, such as the JWKS cache and parsed configuration, is rebuilt
from scratch on every start.

---

## 4. HTTP surface

All on the one port, following the personal-utils shape (Outpost §9):

| Path | What |
|---|---|
| `GET /up` | Healthcheck (§4.1) |
| `/v1/…` | REST API, JSON (§8) |
| `GET /v1/openapi.json` | OpenAPI 3.1 description of `/v1` |
| `/mcp` | MCP, streamable HTTP transport (§9) |
| everything else | The web UI: static assets embedded with `go:embed`, with unknown paths falling back to `index.html` for client-side routing |

The UI is a client of `/v1` like any other. It gets no private endpoints, so
REST, MCP, and the UI cannot drift apart. Its design is a separate spec (§13).

### 4.1 Healthcheck

`GET /up` reads one `atomic.Bool` and returns 200 or 503 with no body. It does
no I/O, takes no locks, needs no authentication, and is exempt from identity
resolution and request logging.

It means "this process is working". If the database starts failing at
runtime, `/up` stays 200 and the affected `/v1` and `/mcp` calls return 503
with the error logged. Restarting would not fix a full disk.

For a human view of the whole system, `GET /v1/status` reports version, schema
version, auth mode, recipe count, storage size, and whether the JWKS was last
fetched successfully.

---

## 5. Identity

Every write records who made it, and some data is per person. Requests reach
Sous by three routes, and identity is resolved per request in this order.

**1. Through the Outpost login gateway.** The gateway strips any client-sent
`X-Outpost-*` headers, then sets `X-Outpost-User`, `-Name`, `-Email`,
`-Groups`, and `X-Outpost-Identity`. The last is an EdDSA-signed JWT, valid for
60 seconds, with `aud` set to the app's host (`<name>.<domain>`).

When `OUTPOST_AUTH=gateway`, Sous verifies `X-Outpost-Identity`:

- signature against `OUTPOST_JWKS_URL`, by `kid`;
- `iss` equals `OUTPOST_ISSUER`;
- `aud` equals the request's host without the port (the gateway preserves the
  browser's `Host`);
- `exp` and `iat` within a 30-second clock-skew allowance.

On success, identity comes **from the token's claims** (`sub`, `name`,
`email`, `groups`), never from the plain headers. Anything on the machine can
reach `127.0.0.1:<port>` directly and set headers; only the signature proves
the gateway sent them.

A token that is present but fails verification gets a 401. It is never
quietly downgraded to the next route.

The JWKS is fetched on first need and cached in memory. It is refetched when a
token names an unknown `kid` (Outpost keeps the old key published for a day
after rotation), at most once a minute. If the JWKS cannot be fetched,
gateway requests get 503 and `/up` stays 200.

When `OUTPOST_AUTH` is unset, `X-Outpost-*` headers are ignored entirely.

**2. Directly over the tailnet (tsnet), or from a local process.** The gateway
and tsnet run side by side, so a request may arrive with no identity token.
Such a request may name a person with the `X-Sous-Person` header (agents,
scripts) or the `sous_person` cookie (the UI's "who's cooking?" picker). This
is **attribution, not authentication**: tailnet membership is the boundary
(Outpost §10), and the household trusts itself. In gateway mode with
`SOUS_REQUIRE_GATEWAY=true`, this route is closed and gets 401.

**3. Nobody.** Reads are allowed. Writes get 401
`{"error":{"code":"who_are_you"}}`, so every change has an author.

### People

A `people` row is keyed by a username:

- Gateway users are created or updated on first sight, from the token's `sub`,
  `name`, and `email`.
- Without the gateway, people are created with `POST /v1/people`. An
  unrecognised `X-Sous-Person` is a 401, not an implicit create, so a typo
  doesn't make a new household member.

A username is the same person by either route, so enabling the gateway later
keeps everyone's history.

Each request also records **how** it arrived (`ui`, `api`, or `mcp`), so agent
edits can be told apart from a person's.

---

## 6. Logging

`slog` JSON handler on stdout, one object per line, unbuffered (Outpost §7).
No colour, no progress output.

- **Lifecycle:** `starting`, `migrated` (from/to version), `ready`,
  `shutting down`, `stopped`, and any fatal error just before a non-zero exit.
- **Requests:** method, path, status, duration, person, and route
  (gateway/direct/none). `/up` is never logged; at one probe every 3 seconds
  it would drown everything else.
- **Never logged:** identity tokens, cookies, request bodies.

---

## 7. Data model

SQLite, foreign keys on, WAL mode. IDs are UUIDv7 strings: time-ordered, safe
to expose. Every table that holds user-editable data has `created_at`,
`created_by`, `updated_at`, `updated_by` (person IDs). Times are UTC RFC 3339.

### Recipes

```
recipes
  id, title, source_url, description, notes,
  yield_text           "Yield 4 to 6 servings", as written
  servings             4, parsed from yield_text, nullable; the basis for scaling
  prep_minutes, cook_minutes, total_minutes    nullable integers
  time_text            original time wording when it doesn't parse ("about 3 hours")
  nutrition_text       free text in the basics
  version              integer, incremented on every change (§8, concurrency)
  deleted_at           soft delete (§8)
  + audit columns

recipe_ingredients
  recipe_id, position,
  section              "For the sauce", nullable; groups lines under a heading
  raw                  the line exactly as written; always kept, always shown on request
  quantity, quantity_max      2 and 4 for "2 to 4"; nullable
  unit                 normalised: "tbsp", "g", "cup"…; nullable
  item                 "butter"
  note                 "as needed", "finely diced"
  parser_version       so lines can be re-parsed when the parser improves

recipe_steps
  recipe_id, position, section, text

images
  hash (sha-256, primary key), content_type, width, height, bytes
recipe_images
  recipe_id, image_hash, position          position 0 is the cover

tags
  id, name (unique, case-insensitive)
recipe_tags
  recipe_id, tag_id
```

**Ingredient parsing** runs on the server, on every write, so every client
gets the same result. Clients send `raw` lines, and the server returns the
parsed fields alongside. The basics parser handles integers, decimals,
fractions and unicode fractions (`¼`), ranges (`2 to 4`, `2–4`), common US and
metric units with abbreviations, and a trailing comma clause as `note`. When it
can't parse a line, `item` is the whole line and the other fields are null.
That is still a valid ingredient, just one that can't be scaled. Scaling and
conversion themselves are a feature spec.

### Per person

```
person_recipe
  person_id, recipe_id, favorite (bool), want_to_cook (bool), updated_at
```

Ratings, personal notes, and cook history are feature specs. They follow this
same keyed-by-(person, recipe) shape.

### Change log

```
changes
  id, at, person_id, via (ui|api|mcp), entity, entity_id,
  action (create|update|delete|restore), summary
```

Append-only, written in the same transaction as the change it describes. With
agents holding full write access over MCP, this is how you find out what an
agent did.

### Search

An FTS5 table indexes recipe title, description, notes, ingredient `item`s,
and tag names, kept in sync by triggers. It lives inside `sous.db`, so it is
backed up and restored with everything else.

---

## 8. REST API (`/v1`)

JSON in and out. Errors are always
`{"error":{"code":"…","message":"…"}}`, with codes stable enough to branch on.

| Method & path | What |
|---|---|
| `GET /v1/status` | System status (§4.1) |
| `GET /v1/me` | The resolved person, or 401 |
| `GET /v1/people` · `POST /v1/people` | List, or create (no-gateway mode) |
| `GET /v1/recipes` | List and search: `q` (full text), `tag` (repeatable), `ingredient` (repeatable, all must match), `favorite`, `want_to_cook`, `sort` (`title`, `updated`, `created`), `cursor`, `limit` (default 50, max 200) |
| `POST /v1/recipes` | Create. Body: recipe fields, `ingredients` as raw lines, `steps` as text, `tags` as names, `images` as hashes |
| `GET /v1/recipes/{id}` | Full recipe with parsed ingredients, steps, tags, images, and the caller's `person_recipe` flags |
| `PATCH /v1/recipes/{id}` | Partial update. Sending `ingredients` or `steps` replaces that whole list |
| `DELETE /v1/recipes/{id}` | Soft delete: sets `deleted_at` and hides it from lists |
| `POST /v1/recipes/{id}/restore` | Undo a delete |
| `GET /v1/trash` | Deleted recipes |
| `PUT /v1/recipes/{id}/mine` | Set the caller's `favorite` / `want_to_cook` |
| `GET /v1/tags` | Tags with recipe counts |
| `POST /v1/ingredients/parse` | Parse `{lines}` without saving; returns each line as the server would store it. The editor's live preview uses it. Not a write, so it needs no person |
| `POST /v1/images` | Upload raw bytes (`image/jpeg`, `png`, `webp`, `heic`); returns `{hash}` |
| `GET /v1/images/{hash}` | The image. `Cache-Control: immutable`, since the content never changes under a hash |
| `GET /v1/changes` | The change log, newest first, filterable by `entity_id`, `person`, `via` |

- **Concurrency.** Recipes carry `version`, returned as an `ETag`. `PATCH` and
  `DELETE` require `If-Match`. A stale version gets 412 with the current
  recipe, so two people (or a person and an agent) can't silently overwrite
  each other.
- **Pagination** is by opaque cursor, never by offset.
- **List items are summaries:** `id`, `title`, `source_host`, `tags`, `cover` (first image hash), `total_minutes`, `time_text`, `ingredient_items` (parsed `item`s, lower-cased), the caller's `mine` flags, `version`, and `updated_at`. The web UI pages through the whole list once and searches it locally, so the library stays instant at household scale.
- **Deletion is soft** in the basics. Purging the trash is a deliberate later
  action, not a timer, because Outpost provides no scheduling (Outpost §10).

`GET /v1/openapi.json` is generated from the same route and type definitions
the handlers use, so it cannot fall out of date. It is what Outpost's
`SKILL.md` generator will read (Outpost §9).

---

## 9. MCP (`/mcp`)

Streamable HTTP transport, using the official Go SDK
(`github.com/modelcontextprotocol/go-sdk`). Tools call the same service layer
as the REST handlers. The basics expose:

| Tool | Maps to |
|---|---|
| `search_recipes` | `GET /v1/recipes` |
| `get_recipe` | `GET /v1/recipes/{id}` |
| `create_recipe` | `POST /v1/recipes` |
| `update_recipe` | `PATCH /v1/recipes/{id}` (takes `version` in place of `If-Match`) |
| `delete_recipe` / `restore_recipe` | soft delete and restore |
| `set_my_recipe_flags` | `PUT /v1/recipes/{id}/mine` |
| `list_tags` | `GET /v1/tags` |
| `recent_changes` | `GET /v1/changes` |

Agents get full read/write, as decided. Identity resolves exactly as in §5:
through the gateway, or by `X-Sous-Person` configured in the agent's MCP
client. Every write is logged with `via = mcp`. Soft delete plus the change log
make an agent's mistakes recoverable.

The operator sets `mcp = true` in the manifest; `outpost.toml` declares it
(§10).

---

## 10. Build, release, and install

```
cmd/sous/            main: config, lifecycle, wiring
internal/config/     env and flags (§2)
internal/storage/    layout, SQLite, migrations, images (§3, §7)
internal/identity/   gateway verification, JWKS cache, attribution (§5)
internal/recipes/    domain logic and the ingredient parser
internal/api/        REST handlers and the OpenAPI document (§8)
internal/mcp/        MCP tools (§9)
internal/ui/         go:embed of the built web assets
web/                 UI source; builds into internal/ui/dist
scripts/mela-import/ the one-time import (§11)
```

**Releases.** A GitHub Actions workflow on a version tag builds, with
`CGO_ENABLED=0`:

```
sous-darwin-arm64
sous-linux-amd64
sous-linux-arm64
SHA256SUMS
outpost.toml
```

All are attached to the release: bare binaries, checksums so Outpost verifies
the download, and `outpost.toml` so the descriptor ships with the binary it
describes (Outpost §14).

```toml
# outpost.toml
name                = "sous"
healthcheck         = "/up"
mcp                 = true
restart_policy      = "on-failure"
start_grace_seconds = 30
```

There is no `args` and no fixed `port`: Sous reads `OUTPOST_PORT`, which
`outpost add` writes into every manifest it generates, and Outpost assigns the
port. Installing is then:

```sh
outpost add https://github.com/sroberts/sous
```

The repository is private, so the operator needs `gh` authenticated or
`GITHUB_TOKEN` set (Outpost §14).

A hand-written manifest works too, and must set the port in one of the two
places Sous reads:

```toml
binary = "/opt/outpost/bin/sous"
env    = { OUTPOST_PORT = "8095" }
port   = 8095
mcp    = true
```

---

## 11. The Mela import

A one-time script in `scripts/mela-import/`. It is Python run with `uv`, and
separate from the service: Sous itself contains no Mela code, and the script
can be deleted after use.

It reads the `.melarecipes` zip, and for each recipe:

- uploads its base64 images to `POST /v1/images`;
- maps the fields and `POST`s to `/v1/recipes`:
  - `title`, `link` → `source_url`, `notes`, `nutrition`;
  - `yield` → `yield_text`;
  - times → minutes where they parse, else `time_text`;
  - `ingredients` and `instructions` split on newlines;
  - `categories` → tags;
- sets `favorite` and `wantToCook` via `PUT …/mine` for the person running the
  import.

It sends `X-Sous-Person`, so the import is attributed to that person like
any other API client. It is idempotent by `source_url` plus title: a re-run
skips recipes already present, so an interrupted import can be resumed.

---

## 12. Conformance

Every item from the Outpost conformance checklist (Outpost §11):

| Requirement | Where |
|---|---|
| Listens on exactly the declared port | §1 step 2, §2 |
| Binds `127.0.0.1` only | §1 step 2 |
| Plain HTTP, no TLS | §4 |
| Port from a flag and from `OUTPOST_PORT` | §2 |
| Never selects a port dynamically | §2: no port → exit 1 |
| `GET /up` returns 2xx when healthy | §4.1 |
| Healthcheck needs no authentication | §4.1, exempt from §5 |
| Responds well under 2s | §4.1: one atomic read |
| Cheap enough every 3s | §4.1: no I/O, no locks, not logged |
| Non-2xx until ready | §1 steps 3–5 |
| Not unhealthy because a dependency is down | §4.1 (database), §5 (JWKS) |
| Config from env and flags only | §2 |
| Starts unattended | §2 |
| No config file at a well-known path | §2 |
| All state under `OUTPOST_STORAGE_DIR` | §3 |
| No state anywhere else | §3, including `tmp/` inside storage |
| Starts after a restore replaces storage | §3: forward migrations, empty-dir start, downgrade refusal |
| Symlinks stay inside storage | §3: none are created |
| Exits non-zero on unrecoverable failure | §1 startup and shutdown |
| Never exits zero on an error | §1: exit 0 only after a clean, requested stop |
| Handles SIGTERM, flushes, exits | §1 shutdown |
| Shutdown within 10s | §1: 6s drain + checkpoint |
| Foreground, no daemonizing, no pidfile | §1 |
| Restarts cleanly with no repair | §1 (WAL), §3 |
| Logs to stdout/stderr only | §6 |
| No long buffering | §6 |
| No ANSI escapes | §6 |

**Verified automatically.** `internal/conformance` holds a Go test that builds
the binary and runs Outpost §13's checks against it: loopback-only bind, `/up`
timing, 503-then-200 at startup, SIGTERM exit time, non-zero exit on a bad
port, no writes outside a scratch storage directory, a clean start after that
directory is emptied, and a stable PID. It runs in CI on every push, then by
hand under real Outpost before the first release (`outpost apply`, `restart`,
`backup`, `restore`, `logs`).

---

## 13. Out of scope: later specs

Each builds on the model and API above without changing what this spec defines.

- **Web UI and offline:** the warm editorial design, the installable PWA, the
  offline cache for opened recipes, and the offline shopping list with sync.
- **Cook mode:** step view, wake lock, ingredient check-offs, step timers.
- **Scaling and units:** servings-based scaling, US ↔ metric conversion.
- **Meal plan and shopping list:** a weekly plan, and a combined, de-duplicated
  list grouped by aisle.
- **Import from URL:** schema.org `Recipe` extraction from a link.
- **Collections:** hand-ordered groups of recipes.
- **Ratings, personal notes, cook history:** per person, keyed like
  `person_recipe`.
- **Image derivatives:** resized variants for the UI, generated on demand and
  stored under `images/`.
