# /// script
# requires-python = ">=3.12"
# dependencies = ["pillow>=11"]
# ///
"""Import a Mela export into a running Sous, through its REST API.

A one-time, throwaway script (spec.md §11). Sous itself has no Mela code;
delete this once the import is done. Usage:

    uv run scripts/mela-import/mela_import.py Recipes.melarecipes \\
        --url http://127.0.0.1:8099 --person scott [--name Scott] [--dry-run]

Everything is attributed to --person, who must exist in Sous; --name creates
them first if they don't. Favorites and want-to-cook flags become that
person's. Re-running skips recipes already present (same source link and
title), so an interrupted import can simply be run again.
"""

import argparse
import base64
import io
import json
import re
import sys
import urllib.error
import urllib.request
import zipfile
from dataclasses import dataclass, field

from PIL import Image

ACCEPTED = {"image/jpeg", "image/png", "image/webp", "image/heic"}

# ---------------------------------------------------------------- mapping

_UNIT = {"h": 60, "hr": 60, "hrs": 60, "hour": 60, "hours": 60,
         "m": 1, "min": 1, "mins": 1, "minute": 1, "minutes": 1}
_NUM = r"\d+(?:\.\d+)?(?:\s+\d+/\d+)?|\d+/\d+|\d*[½¼¾⅓⅔]"
_PART = re.compile(rf"({_NUM})\s*({'|'.join(sorted(_UNIT, key=len, reverse=True))})\.?(?![a-z])")
_JOINER = re.compile(r"^[\s,]*(?:and\s+)?$")
_VULGAR = {"½": 0.5, "¼": 0.25, "¾": 0.75, "⅓": 1 / 3, "⅔": 2 / 3}


def _number(s: str) -> float:
    total = 0.0
    for piece in s.split():
        if piece[-1] in _VULGAR:
            total += _VULGAR[piece[-1]] + (float(piece[:-1]) if piece[:-1] else 0)
        elif "/" in piece:
            a, b = piece.split("/")
            total += float(a) / float(b)
        else:
            total += float(piece)
    return total


def _duration(text: str | None) -> float | None:
    t = (text or "").strip().lower()
    t = re.sub(r"^[a-z/ ]*time:\s*", "", t)  # "Time: 20 minutes", "prep/total time: 30 min."
    t = re.sub(r"^about\s+", "", t)
    if re.fullmatch(r"\d+", t):
        return float(t)
    if m := re.fullmatch(r"(\d+):([0-5]\d)", t):  # "1:10"
        return int(m.group(1)) * 60 + int(m.group(2))
    total, pos = 0.0, 0
    for m in _PART.finditer(t):
        if not _JOINER.match(t[pos:m.start()]):
            return None
        total += _number(m.group(1)) * _UNIT[m.group(2)]
        pos = m.end()
    if pos == 0 or t[pos:].strip(" ."):
        return None
    return total


def minutes(text: str | None) -> int | None:
    """Minutes for a plain duration ('1 hour 15 minutes', '1h', '45', '1:10',
    'about 20 min'). None for zero, and for anything with more meaning than a
    number: ranges, 'plus overnight', 'at least', notes in parentheses. Those
    keep their wording in time_text instead."""
    d = _duration(text)
    return round(d) or None if d is not None else None


def time_text(d: dict) -> str | None:
    """The times that didn't parse, as written. Labelled unless it's only the
    total, which reads fine on its own."""
    labels = {"prepTime": "Prep", "cookTime": "Cook", "totalTime": "Total"}
    loose = [(k, d[k].strip()) for k in labels
             if (d.get(k) or "").strip() and _duration(d[k]) is None]
    if not loose:
        return None
    if [k for k, _ in loose] == ["totalTime"]:
        return loose[0][1]
    return " · ".join(f"{labels[k]}: {v}" for k, v in loose)


def lines(block: str | None) -> list[str]:
    return [ln.strip() for ln in (block or "").splitlines() if ln.strip()]


