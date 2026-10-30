# Sous

A household recipe manager: web UI, REST API, and MCP from one Go binary. It
runs as a service under Outpost or on its own; keep both working.

- [`README.md`](README.md): what Sous is and the decisions behind it.
- [`spec.md`](spec.md): the basics design spec. Read it before changing
  anything structural.
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
  test looks wrong, say so; don't edit it.
- The Mela export (`*.melarecipes`) is personal data and is never committed.
