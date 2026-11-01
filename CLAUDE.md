# Sous

A household recipe manager: web UI, REST API, and MCP from one Go binary. It
runs as a service under Outpost or on its own; keep both working.

- [`README.md`](README.md): what Sous is and how to get it running. The
  guides in [`docs/`](docs) cover the rest for people using Sous; keep both
  in step with behaviour.
- [`spec.md`](spec.md): the basics design spec. Read it before changing
  anything structural.
- [`DESIGN.md`](DESIGN.md) and [`PRODUCT.md`](PRODUCT.md): the UI's visual
  system and who it's for. UI work follows them rather than inventing a new
  look.
- [`CONTRIBUTING.md`](CONTRIBUTING.md): workflow, CI commands, and review rules.
- [`outpost-spec.md`](outpost-spec.md): the Outpost service spec Sous must
  conform to. Don't change behaviour in a way that breaks its checklist
  (`spec.md` §12 maps each item).

## Workflow: GitHub Flow

All development follows GitHub Flow. No exceptions, including docs.

1. **Branch** off an up-to-date `main` with a descriptive name
   (`identity-gateway`, `fix-ingredient-ranges`).
2. **Build** on the branch, committing in focused steps.
3. **PR**: push the branch and open a pull request with `gh pr create`.
4. **Merge only with user approval.** Stop at the open PR and wait. Approval
   for one PR does not carry over to the next.

Never commit or push directly to `main`.

## Conventions

- **Go** for the service (see `spec.md` §1 and §10 for layout and
  dependencies).
- **Python** (the Mela import script, any tooling) is managed with `uv`.
- **Tests** are not changed without the user's explicit authorization. If a
  test looks wrong, say so; don't edit it. Golden files
  (`openapi.golden.json`, `parse_golden.json`) count as tests.
- **Personal data is never committed:** the Mela export (`*.melarecipes`),
  `web/mock-local/`, and `*.local.json`.
- **User-facing changes** get an entry under `[Unreleased]` in
  [`CHANGELOG.md`](CHANGELOG.md).
- **Sample recipes** (`internal/samples/`) use only public-domain text and
  public-domain or CC0 images, recorded in `SOURCES.md`. Recipes inspired by
  living or recent chefs are written fresh, never copied or closely
  paraphrased from their books.
