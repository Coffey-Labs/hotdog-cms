#!/usr/bin/env python3
"""HotDog CMS mark. Full mark = concept C "Live preview": the dog peeks over a window whose
kitten page is half markup (blueprint), half rendered. Small cut = concept A "Peek": the dog
over a plain window, for 40 px and below. Mustard ground.

Writes every SVG into ../svg and renders the PNGs into ../png with rsvg-convert.
Run from anywhere:  python3 source/gen.py
"""
import json
import os
import subprocess
import sys

SRC = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, SRC)
from base import (INK, MUSTARD, KETCHUP, PAGE, PAGE_LINE, BP_NAVY, BP_GRID, BP_LINE,  # noqa: E402
                  dog, cat, paws, browser)

ROOT = os.path.dirname(SRC)
SVG = os.path.join(ROOT, "svg")
PNG = os.path.join(ROOT, "png")
GROUND = MUSTARD


def full(p):
    """Concept C in the 128 grid. Head sits low so the chin tucks in behind the paws."""
    x, y, w, h = 10, 60, 108, 64
    defs = (f'<clipPath id="{p}L"><rect x="{x}" y="{y}" width="{64 - x}" height="{h}"/></clipPath>'
            f'<clipPath id="{p}R"><rect x="64" y="{y}" width="{x + w - 64}" height="{h}"/></clipPath>'
            f'<pattern id="{p}g" width="6" height="6" patternUnits="userSpaceOnUse"><path d="M6 0 H0 V6" fill="none" stroke="{BP_GRID}" stroke-opacity=".28" stroke-width=".6"/></pattern>')
    page_cat = "translate(64 96) scale(.38) translate(-64 -60)"
    bp = (f'<g clip-path="url(#{p}L)"><rect x="{x}" y="{y}" width="{w}" height="{h}" fill="{BP_NAVY}"/>'
          f'<rect x="{x}" y="{y}" width="{w}" height="{h}" fill="url(#{p}g)"/>'
          f'{cat(page_cat, line=BP_LINE)}'
          f'<rect x="20" y="76" width="34" height="5" rx="1" fill="none" stroke="{BP_LINE}" stroke-width="1.2" stroke-dasharray="2 1.6"/>'
          f'<path d="M20 113 H57 M20 119 H50" stroke="{BP_LINE}" stroke-width="1.4" stroke-dasharray="2.5 2" stroke-linecap="round"/></g>')
    rd = (f'<g clip-path="url(#{p}R)">{cat(page_cat)}'
          f'<rect x="70" y="76" width="38" height="5" rx="2.5" fill="{KETCHUP}"/>'
          f'<path d="M71 113 H108 M71 119 H98" stroke="{PAGE_LINE}" stroke-width="3" stroke-linecap="round"/></g>')
    split = (f'<path d="M64 {y + 11} V{y + h}" stroke="{MUSTARD}" stroke-width="2.6"/>'
             f'<circle cx="64" cy="{y + 55}" r="4.6" fill="{MUSTARD}" stroke="{INK}" stroke-width="2"/>')
    return (f'<defs>{defs}</defs>'
            + dog("translate(64 40) scale(.64) translate(-64 -60)")
            + browser(x, y, w, h, body=bp + rd + split)
            + paws("translate(64 61) scale(.64) translate(-64 -88)"))


def small(p):
    """Concept A for 40 px and below: bigger dog, heavier lines, no toes, dots or text lines."""
    page = f'<rect x="22" y="106" width="44" height="7" rx="3.5" fill="{KETCHUP}"/>'
    return (dog("translate(64 58) scale(1) translate(-64 -60)", sw=1.2)
            + browser(10, 90, 108, 60, bar=9, body=page, dots=False, sw=1.2)
            + paws("translate(64 91) scale(1.04) translate(-64 -88)", toes=False, sw=1.2))


def svg(vb, inner, w=None, h=None, title=True):
    size = f' width="{w}" height="{h}"' if w else ""
    t = "<title>HotDog CMS</title>" if title else ""
    return f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="{vb}"{size} role="img" aria-label="HotDog CMS">{t}{inner}</svg>\n'


def tile_inner(p, art, rx=28):
    # inset the art so the window's lower corners clear the tile's rounding
    return (f'<defs><clipPath id="{p}clip"><rect width="128" height="128" rx="{rx}"/></clipPath></defs>'
            f'<g clip-path="url(#{p}clip)"><rect width="128" height="128" fill="{GROUND}"/>'
            f'<g transform="translate(64 64) scale(.9) translate(-64 -60)">{art(p)}</g></g>')


