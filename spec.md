# Sous: Basics Design Spec

The foundation Sous is built on: how the service runs, under Outpost or on its
own, where its state lives, who is calling, and the core recipe model with its REST and MCP
surface. Feature specs (UI, cook mode, meal planning, and the rest) build on
this and are listed in §13.

Conformance target: [`outpost-spec.md`](outpost-spec.md). Section references
like "Outpost §4" point there. §12 maps every item on its conformance
checklist to where this design satisfies it.

---

## 1. The process

One Go binary, `sous`. Running it with no subcommand serves. There are no
other subcommands in the basics: the Mela import is a separate script (§11).

Outpost is the primary host, but nothing requires it. Run alone, Sous takes
its port and storage from `--port` and `--data` (§2), and identity falls to
the "who's cooking?" route (§5). The Outpost contract (loopback only, plain
HTTP, foreground, clean SIGTERM) holds either way, so a standalone Sous
behaves like one under Outpost with no gateway.

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
   the database, run migrations, rebuild in-memory state. Then, in a library
   that has never been used, add the sample recipes (§3). Failure to open →
   exit 1; failure to add samples → log a warning and carry on.
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
| Storage | `OUTPOST_STORAGE_DIR`, else `--data` | none, required | Outpost's value always wins, so its backups cover everything. `--data` is for running without Outpost; both set to different directories → exit 1. |
| Auth mode | `OUTPOST_AUTH` | unset | `gateway` turns on token verification (§5). Any other value → exit 1. |
| Issuer | `OUTPOST_ISSUER` | none | Required when `OUTPOST_AUTH=gateway`. |
| JWKS URL | `OUTPOST_JWKS_URL` | none | Required when `OUTPOST_AUTH=gateway`. |
| Require gateway | `SOUS_REQUIRE_GATEWAY` | `false` | When `true` in gateway mode, requests without a valid identity token get 401 (§5). |
| Log level | `SOUS_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error`. |
| Sample recipes | `SOUS_SAMPLES` | `true` | `false` skips adding the sample recipes (§3) on this start; it removes nothing. Any other value → exit 1. |

Sous needs no secrets in the basics. If a later feature does (an API key for a
nutrition service, say), it takes a *path* via `SOUS_<NAME>_FILE` and reads
the file, per Outpost §3.

`sous --help` prints these and exits 0; `sous --version` prints the version.

Run without Outpost, the flags alone are enough:

```sh
sous --port 8080 --data ~/sous-library
```

For local development:

```sh
go run ./cmd/sous --port 8099 --data ./.dev-data
```

---

## 3. Storage

Everything Sous needs to survive a restart is under the storage directory:
`OUTPOST_STORAGE_DIR` under Outpost (Outpost §4), or `--data` on its own
(§2). Nothing is written anywhere else: not `$HOME`, not `/tmp`, not beside
the binary.

