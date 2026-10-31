# Developing Sous

[`CONTRIBUTING.md`](../CONTRIBUTING.md) has the workflow, the rules, and what
CI checks.

```sh
cd web && npm ci && npm run build && cd ..   # build the UI into the binary
go run ./cmd/sous --port 8099 --data ./.dev-data
```

Then open http://127.0.0.1:8099. `.dev-data/` is gitignored. Without the UI
build, the binary still runs and serves a page saying how to build it.

```sh
go test -short ./...   # unit and API tests
go test ./...          # plus the Outpost conformance suite, which builds the binary
```

The design: [`spec.md`](../spec.md) for the service, and
[`outpost-spec.md`](../outpost-spec.md) for the Outpost contract it keeps:
loopback only, port from `--port` or `OUTPOST_PORT`, health at `GET /up`,
foreground, clean SIGTERM shutdown, logs to stdout. All state lives in the
storage directory, so `outpost backup` and `restore` cover everything.

## The web UI

The UI lives in `web/` (Svelte 5 + Vite) and builds into `internal/ui/dist`
for the Go binary to embed. The Vite dev server answers `/v1` with a mock API
(`web/mock/`); `MOCK_OUTPOST=<username> npm run dev` makes it act as the
Outpost gateway, signed in as that person.

```sh
cd web && npm install && npm run dev
```

The mock serves a few sample recipes from `web/mock/fixtures.json`, plus the
twelve a new library starts with (`internal/samples/data`). To design
against the real library instead, extract the Mela export into the
gitignored `web/mock-local/`:

```sh
uv run --no-project scripts/mela-sample/sample.py Recipes.melarecipes web/mock-local
```

## Checking the importer against real sites

The importer's tests run offline against saved fixtures. To check the real
sites, run the live check, which fetches the 25 links in
`internal/importer/testdata/testset.json` and prints how each one read:

```sh
go test -tags live -run Live -v -count=1 ./internal/importer
```

To re-record the fixtures, then regenerate and review the expected output:

```sh
go test -tags live -run Live -v -count=1 ./internal/importer -record
go test -run TestFixtures ./internal/importer -update
git diff internal/importer/testdata/pages   # review the *.want.json files
```

Fixtures hold only the extracted recipe data and the link back to the source
(the recipe's JSON-LD, or the recipe's microdata element), never a whole page.
A test fails if one contains page markup or grows past 200 KB.
