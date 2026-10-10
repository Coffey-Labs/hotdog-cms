HotDog CMS logo bundle
======================

The mark (picked 2026-10-10): a kitten-style dog drawn from the Coffey Labs
family kitten (same skull, closed eyes, stroke weights and peek pose; drop
ears, muzzle and tongue in place of pointed ears and whiskers).

Full mark, concept C "Live preview": the dog peeks over a browser window whose
kitten page is half markup (blueprint) and half rendered, with the split
handle between them. Small cut, concept A "Peek": the dog over a plain window,
for 40 px and below. Both sit on the mustard ground. The head sits low so the
chin tucks in behind the paws.

Wordmark, concept X2 "Code and casual" (picked 2026-10-10): "Hot" in plain
monospace drawn as a hollow outline (the markup), "Dog" in heavy casual sans
(the rendered page), a mustard split line between them, and a "cms" pill.
All three are Recursive (SIL OFL), converted to outlines:
  Hot  MONO 1, CASL 0, wght 700     Dog  MONO 0, CASL 1, wght 900
  cms  MONO 1, CASL 1, wght 700 at 0.38 em

Rebuild everything:  python3 source/gen.py   (needs rsvg-convert)
source/base.py holds the shared drawing (dog, kitten, paws, window).
source/word.json holds the wordmark outlines and the social card's text
(tagline: "Write in Markdown. See it live. Publish from git."; change
TAGLINE in word.py). Only to change the type or the text:
  python3 source/word.py <Recursive variable .ttf>
  (needs fonttools, skia-pathops, uharfbuzz; font from github.com/google/fonts,
  ofl/recursive)


Files
-----
svg/icon-tile.svg            app icon: full mark on the mustard rounded tile
svg/icon-tile-square.svg     same, square corners (apple-touch; iOS rounds it)
svg/icon-flat.svg            full mark on transparent
svg/icon-small.svg           SMALL CUT on transparent, for inline marks at 40 px and below
svg/favicon.svg              SMALL CUT on the tile, for 40 px and below
svg/avatar.svg               small cut, full-bleed square for round crops
                             (Gitea, GitHub, Discord, forum); the window runs
                             off the bottom so a circle crop curves it

png/favicon-16/32/48.png     from favicon.svg (small cut)
png/favicon-180.png          apple-touch, from icon-tile-square.svg
png/favicon-192/512.png      from icon-tile.svg
png/icon-tile-512/1024/2048.png
png/icon-flat-1024.png
svg/wordmark-{light,dark}.svg          name only, transparent
svg/wordmark-mono-{black,white}.svg    one color; "cms" knocked out of the pill
svg/logo-horizontal-{light,dark}.svg   full mark + name, hero and README use
svg/logo-header-{light,dark}.svg       SMALL CUT + name, for site headers
                                       where the lockup is 40 px tall or less
svg/logo-stacked-{light,dark}.svg      mark over name
svg/social-preview.svg                 1280x640 card, concept S2 "Big preview": the dog
                                       over a window split into Markdown source
                                       and the rendered page; all text in outlines

png/avatar-512.png
png/wordmark-{light,dark}.png          1280 wide
png/logo-horizontal-{light,dark}.png   1280 wide
png/logo-header-{light,dark}.png       640 wide
png/logo-stacked-{light,dark}.png      720 wide
png/social-preview.png                 1280x640


Colors
------
#F4BE2C  mustard: ground, split line and handle
#E4483A  ketchup: page heading bar, first window dot
#1A1310  outline ink
#E8B57A  dog fur      #9A5A2E  ears      #FBE9CF  muzzle      #EF7486  tongue
#F9A34C  kitten fur (family)   #F47F35  kitten inner ear   #E6690E  whiskers
#FFF8EC  page        #D9CBB4  page text lines
Wordmark: light  #1A1310 name, #E0A81C split, #D93A2B pill, #FFFDF8 pill text
          dark   #FFF8EC name, #F4BE2C split and pill, #1A1310 pill text
          (ketchup on light grounds: mustard on cream is too faint to read)
#0E2340  blueprint ground   #4EA8FF  blueprint grid   #DCEBFF  blueprint line
         (the Coffey Labs blueprint colors)


Rules
-----
- Use the small cut (favicon.svg / avatar.svg / icon-small.svg) at 40 px and
  below. The full mark's page, kitten and split turn to mush that small.
- The name is written "HotDog CMS" in prose. The lockups set "cms" lowercase
  in the pill.
- There is no one-color version of the mark itself (the half-rendered page
  needs its colors). Use the one-color wordmark alone.
- On the mustard ground (social card) the wordmark uses an ink name and pill
  with mustard "cms" and a ketchup split line.
- The three window dots (red, mustard, green) stand for the beginner,
  intermediate and advanced views. The small cut drops them.
- Never borrow from the original HotDog (Sausage Software) box art or icon,
  and never use "Pro" in the name.
- No mascot name in any art, title or alt text.
