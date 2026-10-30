---
name: plumbline
description: Use plumbline to assess this repo's AI Codebase Maturity Model (ACMM) level and apply scaffolded fixes. Invoke when the user asks about AI coding readiness, missing instruction files (CLAUDE.md, AGENTS.md, copilot-instructions, .cursorrules, .windsurfrules), CI quality gates, contributor guides, PR templates, commit conventions, or "what should we add next to make this repo better for AI-driven development?"
---

# plumbline workflow

Plumbline is a deterministic Go CLI that scans a repository for AI coding
readiness signals and reports its ACMM level (1–5). Detection is
deterministic — no LLM calls, no network. The CLI is the load-bearing
interface; an interactive Bubble Tea TUI is also available on terminals.

## When to invoke

- User asks "is this repo ready for AI coding?" or about AI maturity.
- User asks about adding CLAUDE.md / AGENTS.md / copilot-instructions /
  .cursorrules / .windsurfrules.
- User wants to improve PR templates, contributor guides, or commit rules.
- User asks "what's the next thing to add for better AI workflows?"
- A plumbline CI gate is failing and you need to know why.

## Recommended call sequence

1. **Get the verdict (machine-readable).**

   ```
   plumbline --json
   ```

   Parse `verdict.level` (1–5), `verdict.level_scores` (per-level
   averages), and `verdict.next_gap` (signals at L+1 not yet Found).
   `next_gap` is empty at L5 — there is no L6, so read `level_scores`
   and the per-signal statuses to find what work remains.

2. **For each gap, get evidence + a fix recipe.**

   ```
   plumbline inspect <signal-id> --json
   ```

   Returns a `signal-result` object with `status`, `score`,
   `confidence`, `evidence` (file paths + excerpts), `notes`
   (the "why"), and `fix_hint` (the "how to fix").

   `plumbline explain <signal-id>` prints the detection rule and
   rationale *without* scanning — cheaper when you only need the "what".

3. **Preview a scaffolded fix; apply only with explicit confirmation.**

   ```
   plumbline fix <signal-id>           # dry-run, prints the plan
   plumbline fix <signal-id> --apply   # actually writes
   ```

   **Only some signals have fixers** — the L2 scaffolding ones. Every other
   signal exits 2 with "has no fixer". That exit code is the authoritative
   check, so run the dry-run rather than guessing from the ID. Signals
   without a fixer need judgment a scaffold can't supply (which SDK, which
   triage rules); read `fix_hint` and write the artifact yourself.

   Inputs can be supplied with repeatable `--input KEY=VALUE` pairs.
   Fixers refuse to overwrite existing files; if the target already exists
   they append a marked block instead.

4. **Track maturity over time.**

   ```
   plumbline snapshot                         # writes .plumbline.toon
   plumbline diff base.toon head.toon --json  # verdict delta
   ```

   `snapshot` is reproducible by default (volatile `scanned_at` /
   `repo` fields normalized), so a committed `.plumbline.toon` can be
   guarded with a CI drift gate. `diff` computes a PR delta against that
   committed base without re-assessing both sides.

5. **Wire it into CI, and badge the result.**

   ```
   plumbline install-ci --fail-below 3        # dry-run
   plumbline install-ci --fail-below 3 --apply
   plumbline badge                            # writes .plumbline-badge.svg
   ```

   `install-ci` writes `.github/workflows/plumbline.yml`
   (`--variant full|gate|badge`). `badge` renders a
   self-contained SVG the README references with a relative path:
   `![ACMM level](.plumbline-badge.svg)`. Both outputs are
   byte-stable for an unchanged verdict, so CI can regenerate and fail on
   a diff.

   `plumbline install-dependabot` is the companion: it writes
   `.github/dependabot.yml` with one update block per dependency
   manifest actually present, plus `github-actions` when the repo
   has workflows. It sits **outside** the assessment loop: it is not in
   `next_gap`, no signal detects it, and it moves no verdict.
   Offer it as dependency hygiene when a user asks about stale actions or
   dependencies — never as a way to raise a level, and never imply
   re-assessing will show a change.

   **This is the only workflow plumbline generates** — the one that runs
   plumbline. Do not expect it to scaffold a coverage gate, nightly
   suite, or triage automation; those remain advisory via `fix_hint`.
   It earns the repo no credit — `l3.build-lint-gate` ignores
   it — so never tell a user that installing it means their repo gates
   its own build.

