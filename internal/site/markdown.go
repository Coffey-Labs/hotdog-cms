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
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
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
}

// NewMarkdown builds the renderer for a site's settings. host is the site's
// own host name, so links elsewhere can be told apart.
func NewMarkdown(c MarkdownConfig, host string) *Markdown {
	exts := []goldmark.Extender{extension.Table, extension.Strikethrough, extension.TaskList, extension.Footnote}
	if c.AutolinkOn() {
		exts = append(exts, extension.Linkify)
	}
	rendererOpts := []goldmark.Option{
		goldmark.WithExtensions(exts...),
		goldmark.WithParserOptions(parser.WithAutoHeadingID(), parser.WithAttribute()),
	}
	m := &Markdown{trust: c.TrustHTML, emailOff: c.EmailOff, newTab: c.ExternalLinks == "new_tab", host: strings.ToLower(host)}
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
		m.sanitize = p
	}
	m.md = goldmark.New(rendererOpts...)
	return m
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
	out := src
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
	var buf bytes.Buffer
	if err := m.md.Convert(src, &buf); err != nil {
		return "", err
	}
	out := buf.Bytes()
	if m.sanitize != nil {
		out = m.sanitize.SanitizeBytes(out)
	}
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
