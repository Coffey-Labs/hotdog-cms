package site

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// The look of a site: what its theme lets people change without touching CSS
// (colors, fonts, corners, the logo), and inside what limits. The theme
// declares that in look.yaml; a site's choices go in site.yaml under look:.
// The build writes the choices into a small stylesheet of CSS custom
// properties, loaded after the theme's own, so no page needs an inline style.
//
//	# look.yaml
//	tokens:
//	  - { name: accent, label: Accent, type: color, default: "#c2541b", dark: "#f2a65a" }
//	  - name: font
//	    type: font
//	    default: system
//	    options: { system: "system-ui, sans-serif", serif: "Georgia, serif" }
//	  - { name: logo, type: image }
//	contrast:
//	  - [text, bg]      # pairs that have to stay readable: 4.5:1, light and dark
//
//	# site.yaml
//	look:
//	  accent: "#0a7d55"                            # light only; dark keeps the theme's
//	  text: { light: "#1b1b1b", dark: "#f0f0f0" }
//	  font: serif
//	  logo: /media/2026/10/logo.png

// LookFile is a theme's look.yaml.
type LookFile struct {
	Tokens   []LookToken `yaml:"tokens" json:"tokens"`
	Contrast [][]string  `yaml:"contrast" json:"contrast"`
}

// LookToken is one thing the look can change.
type LookToken struct {
	Name    string      `yaml:"name" json:"name"`
	Label   string      `yaml:"label" json:"label,omitempty"`
	Type    string      `yaml:"type" json:"type"` // color, font, select, image
	Help    string      `yaml:"help" json:"help,omitempty"`
	Default string      `yaml:"default" json:"default,omitempty"`
	Dark    string      `yaml:"dark" json:"dark,omitempty"` // a color's default in dark mode
	Options LookOptions `yaml:"options" json:"options,omitempty"`
}

// LookOption is one choice of a font or select token: what the editor shows,
// and the CSS value it stands for.
type LookOption struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// LookOptions keeps the order the theme wrote its options in.
type LookOptions []LookOption

func (o *LookOptions) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("options must be a map of name: CSS value")
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		*o = append(*o, LookOption{Name: n.Content[i].Value, Value: n.Content[i+1].Value})
	}
	return nil
}

func (o LookOptions) value(name string) (string, bool) {
	for _, x := range o {
		if x.Name == name {
			return x.Value, true
		}
	}
	return "", false
}

func (o LookOptions) names() string {
	var n []string
	for _, x := range o {
		n = append(n, x.Name)
	}
	return strings.Join(n, ", ")
}

// LookValue is a site's choice for one token. Dark is empty when the site
// doesn't set a separate dark-mode value.
type LookValue struct {
	Light string `json:"light"`
	Dark  string `json:"dark,omitempty"`
}

var (
	tokenName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	hexColor  = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)
)

// LoadLook reads a site's look.yaml. A site without one has no look to
// change, and gets nil.
func LoadLook(dir string) (*LookFile, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "look.yaml"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var lf LookFile
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&lf); err != nil {
		return nil, fmt.Errorf("look.yaml: %w", err)
	}
	seen := map[string]*LookToken{}
	for i := range lf.Tokens {
		t := &lf.Tokens[i]
		if !tokenName.MatchString(t.Name) || seen[t.Name] != nil {
			return nil, fmt.Errorf("look.yaml: tokens[%d]: needs a name of lowercase letters, digits and dashes, used once", i)
		}
		seen[t.Name] = t
		switch t.Type {
		case "color":
			if !hexColor.MatchString(t.Default) || (t.Dark != "" && !hexColor.MatchString(t.Dark)) {
				return nil, fmt.Errorf("look.yaml: %s: a color needs a default (and any dark) like #c2541b", t.Name)
			}
		case "font", "select":
			if len(t.Options) == 0 {
				return nil, fmt.Errorf("look.yaml: %s: needs options", t.Name)
			}
			if _, ok := t.Options.value(t.Default); t.Default != "" && !ok {
				return nil, fmt.Errorf("look.yaml: %s: default %q isn't one of %s", t.Name, t.Default, t.Options.names())
			}
		case "image":
		default:
			return nil, fmt.Errorf("look.yaml: %s: type %q isn't color, font, select or image", t.Name, t.Type)
		}
		if t.Label == "" {
			t.Label = strings.ToUpper(t.Name[:1]) + strings.ReplaceAll(t.Name[1:], "-", " ")
		}
	}
	for i, pair := range lf.Contrast {
		if len(pair) != 2 || seen[pair[0]] == nil || seen[pair[1]] == nil || seen[pair[0]].Type != "color" || seen[pair[1]].Type != "color" {
			return nil, fmt.Errorf("look.yaml: contrast[%d]: a pair of two color tokens", i)
		}
	}
	return &lf, nil
}

