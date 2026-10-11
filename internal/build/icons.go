package build

import (
	"fmt"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/markup"
	"html"
	"html/template"
	"os"
	"path/filepath"
	"strings"
)

// Icons are the SVG files in a site's icons/ folder, inlined into pages by
// {{ icon "name" "class" }} so they take currentColor and follow the theme
// without a request each. They are the site's own files, trusted like its
// templates, but still checked when loaded: an icon is a picture, and nothing
// in one may run, fetch, or link anywhere.
type Icons map[string]string

func loadIcons(dir string) (Icons, error) {
	icons := Icons{}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return icons, nil
	}
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".svg") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		s := strings.TrimSpace(string(raw))
		if i := strings.Index(s, "<svg"); i > 0 {
			s = s[i:] // drop an XML declaration or comment before the root
		}
		if !strings.HasPrefix(s, "<svg") {
			return nil, fmt.Errorf("icons/%s: not an SVG", e.Name())
		}
		if found := markup.SVG(s); len(found) > 0 {
			return nil, fmt.Errorf("icons/%s: %s; an icon is a picture, nothing that runs or loads other files", e.Name(), strings.Join(found, "; "))
		}
		icons[strings.TrimSuffix(e.Name(), ".svg")] = s
	}
	return icons, nil
}

// render returns the icon with a class and the attributes that keep a
// decorative picture out of the accessibility tree and the tab order.
func (ic Icons) render(name string, class ...string) (template.HTML, error) {
	s, ok := ic[name]
	if !ok {
		return "", fmt.Errorf("no icons/%s.svg", name)
	}
	attrs := ` aria-hidden="true" focusable="false"`
	if len(class) > 0 && class[0] != "" {
		attrs = ` class="` + html.EscapeString(strings.Join(class, " ")) + `"` + attrs
	}
	// Safe: the markup is a checked file from the site itself, and the only
	// thing added to it is an escaped class name.
	return template.HTML("<svg" + attrs + s[len("<svg"):]), nil
}
