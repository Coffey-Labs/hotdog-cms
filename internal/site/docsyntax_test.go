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
	got := render(t, MarkdownConfig{}, "Press ++ctrl+alt+del++, ++cmd+k++ or ++f5++; C++ and a++b stay text.\n")
	for _, want := range []string{
		`<span class="keys"><kbd class="key-ctrl">Ctrl</kbd><span>+</span><kbd class="key-alt">Alt</kbd><span>+</span><kbd class="key-del">Del</kbd></span>`,
		`<kbd class="key-cmd">Cmd</kbd><span>+</span><kbd class="key-k">K</kbd>`,
		`<kbd class="key-f5">F5</kbd>`,
		"C++ and a++b stay text.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
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
