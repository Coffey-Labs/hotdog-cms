package importer

import (
	"strings"
	"testing"
)

func TestToMarkdown(t *testing.T) {
	for _, c := range []struct{ name, in, want string }{
		{"paragraphs and inline", `<p>Some <strong>bold</strong>, <em>italic</em> and <code>code</code> with a <a href="/about/">link</a>.</p><p>Second.</p>`,
			"Some **bold**, *italic* and `code` with a [link](/about/).\n\nSecond.\n"},
		{"headings and rule", `<h2>Title</h2><p>Text</p><hr><h3 id="x">Sub <em>title</em></h3>`, "## Title\n\nText\n\n---\n\n### Sub *title*\n"},
		{"nested lists", `<ul><li>One</li><li>Two<ul><li>Two a</li></ul></li></ul><ol start="3"><li>Three</li><li><p>Four</p></li></ol>`,
			"- One\n- Two\n  - Two a\n\n3. Three\n4. Four\n"},
		{"quote", `<blockquote><p>Said <em>this</em>.</p><p>And that.</p></blockquote>`, "> Said *this*.\n>\n> And that.\n"},
		{"code block", `<pre class="wp-block-code"><code class="language-go">fmt.Println("a ` + "```" + ` b")</code></pre>`, "````go\nfmt.Println(\"a ``` b\")\n````\n"},
		{"figure", `<figure class="wp-block-image"><img src="https://ex.org/a.jpg" alt="A dock"><figcaption>At dawn</figcaption></figure>`, "![A dock](https://ex.org/a.jpg)\n\n*At dawn*\n"},
		{"escaping", `<p>1. Not a list, a *star*, snake_case and _edge_ [brackets]</p>`, "1\\. Not a list, a \\*star\\*, snake_case and \\_edge\\_ \\[brackets\\]\n"},
		{"break and strike", `<p>Line one<br>line two <del>gone</del></p>`, "Line one\\\nline two ~~gone~~\n"},
		{"simple table", `<table><thead><tr><th>Plan</th><th>Price</th></tr></thead><tbody><tr><td>Basic</td><td>$5</td></tr></tbody></table>`,
			"| Plan | Price |\n| --- | --- |\n| Basic | $5 |\n"},
		{"block comments ignored", "<!-- wp:paragraph -->\n<p>Hi</p>\n<!-- /wp:paragraph -->", "Hi\n"},
	} {
		got := ToMarkdown(c.in)
		if got.Markdown != c.want || len(got.Lossy) != 0 {
			t.Errorf("%s:\ngot  %q (lossy %v)\nwant %q", c.name, got.Markdown, got.Lossy, c.want)
		}
	}
}

func TestLossy(t *testing.T) {
	for _, c := range []struct{ in, why string }{
		{`<figure class="wp-block-embed"><iframe src="https://www.youtube.com/embed/x"></iframe></figure>`, "an embed"},
		{`<div class="wp-block-columns"><div class="wp-block-column"><p>a</p></div></div>`, "a layout block (wp-block-columns)"},
		{`<table><tr><td colspan="2">a</td></tr></table>`, "a table with merged or complex cells"},
		{`<p style="color:red">Red</p>`, "inline styles"},
		{`<p>E = mc<sup>2</sup></p>`, "superscript"},
	} {
		got := ToMarkdown(c.in)
		if !strings.Contains(strings.Join(got.Lossy, ","), c.why) {
			t.Errorf("%s: lossy %v, want %q", c.in, got.Lossy, c.why)
		}
	}
}
