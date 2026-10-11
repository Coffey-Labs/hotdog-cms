package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fieldMap(fs []Field, prefix string, out map[string]string) map[string]string {
	for _, f := range fs {
		out[prefix+f.Name] = f.Type
		fieldMap(f.Fields, prefix+f.Name+".", out)
	}
	return out
}

func TestSectionLibrary(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(body), 0o644)
	}
	write("sections/hero.html", `{{/* section
label: Big hero
about: The opening.
fields:
  - { name: heading, required: true }
  - name: buttons
    type: list
    fields:
      - { name: href, type: link }
*/}}<h1>{{ .Data.heading }}</h1>`)
	write("sections/broken.html", `{{/* section
fields:
  - { name: x, type: colour }
*/}}`)
	// No declaration: read from what it uses, its partial included.
	write("sections/feature.html", `{{ $d := .Data }}{{ $n := default 4 $d.count }}
{{ if .Data.wide }}<div class="wide">{{ end }}
{{ template "head" .Data }}
{{ with $d.text }}{{ markdownify . }}{{ end }}
{{ range .Data.items }}<a href="{{ .href }}">{{ .title }}</a>{{ if .new }}!{{ end }}{{ end }}
{{ range $i, $l := split "\n" .Data.lines }}{{ $l }}{{ end }}
{{ with .Data.cta }}<a href="{{ .url }}">{{ .label }}</a>{{ end }}
<img src="{{ .Data.image }}"> {{ .Page.Title }} {{ $.Site.Config.Name }}`)
	write("partials/head.html", `{{ with .eyebrow }}{{ . }}{{ end }}{{ if .heading }}<h2>{{ .heading }}</h2>{{ end }}`)

	lib, err := SectionLibrary(dir)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]SectionInfo{}
	for _, s := range lib {
		by[s.Name] = s
	}
	hero := by["hero"]
	if !hero.Declared || hero.Label != "Big hero" || hero.About != "The opening." || len(hero.Fields) != 2 || hero.Fields[0].Type != "text" || !hero.Fields[0].Required || hero.Fields[1].Fields[0].Type != "link" {
		t.Errorf("hero: %+v", hero)
	}
	if b := by["broken"]; b.Declared || !strings.Contains(b.Problem, "colour") {
		t.Errorf("broken: %+v", b)
	}
	got := fieldMap(by["feature"].Fields, "", map[string]string{})
	want := map[string]string{
		"count": "number", "wide": "bool", "eyebrow": "text", "heading": "text", "text": "markdown",
		"items": "list", "items.href": "link", "items.title": "text", "items.new": "bool",
		"lines": "text", "cta": "group", "cta.url": "link", "cta.label": "text", "image": "image",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: %q, want %q", k, got[k], v)
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("unexpected field %s (%s)", k, got[k])
		}
	}
	if by["feature"].Declared || by["feature"].Label != "Feature" {
		t.Errorf("feature: %+v", by["feature"])
	}
}

func TestStarterSectionsDeclared(t *testing.T) {
	lib, err := SectionLibrary(filepath.Join("..", "..", "starter", "site"))
	if err != nil {
		t.Fatal(err)
	}
	if len(lib) == 0 {
		t.Fatal("no sections")
	}
	for _, s := range lib {
		if !s.Declared || s.Problem != "" {
			t.Errorf("%s: declared %v, problem %q", s.Name, s.Declared, s.Problem)
		}
	}
}
