#!/usr/bin/env python3
"""HotDog CMS mark concepts, round 1. Family-kitten geometry (128 grid), dog derived from it."""
import os, subprocess

OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "out")

INK = "#1A1310"
DOG, DOG_EAR, MUZZLE, TONGUE = "#E8B57A", "#9A5A2E", "#FBE9CF", "#EF7486"
CAT, CAT_EAR, WHISK = "#F9A34C", "#F47F35", "#E6690E"
MUSTARD, KETCHUP = "#F4BE2C", "#E4483A"
PAGE, PAGE_LINE = "#FFF8EC", "#D9CBB4"
BP_NAVY, BP_GRID, BP_LINE = "#0E2340", "#4EA8FF", "#DCEBFF"
GROUNDS = {"dark": "#1C1512", "mustard": MUSTARD}

# --- kitten (exact family geometry) ---
K_HEAD = ("M31 64 C30 50 31 38 34 27 Q36 20 42 23 L55 32 Q64 29.5 73 32 L86 23 Q92 20 94 27 "
          "C97 38 98 50 97 64 C96 83 82 92 64 92 C46 92 32 83 31 64 Z")
K_EARS = "M38 30 L50 37.5 L40 45.5 Z M90 30 L78 37.5 L88 45.5 Z"
K_EYES = "M44 60 Q49.5 53 55 60 M73 60 Q78.5 53 84 60"
K_NOSE = "M60.8 65.5 h6.4 l-3.2 3.6 z"
K_MOUTH = "M56 71.5 Q60 76.5 64 71.5 Q68 76.5 72 71.5"
K_WHISK = "M14 58 L30 60 M15 67 L30 65 M114 58 L98 60 M113 67 L98 65"

# --- dog: same skull, closed eyes and stroke weights; drop ears, muzzle, tongue ---
D_HEAD = "M31 62 C30 42 44 28 64 28 C84 28 98 42 97 62 C96 82 82 92 64 92 C46 92 32 82 31 62 Z"
D_EAR_L = "M44 31 C31 28 19 38 17 55 C16 68 21 77 29 76 C35 75 37 67 38 59 C39 49 42 41 49 35 Z"
D_EAR_R = "M84 31 C97 28 109 38 111 55 C112 68 107 77 99 76 C93 75 91 67 90 59 C89 49 86 41 79 35 Z"
D_MUZZLE = "M45 75 C45 65 53 61 64 61 C75 61 83 65 83 75 C83 85 75 90 64 90 C53 90 45 85 45 75 Z"
D_EYES = "M43 55 Q48.5 48 54 55 M74 55 Q79.5 48 85 55"
D_NOSE = "M57 65.5 Q64 61.5 71 65.5 Q70.5 72 64 73.5 Q57.5 72 57 65.5 Z"
D_MOUTH = "M64 73.5 V77 M56 76.5 Q60 81.5 64 77 Q68 81.5 72 76.5"
D_TONGUE = "M60.6 79.4 Q60.6 87 64 87 Q67.4 87 67.4 79.4 Q64 81.4 60.6 79.4 Z"

PAW_L = "M36.5 90 C36 81 40 76.5 46 76.5 C52 76.5 56 81 55.5 90 C55.5 94 51 96 46 96 C41 96 36.5 94 36.5 90 Z"
PAW_R = "M91.5 90 C92 81 88 76.5 82 76.5 C76 76.5 72 81 72.5 90 C72.5 94 77 96 82 96 C87 96 91.5 94 91.5 90 Z"
TOES = "M43 88.5 V93 M49 88.5 V93 M79 88.5 V93 M85 88.5 V93"

G = f'stroke="{INK}" stroke-linecap="round" stroke-linejoin="round"'


