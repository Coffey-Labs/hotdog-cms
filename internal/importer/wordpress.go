package importer

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// WordPress, two ways: a live site through its REST API, or an export file
// (Tools → Export, WXR). With a username and an Application Password the
// API also gives drafts and private posts; without, what's published. The
// password is used for the requests and never written anywhere.

// WordPressOptions says where the site is and how to read it.
type WordPressOptions struct {
	URL      string // https://example.com
	User     string
	Password string // an Application Password (Users → Profile), not the login password
	Posts    string // the collection posts go into; default "articles"
	Client   *http.Client
}

type wpRendered struct {
	Rendered string `json:"rendered"`
}

type wpItem struct {
	ID       int        `json:"id"`
	Type     string     `json:"type"`
	Slug     string     `json:"slug"`
	Status   string     `json:"status"`
	Link     string     `json:"link"`
	DateGMT  string     `json:"date_gmt"`
	ModGMT   string     `json:"modified_gmt"`
	Parent   int        `json:"parent"`
	Title    wpRendered `json:"title"`
	Content  wpRendered `json:"content"`
	Excerpt  wpRendered `json:"excerpt"`
	Embedded struct {
		Author []struct {
			Name string `json:"name"`
		} `json:"author"`
		Media []struct {
			SourceURL string `json:"source_url"`
			AltText   string `json:"alt_text"`
		} `json:"wp:featuredmedia"`
		Terms [][]struct {
			Name     string `json:"name"`
			Taxonomy string `json:"taxonomy"`
		} `json:"wp:term"`
	} `json:"_embedded"`
}

// FromWordPress reads a live WordPress site's posts and pages.
func FromWordPress(o WordPressOptions) ([]Item, error) {
	if o.Client == nil {
		o.Client = &http.Client{Timeout: 60 * time.Second}
	}
	if o.Posts == "" {
		o.Posts = "articles"
	}
	base := strings.TrimRight(o.URL, "/") + "/wp-json/wp/v2/"
	status := "publish"
	if o.Password != "" {
		status = "publish,future,draft,pending,private"
	}
	get := func(kind string) ([]wpItem, error) {
		var all []wpItem
		for page, pages := 1, 1; page <= pages; page++ {
			q := url.Values{"per_page": {"100"}, "page": {strconv.Itoa(page)}, "_embed": {"1"}, "status": {status}, "orderby": {"id"}, "order": {"asc"}}
			req, _ := http.NewRequest(http.MethodGet, base+kind+"?"+q.Encode(), nil)
			req.Header.Set("Accept", "application/json")
			req.Header.Set("User-Agent", "hotdog-cms-import")
			if o.Password != "" {
				req.SetBasicAuth(o.User, o.Password)
			}
			res, err := o.Client.Do(req)
			if err != nil {
				return nil, err
			}
			body, _ := io.ReadAll(io.LimitReader(res.Body, 64<<20))
			res.Body.Close()
			if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
				return nil, fmt.Errorf("WordPress refused the sign-in (%s): use an Application Password from Users → Profile, not the login password", res.Status)
			}
			if res.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("%s: %s", kind, res.Status)
			}
			var batch []wpItem
			if err := json.Unmarshal(body, &batch); err != nil {
				return nil, fmt.Errorf("%s: not the WordPress REST API? %w", kind, err)
			}
			all = append(all, batch...)
			if n, err := strconv.Atoi(res.Header.Get("X-WP-TotalPages")); err == nil {
				pages = n
			}
		}
		return all, nil
	}
	posts, err := get("posts")
	if err != nil {
		return nil, err
	}
	pages, err := get("pages")
	if err != nil {
		return nil, err
	}
	// A page's parents become its folder: /about/team/ stays /about/team/.
	byID := map[int]wpItem{}
	for _, p := range pages {
		byID[p.ID] = p
	}
	dirOf := func(p wpItem) string {
		var parts []string
		for seen := 0; p.Parent != 0 && seen < 20; seen++ {
			parent, ok := byID[p.Parent]
			if !ok {
				break
			}
			parts = append([]string{parent.Slug}, parts...)
			p = parent
		}
		return path.Join(parts...)
	}
	parents := map[int]bool{}
	for _, p := range pages {
		parents[p.Parent] = true
	}
	var items []Item
	for _, p := range posts {
		items = append(items, wpToItem(p, "post", o.Posts))
	}
	for _, p := range pages {
		it := wpToItem(p, "page", dirOf(p))
		it.Index = parents[p.ID]
		items = append(items, it)
	}
	return items, nil
}

