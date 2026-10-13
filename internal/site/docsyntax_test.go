package site

import (
	"strings"
	"testing"
)

func render(t *testing.T, c MarkdownConfig, src string) string {
	t.Helper()
	h, err := NewMarkdown(c, "example.org").Render([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return string(h)
}

func TestAdmonitions(t *testing.T) {
	got := render(t, MarkdownConfig{}, "!!! note\n    Body **bold**.\n\n!!! warning \"Watch out\"\n    One.\n\n    - a\n    - b\n\n!!! tip \"\"\n    No title.\n\n!!! danger extra\n    Two classes.\n")
	for _, want := range []string{
		"<div class=\"admonition note\">\n<p class=\"admonition-title\">Note</p>\n<p>Body <strong>bold</strong>.</p>\n</div>",
		"<div class=\"admonition warning\">\n<p class=\"admonition-title\">Watch out</p>\n<p>One.</p>\n<ul>\n<li>a</li>\n<li>b</li>\n</ul>\n</div>",
		"<div class=\"admonition tip\">\n<p>No title.</p>\n</div>",
		"<div class=\"admonition danger extra\">",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
}

func TestCollapsibleAndNesting(t *testing.T) {
	got := render(t, MarkdownConfig{}, "??? info \"Click\"\n    Hidden.\n\n???+ danger\n    Open.\n\n    !!! note \"Nested\"\n        Inside.\n\nAfter.\n")
	for _, want := range []string{
		"<details class=\"info\">\n<summary>Click</summary>\n<p>Hidden.</p>\n</details>",
		"<details class=\"danger\" open>\n<summary>Danger</summary>\n<p>Open.</p>\n<div class=\"admonition note\">\n<p class=\"admonition-title\">Nested</p>\n<p>Inside.</p>\n</div>\n</details>",
		"</details>\n<p>After.</p>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
}

func TestNotAnAdmonition(t *testing.T) {
	got := render(t, MarkdownConfig{}, "A line\n!!! note in a paragraph.\n\n    !!! note\n    (indented four: code)\n")
	if strings.Contains(got, "admonition") {
		t.Errorf("an admonition where there is none:\n%s", got)
	}
}

func TestTabs(t *testing.T) {
	got := render(t, MarkdownConfig{}, "=== \"Linux\"\n    ```sh\n    a\n    ```\n\n===+ \"macOS\"\n    Two.\n\n===! \"Other\"\n    New set.\n\n=== \"More\"\n    Same set.\n")
	for _, want := range []string{
		`<div class="tabbed-set tabbed-alternate" data-tabs="1:2"><input id="__tabbed_1_1" name="__tabbed_1" type="radio"><input checked="checked" id="__tabbed_1_2" name="__tabbed_1" type="radio"><div class="tabbed-labels"><label for="__tabbed_1_1">Linux</label><label for="__tabbed_1_2">macOS</label></div>`,
		"<div class=\"tabbed-block\">\n<pre><code class=\"language-sh\">a\n</code></pre>\n</div>",
		`data-tabs="2:2"><input checked="checked" id="__tabbed_2_1"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
}

func TestKeys(t *testing.T) {
	// As pymdownx.keys writes them: a key's own name in the class, whatever
	// name it was written with, and unknown keys left as text.
	got := render(t, MarkdownConfig{}, "Press ++ctrl+alt+del++, ++cmd+k++ or ++f5++; ++question++, ++hash++; C++ and a++b stay text.\n")
	for _, want := range []string{
		`<span class="keys"><kbd class="key-control">Ctrl</kbd><span>+</span><kbd class="key-alt">Alt</kbd><span>+</span><kbd class="key-delete">Del</kbd></span>`,
		`<kbd class="key-command">Cmd</kbd><span>+</span><kbd class="key-k">K</kbd>`,
		`<kbd class="key-f5">F5</kbd>`,
		`<span class="keys"><kbd class="key-question">?</kbd></span>, ++hash++;`,
		"C++ and a++b stay text.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	// The site's own names (pymdownx.keys' key_map).
	got = render(t, MarkdownConfig{Keys: map[string]string{"hash": "#", "ctrl": "Control"}}, "++hash++ and ++ctrl+k++\n")
	if !strings.Contains(got, `<kbd class="key-hash">#</kbd>`) || !strings.Contains(got, `<kbd class="key-control">Control</kbd>`) {
		t.Errorf("site keys: %s", got)
	}
}

func TestTrailingAttributesAndDefinitions(t *testing.T) {
	src := "![Shot](a.png){ .shot }\n[Go](https://example.org){ #here .button style=\"x\" } on.\n\nTerm\n:   Meaning.\n"
	got := render(t, MarkdownConfig{}, src)
	for _, want := range []string{`<img src="a.png" alt="Shot" class="shot">`, `<a href="https://example.org" id="here" class="button">Go</a> on.`, "<dl>\n<dt>Term</dt>\n<dd>Meaning.</dd>\n</dl>"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "style") || strings.Contains(got, "{") {
		t.Errorf("attributes left in, or style let through:\n%s", got)
	}
	// A brace that isn't attributes is text.
	if got := render(t, MarkdownConfig{}, "![a](b.png){not attributes}\n"); !strings.Contains(got, "{not attributes}") {
		t.Errorf("text eaten: %s", got)
	}
}

func TestMarkdownInHTML(t *testing.T) {
	src := "<div class=\"grid\" markdown>\n  **Bold** inside.\n\n  - one\n</div>\n\n```html\n<div markdown>\nleft alone in code\n</div>\n```\n"
	got := render(t, MarkdownConfig{TrustHTML: true}, src)
	for _, want := range []string{"<div class=\"grid\">\n<p><strong>Bold</strong> inside.</p>\n<ul>\n<li>one</li>\n</ul>\n</div>", "&lt;div markdown&gt;"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	// Sanitized HTML: the same, through the sanitizer.
	if got := render(t, MarkdownConfig{UnsafeHTML: true}, src); !strings.Contains(got, "<strong>Bold</strong>") {
		t.Errorf("sanitized: %s", got)
	}
	// No HTML allowed: the block goes, and its Markdown isn't let out.
	if got := render(t, MarkdownConfig{}, src); strings.Contains(got, "grid") || strings.Contains(got, "Bold") {
		t.Errorf("HTML off but the block came through:\n%s", got)
	}
}

func TestTabsSurviveTheSanitizer(t *testing.T) {
	got := render(t, MarkdownConfig{UnsafeHTML: true}, "=== \"A\"\n    One.\n\n=== \"B\"\n    Two.\n\n??? note\n    x\n\n<input type=\"text\" name=\"q\" onfocus=\"x()\">\n")
	for _, want := range []string{`<input checked="checked" id="__tabbed_1_1" name="__tabbed_1" type="radio">`, `<label for="__tabbed_1_2">B</label>`, `<details class="note">`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, `type="text"`) || strings.Contains(got, "onfocus") {
		t.Errorf("the sanitizer let an author's input through:\n%s", got)
	}
}

func TestMkDocsHeadingIDs(t *testing.T) {
	for in, want := range map[string]string{
		"On Linux & BSD":       "on-linux-bsd",
		"Größe und Gewicht":    "groe-und-gewicht",
		"  What's   new?  ":    "whats-new",
		"snake_case -- dashes": "snake_case-dashes",
		"日本語":                  "",
	} {
		if got := mkdocsSlug(in); got != want {
			t.Errorf("mkdocsSlug(%q) = %q, want %q", in, got, want)
		}
	}
	md := NewMarkdown(MarkdownConfig{HeadingIDs: "mkdocs"}, "example.org")
	out, err := md.Render([]byte("## On Linux & BSD\n\n## On Linux & BSD\n\n## On Linux & BSD\n\n## 日本語\n\n## Mine {#on-linux-bsd_3}\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`id="on-linux-bsd"`, `id="on-linux-bsd_1"`, `id="on-linux-bsd_2"`, `id="_1"`, `id="on-linux-bsd_3"`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("missing %s in\n%s", want, out)
		}
	}
	plain, _ := NewMarkdown(MarkdownConfig{}, "example.org").Render([]byte("## On Linux & BSD\n"))
	if !strings.Contains(string(plain), `id="on-linux--bsd"`) {
		t.Errorf("the default is unchanged: %s", plain)
	}
}

// A written title is Markdown, as Python-Markdown reads it.
func TestAdmonitionTitleMarkdown(t *testing.T) {
	got := render(t, MarkdownConfig{}, "!!! quote \"Which is what `SOURCE_URL` is for\"\n\n    Body *text*.\n\n??? note \"`--max` and **bold**\"\n\n    Folded.\n\n!!! tip \"\"\n\n    No title.\n\n!!! warning\n\n    Default title.\n")
	for _, want := range []string{
		"<div class=\"admonition quote\">\n<p class=\"admonition-title\">Which is what <code>SOURCE_URL</code> is for</p>\n<p>Body <em>text</em>.</p>\n</div>",
		"<details class=\"note\">\n<summary><code>--max</code> and <strong>bold</strong></summary>\n<p>Folded.</p>\n</details>",
		"<div class=\"admonition tip\">\n<p>No title.</p>\n</div>",
		"<div class=\"admonition warning\">\n<p class=\"admonition-title\">Warning</p>\n<p>Default title.</p>\n</div>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
}

func TestCodeTitle(t *testing.T) {
	got := render(t, MarkdownConfig{}, "```json title=\"servers.json\"\n{\"a\": 1}\n```\n\n```sh\nls\n```\n")
	for _, want := range []string{
		"<div class=\"highlight\"><span class=\"filename\">servers.json</span><pre><code class=\"language-json\">{&quot;a&quot;: 1}\n</code></pre>\n</div>",
		"<pre><code class=\"language-sh\">ls\n</code></pre>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Count(got, "filename") != 1 {
		t.Errorf("a block without a title has none:\n%s", got)
	}
}
