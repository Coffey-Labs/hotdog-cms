// Package importer brings sites in from elsewhere: WordPress (a live site or
// an export file), Hugo, Jekyll and Ghost. Pages become Markdown where that's
// faithful, keep their HTML where it isn't, bring their pictures through the
// same re-encoding as uploads, and leave a redirect at every old address.
package importer

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Converted is a body turned into Markdown, and what couldn't be.
type Converted struct {
	Markdown string
	Lossy    []string // why Markdown would lose something; empty when it's faithful
	Images   []string // every image address, in order
}

// ToMarkdown converts an HTML body. When something can't be said in Markdown
// (an embed, a layout block, a table with merged cells), Lossy says what, and
// the caller keeps the HTML instead.
func ToMarkdown(src string) Converted {
	c := &conv{}
	nodes, err := html.ParseFragment(strings.NewReader(src), &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body})
	if err != nil {
		return Converted{Lossy: []string{"HTML that couldn't be read"}}
	}
	var b strings.Builder
	for _, n := range nodes {
		c.block(&b, n, "")
	}
	md := regexp.MustCompile(`\n{3,}`).ReplaceAllString(strings.TrimSpace(b.String()), "\n\n")
	return Converted{Markdown: md + "\n", Lossy: dedupe(c.lossy), Images: c.images}
}

type conv struct {
	lossy  []string
	images []string
}

// captionWidth reports a WordPress caption's own width (style="width: 1600px"
// on a wp-caption figure or div): WordPress sizes the box to its picture, and
// the Markdown figure doesn't need it.
func captionWidth(n *html.Node, style string) bool {
	return (n.Data == "figure" || n.Data == "div") && strings.Contains(attr(n, "class"), "wp-caption") &&
		captionWidthRe.MatchString(style)
}

var captionWidthRe = regexp.MustCompile(`^\s*(max-)?width:\s*\d+px;?\s*$`)

func dedupe(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

func attr(n *html.Node, k string) string {
	for _, a := range n.Attr {
		if a.Key == k {
			return a.Val
		}
	}
	return ""
}

// Elements that are layout or behavior, never words: Markdown can't hold them.
var lossyTags = map[string]string{
	"iframe": "an embed", "video": "a video", "audio": "audio", "script": "a script", "style": "inline styles",
	"form": "a form", "input": "a form", "select": "a form", "textarea": "a form", "button": "a button",
	"object": "an embed", "embed": "an embed", "svg": "a drawing (SVG)", "canvas": "a canvas", "map": "an image map",
	"details": "a collapsible section", "sup": "superscript", "sub": "subscript", "u": "underlining", "mark": "highlighting",
}

// WordPress block classes that are layout.
var layoutClass = regexp.MustCompile(`\bwp-block-(columns|column|cover|media-text|buttons|button|embed|table|pullquote|spacer|separator-dots|social-links)\b|\belementor|\bvc_|\bet_pb_|\bfl-row|\bwp-block-gallery\b.*\bcolumns-`)

func (c *conv) block(b *strings.Builder, n *html.Node, indent string) {
	switch n.Type {
	case html.TextNode:
		if t := strings.TrimSpace(n.Data); t != "" {
			b.WriteString(indent + escapeText(collapse(n.Data), true) + "\n\n")
		}
		return
	case html.CommentNode:
		return // WordPress block markers
	case html.ElementNode:
	default:
		return
	}
	if why, ok := lossyTags[n.Data]; ok {
		c.lossy = append(c.lossy, why)
	}
	if cls := attr(n, "class"); layoutClass.MatchString(cls) {
		c.lossy = append(c.lossy, "a layout block ("+strings.Fields(layoutClass.FindString(cls))[0]+")")
	}
	if st := attr(n, "style"); st != "" && !captionWidth(n, st) {
		c.lossy = append(c.lossy, "inline styles")
	}
	switch n.Data {
	case "p":
		if t := strings.TrimSpace(c.inline(n)); t != "" {
			t = escapeLineStart(t)
			b.WriteString(indent + strings.ReplaceAll(t, "\n", "\n"+indent) + "\n\n")
		}
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(n.Data[1] - '0')
		b.WriteString(indent + strings.Repeat("#", level) + " " + strings.TrimSpace(strings.ReplaceAll(c.inline(n), "\n", " ")) + "\n\n")
	case "ul", "ol":
		c.list(b, n, indent)
		b.WriteString("\n")
	case "blockquote":
		var inner strings.Builder
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			c.blockOrInline(&inner, ch, "")
		}
		for _, l := range strings.Split(strings.TrimSpace(inner.String()), "\n") {
			b.WriteString(indent + strings.TrimRight("> "+l, " ") + "\n")
		}
		b.WriteString("\n")
	case "pre":
		code := textOf(n)
		lang := ""
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			if ch.Type == html.ElementNode && ch.Data == "code" {
				if m := regexp.MustCompile(`language-([\w+-]+)`).FindStringSubmatch(attr(ch, "class")); m != nil {
					lang = m[1]
				}
			}
		}
		fence := "```"
		for strings.Contains(code, fence) {
			fence += "`"
		}
		b.WriteString(indent + fence + lang + "\n" + strings.TrimRight(code, "\n") + "\n" + indent + fence + "\n\n")
	case "hr":
		b.WriteString(indent + "---\n\n")
	case "figure":
		var caption string
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			if ch.Type == html.ElementNode && ch.Data == "figcaption" {
				caption = strings.TrimSpace(c.inline(ch))
				continue
			}
			c.blockOrInline(b, ch, indent)
		}
		if caption != "" {
			b.WriteString(indent + "*" + caption + "*\n\n")
		}
	case "table":
		c.table(b, n, indent)
	case "img":
		b.WriteString(indent + c.image(n) + "\n\n")
	case "br":
	default:
		// div, section, article, span and the like: their content, in place.
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			c.blockOrInline(b, ch, indent)
		}
	}
}

