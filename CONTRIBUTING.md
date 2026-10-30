# Contributing to Sous

Thanks for helping. Sous is a household recipe manager: one Go binary with an
embedded Svelte UI, a REST API and MCP. It runs under
[Outpost](https://github.com/sroberts/outpost) or on its own, and both have to
keep working.

Read [`spec.md`](spec.md) before changing anything structural, and
[`outpost-spec.md`](outpost-spec.md) before changing how the process starts,
stops, logs or stores data. Spec §12 maps each Outpost requirement to the code
that meets it.

## Workflow: GitHub Flow

1. **Branch** off an up-to-date `main` with a descriptive name
   (`fix-ingredient-ranges`, `import-paywall-notice`).
2. **Build** on the branch, committing in focused steps.
3. **Open a pull request** and fill in the template.
4. **Merge** only after the maintainer approves that PR.

Nothing is committed straight to `main`, docs included.

## Building and testing

You need Go (the version in `go.mod`), Node LTS for the UI, and
[`uv`](https://docs.astral.sh/uv/) for the Python scripts.

```sh
cd web && npm ci && npm run build && cd ..   # build the UI into the binary
go run ./cmd/sous --port 8099 --data ./.dev-data
```

Before opening a PR, run what CI runs:

```sh
(cd web && npm run check)                     # svelte-check
test -z "$(gofmt -l .)"                       # formatting
go vet ./... && go vet -tags live ./internal/importer
SOUS_REQUIRE_UI=1 go test -race ./...         # unit, API and Outpost conformance tests
```

For UI work, `cd web && npm run dev` serves the app against a mock API.
`MOCK_OUTPOST=<username> npm run dev` makes the mock act as the Outpost
login gateway.

The importer's tests run offline against saved fixtures. To check the real
recipe sites, run `go test -tags live -run Live -v -count=1 ./internal/importer`.
The README explains re-recording fixtures.

## What CI and the scheduled jobs check

- **Every push and PR** (`ci.yml`): the UI build and svelte-check, gofmt,
  go vet, the parser golden, the test suite with the race detector, and a
  **coverage floor**: total Go coverage below `COVERAGE_FLOOR` fails the
  build. Raise the floor as coverage grows; never lower it to make a PR
  pass. `plumbline.yml` fails if the repo drops below ACMM level 3.
- **Nightly** (`nightly.yml`): the full coverage suite, `govulncheck` and
  `npm audit` for known vulnerabilities, and the live importer check against
  the 25 real sites, which reports rather than fails.
- **Weekly** (`flaky-tests.yml`): runs the suite five times and fails on any
  test that both passes and fails, or fails every time. Known flakes go in `flaky-tests.json`
  with a tracking issue, and come out once fixed.
- **Weekly** (`acceptance-rates.yml`): recounts how often AI-authored PRs
  are merged and proposes the updated `acceptance-rates.json` as a PR.
  A falling rate, or an area where agent PRs keep being closed, is the cue
  to tighten instructions or keep that work supervised.

## Rules that matter here

- **Existing tests are not changed without the maintainer's explicit
  approval.** Adding tests is always welcome. If an existing test looks
  wrong, say so in the PR; don't edit it to make your change pass. Generated
  golden files (`openapi.golden.json`, `parse_golden.json`) count as tests.
- **Personal data is never committed.** That includes a Mela export
  (`*.melarecipes`), anything in `web/mock-local/`, and `*.local.json`.
- **Importer fixtures hold only the extracted recipe data and the link back**,
  never a whole page.
- **Python tooling uses `uv`** (`uv run …`), not a hand-made virtualenv.
- **New dependencies need a reason** in the PR. Sous builds without cgo, so
  pure-Go libraries only.
- **Match the surrounding code**: its comment density, naming and idiom. UI
  changes follow [`DESIGN.md`](DESIGN.md) and its tokens rather than adding new
  colours or type.

## Commits and pull requests

Commit subjects are imperative and say what the change does ("Add a --data
flag for running without Outpost"). An area prefix is fine (`safefetch:`),
but there are no `feat:`/`fix:` types. The body explains why. `.gitmessage`
has the template: `git config commit.template .gitmessage`.

A pull request says what changed and why, how it was tested, and, in its own
section, what nobody has verified. A PR is reviewed for:

- behaviour that still works both with Outpost and on its own;
- tests for new behaviour, and the full CI suite passing;
- docs (`README.md`, `spec.md`) updated when behaviour or configuration
  changes;
- no personal data, secrets or whole third-party pages in the diff.

Common reasons a PR is sent back: an edited existing test, a change that only
works under one way of running Sous, or a UI change that breaks at phone
width (check 390px).

## Releasing

Sous uses [semantic versioning](https://semver.org/). Before 1.0, a minor
release may include breaking changes; call them out in the changelog.

1. On a branch, move the `[Unreleased]` entries in `CHANGELOG.md` under the
   new version with today's date, and merge it.
2. Tag `main` and push the tag: `git tag -a v0.3.0 -m "Sous v0.3.0" && git push origin v0.3.0`.
3. `release.yml` builds the binaries, checksums and `outpost.toml`, and
   publishes the release with notes grouped by label (`.github/release.yml`).
4. Edit the release to add the changelog's highlights and any upgrade steps
   at the top.

## Reporting problems

Security problems go through private reporting; see [SECURITY.md](SECURITY.md).
For everything else, open an issue with the Sous version (`sous --version`),
how you run it (with Outpost, gateway on or off, or on its own) and the steps
to reproduce. For a recipe site that didn't import, include the link and what
was missing, but don't paste paywalled recipe text.
