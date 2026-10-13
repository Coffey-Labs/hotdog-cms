package site

// Documentation syntax, written the way MkDocs writes it (Python-Markdown
// and the pymdownx extensions), so a docs site moves over without its pages
// being rewritten, and rendered with the same HTML and class names, so the
// stylesheets written for it keep working:
//
//	!!! note "Title"           admonition         <div class="admonition note">
//	??? tip "Title"            collapsible        <details class="tip">
//	???+ tip                   …open to start     <details class="tip" open>
//	=== "Linux"                content tabs       <div class="tabbed-set">
//	++ctrl+alt+del++           keys               <span class="keys"><kbd>…
//	![alt](src){ .shot }       attributes on a picture or a link
//	<div markdown>…</div>      Markdown inside an HTML block (when the site
//	                           allows HTML; see mdInHTML)
//
// The bodies of admonitions and tabs are ordinary Markdown, indented four
// spaces under their first line, and can hold anything a page can, other
// admonitions and tabs included.

import (
	"bytes"
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

var (
	kindAdmonition = ast.NewNodeKind("Admonition")
	kindAdmTitle   = ast.NewNodeKind("AdmonitionTitle")
	kindTab        = ast.NewNodeKind("Tab")
	kindTabSet     = ast.NewNodeKind("TabSet")
	kindKeys       = ast.NewNodeKind("Keys")
	kindCodeTitle  = ast.NewNodeKind("CodeTitle")
)

// docsSyntax is the goldmark extension for all of the above. anyAttribute
// is the site trusting its authors' HTML: without it, attributes written
// after a picture or a link are limited to id and class.
type docsSyntax struct {
	anyAttribute bool
	keys         map[string]string // site.yaml markdown.keys: names for keys, as pymdownx.keys' key_map
}

func (d docsSyntax) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithBlockParsers(
			util.Prioritized(admonitionParser{}, 150),
			util.Prioritized(tabParser{}, 150),
		),
		parser.WithInlineParsers(util.Prioritized(keysParser{keys: d.keys}, 150)),
		parser.WithASTTransformers(
			util.Prioritized(tabSets{}, 3000),
			util.Prioritized(trailingAttributes{anyAttribute: d.anyAttribute}, 2000),
			util.Prioritized(codeTitles{}, 2000),
		),
	)
	m.Renderer().AddOptions(renderer.WithNodeRenderers(util.Prioritized(docsRenderer{keys: d.keys}, 500)))
}

// --- admonitions and collapsible blocks -------------------------------------

type admonition struct {
	ast.BaseBlock
	Classes     []string // the type first ("note"), then any extra classes
	Title       string
	Collapsible bool // ??? : a <details>
	Open        bool // ???+ : open to start with
	TitleNode   bool // a written title, in its own child so its Markdown is read
}

// admonitionTitle is a written title: "Which is what `SOURCE_URL` is for"
// keeps its code, as MkDocs keeps it.
type admonitionTitle struct{ ast.BaseBlock }

func (n *admonitionTitle) Kind() ast.NodeKind { return kindAdmTitle }
func (n *admonitionTitle) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, nil, nil)
}

func (n *admonition) Kind() ast.NodeKind { return kindAdmonition }
func (n *admonition) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, map[string]string{"Classes": strings.Join(n.Classes, " "), "Title": n.Title}, nil)
}

// !!! type [more classes] ["Title"]   ??? type ["Title"]   ???+ type ["Title"]
// An empty title ("") leaves the title out; no title at all uses the type,
// capitalized, as MkDocs does.
var admonitionRe = regexp.MustCompile(`^(!!!|\?\?\?\+?)[ \t]+([A-Za-z][\w-]*(?:[ \t]+[A-Za-z][\w-]*)*)(?:[ \t]+"(.*)")?[ \t]*$`)

type admonitionParser struct{}

func (admonitionParser) Trigger() []byte { return []byte{'!', '?'} }

func (admonitionParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, seg := reader.PeekLine()
	w, pos := util.IndentWidth(line, reader.LineOffset())
	if w > 3 {
		return nil, parser.NoChildren
	}
	rest := util.TrimRightSpace(line[pos:])
	m := admonitionRe.FindSubmatch(rest)
	if m == nil {
		return nil, parser.NoChildren
	}
	classes := strings.Fields(string(m[2]))
	n := &admonition{Classes: classes, Collapsible: m[1][0] == '?', Open: bytes.HasSuffix(m[1], []byte("+"))}
	if m[3] != nil {
		n.Title = string(m[3])
		if n.Title != "" {
			idx := admonitionRe.FindSubmatchIndex(rest)
			t := &admonitionTitle{}
			t.Lines().Append(text.NewSegment(seg.Start+pos+idx[6], seg.Start+pos+idx[7]))
			n.AppendChild(n, t)
			n.TitleNode = true
		}
	} else {
		n.Title = strings.ToUpper(classes[0][:1]) + classes[0][1:]
	}
	reader.AdvanceToEOL()
	return n, parser.HasChildren
}