// blockOrInline handles a child that may be a block or loose inline content.
func (c *conv) blockOrInline(b *strings.Builder, n *html.Node, indent string) {
	if n.Type == html.ElementNode && isInline(n.Data) {
		if t := strings.TrimSpace(c.inline(&html.Node{Type: html.ElementNode, Data: "span", FirstChild: n, LastChild: n})); t != "" {
			b.WriteString(indent + t + "\n\n")
		}
		return
	}
	c.block(b, n, indent)
}

func isInline(tag string) bool {
	switch tag {
	case "a", "strong", "b", "em", "i", "code", "span", "del", "s", "strike", "abbr", "cite", "q", "small", "time", "kbd", "var", "samp", "sup", "sub", "u", "mark", "br":
		return true
	}
	return false
}

func (c *conv) list(b *strings.Builder, n *html.Node, indent string) {
	i := 1
	if s := attr(n, "start"); s != "" {
		fmt.Sscanf(s, "%d", &i)
	}
	for li := n.FirstChild; li != nil; li = li.NextSibling {
		if li.Type != html.ElementNode || li.Data != "li" {
			continue
		}
		marker := "- "
		if n.Data == "ol" {
			marker = fmt.Sprintf("%d. ", i)
			i++
		}
		pad := indent + strings.Repeat(" ", len(marker))
		var text strings.Builder
		var nested strings.Builder
		for ch := li.FirstChild; ch != nil; ch = ch.NextSibling {
			if ch.Type == html.ElementNode && (ch.Data == "ul" || ch.Data == "ol") {
				c.list(&nested, ch, pad)
				continue
			}
			if ch.Type == html.ElementNode && (ch.Data == "p" || ch.Data == "div") {
				text.WriteString(" " + c.inline(ch))
				continue
			}
			text.WriteString(c.inlineNode(ch))
		}
		line := strings.TrimSpace(collapseKeepBreaks(text.String()))
		b.WriteString(indent + marker + strings.ReplaceAll(line, "\n", "\n"+pad) + "\n")
		b.WriteString(nested.String())
	}
}

func (c *conv) table(b *strings.Builder, n *html.Node, indent string) {
	var rows [][]string
	head := -1
	var walk func(*html.Node)
	simple := true
	walk = func(x *html.Node) {
		for ch := x.FirstChild; ch != nil; ch = ch.NextSibling {
			if ch.Type != html.ElementNode {
				continue
			}
			switch ch.Data {
			case "tr":
				var cells []string
				for td := ch.FirstChild; td != nil; td = td.NextSibling {
					if td.Type != html.ElementNode || (td.Data != "td" && td.Data != "th") {
						continue
					}
					if attr(td, "colspan") != "" || attr(td, "rowspan") != "" {
						simple = false
					}
					for k := td.FirstChild; k != nil; k = k.NextSibling {
						if k.Type == html.ElementNode && !isInline(k.Data) && k.Data != "p" {
							simple = false
						}
					}
					if td.Data == "th" && head < 0 {
						head = len(rows)
					}
					cells = append(cells, strings.ReplaceAll(strings.TrimSpace(strings.ReplaceAll(c.inline(td), "\n", " ")), "|", `\|`))
				}
				rows = append(rows, cells)
			default:
				walk(ch)
			}
		}
	}
	walk(n)
	if !simple || len(rows) == 0 {
		c.lossy = append(c.lossy, "a table with merged or complex cells")
		return
	}
	width := 0
	for _, r := range rows {
		width = max(width, len(r))
	}
	if head != 0 { // GitHub tables need a header row: an empty one
		rows = append([][]string{make([]string, width)}, rows...)
	}
	for i, r := range rows {
		for len(r) < width {
			r = append(r, "")
		}
		b.WriteString(indent + "| " + strings.Join(r, " | ") + " |\n")
		if i == 0 {
			b.WriteString(indent + "|" + strings.Repeat(" --- |", width) + "\n")
		}
	}
	b.WriteString("\n")
}

