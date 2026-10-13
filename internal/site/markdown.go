package site

import (
	"bytes"
	"fmt"
	"html/template"
	"net/url"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Markdown renders page bodies. Raw HTML is dropped unless the site turns it
// on, and even then it passes a sanitizer, so a page can carry a <figure> or a
// <details> but not a <script> or an onclick.
type Markdown struct {
	trust    bool
	md       goldmark.Markdown
	sanitize *bluemonday.Policy
	emailOff bool
	newTab   bool
	host     string // this site's host, for telling external links apart

	media      map[string]*MediaPicture // static/media/, by main file address
	imageSizes string
}

// NewMarkdown builds the renderer for a site's settings. host is the site's
// own host name, so links elsewhere can be told apart.
func NewMarkdown(c MarkdownConfig, host string) *Markdown {
	exts := []goldmark.Extender{extension.Table, extension.Strikethrough, extension.TaskList, extension.Footnote,
		extension.DefinitionList, docsSyntax{anyAttribute: c.TrustHTML}}
	if c.AutolinkOn() {
		exts = append(exts, extension.Linkify)
	}
	rendererOpts := []goldmark.Option{
		goldmark.WithExtensions(exts...),
		goldmark.WithParserOptions(parser.WithAutoHeadingID(), parser.WithAttribute()),
	}
	if !c.TrustHTML {
		// {#id .class} after a heading is handy; {style=…} or {onclick=…}
		// would be raw HTML by another name.
		rendererOpts = append(rendererOpts, goldmark.WithParserOptions(parser.WithASTTransformers(util.Prioritized(idClassOnly{}, 1000))))
	}
	m := &Markdown{trust: c.TrustHTML, emailOff: c.EmailOff, newTab: c.ExternalLinks == "new_tab", host: strings.ToLower(host), imageSizes: c.ImageSizes}
	if m.imageSizes == "" {
		m.imageSizes = defaultImageSizes
	}
	if c.TrustHTML {
		rendererOpts = append(rendererOpts, goldmark.WithRendererOptions(html.WithUnsafe()))
	} else if c.UnsafeHTML {
		rendererOpts = append(rendererOpts, goldmark.WithRendererOptions(html.WithUnsafe()))
		p := bluemonday.UGCPolicy()
		p.AllowAttrs("class", "id").Globally()
		p.AllowElements("figure", "figcaption", "details", "summary", "picture", "source", "video", "audio")
		p.AllowAttrs("srcset", "sizes", "type", "media").OnElements("source", "img")
		p.AllowAttrs("controls", "poster", "src", "preload", "muted", "loop", "playsinline").OnElements("video", "audio")
		p.AllowAttrs("loading", "decoding", "width", "height").OnElements("img")
		// Collapsible blocks and content tabs (docsyntax.go): the tabs are
		// radio buttons and labels, so those are allowed, and nothing else
		// about them.
		p.AllowAttrs("open").OnElements("details")
		p.AllowElements("kbd", "label")
		p.AllowAttrs("for").OnElements("label")
		p.AllowAttrs("data-tabs").OnElements("div")
		p.AllowAttrs("type").Matching(regexp.MustCompile(`^radio$`)).OnElements("input")
		p.AllowAttrs("name", "checked").OnElements("input")
		m.sanitize = p
	}
	m.md = goldmark.New(rendererOpts...)
	return m
}

// idClassOnly drops every Markdown attribute but id and class.
type idClassOnly struct{}

func (idClassOnly) Transform(doc *ast.Document, _ text.Reader, _ parser.Context) {
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		attrs := n.Attributes()
		if len(attrs) == 0 {
			return ast.WalkContinue, nil
		}
		var keep []ast.Attribute
		for _, a := range attrs {
			if k := string(a.Name); k == "id" || k == "class" {
				keep = append(keep, a)
			}
		}
		n.RemoveAttributes()
		for _, a := range keep {
			n.SetAttribute(a.Name, a.Value)
		}
		return ast.WalkContinue, nil
	})
}

