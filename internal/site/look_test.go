package site

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestStarterLook(t *testing.T) {
	lf, err := LoadLook(filepath.Join("..", "..", "starter", "site"))
	if err != nil || lf == nil {
		t.Fatalf("starter look.yaml: %v", err)
	}
	// The starter's own colors pass its own contrast rules.
	vals, err := lf.Values(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range lf.Contrasts(vals) {
		if r.Light < MinContrast || r.Dark < MinContrast {
			t.Errorf("%s on %s: %.2f light, %.2f dark", r.Fore, r.Back, r.Light, r.Dark)
		}
	}
	if lf.CSS(vals) != nil {
		t.Error("no choices should mean no stylesheet")
	}

	vals, err = lf.Values(map[string]any{
		"accent": "#0a7d55",
		"text":   map[string]any{"light": "#111111", "dark": "#f0f0f0"},
		"font":   "serif",
		"radius": "sharp",
		"logo":   "/media/2026/10/logo.png",
	})
	if err != nil {
		t.Fatal(err)
	}
	css := string(lf.CSS(vals))
	for _, want := range []string{
		":root { --accent: #0a7d55; --text: #111111; --font: Charter, \"Bitstream Charter\"",
		"--radius: 4px; }",
		`@media (prefers-color-scheme: dark) { :root:not([data-theme="light"]) { --text: #f0f0f0; } }`,
		`:root[data-theme="dark"] { --text: #f0f0f0; }`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("CSS missing %q:\n%s", want, css)
		}
	}
	if strings.Contains(css, "logo") || strings.Contains(css, "--accent: #f2a65a") {
		t.Errorf("CSS has what it shouldn't:\n%s", css)
	}
	if lf.Get(vals, "logo") != "/media/2026/10/logo.png" || lf.Get(vals, "band") != "#1d2b3a" || lf.Get(vals, "font") != "serif" {
		t.Error("Get")
	}
	// The dark-mode accent is still the theme's; light uses the choice.
	for _, r := range lf.Contrasts(vals) {
		if r.Fore == "accent" && (r.Light != Contrast("#0a7d55", "#fbfaf7") || r.Dark != Contrast("#f2a65a", "#12161c")) {
			t.Errorf("accent contrast: %+v", r)
		}
	}

	_, err = lf.Values(map[string]any{"accent": "green", "font": "comic", "shadow": "big", "radius": map[string]any{"light": "x"}, "logo": "javascript:alert(1)"})
	for _, want := range []string{`"green" isn't a color`, `"comic" isn't one of inter, system, serif, rounded`, "the theme has no shadow", "only a color has light and dark", "should be an address on the site"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
}

func TestContrast(t *testing.T) {
	if Contrast("#000", "#fff") != 21 || Contrast("#fff", "#fff") != 1 {
		t.Error("extremes")
	}
}
