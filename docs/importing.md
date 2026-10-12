# Importing a site

> **Only import content you own or have the rights to use.** The importers
> are migration tools: they're for moving *your own* site to HotDog CMS from
> the tool it's on now. They're not for copying sites, posts or pictures that
> belong to someone else, and they say so every time they run.

`hotdog-cms import` brings a site in from WordPress, Hugo, Jekyll or Ghost,
into a HotDog CMS site made with `hotdog-cms new`:

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

Every importer takes `-site DIR` (default `.`), `-posts COLLECTION` (default
`articles`) and `-overwrite`.

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

## The WordPress password

Use an **Application Password** (Users → Profile → Application Passwords),
not the login password. It's read from `HOTDOG_WP_PASSWORD`, used for the
requests, and never written anywhere. Revoke it in WordPress when the import
is done.
