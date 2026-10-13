package site

import (
	"html/template"
	"path"
	"regexp"
	"strings"
)

// srcLinkRe finds links written to a page's source file, the way MkDocs
// sites and most Markdown editors write them: about.md,
// ../install/linux.md#docker. Only relative paths: an address with a scheme
// is somewhere else.
var srcLinkRe = regexp.MustCompile(`(<a\s[^>]*?href=")([^"#?:]+\.md)((?:#[^"]*)?)"`)

// resolveSourceLinks turns each link to another page's source file into that
// page's address, keeping any #fragment. Paths are relative to the linking
// page's own source, as on disk; a path starting with / is from content/. A
// section's index.md also finds its _index.md, MkDocs' name for what HotDog
// calls _index.md. A link to a .md file no page came from is left as written,
// for the link check to report.
func (s *Site) resolveSourceLinks() {
	bySource := map[string]*Page{}
	for _, p := range s.Pages {
		if p.Source != "" {
			bySource[p.Source] = p
		}
	}
	for _, p := range s.Pages {
		if p.Source == "" || !strings.Contains(string(p.Content), ".md") {
			continue
		}
		dir := path.Dir(p.Source)
		p.Content = template.HTML(srcLinkRe.ReplaceAllStringFunc(string(p.Content), func(m string) string {
			sub := srcLinkRe.FindStringSubmatch(m)
			target := sub[2]
			var rel string
			if strings.HasPrefix(target, "/") {
				rel = path.Clean(strings.TrimPrefix(target, "/"))
			} else {
				rel = path.Clean(path.Join(dir, target))
			}
			if strings.HasPrefix(rel, "../") || rel == ".." {
				return m
			}
			to := bySource[rel]
			if to == nil && path.Base(rel) == "index.md" {
				to = bySource[path.Join(path.Dir(rel), "_index.md")]
			}
			if to == nil && path.Base(rel) == "README.md" {
				to = bySource[path.Join(path.Dir(rel), "_index.md")]
			}
			if to == nil {
				return m
			}
			return sub[1] + to.Path + sub[3] + `"`
		}))
	}
}
