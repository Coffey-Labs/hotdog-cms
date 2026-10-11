package preview

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/forge"
)

// Auth is the one set of credentials a preview server asks for.
type Auth struct{ User, Pass string }

// Handler serves the index of previews at the bare domain and each preview
// at <slug>.<domain>.
func (b *Builder) Handler(auth *Auth) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		// Previews are unpublished by definition: never indexed, never cached,
		// never framed by another site.
		h.Set("X-Robots-Tag", "noindex, nofollow, noarchive")
		h.Set("Cache-Control", "no-store")
		if len(b.o.FrameAncestors) > 0 {
			h.Set("Content-Security-Policy", "frame-ancestors 'self' "+strings.Join(b.o.FrameAncestors, " "))
		} else {
			h.Set("X-Frame-Options", "SAMEORIGIN")
		}
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		// The webhook proves itself with the platform's signature instead of
		// the preview credentials, which a platform can't send.
		if r.URL.Path == "/_hotdog/hook" {
			b.hook(w, r)
			return
		}
		if auth != nil {
			u, p, ok := r.BasicAuth()
			if !ok || subtle.ConstantTimeCompare([]byte(u), []byte(auth.User)) != 1 ||
				subtle.ConstantTimeCompare([]byte(p), []byte(auth.Pass)) != 1 {
				h.Set("WWW-Authenticate", `Basic realm="hotdog-cms previews", charset="UTF-8"`)
				http.Error(w, "Sign in to see previews.", http.StatusUnauthorized)
				return
			}
		}
		if r.URL.Path == "/robots.txt" {
			h.Set("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprint(w, "User-agent: *\nDisallow: /\n")
			return
		}
		host := r.Host
		if hh, _, err := net.SplitHostPort(host); err == nil {
			host = hh
		}
		host = strings.ToLower(strings.TrimSuffix(host, "."))
		if host == b.o.Domain {
			b.index(w)
			return
		}
		slug, ok := strings.CutSuffix(host, "."+b.o.Domain)
		if !ok || strings.Contains(slug, ".") {
			http.Error(w, "No preview at this address.", http.StatusNotFound)
			return
		}
		dir, ok := b.dirFor(slug)
		if !ok {
			http.Error(w, "No preview at this address. It may still be building, or its branch may be gone.", http.StatusNotFound)
			return
		}
		serveFiles(w, r, dir)
	})
}

