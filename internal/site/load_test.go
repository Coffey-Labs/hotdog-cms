package site

import "testing"

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
