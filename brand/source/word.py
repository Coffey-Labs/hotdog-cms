#!/usr/bin/env python3
"""Turn the HotDog CMS wordmark (concept X2) into outlines: source/word.json.

"Hot"  Recursive, plain mono   (MONO 1, CASL 0, wght 700)  the markup half
"Dog"  Recursive, casual sans  (MONO 0, CASL 1, wght 900)  the rendered half
"cms"  Recursive, casual mono  (MONO 1, CASL 1, wght 700)  the pill
plus the social card's tagline and window text (TAGLINE below).

Needs fonttools, skia-pathops and uharfbuzz, plus the Recursive variable font
(OFL, github.com/google/fonts ofl/recursive). Only needed to change the type;
gen.py reads word.json and needs neither.

    python3 source/word.py /path/to/Recursive[CASL,CRSV,MONO,slnt,wght].ttf
"""
import io
import json
import os
import sys

import pathops
import uharfbuzz as hb
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.transformPen import TransformPen
from fontTools.ttLib import TTFont
from fontTools.varLib.instancer import instantiateVariableFont

SRC = os.path.dirname(os.path.abspath(__file__))
EM = 100.0   # outlines are written at a 100-unit em, baseline at y=0, y down

CODE = lambda w: {"MONO": 1, "CASL": 0, "wght": w, "slnt": 0, "CRSV": 0.5}    # the markup voice
PAGE = lambda w: {"MONO": 0, "CASL": 1, "wght": w, "slnt": 0, "CRSV": 0.5}    # the rendered voice

# The social card's tagline. Change it here, run word.py, then gen.py.
TAGLINE = "Write in Markdown. See it live. Publish from git."

RUNS = {
    "hot": ("Hot", CODE(700), -0.03),
    "dog": ("Dog", PAGE(900), -0.02),
    "cms": ("cms", {"MONO": 1, "CASL": 1, "wght": 700, "slnt": 0, "CRSV": 0.5}, 0.0),
    # social card: tagline, the Markdown source pane and the rendered pane
    "tagline": (TAGLINE, CODE(600), 0.0),
    "md_fence": ("---", CODE(500), 0.0),
    "md_title": ("title: Hello", CODE(500), 0.0),
    "md_h1": ("# Hello, world", CODE(500), 0.0),
    "md_p1": ("Write in **Markdown**,", CODE(500), 0.0),
    "md_p2": ("see it rendered live.", CODE(500), 0.0),
    "page_h1": ("Hello, world", PAGE(900), 0.0),
    "page_p1a": ("Write in ", PAGE(400), 0.0),
    "page_p1b": ("Markdown", PAGE(800), 0.0),
    "page_p1c": (",", PAGE(400), 0.0),
    "page_p2": ("see it rendered live.", PAGE(400), 0.0),
}


def run(vf_path, text, loc, tracking):
    font = instantiateVariableFont(TTFont(vf_path), loc)
    buf = io.BytesIO()
    font.save(buf)
    upem = font["head"].unitsPerEm
    face = hb.Face(buf.getvalue())
    hfont = hb.Font(face)
    b = hb.Buffer()
    b.add_str(text)
    b.guess_segment_properties()
    hb.shape(hfont, b, {"kern": True, "liga": False})
    gs = font.getGlyphSet()
    order = font.getGlyphOrder()
    s = EM / upem
    path = pathops.Path()
    x = 0.0
    for i, (info, pos) in enumerate(zip(b.glyph_infos, b.glyph_positions)):
        name = order[info.codepoint]
        p = pathops.Path()
        gs[name].draw(TransformPen(p.getPen(glyphSet=gs), (s, 0, 0, -s, x + pos.x_offset * s, -pos.y_offset * s)))
        path = pathops.op(path, p, pathops.PathOp.UNION)
        x += pos.x_advance * s
        if i < len(text) - 1:
            x += tracking * EM
    pen = SVGPathPen(None, ntos=lambda v: f"{v:.2f}".rstrip("0").rstrip("."))
    path.draw(pen)
    os2 = font["OS/2"]
    xmin, ymin, xmax, ymax = path.bounds
    return {"d": pen.getCommands(), "advance": round(x, 2),
            "bounds": [round(v, 2) for v in (xmin, ymin, xmax, ymax)],
            "cap": round(os2.sCapHeight * s, 2), "xh": round(os2.sxHeight * s, 2)}


if __name__ == "__main__":
    vf = sys.argv[1]
    out = {k: run(vf, *v) for k, v in RUNS.items()}
    with open(os.path.join(SRC, "word.json"), "w") as f:
        json.dump(out, f, indent=1)
    for k, v in out.items():
        print(k, v["advance"], v["bounds"], "cap", v["cap"], "xh", v["xh"])