def unnumber(steps: list[str]) -> list[str]:
    """Strip '1 Heat the oven…' numbering, which Mela keeps from some sites.
    Only when the non-heading steps are numbered 1, 2, 3… in order, so a
    step that merely starts with a quantity ('2 eggs go in next') is safe."""
    body = [s for s in steps if not s.startswith("#")]
    nums = [re.match(r"(\d+)\s+(?=\S)", s) for s in body]
    if len(body) < 2 or not all(nums) or [int(m.group(1)) for m in nums] != list(range(1, len(body) + 1)):
        return steps
    return [s if s.startswith("#") else re.sub(r"^\d+\s+", "", s, count=1) for s in steps]


def recipe_body(d: dict, images: list[str]) -> dict:
    def text(key: str) -> str | None:
        return (d.get(key) or "").strip() or None

    tags = list(dict.fromkeys(t.strip() for t in d.get("categories") or [] if t.strip()))
    return {k: v for k, v in {
        "title": text("title") or "Untitled",
        "source_url": text("link"),
        "description": text("text"),
        "notes": text("notes"),
        "yield_text": text("yield"),
        "prep_minutes": minutes(d.get("prepTime")),
        "cook_minutes": minutes(d.get("cookTime")),
        "total_minutes": minutes(d.get("totalTime")),
        "time_text": time_text(d),
        "nutrition_text": text("nutrition"),
        "tags": tags,
        "images": images,
        "ingredients": lines(d.get("ingredients")),
        "steps": unnumber(lines(d.get("instructions"))),
    }.items() if v not in (None, [])}


def key(title: str | None, link: str | None) -> tuple[str, str]:
    return ((link or "").strip(), (title or "Untitled").strip().casefold())


# ---------------------------------------------------------------- the API

class ApiError(Exception):
    def __init__(self, status: int, body: bytes):
        self.status = status
        try:
            self.code = json.loads(body)["error"]["code"]
            self.message = json.loads(body)["error"]["message"]
        except (ValueError, KeyError, TypeError):
            self.code, self.message = "", body[:200].decode(errors="replace")
        super().__init__(f"{status} {self.code}: {self.message}")


@dataclass
class Sous:
    url: str
    person: str

    def call(self, method: str, path: str, body=None, raw: bytes | None = None, person=True):
        headers = {"Content-Type": "application/octet-stream" if raw is not None else "application/json"}
        if person:
            headers["X-Sous-Person"] = self.person
        data = raw if raw is not None else (json.dumps(body).encode() if body is not None else None)
        req = urllib.request.Request(self.url + path, data=data, method=method, headers=headers)
        try:
            with urllib.request.urlopen(req, timeout=60) as r:
                b = r.read()
                return json.loads(b) if b else None
        except urllib.error.HTTPError as e:
            raise ApiError(e.code, e.read()) from None

    def titles(self) -> dict[str, list[str]]:
        """Recipe ids by case-folded title, across the whole library."""
        out: dict[str, list[str]] = {}
        cursor = None
        while True:
            page = self.call("GET", "/v1/recipes?limit=200" + (f"&cursor={cursor}" if cursor else ""))
            for r in page["items"]:
                out.setdefault(r["title"].strip().casefold(), []).append(r["id"])
            cursor = page.get("next_cursor")
            if not cursor:
                return out


class Present:
    """Which Mela recipes Sous already has, by source link plus title. List
    summaries carry only the host, so a title match fetches the candidates
    to compare links; a fresh library costs one paged list and nothing more."""

    def __init__(self, sous: Sous | None):
        self.sous = sous
        self.by_title = sous.titles() if sous else {}
        self.added: set[tuple[str, str]] = set()

    def has(self, title: str | None, link: str | None) -> bool:
        k = key(title, link)
        if k in self.added:
            return True
        for rid in self.by_title.get(k[1], []):
            if (self.sous.call("GET", f"/v1/recipes/{rid}").get("source_url") or "").strip() == k[0]:
                return True
        return False

    def add(self, title: str | None, link: str | None) -> None:
        self.added.add(key(title, link))


# ---------------------------------------------------------------- images