func (admonitionParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	return continueIndented(reader)
}

// continueIndented takes the next line into an admonition or a tab when it is
// blank or indented four spaces, and gives the block the line without those
// four spaces; anything less indented ends the block.
func continueIndented(reader text.Reader) parser.State {
	line, _ := reader.PeekLine()
	if util.IsBlank(line) {
		reader.AdvanceToEOL()
		return parser.Continue | parser.HasChildren
	}
	if w, _ := util.IndentWidth(line, reader.LineOffset()); w < 4 {
		return parser.Close
	}
	pos, padding := util.IndentPosition(line, reader.LineOffset(), 4)
	reader.AdvanceAndSetPadding(pos, padding)
	return parser.Continue | parser.HasChildren
}

func (admonitionParser) Close(ast.Node, text.Reader, parser.Context) {}
func (admonitionParser) CanInterruptParagraph() bool                 { return false }
func (admonitionParser) CanAcceptIndentedLine() bool                 { return false }

// --- content tabs ------------------------------------------------------------

type tab struct {
	ast.BaseBlock
	Label    string
	Selected bool // ===+ : the tab shown first
	NewSet   bool // ===! : starts a new set even right after another tab
}

func (n *tab) Kind() ast.NodeKind { return kindTab }
func (n *tab) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, map[string]string{"Label": n.Label}, nil)
}

type tabSet struct {
	ast.BaseBlock
	Index int // the set's number on the page, for the radio buttons' ids
}

func (n *tabSet) Kind() ast.NodeKind { return kindTabSet }
func (n *tabSet) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, map[string]string{"Index": fmt.Sprint(n.Index)}, nil)
}

var tabRe = regexp.MustCompile(`^===([+!]{0,2})[ \t]+"(.*)"[ \t]*$`)

type tabParser struct{}

func (tabParser) Trigger() []byte { return []byte{'='} }

func (tabParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, _ := reader.PeekLine()
	w, pos := util.IndentWidth(line, reader.LineOffset())
	if w > 3 {
		return nil, parser.NoChildren
	}
	m := tabRe.FindSubmatch(util.TrimRightSpace(line[pos:]))
	if m == nil {
		return nil, parser.NoChildren
	}
	n := &tab{Label: string(m[2]), Selected: bytes.Contains(m[1], []byte("+")), NewSet: bytes.Contains(m[1], []byte("!"))}
	reader.AdvanceToEOL()
	return n, parser.HasChildren
}

func (tabParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	return continueIndented(reader)
}
func (tabParser) Close(ast.Node, text.Reader, parser.Context) {}
func (tabParser) CanInterruptParagraph() bool                 { return false }
func (tabParser) CanAcceptIndentedLine() bool                 { return false }

// tabSets gathers tabs that follow one another into a set: one row of labels,
// one panel showing at a time.
type tabSets struct{}

func (tabSets) Transform(doc *ast.Document, _ text.Reader, _ parser.Context) {
	index := 0
	var visit func(parent ast.Node)
	visit = func(parent ast.Node) {
		var set *tabSet
		for c := parent.FirstChild(); c != nil; {
			next := c.NextSibling()
			t, ok := c.(*tab)
			if !ok {
				set = nil
				visit(c)
				c = next
				continue
			}
			if set == nil || t.NewSet {
				index++
				set = &tabSet{Index: index}
				parent.InsertBefore(parent, c, set)
			}
			parent.RemoveChild(parent, c)
			set.AppendChild(set, c)
			visit(c)
			c = next
		}
	}
	visit(doc)
}

// --- keys --------------------------------------------------------------------

type keys struct {
	ast.BaseInline
	Keys []string
}

func (n *keys) Kind() ast.NodeKind { return kindKeys }
func (n *keys) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, map[string]string{"Keys": strings.Join(n.Keys, "+")}, nil)
}

var keysRe = regexp.MustCompile(`^\+\+([A-Za-z0-9][A-Za-z0-9_-]*(?:\+[A-Za-z0-9][A-Za-z0-9_-]*)*)\+\+`)

