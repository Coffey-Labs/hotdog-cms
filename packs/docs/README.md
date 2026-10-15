# Documentation pack

A documentation section for a HotDog CMS site, at /docs/ or on a host of its
own: a sidebar of every part, with the part being read open and the others
folded, a breadcrumb, an "On this page" list of the page's headings beside
it on wide screens, a page for each part with its pages in order, a front
page with a card for each part, and previous and next links at the foot of
every page. Parts can hold parts, as deep as the docs need.

    hotdog-cms pack add docs

The reading order lives in one file, `data/docs-nav.yaml`, so moving a page
is one line (`hotdog-cms pack add docs` prints its format). With `path: /`
on the docs collection in `site.yaml`, the pages sit at the root of their
host, as on a docs subdomain. The docs collection gets a search index
(`/docs/search.json`, with each page's headings so a result can go to its
section); a pack can't carry scripts, so adding a search box is up to the
site. hotdogcms.org/docs is built with this pack.

Code blocks that name their language (` ```yaml `) are colored when the
site is built, and the pack's stylesheet has the colors: each one a hue
mixed into the code's own text color, so they read on light code blocks and
dark ones. A theme that wants other hues sets `--code-keyword`,
`--code-string`, `--code-number`, `--code-function`, `--code-type`,
`--code-tag` and `--code-mark` (behind lines marked with `hl_lines`).
