package endpoint

import (
	"bufio"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The submissions viewer: stored form submissions, read by the editor over a
// listener of its own. Never the public address: the public endpoint stays
// write-only. It answers only with the token, and only on a loopback or
// private address, since submissions are people's personal data.
//
//	viewer:
//	  listen: 10.0.0.5:8182         # a private or loopback address
//	  token_env: HOTDOG_VIEWER_TOKEN # at least 32 characters
//
//	GET /sites/<host>/forms/<form>?limit=50&offset=0   newest first

// ViewerConfig turns the viewer on.
type ViewerConfig struct {
	Listen   string `yaml:"listen"`
	TokenEnv string `yaml:"token_env"`
}

func (v ViewerConfig) check() error {
	if v.Listen == "" {
		return nil
	}
	host, _, err := net.SplitHostPort(v.Listen)
	if err != nil {
		return fmt.Errorf("viewer.listen: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !(ip.IsLoopback() || ip.IsPrivate()) {
		return fmt.Errorf("viewer.listen %q should be a loopback or private address (10.x, 192.168.x, a WireGuard address…): submissions are personal data", v.Listen)
	}
	if v.TokenEnv == "" {
		return fmt.Errorf("viewer needs token_env, the variable holding its token")
	}
	return nil
}

type viewer struct {
	s     *Server
	token []byte
}

// Viewer is the submissions viewer and where it listens, or nil when the
// endpoint has none.
func (s *Server) Viewer() (http.Handler, string, error) {
	v := s.cfg.Viewer
	if v.Listen == "" {
		return nil, "", nil
	}
	tok := os.Getenv(v.TokenEnv)
	if len(tok) < 32 {
		return nil, "", fmt.Errorf("%s must hold the viewer's token, at least 32 characters (openssl rand -hex 32)", v.TokenEnv)
	}
	return &viewer{s: s, token: []byte(tok)}, v.Listen, nil
}

type stored struct {
	Time   string            `json:"time"`
	Values map[string]string `json:"values"`
}

func (v *viewer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	got := []byte(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if subtle.ConstantTimeCompare(got, v.token) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if r.Method != http.MethodGet || len(parts) != 4 || parts[0] != "sites" || parts[2] != "forms" {
		http.NotFound(w, r)
		return
	}
	sr := v.s.sites[strings.ToLower(parts[1])]
	if sr == nil {
		http.NotFound(w, r)
		return
	}
	f := sr.forms[parts[3]]
	if f == nil || !f.Store || sr.cfg.Store == "" {
		http.Error(w, "no stored submissions for that form", http.StatusNotFound)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	var all []stored
	if file, err := os.Open(filepath.Join(sr.cfg.Store, f.Name+".jsonl")); err == nil {
		sc := bufio.NewScanner(file)
		sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
		for sc.Scan() {
			var e stored
			if json.Unmarshal(sc.Bytes(), &e) == nil {
				all = append(all, e)
			}
		}
		file.Close()
	}
	total := len(all)
	out := []stored{}
	for i := total - 1 - offset; i >= 0 && len(out) < limit; i-- {
		out = append(out, all[i]) // newest first
	}
	fields := []map[string]string{}
	for _, fd := range f.Fields {
		fields = append(fields, map[string]string{"name": fd.Name, "label": fd.Label})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"form": f.Name, "fields": fields, "total": total, "submissions": out})
}
