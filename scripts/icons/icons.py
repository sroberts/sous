# /// script
# requires-python = ">=3.12"
# dependencies = ["pillow"]
# ///
"""Draw the Sous app icons: a chef's toque in Print Black on Thermal White.

The toque is three overlapping circles for the puff, a gap, and a band,
with pleats running up the puff. The SVG and every PNG come from the same
shapes, so they always match. The favicon drops the pleats, which blur at
32px. Usage:

    uv run scripts/icons/icons.py            # writes web/public/icons/
"""

from pathlib import Path

from PIL import Image, ImageDraw

INK = "#141516"
TICKET = "#fafbfc"
OUT = Path(__file__).resolve().parents[2] / "web" / "public" / "icons"

# On a 512 grid, centred. Puff circles (cx, cy, r), the flat base of the puff,
# the band below a gap, and the pleats cut up into the puff.
PUFF = [(176, 244, 84), (336, 244, 84), (256, 192, 100)]
PUFF_BASE = (176, 244, 336, 328)
BAND = (172, 344, 340, 420)
BAND_RADIUS = 12
PLEATS = [(210, 256, 222, 340), (250, 236, 262, 340), (290, 256, 302, 340)]


def scaled(box, s):
    """Scale a box about the centre of the 512 grid."""
    return tuple(256 + (v - 256) * s for v in box)


def circle(c, s):
    cx, cy = (256 + (v - 256) * s for v in c[:2])
    return cx, cy, c[2] * s


def svg(scale, pleats, corner):
    def box(b):
        x0, y0, x1, y1 = scaled(b, scale)
        return x0, y0, x1 - x0, y1 - y0

    parts = [f'<rect width="512" height="512" rx="{corner}" fill="{TICKET}"/>']
    shapes = [f'<circle cx="{cx:g}" cy="{cy:g}" r="{r:g}"/>' for cx, cy, r in (circle(c, scale) for c in PUFF)]
    x, y, w, h = box(PUFF_BASE)
    shapes.append(f'<rect x="{x:g}" y="{y:g}" width="{w:g}" height="{h:g}"/>')
    x, y, w, h = box(BAND)
    shapes.append(f'<rect x="{x:g}" y="{y:g}" width="{w:g}" height="{h:g}" rx="{BAND_RADIUS * scale:g}"/>')
    parts.append(f'<g fill="{INK}">{"".join(shapes)}</g>')
    if pleats:
        cuts = []
        for p in PLEATS:
            x, y, w, h = box(p)
            cuts.append(f'<rect x="{x:g}" y="{y:g}" width="{w:g}" height="{h:g}" rx="{w / 2:g}"/>')
        parts.append(f'<g fill="{TICKET}">{"".join(cuts)}</g>')
    return f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512">{"".join(parts)}</svg>\n'


def png(size, scale, pleats, corner):
    """corner is on the 512 grid, like everything else."""
    k = 4  # draw at 4x and downscale for smooth edges
    n = 512 * k
    img = Image.new("RGBA", (n, n), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)
    d.rounded_rectangle((0, 0, n - 1, n - 1), radius=corner * k, fill=TICKET)
    for c in PUFF:
        cx, cy, r = circle(c, scale)
        d.ellipse(((cx - r) * k, (cy - r) * k, (cx + r) * k, (cy + r) * k), fill=INK)
    d.rectangle(tuple(v * k for v in scaled(PUFF_BASE, scale)), fill=INK)
    d.rounded_rectangle(tuple(v * k for v in scaled(BAND, scale)), radius=BAND_RADIUS * scale * k, fill=INK)
    if pleats:
        for p in PLEATS:
            b = tuple(v * k for v in scaled(p, scale))
            d.rounded_rectangle(b, radius=(b[2] - b[0]) / 2, fill=TICKET)
    return img.resize((size, size), Image.LANCZOS)


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    # "any" icons are drawn larger on a rounded tile; maskable and Apple ones
    # are full-bleed squares the system crops, with the toque well inside the
    # 80% safe circle; the favicon fills its tile and drops the pleats.
    (OUT / "icon.svg").write_text(svg(1.12, True, 96))
    (OUT / "favicon.svg").write_text(svg(1.4, False, 96))
    png(192, 1.12, True, 96).save(OUT / "icon-192.png", optimize=True)
    png(512, 1.12, True, 96).save(OUT / "icon-512.png", optimize=True)
    png(192, 0.92, True, 0).convert("RGB").save(OUT / "maskable-192.png", optimize=True)
    png(512, 0.92, True, 0).convert("RGB").save(OUT / "maskable-512.png", optimize=True)
    png(180, 1.0, True, 0).convert("RGB").save(OUT / "apple-touch-icon.png", optimize=True)
    png(32, 1.4, False, 96).save(OUT / "favicon-32.png", optimize=True)
    for f in sorted(OUT.iterdir()):
        print(f.relative_to(OUT.parents[2]), f.stat().st_size)


if __name__ == "__main__":
    main()