```
<storage>/
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
  in its own transaction, with foreign keys off (a table rebuild needs that,
  and SQLite ignores the pragma inside a transaction) and a
  `PRAGMA foreign_key_check` before it commits. A migration that leaves a
  dangling reference rolls back and Sous exits 1.
- **Newer database than the binary understands** (a downgrade) → refuse to
  touch it, log the two versions, exit 1. Opening it could corrupt it.
- **Empty directory** (first run, or a restore of nothing) → create the layout
  and a database, then seed it (next item). A fresh Sous is a valid Sous.
- **Sample recipes.** A library that has never been used gets twelve sample
  recipes on start (`internal/samples`; sources in its `SOURCES.md`). *Never
  used* means no `changes` rows and no `recipes` rows, deleted ones included,
  so deleting the samples is permanent, and a restore that brings data never
  seeds. Signing in through the gateway alone doesn't count as use. Seven
  are historic: each is a public-domain original (Glasse 1751, Simmons 1796, Randolph
  1824, Acton 1845, Beeton 1861, Farmer 1896, Escoffier 1907) with a modern
  adaptation as the recipe, the verbatim text and its citation in `notes`, a
  period illustration, and the tags for its course plus `Historic`. Five are
  classics written for Sous in its own words, four inspired by a named chef's
  book (cited, never quoted), tagged with their course plus `Classic`, with a
  public-domain or CC0 photo where one exists. All are
  credited to the built-in author `_sous` (§5), and their changes are logged
  `via = system`. Seeding runs once per start, before `ready`; if it fails,
  Sous logs a warning and starts anyway. `SOUS_SAMPLES=false` skips it.
  The same files are the test fixtures.
- **Images** are written to `tmp/`, fsynced, then renamed into `images/`. Same
  filesystem, so the rename is atomic and a crash never leaves half an image
  under its final name. Identical bytes deduplicate for free.
- **Orphaned images** (no recipe references them) are left alone in the
  basics. A sweep can come later; it is not needed for correctness.
- **Imported photos** (§14) take the normal image path: fetched, written to
  `tmp/`, renamed into `images/`. They are stored when the preview is made, so
  a preview that is never saved leaves orphaned photos. That is allowed.
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

Sous never reads `Authorization`. A script or Shortcut can send an Outpost
access token (`Authorization: Bearer opat_…`) to the gateway, which consumes
it and forwards the same signed identity for the person who owns the token. A
bad or revoked token never reaches Sous: Outpost answers with a plain-text 401
(or 429 when throttled, 403 outside the allow-list).

A token that is present but fails verification gets a 401. It is never
quietly downgraded to the next route.

The JWKS is fetched on first need and cached in memory. It is refetched when a
token names an unknown `kid` (Outpost keeps the old key published for a day
after rotation), at most once a minute. If the JWKS cannot be fetched,
gateway requests get 503 and `/up` stays 200.

When `OUTPOST_AUTH` is unset, `X-Outpost-*` headers are ignored entirely.

The UI asks `GET /v1/me` on load. When it answers `"source": "outpost"`, the
person is the Outpost login: the UI shows their name, with no "who's cooking?"
picker and no switching people. Their name opens a menu that leads to the
Outpost portal (`portal`, from `OUTPOST_ISSUER`): the account page, which
holds sign-out, and the other apps. Sous can't sign anyone out itself, since
Outpost takes sign-out only as a form posted from the portal. Any other answer
leaves the picker in place.

When the Outpost sign-in ends while a page is open, the gateway answers the
UI's requests with a redirect to its login, or a 401 page for a write. Sous's
own answers are always JSON and never redirect, so the UI reads either as
signed out, says changes aren't being saved, and offers to sign in again,
which reloads the page through the gateway's login and returns to it.

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
  unrecognised `X-Sous-Person` is a 401 `unknown_person`, not an implicit
  create, so a typo doesn't make a new household member. An unrecognised
  `sous_person` cookie (left over after a restore, say) is treated as nobody,
  so the UI simply asks who's cooking again.

A username is the same person by either route, so enabling the gateway later
keeps everyone's history.

`_sous` ("Sous", `source = system`) is reserved. It is the author of the
sample recipes (§3), and it never appears in `GET /v1/people`. Nobody can act
as it: `X-Sous-Person`, the `sous_person` cookie and MCP's caller header treat
it as unknown, and a gateway token whose `sub` is `_sous` gets 401
`invalid_identity`.

Each request also records **how** it arrived (`ui`, `api`, or `mcp`), so agent
edits can be told apart from a person's. Changes Sous makes itself, the
sample recipes, are `system`.

---

## 6. Logging

`slog` JSON handler on stdout, one object per line, unbuffered (Outpost §7).
No colour, no progress output.

- **Lifecycle:** `starting`, `migrated` (from/to version), `ready`,
  `shutting down`, `stopped`, and any fatal error just before a non-zero exit.
- **Samples:** `samples added` with the `count`, when a new library is seeded
  (§3). If seeding fails, a `samples not added` warning with the error.
- **Requests:** method, path, status, duration, person, and route
  (gateway/direct/none). `/up` is never logged; at one probe every 3 seconds
  it would drown everything else.
- **Imports:** one `import` line per attempt (§14): `host`, `path`, `outcome`
  (`ok` or an error code), `method`, `duration_ms`. Never the query string,
  which can carry tokens; never the page.
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
  id, at, person_id, via (ui|api|mcp|system), entity, entity_id,
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
| `GET /v1/me` | The resolved person, with `source`: `outpost` (a verified gateway login) or `device` (named by header or cookie), and for `outpost` the `portal` URL; or 401 |
| `GET /v1/people` · `POST /v1/people` | List, or create (no-gateway mode). `_sous` is never listed (§5) |
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
| `POST /v1/recipes/import/preview` | Read a recipe from a link (`url`) or pasted text (`text`, optional `title`) into a draft, without saving it. Exactly one of `url` or `text`. Needs a person, because it stores photos (§14) |
| `POST /v1/recipes/import` | Read a recipe from a link (`url`) and save it in one step, or return the copy already saved. Needs a person. Always 200 on success, with a `summary` to speak (§14) |
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
| `import_recipe` | `POST /v1/recipes/import/preview`, with optional `save` (§14) |
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
internal/importer/   reading a recipe from a page or pasted text (§14)
internal/safefetch/  the guarded HTTP client the importer fetches with (§14)
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

- uploads its base64 images to `POST /v1/images`. Images Sous refuses (Mela
  holds the odd TIFF or GIF under a `.jpg` name) are converted to JPEG, or to
  PNG when they have transparency, and uploaded again;
- maps the fields and `POST`s to `/v1/recipes`:
  - `title`, `link` → `source_url`, `text` → `description`, `notes`,
    `nutrition`;
  - `yield` → `yield_text`;
  - times → minutes where they are a plain duration (`1 hour 15 minutes`,
    `1h`, `1:10`, `45`); anything with more meaning (ranges, "plus overnight
    soaking", "at least") keeps its wording in `time_text`;
  - `ingredients` and `instructions` split on newlines. `#` headings pass
    through as sections. Steps numbered `1 …`, `2 …` in order lose the
    numbers, since Sous numbers steps itself;
  - `categories` → tags;
