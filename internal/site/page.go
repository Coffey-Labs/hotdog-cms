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
	ImageAlt   string // what the social card shows, for people who can't see it
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
	Raw        string     // the Markdown body as written
	RedirectTo string     // front matter redirect_to: listed like any page, built as a redirect
	PublishAt  time.Time  // front matter publish_at: left out of live builds until then
	LeadImage  *LeadImage // the picture the body opens with, if it opens with one
	Paginator  *Paginator // for list and taxonomy pages
	Event      *Event     // set when the front matter has start: (event.go)
	Site       *Site

	eventStart, eventEnd string // start: and end: as written, read in the site's time zone
}

// LeadImage is an image at the very top of a page's body, which templates can
// use as the page's social card.
type LeadImage struct{ Src, Alt string }

// Paginator is one page of a paginated list.
type Paginator struct {
	Number     int     // 1-based
	TotalPages int     // at least 1
	TotalItems int     // across every page
	Items      []*Page // the items on this page
	First      *Page   // page 1
	Prev, Next *Page   // neighbouring pages, nil at either end
	All        []*Page // every page of the list, in order
}

// PageLink is one entry in a row of numbered page links.
type PageLink struct {
	Number  int
	Path    string
	Current bool
	Gap     bool // stands for the pages left out: draw "…"
}

// Links is a row of numbered links: the first and last pages, and those
// within around of the current one, with gaps for the rest.
//
//	{{ range .Paginator.Links 2 }}…{{ end }}
func (pg *Paginator) Links(around int) []PageLink {
	var out []PageLink
	for i, p := range pg.All {
		n := i + 1
		if n == 1 || n == pg.TotalPages || (n >= pg.Number-around && n <= pg.Number+around) {
			out = append(out, PageLink{Number: n, Path: p.Path, Current: n == pg.Number})
		} else if len(out) > 0 && !out[len(out)-1].Gap {
			out = append(out, PageLink{Gap: true})
		}
	}
	return out
}

// Section is one block of a page built from sections.
type Section struct {
	Type string
	Data map[string]any
}

// Href is where a link to the page should go: its own address, or, for a
// page that redirects (a link post, a page that moved), its destination.
func (p *Page) Href() string {
	if p.RedirectTo != "" {
		return p.RedirectTo
	}
	return p.Path
}

// IsHome reports whether this is the front page.
func (p *Page) IsHome() bool { return p.Path == "/" }

// ReadingTime is minutes at 220 words a minute, rounded, at least one.
func (p *Page) ReadingTime() int {
	m := (p.WordCount + 110) / 220
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