def upload(sous: Sous, data: bytes, report: "Report") -> str | None:
    try:
        return sous.call("POST", "/v1/images", raw=data)["hash"]
    except ApiError as e:
        if e.status != 415:
            raise
    # Sous takes JPEG, PNG, WebP and HEIC. Mela holds the odd TIFF or GIF.
    try:
        img = Image.open(io.BytesIO(data))
        img.seek(0)
        out = io.BytesIO()
        if img.mode in ("RGBA", "LA", "P"):
            img.convert("RGBA").save(out, "PNG")
        else:
            img.convert("RGB").save(out, "JPEG", quality=92)
    except Exception as e:  # noqa: BLE001 - an unreadable image is skipped, not fatal
        report.skipped_images.append(str(e))
        return None
    report.converted_images += 1
    return sous.call("POST", "/v1/images", raw=out.getvalue())["hash"]


# ---------------------------------------------------------------- the run

@dataclass
class Report:
    imported: int = 0
    present: int = 0
    duplicates: list[str] = field(default_factory=list)
    converted_images: int = 0
    skipped_images: list[str] = field(default_factory=list)
    failed: list[str] = field(default_factory=list)


def ensure_person(sous: Sous, name: str | None) -> None:
    try:
        sous.call("GET", "/v1/me")
        return
    except ApiError as e:
        if e.code != "unknown_person" or not name:
            raise
    made = sous.call("POST", "/v1/people", {"name": name}, person=False)
    if made["username"] != sous.person:
        sys.exit(f"--name {name!r} makes the username {made['username']!r}, not {sous.person!r}; "
                 f"run again with --person {made['username']}")
    print(f"added {name} to the household", file=sys.stderr)


def run(path: str, sous: Sous, dry_run: bool) -> Report:
    report = Report()
    with zipfile.ZipFile(path) as z:
        records = [json.loads(z.read(n)) for n in sorted(z.namelist()) if n.endswith(".melarecipe")]
    present = Present(None if dry_run else sous)
    for i, d in enumerate(records, 1):
        title = (d.get("title") or "Untitled").strip()
        if key(d.get("title"), d.get("link")) in present.added:
            # Mela can hold the same recipe twice; the first copy wins.
            report.duplicates.append(title)
            continue
        if present.has(d.get("title"), d.get("link")):
            report.present += 1
            continue
        try:
            if dry_run:
                recipe_body(d, [])
                present.add(d.get("title"), d.get("link"))
                report.imported += 1
                continue
            hashes = [h for b64 in d.get("images") or []
                      if (h := upload(sous, base64.b64decode(b64), report))]
            made = sous.call("POST", "/v1/recipes", recipe_body(d, list(dict.fromkeys(hashes))))
            mine = {"favorite": bool(d.get("favorite")), "want_to_cook": bool(d.get("wantToCook"))}
            if any(mine.values()):
                sous.call("PUT", f"/v1/recipes/{made['id']}/mine", mine)
            present.add(d.get("title"), d.get("link"))
            report.imported += 1
        except ApiError as e:
            report.failed.append(f"{title}: {e}")
            if e.status in (401, 503):
                break  # every later call would fail the same way
        if i % 50 == 0:
            print(f"{i}/{len(records)}", file=sys.stderr)
    return report


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("export", help="the .melarecipes file")
    ap.add_argument("--url", default="http://127.0.0.1:8099", help="where Sous is running")
    ap.add_argument("--person", required=True, help="username the import is attributed to")
    ap.add_argument("--name", help="create --person with this display name if they don't exist")
    ap.add_argument("--dry-run", action="store_true", help="read and map the export; send nothing")
    a = ap.parse_args()

    sous = Sous(a.url.rstrip("/"), a.person)
    if not a.dry_run:
        ensure_person(sous, a.name)
    r = run(a.export, sous, a.dry_run)

    verb = "would import" if a.dry_run else "imported"
    print(f"{verb} {r.imported}, already present {r.present}, failed {len(r.failed)}")
    for t in r.duplicates:
        print(f"skipped a second copy in the export: {t}")
    if r.converted_images:
        print(f"converted {r.converted_images} images to JPEG or PNG")
    for s in r.skipped_images:
        print(f"skipped an unreadable image: {s}")
    for f in r.failed:
        print(f"failed: {f}")
    sys.exit(1 if r.failed else 0)


if __name__ == "__main__":
    main()
