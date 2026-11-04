#!/usr/bin/env python3
"""Render docs/social-preview.png (1280x640) for Sous, mirroring social-preview.svg."""
from PIL import Image, ImageDraw, ImageFont

W, H = 1280, 640
img = Image.new("RGB", (W, H), "#dfe3e6")
d = ImageDraw.Draw(img)

LATO = "/usr/share/fonts/truetype/lato/"
F = lambda name, size: ImageFont.truetype(LATO + name, size)

label_f   = F("Lato-Bold.ttf", 19)
word_f    = F("Lato-Black.ttf", 112)
fig_f     = F("Lato-Black.ttf", 58)
min_f     = F("Lato-Bold.ttf", 30)
name_f    = F("Lato-Semibold.ttf", 26)
body_f    = F("Lato-Medium.ttf", 26)

# Steel rail (three-stop gradient)
rail = d.line  # noqa
for x in range(W):
    t = x / W
    if t < 0.5:
        g = t * 2
        r = int(0xD2 + (0x95 - 0xD2) * g)
        gg = int(0xD6 + (0x9B - 0xD6) * g)
        b = int(0xD9 + (0xA0 - 0xD9) * g)
    else:
        g = (t - 0.5) * 2
        r = int(0x95 + (0x5D - 0x95) * g)
        gg = int(0x9B + (0x63 - 0x9B) * g)
        b = int(0xA0 + (0x68 - 0xA0) * g)
    d.line([(x, 88), (x, 151)], fill=(r, gg, b))
d.rectangle([0, 150, W, 156], fill="#5d6368")

# Clip
d.rounded_rectangle([596, 128, 684, 162], radius=2, fill="#5d6368")
d.rounded_rectangle([596, 128, 684, 134], radius=2, fill="#d2d6d9")

# Ticket (thermal white, square corners, red live top edge)
d.rectangle([360, 152, 920, 166], fill="#bb1c14")
d.rectangle([360, 166, 920, 512], fill="#fafbfc")
# dashed perforation
y = 318
x = 384
while x < 896:
    d.line([(x, y), (min(x + 10, 896), y)], fill="#adb4ba", width=2)
    x += 19
# torn bottom edge (zigzag: white tips at y=512, grey valleys cut up into the ticket)
for i in range(14):
    x0 = 360 + i * 40
    if i % 2 == 0:
        # grey notch triangle: paper torn away up to y=489
        d.polygon([(x0, 489), (x0 + 28, 489), (x0 + 14, 512)], fill="#dfe3e6")
    else:
        # white tip extending to y=512
        d.polygon([(x0, 489), (x0 + 28, 489), (x0 + 14, 512)], fill="#fafbfc")

# Text
d.text((384, 174), "THE PASS  \u00b7  RECIPES", font=label_f, fill="#5f6367")
d.text((382, 220), "Sous", font=word_f, fill="#141516")
d.text((384, 362), "35", font=fig_f, fill="#bb1c14")
d.text((458, 370), "MIN", font=min_f, fill="#141516")
d.text((556, 362), "Shakshuka", font=name_f, fill="#404447")
d.text((384, 428), "The household\u2019s recipes as tickets on a", font=body_f, fill="#404447")
d.text((384, 462), "kitchen pass, clipped to a steel rail.", font=body_f, fill="#404447")

img.save("/mnt/HC_Volume_106549717/work-sous/docs/social-preview.png", "PNG")
print("saved", img.size)
