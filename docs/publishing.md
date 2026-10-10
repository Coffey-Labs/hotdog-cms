# Publishing

cl-cms builds a site into plain files. Where those files go, and how they get
there, is the operator's choice: cl-cms doesn't require a particular host,
container platform or pipeline.

Every content change arrives through git, whether someone edited Markdown or
used the editor. Publishing is the step after that, and there are two ways to
run it:

- **Push**: something with the repository (CI, a deploy script, or a person)
  runs `cl-cms publish`, which builds and sends the site to one or more targets.
- **Pull**: the web server runs `cl-cms pull`, which fetches the repository and
  rebuilds when the branch moves. Nothing connects in to the server, so it needs
  no deploy access; the repository can be read with a read-only key.

## Every route is checked

Before anything is published, `cl-cms check` runs on the build
([checks.md](checks.md)). `cl-cms publish` stops on an error, and the pull
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

    cl-cms publish server          # build, then publish to one target
    cl-cms publish -all            # every target in the file
    cl-cms publish -no-build image # publish the existing build

`dir` and `rsync` only replace a folder that is empty or was written by cl-cms,
so a wrong path can't wipe something else.

## Pull: the agent on the server

    cl-cms pull -repo https://git.example.org/me/site.git -out /var/www/example.org -every 1m

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
ExecStart=/usr/local/bin/cl-cms pull -repo https://git.example.org/me/site.git -out /var/www/example.org -every 1m
Restart=on-failure
NoNewPrivileges=yes
ProtectSystem=strict
ReadWritePaths=/var/www /var/cache/cl-cms
Environment=XDG_CACHE_HOME=/var/cache/cl-cms

[Install]
WantedBy=multi-user.target
```

The service user needs write access to the document root's parent folder
(the swap renames the root), and nothing else.

## Serving the files

Any web server that serves files works. What cl-cms output expects:

- a folder's `index.html` for its address (`/about/` serves `about/index.html`);
- `404.html` for a missing page, with status 404;
- long caching for fingerprinted assets (names like `styles.3f9a1c2b07.css`)
  and revalidation for pages.

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
    location = /.cl-cms-build { return 404; }
    error_page 404 /404.html;
}
```

Caddy:

```
example.org {
	root * /var/www/example.org
	@hashed path_regexp \.[0-9a-f]{10}\.[A-Za-z0-9]+$
	header @hashed Cache-Control "public, max-age=31536000, immutable"
	file_server
	handle_errors {
		rewrite * /404.html
		file_server
	}
}
```

The `container` target writes the same rules into its image.