// protectAddresses puts Cloudflare's email_off markers around everything its
// Email Address Obfuscation would rewrite: mailto links, and email addresses
// and fediverse handles (@user@host, which look like addresses to it) in text.
// Without them a reader sees "[email protected]" where the address was.
func protectAddresses(h []byte) []byte {
	h = mailtoRe.ReplaceAll(h, []byte("<!--email_off-->$0<!--/email_off-->"))
	var b bytes.Buffer
	off := false
	for _, seg := range tagSplitRe.FindAllIndex(h, -1) {
		part := h[seg[0]:seg[1]]
		switch {
		case bytes.Equal(part, []byte("<!--email_off-->")):
			off = true
			b.Write(part)
		case bytes.Equal(part, []byte("<!--/email_off-->")):
			off = false
			b.Write(part)
		case part[0] == '<' || off:
			b.Write(part)
		default:
			b.Write(addressRe.ReplaceAll(part, []byte("<!--email_off-->$0<!--/email_off-->")))
		}
	}
	return b.Bytes()
}

var (
	tagSplitRe = regexp.MustCompile(`<!--.*?-->|<[^>]*>|[^<]+`)
	addressRe  = regexp.MustCompile(`@?[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}`)
	mailtoRe   = regexp.MustCompile(`<a href="mailto:[^"]*"[^>]*>.*?</a>`)
	anchorRe   = regexp.MustCompile(`<a\s[^>]*?href="([^"]+)"[^>]*>`)
)

// External reports whether a link leads off this site: an absolute http(s)
// URL to another host. www. and the bare host count as the same site.
func (m *Markdown) External(href string) bool {
	u, err := url.Parse(href)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false
	}
	h := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	return h != strings.TrimPrefix(m.host, "www.")
}

// Inline renders a short piece of Markdown, such as a subtitle with a code
// span in it, without the paragraph around it.
func (m *Markdown) Inline(src []byte) (template.HTML, error) {
	h, err := m.Render(src)
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(string(h))
	if strings.HasPrefix(s, "<p>") && strings.HasSuffix(s, "</p>") && strings.Count(s, "<p>") == 1 {
		s = s[3 : len(s)-4]
	}
	return template.HTML(s), nil
}

// HTML takes a body written as HTML rather than Markdown (front matter
// format: html), applying the same rule raw HTML in Markdown gets: passed
// through when the site trusts its authors, sanitized when it allows raw HTML,
// refused otherwise.
func (m *Markdown) HTML(src []byte) (template.HTML, error) {
	switch {
	case m.trust:
	case m.sanitize != nil:
		src = m.sanitize.SanitizeBytes(src)
	default:
		return "", fmt.Errorf("format: html needs markdown.trust_html or markdown.unsafe_html in site.yaml")
	}
	out := m.pictures(src)
	if m.newTab {
		out = m.markNewTab(out)
	}
	return template.HTML(out), nil
}

func (m *Markdown) markNewTab(out []byte) []byte {
	return anchorRe.ReplaceAllFunc(out, func(a []byte) []byte {
		sub := anchorRe.FindSubmatch(a)
		if bytes.Contains(bytes.ToLower(a), []byte("target=")) || !m.External(string(sub[1])) {
			return a
		}
		return append(a[:len(a)-1:len(a)-1], []byte(` target="_blank" rel="noopener noreferrer">`)...)
	})
}

// Render turns Markdown into HTML that is safe to place in a page.
func (m *Markdown) Render(src []byte) (template.HTML, error) {
	if m.trust || m.sanitize != nil {
		src = mdInHTML(src)
	}
	var buf bytes.Buffer
	if err := m.md.Convert(src, &buf); err != nil {
		return "", err
	}
	out := buf.Bytes()
	if m.sanitize != nil {
		out = m.sanitize.SanitizeBytes(out)
	}
	// After the sanitizer: the picture markup is ours, not the author's.
	out = m.pictures(out)
	if m.emailOff {
		out = protectAddresses(out)
	}
	if m.newTab {
		out = m.markNewTab(out)
	}
	// Safe by construction: goldmark escapes text and, without WithUnsafe,
	// omits raw HTML; with it, the sanitizer above has had the last word.
	return template.HTML(out), nil
}