- sets `favorite` and `wantToCook` via `PUT …/mine` for the person running the
  import.

It sends `X-Sous-Person`, so the import is attributed to that person like
any other API client; `--name` adds them to the household first if needed.
It is idempotent by `source_url` plus title: a re-run skips recipes already
present, so an interrupted import can be resumed. A recipe that appears
twice in the export is imported once, and the second copy is reported.

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
| Starts after a restore replaces storage | §3: forward migrations, empty-dir start, downgrade refusal; a restore with data is never seeded |
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
- **Import from URL:** specified in §14.
- **Collections:** hand-ordered groups of recipes.
- **Ratings, personal notes, cook history:** per person, keyed like
  `person_recipe`.
- **Image derivatives:** resized variants for the UI, generated on demand and
  stored under `images/`.

---

## 14. Import from a link

A person gives Sous a link; Sous reads the recipe on that page into a draft
the person checks in the editor and then saves. Nothing is saved by the read.
`POST /v1/recipes/import` skips the check and saves (Save in one step).

### How a page is read

Sous fetches the page and tries four readers, in order, and uses the first
that yields a recipe with usable data:

1. **JSON-LD.** `application/ld+json` blocks, including `@graph`, top-level
   arrays, `mainEntity`, and `@id` references. Slightly broken JSON (HTML
   comments, CDATA, raw control characters in strings) is repaired first.
2. **Microdata.** The first `itemscope` of type `schema.org/Recipe`, mapped
   into the same shape as JSON-LD.
3. **h-recipe.** The first `.h-recipe` element, mapped the same way.
4. **Paste.** If none works, the error carries what the page did show (below)
   and the person pastes the recipe text (see Text mode).

The report names the method: `json-ld`, `microdata`, `h-recipe` or `text`.
Notes on what is extracted:

- Titles lose a trailing period and a trailing " Recipe".
- Ingredients come from `recipeIngredient`, with bullets stripped.
- Steps come from `recipeInstructions` in any of its shapes (text, step
  objects, sections). Sections become `# Heading` lines, as in the editor.
  "Step N:" prefixes are removed only when the steps are numbered in order
  (the same rule as the Mela import, §11).
- Tips, and sections named like "notes", "tips" or "cook's notes", go to
  the recipe's notes.
