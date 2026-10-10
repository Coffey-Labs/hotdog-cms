# Previews

`cl-cms preview` builds every branch of a site's repository exactly as it
would be published, and serves each one at its own address. A change on a
branch, or later an editor's draft, can be looked at for real before it
reaches the branch the site publishes from.

    cl-cms preview -repo https://git.example.org/me/site.git

Then open `http://localhost:8160/` for the list, or go straight to a branch at
`http://<branch>.localhost:8160/`. Browsers send every `*.localhost` name to
your own machine, so this needs no DNS.

## What makes it a real preview

- **The same build as production.** The one change is the site's address,
  so links stay on the preview. Nothing is injected into the pages: no
  banner, no reload script.
- **Served by the same rules as production:**
  - a folder serves its `index.html`;
  - a missing page gets `404.html` with a 404;
  - the build marker is hidden.
- **Kept up to date.** Branches that move are rebuilt, at `-every`, 30 seconds
  by default. If a build fails, the previous one stays up and the list shows
  the error. A deleted branch loses its preview.
- **Drafts included** by default (`-drafts=false` to leave them out), since
  that's usually what a preview is for.

## Previews are private

They show work that hasn't been published, so:

- every response says `noindex`, `no-store` and `no-referrer`, and
  `robots.txt` turns crawlers away;
- the server listens on loopback (`127.0.0.1:8160`) unless told otherwise;
- to listen on any other address it requires credentials, and refuses to start
  without them:

```sh
CLCMS_PREVIEW_AUTH='editor:a-long-password' \
  cl-cms preview -repo ... -addr 0.0.0.0:8160 -domain preview.example.org -scheme https
```

Put TLS in front, and point a wildcard DNS name (`*.preview.example.org`) at
it. Like the editor, the preview server belongs beside the CMS, never on the
host that serves the public site.

## Options

| Flag | Default | Meaning |
|---|---|---|
| `-repo` | (required) | The site's repository |
| `-subdir` | | The site's folder inside the repository |
| `-branches` | every branch | Patterns such as `main,draft/*` |
| `-addr` | `127.0.0.1:8160` | Where to listen |
| `-domain` | `localhost` | Previews are `<branch>.<domain>` |
| `-scheme`, `-port` | `http`, the listening port | What browsers use, behind a proxy |
| `-every` | `30s` | How often to look for moved branches |
| `-root` | user cache | Where previews are built |
