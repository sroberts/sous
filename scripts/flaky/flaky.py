# /// script
# requires-python = ">=3.12"
# ///
"""Find flaky Go tests in `go test -json` output from repeated runs.

A test is flaky when it both passed and failed on the same code. Tests that
fail every time are broken, not flaky, and are reported separately. Exits 1
when a flaky test isn't listed in the known-flakes file, so a new flake
can't go unnoticed.

    uv run --no-project scripts/flaky/flaky.py test-runs.jsonl \
        --known flaky-tests.json [--summary "$GITHUB_STEP_SUMMARY"]
"""

import argparse
import json
import sys
from collections import defaultdict


def outcomes(path):
    """Map "package TestName" to its pass and fail counts."""
    counts = defaultdict(lambda: {"pass": 0, "fail": 0})
    with open(path, encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line.startswith("{"):
                continue  # build output mixed into the stream
            try:
                ev = json.loads(line)
            except json.JSONDecodeError:
                continue
            if ev.get("Test") and ev.get("Action") in ("pass", "fail"):
                counts[f'{ev["Package"]} {ev["Test"]}'][ev["Action"]] += 1
    return counts


def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("runs", help="go test -json output")
    ap.add_argument("--known", required=True, help="JSON file listing known flaky tests")
    ap.add_argument("--summary", help="append a Markdown report to this file")
    args = ap.parse_args()

    with open(args.known, encoding="utf-8") as f:
        known = {f'{e["package"]} {e["test"]}' for e in json.load(f).get("flaky", [])}

    counts = outcomes(args.runs)
    if not counts:
        print("no test results found", file=sys.stderr)
        return 2
    flaky = sorted(t for t, c in counts.items() if c["pass"] and c["fail"])
    broken = sorted(t for t, c in counts.items() if c["fail"] and not c["pass"])
    new = [t for t in flaky if t not in known]
    fixed = sorted(known - set(flaky))

    lines = [f"### Flaky-test analysis: {len(counts)} tests"]
    for title, tests in (
        ("New flaky tests (fail this run)", new),
        ("Known flaky tests seen again", [t for t in flaky if t in known]),
        ("Failed every run (broken, not flaky)", broken),
        ("Listed as flaky but stable this week (consider removing)", fixed),
    ):
        if tests:
            lines.append(f"\n**{title}**\n")
            lines += [f"- `{t}` ({counts[t]['pass']} pass, {counts[t]['fail']} fail)" if t in counts else f"- `{t}`" for t in tests]
    if not (flaky or broken):
        lines.append("\nNo flaky or failing tests.")
    report = "\n".join(lines) + "\n"
    print(report)
    if args.summary:
        with open(args.summary, "a", encoding="utf-8") as f:
            f.write(report)

    for t in new:
        print(f"::error::new flaky test: {t} — fix it, or add it to {args.known} with a tracking issue")
    return 1 if new or broken else 0


if __name__ == "__main__":
    sys.exit(main())
