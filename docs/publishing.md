# Publishing

HotDog CMS builds a site into plain files. Where those files go, and how they get
there, is the operator's choice: HotDog CMS doesn't require a particular host,
container platform or pipeline.

Every content change arrives through git, whether someone edited Markdown or
used the editor. Publishing is the step after that, and there are two ways to
run it:

- **Push**: something with the repository (CI, a deploy script, or a person)
  runs `hotdog-cms publish`, which builds and sends the site to one or more targets.
- **Pull**: the web server runs `hotdog-cms pull`, which fetches the repository and
  rebuilds when the branch moves. Nothing connects in to the server, so it needs
  no deploy access; the repository can be read with a read-only key.

## Every route is checked

Before anything is published, `hotdog-cms check` runs on the build
([checks.md](checks.md)). `hotdog-cms publish` stops on an error, and the pull
agent leaves the live site as it was. There is no flag to skip this; a rule a
site means to break is switched off in `site.yaml`, where it is reviewed with
everything else.

## Push: targets

Targets live in `publish.yaml` (see `publish.example.yaml`), or in a file you
keep outside the repository and pass with `-targets`.

| Type | For | What happens |
|---|---|---|
| `dir` | A web server on the same machine | The build replaces the document root in one swap |
| `rsync` | Bare metal or a VPS over SSH | Changed files go to a staging folder on the server, then one swap |
| `container` | Docker, Podman, Kubernetes, any container platform | An image with the site and nginx or Caddy on port 8080; optionally pushed |
| `command` | Anything else (object storage, a CDN, your own script) | Your command runs with the build folder |

    hotdog-cms publish server          # build, then publish to one target
    hotdog-cms publish -all            # every target in the file
    hotdog-cms publish -no-build image # publish the existing build

`dir` and `rsync` only replace a folder that is empty or was written by HotDog CMS,
so a wrong path can't wipe something else.

### Your own steps: before and after

Any target can run commands of yours around publishing: a search index, a
minifier, a cache purge, a ping to a monitor.

```yaml
targets:
  server:
    type: rsync
    host: deploy@${DEPLOY_HOST}
    path: /var/www/example.org
    before:
      - [pagefind, --site, "{out}"]
    after:
      - [curl, -fsS, "https://hc-ping.com/${HEALTHCHECK_ID}"]
```

- Each step is a command and its arguments, run **without a shell**, so
  nothing in a value is interpreted. `{out}` is the build folder;
  `HOTDOG_OUT` and `HOTDOG_TARGET` are set too.
- `before` runs once the site is built and may change the build. If a step
  fails, nothing is published.
- `after` runs once the site is live. If a step fails, the publish has still
  happened, and the error says so.

These steps run with your rights, so they belong to you. They live only in
the targets file, never in `site.yaml` (anyone who can edit the site can
change that). In the editor, `publish.yaml` and CI pipeline files (`.github/`,
`.gitea/`, `.forgejo/`, `.gitlab-ci.yml`, `.woodpecker*` and others) can be
changed only by maintainers of the repository. For the strongest separation,
keep the targets file outside the repository and pass it with `-targets`.

### Running the image locked down

The image only serves files, so it can run with almost everything a
container is allowed taken away. Both web servers run as an unprivileged user
on port 8080 (nginx as uid 101, Caddy as nobody, with its admin API off), so
the same flags work for either:

```
docker run -d -p 80:8080 \
  --read-only \
  --tmpfs /tmp:rw,noexec,nosuid,nodev \
  --cap-drop ALL --security-opt no-new-privileges \
  --memory 256m --pids-limit 128 \
  registry.example.org/example-site:latest
```

`--read-only` keeps the image as it was built; `/tmp` is the one writable
place, and nothing written there can run. With every capability dropped and
no way to gain one, nothing in the container can act as root. The limits
stop one site from starving others on the same machine: a static site uses
under 10 MB of memory and a handful of processes.

## Pull: the agent on the server

    hotdog-cms pull -repo https://git.example.org/me/site.git -out /var/www/example.org -every 1m

With no `-every`, it checks once and exits, which suits a systemd timer or a
webhook. A build that fails leaves the live site as it was.

A systemd service for the long-running form:

```ini
[Unit]
Description=Publish example.org from git
After=network-online.target
Wants=network-online.target

[Service]
User=site-publish
ExecStart=/usr/local/bin/hotdog-cms pull -repo https://git.example.org/me/site.git -out /var/www/example.org -every 1m
Restart=on-failure
NoNewPrivileges=yes
ProtectSystem=strict
ReadWritePaths=/var/www /var/cache/hotdog-cms
Environment=XDG_CACHE_HOME=/var/cache/hotdog-cms

[Install]
WantedBy=multi-user.target
```

The service user needs write access to the document root's parent folder
(the swap renames the root), and nothing else.

## Serving the files

Any web server that serves files works. What HotDog CMS output expects:

- a folder's `index.html` for its address (`/about/` serves `about/index.html`);
- `404.html` for a missing page, with status 404;
- long caching for fingerprinted assets (names like `styles.3f9a1c2b07.css`)
  and revalidation for pages;
- calendar feeds (`.ics`) sent as `text/calendar`;
- HotDog CMS's own bookkeeping files (`/.hotdog-cms-*`) not served.

nginx, for bare metal or a VPS:

```nginx
server {
    listen 443 ssl;
    server_name example.org;
    root /var/www/example.org;
    index index.html;
    absolute_redirect off;

    location / {
        try_files $uri $uri/ =404;
        add_header Cache-Control "no-cache";
    }
    location ~ "\.[0-9a-f]{10}\.[A-Za-z0-9]+$" {
        add_header Cache-Control "public, max-age=31536000, immutable";
    }
    location ~ "\.ics$" { default_type "text/calendar; charset=utf-8"; }
    location ~ "^/\.hotdog-cms-" { return 404; }
    error_page 404 /404.html;
}
```

Caddy:

```
example.org {
	root * /var/www/example.org
	@hashed path_regexp \.[0-9a-f]{10}\.[A-Za-z0-9]+$
	header @hashed Cache-Control "public, max-age=31536000, immutable"
	@ics path *.ics
	header @ics Content-Type "text/calendar; charset=utf-8"
	respond /.hotdog-cms-* 404
	file_server
	handle_errors {
		rewrite * /404.html
		file_server
	}
}
```

The `container` target writes the same rules into its image.

## Telling search engines what changed (IndexNow)

With `indexnow: { key: … }` in `site.yaml`, the build publishes the key as
`/<key>.txt`. A target with `indexnow: true` then pings IndexNow after each
successful publish, with the pages that changed, were added or went away
since that target's last publish. Bing, Yandex, Seznam and the others that
share IndexNow then re-read just those pages.

```yaml
targets:
  live:
    type: rsync
    host: deploy@web.example.org
    path: /var/www/example.org
    indexnow: true        # the live site only; staging targets leave it off
```

- **The pull agent:** takes `-indexnow` for the same thing.
- **What it remembers:** each build carries a manifest of its pages and a
  hash of each. What was last sent is kept per target in the user's cache
  folder, so a rebuild that changes nothing sends nothing.
- **When a ping fails:** it's reported, and the publish itself still counts.
