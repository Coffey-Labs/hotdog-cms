# cl-cms

Websites from Markdown files and templates, with no database.

    go build -o bin/cl-cms ./cmd/cl-cms

    cl-cms new mysite        start a site from the starter
    cd mysite
    cl-cms serve             preview at http://127.0.0.1:8140, reloading as you save
    cl-cms build             write the finished site to public/
    cl-cms check             build, then report broken links and forbidden strings
    cl-cms publish <target>  build, then publish: a folder, a server over SSH, a container image, or your own command
    cl-cms pull -repo URL -out DIR   on the web server: publish from git when the branch moves

A site is a folder:

| Folder | What goes in it |
|---|---|
| `site.yaml` | Name, address, menus, collections, what `check` refuses |
| `content/` | Pages, as Markdown with front matter. A folder such as `articles/` is a collection |
| `layouts/` | Page shapes. `base.html` wraps every page; the others fill its `main` block |
| `partials/` | Pieces the layouts share |
| `sections/` | Blocks a page stacks in its front matter, instead of writing HTML |
| `data/` | YAML or JSON the templates read |
| `assets/` | CSS, scripts, fonts, published with a content hash in the name |
| `static/` | Files copied as they are |

The output is plain files for any web server, on bare metal, a VPS or in a
container; `docs/publishing.md` covers each. `DESIGN.md` has the plan and what
comes next.