// serveFiles answers the way the production web server config does.
func serveFiles(w http.ResponseWriter, r *http.Request, root string) {
	clean := filepath.Clean("/" + r.URL.Path)
	if strings.HasPrefix(filepath.Base(clean), ".hotdog-cms") {
		clean = "/nonexistent" // hotdog-cms's own bookkeeping, as in production
	}
	p := filepath.Join(root, filepath.FromSlash(clean))
	if fi, err := os.Stat(p); err == nil && fi.IsDir() {
		if !strings.HasSuffix(r.URL.Path, "/") {
			http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
			return
		}
		p = filepath.Join(p, "index.html")
	}
	if fi, err := os.Stat(p); err != nil || fi.IsDir() {
		data, err := os.ReadFile(filepath.Join(root, "404.html"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write(data)
		return
	}
	http.ServeFile(w, r, p)
}

var indexTpl = template.Must(template.New("index").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex"><title>Previews</title>
<style>
:root{color-scheme:light dark;--bg:#fbfaf7;--fg:#1d2430;--muted:#5d6573;--line:#e4e1da;--card:#fff;--bad:#b42318}
@media (prefers-color-scheme:dark){:root{--bg:#12161c;--fg:#e9e6e0;--muted:#a2a8b3;--line:#2a313c;--card:#1a2029;--bad:#f97066}}
body{margin:0;background:var(--bg);color:var(--fg);font:16px/1.5 system-ui,sans-serif}
main{width:min(880px,100% - 32px);margin:48px auto}
h1{margin:0 0 4px}p.sub{color:var(--muted);margin:0 0 28px}
ul{list-style:none;padding:0;display:grid;gap:12px}
li{background:var(--card);border:1px solid var(--line);border-radius:12px;padding:16px 18px}
a{color:inherit;font-weight:650}code{font-size:.9em}.meta{color:var(--muted);font-size:.9rem}
.err{color:var(--bad);white-space:pre-wrap;font-size:.88rem;margin:8px 0 0}.bad{color:var(--bad)}
</style></head><body><main>
<h1>Previews</h1>
<p class="sub">Each branch, built exactly as it would be published. Nothing here is live.</p>
{{ if not . }}<p>No branches yet.</p>{{ end }}
<ul>{{ range . }}<li>
  {{ if .Commit }}<a href="{{ .URL }}">{{ .Branch }}</a>{{ else }}<strong>{{ .Branch }}</strong>{{ end }}
  <div class="meta">{{ if .Commit }}<code>{{ printf "%.10s" .Commit }}</code> · {{ .Pages }} pages · built {{ .Built.Format "2 Jan 15:04:05" }}{{ else }}no good build yet{{ end }}</div>
  {{ if .Commit }}<div class="meta">{{ if .Errors }}<strong class="bad">{{ .Errors }} check error(s): publishing is blocked until they're fixed</strong>{{ else }}Passes the checks{{ end }}{{ with .Warnings }} · {{ . }} warning(s){{ end }}</div>{{ end }}
  {{ with .Error }}<p class="err">The last build failed, so the previous build (if any) is still shown.
{{ . }}</p>{{ end }}
</li>{{ end }}</ul>
</main></body></html>`))

func (b *Builder) index(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = indexTpl.Execute(w, b.List())
}

// Serve listens on addr. Off loopback it insists on credentials: previews
// show what hasn't been published, so they are private unless set otherwise.
func (b *Builder) Serve(addr string, auth *Auth, every time.Duration) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	loopback := host == "localhost" || (ip != nil && ip.IsLoopback())
	if !loopback && auth == nil {
		return fmt.Errorf("previews show unpublished work: to listen on %s, set credentials (HOTDOG_PREVIEW_AUTH=user:password) and put TLS in front", addr)
	}
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			if err := b.Sync(); err != nil {
				fmt.Fprintf(b.o.Log, "previews: %v\n", err)
			}
			select {
			case <-t.C:
			case <-b.kick:
			}
		}
	}()
	srv := &http.Server{Addr: addr, Handler: b.Handler(auth), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 120 * time.Second, MaxHeaderBytes: 16 << 10}
	fmt.Fprintf(b.o.Log, "previews of %s at %s://%s%s/\n", b.o.Repo, b.o.Scheme, b.o.Domain, portSuffix(b.o.Port))
	return srv.ListenAndServe()
}

// hook takes a platform's push webhook and syncs at once.
func (b *Builder) hook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if b.o.HookSecret == "" {
		http.Error(w, "No webhook is set up here.", http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 5<<20))
	if err != nil {
		http.Error(w, "unreadable body", http.StatusBadRequest)
		return
	}
	kind := forge.Git
	if b.o.Forge != nil {
		kind = b.o.Forge.Repo.Kind
	}
	if err := forge.Verify(kind, r.Header, body, b.o.HookSecret); err != nil {
		fmt.Fprintf(b.o.Log, "webhook refused: %v\n", err)
		http.Error(w, "signature check failed", http.StatusUnauthorized)
		return
	}
	push, err := forge.ParsePush(kind, r.Header, body)
	if errors.Is(err, forge.ErrNotPush) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		http.Error(w, "unreadable push event", http.StatusBadRequest)
		return
	}
	if b.wanted(push.Branch) {
		fmt.Fprintf(b.o.Log, "webhook: %s moved to %s\n", push.Branch, short(push.Commit))
		b.Kick()
	}
	w.WriteHeader(http.StatusAccepted)
}

func portSuffix(p string) string {
	if p == "" {
		return ""
	}
	return ":" + p
}
