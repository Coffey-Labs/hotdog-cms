package site

// Code highlighting, done when the site is built, so a page needs no script
// for it. A fenced block whose info names a language chroma knows,
//
//	```go
//
// comes out as pymdownx.highlight writes it, with Pygments' class names,
// so a stylesheet written for MkDocs colors it too:
//
//	<div class="highlight"><pre><code class="language-go"><span class="kd">func</span> …
//
// The colors are the site's stylesheet's to choose (the starter's and the
// docs pack's have them); without any, the code reads as it did before.
// A block with no language, or one chroma doesn't know, is left as it was.
//
// hl_lines="2 4-5" in the info marks lines, as pymdownx.highlight does:
// each is wrapped in <span class="hll">.

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

type codeHighlight struct{}

func (codeHighlight) Extend(m goldmark.Markdown) {
	// Ahead of goldmark's own renderer (1000), which still draws the blocks
	// this one leaves alone.
	m.Renderer().AddOptions(renderer.WithNodeRenderers(util.Prioritized(codeRenderer{}, 200)))
}

type codeRenderer struct{}

func (codeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, renderFencedCode)
}

// plainFenced is goldmark's own renderer for a fenced block, for the blocks
// that aren't highlighted, so they come out exactly as they always have.
var plainFenced = func() renderer.NodeRendererFunc {
	var g grabFenced
	html.NewRenderer().RegisterFuncs(&g)
	return g.f
}()

type grabFenced struct{ f renderer.NodeRendererFunc }

func (g *grabFenced) Register(k ast.NodeKind, f renderer.NodeRendererFunc) {
	if k == ast.KindFencedCodeBlock {
		g.f = f
	}
}

var hlLinesRe = regexp.MustCompile(`\bhl_lines=(?:"([^"]*)"|'([^']*)')`)

func renderFencedCode(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.FencedCodeBlock)
	var info []byte
	if n.Info != nil {
		info = n.Info.Segment.Value(src)
	}
	lang := strings.ToLower(string(n.Language(src)))
	marked := markedLines(info)
	lexer := codeLexer(lang)
	if lexer == nil && len(marked) > 0 {
		lexer = lexers.Fallback
	}
	if lexer == nil {
		return plainFenced(w, src, node, entering)
	}
	if !entering {
		return ast.WalkContinue, nil
	}
	var code strings.Builder
	for i := 0; i < n.Lines().Len(); i++ {
		l := n.Lines().At(i)
		code.Write(l.Value(src))
	}
	it, err := chroma.Coalesce(lexer).Tokenise(&chroma.TokeniseOptions{State: "root", EnsureLF: true}, code.String())
	if err != nil {
		// chroma gave up on it; the block is still worth showing.
		return plainFenced(w, src, node, entering)
	}
	_, titled := n.Parent().(*codeTitle)
	if !titled {
		w.WriteString(`<div class="highlight">`)
	}
	w.WriteString(`<pre><code`)
	if lang != "" {
		w.WriteString(` class="language-`)
		w.Write(util.EscapeHTML([]byte(lang)))
		w.WriteString(`"`)
	}
	w.WriteString(">")
	for i, line := range chroma.SplitTokensIntoLines(it.Tokens()) {
		if marked[i+1] {
			w.WriteString(`<span class="hll">`)
		}
		for _, t := range line {
			if cls := tokenClass(t.Type); cls != "" {
				w.WriteString(`<span class="` + cls + `">`)
				w.Write(util.EscapeHTML([]byte(t.Value)))
				w.WriteString("</span>")
			} else {
				w.Write(util.EscapeHTML([]byte(t.Value)))
			}
		}
		if marked[i+1] {
			w.WriteString("</span>")
		}
	}
	w.WriteString("</code></pre>")
	if !titled {
		w.WriteString("</div>")
	}
	w.WriteString("\n")
	return ast.WalkSkipChildren, nil
}

// codeLexer finds the lexer for a fence's language, by name, alias or file
// extension. Plain text gets none: there's nothing in it to color.
func codeLexer(lang string) chroma.Lexer {
	if lang == "" {
		return nil
	}
	l := lexers.Get(lang)
	if l == nil || l.Config().Name == "plaintext" {
		return nil
	}
	return l
}

// tokenClass is the Pygments class for a token, or its parent's when it has
// none of its own; plain text has none.
func tokenClass(t chroma.TokenType) string {
	for ; t != 0; t = t.Parent() {
		if cls, ok := chroma.StandardTypes[t]; ok {
			if cls == "w" {
				return "" // whitespace: nothing to color
			}
			return cls
		}
	}
	return ""
}

// markedLines reads hl_lines="2 4-5" into the line numbers it names.
func markedLines(info []byte) map[int]bool {
	m := hlLinesRe.FindSubmatch(info)
	if m == nil {
		return nil
	}
	out := map[int]bool{}
	for _, f := range strings.Fields(string(m[1]) + string(m[2])) {
		from, to, ok := strings.Cut(f, "-")
		a, err := strconv.Atoi(from)
		if err != nil {
			continue
		}
		b := a
		if ok {
			if b, err = strconv.Atoi(to); err != nil {
				continue
			}
		}
		for i := a; i <= b && i-a < 10000; i++ {
			out[i] = true
		}
	}
	return out
}
