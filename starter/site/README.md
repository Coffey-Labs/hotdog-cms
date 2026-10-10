# Your HotDog CMS site

    hotdog-cms serve     preview at http://127.0.0.1:8140, rebuilding as you save
    hotdog-cms build     write the finished site to public/
    hotdog-cms check     build, then look for broken links and forbidden strings

| Folder | What goes in it |
|---|---|
| `content/` | Pages, as Markdown. `about.md` becomes `/about/`; a folder such as `articles/` is a collection |
| `layouts/` | Page shapes. `base.html` wraps every page; the others fill its `main` block |
| `partials/` | Pieces layouts share: head, nav, footer |
| `sections/` | Blocks a page can stack in its front matter (`sections:`) |
| `data/` | YAML or JSON that templates and sections read |
| `assets/` | CSS, scripts, fonts. Published with a content hash in the name |
| `static/` | Files copied as they are: favicon, `.well-known/`, images |

`public/` is the output. Don't edit it; it's replaced on every build.
