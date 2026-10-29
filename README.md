# Sous

A household recipe manager, run as a service under [Outpost](outpost-spec.md).
Web UI, REST API, and MCP from one Go binary.

Status: the basics run. The service, REST API, MCP, and web UI are built to
[`spec.md`](spec.md); the feature specs it lists in §13 come next.

## Shape

- **One Go binary**, conforming to [`outpost-spec.md`](outpost-spec.md): binds
  `127.0.0.1`, port from `--port` or `OUTPOST_PORT`, health at `GET /up`,
  REST under `/v1` with `GET /v1/openapi.json`, MCP at `/mcp`, web UI embedded.
  Foreground only, clean SIGTERM shutdown, logs to stdout/stderr.
- **State** lives entirely in `OUTPOST_STORAGE_DIR`: SQLite plus an images
  directory, so `outpost backup`/`restore` cover everything.

## Data

- **Recipes:** title, source link, yield, prep/cook/total time, notes,
  nutrition, photos.
- **Ingredients** are parsed into quantity / unit / item / note, with the
  original line always kept alongside.
- **Steps** are stored individually, so cook mode can walk them and start timers.
- **Tags** (free-form) and **collections** (hand-ordered groups).
- **Household:** one shared library. Favorites, ratings, notes, and cook
  history are per person. Every change is attributed to whoever made it.

## Features

- Search by title, tag, and ingredient ("what can I make with…")
- Cook mode: step-by-step, screen awake, ingredient check-offs, step timers
- Scaling by servings, US ↔ metric conversion
- Weekly meal plan → combined, de-duplicated shopping list grouped by aisle
- Import from URL via the page's schema.org Recipe data

## Interfaces

- **Web UI:** warm, editorial, photo-forward. Installable PWA. Works offline
  for recipes already opened and for the shopping list, and syncs on reconnect.
- **REST and MCP** have parity. MCP agents get full read/write, attributed
  like any other change.

## Running the service

```sh
cd web && npm ci && npm run build && cd ..   # build the UI into the binary
OUTPOST_STORAGE_DIR=./.dev-data go run ./cmd/sous --port 8099
```

Then open http://127.0.0.1:8099. Everything Sous keeps lives in
`OUTPOST_STORAGE_DIR`; `.dev-data/` is gitignored. Without the UI build, the
binary still runs and serves a page saying how to build it.

```sh
go test -short ./...   # unit and API tests
go test ./...          # plus the Outpost conformance suite, which builds the binary
```

MCP clients connect to `http://127.0.0.1:8099/mcp` and name themselves with
an `X-Sous-Person` header, unless they come through the Outpost gateway.

## Developing the web UI

The UI lives in `web/` (Svelte 5 + Vite) and builds into `internal/ui/dist`
for the Go binary to embed. Until the service exists, the Vite dev server
answers `/v1` with a mock API (`web/mock/`).

```sh
cd web && npm install && npm run dev
```

The mock serves a few sample recipes from `web/mock/fixtures.json`. To design
against the real library instead, extract the Mela export into the
gitignored `web/mock-local/`:

```sh
uv run --no-project scripts/mela-sample/sample.py Recipes.melarecipes web/mock-local
```

## Mela import

A one-time, throwaway script loads the Mela export through the REST API. The
app itself carries no Mela code. The export is not committed.

```sh
uv run scripts/mela-import/mela_import.py Recipes.melarecipes \
    --url http://127.0.0.1:8099 --person scott --name Scott
```

`--dry-run` reads and maps the export without sending anything. Re-running
skips what's already there, so an interrupted import can just be run again.
Details are in [`spec.md`](spec.md) §11.

## Identity

Outpost's login gateway supplies the caller as a signed `X-Outpost-Identity`
token, verified against `OUTPOST_JWKS_URL`. Without the gateway, a person is
chosen on each device. Details are in [`spec.md`](spec.md) §5.
