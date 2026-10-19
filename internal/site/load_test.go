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

func TestCSVData(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "prices.csv"), []byte("\xef\xbb\xbfItem,Price\n\"Rye, dark\",$5\n"), 0o644)
	data, err := loadData(dir)
	if err != nil {
		t.Fatal(err)
	}
	rows, ok := data["prices"].([]any)
	if !ok || len(rows) != 2 {
		t.Fatalf("prices: %#v", data["prices"])
	}
	if head := rows[0].([]any); head[0] != "Item" || head[1] != "Price" {
		t.Errorf("header (byte order mark kept?): %#v", head)
	}
	if row := rows[1].([]any); row[0] != "Rye, dark" || row[1] != "$5" {
		t.Errorf("row: %#v", row)
	}
	os.WriteFile(filepath.Join(dir, "prices.csv"), []byte("a,b\nonly one\n"), 0o644)
	if _, err := loadData(dir); err == nil || !strings.Contains(err.Error(), "data/prices.csv") {
		t.Errorf("a row short of cells: %v", err)
	}
}

func TestDataFolders(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "exams", "old"), 0o755)
	os.MkdirAll(filepath.Join(dir, ".cache"), 0o755)
	os.WriteFile(filepath.Join(dir, "team.yaml"), []byte("- Ana\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "exams", "one.yaml"), []byte("questions: 60\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "exams", "two.json"), []byte(`{"questions": 40}`), 0o644)
	os.WriteFile(filepath.Join(dir, "exams", "old", "zero.yaml"), []byte("questions: 10\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "exams", "notes.txt"), []byte("not data"), 0o644)
	os.WriteFile(filepath.Join(dir, ".cache", "x.yaml"), []byte(": not yaml"), 0o644)
	data, err := loadData(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"team", "exams/one", "exams/two", "exams/old/zero"} {
		if data[key] == nil {
			t.Errorf("%s not read", key)
		}
	}
	if len(data) != 4 {
		t.Errorf("read %d files, want 4: %v", len(data), data)
	}
	s := &Site{Data: data}
	folder := s.DataFolder("exams")
	if len(folder) != 2 || folder["one"] == nil || folder["two"] == nil {
		t.Errorf("exams folder: %v", folder)
	}
	if s.DataFolder("team") != nil || s.DataFolder("nothing") != nil {
		t.Error("a file or a missing name read as a folder")
	}
	// data/exams.yaml beside data/exams/ would make "exams" mean two things.
	os.WriteFile(filepath.Join(dir, "exams.yaml"), []byte("a: 1\n"), 0o644)
	if _, err := loadData(dir); err == nil || !strings.Contains(err.Error(), "data/exams/") {
		t.Errorf("a file and a folder with one name: %v", err)
	}
	os.Remove(filepath.Join(dir, "exams.yaml"))
	os.WriteFile(filepath.Join(dir, "exams", "one.json"), []byte(`{}`), 0o644)
	if _, err := loadData(dir); err == nil || !strings.Contains(err.Error(), "exams/one") {
		t.Errorf("two files named exams/one: %v", err)
	}
}
