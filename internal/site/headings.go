package site

import (
	"html"
	"html/template"
	"regexp"
	"strings"
)

// Heading is a section heading in a page's body, for an "on this page" list
// or a search result that goes to the section.
type Heading struct {
	Level int    // 2 or 3
	ID    string // the anchor, as the page has it
	Text  string // plain text
}

var (
	headingRe = regexp.MustCompile(`(?s)<h([23])\s[^>]*?\bid="([^"]+)"[^>]*>(.*?)</h[23]>`)
	anyTagRe  = regexp.MustCompile(`<[^>]*>`)
)

// headings lists a rendered body's h2 and h3 headings that have an anchor,
// in order. It reads the finished HTML, so the anchors are the ones the page
// ends up with, after any sanitizing.
func headings(body template.HTML) []Heading {
	var out []Heading
	for _, m := range headingRe.FindAllStringSubmatch(string(body), -1) {
		text := strings.Join(strings.Fields(html.UnescapeString(anyTagRe.ReplaceAllString(m[3], ""))), " ")
		if text == "" {
			continue
		}
		out = append(out, Heading{Level: int(m[1][0] - '0'), ID: html.UnescapeString(m[2]), Text: text})
	}
	return out
}
