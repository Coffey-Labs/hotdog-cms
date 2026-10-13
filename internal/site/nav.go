package site

import (
	"fmt"
	"path"
	"strings"
)

// A navigation tree, from a data file: the docs pack's sidebar, breadcrumb
// and previous and next links, and any site's nested menu. Each entry is a
// page, a heading over more entries (with a page of its own or not), or a
// link elsewhere:
//
//	- page: docs/index.md            # a source file under content/
//	- title: Run it
//	  page: docs/run/_index.md       # optional: the heading's own page
//	  children:
//	    - docs/run/sizing.md         # a page, titled as it titles itself
//	    - title: Clustering
//	      page: docs/run/clustering.md
//	- title: Back to the main site
//	  url: https://example.org/
//
// url: may also be an address on the site (/about/). Any other key (icon:,
// color:) is kept in the entry's .Params for the theme. The docs pack's first
// format, parts and page names, still reads:
//
//	- part: start
//	  title: Start here
//	  blurb: What it is.
//	  pages: [what-it-is, how-it-works]
//
// An entry whose page isn't in this build (a draft, say) is left out; a
// heading over it keeps the rest.

// NavItem is one entry in a navigation tree, as seen from one page.
type NavItem struct {
	Title    string
	URL      string // the page's address, or the link's
	Page     *Page  // nil for a heading without a page, or a link elsewhere
	Blurb    string
	External bool
	Children []*NavItem
	Here     bool // this is the page being shown
	Open     bool // the page being shown is this one or under it
	Parent   *NavItem
	// Params holds the entry's other keys, as written (icon: rocket, say),
	// for a theme to draw with.
	Params map[string]any
}

// Nav is a navigation tree as seen from one page: where that page is in it,
// and its neighbours in reading order (every entry with a page, depth first,
// a heading's page before what's under it).
type Nav struct {
	Items      []*NavItem
	Here       *NavItem   // nil if the page isn't in the tree
	Trail      []*NavItem // the headings above Here, from the top
	Prev, Next *NavItem
	Pages      []*NavItem // every entry with a page, in reading order
}

// PageFrom returns the page built from a source file under content/
// ("docs/run/sizing.md"), or nil.
func (s *Site) PageFrom(source string) *Page {
	if s.bySource == nil {
		s.bySource = map[string]*Page{}
		for _, p := range s.Pages {
			if p.Source != "" {
				s.bySource[p.Source] = p
			}
		}
	}
	return s.bySource[strings.TrimPrefix(path.Clean("/"+source), "/")]
}

// NavFor reads the navigation tree in data/<name> and places page in it.
func (s *Site) NavFor(name string, page *Page) (*Nav, error) {
	raw, ok := s.Data[name]
	if !ok {
		return nil, fmt.Errorf("no data/%s.yaml or data/%s.json", name, name)
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("data/%s: a navigation tree is a list of entries", name)
	}
	nav := &Nav{}
	var err error
	if nav.Items, err = s.navItems(name, list, nil); err != nil {
		return nil, err
	}
	var walk func([]*NavItem)
	walk = func(items []*NavItem) {
		for _, it := range items {
			if it.Page != nil {
				nav.Pages = append(nav.Pages, it)
				if page != nil && it.Page == page && nav.Here == nil {
					it.Here = true
					nav.Here = it
				}
			}
			walk(it.Children)
		}
	}
	walk(nav.Items)
	if h := nav.Here; h != nil {
		for i, it := range nav.Pages {
			if it == h {
				if i > 0 {
					nav.Prev = nav.Pages[i-1]
				}
				if i < len(nav.Pages)-1 {
					nav.Next = nav.Pages[i+1]
				}
			}
		}
		for it := h; it != nil; it = it.Parent {
			it.Open = true
			if it != h {
				nav.Trail = append([]*NavItem{it}, nav.Trail...)
			}
		}
	}
	return nav, nil
}

func (s *Site) navItems(name string, list []any, parent *NavItem) ([]*NavItem, error) {
	var out []*NavItem
	for _, raw := range list {
		it := &NavItem{Parent: parent}
		var children []any
		switch e := raw.(type) {
		case string:
			it.Page = s.PageFrom(e)
			if it.Page == nil {
				continue
			}
		case map[string]any:
			it.Title, it.Blurb = str(e["title"]), str(e["blurb"])
			it.Params = map[string]any{}
			for k, v := range e {
				switch k {
				case "title", "blurb", "page", "url", "children", "part", "pages":
				default:
					it.Params[k] = v
				}
			}
			if part := str(e["part"]); part != "" {
				// The docs pack's first format: a part's page and its pages by name.
				it.Page = s.PageFrom("docs/" + part + "/_index.md")
				for _, slug := range strList(e["pages"]) {
					pg := s.PageFrom("docs/" + part + "/" + slug + ".md")
					if pg == nil {
						pg = s.PageFrom("docs/" + part + "/" + slug + "/index.md")
					}
					if pg != nil {
						it.Children = append(it.Children, &NavItem{Title: pg.Title, URL: pg.Path, Page: pg, Parent: it})
					}
				}
			} else {
				if p := str(e["page"]); p != "" {
					if it.Page = s.PageFrom(p); it.Page == nil && e["children"] == nil {
						continue
					}
				}
				if u := str(e["url"]); u != "" && it.Page == nil {
					if strings.HasPrefix(u, "/") {
						if it.Page = s.PageAt(u); it.Page == nil {
							it.URL = u
						}
					} else {
						it.URL, it.External = u, true
					}
				}
				if c, ok := e["children"].([]any); ok {
					children = c
				} else if e["children"] != nil {
					return nil, fmt.Errorf("data/%s: %q: children should be a list", name, it.Title)
				}
			}
		default:
			return nil, fmt.Errorf("data/%s: an entry should be a page's source file or a title with page:, url: or children:", name)
		}
		kids, err := s.navItems(name, children, it)
		if err != nil {
			return nil, err
		}
		it.Children = append(it.Children, kids...)
		if it.Page != nil {
			it.URL = it.Page.Path
			if it.Title == "" {
				it.Title = it.Page.Title
			}
		}
		if it.Page == nil && it.URL == "" && len(it.Children) == 0 {
			continue
		}
		out = append(out, it)
	}
	return out, nil
}