- Times are ISO 8601 durations or free text. Sous never invents a total: a
  lone total is shown bare, otherwise "Prep: … · Cook: …" goes in `time_text`.
- Nutrition is joined into `nutrition_text` in a fixed order with " · ".
- With several recipes on a page, the first with ingredients is used, with a
  warning.
- **No tags are imported.** Tagging is done by hand.
- `source_url` is the page's canonical URL when it is on the same host, else
  the final URL after redirects, else the link given, with the fragment and
  tracking parameters (`utm_*`, `fbclid`, `gclid`, `mc_*`) removed.

### Preview, then save

`POST /v1/recipes/import/preview` takes `{url}` or `{text, url?, title?}` and
returns 200 with:

- `draft`: a recipe in the shape `POST /v1/recipes` accepts, with photos
  already stored and listed in `images`;
- `ingredients`: the draft's lines as the server would store them;
- `images`: every photo candidate found, best first, each with `hash` once
  stored or `problem` if it was skipped;
- `report`: `method`, `source_url`, `fetched_url`, `archived`, and lists of
  what was `found`, what is `missing`, and `warnings`;
- `duplicates`: see below.

The web UI shows the draft in the editor under a banner that says how the page
was read and what is missing. The person edits, then saves through the normal
create call. Agents do the same, or pass `save` to `import_recipe`.

The route needs a person (`Write: true`), because stored photos record who
added them.

### Save in one step

`POST /v1/recipes/import` takes `{url}` only, runs the same importer, and
saves what it reads. It is made for the iOS "Save to Sous" Shortcut (below),
where no one is there to review a draft. It needs a person, and its changes
are recorded with `via` `api`. It returns 200 with:

```json
{"summary":"Saved Garlic Noodles.","id":"…","title":"Garlic Noodles","version":1,
 "path":"/r/<id>","url":null,"duplicate":false,"restored":false,
 "missing":[],"warnings":[]}
```

- `summary`: one sentence meant to be spoken. One of "Saved X.", "Saved X, but
  no steps were found." (or "no ingredients", or "no ingredients or steps"),
  "Already in Sous: X." or "Restored X from the trash." A page with no title
  is saved as "Recipe from <host>", with a warning.
