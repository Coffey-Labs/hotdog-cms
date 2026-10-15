package site

import (
	"strings"
	"testing"
)

func TestHighlight(t *testing.T) {
	got := render(t, MarkdownConfig{}, "```go\nfunc main() { return \"<hi>\" } // done\n```\n")
	for _, want := range []string{
		`<div class="highlight"><pre><code class="language-go"><span class="kd">func</span> <span class="nf">main</span>`,
		`<span class="s">&quot;&lt;hi&gt;&quot;</span>`,
		`<span class="c1">// done</span>` + "\n</code></pre></div>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
}

func TestHighlightLeavesPlainBlocks(t *testing.T) {
	for _, src := range []string{"```\na < b\n```\n", "```nosuchlanguage\na < b\n```\n", "```text\na < b\n```\n", "    a < b\n"} {
		got := render(t, MarkdownConfig{}, src)
		if strings.Contains(got, "highlight") || strings.Contains(got, "<span") || !strings.Contains(got, "a &lt; b\n</code></pre>") {
			t.Errorf("%q: want the block as it was, got\n%s", src, got)
		}
	}
	off := false
	if got := render(t, MarkdownConfig{Highlight: &off}, "```go\nfunc f() {}\n```\n"); got != "<pre><code class=\"language-go\">func f() {}\n</code></pre>\n" {
		t.Errorf("highlight: false still colored:\n%s", got)
	}
}

func TestHighlightWithTitleAndLines(t *testing.T) {
	got := render(t, MarkdownConfig{}, "```yaml title=\"site.yaml\" hl_lines=\"2 4-5\"\na: 1\nb: 2\nc: 3\nd: 4\ne: 5\n```\n")
	if strings.Count(got, `class="highlight"`) != 1 || !strings.HasPrefix(got, `<div class="highlight"><span class="filename">site.yaml</span><pre><code class="language-yaml">`) {
		t.Errorf("want one highlight box, the file name over the code:\n%s", got)
	}
	if n := strings.Count(got, `<span class="hll">`); n != 3 {
		t.Errorf("want lines 2, 4 and 5 marked, got %d:\n%s", n, got)
	}
	if !strings.Contains(got, "\n</span><span class=\"hll\"><span class=\"nt\">b</span>") && !strings.Contains(got, `<span class="hll"><span class="nt">b</span>`) {
		t.Errorf("line 2 isn't the marked one:\n%s", got)
	}
	// Marking lines works on a block in no language too.
	if got := render(t, MarkdownConfig{}, "``` hl_lines=\"1\"\nplain\n```\n"); !strings.Contains(got, "<span class=\"hll\">plain\n</span>") {
		t.Errorf("unmarked plain line:\n%s", got)
	}
}

// With unsafe_html the output passes the sanitizer, which must keep the
// classes the colors hang on.
func TestHighlightSurvivesSanitizer(t *testing.T) {
	got := render(t, MarkdownConfig{UnsafeHTML: true}, "```bash\necho \"hi\" # say it\n```\n")
	for _, want := range []string{`<div class="highlight">`, `<span class="nb">echo</span>`, `<span class="c1"># say it</span>`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
}
