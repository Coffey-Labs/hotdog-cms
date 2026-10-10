package build

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"os"
	"path"
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

// writeRSS writes an RSS 2.0 feed for one collection: title, link, date,
// summary and categories per item.
func writeRSS(s *site.Site, name string, cc site.CollectionConfig, out string) (string, error) {
	type guid struct {
		IsPermaLink string `xml:"isPermaLink,attr"`
		Value       string `xml:",chardata"`
	}
	type item struct {
		Title       string   `xml:"title"`
		Link        string   `xml:"link"`
		GUID        guid     `xml:"guid"`
		PubDate     string   `xml:"pubDate,omitempty"`
		Description string   `xml:"description,omitempty"`
		Categories  []string `xml:"category"`
	}
	type atomLink struct {
		Href string `xml:"href,attr"`
		Rel  string `xml:"rel,attr"`
		Type string `xml:"type,attr"`
	}
	type channel struct {
		Title         string   `xml:"title"`
		Link          string   `xml:"link"`
		AtomLink      atomLink `xml:"atom:link"`
		Description   string   `xml:"description"`
		Language      string   `xml:"language"`
		LastBuildDate string   `xml:"lastBuildDate"`
		Items         []item   `xml:"item"`
	}
	type rss struct {
		XMLName xml.Name `xml:"rss"`
		Version string   `xml:"version,attr"`
		Atom    string   `xml:"xmlns:atom,attr"`
		Channel channel  `xml:"channel"`
	}
	path := cc.FeedPath
	if path == "" {
		path = "/" + name + "/feed.xml"
	}
	size := cc.FeedSize
	if size <= 0 {
		size = 20
	}
	title := cc.FeedTitle
	if title == "" {
		title = s.Config.Name
	}
	desc := cc.FeedDescription
	if desc == "" {
		desc = s.Config.Description
	}
	rfc := func(t time.Time) string { return t.UTC().Format("Mon, 02 Jan 2006 15:04:05") + " GMT" }
	ch := channel{
		Title: title, Link: s.Config.URL + "/" + name + "/",
		AtomLink:    atomLink{Href: s.Config.URL + path, Rel: "self", Type: "application/rss+xml"},
		Description: desc, Language: s.Config.Language, LastBuildDate: rfc(s.BuildTime),
	}
	for i, p := range s.Collection(name) {
		if i >= size {
			break
		}
		if !indexable(p) {
			continue
		}
		if i == 0 && !p.Date.IsZero() {
			ch.LastBuildDate = rfc(p.Date)
		}
		it := item{Title: p.Title, Link: p.Permalink, GUID: guid{"true", p.Permalink}, Description: site.Excerpt(400, p.Summary)}
		if !p.Date.IsZero() {
			it.PubDate = rfc(p.Date)
		}
		for _, tax := range s.Config.Taxonomies {
			for _, v := range strList(p.Params[tax]) {
				it.Categories = append(it.Categories, v)
			}
		}
		ch.Items = append(ch.Items, it)
	}
	dst := filepath.Join(out, filepath.FromSlash(strings.TrimPrefix(path, "/")))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	return path, writeXML(dst, rss{Version: "2.0", Atom: "http://www.w3.org/2005/Atom", Channel: ch})
}

func strList(v any) []string {
	switch t := v.(type) {
	case []any:
		var out []string
		for _, x := range t {
			if s := strings.TrimSpace(fmt.Sprint(x)); s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		var out []string
		for _, s := range strings.Split(t, ",") {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// writeSearch writes /<collection>/search.json: per page its address, title,
// date, terms, summary and Markdown body, for a search box to load when
// someone actually searches.
func writeSearch(s *site.Site, name string, out string) error {
	type entry struct {
		URL     string      `json:"u"`
		Title   string      `json:"t"`
		Date    string      `json:"d,omitempty"`
		Terms   [][2]string `json:"g,omitempty"` // [as written, address]
		Summary string      `json:"s,omitempty"`
		Text    string      `json:"x"`
	}
	var entries []entry
	for _, p := range s.Collection(name) {
		if !indexable(p) {
			continue
		}
		e := entry{URL: p.Path, Title: p.Title, Summary: site.Excerpt(220, p.Summary), Text: p.Raw}
		if !p.Date.IsZero() {
			e.Date = p.Date.Format("2006-01-02")
		}
		for _, tax := range s.Config.Taxonomies {
			for _, v := range strList(p.Params[tax]) {
				e.Terms = append(e.Terms, [2]string{v, s.TermURL(tax, v)})
			}
		}
		entries = append(entries, e)
	}
	b, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	dst := filepath.Join(out, name, "search.json")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

// RedirectsFile lists the site's redirects inside a build, one "from to" per
// line, for publishers that turn them into web server rules.
const RedirectsFile = ".cl-cms-redirects"

var redirectTpl = template.Must(template.New("r").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Moved</title>
<meta name="robots" content="noindex">
<link rel="canonical" href="{{ . }}">
<meta http-equiv="refresh" content="0; url={{ . }}">
</head><body><p>This page has moved to <a href="{{ . }}">{{ . }}</a>.</p></body></html>
`))

// writeRedirects writes a page at each old address that sends the visitor
// on, which works on any host, and the list a web server config can use to
// answer with a real 301 instead.
func writeRedirects(s *site.Site, out string) error {
	var list strings.Builder
	for _, r := range s.Config.Redirects {
		to := r.To
		fmt.Fprintf(&list, "%s %s\n", r.From, to)
		dst := pageFile(out, r.From)
		if !strings.HasSuffix(r.From, "/") && path.Ext(r.From) == "" {
			dst = filepath.Join(out, filepath.FromSlash(r.From), "index.html")
		}
		if _, err := os.Stat(dst); err == nil {
			return fmt.Errorf("redirect from %s: a page already lives there", r.From)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		var buf bytes.Buffer
		if err := redirectTpl.Execute(&buf, to); err != nil {
			return err
		}
		if err := os.WriteFile(dst, buf.Bytes(), 0o644); err != nil {
			return err
		}
	}
	if list.Len() == 0 {
		return nil
	}
	return os.WriteFile(filepath.Join(out, RedirectsFile), []byte(list.String()), 0o644)
}
