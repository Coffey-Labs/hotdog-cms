package build

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
)

// funcs is everything a template can call. There is deliberately no way to
// mark a string as trusted HTML: the only HTML that reaches a page unescaped
// is what came through the Markdown renderer, which applies the site's raw
// HTML rule. A template author cannot open a hole by accident.
func funcs(s *site.Site, a *Assets, md *site.Markdown, icons Icons) template.FuncMap {
	hashes := map[string]string{} // fileHash results; pages render one at a time
	return template.FuncMap{
		// asset "styles.css" -> /styles.3f9a1c2b07.css; an unknown name fails the build.
		"asset": a.URL,
		// absURL "/about/" -> https://example.org/about/
		"absURL": func(p string) string {
			if strings.Contains(p, "://") {
				return p
			}
			return s.Config.URL + "/" + strings.TrimPrefix(p, "/")
		},
		// markdownify renders a front matter or data string as Markdown.
		"markdownify": func(v any) (template.HTML, error) {
			return md.Render([]byte(fmt.Sprint(v)))
		},
		// inline renders a short Markdown string without a paragraph around it.
		"inline": func(v any) (template.HTML, error) {
			return md.Inline([]byte(fmt.Sprint(v)))
		},
		// icon "search" "i" inlines icons/search.svg with that class.
		"icon": icons.render,
		// split "\n" .Data.heading, for a heading written over two lines.
		"split": func(sep string, v any) []string { return strings.Split(strings.TrimRight(fmt.Sprint(v), "\n"), sep) },
		// date "2 January 2006" .Date; "iso" gives 2006-01-02, "rfc3339" the full stamp.
		"date": func(layout string, t time.Time) string {
			if t.IsZero() {
				return ""
			}
			switch layout {
			case "iso":
				return t.Format("2006-01-02")
			case "rfc3339":
				return t.Format("2006-01-02T15:04:05-07:00") // RFC 3339, with +00:00 rather than Z
			}
			return t.Format(layout)
		},
		// slugify takes anything, because front matter doesn't promise strings:
		// `tags: [2013, Security]` has a number in it.
		"slugify":    func(v any) string { return site.Slugify(fmt.Sprint(v)) },
		"collection": s.Collection,
		// pageAt "/articles/x/": the page at an address, or nil.
		"pageAt": s.PageAt,
		"menu":   s.Menu,
		"data": func(name string) (any, error) {
			v, ok := s.Data[name]
			if !ok {
				return nil, fmt.Errorf("no data/%s.yaml or data/%s.json", name, name)
			}
			return v, nil
		},
		"limit": func(n int, list any) (any, error) {
			v := reflect.ValueOf(list)
			if v.Kind() != reflect.Slice {
				return nil, errors.New("limit: not a list")
			}
			if n < v.Len() {
				return v.Slice(0, n).Interface(), nil
			}
			return list, nil
		},
		"dict": func(kv ...any) (map[string]any, error) {
			if len(kv)%2 != 0 {
				return nil, errors.New("dict: needs key, value pairs")
			}
			m := make(map[string]any, len(kv)/2)
			for i := 0; i < len(kv); i += 2 {
				k, ok := kv[i].(string)
				if !ok {
					return nil, errors.New("dict: keys must be strings")
				}
				m[k] = kv[i+1]
			}
			return m, nil
		},
		"list":   func(v ...any) []any { return v },
		"append": func(l []any, v ...any) []any { return append(append([]any{}, l...), v...) },
		// concat joins lists: concat .a .b
		"concat": func(lists ...any) []any {
			var out []any
			for _, l := range lists {
				if xs, ok := l.([]any); ok {
					out = append(out, xs...)
				}
			}
			return out
		},
		// hostOf "https://www.example.org/x" -> www.example.org
		"hostOf": func(v any) string {
			u, err := url.Parse(fmt.Sprint(v))
			if err != nil {
				return ""
			}
			return u.Hostname()
		},
		"lower":    strings.ToLower,
		"upper":    strings.ToUpper,
		"join":     strings.Join,
		"contains": strings.Contains,
		// hasPrefix "/img/" .  and  hasSuffix ".jpg" .  (the affix first, like trimPrefix)
		"hasPrefix": func(prefix, s string) bool { return strings.HasPrefix(s, prefix) },
		"hasSuffix": func(suffix, s string) bool { return strings.HasSuffix(s, suffix) },
		// cond test a b: a if test, else b.
		"cond": func(test bool, a, b any) any {
			if test {
				return a
			}
			return b
		},
		"trimPrefix": func(prefix, s string) string { return strings.TrimPrefix(s, prefix) },
		"trimSuffix": func(suffix, s string) string { return strings.TrimSuffix(s, suffix) },
		"replace":    func(old, new, s string) string { return strings.ReplaceAll(s, old, new) },
		"default": func(def, v any) any {
			if v == nil || v == "" {
				return def
			}
			return v
		},
		"now": func() time.Time { return s.BuildTime },
		// kindIs "string" .x: what a front matter or data value is (string, map, list, number, bool).
		"kindIs": func(kind string, v any) bool {
			switch v.(type) {
			case string:
				return kind == "string"
			case map[string]any:
				return kind == "map"
			case []any:
				return kind == "list"
			case int, int64, float64:
				return kind == "number"
			case bool:
				return kind == "bool"
			}
			return false
		},
		// joinAny ", " .list: join a list of any values as text.
		"joinAny": func(sep string, v []any) string {
			parts := make([]string, len(v))
			for i, x := range v {
				parts[i] = fmt.Sprint(x)
			}
			return strings.Join(parts, sep)
		},
		"add": func(a, b int) int { return a + b },
		"mod": func(a, b int) int { return a % b },
		// excerpt 220 .Summary: plain text, cut at a word, with an ellipsis.
		"excerpt": func(n int, v any) string { return site.Excerpt(n, fmt.Sprint(v)) },
		// termURL "tags" "Open Source" -> /tags/open-source/ (or "" if it has none).
		"termURL": s.TermURL,
		// isExternal: an absolute link to another site.
		"isExternal": md.External,
		// Cloudflare's markers that turn Email Address Obfuscation off for a
		// region. Fixed strings, so nothing a page writes can reach them.
		"emailOff":    func() template.HTML { return "<!--email_off-->" },
		"emailOffEnd": func() template.HTML { return "<!--/email_off-->" },
		// formatDate "January 2006" "2025-03-04": a date written as text.
		"formatDate": func(layout string, v any) (string, error) {
			str := strings.TrimSpace(fmt.Sprint(v))
			if str == "" || v == nil {
				return "", nil
			}
			t, err := time.Parse("2006-01-02", str)
			if err != nil {
				return "", fmt.Errorf("formatDate: %q is not a date like 2026-10-10", str)
			}
			return t.Format(layout), nil
		},
		// staticFile "/img/x.jpg": whether static/ has it.
		"staticFile": func(p string) bool {
			fi, err := os.Stat(staticPath(s.Dir, p))
			return err == nil && !fi.IsDir()
		},
		// imageSize "/img/x.jpg" -> {w, h}, for og:image dimensions.
		"imageSize": func(p string) (map[string]int, error) {
			f, err := os.Open(staticPath(s.Dir, p))
			if err != nil {
				return nil, err
			}
			defer f.Close()
			c, _, err := image.DecodeConfig(f)
			if err != nil {
				return nil, fmt.Errorf("imageSize %s: %w", p, err)
			}
			return map[string]int{"w": c.Width, "h": c.Height}, nil
		},
		// fileHash "/img/x.jpg": 8 hex characters of the file's MD5, for a
		// cache-busting ?v= on files in static/ that have no hashed name.
		"fileHash": func(p string) (string, error) {
			if h, ok := hashes[p]; ok {
				return h, nil
			}
			b, err := os.ReadFile(staticPath(s.Dir, p))
			if err != nil {
				return "", err
			}
			sum := md5.Sum(b)
			hashes[p] = hex.EncodeToString(sum[:])[:8]
			return hashes[p], nil
		},
		"sectionContext": func(p *site.Page, sec site.Section) SectionContext {
			return SectionContext{Type: sec.Type, Data: sec.Data, Page: p, Site: s}
		},
	}
}

// staticPath maps a site path like /img/x.jpg into the site's static folder,
// never outside it.
func staticPath(siteDir, p string) string {
	return filepath.Join(siteDir, "static", filepath.FromSlash(path.Clean("/"+p)))
}
