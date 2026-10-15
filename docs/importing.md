# Importing a site

> **Only import content you own or have the rights to use.** The importers
> are migration tools: they're for moving *your own* site to HotDog CMS from
> the tool it's on now. They're not for copying sites, posts or pictures that
> belong to someone else, and they say so every time they run.

`hotdog-cms import` brings a site in from WordPress, Hugo, Jekyll, Ghost or
MkDocs, into a HotDog CMS site made with `hotdog-cms new`:

    hotdog-cms new mysite && cd mysite
    hotdog-cms import wordpress -url https://old.example.com
    hotdog-cms check

| From | Command |
|---|---|
| A live WordPress site | `import wordpress -url https://example.com` (published posts and pages). With `-user name` and an Application Password in `HOTDOG_WP_PASSWORD`, drafts and private posts too. |
| A WordPress export file | `import wxr -file export.xml` (Tools → Export) |
| Ghost | `import ghost -file export.json -url https://old.example.com` (Settings → Labs → Export) |
| Hugo | `import hugo -from ../oldsite` (`-rename posts=articles` by default) |
| Jekyll | `import jekyll -from ../oldsite` |
| MkDocs | `import mkdocs -from ../docs-site` (the folder holding `mkdocs.yml`); see [Documentation from MkDocs](#documentation-from-mkdocs) |

Every importer takes `-site DIR` (default `.`) and `-overwrite`; the blog
importers take `-posts COLLECTION` (default `articles`) too.

## What happens

- **Markdown where it's faithful.** Posts and pages become Markdown files
  with front matter: title, date, updated, summary, tags (and categories),
  author, draft, featured image and its alt text.
- **HTML where Markdown would lose something:** embeds, layout blocks (page
  builders, WordPress columns), tables with merged cells, inline styles. Those
  pages keep their HTML (`format: html`), and `markdown.unsafe_html` is turned
  on so they build. The HTML is sanitized at build time, so scripts and
  embeds are dropped. The report lists each page and why.
- **Pictures come across,** through the same re-encoding as uploads: no
  location or camera data, resized, WebP copies. A picture that can't be
  fetched keeps its old address, and the report says so.
- **Old addresses keep working.** Every page's old permalink (WordPress links,
  Hugo `url`/`aliases`, Jekyll permalinks and `redirect_from`, Ghost's
  `/slug/`) becomes a redirect to its new address, in `site.yaml`.
- **Page hierarchies stay:** a WordPress page with pages under it becomes its
  folder's own page, listing its children.
- **What only the old tool could run** (WordPress shortcodes like
  `[gallery]`, Hugo shortcodes, Jekyll's Liquid) is left as text and listed in
  the report. WordPress's `[caption]` becomes a figure.
- **Nothing is overwritten** unless you say `-overwrite`. Running an import
  again skips what's there.

The report goes to `IMPORT-REPORT.md` in the site. Read it, then run
`hotdog-cms check` and `hotdog-cms serve`.

## Documentation from MkDocs

    hotdog-cms new docs && cd docs && rm content/index.md
    hotdog-cms import mkdocs -from ../old-docs
    hotdog-cms check

A documentation site keeps its addresses, its reading order and the way its
pages are written:

- **Pages come across as written.** HotDog reads MkDocs' own syntax:
  admonitions (`!!! note`, `??? tip`), content tabs (`=== "Linux"`), keys
  (`++ctrl+k++`), definition lists, `{ .class }` attributes, Markdown in HTML
  (`markdown` on a tag), code blocks' file names (`title="…"`). A page's
  first heading becomes its title, and `index.md` (or `README.md`) its
  folder's own page.
- **Addresses stay.** The docs go into the docs pack's collection (the pack
  is added if the site hasn't got it) with `path: /`, so `run/sizing.md` is
  still `/run/sizing/`. Use `-path /docs/` to put them under `/docs/` on a
  site that has its own front page; the front page is why `content/index.md`
  has to go with `-path /`.
- **Links keep landing.** Links to other pages' `.md` files are followed to
  their addresses at build time, and headings get the anchors MkDocs gives
  them (`markdown.heading_ids: mkdocs`), so `sizing.md#memory` still finds its
  section. Links to pictures and downloads in the docs are made to work from
  the page's address. Everything else in `docs_dir` goes into `static/`.
- **The nav becomes `data/docs-nav.yaml`**, the docs pack's reading order:
  sections, a section's index page as its own, links elsewhere. Without a
  nav, the files' order is used, as MkDocs does.
- **Settings:** the site's name, address and description, if `site.yaml`
  still has the new site's; pymdownx.keys' `key_map` (as `markdown.keys`).

What doesn't come across is listed in `IMPORT-REPORT.md`, page by page:
snippet includes (`--8<--`), template tags (the macros plugin),
abbreviations, icons and emoji (`:material-check:`), Mermaid diagrams (shown
as code), code annotations, and the theme's overrides and extra CSS and
JavaScript: the site's own look replaces the theme. Code blocks are colored
as pymdownx.highlight colors them, with the same class names, and
`hl_lines` marks lines; line numbers (`linenums`) aren't drawn.

MkDocs passes any HTML in a page through as it is; HotDog sanitizes it, so
an inline `<svg>`, a `<script>` or a `style=""` is removed, and the report
lists each. If everyone who writes the docs can be trusted with scripts,
`-trust-html` turns the sanitizer off for the site (`markdown.trust_html`).

## The WordPress password

Use an **Application Password** (Users → Profile → Application Passwords),
not the login password. It's read from `HOTDOG_WP_PASSWORD`, used for the
requests, and never written anywhere. Revoke it in WordPress when the import
is done.