func wpTime(s string) time.Time {
	t, _ := time.Parse("2006-01-02T15:04:05", s)
	return t
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

// plain turns a rendered title or excerpt into text.
func plain(s string) string {
	s = html.UnescapeString(tagRe.ReplaceAllString(s, ""))
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "[…]"))
	s = strings.TrimSuffix(s, "[&hellip;]")
	return strings.Join(strings.Fields(s), " ")
}

func wpToItem(p wpItem, kind, dir string) Item {
	it := Item{
		Kind: kind, Title: plain(p.Title.Rendered), Slug: p.Slug, Date: wpTime(p.DateGMT), Updated: wpTime(p.ModGMT),
		Draft: p.Status != "publish", Summary: plain(p.Excerpt.Rendered), HTML: p.Content.Rendered, OldURL: p.Link, Dir: dir,
	}
	if kind == "page" {
		it.Summary = "" // a page's excerpt is usually its first paragraph again
	}
	if len(p.Embedded.Author) > 0 {
		it.Author = p.Embedded.Author[0].Name
	}
	if len(p.Embedded.Media) > 0 {
		it.Image, it.ImageAlt = p.Embedded.Media[0].SourceURL, p.Embedded.Media[0].AltText
	}
	for _, group := range p.Embedded.Terms {
		for _, t := range group {
			if (t.Taxonomy == "post_tag" || t.Taxonomy == "category") && t.Name != "Uncategorized" {
				it.Tags = append(it.Tags, html.UnescapeString(t.Name))
			}
		}
	}
	if p.Status == "private" {
		it.Notes = append(it.Notes, "private in WordPress: imported as a draft")
	}
	return it
}

// The export file (WXR): RSS with WordPress's own elements.
type wxr struct {
	Channel struct {
		Link  string    `xml:"link"`
		Items []wxrItem `xml:"item"`
	} `xml:"channel"`
}