// keysParser reads ++ctrl+alt+del++. As pymdownx.keys does, it takes only
// keys it knows (keymap.go, and the site's own markdown.keys); anything else
// stays text.
type keysParser struct{ keys map[string]string }

func (keysParser) Trigger() []byte { return []byte{'+'} }

func (kp keysParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, _ := block.PeekLine()
	m := keysRe.FindSubmatch(line)
	if m == nil {
		return nil
	}
	ks := strings.Split(strings.ToLower(string(m[1])), "+")
	for _, k := range ks {
		_, mine := kp.keys[k]
		_, known := keyLabels[keyName(k)]
		if !mine && !known {
			return nil
		}
	}
	block.Advance(len(m[0]))
	return &keys{Keys: ks}
}

// keyName is a key's own name, for a shortcut written with another of its
// names (ctrl for control, esc for escape), as pymdownx.keys has them
// (keymap.go).
func keyName(k string) string {
	if n, ok := keyAliases[k]; ok {
		return n
	}
	return k
}

func (r docsRenderer) keyLabel(k string) string {
	if n, ok := r.keys[k]; ok {
		return n
	}
	name := keyName(k)
	if n, ok := r.keys[name]; ok {
		return n
	}
	if n, ok := keyLabels[name]; ok {
		return n
	}
	if len(k) == 1 || (k[0] == 'f' && len(k) <= 3 && strings.Trim(k[1:], "0123456789") == "") {
		return strings.ToUpper(k)
	}
	return strings.ToUpper(k[:1]) + k[1:]
}

// --- attributes after a picture or a link ------------------------------------

// trailingAttributes reads { .class #id key=value } written straight after
// an image or a link, the way attr_list does, and puts them on it. On a site
// that doesn't trust its authors' HTML, only id and class are kept: anything
// else (style, onerror) would be raw HTML by another name.
type trailingAttributes struct{ anyAttribute bool }

var attrTokenRe = regexp.MustCompile(`\.([\w-]+)|#([\w-]+)|([\w-]+)=(?:"([^"]*)"|'([^']*)'|([^\s}]+))`)

func (ta trailingAttributes) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	src := reader.Source()
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if n.Kind() != ast.KindImage && n.Kind() != ast.KindLink {
			return ast.WalkContinue, nil
		}
		// The text after it, which other extensions (autolinking, say) may
		// have split into pieces: read across pieces that touch.
		var texts []*ast.Text
		var v []byte
		for c := n.NextSibling(); c != nil; c = c.NextSibling() {
			t, ok := c.(*ast.Text)
			if !ok || (len(texts) > 0 && t.Segment.Start != texts[len(texts)-1].Segment.Stop) {
				break
			}
			texts = append(texts, t)
			v = append(v, t.Segment.Value(src)...)
			if bytes.IndexByte(v, '}') >= 0 || t.SoftLineBreak() || t.HardLineBreak() {
				break
			}
		}
		if len(v) == 0 || v[0] != '{' {
			return ast.WalkContinue, nil
		}
		end := bytes.IndexByte(v, '}')
		if end < 0 {
			return ast.WalkContinue, nil
		}
		inner := strings.TrimSpace(strings.TrimPrefix(string(v[1:end]), ":"))
		if inner == "" || strings.TrimSpace(attrTokenRe.ReplaceAllString(inner, "")) != "" {
			return ast.WalkContinue, nil // not only attributes: leave the text alone
		}
		var classes []string
		for _, m := range attrTokenRe.FindAllStringSubmatch(inner, -1) {
			switch {
			case m[1] != "":
				classes = append(classes, m[1])
			case m[2] != "":
				n.SetAttributeString("id", []byte(m[2]))
			case m[3] != "" && ta.anyAttribute:
				n.SetAttributeString(m[3], []byte(m[4]+m[5]+m[6]))
			}
		}
		if len(classes) > 0 {
			if c, ok := n.AttributeString("class"); ok {
				classes = append([]string{string(c.([]byte))}, classes...)
			}
			n.SetAttributeString("class", []byte(strings.Join(classes, " ")))
		}
		// Drop the attribute text from the pieces it was in, keeping what
		// follows the closing brace.
		cut := texts[0].Segment.Start + end + 1
		for _, t := range texts {
			switch {
			case t.Segment.Stop <= cut && !t.SoftLineBreak() && !t.HardLineBreak():
				t.Parent().RemoveChild(t.Parent(), t)
			case t.Segment.Stop <= cut:
				t.Segment = t.Segment.WithStart(t.Segment.Stop) // keep the line break
			case t.Segment.Start < cut:
				t.Segment = t.Segment.WithStart(cut)
			}
		}
		return ast.WalkContinue, nil
	})
}

