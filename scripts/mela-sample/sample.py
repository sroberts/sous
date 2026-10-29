# /// script
# requires-python = ">=3.12"
# ///
"""Extract a Mela export into web/mock-local/ for UI development.

Development only: the output is gitignored and is read by the Vite mock
API, never bundled. Usage:

    uv run scripts/mela-sample/sample.py Recipes.melarecipes web/mock-local
"""

import base64
import hashlib
import json
import re
import sys
import zipfile
from datetime import datetime, timedelta, timezone
from pathlib import Path
from urllib.parse import urlparse

MELA_EPOCH = datetime(2001, 1, 1, tzinfo=timezone.utc)

UNITS = {"hour": 60, "hr": 60, "minute": 1, "min": 1}


def minutes(text: str) -> int | None:
    """Parse '1 hour 20 minutes', '45 min', 'about 3 hours'. None if unsure."""
    text = (text or "").lower()
    found = re.findall(r"(\d+(?:\.\d+)?)\s*(hours?|hrs?|minutes?|mins?)\b", text)
    if not found:
        return None
    total = 0.0
    for amount, unit in found:
        total += float(amount) * UNITS[unit.rstrip("s")]
    return round(total) or None


def lines(block: str) -> list[str]:
    return [ln.strip() for ln in (block or "").splitlines() if ln.strip()]


def image_ext(data: bytes) -> str:
    if data[:3] == b"\xff\xd8\xff":
        return "jpg"
    if data[:8] == b"\x89PNG\r\n\x1a\n":
        return "png"
    if data[8:12] == b"WEBP":
        return "webp"
    return "jpg"


def main(src: str, out: str) -> None:
    out_dir = Path(out)
    img_dir = out_dir / "images"
    img_dir.mkdir(parents=True, exist_ok=True)
    recipes = []
    with zipfile.ZipFile(src) as z:
        for name in z.namelist():
            d = json.loads(z.read(name))
            images = []
            for b64 in d.get("images") or []:
                data = base64.b64decode(b64)
                digest = hashlib.sha256(data).hexdigest()
                path = img_dir / f"{digest}.{image_ext(data)}"
                if not path.exists():
                    path.write_bytes(data)
                images.append(digest)
            prep, cook, total = (minutes(d.get(k)) for k in ("prepTime", "cookTime", "totalTime"))
            time_text = next(
                (d.get(k) for k in ("totalTime", "cookTime", "prepTime") if d.get(k) and minutes(d.get(k)) is None),
                None,
            )
            created = MELA_EPOCH + timedelta(seconds=d.get("date") or 0)
            recipes.append({
                "title": d.get("title") or "Untitled",
                "source_url": d.get("link") or None,
                "description": d.get("text") or None,
                "notes": d.get("notes") or None,
                "yield_text": d.get("yield") or None,
                "prep_minutes": prep,
                "cook_minutes": cook,
                "total_minutes": total,
                "time_text": time_text,
                "nutrition_text": d.get("nutrition") or None,
                "tags": d.get("categories") or [],
                "ingredients": lines(d.get("ingredients")),
                "steps": lines(d.get("instructions")),
                "images": images,
                "favorite": bool(d.get("favorite")),
                "want_to_cook": bool(d.get("wantToCook")),
                "created_at": created.isoformat().replace("+00:00", "Z"),
            })
    (out_dir / "recipes.json").write_text(json.dumps(recipes, ensure_ascii=False))
    print(f"wrote {len(recipes)} recipes to {out_dir}", file=sys.stderr)
    print(f"sources: {len({urlparse(r['source_url'] or '').netloc for r in recipes})} hosts", file=sys.stderr)


if __name__ == "__main__":
    if len(sys.argv) != 3:
        sys.exit(__doc__)
    main(sys.argv[1], sys.argv[2])
