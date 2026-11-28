# /// script
# requires-python = ">=3.11"
# ///
"""Draft the five hand-built gate graphs into a running Sous, for review
flows: each sample recipe's structure goes through POST
/v1/recipes/{id}/graph/draft, so Sous's decision model labels it as it
would an assistant's draft. Sous needs its decision model (Laya on Ollama).

    uv run scripts/seed_step_graphs.py http://127.0.0.1:8099 ann

The second argument is the person to draft as (created if missing).
"""

import glob
import json
import os
import sys
import time
import urllib.error
import urllib.request

ROOT = os.path.join(os.path.dirname(__file__), "..", "internal")


def main(base: str, person: str) -> None:
    def req(method, path, body=None, named=True):
        headers = {"Content-Type": "application/json"}
        if named:
            headers["X-Sous-Person"] = person
        r = urllib.request.Request(base + path, data=None if body is None else json.dumps(body).encode(), method=method,
                                   headers=headers)
        try:
            return json.load(urllib.request.urlopen(r, timeout=600))
        except urllib.error.HTTPError as e:
            return {"error": json.loads(e.read() or b"{}").get("error", {})}

    # A fresh Sous adds its sample recipes as it starts: wait for them. The
    # person is made first, unnamed, as Sous refuses a name it doesn't know.
    for _ in range(60):
        page = req("GET", "/v1/recipes?limit=200", named=False)
        if page.get("items"):
            break
        time.sleep(1)
    else:
        sys.exit("Sous didn't become ready with recipes")
    req("POST", "/v1/people", {"name": person}, named=False)
    titles = {r["title"]: r["id"] for r in page["items"]}
    for f in sorted(glob.glob(os.path.join(ROOT, "recipes", "testdata", "graphs", "*.json"))):
        fx = json.load(open(f))
        title = json.load(open(os.path.join(ROOT, "samples", "data", fx["sample"] + ".json")))["title"]
        if title not in titles:
            print(f"{title}: not in this library, skipped")
            continue
        r = req("GET", "/v1/recipes/" + titles[title])
        items = []
        for it in fx["items"]:
            d = {"id": it["id"], "kind": it.get("kind", "ingredient"), "label": it["label"]}
            if it.get("line"):
                d["line"] = next(i + 1 for i, x in enumerate(r["ingredients"]) if it["line"] in x["raw"])
            items.append(d)
        nodes = []
        for n in fx["nodes"]:
            find = n["implied_step"] if n.get("implied") else n["quote"]
            step = next(i + 1 for i, s in enumerate(r["steps"]) if find in s["text"])
            nodes.append({"id": n["id"], "step": step, "quote": "" if n.get("implied") else n["quote"], "text": n["text"],
                          "inputs": n.get("in", []), "outputs": n.get("out", []), "implied": n.get("implied", False)})
        res = req("POST", f"/v1/recipes/{r['id']}/graph/draft",
                  {"recipe_version": r["version"], "mode": "apply", "force": True, "serve": fx["serve"], "items": items, "nodes": nodes})
        if "error" in res:
            print(f"{title}: {res['error'].get('message')}")
        else:
            print(f"{title}: {res['graph']['graph']['status']}, {res['graph']['review_count']} to review")


if __name__ == "__main__":
    main(sys.argv[1].rstrip("/") if len(sys.argv) > 1 else "http://127.0.0.1:8099", sys.argv[2] if len(sys.argv) > 2 else "ann")