// --- rendering ---------------------------------------------------------------

type docsRenderer struct{ keys map[string]string }

func (r docsRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindAdmonition, renderAdmonition)
	reg.Register(kindAdmTitle, renderAdmonitionTitle)
	reg.Register(kindTabSet, renderTabSet)
	reg.Register(kindTab, renderTab)
	reg.Register(kindKeys, r.renderKeys)
	reg.Register(kindCodeTitle, renderCodeTitle)
}

func renderAdmonition(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*admonition)
	cls := html.EscapeString(strings.Join(n.Classes, " "))
	if n.Collapsible {
		if entering {
			open := ""
			if n.Open {
				open = " open"
			}
			fmt.Fprintf(w, "<details class=\"%s\"%s>\n", cls, open)
			if !n.TitleNode {
				fmt.Fprintf(w, "<summary>%s</summary>\n", html.EscapeString(n.Title))
			}
		} else {
			w.WriteString("</details>\n")
		}
		return ast.WalkContinue, nil
	}
	if entering {
		fmt.Fprintf(w, "<div class=\"admonition %s\">\n", cls)
		if n.Title != "" && !n.TitleNode {
			fmt.Fprintf(w, "<p class=\"admonition-title\">%s</p>\n", html.EscapeString(n.Title))
		}
	} else {
		w.WriteString("</div>\n")
	}
	return ast.WalkContinue, nil
}

func renderAdmonitionTitle(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	summary := node.Parent().(*admonition).Collapsible
	switch {
	case entering && summary:
		w.WriteString("<summary>")
	case entering:
		w.WriteString(`<p class="admonition-title">`)
	case summary:
		w.WriteString("</summary>\n")
	default:
		w.WriteString("</p>\n")
	}
	return ast.WalkContinue, nil
}

// --- a code block's file name ------------------------------------------------

// codeTitle wraps a fenced code block whose info names a file, ```json
// title="servers.json", as pymdownx.highlight shows it: the name above the
// code.
type codeTitle struct {
	ast.BaseBlock
	Title string
}

func (n *codeTitle) Kind() ast.NodeKind { return kindCodeTitle }
func (n *codeTitle) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, map[string]string{"Title": n.Title}, nil)
}

var codeTitleRe = regexp.MustCompile(`\btitle=(?:"([^"]*)"|'([^']*)')`)

type codeTitles struct{}

func (codeTitles) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	var found []*ast.FencedCodeBlock
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if fc, ok := n.(*ast.FencedCodeBlock); ok && entering && fc.Info != nil {
			found = append(found, fc)
		}
		return ast.WalkContinue, nil
	})
	for _, fc := range found {
		m := codeTitleRe.FindSubmatch(fc.Info.Segment.Value(reader.Source()))
		if m == nil {
			continue
		}
		t := &codeTitle{Title: string(m[1]) + string(m[2])}
		fc.Parent().ReplaceChild(fc.Parent(), fc, t)
		t.AppendChild(t, fc)
	}
}

func renderCodeTitle(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		fmt.Fprintf(w, "<div class=\"highlight\"><span class=\"filename\">%s</span>", html.EscapeString(node.(*codeTitle).Title))
	} else {
		w.WriteString("</div>\n")
	}
	return ast.WalkContinue, nil
}

// renderTabSet writes pymdownx.tabbed's "alternate" markup: a radio button
// per tab, then the labels, then the panels. A stylesheet shows the panel
// whose button is checked, so the tabs work without a script; without the
// stylesheet every panel shows, one after another, under its label.
func renderTabSet(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*tabSet)
	if !entering {
		w.WriteString("</div>\n</div>\n")
		return ast.WalkContinue, nil
	}
	var tabs []*tab
	selected := 0
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		t := c.(*tab)
		if t.Selected && selected == 0 {
			selected = len(tabs)
		}
		tabs = append(tabs, t)
	}
	fmt.Fprintf(w, "<div class=\"tabbed-set tabbed-alternate\" data-tabs=\"%d:%d\">", n.Index, len(tabs))
	for i := range tabs {
		checked := ""
		if i == selected {
			checked = ` checked="checked"`
		}
		fmt.Fprintf(w, "<input%s id=\"__tabbed_%d_%d\" name=\"__tabbed_%d\" type=\"radio\">", checked, n.Index, i+1, n.Index)
	}
	w.WriteString("<div class=\"tabbed-labels\">")
	for i, t := range tabs {
		fmt.Fprintf(w, "<label for=\"__tabbed_%d_%d\">%s</label>", n.Index, i+1, html.EscapeString(t.Label))
	}
	w.WriteString("</div>\n<div class=\"tabbed-content\">\n")
	return ast.WalkContinue, nil
}