def tile(p, art, square=False, size=512):
    return svg("0 0 128 128", tile_inner(p, art, 0 if square else 28), size, size)


# ---------------------------------------------------------------- wordmark (concept X2)
# Outlines come from word.py (Recursive): 100-unit em, baseline y=0, y down.
WORD = json.load(open(os.path.join(SRC, "word.json")))
THEMES = {  # fg, split, pill, pill text
    "light": ("#1A1310", "#E0A81C", "#D93A2B", "#FFFDF8"),
    "dark": ("#FFF8EC", MUSTARD, MUSTARD, "#1A1310"),
    "mustard": ("#1A1310", KETCHUP, "#1A1310", MUSTARD),   # on the mustard ground: ketchup split, ink pill
}
HOLLOW = 8              # stroke drawn inside "Hot" (clipped to the letters): 4 units of line
MID = -27               # vertical centre the split and the pill sit on
SPLIT_X, SPLIT_W, SPLIT_H = WORD["hot"]["advance"] + 6, 5.5, 102
DOG_X = SPLIT_X + SPLIT_W + 6
CMS_S = .38
PILL_X = DOG_X + WORD["dog"]["advance"] + 10
PILL_PX, PILL_H = .45 * 100 * CMS_S, 48
PILL_W = WORD["cms"]["advance"] * CMS_S + 2 * PILL_PX
WORD_W = PILL_X + PILL_W
WORD_TOP, WORD_BOT = MID - SPLIT_H / 2, WORD["dog"]["bounds"][3]


def word(p, theme=None, mono=None):
    """The wordmark at the 100-unit em, origin at the left baseline. mono=colour draws it in one colour
    with "cms" knocked out of the pill."""
    fg, split, pill, ptxt = (mono, mono, mono, None) if mono else THEMES[theme]
    cms_x = PILL_X + PILL_PX
    cms_base = MID + WORD["cms"]["xh"] * CMS_S / 2
    cms = f'<path transform="translate({cms_x:.2f} {cms_base:.2f}) scale({CMS_S})" d="{WORD["cms"]["d"]}"'
    pill_rect = f'x="{PILL_X:.2f}" y="{MID - PILL_H / 2:.2f}" width="{PILL_W:.2f}" height="{PILL_H}" rx="13.3"'
    if mono:
        knock = (f'<mask id="{p}k" maskUnits="userSpaceOnUse" x="{PILL_X - 1:.2f}" y="{MID - PILL_H / 2 - 1:.2f}" width="{PILL_W + 2:.2f}" height="{PILL_H + 2}">'
                 f'<rect {pill_rect} fill="#fff"/>{cms} fill="#000"/></mask>')
        pill_svg = f'<defs>{knock}</defs><rect {pill_rect} fill="{pill}" mask="url(#{p}k)"/>'
    else:
        pill_svg = f'<rect {pill_rect} fill="{pill}"/>{cms} fill="{ptxt}"/>'
    hot = WORD["hot"]["d"]
    return (f'<defs><clipPath id="{p}h"><path d="{hot}"/></clipPath></defs>'
            f'<path d="{hot}" fill="none" stroke="{fg}" stroke-width="{HOLLOW}" clip-path="url(#{p}h)"/>'
            f'<rect x="{SPLIT_X:.2f}" y="{MID - SPLIT_H / 2:.2f}" width="{SPLIT_W}" height="{SPLIT_H}" rx="2.5" fill="{split}"/>'
            f'<path transform="translate({DOG_X:.2f} 0)" d="{WORD["dog"]["d"]}" fill="{fg}"/>'
            + pill_svg)


def wordmark(p, theme=None, mono=None):
    pad = 4
    vb = f"{-pad} {WORD_TOP - pad:.2f} {WORD_W + 2 * pad:.2f} {WORD_BOT - WORD_TOP + 2 * pad:.2f}"
    w = 1000
    return svg(vb, word(p, theme, mono), w, round(w * (WORD_BOT - WORD_TOP + 2 * pad) / (WORD_W + 2 * pad)))


MARK_H = 150            # the tile beside the name is 1.5 em, as in the concept sheet


