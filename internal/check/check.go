// Package check reads a built site and reports what would embarrass it in
// production: links to pages that don't exist, missing images, and strings
// the site has said must never be published.
package check

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/net/html"

	"git.coffeylabs.org/coffey-labs/cl-cms/internal/site"
)

// Problem is one finding.
type Problem struct {
	File string
	What string
}

func (p Problem) String() string { return p.File + ": " + p.What }

// Run checks the built site in out against the site in siteDir.
func Run(siteDir, out string, cfg *site.Config) ([]Problem, error) {
	var problems []Problem
	forbid := make([]*regexp.Regexp, 0, len(cfg.Check.Forbid))
	for _, f := range cfg.Check.Forbid {
		re, err := regexp.Compile(f)
		if err != nil {
			return nil, fmt.Errorf("site.yaml check.forbid %q: %w", f, err)
		}
		forbid = append(forbid, re)
	}

	// Forbidden strings, in what people write as well as in what gets built:
	// a template or data file can leak a host name as easily as a page.
	for _, d := range []string{"content", "data", "layouts", "partials", "sections", "assets", "static", "forms", "site.yaml"} {
		_ = filepath.WalkDir(filepath.Join(siteDir, d), func(p string, e fs.DirEntry, err error) error {
			if err != nil || e.IsDir() || !textFile(p) {
				return nil
			}
			problems = append(problems, scan(siteDir, p, forbid)...)
			return nil
		})
	}

	err := filepath.WalkDir(out, func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(out, p)
		rel = filepath.ToSlash(rel)
		if textFile(p) {
			for _, pr := range scan(out, p, forbid) {
				pr.File = "public/" + pr.File
				problems = append(problems, pr)
			}
		}
		if !strings.HasSuffix(p, ".html") {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		pageURL := "/" + rel
		pageURL = strings.TrimSuffix(pageURL, "index.html")
		for _, ref := range links(f) {
			target, ok := internal(ref, pageURL, cfg.URL)
			if !ok {
				continue
			}
			if !exists(out, target) {
				problems = append(problems, Problem{File: "public/" + rel, What: "links to " + ref + ", which is not in the site"})
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(problems, func(i, j int) bool { return problems[i].File < problems[j].File })
	return problems, nil
}

func textFile(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".md", ".html", ".htm", ".css", ".js", ".json", ".yaml", ".yml", ".txt", ".xml", ".svg", ".webmanifest":
		return true
	}
	return false
}

func scan(root, p string, forbid []*regexp.Regexp) []Problem {
	if len(forbid) == 0 {
		return nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	rel, _ := filepath.Rel(root, p)
	var out []Problem
	for _, re := range forbid {
		if loc := re.FindIndex(data); loc != nil {
			line := 1 + strings.Count(string(data[:loc[0]]), "\n")
			out = append(out, Problem{File: fmt.Sprintf("%s:%d", filepath.ToSlash(rel), line), What: fmt.Sprintf("matches forbidden pattern %q", re.String())})
		}
	}
	return out
}

// links returns every href and src in an HTML document, srcset included.
func links(f *os.File) []string {
	var out []string
	z := html.NewTokenizer(f)
	for {
		switch z.Next() {
		case html.ErrorToken:
			return out
		case html.StartTagToken, html.SelfClosingTagToken:
			t := z.Token()
			for _, a := range t.Attr {
				switch a.Key {
				case "href", "src", "poster":
					if t.Data == "link" && a.Key == "href" && hasAttr(t, "rel", "preconnect", "dns-prefetch") {
						continue
					}
					out = append(out, a.Val)
				case "srcset":
					for _, part := range strings.Split(a.Val, ",") {
						if f := strings.Fields(part); len(f) > 0 {
							out = append(out, f[0])
						}
					}
				}
			}
		}
	}
}

func hasAttr(t html.Token, key string, vals ...string) bool {
	for _, a := range t.Attr {
		if a.Key != key {
			continue
		}
		for _, v := range vals {
			if strings.Contains(a.Val, v) {
				return true
			}
		}
	}
	return false
}

// internal resolves a reference against the page it is on, and reports
// whether it points into this site.
func internal(ref, pageURL, siteURL string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.HasPrefix(ref, "#") {
		return "", false
	}
	u, err := url.Parse(ref)
	if err != nil {
		return "", false
	}
	if u.Scheme != "" || u.Host != "" {
		site, err := url.Parse(siteURL)
		if err != nil || u.Host != site.Host || (u.Scheme != "http" && u.Scheme != "https") {
			return "", false
		}
		return u.Path, true
	}
	if strings.HasPrefix(u.Path, "/") {
		return u.Path, true
	}
	return path.Join(path.Dir(pageURL+"x"), u.Path), true
}

func exists(root, urlPath string) bool {
	if urlPath == "" {
		urlPath = "/"
	}
	p := filepath.Join(root, filepath.FromSlash(path.Clean("/"+urlPath)))
	fi, err := os.Stat(p)
	if err != nil {
		return false
	}
	if fi.IsDir() {
		_, err = os.Stat(filepath.Join(p, "index.html"))
		return err == nil
	}
	return true
}
