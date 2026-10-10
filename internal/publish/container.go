package publish

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"git.coffeylabs.org/coffey-labs/cl-cms/internal/build"
	"git.coffeylabs.org/coffey-labs/cl-cms/internal/check"
)

// The image serves files and nothing else: an unprivileged web server, the
// site, and a config that answers the way cl-cms lays a site out. Folders get
// their index.html, a missing path gets 404.html with a 404, fingerprinted
// assets are cached for a year, and pages are revalidated every time.

const nginxContainerfile = `# Written by cl-cms. The site, served by unprivileged nginx on port 8080.
FROM docker.io/nginxinc/nginx-unprivileged:1.29-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY site/ /usr/share/nginx/html/
EXPOSE 8080
`

const nginxConf = `server {
    listen 8080;
    server_name _;
    root /usr/share/nginx/html;
    index index.html;
    server_tokens off;
    # Redirects (/about -> /about/) keep the visitor's own host and port. An
    # absolute redirect would name the container's port 8080, which the
    # outside world can't reach.
    absolute_redirect off;

    add_header X-Content-Type-Options "nosniff" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;
    add_header X-Frame-Options "SAMEORIGIN" always;

    location / {
        try_files $uri $uri/ =404;
        add_header Cache-Control "no-cache" always;
        add_header X-Content-Type-Options "nosniff" always;
        add_header Referrer-Policy "strict-origin-when-cross-origin" always;
        add_header X-Frame-Options "SAMEORIGIN" always;
    }

    # cl-cms puts a content hash in every asset's name, so a name never
    # changes content and can be cached for good.
    location ~ "\.[0-9a-f]{10}\.[A-Za-z0-9]+$" {
        add_header Cache-Control "public, max-age=31536000, immutable" always;
        add_header X-Content-Type-Options "nosniff" always;
    }

    location = /` + build.Marker + ` { return 404; }
    location = /` + build.RedirectsFile + ` { return 404; }
{{REDIRECTS}}    error_page 404 /404.html;
}
`

const caddyContainerfile = `# Written by cl-cms. The site, served by Caddy on port 8080.
FROM docker.io/library/caddy:2-alpine
COPY Caddyfile /etc/caddy/Caddyfile
COPY site/ /srv/
EXPOSE 8080
`

const caddyFile = `:8080 {
	root * /srv
	header {
		X-Content-Type-Options nosniff
		Referrer-Policy strict-origin-when-cross-origin
		X-Frame-Options SAMEORIGIN
		-Server
	}
	@hashed path_regexp \.[0-9a-f]{10}\.[A-Za-z0-9]+$
	header @hashed Cache-Control "public, immutable, max-age=31536000"
	header ?Cache-Control "no-cache"
	respond /` + build.Marker + ` 404
	respond /` + build.RedirectsFile + ` 404
{{REDIRECTS}}	file_server
	handle_errors {
		@404 expression {err.status_code} == 404
		rewrite @404 /404.html
		file_server
	}
}
`

// container writes a build context and, unless only the context was asked
// for, builds the image and pushes it if told to.
func container(t Target, out string, log io.Writer) error {
	ctx := t.Context
	if ctx == "" {
		dir, err := os.MkdirTemp("", "cl-cms-image-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		ctx = dir
	} else if err := os.MkdirAll(ctx, 0o755); err != nil {
		return err
	}
	csp := ""
	if t.CSP {
		csp = check.Policy(out)
	}
	if err := writeContext(ctx, out, t.Server, csp); err != nil {
		return err
	}
	if t.Image == "" {
		fmt.Fprintf(log, "build context written to %s\n", ctx)
		return nil
	}
	tool := t.Tool
	if tool == "" {
		for _, c := range []string{"podman", "docker"} {
			if _, err := exec.LookPath(c); err == nil {
				tool = c
				break
			}
		}
	}
	if tool == "" {
		return fmt.Errorf("container target needs docker or podman installed, or set context to only write the build context")
	}
	run := func(args ...string) error {
		cmd := exec.Command(tool, args...)
		cmd.Stdout, cmd.Stderr = log, log
		return cmd.Run()
	}
	if err := run("build", "-t", t.Image, ctx); err != nil {
		return fmt.Errorf("%s build: %w", tool, err)
	}
	fmt.Fprintf(log, "built image %s\n", t.Image)
	if t.Push {
		if err := run("push", t.Image); err != nil {
			return fmt.Errorf("%s push: %w", tool, err)
		}
		fmt.Fprintf(log, "pushed %s\n", t.Image)
	}
	return nil
}

func writeContext(ctx, out, server, csp string) error {
	site := filepath.Join(ctx, "site")
	if err := os.RemoveAll(site); err != nil {
		return err
	}
	if err := build.CopyTree(out, site); err != nil {
		return err
	}
	// The site's redirects become real 301s; the pages the build wrote at
	// the old addresses are only the fallback for hosts that can't do this.
	var nginxRules, caddyRules strings.Builder
	if raw, err := os.ReadFile(filepath.Join(out, build.RedirectsFile)); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			from, to, ok := strings.Cut(line, " ")
			if !ok || strings.ContainsAny(from+to, "\";{}\n\r\t ") {
				return fmt.Errorf("redirect %q can't be written into a server config", line)
			}
			fmt.Fprintf(&nginxRules, "    location = %s { return 301 %s; }\n", from, to)
			fmt.Fprintf(&caddyRules, "\tredir %s %s 301\n", from, to)
		}
	}
	nconf, cconf := nginxConf, caddyFile
	if csp != "" {
		if strings.ContainsAny(csp, "\"\n\r") {
			return fmt.Errorf("content security policy can't be written into a server config")
		}
		h := `add_header Content-Security-Policy "` + csp + `" always;`
		nconf = strings.ReplaceAll(nconf, `add_header X-Content-Type-Options "nosniff" always;`, `add_header X-Content-Type-Options "nosniff" always;`+"\n        "+h)
		cconf = strings.Replace(cconf, "\t\tX-Content-Type-Options nosniff", "\t\tX-Content-Type-Options nosniff\n\t\tContent-Security-Policy \""+csp+"\"", 1)
	}
	files := map[string]string{"Containerfile": nginxContainerfile, "nginx.conf": strings.Replace(nconf, "{{REDIRECTS}}", nginxRules.String(), 1)}
	if server == "caddy" {
		files = map[string]string{"Containerfile": caddyContainerfile, "Caddyfile": strings.Replace(cconf, "{{REDIRECTS}}", caddyRules.String(), 1)}
	}
	// Dockerfile as well, for tools that look only for that name.
	files["Dockerfile"] = files["Containerfile"]
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(ctx, name), []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}
