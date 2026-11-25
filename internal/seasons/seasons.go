// Package seasons dresses a site for holidays through the year: a strip along
// the top of the page, a prop on the logo and a greeting, worked out in each
// visitor's browser from their own date every time a page loads. Nothing is
// scheduled and nothing needs a rebuild when a holiday starts or ends.
//
// The script is src/core.js (the dates), src/render.js (the drawings) and
// src/run.js (the page), wrapped here; src/season.css is its stylesheet and
// props/ holds a drawing of each prop on its own. List a year's dates with
// `TZ=UTC node internal/seasons/src/dates-test.js 2026 2027`.
package seasons

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"regexp"
	"sort"
	"strings"
)

//go:embed src/core.js src/render.js src/run.js src/season.css
var src embed.FS

//go:embed props/*.svg
var propFS embed.FS

// Script is the browser script, Style its stylesheet, and Props each prop's
// drawing (an SVG laid over the site's logo), by prop name.
var (
	Script []byte
	Style  []byte
	Props  = map[string][]byte{}
)

// IDs are the holidays and December observances a site can name in only:
// and except:, in the order the script checks them.
var IDs []string

const header = `/* HotDog CMS seasons: holidays through the year, worked out on the
 * visitor's own clock every time a page loads (internal/seasons).
 * Preview: ?season=<holiday id>, ?season=YYYY-MM-DD, ?season=off,
 * ?season=auto; each lasts for the browser tab's session. */
`

// boot reads the script tag's data-seasons (see Tags) and starts.
const boot = `
  var me = document.currentScript, c = {};
  try { c = JSON.parse(me.getAttribute('data-seasons')) || {}; } catch (e) {}
  var props = c.props || {};
  run({ swap: c.art || [], anchor: c.anchor, skip: c.skip, overlay: c.logo,
    overlaySrc: function (p) { return props[p]; }, only: c.only, except: c.except,
    greet: c.greetings !== false, css: false });
`

// overlayCSS places each kind of prop on the logo it's laid over, in
// proportion to the logo (drawn for a square one). A site's own stylesheet can
// move them: .season-prop-head, -eyes, -ear, -pin, -side.
const overlayCSS = `.season-prop{z-index:1;height:auto}
.season-prop-head{left:0;top:-31%;width:110%;transform:rotate(-12deg)}
.season-prop-eyes{left:5%;top:16.5%;width:95%}
.season-prop-ear{left:62%;top:-13%;width:47.5%}
.season-prop-pin{left:60%;top:62%;width:47.5%}
.season-prop-side{left:-55%;top:-7.5%;width:47.5%}
`

var idRe = regexp.MustCompile(`\bid: '([a-z-]+)'`)

func init() {
	read := func(name string) string {
		b, err := src.ReadFile(name)
		if err != nil {
			panic(err)
		}
		return string(b)
	}
	core, render, run := read("src/core.js"), read("src/render.js"), read("src/run.js")
	Style = []byte(read("src/season.css") + overlayCSS)
	// The stylesheet is linked, not injected (css: false), so the script's
	// own copy stays empty.
	render = strings.Replace(render, "  // CSS (the strip's look) comes from season.css, put in by the build.\n", "  var CSS = '';\n", 1)
	Script = []byte(header + "(function () {\n" + core + render + run + boot + "})();\n")

	seen := map[string]bool{}
	for _, m := range idRe.FindAllStringSubmatch(core, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			IDs = append(IDs, m[1])
		}
	}
	entries, err := propFS.ReadDir("props")
	if err != nil {
		panic(err)
	}
	for _, e := range entries {
		b, err := propFS.ReadFile("props/" + e.Name())
		if err != nil {
			panic(err)
		}
		Props[strings.TrimSuffix(e.Name(), ".svg")] = b
	}
}

// PropNames lists the props, sorted: the names a site's own art is drawn for.
func PropNames() []string {
	names := make([]string, 0, len(Props))
	for n := range Props {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Known reports whether id names a holiday or an observance.
func Known(id string) bool {
	for _, k := range IDs {
		if k == id {
			return true
		}
	}
	return false
}

// Options is what Tags needs from site.yaml's seasons: block, plus the
// addresses of the fingerprinted files.
type Options struct {
	Anchor, Logo, Skip string
	Art                [][2]string // [part of an image's address, address with {p} for the prop]
	Only, Except       []string
	Greetings          bool
	ScriptURL, CSSURL  string
	PropURLs           map[string]string
}

// Tags is the stylesheet link and the script tag for a page's head.
func Tags(o Options) (template.HTML, error) {
	cfg := map[string]any{"props": o.PropURLs}
	if o.Anchor != "" {
		cfg["anchor"] = o.Anchor
	}
	if o.Logo != "" {
		cfg["logo"] = o.Logo
	}
	if o.Skip != "" {
		cfg["skip"] = o.Skip
	}
	if len(o.Art) > 0 {
		cfg["art"] = o.Art
	}
	if len(o.Only) > 0 {
		cfg["only"] = o.Only
	}
	if len(o.Except) > 0 {
		cfg["except"] = o.Except
	}
	if !o.Greetings {
		cfg["greetings"] = false
	}
	j, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("seasons: %w", err)
	}
	esc := template.HTMLEscapeString
	return template.HTML(`<link rel="stylesheet" href="` + esc(o.CSSURL) + `">` +
		`<script src="` + esc(o.ScriptURL) + `" defer data-seasons="` + esc(string(j)) + `"></script>`), nil
}
