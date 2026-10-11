package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSimpleFrontMatter(t *testing.T) {
	fm, ok := simpleFrontMatter("title: \"A title\"\ndate: 2026-08-17\ntags: [#DarkMode, #Linux, 2013]\ndraft: false\n")
	if !ok {
		t.Fatal("rejected hand-written front matter")
	}
	if fm["title"] != "A title" {
		t.Errorf("title = %q", fm["title"])
	}
	tags, _ := fm["tags"].([]any)
	if len(tags) != 3 || tags[0] != "#DarkMode" || tags[2] != "2013" {
		t.Errorf("tags = %v", fm["tags"])
	}
	if fm["draft"] != false {
		t.Errorf("draft = %v", fm["draft"])
	}
	if _, ok := simpleFrontMatter("sections:\n  - hero:\n      heading: x\n"); ok {
		t.Error("accepted nested YAML; that has to go to the YAML parser and its error")
	}
}

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{
		"#SelfHosted":     "selfhosted",
		"getting started": "getting-started",
		"C++ & Rust!":     "c-rust",
		"Ünïcode":         "ünïcode",
		"  --  ":          "",
	} {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSections(t *testing.T) {
	got, err := sections([]any{"divider", map[string]any{"hero": map[string]any{"heading": "Hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Type != "divider" || got[1].Data["heading"] != "Hi" {
		t.Errorf("sections = %+v", got)
	}
	if _, err := sections([]any{map[string]any{"a": nil, "b": nil}}); err == nil {
		t.Error("accepted two sections in one list item")
	}
}

func TestEmailOffWrapsTextAddresses(t *testing.T) {
	md := NewMarkdown(MarkdownConfig{EmailOff: true}, "example.org")
	h, _ := md.Render([]byte("Mail [me](mailto:a@example.org), or press@example.org, or follow @jc@social.example.\n"))
	s := string(h)
	for _, want := range []string{
		`<!--email_off--><a href="mailto:a@example.org">me</a><!--/email_off-->`,
		`<!--email_off--><a href="mailto:press@example.org">press@example.org</a><!--/email_off-->`, // autolinked
		`<!--email_off-->@jc@social.example<!--/email_off-->`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s in %s", want, s)
		}
	}
	if strings.Count(s, "<!--email_off-->") != 3 {
		t.Errorf("addresses wrapped twice: %s", s)
	}
}

func TestFediverseHandle(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		v  string
		ok bool
	}{{"@john@linuxexpert.org", true}, {"john@linuxexpert.org", false}, {"@john", false}, {"", true}} {
		y := "name: T\nurl: https://example.org\n"
		if c.v != "" {
			y += "fediverse: \"" + c.v + "\"\n"
		}
		os.WriteFile(filepath.Join(dir, "site.yaml"), []byte(y), 0o644)
		if _, err := LoadConfig(dir); (err == nil) != c.ok {
			t.Errorf("%q: %v", c.v, err)
		}
	}
}
