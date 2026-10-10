// Package serve is the local preview: it builds the site into a private
// folder, serves it, and rebuilds and reloads the browser when a file changes.
// It is for the person writing the site, on their own machine; it is not a
// production server.
package serve

import (
	"bytes"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"git.coffeylabs.org/coffey-labs/cl-cms/internal/build"
)

// Options for the preview server.
type Options struct {
	SiteDir string
	Addr    string // host:port; loopback by default
	Drafts  bool
}

const reloadScript = `<script>(()=>{const s=new EventSource("/_cl/events");` +
	`s.addEventListener("reload",()=>location.reload());` +
	`s.addEventListener("error-msg",e=>console.error("cl-cms build failed:\n"+e.data));})();</script>`

type hub struct {
	mu      sync.Mutex
	clients map[chan string]bool
}

func (h *hub) send(msg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c <- msg:
		default:
		}
	}
}

// Run serves until the process is stopped.
func Run(opt Options) error {
	siteDir, err := filepath.Abs(opt.SiteDir)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", opt.Addr)
	if err != nil {
		return err
	}
	base := "http://" + ln.Addr().String()
	tmp, err := os.MkdirTemp("", "cl-cms-preview-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	out := filepath.Join(tmp, "site")

	var mu sync.RWMutex
	rebuild := func() error {
		mu.Lock()
		defer mu.Unlock()
		res, err := build.Run(build.Options{SiteDir: siteDir, Out: out, Drafts: opt.Drafts, URL: base, Dev: true})
		if err != nil {
			return err
		}
		log.Printf("built %d pages in %s", res.Pages, res.Duration.Round(time.Millisecond))
		return nil
	}
	if err := rebuild(); err != nil {
		return err
	}

	h := &hub{clients: map[chan string]bool{}}
	go watch(siteDir, func() {
		if err := rebuild(); err != nil {
			log.Printf("build failed: %v", err)
			h.send("error-msg\n" + err.Error())
			return
		}
		h.send("reload\n")
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/_cl/events", func(w http.ResponseWriter, r *http.Request) {
		fl, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		c := make(chan string, 4)
		h.mu.Lock()
		h.clients[c] = true
		h.mu.Unlock()
		defer func() { h.mu.Lock(); delete(h.clients, c); h.mu.Unlock() }()
		fmt.Fprint(w, ": connected\n\n")
		fl.Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case msg := <-c:
				event, data, _ := strings.Cut(msg, "\n")
				fmt.Fprintf(w, "event: %s\n", event)
				for _, line := range strings.Split(data, "\n") {
					fmt.Fprintf(w, "data: %s\n", line)
				}
				fmt.Fprint(w, "\n")
				fl.Flush()
			}
		}
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		defer mu.RUnlock()
		servePreview(w, r, out)
	})
	log.Printf("previewing %s at %s (Ctrl+C to stop)", siteDir, base)
	return http.Serve(ln, mux)
}

// servePreview answers the way the production nginx config does: a folder
// gets its index.html, a missing path gets 404.html with a 404.
func servePreview(w http.ResponseWriter, r *http.Request, root string) {
	p := filepath.Join(root, filepath.FromSlash(filepath.Clean("/"+r.URL.Path)))
	if fi, err := os.Stat(p); err == nil && fi.IsDir() {
		if !strings.HasSuffix(r.URL.Path, "/") {
			http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
			return
		}
		p = filepath.Join(p, "index.html")
	}
	status := http.StatusOK
	data, err := os.ReadFile(p)
	if err != nil {
		status = http.StatusNotFound
		p = filepath.Join(root, "404.html")
		if data, err = os.ReadFile(p); err != nil {
			http.NotFound(w, r)
			return
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	if strings.HasSuffix(p, ".html") {
		data = bytes.Replace(data, []byte("</body>"), []byte(reloadScript+"</body>"), 1)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		_, _ = w.Write(data)
		return
	}
	if status != http.StatusOK {
		w.WriteHeader(status)
		_, _ = w.Write(data)
		return
	}
	http.ServeFile(w, r, p)
}

// watch calls fn after files under dir change, once things go quiet for a
// moment, so saving several files rebuilds once.
func watch(dir string, fn func()) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("watching disabled: %v", err)
		return
	}
	add := func(root string) {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			name := d.Name()
			if p != dir && (strings.HasPrefix(name, ".") || name == "public" || name == "node_modules") {
				return fs.SkipDir
			}
			_ = w.Add(p)
			return nil
		})
	}
	add(dir)
	var timer *time.Timer
	for {
		select {
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			if ev.Op&fsnotify.Create != 0 {
				if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
					add(ev.Name)
				}
			}
			base := filepath.Base(ev.Name)
			if strings.HasPrefix(base, ".") || strings.HasSuffix(base, "~") || strings.HasSuffix(base, ".swp") {
				continue
			}
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(150*time.Millisecond, fn)
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			log.Printf("watch: %v", err)
		}
	}
}