type wxrItem struct {
	Title      string `xml:"title"`
	Link       string `xml:"link"`
	Creator    string `xml:"http://purl.org/dc/elements/1.1/ creator"`
	Content    string `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
	Excerpt    string `xml:"http://wordpress.org/export/1.2/excerpt/ encoded"`
	ID         int    `xml:"http://wordpress.org/export/1.2/ post_id"`
	DateGMT    string `xml:"http://wordpress.org/export/1.2/ post_date_gmt"`
	Name       string `xml:"http://wordpress.org/export/1.2/ post_name"`
	Status     string `xml:"http://wordpress.org/export/1.2/ status"`
	Parent     int    `xml:"http://wordpress.org/export/1.2/ post_parent"`
	Type       string `xml:"http://wordpress.org/export/1.2/ post_type"`
	Attachment string `xml:"http://wordpress.org/export/1.2/ attachment_url"`
	Categories []struct {
		Domain string `xml:"domain,attr"`
		Name   string `xml:",chardata"`
	} `xml:"category"`
	Meta []struct {
		Key   string `xml:"http://wordpress.org/export/1.2/ meta_key"`
		Value string `xml:"http://wordpress.org/export/1.2/ meta_value"`
	} `xml:"http://wordpress.org/export/1.2/ postmeta"`
}

var shortcode = regexp.MustCompile(`\[(/?)([a-z_][a-z0-9_-]*)([^\]]*)\]`)

// FromWXR reads a WordPress export file.
func FromWXR(r io.Reader, posts string) ([]Item, string, error) {
	if posts == "" {
		posts = "articles"
	}
	var doc wxr
	dec := xml.NewDecoder(r)
	dec.Strict = false
	if err := dec.Decode(&doc); err != nil {
		return nil, "", fmt.Errorf("not a WordPress export file: %w", err)
	}
	attach := map[string]struct{ url, alt string }{}
	byID := map[int]wxrItem{}
	for _, it := range doc.Channel.Items {
		byID[it.ID] = it
		if it.Type == "attachment" {
			alt := ""
			for _, m := range it.Meta {
				if m.Key == "_wp_attachment_image_alt" {
					alt = m.Value
				}
			}
			attach[strconv.Itoa(it.ID)] = struct{ url, alt string }{it.Attachment, alt}
		}
	}
	dirOf := func(it wxrItem) string {
		var parts []string
		for seen := 0; it.Parent != 0 && seen < 20; seen++ {
			parent, ok := byID[it.Parent]
			if !ok || parent.Type != "page" {
				break
			}
			parts = append([]string{parent.Name}, parts...)
			it = parent
		}
		return path.Join(parts...)
	}
	parents := map[int]bool{}
	for _, w := range doc.Channel.Items {
		if w.Type == "page" {
			parents[w.Parent] = true
		}
	}
	var items []Item
	for _, w := range doc.Channel.Items {
		if w.Type != "post" && w.Type != "page" {
			continue
		}
		if w.Status == "trash" || w.Status == "auto-draft" || w.Status == "inherit" {
			continue
		}
		body, notes := wxrBody(w.Content)
		it := Item{
			Kind: w.Type, Title: w.Title, Slug: w.Name, Date: wpTimeSpace(w.DateGMT), Draft: w.Status != "publish",
			Summary: plain(w.Excerpt), HTML: body, OldURL: w.Link, Author: w.Creator, Notes: notes,
		}
		if w.Type == "post" {
			it.Dir = posts
		} else {
			it.Dir = dirOf(w)
			it.Index = parents[w.ID]
		}
		for _, c := range w.Categories {
			if (c.Domain == "post_tag" || c.Domain == "category") && c.Name != "Uncategorized" {
				it.Tags = append(it.Tags, strings.TrimSpace(c.Name))
			}
		}
		for _, m := range w.Meta {
			if m.Key == "_thumbnail_id" {
				if a, ok := attach[m.Value]; ok {
					it.Image, it.ImageAlt = a.url, a.alt
				}
			}
		}
		if it.Slug == "" {
			it.Slug = slugify(it.Title)
		}
		items = append(items, it)
	}
	return items, doc.Channel.Link, nil
}

func wpTimeSpace(s string) time.Time {
	t, _ := time.Parse("2006-01-02 15:04:05", s)
	return t
}

// wxrBody turns post_content (as stored, not rendered) into HTML: the
// classic editor's blank-line paragraphs become <p>, [caption] becomes a
// figure, and any other shortcode is noted, since only WordPress could run it.
func wxrBody(raw string) (string, []string) {
	var notes []string
	seen := map[string]bool{}
	body := regexp.MustCompile(`(?s)\[caption[^\]]*\](.*?)\[/caption\]`).ReplaceAllStringFunc(raw, func(m string) string {
		inner := regexp.MustCompile(`(?s)\[caption[^\]]*\](.*?)\[/caption\]`).FindStringSubmatch(m)[1]
		img := regexp.MustCompile(`<img[^>]*>`).FindString(inner)
		cap := strings.TrimSpace(tagRe.ReplaceAllString(strings.Replace(inner, img, "", 1), ""))
		return "<figure>" + img + "<figcaption>" + cap + "</figcaption></figure>"
	})
	for _, m := range shortcode.FindAllStringSubmatch(body, -1) {
		if m[1] == "" && !seen[m[2]] && m[2] != "caption" {
			seen[m[2]] = true
			notes = append(notes, "the ["+m[2]+"] shortcode, which only WordPress could run; it's left as text")
		}
	}
	if !strings.Contains(body, "<!-- wp:") && !regexp.MustCompile(`(?i)<p[\s>]`).MatchString(body) {
		var out []string
		for _, para := range regexp.MustCompile(`\n\s*\n`).Split(strings.TrimSpace(body), -1) {
			if para = strings.TrimSpace(para); para == "" {
				continue
			}
			if regexp.MustCompile(`(?i)^<(h[1-6]|ul|ol|blockquote|pre|figure|table|div|hr)`).MatchString(para) {
				out = append(out, para)
			} else {
				out = append(out, "<p>"+strings.ReplaceAll(para, "\n", "<br>")+"</p>")
			}
		}
		body = strings.Join(out, "\n")
	}
	return body, notes
}
