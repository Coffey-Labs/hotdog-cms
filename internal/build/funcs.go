package build

import (
	"errors"
	"fmt"
	"html/template"
	"reflect"
	"strings"
	"time"

	"git.coffeylabs.org/coffey-labs/cl-cms/internal/site"
)

// funcs is everything a template can call. There is deliberately no way to
// mark a string as trusted HTML: the only HTML that reaches a page unescaped
// is what came through the Markdown renderer, which applies the site's raw
// HTML rule. A template author cannot open a hole by accident.
func funcs(s *site.Site, a *Assets, md *site.Markdown, icons Icons) template.FuncMap {
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
				return t.Format(time.RFC3339)
			}
			return t.Format(layout)
		},
		// slugify takes anything, because front matter doesn't promise strings:
		// `tags: [2013, Security]` has a number in it.
		"slugify":    func(v any) string { return site.Slugify(fmt.Sprint(v)) },
		"collection": s.Collection,
		"menu":       s.Menu,
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
		"list":       func(v ...any) []any { return v },
		"lower":      strings.ToLower,
		"upper":      strings.ToUpper,
		"join":       strings.Join,
		"contains":   strings.Contains,
		"hasPrefix":  strings.HasPrefix,
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
		"add": func(a, b int) int { return a + b },
		"sectionContext": func(p *site.Page, sec site.Section) SectionContext {
			return SectionContext{Type: sec.Type, Data: sec.Data, Page: p, Site: s}
		},
	}
}
