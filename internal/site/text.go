package site

import (
	"regexp"
	"strings"
)

var (
	plainFenceRe = regexp.MustCompile("(?s)```.*?```")
	plainImageRe = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	plainLinkRe  = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	plainTagRe   = regexp.MustCompile(`<[^>]+>`)
	plainMarkRe  = regexp.MustCompile("[#>*_`~]+")
	plainSpaceRe = regexp.MustCompile(`\s+`)
	wordRe       = regexp.MustCompile(`\p{L}[\p{L}'\-]*`)
)

// PlainMarkdown strips Markdown to bare words: no code blocks, images, link
// targets, HTML tags or emphasis marks.
func PlainMarkdown(md string) string {
	s := plainFenceRe.ReplaceAllString(md, " ")
	s = plainImageRe.ReplaceAllString(s, " ")
	s = plainLinkRe.ReplaceAllString(s, "$1")
	s = plainTagRe.ReplaceAllString(s, " ")
	s = plainMarkRe.ReplaceAllString(s, "")
	return strings.TrimSpace(plainSpaceRe.ReplaceAllString(s, " "))
}

// CountWords counts words the way a reader would: runs of letters, with
// apostrophes and hyphens allowed inside a word.
func CountWords(s string) int { return len(wordRe.FindAllStringIndex(s, -1)) }

// Excerpt is the plain text of some Markdown, cut at a word boundary to at
// most limit characters, with an ellipsis if anything was cut.
func Excerpt(limit int, md string) string {
	s := PlainMarkdown(md)
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	cut := string(r[:limit])
	if i := strings.LastIndex(cut, " "); i > 0 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,;:.") + "…"
}
