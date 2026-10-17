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
		{"wordpress caption", `<figure id="attachment_5" style="width: 1600px" class="wp-caption aligncenter"><img class="size-full" src="/a.jpg" alt="A dock" width="1600" height="900" srcset="/a-300.jpg 300w" sizes="100vw"><figcaption class="wp-caption-text">At dawn</figcaption></figure>`,
			"![A dock](/a.jpg)\n\n*At dawn*\n"},
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

func TestSafeImportPaths(t *testing.T) {
	for in, want := range map[string]string{"../../x": "x", "/a//b/": "a/b", `..\..\etc`: "etc", "news/2026": "news/2026"} {
		if got := safeDir(in); got != want {
			t.Errorf("safeDir(%q) = %q, want %q", in, got, want)
		}
	}
	if s := slugify("../../../x/README"); strings.Contains(s, "/") || strings.Contains(s, "..") {
		t.Errorf("slugify kept a path: %q", s)
	}
	if s := slugify("Café Ñandú 日本"); s != "café-ñandú-日本" {
		t.Errorf("letters lost: %q", s)
	}
}

func TestShortSummary(t *testing.T) {
	long := "In the realm of Linux system administration, tools like sudo are indispensable. However, there's a newcomer on the block, run0. This tool is designed to provide a secure way to run commands as root."
	if got := shortSummary(long); got != "In the realm of Linux system administration, tools like sudo are indispensable. However, there's a newcomer on the block, run0." {
		t.Errorf("sentences: %q", got)
	}
	one := strings.Repeat("word ", 50) + "end."
	if got := shortSummary(one); len([]rune(got)) > 160 || !strings.HasSuffix(got, "word…") {
		t.Errorf("one long sentence: %q (%d)", got, len([]rune(got)))
	}
	dotted := "A green canvas apron with a deep pocket for your garden tools. A small logo sits on the chest. The full logo and example.org run across the back. Sizes S to XL, made to order."
	if got := shortSummary(dotted); got != "A green canvas apron with a deep pocket for your garden tools. A small logo sits on the chest. The full logo and example.org run across the back." {
		t.Errorf("a full stop inside a word: %q", got)
	}
	if got := shortSummary("Short and sweet."); got != "Short and sweet." {
		t.Errorf("short: %q", got)
	}
}

func TestCleanHTMLDropsSrcset(t *testing.T) {
	got := cleanHTML(`<img src="/a.jpg" srcset="https://old.example/a-300.jpg 300w, https://old.example/a-1024.jpg 1024w" sizes="(max-width: 1600px) 100vw, 1600px" alt="x">`)
	if strings.Contains(got, "srcset") || strings.Contains(got, "sizes") || !strings.Contains(got, `src="/a.jpg"`) {
		t.Errorf("cleanHTML: %q", got)
	}
}

func TestUnlinkOldPictures(t *testing.T) {
	md := "[![Shot](https://old.example/wp-content/uploads/a-150x150.jpg)](https://old.example/a/)\n\n" +
		"[![Post](/p.jpg)](https://old.example/kept-post/)\n\n[![Elsewhere](/e.jpg)](https://other.example/x/)\n"
	got := unlinkOldPictures(md, "https://old.example", map[string]bool{"/kept-post/": true})
	want := "![Shot](https://old.example/wp-content/uploads/a-150x150.jpg)\n\n" +
		"[![Post](/p.jpg)](https://old.example/kept-post/)\n\n[![Elsewhere](/e.jpg)](https://other.example/x/)\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}
