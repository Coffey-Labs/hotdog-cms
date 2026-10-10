package endpoint

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
)

// Search answered here instead of in the browser: the visitor downloads ten
// results instead of the whole index. It reads the same search.json the build
// writes for in-browser search, so a site can offer either, or both.
//
// Scoring: a hit in the title outranks one in the terms (tags), which
// outranks hits in the body (counted, up to five), and a page matching every
// word of the query earns a bonus.

type searchEntry struct {
	URL     string      `json:"u"`
	Title   string      `json:"t"`
	Date    string      `json:"d,omitempty"`
	Terms   [][2]string `json:"g,omitempty"`
	Summary string      `json:"s,omitempty"`
	Text    string      `json:"x"`

	lowTitle, lowTerms, lowText string
}

type searchIndex struct {
	mod     time.Time
	entries []*searchEntry
}

type searcher struct {
	mu    sync.Mutex
	cache map[string]*searchIndex // by file
}

func newSearcher() *searcher { return &searcher{cache: map[string]*searchIndex{}} }

var collectionRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// index loads a collection's search.json, again only when the file changes.
func (sr *searcher) index(file string) ([]*searchEntry, error) {
	fi, err := os.Stat(file)
	if err != nil {
		return nil, err
	}
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if ix, ok := sr.cache[file]; ok && ix.mod.Equal(fi.ModTime()) {
		return ix.entries, nil
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var entries []*searchEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}
	for _, e := range entries {
		e.lowTitle = strings.ToLower(e.Title)
		var t []string
		for _, g := range e.Terms {
			t = append(t, g[0])
		}
		e.lowTerms = strings.ToLower(strings.Join(t, " "))
		e.lowText = strings.ToLower(e.Text)
	}
	sr.cache[file] = &searchIndex{mod: fi.ModTime(), entries: entries}
	return entries, nil
}

var querySplit = regexp.MustCompile(`[^\p{L}\p{N}+#._-]+`)

func queryTerms(q string) []string {
	var out []string
	for _, t := range querySplit.Split(strings.ToLower(strings.TrimSpace(q)), -1) {
		if utf8.RuneCountInString(t) >= 2 {
			out = append(out, t)
		}
	}
	return out
}

type searchResult struct {
	URL     string      `json:"u"`
	Title   string      `json:"t"`
	Date    string      `json:"d,omitempty"`
	Terms   [][2]string `json:"g,omitempty"`
	Snippet string      `json:"snippet"`
	score   int
}

func search(entries []*searchEntry, q string, limit int) []searchResult {
	terms := queryTerms(q)
	if len(terms) == 0 {
		return nil
	}
	var out []searchResult
	for _, e := range entries {
		score, matched := 0, 0
		for _, t := range terms {
			hit := 0
			if strings.Contains(e.lowTitle, t) {
				hit += 12
			}
			if strings.Contains(e.lowTerms, t) {
				hit += 6
			}
			if n := strings.Count(e.lowText, t); n > 0 {
				hit += min(5, n)
			}
			if hit > 0 {
				matched++
				score += hit
			}
		}
		if matched == 0 {
			continue
		}
		if matched == len(terms) {
			score += 20
		}
		out = append(out, searchResult{URL: e.URL, Title: e.Title, Date: e.Date, Terms: e.Terms, Snippet: snippet(e.Text, terms), score: score})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].Date > out[j].Date
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// snippet is a window of plain text around the first word of the query found.
func snippet(md string, terms []string) string {
	plain := []rune(site.PlainMarkdown(md))
	lower := []rune(strings.ToLower(string(plain)))
	at := -1
	for _, t := range terms {
		if i := strings.Index(string(lower), t); i >= 0 {
			at = utf8.RuneCountInString(string(lower)[:i])
			break
		}
	}
	if at < 0 {
		if len(plain) > 200 {
			return string(plain[:200]) + "…"
		}
		return string(plain)
	}
	start := max(0, at-66)
	end := min(len(plain), start+200)
	out := strings.TrimSpace(string(plain[start:end])) + "…"
	if start > 0 {
		out = "…" + out
	}
	return out
}

// handleSearch answers GET /_hotdog/search/<collection>?q=...&page=N as JSON.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request, rt *siteRuntime, collection string) {
	if rt.cfg.Public == "" || !collectionRe.MatchString(collection) {
		http.NotFound(w, r)
		return
	}
	entries, err := s.search.index(filepath.Join(rt.cfg.Public, collection, "search.json"))
	if err != nil {
		http.Error(w, "no search for this collection", http.StatusNotFound)
		return
	}
	q := r.URL.Query().Get("q")
	if r := []rune(q); len(r) > 120 {
		q = string(r[:120])
	}
	per := 12
	if n, err := strconv.Atoi(r.URL.Query().Get("per_page")); err == nil && n > 0 && n <= 50 {
		per = n
	}
	results := search(entries, q, 40)
	pages := max(1, (len(results)+per-1)/per)
	page := 1
	if n, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && n > 1 {
		page = min(n, pages)
	}
	from := min(len(results), (page-1)*per)
	to := min(len(results), page*per)
	writeJSON(w, http.StatusOK, map[string]any{
		"query": q, "terms": queryTerms(q), "total": len(results), "page": page, "pages": pages, "results": results[from:to],
	})
}