// Values reads a site's look: from site.yaml against the theme's tokens.
// Anything that doesn't fit is reported, in words.
func (lf *LookFile) Values(look map[string]any) (map[string]LookValue, error) {
	out := map[string]LookValue{}
	var problems []string
	keys := make([]string, 0, len(look))
	for k := range look {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t := lf.token(k)
		if t == nil {
			problems = append(problems, fmt.Sprintf("look.%s: the theme has no %s (look.yaml)", k, k))
			continue
		}
		var v LookValue
		switch x := look[k].(type) {
		case string:
			v.Light = strings.TrimSpace(x)
		case map[string]any:
			if t.Type != "color" {
				problems = append(problems, fmt.Sprintf("look.%s: only a color has light and dark values", k))
				continue
			}
			v.Light, _ = x["light"].(string)
			v.Dark, _ = x["dark"].(string)
		default:
			problems = append(problems, fmt.Sprintf("look.%s: expected text", k))
			continue
		}
		switch t.Type {
		case "color":
			for _, c := range []string{v.Light, v.Dark} {
				if c != "" && !hexColor.MatchString(c) {
					problems = append(problems, fmt.Sprintf("look.%s: %q isn't a color like #c2541b", k, c))
				}
			}
		case "font", "select":
			if _, ok := t.Options.value(v.Light); !ok {
				problems = append(problems, fmt.Sprintf("look.%s: %q isn't one of %s", k, v.Light, t.Options.names()))
			}
		case "image":
			if v.Light != "" && !strings.HasPrefix(v.Light, "/") && !strings.HasPrefix(v.Light, "https://") {
				problems = append(problems, fmt.Sprintf("look.%s: %q should be an address on the site (/media/…) or https://", k, v.Light))
			}
		}
		out[k] = v
	}
	if len(problems) > 0 {
		return out, fmt.Errorf("site.yaml: %s", strings.Join(problems, "; "))
	}
	return out, nil
}

func (lf *LookFile) token(name string) *LookToken {
	for i := range lf.Tokens {
		if lf.Tokens[i].Name == name {
			return &lf.Tokens[i]
		}
	}
	return nil
}

// Get is a token's value as templates see it: the site's choice, or the
// theme's default. A font or select gives its option's name.
func (lf *LookFile) Get(vals map[string]LookValue, name string) string {
	if v, ok := vals[name]; ok && v.Light != "" {
		return v.Light
	}
	if t := lf.token(name); t != nil {
		return t.Default
	}
	return ""
}

// CSS is the stylesheet for a site's choices: only what it changes, so the
// theme's own values stand for the rest. Empty when nothing is changed. Dark
// values use the selectors themes follow for dark mode: the system setting
// unless the reader picked light, or the reader picking dark.
func (lf *LookFile) CSS(vals map[string]LookValue) []byte {
	var light, dark []string
	for _, t := range lf.Tokens {
		v, ok := vals[t.Name]
		if !ok {
			continue
		}
		css := func(s string) string {
			if t.Type == "font" || t.Type == "select" {
				s, _ = t.Options.value(s)
			}
			return s
		}
		switch t.Type {
		case "image":
			continue
		case "color":
			if v.Light != "" {
				light = append(light, fmt.Sprintf("--%s: %s;", t.Name, v.Light))
			}
			if v.Dark != "" {
				dark = append(dark, fmt.Sprintf("--%s: %s;", t.Name, v.Dark))
			}
		default:
			if v.Light != "" {
				light = append(light, fmt.Sprintf("--%s: %s;", t.Name, css(v.Light)))
			}
		}
	}
	if len(light)+len(dark) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString("/* The site's look, from site.yaml (look:). Written by HotDog CMS. */\n")
	if len(light) > 0 {
		fmt.Fprintf(&b, ":root { %s }\n", strings.Join(light, " "))
	}
	if len(dark) > 0 {
		d := strings.Join(dark, " ")
		fmt.Fprintf(&b, "@media (prefers-color-scheme: dark) { :root:not([data-theme=\"light\"]) { %s } }\n:root[data-theme=\"dark\"] { %s }\n", d, d)
	}
	return []byte(b.String())
}

// ContrastResult is how readable one pair is, in each mode.
type ContrastResult struct {
	Fore  string  `json:"fore"`
	Back  string  `json:"back"`
	Light float64 `json:"light"`
	Dark  float64 `json:"dark"`
}

// MinContrast is WCAG AA for body text.
const MinContrast = 4.5

// Contrasts works out every declared pair with the site's choices applied.
func (lf *LookFile) Contrasts(vals map[string]LookValue) []ContrastResult {
	color := func(name string, dark bool) string {
		t := lf.token(name)
		v := vals[name]
		if dark {
			for _, c := range []string{v.Dark, t.Dark, v.Light, t.Default} {
				if c != "" {
					return c
				}
			}
		}
		if v.Light != "" {
			return v.Light
		}
		return t.Default
	}
	var out []ContrastResult
	for _, p := range lf.Contrast {
		out = append(out, ContrastResult{Fore: p[0], Back: p[1],
			Light: Contrast(color(p[0], false), color(p[1], false)),
			Dark:  Contrast(color(p[0], true), color(p[1], true))})
	}
	return out
}

// Contrast is the WCAG contrast ratio of two #rgb or #rrggbb colors.
func Contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return math.Round((la+0.05)/(lb+0.05)*100) / 100
}

func luminance(hex string) float64 {
	h := strings.TrimPrefix(hex, "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) != 6 {
		return 0
	}
	var c [3]float64
	for i := 0; i < 3; i++ {
		v, _ := strconv.ParseUint(h[i*2:i*2+2], 16, 8)
		x := float64(v) / 255
		if x <= 0.03928 {
			c[i] = x / 12.92
		} else {
			c[i] = math.Pow((x+0.055)/1.055, 2.4)
		}
	}
	return 0.2126*c[0] + 0.7152*c[1] + 0.0722*c[2]
}
