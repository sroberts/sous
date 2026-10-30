# /// script
# requires-python = ">=3.12"
# ///
"""Measure how often AI-authored pull requests are accepted.

Reads every pull request with the gh CLI and writes acceptance-rates.json:
merged versus closed-without-merging, for AI-authored PRs (their body
carries the Claude Code attribution) and for the rest, overall and by area
label. Open PRs are counted but don't affect a rate.

    uv run --no-project scripts/acceptance/acceptance.py [--out acceptance-rates.json]

Needs `gh` signed in (GH_TOKEN in CI) and run inside the repository.
"""

import argparse
import json
import subprocess
import sys

AGENT_MARKERS = ("Generated with [Claude Code]", "claude.ai/code/session_")
# Labels that name an area of the code; the first one found wins.
AREAS = ("importer", "ui", "api", "mcp", "outpost", "identity", "documentation", "project-setup")


def pull_requests():
    out = subprocess.run(
        ["gh", "pr", "list", "--state", "all", "--limit", "1000",
         "--json", "number,state,body,labels,createdAt"],
        check=True, capture_output=True, text=True,
    ).stdout
    return json.loads(out)


def tally():
    return {"merged": 0, "closed_unmerged": 0, "open": 0, "acceptance_rate": None}


def add(t, state):
    key = {"MERGED": "merged", "CLOSED": "closed_unmerged", "OPEN": "open"}[state]
    t[key] += 1


def finish(t):
    decided = t["merged"] + t["closed_unmerged"]
    t["acceptance_rate"] = round(t["merged"] / decided, 3) if decided else None
    return t


def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--out", default="acceptance-rates.json")
    args = ap.parse_args()

    prs = pull_requests()
    authors = {"agent": tally(), "human": tally()}
    areas = {}
    for pr in prs:
        body = pr.get("body") or ""
        who = "agent" if any(m in body for m in AGENT_MARKERS) else "human"
        add(authors[who], pr["state"])
        if who == "agent":
            labels = {l["name"] for l in pr.get("labels", [])}
            area = next((a for a in AREAS if a in labels), "unlabelled")
            add(areas.setdefault(area, tally()), pr["state"])

    report = {
        "about": (
            "Acceptance of AI-authored pull requests: merged / (merged + closed "
            "without merging). Written by scripts/acceptance/acceptance.py from a "
            "weekly workflow; review the rate before widening what agents do "
            "unattended, and look at rejected areas first."
        ),
        "pull_requests": len(prs),
        "by_author": {k: finish(v) for k, v in authors.items()},
        "agent_by_area": {k: finish(v) for k, v in sorted(areas.items())},
    }
    with open(args.out, "w", encoding="utf-8") as f:
        json.dump(report, f, indent=2)
        f.write("\n")
    agent = report["by_author"]["agent"]
    print(f"{len(prs)} PRs; agent acceptance {agent['acceptance_rate']} "
          f"({agent['merged']} merged, {agent['closed_unmerged']} rejected, {agent['open']} open)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