def horizontal(p, theme, art=None, gap=32):
    art = art or full
    top = MID - MARK_H / 2
    inner = (f'<svg x="0" y="{top:.2f}" width="{MARK_H}" height="{MARK_H}" viewBox="0 0 128 128">{tile_inner(p + "m", art)}</svg>'
             f'<g transform="translate({MARK_H + gap} 0)">{word(p, theme)}</g>')
    w = MARK_H + gap + WORD_W
    pad = 6
    vb = f"{-pad} {top - pad:.2f} {w + 2 * pad:.2f} {MARK_H + 2 * pad}"
    return svg(vb, inner, 1280, round(1280 * (MARK_H + 2 * pad) / (w + 2 * pad)))


def stacked(p, theme):
    m = 220
    w = max(WORD_W, m)
    x_m = (w - m) / 2
    y_w = m + 40 - WORD_TOP          # name baseline: mark, a gap, then the top of the name
    inner = (f'<svg x="{x_m:.2f}" y="0" width="{m}" height="{m}" viewBox="0 0 128 128">{tile_inner(p + "m", full)}</svg>'
             f'<g transform="translate({(w - WORD_W) / 2:.2f} {y_w:.2f})">{word(p, theme)}</g>')
    h = y_w + WORD_BOT
    pad = 10
    return svg(f"{-pad} {-pad} {w + 2 * pad:.2f} {h + 2 * pad:.2f}", inner, 720, round(720 * (h + 2 * pad) / (w + 2 * pad)))


def txt(key, x, y, size, fill):
    """A run from word.json at a font size in px, (x, y) = left end of the baseline."""
    return f'<path transform="translate({x:.2f} {y:.2f}) scale({size / 100:.4f})" d="{WORD[key]["d"]}" fill="{fill}"/>'


def social(p):
    """1280x640 card, concept S2 "Big preview": a large version of mark C. On mustard, the dog peeks over a
    window whose left pane is the Markdown source and whose right pane is the rendered page; the name and
    the tagline sit top left."""
    W, H = 1280, 640
    x0, y0, x1, mid, bar = 80, 336, 1200, 640, 40
    code = [("md_fence", 0), ("md_title", 1), ("md_fence", 2), ("md_h1", 4), ("md_p1", 6), ("md_p2", 7)]
    src = "".join(txt(k, x0 + 34, y0 + 76 + i * 30, 21, BP_LINE) for k, i in code)
    body = "#4A3D33"
    px, size = mid + 40, 24
    ren = (txt("page_h1", px, y0 + 112, 46, INK)
           + txt("page_p1a", px, y0 + 166, size, body)
           + txt("page_p1b", px + WORD["page_p1a"]["advance"] * size / 100, y0 + 166, size, INK)
           + txt("page_p1c", px + (WORD["page_p1a"]["advance"] + WORD["page_p1b"]["advance"]) * size / 100, y0 + 166, size, body)
           + txt("page_p2", px, y0 + 200, size, body)
           + f'<rect x="{px}" y="{y0 + 236}" width="300" height="14" rx="7" fill="{PAGE_LINE}"/>')
    dots = "".join(f'<circle cx="{x0 + 30 + i * 24}" cy="{y0 + bar / 2}" r="7" fill="{c}"/>'
                   for i, c in enumerate((KETCHUP, MUSTARD, "#7FBF6A")))
    win = (f'<defs><pattern id="{p}g" width="20" height="20" patternUnits="userSpaceOnUse"><path d="M20 0 H0 V20" fill="none" stroke="{BP_GRID}" stroke-opacity=".25"/></pattern>'
           f'<clipPath id="{p}w"><rect x="{x0}" y="{y0}" width="{x1 - x0}" height="400" rx="22"/></clipPath></defs>'
           f'<g clip-path="url(#{p}w)"><rect x="{x0}" y="{y0}" width="{x1 - x0}" height="400" fill="{PAGE}"/>'
           f'<rect x="{x0}" y="{y0}" width="{mid - x0}" height="400" fill="{BP_NAVY}"/><rect x="{x0}" y="{y0}" width="{mid - x0}" height="400" fill="url(#{p}g)"/>'
           f'{src}{ren}'
           f'<rect x="{mid - 4}" y="{y0 + bar}" width="8" height="400" fill="{MUSTARD}"/>'
           f'<rect x="{x0}" y="{y0}" width="{x1 - x0}" height="{bar}" fill="{PAGE}"/></g>'
           f'<path d="M{x0} {y0 + bar} H{x1}" stroke="{INK}" stroke-width="9"/>{dots}'
           f'<circle cx="{mid}" cy="{y0 + 268}" r="15" fill="{MUSTARD}" stroke="{INK}" stroke-width="6"/>'
           f'<rect x="{x0}" y="{y0}" width="{x1 - x0}" height="400" rx="22" fill="none" stroke="{INK}" stroke-width="12"/>')
    # the dog with its paws on the window's top edge: small-cut geometry, toes back on at this size
    pup = (f'<g transform="translate(1066 {y0}) scale(2.5) translate(-64 -90)">'
           + dog("translate(64 58) scale(1) translate(-64 -60)")
           + paws("translate(64 91) scale(1.04) translate(-64 -88)") + '</g>')
    name = f'<g transform="translate(80 70) scale(.98) translate(0 {-WORD_TOP:.2f})">{word(p, "mustard")}</g>'
    return svg(f"0 0 {W} {H}",
               f'<rect width="{W}" height="{H}" fill="{MUSTARD}"/>{win}{pup}{name}'
               + txt("tagline", 84, 244, 26, INK), W, H, title=False)


