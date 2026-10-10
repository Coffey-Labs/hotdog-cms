package site

import (
	"html/template"
	"strings"
	"time"
)

// Page kinds.
const (
	KindPage  = "page"  // a Markdown file
	KindList  = "list"  // a collection's index: content/<c>/_index.md, or generated
	KindTerm  = "term"  // one taxonomy value: /tags/linux/
	KindTerms = "terms" // a taxonomy's index: /tags/
	KindError = "404"   // content/404.md
)

// Page is anything that becomes one HTML file.
type Page struct {
	Kind       string
	Source     string // path under content/, slash-separated; empty for generated pages
	Title      string
	Summary    string
	Date       time.Time
	Updated    time.Time
	Draft      bool
	NoIndex    bool
	Layout     string
	Path       string // URL path: "/", "/about/", "/articles/x/", "/404.html"
	Permalink  string // absolute URL
	Image      string // social card, as written in front matter
	Weight     int
	Content    template.HTML
	Params     map[string]any // every front matter key, as written
	Sections   []Section
	Collection string              // first folder under content/, if any
	Terms      map[string][]string // taxonomy -> values on this page
	Pages      []*Page             // for list, term and terms pages: what they list
	Term       string              // for a term page: the value; for a terms page: the taxonomy
	Taxonomy   string              // for term and terms pages
	Prev, Next *Page               // neighbours within the collection, in its order
	WordCount  int
	Site       *Site
}

// Section is one block of a page built from sections.
type Section struct {
	Type string
	Data map[string]any
}

// IsHome reports whether this is the front page.
func (p *Page) IsHome() bool { return p.Path == "/" }

// ReadingTime is minutes at 220 words a minute, at least one.
func (p *Page) ReadingTime() int {
	m := (p.WordCount + 219) / 220
	if m < 1 {
		return 1
	}
	return m
}

// Lastmod is the date a sitemap should give: updated, else date.
func (p *Page) Lastmod() time.Time {
	if !p.Updated.IsZero() {
		return p.Updated
	}
	return p.Date
}

// Param returns one front matter value, or nil.
func (p *Page) Param(key string) any { return p.Params[key] }

// InSection reports whether the page lives under the given URL path, for
// marking the current item in a menu: /articles/ is active on /articles/x/.
func (p *Page) InSection(path string) bool {
	if path == "/" {
		return p.Path == "/"
	}
	return strings.HasPrefix(p.Path, path)
}
