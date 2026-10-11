package check

import (
	"bytes"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// page is what one built HTML file says about itself.
type page struct {
	file        string // public/...
	url         string // its address
	redirect    bool   // a meta-refresh page hotdog-cms wrote for a redirect
	static      bool   // copied as-is from static/ (a verification file, say), not a page
	noindex     bool
	title       string
	description string
	canonical   string
	ogImage     string
	ogImageAlt  bool

	scripts      []string // external script sources
	inlineJS     []string // executable inline scripts (their first line)
	inlineStyles int      // <style> elements and style="" attributes
	handlers     []string // on* attributes
	frames       []string
	styles       []string // external stylesheets
	images       []string // img/source/video sources
	imgNoAlt     int
	links        []string // every href and src, for the link check
	insecure     []string // http:// resources
	exposed      []string // addresses and handles outside email_off
	mailtos      []string
}

var (
	emailRe  = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}`)
	handleRe = regexp.MustCompile(`@[A-Za-z0-9_]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)+`)
)

// Script types that hold data rather than code, which no CSP restricts.
var dataScript = map[string]bool{"application/ld+json": true, "application/json": true, "text/template": true, "importmap": false}

func (c *checker) readPages() ([]*page, error) {
	var pages []*page
	err := filepath.WalkDir(c.out, func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() || !strings.HasSuffix(p, ".html") {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(c.out, p)
		rel = filepath.ToSlash(rel)
		u := "/" + strings.TrimSuffix(rel, "index.html")
		pg := parsePage(data, "public/"+rel, u)
		if _, err := os.Stat(filepath.Join(c.siteDir, "static", filepath.FromSlash(rel))); err == nil {
			pg.static = true
		}
		pages = append(pages, pg)
		return nil
	})
	return pages, err
}

func attr(t html.Token, key string) (string, bool) {
	for _, a := range t.Attr {
		if a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}

func parsePage(data []byte, file, u string) *page {
	pg := &page{file: file, url: u}
	z := html.NewTokenizer(bytes.NewReader(data))
	emailOff := false
	var inTitle, inScript bool
	var scriptIsCode bool
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return pg
		case html.CommentToken:
			switch strings.TrimSpace(string(z.Text())) {
			case "email_off":
				emailOff = true
			case "/email_off":
				emailOff = false
			}
		case html.TextToken:
			text := string(z.Text())
			switch {
			case inTitle:
				pg.title += text
			case inScript:
				if scriptIsCode && strings.TrimSpace(text) != "" {
					first := strings.TrimSpace(strings.SplitN(strings.TrimSpace(text), "\n", 2)[0])
					if len(first) > 60 {
						first = first[:60] + "…"
					}
					pg.inlineJS = append(pg.inlineJS, first)
				}
			default:
				if !emailOff {
					for _, m := range handleRe.FindAllString(text, -1) {
						pg.exposed = append(pg.exposed, m)
					}
					clean := handleRe.ReplaceAllString(text, "")
					for _, m := range emailRe.FindAllString(clean, -1) {
						pg.exposed = append(pg.exposed, m)
					}
				}
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "title":
				inTitle = false
			case "script":
				inScript = false
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			t := z.Token()
			for _, a := range t.Attr {
				if strings.HasPrefix(a.Key, "on") && len(a.Key) > 2 {
					pg.handlers = append(pg.handlers, t.Data+" "+a.Key)
				}
				if a.Key == "style" {
					pg.inlineStyles++
				}
			}
			switch t.Data {
			case "title":
				inTitle = tt == html.StartTagToken
			case "style":
				pg.inlineStyles++
			case "script":
				src, hasSrc := attr(t, "src")
				typ, _ := attr(t, "type")
				if hasSrc {
					pg.scripts = append(pg.scripts, src)
					pg.links = append(pg.links, src)
					if strings.HasPrefix(src, "http://") {
						pg.insecure = append(pg.insecure, src)
					}
				}
				inScript = tt == html.StartTagToken
				scriptIsCode = !hasSrc && !dataScript[strings.ToLower(typ)]
			case "meta":
				name, _ := attr(t, "name")
				prop, _ := attr(t, "property")
				content, _ := attr(t, "content")
				equiv, _ := attr(t, "http-equiv")
				switch {
				case name == "robots" && strings.Contains(content, "noindex"):
					pg.noindex = true
				case name == "description":
					pg.description = content
				case prop == "og:image":
					pg.ogImage = content
				case prop == "og:image:alt" && content != "":
					pg.ogImageAlt = true
				case strings.EqualFold(equiv, "refresh"):
					pg.redirect = true
				}
			case "link":
				rel, _ := attr(t, "rel")
				href, _ := attr(t, "href")
				rels := " " + strings.ToLower(rel) + " "
				switch {
				case strings.Contains(rels, " canonical "):
					pg.canonical = href
				case strings.Contains(rels, " stylesheet "):
					pg.styles = append(pg.styles, href)
					pg.links = append(pg.links, href)
				case strings.Contains(rels, " preconnect ") || strings.Contains(rels, " dns-prefetch "):
				default:
					pg.links = append(pg.links, href)
				}
				if strings.HasPrefix(href, "http://") && (strings.Contains(rels, " stylesheet ") || strings.Contains(rels, " icon ") || strings.Contains(rels, " preload ")) {
					pg.insecure = append(pg.insecure, href)
				}
			case "iframe":
				if src, ok := attr(t, "src"); ok {
					pg.frames = append(pg.frames, src)
					pg.links = append(pg.links, src)
					if strings.HasPrefix(src, "http://") {
						pg.insecure = append(pg.insecure, src)
					}
				}
			case "img", "source", "video", "audio":
				if t.Data == "img" {
					if _, ok := attr(t, "alt"); !ok {
						pg.imgNoAlt++
					}
				}
				for _, k := range []string{"src", "poster"} {
					if v, ok := attr(t, k); ok {
						pg.images = append(pg.images, v)
						pg.links = append(pg.links, v)
						if strings.HasPrefix(v, "http://") {
							pg.insecure = append(pg.insecure, v)
						}
					}
				}
				if v, ok := attr(t, "srcset"); ok {
					for _, part := range strings.Split(v, ",") {
						if f := strings.Fields(part); len(f) > 0 {
							pg.images = append(pg.images, f[0])
							pg.links = append(pg.links, f[0])
						}
					}
				}
			case "a", "area":
				if href, ok := attr(t, "href"); ok {
					pg.links = append(pg.links, href)
					if strings.HasPrefix(strings.ToLower(href), "mailto:") {
						pg.mailtos = append(pg.mailtos, href)
						if !emailOff {
							pg.exposed = append(pg.exposed, strings.TrimPrefix(href, "mailto:"))
						}
					}
				}
			}
			if t.Data == "script" && tt == html.SelfClosingTagToken {
				inScript = false
			}
		}
	}
}

// host returns a reference's host if it points off the site, else "".
func (c *checker) host(ref string) string {
	u, err := url.Parse(strings.TrimSpace(ref))
	if err != nil {
		return ""
	}
	if u.Scheme == "" && strings.HasPrefix(ref, "//") {
		u, _ = url.Parse("https:" + ref)
	}
	if u == nil || u.Host == "" || (u.Scheme != "" && u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	own, _ := url.Parse(c.cfg.URL)
	h := strings.ToLower(u.Hostname())
	if own != nil && (h == own.Hostname() || h == "www."+own.Hostname() || "www."+h == own.Hostname()) {
		return ""
	}
	return h
}

// internal resolves a link against its page and reports whether it points
// into this site.
func (c *checker) internal(ref, pageURL string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.HasPrefix(ref, "#") {
		return "", false
	}
	u, err := url.Parse(ref)
	if err != nil {
		return "", false
	}
	if u.Scheme != "" || u.Host != "" {
		if c.host(ref) != "" || (u.Scheme != "http" && u.Scheme != "https") {
			return "", false
		}
		return u.Path, true
	}
	if strings.HasPrefix(u.Path, "/") {
		return u.Path, true
	}
	return path.Join(path.Dir(pageURL+"x"), u.Path), true
}

func exists(root, urlPath string) bool {
	if urlPath == "" {
		urlPath = "/"
	}
	p := filepath.Join(root, filepath.FromSlash(path.Clean("/"+urlPath)))
	fi, err := os.Stat(p)
	if err != nil {
		return false
	}
	if fi.IsDir() {
		_, err = os.Stat(filepath.Join(p, "index.html"))
		return err == nil
	}
	return true
}

// Meta is what a page tells search engines and link previews about itself.
type Meta struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Canonical   string `json:"canonical"`
	Image       string `json:"image"`
	ImageAlt    bool   `json:"imageAlt"`
	NoIndex     bool   `json:"noindex"`
}

// PageMeta reads it from a rendered page, the way the checks do.
func PageMeta(html []byte) Meta {
	pg := parsePage(html, "", "/")
	return Meta{Title: strings.TrimSpace(pg.title), Description: pg.description, Canonical: pg.canonical, Image: pg.ogImage, ImageAlt: pg.ogImageAlt, NoIndex: pg.noindex}
}
