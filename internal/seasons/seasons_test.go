package seasons

import (
	"regexp"
	"strings"
	"testing"
)

func TestScriptAndProps(t *testing.T) {
	s := string(Script)
	for _, want := range []string{"function pick(", "function run(", "data-seasons", "var CSS = '';"} {
		if !strings.Contains(s, want) {
			t.Errorf("script lacks %q", want)
		}
	}
	if strings.Contains(s, "comes from season.css") {
		t.Error("the stylesheet placeholder was left in")
	}
	for _, id := range []string{"advent", "lunarnewyear", "diwali", "pride", "hanukkah", "christmas", "kwanzaa"} {
		if !Known(id) {
			t.Errorf("%s isn't a known holiday", id)
		}
	}
	if Known("arbor") {
		t.Error("made-up holiday known")
	}
	// Every prop a holiday wears has a drawing, and the drawings are plain.
	for _, p := range regexpAll(`prop: '([a-z]+)'`, s) {
		if Props[p] == nil {
			t.Errorf("no drawing for the %s prop", p)
		}
	}
	for name, svg := range Props {
		low := strings.ToLower(string(svg))
		if strings.Contains(low, "<script") || strings.Contains(low, " on") || strings.Contains(low, "href") {
			t.Errorf("%s.svg has script, a handler or a link", name)
		}
	}
	if !strings.Contains(string(Style), ".season-lights{") || !strings.Contains(string(Style), ".season-prop-head{") {
		t.Error("stylesheet incomplete")
	}
}

func TestTags(t *testing.T) {
	h, err := Tags(Options{Anchor: "header", Logo: ".brand-logo", Except: []string{"halloween"},
		ScriptURL: "/s.js", CSSURL: "/s.css", PropURLs: map[string]string{"santa": "/p/santa.svg"}})
	if err != nil {
		t.Fatal(err)
	}
	got := string(h)
	for _, want := range []string{`<link rel="stylesheet" href="/s.css">`, `<script src="/s.js" defer data-seasons="`,
		`&#34;except&#34;:[&#34;halloween&#34;]`, `&#34;greetings&#34;:false`, `&#34;logo&#34;:&#34;.brand-logo&#34;`} {
		if !strings.Contains(got, want) {
			t.Errorf("tags lack %s:\n%s", want, got)
		}
	}
}

func regexpAll(pat, s string) []string {
	var out []string
	for _, m := range regexp.MustCompile(pat).FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	return out
}