func (c *conv) image(n *html.Node) string {
	src := attr(n, "src")
	if src == "" {
		if s := attr(n, "data-src"); s != "" {
			src = s
		}
	}
	c.images = append(c.images, src)
	alt := strings.ReplaceAll(strings.ReplaceAll(attr(n, "alt"), "[", ""), "]", "")
	title := attr(n, "title")
	if title != "" {
		return fmt.Sprintf(`![%s](%s "%s")`, alt, src, strings.ReplaceAll(title, `"`, "'"))
	}
	return fmt.Sprintf("![%s](%s)", alt, src)
}

func (c *conv) inline(n *html.Node) string {
	var b strings.Builder
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		b.WriteString(c.inlineNode(ch))
	}
	return collapseKeepBreaks(b.String())
}

func (c *conv) inlineNode(n *html.Node) string {
	switch n.Type {
	case html.TextNode:
		return escapeText(n.Data, false)
	case html.ElementNode:
	default:
		return ""
	}
	if why, ok := lossyTags[n.Data]; ok {
		c.lossy = append(c.lossy, why)
	}
	inner := func() string { return c.inline(n) }
	wrap := func(mark string) string {
		t := inner()
		lead, trail := "", ""
		if strings.HasPrefix(t, " ") {
			lead = " "
		}
		if strings.HasSuffix(t, " ") {
			trail = " "
		}
		t = strings.TrimSpace(t)
		if t == "" {
			return lead + trail
		}
		return lead + mark + t + mark + trail
	}
	switch n.Data {
	case "strong", "b":
		return wrap("**")
	case "em", "i", "cite":
		return wrap("*")
	case "del", "s", "strike":
		return wrap("~~")
	case "code", "kbd", "samp":
		t := textOf(n)
		tick := "`"
		for strings.Contains(t, tick) {
			tick += "`"
		}
		if strings.HasPrefix(t, "`") || strings.HasSuffix(t, "`") {
			t = " " + t + " "
		}
		return tick + t + tick
	case "a":
		href := attr(n, "href")
		t := strings.TrimSpace(inner())
		if href == "" {
			return t
		}
		if t == "" {
			return ""
		}
		if title := attr(n, "title"); title != "" && title != t {
			return fmt.Sprintf(`[%s](%s "%s")`, t, href, strings.ReplaceAll(title, `"`, "'"))
		}
		return "[" + t + "](" + href + ")"
	case "img":
		return c.image(n)
	case "br":
		return "\\\n"
	case "p", "div":
		return " " + inner() + " "
	default:
		return inner()
	}
}

func textOf(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		if x.Type == html.TextNode {
			b.WriteString(x.Data)
		}
		for ch := x.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(n)
	return b.String()
}

var spaces = regexp.MustCompile(`[ \t\r\n]+`)

func collapse(s string) string { return spaces.ReplaceAllString(s, " ") }

// collapseKeepBreaks collapses whitespace but keeps the hard breaks (\ then
// a newline) inline conversion produced.
func collapseKeepBreaks(s string) string {
	parts := strings.Split(s, "\\\n")
	for i, p := range parts {
		parts[i] = collapse(p)
		if i > 0 {
			parts[i] = strings.TrimLeft(parts[i], " ")
		}
	}
	return strings.Join(parts, "\\\n")
}

var (
	mdSpecial = strings.NewReplacer(`\`, `\\`, "`", "\\`", "*", `\*`, "[", `\[`, "]", `\]`, "<", `\<`)
	// An underscore inside a word (snake_case) can't start emphasis; only
	// one at a word's edge needs escaping.
	edgeUnderscore = regexp.MustCompile(`(^|[^\pL\pN])_|_([^\pL\pN]|$)`)
	lineStart      = regexp.MustCompile(`^(\s*)([#>+-]|\d+\.)(\s)`)
)

// escapeText keeps text from being read as Markdown.
func escapeText(s string, block bool) string {
	s = mdSpecial.Replace(s)
	s = edgeUnderscore.ReplaceAllStringFunc(s, func(m string) string { return strings.ReplaceAll(m, "_", `\_`) })
	if block {
		s = escapeLineStart(s)
	}
	return s
}

// escapeLineStart keeps a line from opening a heading, quote or list: a
// backslash before the marker, or before the dot of "1." (a digit itself
// can't be escaped).
func escapeLineStart(s string) string {
	return lineStart.ReplaceAllStringFunc(s, func(m string) string {
		sub := lineStart.FindStringSubmatch(m)
		if strings.HasSuffix(sub[2], ".") {
			return sub[1] + strings.TrimSuffix(sub[2], ".") + `\.` + sub[3]
		}
		return sub[1] + `\` + sub[2] + sub[3]
	})
}
