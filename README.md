# HotDog CMS

Websites from Markdown files and templates, with no database. Write in
Markdown, see it live, publish from git. [hotdogcms.org](https://hotdogcms.org)

## Install

Download `hotdog-cms` for Linux, macOS or Windows from the
[releases](https://git.coffeylabs.org/coffey-labs/hotdog-cms/releases) and check it
against `SHA256SUMS`. Or, with Go 1.26 or newer:

    go install git.coffeylabs.org/coffey-labs/hotdog-cms/cmd/hotdog-cms@latest

The editor, previews and the pull agent also need `git`.

## Use

    hotdog-cms new mysite        start a site from the starter
    cd mysite
    hotdog-cms serve             preview at http://127.0.0.1:8140, reloading as you save
    hotdog-cms build             write the finished site to public/
    hotdog-cms check [-csp] [-fix] build, then check it: tracking, privacy, links, SEO, security (docs/checks.md)
    hotdog-cms publish <target>  build, then publish: a folder, a server over SSH, a container image, or your own command
    hotdog-cms pull -repo URL -out DIR   on the web server: publish from git when the branch moves
    hotdog-cms preview -repo URL     every branch built as it would be published, at <branch>.localhost:8160
    hotdog-cms endpoint -config FILE forms and search, behind your web server (docs/forms.md)
    hotdog-cms import <from>       bring a site in: wordpress, wxr, ghost, hugo, jekyll (docs/importing.md)
    hotdog-cms pack add <pack>     sections, layouts and styles from a folder or repository, no code (docs/packs.md)
    hotdog-cms ci <platform>         the CI file for GitHub, GitLab, Gitea, Forgejo, Bitbucket, Woodpecker, or a git hook

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
container; `docs/publishing.md` covers each, `docs/previews.md` covers
looking at a change before it's published, and `docs/platforms.md` covers
GitHub, GitLab, Gitea, Forgejo, Bitbucket and plain git. The editor is in
`docs/editor.md`, quick posts in `docs/quick-posts.md`, events and calendar
feeds in `docs/events.md`. `docs/editor-roadmap.md` has what's built and what comes next, and
`docs/security.md` how it's kept safe.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Contributors accept the
[contributor agreement](CLA.md) once, in their first pull request.

## License

Copyright (C) 2026 Coffey Labs LLC.

The engine and the editor are AGPL-3.0-or-later; see [LICENSE](LICENSE).
The starter site in `starter/` and HotDog themes are MIT; see
[starter/site/LICENSE-starter.txt](starter/site/LICENSE-starter.txt). A
site built with HotDog CMS carries no AGPL obligations. The vendored WebP
encoder in `internal/media/webpenc/` keeps its own MIT and BSD licenses.
