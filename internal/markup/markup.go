// Package markup checks HTML and SVG that comes from people other than the
// site's maintainers (packs, icons) for anything that would run code in a
// visitor's browser or send them elsewhere unexpectedly.
package markup

import (
	"fmt"
	"io"
	"strings"
	"text/template/parse"

	"golang.org/x/net/html"
)

// A pack's templates are checked as the HTML they'll produce, not with a
// pattern: the template is parsed, every action ({{ … }}) becomes a
// placeholder, and the result is read by an HTML tokenizer. So a script
// split by a template comment (<scri{{/**/}}pt>), a handler built from an
// action (on{{.x}}click=), an entity-encoded javascript: link or an
// attribute without spaces (<img src=x onerror=…>) all show up for what
// they are.

const placeholder = "hdpackx"

// forbidden elements: they run code, load other pages or documents, or
// change where links and forms go.
var forbidden = map[string]bool{
	"script": true, "iframe": true, "frame": true, "frameset": true, "object": true, "embed": true, "applet": true,
	"base": true, "meta": true, "portal": true, "noscript": true,
}

// urlAttrs hold addresses; their values must be web, mail, phone or relative links.
var urlAttrs = map[string]bool{
	"href": true, "src": true, "action": true, "formaction": true, "poster": true, "data": true, "background": true,
	"cite": true, "longdesc": true, "usemap": true, "xlink:href": true, "ping": true, "manifest": true, "codebase": true,
}

// Template lists what in a Go template would run code in a visitor's
// browser or send them elsewhere unexpectedly.
func Template(src string) ([]string, error) {
	trees, err := parse.Parse("pack", src, "", "", map[string]any{})
	if err != nil {
		// Unknown functions are expected (asset, markdownify, …): parse
		// again without checking them.
		t := parse.New("pack")
		t.Mode = parse.SkipFuncCheck
		trees = map[string]*parse.Tree{}
		if _, err := t.Parse(src, "", "", trees); err != nil {
			return nil, fmt.Errorf("the template doesn't parse: %w", err)
		}
	}
	var b strings.Builder
	for _, tr := range trees {
		if tr.Root != nil {
			flatten(&b, tr.Root)
		}
	}
	return scan(b.String(), false), nil
}

// SVG lists the same for an SVG file, which may also not reach outside
// itself (href only to "#id" in the same file) or embed HTML or styles.
func SVG(src string) []string {
	return scan(src, true)
}

// flatten writes a template's text with every action as the placeholder.
// Control structures contribute every branch, so whatever the template
// could produce is checked.
func flatten(b *strings.Builder, n parse.Node) {
	switch n := n.(type) {
	case *parse.ListNode:
		if n == nil {
			return
		}
		for _, c := range n.Nodes {
			flatten(b, c)
		}
	case *parse.TextNode:
		b.Write(n.Text)
	case *parse.ActionNode, *parse.TemplateNode:
		b.WriteString(placeholder)
	case *parse.IfNode:
		flatten(b, n.List)
		flatten(b, n.ElseList)
	case *parse.RangeNode:
		flatten(b, n.List)
		flatten(b, n.ElseList)
	case *parse.WithNode:
		flatten(b, n.List)
		flatten(b, n.ElseList)
	}
}

func scan(markup string, svg bool) []string {
	var problems []string
	z := html.NewTokenizer(strings.NewReader(markup))
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			if z.Err() != io.EOF {
				problems = append(problems, "the markup can't be read")
			}
			return problems
		}
		if tt != html.StartTagToken && tt != html.SelfClosingTagToken {
			continue
		}
		tok := z.Token()
		name := strings.ToLower(tok.Data)
		if strings.Contains(name, placeholder) {
			problems = append(problems, "an element whose name comes from a template action")
			continue
		}
		if forbidden[name] || (svg && (name == "foreignobject" || name == "style")) {
			problems = append(problems, fmt.Sprintf("a <%s> element", name))
		}
		if name == "link" && !ownStylesheet(tok.Attr) {
			problems = append(problems, "a <link> other than a stylesheet on the site ({{ asset \"packs/<name>/x.css\" }})")
		}
		for _, a := range tok.Attr {
			key := strings.ToLower(a.Key)
			switch {
			case strings.Contains(key, placeholder):
				problems = append(problems, fmt.Sprintf("an attribute on <%s> whose name comes from a template action", name))
			case strings.HasPrefix(key, "on"):
				problems = append(problems, fmt.Sprintf("an event handler (%s) on <%s>", key, name))
			case key == "srcdoc" || key == "http-equiv":
				problems = append(problems, fmt.Sprintf("%s on <%s>", key, name))
			case svg && (key == "href" || key == "xlink:href" || key == "src"):
				if v := strings.TrimSpace(a.Val); !strings.HasPrefix(v, "#") {
					problems = append(problems, fmt.Sprintf("a %s on <%s> that reaches outside the file (%q)", key, name, a.Val))
				}
			case urlAttrs[key] || key == "srcset":
				if !safeURL(a.Val) {
					problems = append(problems, fmt.Sprintf("a %s on <%s> that isn't a web, mail, phone or relative link (%q)", key, name, a.Val))
				}
			}
		}
	}
}

// safeURL accepts relative links, web, mail and phone links, and values that are a
// single template action (which html/template itself filters). An action
// after a fixed start ("java{{.x}}script:") is refused, since the template
// escaper can't see the whole scheme.
func safeURL(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" || v == placeholder {
		return true
	}
	lower := strings.ToLower(v)
	for _, p := range []string{"https://", "http://", "mailto:", "tel:", "/", "#", "?", "./", "../"} {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	if strings.HasPrefix(v, placeholder) {
		// "{{ asset "x" }}" or "{{ .Href }}" first: html/template filters the
		// scheme of what the action produces.
		return true
	}
	// No scheme at all (a relative path like "img/x.png") is fine; anything
	// with a colon before the first slash is a scheme we don't allow.
	colon := strings.IndexByte(v, ':')
	slash := strings.IndexAny(v, "/?#")
	return colon < 0 || (slash >= 0 && slash < colon)
}

// ownStylesheet: a <link rel="stylesheet"> to a file on the site itself,
// which is how a pack brings its styles.
func ownStylesheet(attrs []html.Attribute) bool {
	rel, href := "", ""
	for _, a := range attrs {
		switch strings.ToLower(a.Key) {
		case "rel":
			rel = strings.ToLower(strings.TrimSpace(a.Val))
		case "href":
			href = strings.TrimSpace(a.Val)
		}
	}
	return rel == "stylesheet" && (href == placeholder || (strings.HasPrefix(href, "/") && !strings.HasPrefix(href, "//")))
}