def flat(p):
    """Full mark on transparent; the window closes at the bottom, so nothing is cut."""
    return svg("4 14 120 114", full(p), 400, 380)


def small_flat(p):
    return svg("8 22 112 112", f'<defs><clipPath id="{p}c"><rect x="0" y="0" width="128" height="124"/></clipPath></defs>'
               f'<g clip-path="url(#{p}c)">{small(p)}</g>', 64, 64)


def avatar(p):
    """Full-bleed square for round crops: the window runs off the bottom, so a circle crop curves it."""
    return svg("0 0 128 128",
               f'<rect width="128" height="128" fill="{GROUND}"/>'
               f'<g transform="translate(64 70) scale(.84) translate(-64 -70)">{small(p)}</g>', 512, 512)


def write(name, text):
    with open(os.path.join(SVG, name), "w") as f:
        f.write(text)


def render(src, out, w, h=None):
    cmd = ["rsvg-convert", "-w", str(w)] + (["-h", str(h)] if h else [])
    subprocess.run(cmd + [os.path.join(SVG, src), "-o", os.path.join(PNG, out)], check=True)


if __name__ == "__main__":
    os.makedirs(SVG, exist_ok=True)
    os.makedirs(PNG, exist_ok=True)
    write("icon-tile.svg", tile("ct", full))
    write("icon-tile-square.svg", tile("cq", full, square=True))
    write("icon-flat.svg", flat("cf"))
    write("icon-small.svg", small_flat("cs"))
    write("favicon.svg", tile("fv", small, size=64))
    write("avatar.svg", avatar("av"))

    for s in (16, 32, 48):
        render("favicon.svg", f"favicon-{s}.png", s)
    render("icon-tile-square.svg", "favicon-180.png", 180)    # apple-touch: iOS rounds it
    render("icon-tile.svg", "favicon-192.png", 192)
    render("icon-tile.svg", "favicon-512.png", 512)
    for s in (512, 1024, 2048):
        render("icon-tile.svg", f"icon-tile-{s}.png", s)
    render("icon-flat.svg", "icon-flat-1024.png", 1024)
    render("avatar.svg", "avatar-512.png", 512)

    for th in ("light", "dark"):
        write(f"wordmark-{th}.svg", wordmark(f"w{th[0]}", th))
        write(f"logo-horizontal-{th}.svg", horizontal(f"h{th[0]}", th))
        write(f"logo-header-{th}.svg", horizontal(f"r{th[0]}", th, art=small, gap=36))
        write(f"logo-stacked-{th}.svg", stacked(f"s{th[0]}", th))
        render(f"wordmark-{th}.svg", f"wordmark-{th}.png", 1280)
        render(f"logo-horizontal-{th}.svg", f"logo-horizontal-{th}.png", 1280)
        render(f"logo-header-{th}.svg", f"logo-header-{th}.png", 640)
        render(f"logo-stacked-{th}.svg", f"logo-stacked-{th}.png", 720)
    write("wordmark-mono-black.svg", wordmark("mk", mono="#111315"))
    write("wordmark-mono-white.svg", wordmark("mw", mono="#FFFFFF"))
    write("social-preview.svg", social("so"))
    render("social-preview.svg", "social-preview.png", 1280, 640)
    print("hotdog bundle written:", len(os.listdir(SVG)), "svg,", len(os.listdir(PNG)), "png")
