#!/usr/bin/env python3
"""Generate the BackupVouch brand files (SVGs and rasters) from one set of constants.

Requirements: pip install fonttools cairosvg pillow
Fonts: IBM Plex Sans (SIL OFL 1.1) taken from the npm package @fontsource/ibm-plex-sans:
  npm i @fontsource/ibm-plex-sans   (files/ibm-plex-sans-latin-400-normal.woff and -600-)
Usage: python3 assets/brand/build.py /path/to/node_modules/@fontsource/ibm-plex-sans/files
Run from the repository root. Output goes next to this script.
"""
import os, sys, io
from fontTools.ttLib import TTFont
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.transformPen import TransformPen

HERE = os.path.dirname(os.path.abspath(__file__))
FONTS = sys.argv[1] if len(sys.argv) > 1 else "node_modules/@fontsource/ibm-plex-sans/files"

# Palette (see docs/BRAND.md)
INK, TEAL, SLATE, MIST, PAPER = "#14212E", "#0E7C73", "#6B7F95", "#C9D3DE", "#F5F7FA"
D_INK, D_TEAL, D_SLATE, D_MIST = "#E8EEF4", "#35C2B5", "#5B6E83", "#34465A"   # on dark
FAV_TEAL, FAV_SLATE = "#12A094", "#7D90A6"                                       # favicon, both themes

# Mark construction (64 x 64 grid): one check polyline, repeated 3 times, dy apart.
CHECK = [(8, 23), (21, 36), (56, 9)]
DY, SW = 9, 6
FAV_CHECK, FAV_DY, FAV_SW = [(8, 22), (23, 37), (56, 8)], 15, 12

def poly(pts, dy):
    return "M" + " L".join(f"{x} {y+dy}" for x, y in pts)

def layer(pts, dy, color, sw):
    return (f'<path d="{poly(pts, dy)}" fill="none" stroke="{color}" stroke-width="{sw}" '
            f'stroke-linejoin="miter" stroke-miterlimit="10" stroke-linecap="butt"/>')

def mark_body(c_back, c_mid, c_front):
    # back layer first, front layer (the verified one) last, on top
    return "".join([layer(CHECK, 2*DY, c_back, SW), layer(CHECK, DY, c_mid, SW), layer(CHECK, 0, c_front, SW)])

def text_path(font_file, text, size, x, baseline, fill):
    f = TTFont(os.path.join(FONTS, font_file))
    gs, cmap, hmtx, upm = f.getGlyphSet(), f.getBestCmap(), f["hmtx"], f["head"].unitsPerEm
    s, cur, d = size / upm, x, []
    for ch in text:
        g = cmap[ord(ch)]
        pen = SVGPathPen(gs, ntos=lambda v: f"{v:.2f}".rstrip("0").rstrip("."))
        gs[g].draw(TransformPen(pen, (s, 0, 0, -s, cur, baseline)))
        d.append(pen.getCommands())
        cur += hmtx[g][0] * s
    return f'<path fill="{fill}" d="{"".join(d)}"/>', cur

def svg(w, h, body, title):
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {w} {h}" width="{w}" height="{h}" role="img" '
            f'aria-label="{title}"><title>{title}</title>{body}</svg>\n')

def lockup(ink, back, mid, front, wordmark=None):
    size, base, x0 = 44, 47.5, 64 + 16
    a, mid_x = text_path("ibm-plex-sans-latin-400-normal.woff", "Backup", size, x0, base, ink)
    b, end = text_path("ibm-plex-sans-latin-600-normal.woff", "Vouch", size, mid_x, base, ink)
    return mark_body(back, mid, front) + a + b, end

def write(name, content):
    with open(os.path.join(HERE, name), "w") as fh:
        fh.write(content)

def main():
    T = "BackupVouch"
    write("logo-mark.svg", svg(64, 64, mark_body(MIST, SLATE, TEAL), T))
    body, end = lockup(INK, MIST, SLATE, TEAL); W = round(end)
    write("logo.svg", svg(W, 64, body, T))
    body, end = lockup(D_INK, D_MIST, D_SLATE, D_TEAL)
    write("logo-dark.svg", svg(W, 64, body, T))
    write("logo-mark-dark.svg", svg(64, 64, mark_body(D_MIST, D_SLATE, D_TEAL), T))
    body, end = lockup(INK, INK, INK, INK)   # mono: layers differ by nothing, so use gaps only
    write("logo-mono.svg", svg(W, 64, body, T))
    fav = layer(FAV_CHECK, FAV_DY, FAV_SLATE, FAV_SW) + layer(FAV_CHECK, 0, FAV_TEAL, FAV_SW)
    write("favicon.svg", svg(64, 64, fav, T))
    # social preview: dark card, lockup scaled, tagline in outlines
    scale = 1.9
    lw = W * scale
    tag, tw = text_path("ibm-plex-sans-latin-400-normal.woff", "Proves your database backups restore.", 40, 0, 0, D_INK)
    card = (f'<rect width="1280" height="640" fill="{INK}"/>'
            f'<g transform="translate({(1280-lw)/2:.1f} 190) scale({scale})">{lockup(D_INK, D_MIST, D_SLATE, D_TEAL)[0]}</g>'
            f'<g transform="translate({(1280-tw)/2:.1f} 415)">{tag}</g>')
    write("social-preview.svg", svg(1280, 640, card, T + " social preview"))
    render()

def render():
    import cairosvg
    from PIL import Image
    def png(src, out, size, bg=None):
        cairosvg.svg2png(url=os.path.join(HERE, src), write_to=os.path.join(HERE, out),
                         output_width=size[0], output_height=size[1], background_color=bg)
    png("favicon.svg", "favicon-32.png", (32, 32))
    # apple touch icon: opaque, padded
    import tempfile
    big = io.BytesIO()
    cairosvg.svg2png(url=os.path.join(HERE, "favicon.svg"), write_to=big, output_width=124, output_height=124)
    icon = Image.new("RGB", (180, 180), PAPER); fg = Image.open(io.BytesIO(big.getvalue())).convert("RGBA")
    icon.paste(fg, (28, 28), fg); icon.save(os.path.join(HERE, "apple-touch-icon.png"))
    png("social-preview.svg", "social-preview.png", (1280, 640))
    imgs = []
    for s in (16, 32, 48):
        b = io.BytesIO(); cairosvg.svg2png(url=os.path.join(HERE, "favicon.svg"), write_to=b, output_width=s, output_height=s)
        imgs.append(Image.open(io.BytesIO(b.getvalue())).convert("RGBA"))
    imgs[2].save(os.path.join(HERE, "favicon.ico"), sizes=[(16, 16), (32, 32), (48, 48)], append_images=imgs[:2])

if __name__ == "__main__":
    main()