- `path` is the recipe's page in the web UI. `url` is that page as an absolute
  link through the gateway (the portal's scheme and the request's host, which
  the identity token's `aud` has matched), else `null`.
- `missing` and `warnings` are the report's.

A **duplicate** (found as under Duplicates) saves nothing new: the existing
recipe is returned with `duplicate: true`. If that copy is in the trash it is
restored, attributed to the caller, and `restored` is `true`.

A **partial** read, with no ingredients or no steps, is saved anyway; the
`summary` says what is missing, and the person can finish it in the editor.

**Failures** have the same shapes as everywhere else: a 422 with the error
object and `page` for import failures, and the plain error object for the
rest. On this route only, they also carry a top-level `summary` string for
Siri to speak: a fixed sentence for each import error code, and the
`message` for the rest (`who_are_you` instead says to check the shortcut's
header).

No rate limit is added. Every call needs a person; the same fetch is already
open through the preview and MCP; the importer already caps concurrency at 4
with a 25 second budget and the fetch guard limits each fetch; and Outpost
throttles rejected tokens.

### Errors

Every import failure is a 422 with the usual error object plus `page`:

```json
{"error":{"code":"blocked","message":"…"},"page":{"source_url":"…","title":"…","image":{…}}}
```

`page` is what could still be read from the page (its title and best photo),
or `null` when nothing was fetched. It is enough to start a pasted recipe.
The codes:

| Code | When |
|---|---|
| `unsupported_url` | Not an http(s) link, has credentials in it, points at an address Sous won't fetch (below), or the page isn't HTML |
| `not_found` | The site answered 404 or 410, or the name doesn't resolve |
| `blocked` | 401, 403, 429 or 451; a 503 from a bot challenge; or a 200 that is a bot wall (Cloudflare, captcha, "enable JavaScript") with no recipe. Sites such as Reddit end up here |
| `site_error` | Any other 5xx |
| `timeout` | The site didn't answer in time |
| `too_large` | The page is over 5 MiB |
| `paywalled` | See the paywall rule |
| `no_recipe_found` | Anything else: the page loaded but holds no usable recipe |

Save in one step returns these too, with the `summary` described there.
Request mistakes (both or neither of `url` and `text`) are the ordinary 400
`invalid`. These errors are separate from `recipes.Error`, which already uses
`not_found` and `too_large` for other statuses; callers check for an import
error first.

### The fetch guard

The importer fetches whatever address a person types, so the fetcher
(`internal/safefetch/`) is the trust boundary and is the only HTTP client the
importer uses.

- **Blocked addresses.** A host is refused unless every address it resolves
  to is a public unicast address. Refused: loopback, private (RFC 1918 and
  ULA), link-local, multicast, unspecified; `0.0.0.0/8`, `100.64.0.0/10`
  (CGNAT), `192.0.0.0/24`, the documentation ranges (`192.0.2.0/24`,
  `198.51.100.0/24`, `203.0.113.0/24`, `2001:db8::/32`), `198.18.0.0/15`,
  `240.0.0.0/4`, broadcast, and the IPv6 transition ranges (`64:ff9b::/96`,
  `64:ff9b:1::/48`, `2002::/16`, `2001::/32`, `fec0::/10`). IPv4-mapped IPv6
  addresses are unmapped before the check. The host's own interface
  addresses, read at startup, are refused too. Any public port is allowed.
- **Pinned at dial time.** The check happens in the dialer, on the addresses
  it just resolved, and the connection is made to that checked address. There
  is no second lookup, so DNS rebinding cannot swap in another address. A
  second check on the socket backs this up. This applies to every redirect hop.
- **Caps.** 10 seconds per fetch; at most 5 redirects, to http or https only;
  5 MiB per page, 25 MiB per image, counted after decompression; response
  headers at most 64 KiB. Non-2xx bodies are read to at most 64 KiB. At most
  4 imports run at once, and one preview has a 25 second budget overall.
- **No ambient credentials.** No cookie jar, and no `Cookie` or
  `Authorization` header is ever sent. Environment proxies are ignored.
- **Honest User-Agent:** `Sous/<version> (+https://github.com/sroberts/sous)`.
  Sous does not imitate a browser; a site that refuses it gets `blocked` and
  the paste fallback.
- **`robots.txt` is not consulted.** A person asked for this one page; it is
  not crawling. This is a recorded decision, not an oversight.
- Tests may relax the address check only through hooks that panic outside
  `go test`.

### Images

Photos are fetched and stored while the preview is made, through the normal
image path (§3), so the draft already points at stored hashes and the editor
needs no second step. Candidates come from the recipe's `image`, then
`og:image`, then `twitter:image`, ranked by known size, then by size hints in
the URL, with unhinted URLs ahead of small thumbnails. Up to three are tried
in order and the first that works is the cover. The page is sent as `Referer`.

- JPEG, PNG, WebP and HEIC are stored as they are.
- **GIF** is converted: the first frame, to PNG if it has transparency, else
  JPEG at quality 90.
- TIFF, AVIF, SVG, BMP and anything unrecognised are **skipped**; the
  candidate records a `problem` and the next is tried. The `Accept` header for
  images does not offer AVIF.
- A photo that can't be stored never fails the import; the banner says so.

### Duplicates

A recipe imported from a link is found again by its `source_url`, with no
schema change and no index. Sous compares `NormalizeSourceURL` (lower-case
host, no `www.`, http equal to https, no fragment, no tracking parameters, no
trailing slash) of the link given, the final URL and the canonical URL against
every recipe's `source_url`, in Go. At household scale that is cheap. Matches
return in `duplicates` as `{id, title, version, deleted}`; a match in the
trash has `deleted: true`.

In the web UI a duplicate stops the import at a choice: open the existing
recipe, update it with this version, save as new, or (for a trashed one)
restore it. Updating replaces the title, description, yield, times,
`time_text`, nutrition, ingredients, steps and `source_url`; it keeps tags,
notes and existing photos, appends the imported cover if it is new, and saves
through `PATCH` with the usual `If-Match` handling. MCP `import_recipe` with
`save: true` on a duplicate returns an `exists` error rather than a copy.
`POST /v1/recipes/import` instead returns the existing recipe as above, and
restores it if it was in the trash.

### Wayback Machine links

A link of the form `web.archive.org/web/<timestamp>/<original>` is read from
the capture without the archive's toolbar (`/web/<timestamp>id_/<original>`).
`source_url` is the cleaned original address, not the archive's, so the recipe
is credited to the site it came from and duplicates match across both forms.
The report sets `archived` and `fetched_url` to the archive URL, and the
banner says so. Image URLs are rewritten to the archive's raw-asset form
(`/web/<timestamp>im_/<asset>`), because the original site may be gone.

### The paywall rule

If usable recipe data is present, Sous imports it, even when the page marks
itself `isAccessibleForFree: false`, and adds the warning "The site marks this
for subscribers; check it's complete." Only when the page is marked not-free
and has no usable recipe does the import fail with `paywalled`. Sous sends no
cookies and holds no site logins, so it reads what the site publishes to
anyone.

### Text mode

`{text}` (with an optional `title`, and optionally the `url` the recipe is
credited to) skips fetching entirely. The text is split into a title,
ingredients and steps: lines are trimmed and bullets stripped; headings such
as "Ingredients" and "Directions", "Instructions", "Method", "Steps" or
"Preparation" divide the sections; without headings, a leading run of
quantity-like or short unpunctuated lines are ingredients and the rest are
steps. Numbered steps are un-numbered by the same rule as above. The report
method is `text`. This is the paste fallback after a failed link; a `url` sent
with it is credited and checked for duplicates but never fetched. Text that
yields neither ingredients nor steps is a 422 `no_recipe_found`.

### MCP

`import_recipe {url, save?}` is registered when the server has an importer.
It returns the preview plus `saved`, the created recipe when `save` was true.
It is marked open-world (it reaches the web) and non-destructive. Without
`save` nothing is stored but photos: the agent checks the draft and passes it
to `create_recipe`. Failures carry the codes above, and the tool description
tells the agent to ask the person to paste the recipe.

### Bookmarklet and iOS Shortcut

The `/import` page opens with `?url=` (also `text` and `title`) and reads
automatically, so anything that can open a URL can send a recipe.
The page itself shows both setups; there is no settings area.

- **Bookmarklet** (desktop): a draggable link that opens
  `<sous>/import?url=` plus the current page's address.
- **iOS Shortcut:** receive URLs and Safari pages from the Share Sheet; Get
  URLs from Input; URL Encode; Text `https://<sous host>/import?url=[Encoded
  URL]`; Open URLs. Tailscale must be connected and Safari signed in through
  the gateway.

- **Siri Shortcut ("Save to Sous"):** receive URLs and Safari pages; Get URLs
  from Input (Ask For URLs if there is no input); Get Contents of URL
  `<sous>/v1/recipes/import`, POST, JSON body `{url}`, with a header for the
  mode below; Get Dictionary Value `summary`; if it has a value, Speak Text it,
  else speak "Sous said: " and the contents. After that "Hey Siri, Save to
  Sous" works. The `/import` page shows these steps with the right header.
  - *Under Outpost:* `Authorization: Bearer opat_…`, a token made on the
    Outpost machine with `outpost token create <username> --name siri --app
    sous` (optionally `--expires 90d`). The secret is shown once. List with
    `outpost token list <username>`, revoke with `outpost token revoke <id>`.
    The token can do anything that person can do in Sous, so revoke it if the
    phone is lost.
  - *On its own:* `X-Sous-Person: <username>`, with the trust model of §5 route
    2. Sous binds `127.0.0.1`, so the phone reaches it through `tailscale
    serve` or a reverse proxy. `SOUS_REQUIRE_GATEWAY=true` closes this route.

A PWA `share_target` is not built, but the `url`, `text` and `title`
parameters are what one would send.

### Out of scope

- **Paywall handling.** Signed-in fetching, cookies and site logins are for a
  later spec. Until then Sous never sends cookies.
- **Re-import.** There is no "refresh from source". Updating a duplicate with
  a fresh read (above) is the whole of it.
- Importing tags, collections or ratings from a page.
