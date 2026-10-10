package site

import (
	"bytes"
	"html/template"
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
	md       goldmark.Markdown
	sanitize *bluemonday.Policy
	emailOff bool
}

// NewMarkdown builds the renderer for a site's settings.
func NewMarkdown(c MarkdownConfig) *Markdown {
	rendererOpts := []goldmark.Option{
		goldmark.WithExtensions(extension.GFM, extension.Footnote),
		goldmark.WithParserOptions(parser.WithAutoHeadingID(), parser.WithAttribute()),
	}
	m := &Markdown{emailOff: c.EmailOff}
	if c.UnsafeHTML {
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

var mailtoRe = regexp.MustCompile(`<a href="mailto:[^"]*"[^>]*>.*?</a>`)

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
		out = mailtoRe.ReplaceAll(out, []byte("<!--email_off-->$0<!--/email_off-->"))
	}
	// Safe by construction: goldmark escapes text and, without WithUnsafe,
	// omits raw HTML; with it, the sanitizer above has had the last word.
	return template.HTML(out), nil
}