6. **Discover the catalog.**

   ```
   plumbline signals --json
   plumbline schema verdict
   plumbline schema signal-result
   plumbline help <topic>
   ```

## Stable contracts

- **Signal IDs** are stable across patch releases:
  - L2: `l2.agent-instructions`, `l2.contributor-guide`, `l2.pr-template`, `l2.commit-rules`
  - L3: build-lint-gate, coverage-gate, coverage-suite, nightly-compliance, flaky-analysis, error-monitoring, user-feedback, acceptance-tracking, metrics-acted-on
  - L4: self-modifying-config, auto-triage, threshold-block, worktree-agents, error-recovery, measurement-backed
  - L5: issue-to-pr, self-improvement, docs-from-prs, multi-repo-orchestration
- **JSON Schemas** (draft 2020-12) for verdict / signal-result / event / config
  via `plumbline schema <name>`.
- **Exit codes**: 0 ok, 1 gate-failed (`--fail-below`), 2 cannot-run, 3 config-error.
- **Report formats**: `--report toon|json|yaml|markdown|sarif`. TOON is the
  default CLI output; `--json` is shorthand for `--report json`.
- **Read-only by default.** Only `plumbline fix --apply`,
  `plumbline install-skill --apply`, and
  `plumbline install-ci --apply` write inside the target repo.
  `plumbline badge` writes only its own SVG, at a path you name.

## Notes for agents

- Plumbline collapses CLAUDE.md / AGENTS.md / copilot-instructions /
  .cursorrules / .windsurfrules into ONE signal (`l2.agent-instructions`).
  Whichever the team uses is fine; don't suggest adding all of them.
- The four-step partial-credit rubric is fixed: 0.0 / 0.33 / 0.67 / 1.0.
  Don't propose a "0.5" — it's not in the rubric.
- `--min-confidence high` gates *partial* credit only: a signal scoring a
  full 1.0 is always honored no matter its confidence. On a repo whose
  Found signals all score 1.0, raising the gate changes nothing — that is
  by design, not a bug.
- A signal that genuinely doesn't apply (error monitoring for a CLI with no
  runtime, say) belongs in `.plumbline.yml` as `enabled: false`, not
  faked. Disabled signals drop out of the level average rather than
  scoring 0, so the level score rises.
- Scope a run with `--level N`, `--family <name>`,
  `--include-signal`, or `--exclude-signal` (all repeatable).
- Workflow signals (L3+) parse GitHub Actions YAML only in MVP; other
  CI systems are deferred behind `--ci-system`.
- `plumbline install-ci --list` shows the workflow variants; the
  TUI has the same picker under `[w]`, and `[i]` for the
  skill installer.
- `plumbline install-skill --list` shows the eight supported agent tools
  (claude, cursor, codex, gemini, opencode, windsurf, cline, copilot);
  `--global` installs at user scope where the tool documents one.

## When NOT to invoke

- General code review (use the user's preferred review tool).
- Test coverage analysis itself (plumbline detects whether a coverage
  *gate* exists; it doesn't compute coverage).
- Static analysis on application code (use golangci-lint, eslint, etc.).
- Anything that requires running the user's tests / building their app.

## Reference

- Spec: `SPEC.md` in this repo (or in the plumbline repo if you've
  installed it via `go install`).
- Source paper: https://arxiv.org/abs/2604.09388v1 (Andy Anderson,
  "The AI Codebase Maturity Model", IBM Research).
- Topical help: `plumbline help <topic>` (levels, signals, scoring,
  output, config, ci, agents, profiles, compatibility, fix).
