package build

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"git.coffeylabs.org/coffey-labs/cl-cms/internal/site"
)

func indexable(p *site.Page) bool {
	return !p.NoIndex && !p.Draft && p.Kind != site.KindError
}

func writeSitemap(s *site.Site, out string) error {
	type url struct {
		Loc     string `xml:"loc"`
		Lastmod string `xml:"lastmod,omitempty"`
	}
	type urlset struct {
		XMLName xml.Name `xml:"urlset"`
		NS      string   `xml:"xmlns,attr"`
		URLs    []url    `xml:"url"`
	}
	set := urlset{NS: "http://www.sitemaps.org/schemas/sitemap/0.9"}
	for _, p := range s.Pages {
		if !indexable(p) {
			continue
		}
		u := url{Loc: p.Permalink}
		if lm := p.Lastmod(); !lm.IsZero() {
			u.Lastmod = lm.Format("2006-01-02")
		}
		set.URLs = append(set.URLs, u)
	}
	return writeXML(filepath.Join(out, "sitemap.xml"), set)
}

func writeRobots(s *site.Site, out string) error {
	body := "User-agent: *\nAllow: /\n"
	if s.Config.Outputs.SitemapOn() {
		body += "\nSitemap: " + s.Config.URL + "/sitemap.xml\n"
	}
	return os.WriteFile(filepath.Join(out, "robots.txt"), []byte(body), 0o644)
}

// writeFeed writes an Atom feed for one collection.
func writeFeed(s *site.Site, name string, cc site.CollectionConfig, out string) (string, error) {
	type link struct {
		Href string `xml:"href,attr"`
		Rel  string `xml:"rel,attr,omitempty"`
	}
	type text struct {
		Type string `xml:"type,attr,omitempty"`
		Body string `xml:",chardata"`
	}
	type entry struct {
		Title   string `xml:"title"`
		Link    link   `xml:"link"`
		ID      string `xml:"id"`
		Updated string `xml:"updated"`
		Summary string `xml:"summary,omitempty"`
		Content text   `xml:"content"`
	}
	type author struct {
		Name string `xml:"name"`
	}
	type feed struct {
		XMLName xml.Name `xml:"feed"`
		NS      string   `xml:"xmlns,attr"`
		Title   string   `xml:"title"`
		ID      string   `xml:"id"`
		Updated string   `xml:"updated"`
		Links   []link   `xml:"link"`
		Author  *author  `xml:"author,omitempty"`
		Entries []entry  `xml:"entry"`
	}
	path := cc.FeedPath
	if path == "" {
		path = "/" + name + "/feed.xml"
	}
	size := cc.FeedSize
	if size <= 0 {
		size = 20
	}
	list := s.Lists[name]
	title := s.Config.Name
	if list != nil && list.Title != "" {
		title = list.Title + " · " + s.Config.Name
	}
	f := feed{
		NS:    "http://www.w3.org/2005/Atom",
		Title: title,
		ID:    s.Config.URL + path,
		Links: []link{{Href: s.Config.URL + path, Rel: "self"}, {Href: s.Config.URL + "/" + name + "/"}},
	}
	if s.Config.Author != "" {
		f.Author = &author{Name: s.Config.Author}
	}
	var newest time.Time
	for i, p := range s.Collection(name) {
		if i >= size {
			break
		}
		if !indexable(p) {
			continue
		}
		lm := p.Lastmod()
		if lm.IsZero() {
			lm = s.BuildTime
		}
		if lm.After(newest) {
			newest = lm
		}
		f.Entries = append(f.Entries, entry{
			Title: p.Title, Link: link{Href: p.Permalink}, ID: p.Permalink,
			Updated: lm.Format(time.RFC3339), Summary: p.Summary,
			Content: text{Type: "html", Body: string(p.Content)},
		})
	}
	if newest.IsZero() {
		newest = s.BuildTime
	}
	f.Updated = newest.Format(time.RFC3339)
	dst := filepath.Join(out, filepath.FromSlash(strings.TrimPrefix(path, "/")))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	return path, writeXML(dst, f)
}

// writeLLMs writes llms.txt: the site's pages with their summaries, for
// language-model crawlers that read it instead of every page.
func writeLLMs(s *site.Site, out string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", s.Config.Name)
	if s.Config.Description != "" {
		fmt.Fprintf(&b, "> %s\n\n", s.Config.Description)
	}
	b.WriteString("## Pages\n\n")
	for _, p := range s.Pages {
		if !indexable(p) || p.Kind == site.KindTerm || p.Kind == site.KindTerms {
			continue
		}
		line := fmt.Sprintf("- [%s](%s)", p.Title, p.Permalink)
		if p.Summary != "" {
			line += ": " + p.Summary
		}
		b.WriteString(line + "\n")
	}
	return os.WriteFile(filepath.Join(out, "llms.txt"), []byte(b.String()), 0o644)
}

func writeXML(path string, v any) error {
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	buf.WriteByte('\n')
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
