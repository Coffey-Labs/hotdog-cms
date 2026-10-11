package editor

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// The live preview shows a page as it is being edited, before it's saved,
// rendered by the same engine as the build. It runs on its own origin
// (<token>.<domain>), not the editor's: a site's own scripts run in the
// preview, and on a separate origin they can't reach the editor's session.
// Each person, site and branch gets an unguessable host name that expires
// after an idle while; only the editor may frame it.

// LiveConfig is where the live preview listens and how browsers reach it.
type LiveConfig struct {
	Listen string `yaml:"listen"` // default 127.0.0.1:8191
	Domain string `yaml:"domain"` // previews are <token>.<domain>; default localhost
	Port   string `yaml:"port"`   // the port browsers use; default the listening port on localhost
	Scheme string `yaml:"scheme"` // default http
}

type liveEntry struct {
	owner   string
	dir     string            // the branch's full build, for everything but the page being edited
	overlay map[string][]byte // page address -> HTML rendered from the unsaved edit
	used    time.Time
}

type liveServer struct {
	cfg     LiveConfig
	editor  string // the editor's origin, the only one allowed to frame previews
	mu      sync.Mutex
	entries map[string]*liveEntry // by token
	byOwner map[string]string
}

const liveIdle = 2 * time.Hour

func newLiveServer(cfg LiveConfig, editorOrigin string) *liveServer {
	if cfg.Listen == "" {
		cfg.Listen = "127.0.0.1:8191"
	}
	if cfg.Domain == "" {
		cfg.Domain = "localhost"
	}
	if cfg.Scheme == "" {
		cfg.Scheme = "http"
	}
	if cfg.Port == "" && cfg.Domain == "localhost" {
		if _, p, err := net.SplitHostPort(cfg.Listen); err == nil {
			cfg.Port = p
		}
	}
	return &liveServer{cfg: cfg, editor: editorOrigin, entries: map[string]*liveEntry{}, byOwner: map[string]string{}}
}

func (l *liveServer) origin() string {
	return l.cfg.Scheme + "://*." + l.cfg.Domain + portSuffix(l.cfg.Port)
}

// show points an owner's preview at a build and, if html is given, overlays
// one page with the unsaved version, plus any other files (such as a look
// stylesheet not yet saved). It returns the page's preview address.
func (l *liveServer) show(owner, dir, pagePath string, html []byte, extra map[string][]byte) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	for tok, e := range l.entries {
		if now.Sub(e.used) > liveIdle {
			delete(l.entries, tok)
			delete(l.byOwner, e.owner)
		}
	}
	tok, ok := l.byOwner[owner]
	if !ok {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		tok = hex.EncodeToString(b)
		l.byOwner[owner] = tok
		l.entries[tok] = &liveEntry{owner: owner, overlay: map[string][]byte{}}
	}
	e := l.entries[tok]
	if e.dir != dir {
		e.dir, e.overlay = dir, map[string][]byte{}
	}
	if html != nil {
		e.overlay[pagePath] = html
	}
	for p, data := range extra {
		e.overlay[p] = data
	}
	e.used = now
	return l.cfg.Scheme + "://" + tok + "." + l.cfg.Domain + portSuffix(l.cfg.Port) + pagePath
}

func (l *liveServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Security-Policy", "frame-ancestors "+l.editor)
	h.Set("X-Robots-Tag", "noindex, nofollow, noarchive")
	h.Set("Cache-Control", "no-store")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Content-Type-Options", "nosniff")
	host := r.Host
	if hh, _, err := net.SplitHostPort(host); err == nil {
		host = hh
	}
	tok, ok := strings.CutSuffix(strings.ToLower(host), "."+l.cfg.Domain)
	l.mu.Lock()
	e := l.entries[tok]
	if e != nil {
		e.used = time.Now()
	}
	var html []byte
	if e != nil {
		html = e.overlay[r.URL.Path]
	}
	dir := ""
	if e != nil {
		dir = e.dir
	}
	l.mu.Unlock()
	if !ok || e == nil {
		http.Error(w, "This preview has expired. Reopen the page in the editor.", http.StatusNotFound)
		return
	}
	if html != nil {
		switch path.Ext(r.URL.Path) {
		case ".css":
			h.Set("Content-Type", "text/css; charset=utf-8")
		default:
			h.Set("Content-Type", "text/html; charset=utf-8")
		}
		_, _ = w.Write(html)
		return
	}
	clean := path.Clean("/" + r.URL.Path)
	p := filepath.Join(dir, filepath.FromSlash(clean))
	if fi, err := os.Stat(p); err == nil && fi.IsDir() {
		if !strings.HasSuffix(r.URL.Path, "/") {
			http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
			return
		}
		p = filepath.Join(p, "index.html")
	}
	if fi, err := os.Stat(p); err != nil || fi.IsDir() || strings.HasPrefix(filepath.Base(p), ".hotdog-cms") {
		if data, err := os.ReadFile(filepath.Join(dir, "404.html")); err == nil {
			h.Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write(data)
			return
		}
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, p)
}
