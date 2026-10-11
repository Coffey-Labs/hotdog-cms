package build

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
)

// Templates holds one parsed set per layout. Each set is the shared part
// (layouts/base.html, every partial, every section) plus that layout's own
// file, which fills the blocks base.html leaves open.
//
// Names, as a template refers to them:
//
//	{{ template "head" . }}           partials/head.html
//	{{ template "section/hero" . }}   sections/hero.html
//	{{ template "sections" . }}       every section in the page's front matter
type Templates struct {
	layouts  map[string]*template.Template
	sections map[string]bool
}

// SectionContext is what a section template receives: its own fields, and
// the page and site around it.
type SectionContext struct {
	Type string
	Data map[string]any
	Page *site.Page
	Site *site.Site
}

func loadTemplates(dir string, funcs template.FuncMap) (*Templates, error) {
	t := &Templates{layouts: map[string]*template.Template{}, sections: map[string]bool{}}
	baseSrc, err := os.ReadFile(filepath.Join(dir, "layouts", "base.html"))
	if err != nil {
		return nil, fmt.Errorf("layouts/base.html: %w", err)
	}
	common := template.New("base").Funcs(funcs)
	if _, err := common.Parse(string(baseSrc)); err != nil {
		return nil, fmt.Errorf("layouts/base.html: %w", err)
	}
	partials, err := htmlFiles(filepath.Join(dir, "partials"))
	if err != nil {
		return nil, err
	}
	for _, name := range partials {
		if err := parseInto(common, name, filepath.Join(dir, "partials", name+".html"), "partials/"); err != nil {
			return nil, err
		}
	}
	secs, err := htmlFiles(filepath.Join(dir, "sections"))
	if err != nil {
		return nil, err
	}
	// One dispatching template, written from the section files that exist, so
	// a page can name its sections in front matter without anything being
	// looked up by name at run time.
	var dispatch strings.Builder
	dispatch.WriteString(`{{define "hotdog:section"}}`)
	for i, name := range secs {
		if err := parseInto(common, "section/"+name, filepath.Join(dir, "sections", name+".html"), "sections/"); err != nil {
			return nil, err
		}
		t.sections[name] = true
		kw := "else if"
		if i == 0 {
			kw = "if"
		}
		fmt.Fprintf(&dispatch, `{{%s eq .Type %q}}{{template %q .}}`, kw, name, "section/"+name)
	}
	if len(secs) > 0 {
		dispatch.WriteString(`{{end}}`)
	}
	dispatch.WriteString(`{{end}}`)
	dispatch.WriteString(`{{define "sections"}}{{range .Sections}}{{template "hotdog:section" (sectionContext $ .)}}{{end}}{{end}}`)
	if _, err := common.Parse(dispatch.String()); err != nil {
		return nil, err
	}

	layouts, err := htmlFiles(filepath.Join(dir, "layouts"))
	if err != nil {
		return nil, err
	}
	for _, name := range layouts {
		if name == "base" {
			continue
		}
		set, err := common.Clone()
		if err != nil {
			return nil, err
		}
		src, err := os.ReadFile(filepath.Join(dir, "layouts", name+".html"))
		if err != nil {
			return nil, err
		}
		if _, err := set.New("layout/" + name).Parse(string(src)); err != nil {
			return nil, fmt.Errorf("layouts/%s.html: %w", name, err)
		}
		t.layouts[name] = set
	}
	return t, nil
}

func parseInto(set *template.Template, name, file, label string) error {
	src, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	if _, err := set.New(name).Parse(string(src)); err != nil {
		return fmt.Errorf("%s%s: %w", label, filepath.Base(file), err)
	}
	return nil
}

func htmlFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".html") {
			out = append(out, strings.TrimSuffix(e.Name(), ".html"))
		}
	}
	sort.Strings(out)
	return out, nil
}

// layoutFor picks a page's layout: its own front matter, else its
// collection's setting, else the default for its kind.
func (t *Templates) layoutFor(p *site.Page, cfg *site.Config) (string, error) {
	try := []string{}
	if p.Layout != "" {
		if _, ok := t.layouts[p.Layout]; !ok {
			return "", fmt.Errorf("layout %q: no layouts/%s.html", p.Layout, p.Layout)
		}
		return p.Layout, nil
	}
	switch p.Kind {
	case site.KindPage:
		if p.IsHome() {
			try = append(try, "home")
		}
		if p.Collection != "" {
			if l := cfg.Collections[p.Collection].Layout; l != "" {
				try = append(try, l)
			}
		}
		try = append(try, "page")
	case site.KindList:
		if l := cfg.Collections[p.Collection].ListLayout; l != "" {
			try = append(try, l)
		}
		try = append(try, "list")
	case site.KindTerm:
		try = append(try, "term", "list")
	case site.KindTerms:
		try = append(try, "terms", "list")
	case site.KindError:
		try = append(try, "404", "page")
	}
	for _, name := range try {
		if _, ok := t.layouts[name]; ok {
			return name, nil
		}
	}
	return "", fmt.Errorf("no layout for this %s page (tried %s)", p.Kind, strings.Join(try, ", "))
}

func (t *Templates) checkSections(p *site.Page) error {
	for _, s := range p.Sections {
		if !t.sections[s.Type] {
			return fmt.Errorf("section %q: no sections/%s.html", s.Type, s.Type)
		}
	}
	return nil
}