func renderTab(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		w.WriteString("<div class=\"tabbed-block\">\n")
	} else {
		w.WriteString("</div>\n")
	}
	return ast.WalkContinue, nil
}

func (r docsRenderer) renderKeys(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*keys)
	w.WriteString(`<span class="keys">`)
	for i, k := range n.Keys {
		if i > 0 {
			w.WriteString("<span>+</span>")
		}
		fmt.Fprintf(w, `<kbd class="key-%s">%s</kbd>`, html.EscapeString(keyName(k)), html.EscapeString(r.keyLabel(k)))
	}
	w.WriteString(`</span>`)
	return ast.WalkSkipChildren, nil
}

// --- Markdown inside HTML blocks ---------------------------------------------

// mdInHTML lets a block of HTML hold Markdown, as md_in_html does: an
// opening tag on a line of its own carrying a markdown attribute,
//
//	<div class="grid" markdown>
//	A **paragraph**, a list, anything.
//	</div>
//
// becomes the tag without the attribute, a blank line, the Markdown
// (dedented), a blank line and the closing tag. CommonMark ends an HTML block
// at a blank line, so what's between is read as Markdown again. Only for
// sites whose HTML is trusted or sanitized; otherwise the HTML is dropped
// anyway. Tags whose closing tag isn't on a line of its own are left as they
// are.
var (
	mdOpenRe  = regexp.MustCompile(`^( {0,3})<([a-zA-Z][a-zA-Z0-9]*)\b([^>]*?)\s+markdown(?:=(?:"[^"]*"|'[^']*'|\S+))?([^>]*)>\s*$`)
	mdAttrRe  = regexp.MustCompile(`\s+markdown(?:=(?:"[^"]*"|'[^']*'|[^\s>]+))?`)
	fenceLine = regexp.MustCompile("^\\s*(```|~~~)")
)

func mdInHTML(src []byte) []byte {
	if !bytes.Contains(src, []byte("markdown")) {
		return src
	}
	lines := strings.SplitAfter(string(src), "\n")
	return []byte(strings.Join(mdInHTMLLines(lines), ""))
}

func mdInHTMLLines(lines []string) []string {
	var out []string
	inFence := false
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if fenceLine.MatchString(line) {
			inFence = !inFence
		}
		m := mdOpenRe.FindStringSubmatch(strings.TrimRight(line, "\r\n"))
		if inFence || m == nil {
			out = append(out, line)
			continue
		}
		tag := strings.ToLower(m[2])
		openRe := regexp.MustCompile(`(?i)<` + tag + `\b[^>]*[^/]>|<` + tag + `>`)
		closeRe := regexp.MustCompile(`(?i)</` + tag + `\s*>`)
		depth, end := 1, -1
		for j := i + 1; j < len(lines); j++ {
			depth += len(openRe.FindAllString(lines[j], -1)) - len(closeRe.FindAllString(lines[j], -1))
			if depth == 0 {
				if strings.TrimSpace(closeRe.ReplaceAllString(lines[j], "")) == "" {
					end = j
				}
				break
			}
		}
		if end < 0 {
			out = append(out, line)
			continue
		}
		inner := mdInHTMLLines(dedent(lines[i+1 : end]))
		out = append(out, mdAttrRe.ReplaceAllString(strings.TrimRight(line, "\r\n"), "")+"\n", "\n")
		out = append(out, inner...)
		if len(inner) > 0 && !strings.HasSuffix(inner[len(inner)-1], "\n") {
			out = append(out, "\n")
		}
		out = append(out, "\n", lines[end])
		i = end
	}
	return out
}

// dedent removes the indentation every non-blank line shares.
func dedent(lines []string) []string {
	common := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n := len(l) - len(strings.TrimLeft(l, " \t"))
		if common < 0 || n < common {
			common = n
		}
	}
	if common <= 0 {
		return lines
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		if len(l) >= common && strings.TrimSpace(l[:common]) == "" {
			out[i] = l[common:]
		} else {
			out[i] = strings.TrimLeft(l, " \t")
		}
	}
	return out
}