def dog(t, tongue=True, line=None, sw=1.0):
    """line=color draws it as a blueprint line drawing instead of filled."""
    if line:
        f, e, m, s = "rgba(78,168,255,.10)", BP_NAVY, "none", f'stroke="{line}" stroke-linecap="round" stroke-linejoin="round"'
        nose = line
    else:
        f, e, m, s, nose = DOG, DOG_EAR, MUZZLE, G, INK
    tg = (f'<path d="{D_TONGUE}" fill="{TONGUE if not line else "none"}" stroke-width="2.4"/>' if tongue else "")
    return (f'<g transform="{t}" {s}>'
            f'<path d="{D_HEAD}" fill="{f}" stroke-width="{5*sw:.2f}"/>'
            f'<path d="{D_MUZZLE}" fill="{m}" stroke-width="{0 if not line else 2.4}"/>'
            f'<path d="{D_EYES}" stroke-width="{4.2*sw:.2f}" fill="none"/>'
            f'{tg}'
            f'<path d="{D_MOUTH}" stroke-width="{3.2*sw:.2f}" fill="none"/>'
            f'<path d="{D_NOSE}" fill="{nose}" stroke-width="2"/>'
            f'<path d="{D_EAR_L}" fill="{e}" stroke-width="{5*sw:.2f}"/>'
            f'<path d="{D_EAR_R}" fill="{e}" stroke-width="{5*sw:.2f}"/>'
            f'</g>')


def cat(t, whiskers=True, line=None):
    if line:
        f, e, s, nose = "rgba(78,168,255,.10)", "none", f'stroke="{line}" stroke-linecap="round" stroke-linejoin="round"', line
    else:
        f, e, s, nose = CAT, CAT_EAR, G, INK
    w = f'<path d="{K_WHISK}" stroke="{WHISK if not line else line}" stroke-width="3" fill="none"/>' if whiskers else ""
    return (f'<g transform="{t}" {s}>{w}<path d="{K_HEAD}" fill="{f}" stroke-width="5"/>'
            f'<path d="{K_EARS}" fill="{e}" stroke="{"none" if not line else line}" stroke-width="2"/>'
            f'<path d="{K_EYES}" stroke-width="4.2" fill="none"/>'
            f'<path d="{K_NOSE}" fill="{nose}" stroke-width="2"/>'
            f'<path d="{K_MOUTH}" stroke-width="3.5" fill="none"/></g>')


def paws(t="", fur=DOG, toes=True, sw=1.0):
    tt = f' transform="{t}"' if t else ""
    return (f'<g{tt} {G}><path d="{PAW_L}" fill="{fur}" stroke-width="{4*sw:.2f}"/><path d="{PAW_R}" fill="{fur}" stroke-width="{4*sw:.2f}"/>'
            + (f'<path d="{TOES}" stroke-width="2.4" fill="none"/>' if toes else "") + '</g>')


def browser(x, y, w, h, bar=10, body="", dots=True, sw=1.0):
    """Window seen from the front; three dots in the bar are the three skill levels."""
    d = "".join(f'<circle cx="{x + 8 + i * 6.5}" cy="{y + bar / 2 + .5}" r="2" fill="{c}"/>'
                for i, c in enumerate((KETCHUP, MUSTARD, "#7FBF6A")))
    return (f'<g><rect x="{x}" y="{y}" width="{w}" height="{h}" rx="6" fill="{PAGE}" stroke="{INK}" stroke-width="4.5"/>'
            f'{body}'
            f'<path d="M{x} {y + bar + 1} H{x + w}" stroke="{INK}" stroke-width="{3*sw:.2f}"/>{d if dots else ""}'
            f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="6" fill="none" stroke="{INK}" stroke-width="{4.5*sw:.2f}"/></g>')


def tile(inner, ground, p):
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 128 128">'
            f'<defs><clipPath id="{p}t"><rect width="128" height="128" rx="28"/></clipPath></defs>'
            f'<g clip-path="url(#{p}t)"><rect width="128" height="128" fill="{GROUNDS[ground]}"/>{inner}</g></svg>')


