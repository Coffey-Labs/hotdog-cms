package importer

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/siteyaml"
)

// MkDocs: a documentation site's pages are Markdown already, and HotDog
// reads MkDocs' own syntax (admonitions, tabs, keys, definition lists,
// attributes, Markdown in HTML), so a page comes across as written. What
// changes is where things are: pages go under content/docs/ (index.md and
// README.md become the folder's _index.md), every other file under static/,
// the nav becomes data/docs-nav.yaml, and the docs collection keeps MkDocs'
// addresses (path: / puts docs.example.org/run/sizing/ where it was).

// MkDocsOptions says what to import and where.
type MkDocsOptions struct {
	From      string // the folder holding mkdocs.yml
	SiteDir   string
	Path      string // where the docs live on the site: "/" (the default) or "/docs/"
	Overwrite bool
	// TrustHTML turns HotDog's HTML sanitizer off for the site, as MkDocs
	// has none: for docs whose every author can be trusted with scripts.
	// Without it, HTML the sanitizer removes (an inline <svg>, say) is
	// listed in the report.
	TrustHTML bool
}

// MkDocsReport is what happened.
type MkDocsReport struct {
	Pages    []string            // content/ files written
	Files    []string            // static/ files copied
	Skipped  []string            // already there
	NavItems int                 // entries in data/docs-nav.yaml
	Notes    map[string][]string // page → what didn't come across
	Settings []string            // mkdocs.yml settings HotDog doesn't use
}

type mkdocsConfig struct {
	SiteName    string    `yaml:"site_name"`
	SiteURL     string    `yaml:"site_url"`
	Description string    `yaml:"site_description"`
	Author      string    `yaml:"site_author"`
	Copyright   string    `yaml:"copyright"`
	DocsDir     string    `yaml:"docs_dir"`
	DirURLs     *bool     `yaml:"use_directory_urls"`
	Nav         yaml.Node `yaml:"nav"`
	Extensions  yaml.Node `yaml:"markdown_extensions"`
	Plugins     yaml.Node `yaml:"plugins"`
	ExtraCSS    []string  `yaml:"extra_css"`
	ExtraJS     yaml.Node `yaml:"extra_javascript"`
	Inherit     string    `yaml:"INHERIT"`
	Theme       struct {
		CustomDir string `yaml:"custom_dir"`
	} `yaml:"theme"`
}

// readMkDocsConfig reads the settings HotDog uses. mkdocs.yml can carry
// Python tags (!!python/name:…, !ENV) on settings that don't concern us, so
// only the keys we want are decoded.
func readMkDocsConfig(file string) (*mkdocsConfig, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("mkdocs.yml: %w", err)
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("mkdocs.yml: not a settings file")
	}
	keep := &yaml.Node{Kind: yaml.MappingNode}
	m := doc.Content[0]
	for i := 0; i+1 < len(m.Content); i += 2 {
		switch m.Content[i].Value {
		case "site_name", "site_url", "site_description", "site_author", "copyright", "docs_dir",
			"use_directory_urls", "nav", "markdown_extensions", "plugins", "extra_css", "extra_javascript", "INHERIT", "theme":
			keep.Content = append(keep.Content, m.Content[i], m.Content[i+1])
		}
	}
	c := &mkdocsConfig{}
	if err := keep.Decode(c); err != nil {
		// theme: may be a bare name; try again without it.
		var noTheme yaml.Node = *keep
		noTheme.Content = nil
		for i := 0; i+1 < len(keep.Content); i += 2 {
			if keep.Content[i].Value != "theme" {
				noTheme.Content = append(noTheme.Content, keep.Content[i], keep.Content[i+1])
			}
		}
		if err := noTheme.Decode(c); err != nil {
			return nil, fmt.Errorf("mkdocs.yml: %w", err)
		}
	}
	if c.DocsDir == "" {
		c.DocsDir = "docs"
	}
	return c, nil
}

// yamlNames lists a markdown_extensions: or plugins: list's entries by name:
// each is a name, or a one-key mapping of a name to its settings.
func yamlNames(n yaml.Node) []string {
	var out []string
	for _, e := range n.Content {
		switch e.Kind {
		case yaml.ScalarNode:
			out = append(out, e.Value)
		case yaml.MappingNode:
			if len(e.Content) > 0 {
				out = append(out, e.Content[0].Value)
			}
		}
	}
	return out
}

// What HotDog reads as MkDocs does, and what it does without being asked.
var mkdocsUnderstood = map[string]bool{
	"admonition": true, "attr_list": true, "def_list": true, "footnotes": true, "md_in_html": true,
	"tables": true, "toc": true, "pymdownx.details": true, "pymdownx.keys": true, "pymdownx.tabbed": true,
	"pymdownx.tasklist": true, "pymdownx.superfences": true, "pymdownx.betterem": true, "fenced_code": true,
	"pymdownx.tilde": true, "sane_lists": true, "meta": true, "pymdownx.magiclink": true,
}

// mkdocsSource maps a file under docs_dir to its source under content/:
// index.md and README.md are a folder's own page.
func mkdocsSource(rel string) string {
	dir, name := path.Split(rel)
	if strings.EqualFold(name, "index.md") || strings.EqualFold(name, "README.md") {
		name = "_index.md"
	}
	return path.Join("docs", dir, name)
}

// FromMkDocs imports an MkDocs site into a HotDog site that has the docs
// pack.
func FromMkDocs(opt MkDocsOptions) (*MkDocsReport, error) {
	if opt.Path == "" {
		opt.Path = "/"
	}
	if !strings.HasPrefix(opt.Path, "/") || !strings.HasSuffix(opt.Path, "/") || !site.SafeURLPath(opt.Path) {
		return nil, fmt.Errorf("-path %q should be a folder on the site, like / or /docs/", opt.Path)
	}
	cfg, err := readMkDocsConfig(filepath.Join(opt.From, "mkdocs.yml"))
	if err != nil {
		return nil, err
	}
	if cfg.DirURLs != nil && !*cfg.DirURLs {
		return nil, errors.New("mkdocs.yml has use_directory_urls: false (pages at /page.html); HotDog writes /page/, so every address would change")
	}
	if _, err := os.Stat(filepath.Join(opt.SiteDir, "layouts", "doc.html")); err != nil {
		return nil, errors.New("the site has no docs pack: hotdog-cms pack add docs")
	}
	if opt.Path == "/" {
		if _, err := os.Stat(filepath.Join(opt.SiteDir, "content", "index.md")); err == nil {
			return nil, errors.New("content/index.md is where the docs' front page goes with -path /: move or delete it first, or import with -path /docs/")
		}
	}
	docs := filepath.Join(opt.From, filepath.FromSlash(cfg.DocsDir))
	rep := &MkDocsReport{Notes: map[string][]string{}}

	// Every file MkDocs would publish: dot files and folders are left out,
	// as MkDocs leaves them out, and so are links.
	var mdFiles, otherFiles []string
	files := map[string]bool{}
	err = filepath.WalkDir(docs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p != docs && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		r, _ := filepath.Rel(docs, p)
		rel := filepath.ToSlash(r)
		files[rel] = true
		if strings.HasSuffix(strings.ToLower(rel), ".md") {
			mdFiles = append(mdFiles, rel)
		} else {
			otherFiles = append(otherFiles, rel)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", docs, err)
	}
	if len(mdFiles) == 0 {
		return nil, fmt.Errorf("no pages in %s", docs)
	}
	// A folder with both index.md and README.md: MkDocs uses index.md.
	for _, rel := range mdFiles {
		dir, name := path.Split(rel)
		if strings.EqualFold(name, "README.md") && files[dir+"index.md"] {
			rep.Notes[rel] = append(rep.Notes[rel], "left out: MkDocs uses index.md in the same folder")
		}
	}

	staticRoot := strings.Trim(opt.Path, "/")
	write := func(dst string, data []byte) (bool, error) {
		full := filepath.Join(opt.SiteDir, filepath.FromSlash(dst))
		if _, err := os.Stat(full); err == nil && !opt.Overwrite {
			rep.Skipped = append(rep.Skipped, dst)
			return false, nil
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return false, err
		}
		return true, os.WriteFile(full, data, 0o644)
	}

	anyHTML := false
	for _, rel := range mdFiles {
		if len(rep.Notes[rel]) > 0 && strings.HasPrefix(rep.Notes[rel][0], "left out") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(docs, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		out, notes, hasHTML := convertMkDocsPage(rel, raw, files, opt.Path, opt.TrustHTML)
		anyHTML = anyHTML || hasHTML
		if len(notes) > 0 {
			rep.Notes[rel] = append(rep.Notes[rel], notes...)
		}
		dst := path.Join("content", mkdocsSource(rel))
		if ok, err := write(dst, out); err != nil {
			return nil, err
		} else if ok {
			rep.Pages = append(rep.Pages, dst)
		}
	}
	for _, rel := range otherFiles {
		data, err := os.ReadFile(filepath.Join(docs, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		dst := path.Join("static", staticRoot, rel)
		if ok, err := write(dst, data); err != nil {
			return nil, err
		} else if ok {
			rep.Files = append(rep.Files, dst)
		}
	}

	// The reading order.
	var nav []any
	if len(cfg.Nav.Content) > 0 {
		nav = mkdocsNav(&cfg.Nav, rep)
	} else {
		nav = navFromFiles(mdFiles)
	}
	rep.NavItems = countNav(nav)
	var navOut bytes.Buffer
	navOut.WriteString("# The docs' reading order: the sidebar, the breadcrumb and the previous and\n# next links (docs pack). Brought over from mkdocs.yml's nav.\n")
	enc := yaml.NewEncoder(&navOut)
	enc.SetIndent(2)
	if err := enc.Encode(nav); err != nil {
		return nil, err
	}
	if _, err := write("data/docs-nav.yaml", navOut.Bytes()); err != nil {
		return nil, err
	}

	if err := mkdocsSiteYAML(opt, cfg, anyHTML, rep); err != nil {
		return nil, err
	}

	// What mkdocs.yml asks for that HotDog doesn't do.
	for _, e := range yamlNames(cfg.Extensions) {
		if !mkdocsUnderstood[e] {
			rep.Settings = append(rep.Settings, "markdown_extensions: "+e)
		}
	}
	for _, p := range yamlNames(cfg.Plugins) {
		if p != "search" { // HotDog writes a search index
			rep.Settings = append(rep.Settings, "plugins: "+p)
		}
	}
	for _, c := range cfg.ExtraCSS {
		rep.Settings = append(rep.Settings, "extra_css: "+c+" (copied with the files; the site's look replaces the theme)")
	}
	for _, j := range yamlNames(cfg.ExtraJS) {
		rep.Settings = append(rep.Settings, "extra_javascript: "+j)
	}
	if cfg.Theme.CustomDir != "" {
		rep.Settings = append(rep.Settings, "theme.custom_dir: "+cfg.Theme.CustomDir+" (theme overrides are MkDocs templates)")
	}
	if cfg.Inherit != "" {
		rep.Settings = append(rep.Settings, "INHERIT: "+cfg.Inherit+" (only this file's settings were read)")
	}
	if cfg.Copyright != "" {
		rep.Settings = append(rep.Settings, "copyright: set copyright: and copyright_url: in site.yaml by hand")
	}
	return rep, nil
}

var (
	fenceRe       = regexp.MustCompile("^\\s*(```|~~~)")
	frontRe       = regexp.MustCompile(`(?s)\A---\r?\n(.*?)\r?\n(?:---|\.\.\.)\r?\n`)
	atxH1Re       = regexp.MustCompile(`^#[ \t]+(.+?)[ \t]*#*[ \t]*$`)
	attrTailRe    = regexp.MustCompile(`\s*\{[^}]*\}\s*$`)
	mdLinkRe      = regexp.MustCompile(`(\]\(\s*<?)([^)\s>]+)`)
	refDefRe      = regexp.MustCompile(`^(\s{0,3}\[[^\]]+\]:\s*<?)(\S+?)(>?(?:\s|$))`)
	htmlAttrRe    = regexp.MustCompile(`((?:src|href)=")([^"]+)(")`)
	sanitizedRe   = regexp.MustCompile(`<(svg|script|iframe|style|object|embed|form|button|canvas|math|select|textarea)\b|\s(style|on[a-z]+)=`)
	htmlTagRe     = regexp.MustCompile(`<(?:div|span|p|figure|details|table|img|a|br|section|kbd|sup|sub|small|center|video|iframe)\b`)
	schemeRe      = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)
	unsupportedRe = map[string]*regexp.Regexp{
		"snippet includes (--8<--)":              regexp.MustCompile(`--8<--`),
		"template tags ({{ … }}, {% … %})":       regexp.MustCompile(`\{\{.*\}\}|\{%.*%\}`),
		"abbreviations (*[ABC]: …)":              regexp.MustCompile(`^\*\[[^\]]+\]:`),
		"icons and emoji (:material-…:)":         regexp.MustCompile(`:(?:material|fontawesome|octicons|simple)-[a-z0-9-]+:`),
		"highlighting (==text==)":                regexp.MustCompile(`==[^=\s][^=]*==`),
		"inserted text (^^text^^)":               regexp.MustCompile(`\^\^[^^\s][^^]*\^\^`),
		"code annotations ({ .annotate })":       regexp.MustCompile(`\{\s*\.annotate\s*\}`),
		"Mermaid diagrams (shown as their code)": regexp.MustCompile("^\\s*(```|~~~)\\s*mermaid"),
	}
)

// convertMkDocsPage readies one page: a title in front matter (from the page's
// first heading, which the docs layout shows itself, or its file name), the
// layout for a folder's own page, and links to files other than pages made
// to work from the page's address. It notes what HotDog won't render.
func convertMkDocsPage(rel string, raw []byte, files map[string]bool, prefix string, trust bool) ([]byte, []string, bool) {
	src := strings.ReplaceAll(string(raw), "\r\n", "\n")
	src = strings.TrimPrefix(src, "\ufeff")
	front := ""
	if m := frontRe.FindStringSubmatch(src); m != nil {
		front = m[1]
		src = src[len(m[0]):]
	}
	var fm map[string]any
	_ = yaml.Unmarshal([]byte(front), &fm)

	lines := strings.Split(src, "\n")
	// The first heading, if the page opens with one, is its title.
	title := ""
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if m := atxH1Re.FindStringSubmatch(l); m != nil {
			title = plainTitle(attrTailRe.ReplaceAllString(m[1], ""))
			lines = lines[i+1:]
		} else if i+1 < len(lines) && strings.TrimSpace(lines[i+1]) != "" && strings.Trim(strings.TrimSpace(lines[i+1]), "=") == "" {
			title = plainTitle(attrTailRe.ReplaceAllString(l, ""))
			lines = lines[i+2:]
		}
		break
	}
	if t, ok := fm["title"].(string); ok && t != "" {
		title = t
	}
	if title == "" {
		base := strings.TrimSuffix(path.Base(rel), path.Ext(rel))
		if strings.EqualFold(base, "index") || strings.EqualFold(base, "readme") {
			base = path.Base(path.Dir(rel))
			if base == "." {
				base = "Home"
			}
		}
		title = strings.ReplaceAll(strings.ReplaceAll(base, "-", " "), "_", " ")
		title = strings.ToUpper(title[:1]) + title[1:]
	}

	notes := map[string]int{}
	hasHTML := false
	dir := path.Dir(rel)
	// A link to a file in the docs (a picture, a download) is written from
	// the page's source file; MkDocs rewrites it for the page's address,
	// and so does this, once. Links to pages (.md) are HotDog's to follow;
	// everything else is left as written.
	fix := func(target string) string {
		if target == "" || strings.HasPrefix(target, "/") || strings.HasPrefix(target, "#") || schemeRe.MatchString(target) {
			return target
		}
		file, rest := target, ""
		if i := strings.IndexAny(target, "#?"); i >= 0 {
			file, rest = target[:i], target[i:]
		}
		if strings.HasSuffix(strings.ToLower(file), ".md") {
			return target
		}
		resolved := path.Join(dir, file)
		if strings.HasPrefix(resolved, "../") || !files[resolved] {
			return target
		}
		return prefix + resolved + rest
	}
	inFence := false
	for i, l := range lines {
		if fenceRe.MatchString(l) {
			for name, re := range unsupportedRe {
				if strings.HasPrefix(name, "Mermaid") && !inFence && re.MatchString(l) {
					notes[name]++
				}
			}
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		for name, re := range unsupportedRe {
			if !strings.HasPrefix(name, "Mermaid") && re.MatchString(l) {
				notes[name]++
			}
		}
		if htmlTagRe.MatchString(l) {
			hasHTML = true
		}
		if !trust {
			for _, m := range sanitizedRe.FindAllStringSubmatch(l, -1) {
				hasHTML = true
				what := strings.ToLower(m[1])
				if what == "" {
					what = strings.TrimSpace(strings.TrimSuffix(m[0], "="))
				} else {
					what = "<" + what + ">"
				}
				notes["HTML the sanitizer removes, "+what+" (markdown.trust_html keeps it)"]++
			}
		}
		if strings.HasPrefix(strings.TrimSpace(l), "|") {
			l = escapeCodePipes(l)
		}
		l = mdLinkRe.ReplaceAllStringFunc(l, func(s string) string {
			m := mdLinkRe.FindStringSubmatch(s)
			return m[1] + fix(m[2])
		})
		l = refDefRe.ReplaceAllStringFunc(l, func(s string) string {
			m := refDefRe.FindStringSubmatch(s)
			return m[1] + fix(m[2]) + m[3]
		})
		l = htmlAttrRe.ReplaceAllStringFunc(l, func(s string) string {
			m := htmlAttrRe.FindStringSubmatch(s)
			return m[1] + fix(m[2]) + m[3]
		})
		lines[i] = l
	}

	var b strings.Builder
	b.WriteString("---\n")
	if _, ok := fm["title"]; !ok {
		t, _ := yaml.Marshal(map[string]string{"title": title})
		b.Write(t)
	}
	if strings.TrimSpace(front) != "" {
		b.WriteString(strings.TrimRight(front, "\n") + "\n")
	}
	if strings.HasSuffix(mkdocsSource(rel), "/_index.md") || mkdocsSource(rel) == "docs/_index.md" {
		// MkDocs shows a folder's own page like any other.
		if _, ok := fm["layout"]; !ok {
			b.WriteString("layout: doc\n")
		}
	}
	b.WriteString("---\n")
	body := strings.TrimLeft(strings.Join(lines, "\n"), "\n")
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteString("\n")
	}

	var out []string
	for name, n := range notes {
		out = append(out, fmt.Sprintf("%s: %d", name, n))
	}
	sort.Strings(out)
	return []byte(b.String()), out, hasHTML
}

// navEntry is one entry of data/docs-nav.yaml, its keys in reading order.
type navEntry struct {
	Title    string `yaml:"title,omitempty"`
	Page     string `yaml:"page,omitempty"`
	URL      string `yaml:"url,omitempty"`
	Children []any  `yaml:"children,omitempty"`
}

// escapeCodePipes writes a table row's | inside `code` as \|. Python-Markdown
// doesn't split a cell inside code; GitHub's tables, which HotDog reads, do
// unless the pipe is escaped, and show \| in code as |.
func escapeCodePipes(row string) string {
	var b strings.Builder
	open := 0 // the length of the backtick run that opened the span, 0 outside one
	for i := 0; i < len(row); {
		c := row[i]
		switch {
		case c == '`':
			n := 1
			for i+n < len(row) && row[i+n] == '`' {
				n++
			}
			if open == 0 {
				open = n
			} else if n == open {
				open = 0
			}
			b.WriteString(row[i : i+n])
			i += n
			continue
		case c == '\\' && open == 0 && i+1 < len(row):
			b.WriteString(row[i : i+2])
			i += 2
			continue
		case c == '|' && open > 0 && (i == 0 || row[i-1] != '\\'):
			b.WriteString("\\|")
		default:
			b.WriteByte(c)
		}
		i++
	}
	if open > 0 { // an unclosed span is no span: leave the row as it was
		return row
	}
	return b.String()
}

var (
	titleLinkRe = regexp.MustCompile(`!?\[([^\]]*)\](?:\([^)]*\)|\[[^\]]*\])`)
	titleTagRe  = regexp.MustCompile(`<[^>]*>`)
	titleMarkRe = regexp.MustCompile("`|\\*\\*|__")
	titleEscRe  = regexp.MustCompile(`\\([!-/:-@\[-` + "`" + `{-~])`)
)

// plainTitle is a heading as the words it shows, for a page's title, which
// HotDog writes as text: "ASN &amp; GeoIP" is ASN & GeoIP, and code and bold
// marks go.
func plainTitle(h string) string {
	h = titleLinkRe.ReplaceAllString(h, "$1")
	h = titleTagRe.ReplaceAllString(h, "")
	h = titleMarkRe.ReplaceAllString(h, "")
	h = titleEscRe.ReplaceAllString(h, "$1")
	return strings.TrimSpace(html.UnescapeString(h))
}

// mkdocsNav turns mkdocs.yml's nav into the docs pack's tree. Its entries
// are a page ("install.md"), a titled page ({Install: install.md}), a link
// ({Home: https://…}), or a titled section ({Install: [ … ]}); a section
// whose first entry is its folder's index page gets that page as its own,
// as MkDocs Material's navigation.indexes shows it.
func mkdocsNav(n *yaml.Node, rep *MkDocsReport) []any {
	var out []any
	for _, e := range n.Content {
		switch e.Kind {
		case yaml.ScalarNode:
			out = append(out, mkdocsSource(strings.TrimPrefix(e.Value, "/")))
		case yaml.MappingNode:
			if len(e.Content) < 2 {
				continue
			}
			title, v := e.Content[0].Value, e.Content[1]
			switch v.Kind {
			case yaml.ScalarNode:
				if schemeRe.MatchString(v.Value) || strings.HasPrefix(v.Value, "/") && !strings.HasSuffix(v.Value, ".md") {
					out = append(out, navEntry{Title: title, URL: v.Value})
				} else {
					out = append(out, navEntry{Title: title, Page: mkdocsSource(strings.TrimPrefix(v.Value, "/"))})
				}
			case yaml.SequenceNode:
				sec := navEntry{Title: title}
				kids := v
				if len(v.Content) > 0 && v.Content[0].Kind == yaml.ScalarNode {
					if first := mkdocsSource(v.Content[0].Value); strings.HasSuffix(first, "/_index.md") {
						sec.Page = first
						kids = &yaml.Node{Kind: yaml.SequenceNode, Content: v.Content[1:]}
					}
				}
				sec.Children = mkdocsNav(kids, rep)
				out = append(out, sec)
			}
		}
	}
	return out
}

// navFromFiles is MkDocs' order when mkdocs.yml has no nav: by folder, a
// folder's index first, then its pages and folders by name.
func navFromFiles(rels []string) []any {
	type folder struct {
		pages []string
		dirs  map[string]*folder
	}
	root := &folder{dirs: map[string]*folder{}}
	for _, rel := range rels {
		f := root
		parts := strings.Split(rel, "/")
		for _, d := range parts[:len(parts)-1] {
			if f.dirs[d] == nil {
				f.dirs[d] = &folder{dirs: map[string]*folder{}}
			}
			f = f.dirs[d]
		}
		f.pages = append(f.pages, rel)
	}
	var walk func(f *folder) (page string, items []any)
	walk = func(f *folder) (string, []any) {
		sort.Strings(f.pages)
		var own string
		var items []any
		for _, p := range f.pages {
			if s := mkdocsSource(p); strings.HasSuffix(s, "_index.md") && own == "" {
				own = s
			} else if !strings.HasSuffix(s, "_index.md") {
				items = append(items, s)
			}
		}
		var dirs []string
		for d := range f.dirs {
			dirs = append(dirs, d)
		}
		sort.Strings(dirs)
		for _, d := range dirs {
			page, kids := walk(f.dirs[d])
			items = append(items, navEntry{Title: strings.ToUpper(d[:1]) + strings.ReplaceAll(strings.ReplaceAll(d[1:], "-", " "), "_", " "), Page: page, Children: kids})
		}
		return own, items
	}
	home, items := walk(root)
	if home != "" {
		items = append([]any{home}, items...)
	}
	return items
}

func countNav(items []any) int {
	n := 0
	for _, it := range items {
		n++
		if e, ok := it.(navEntry); ok {
			n += countNav(e.Children)
		}
	}
	return n
}

// mkdocsKeyMap reads pymdownx.keys' key_map: names for keys.
func mkdocsKeyMap(ext *yaml.Node) map[string]string {
	for _, e := range ext.Content {
		if e.Kind != yaml.MappingNode || len(e.Content) < 2 || e.Content[0].Value != "pymdownx.keys" {
			continue
		}
		var opts struct {
			KeyMap map[string]string `yaml:"key_map"`
		}
		_ = e.Content[1].Decode(&opts)
		return opts.KeyMap
	}
	return nil
}

// mkdocsSiteYAML sets the docs collection's address, MkDocs' heading
// anchors, and HTML in Markdown if pages use it; and the site's name,
// address and description, if site.yaml still has the new site's ones.
func mkdocsSiteYAML(opt MkDocsOptions, cfg *mkdocsConfig, anyHTML bool, rep *MkDocsReport) error {
	p := filepath.Join(opt.SiteDir, "site.yaml")
	raw, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	text := string(raw)
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil || len(doc.Content) != 1 {
		return fmt.Errorf("site.yaml: %v", err)
	}
	get := func(key string, into any) {
		m := doc.Content[0]
		for i := 0; i+1 < len(m.Content); i += 2 {
			if m.Content[i].Value == key {
				_ = m.Content[i+1].Decode(into)
			}
		}
	}
	var colls map[string]map[string]any
	get("collections", &colls)
	docsColl := colls["docs"]
	if docsColl == nil {
		docsColl = map[string]any{"layout": "doc", "list_layout": "doc-index", "sort": "weight", "search": true}
	}
	if _, ok := docsColl["paginate"]; !ok {
		docsColl["paginate"] = 0 // the docs are read through the sidebar, not paged
	}
	if opt.Path != "/docs/" {
		docsColl["path"] = opt.Path
	} else {
		delete(docsColl, "path")
	}
	if text, err = siteyaml.SetEntry(text, "collections", "docs", docsColl); err != nil {
		return err
	}
	md := map[string]any{}
	get("markdown", &md)
	md["heading_ids"] = "mkdocs"
	if opt.TrustHTML {
		md["trust_html"] = true
		delete(md, "unsafe_html")
	}
	if km := mkdocsKeyMap(&cfg.Extensions); len(km) > 0 {
		keys := map[string]any{}
		if old, ok := md["keys"].(map[string]any); ok {
			keys = old
		}
		for k, v := range km {
			keys[k] = v
		}
		md["keys"] = keys
	}
	if anyHTML && md["trust_html"] != true {
		md["unsafe_html"] = true
	}
	if text, err = siteyaml.SetTopLevel(text, "markdown", md); err != nil {
		return err
	}
	var url string
	get("url", &url)
	if url == "https://example.org" || url == "https://example.org/" {
		set := func(key, v string) {
			if v != "" && err == nil {
				text, err = siteyaml.SetTopLevel(text, key, strings.TrimSpace(v))
			}
		}
		set("name", cfg.SiteName)
		set("url", strings.TrimSuffix(cfg.SiteURL, "/"))
		set("description", cfg.Description)
		set("author", cfg.Author)
		if err != nil {
			return err
		}
	} else if cfg.SiteName != "" {
		rep.Settings = append(rep.Settings, "site_name, site_url, site_description: site.yaml already has its own, so they were left as they are")
	}
	return os.WriteFile(p, []byte(text), 0o644)
}

// Markdown is the report, for IMPORT-REPORT.md.
func (r *MkDocsReport) Markdown(source string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Import from %s\n\n", source)
	fmt.Fprintf(&b, "%d page(s) and %d other file(s) written, %d already there and skipped; %d entries in data/docs-nav.yaml.\n\n", len(r.Pages), len(r.Files), len(r.Skipped), r.NavItems)
	if len(r.Settings) > 0 {
		b.WriteString("## Settings in mkdocs.yml that didn't come across\n\n")
		for _, s := range r.Settings {
			fmt.Fprintf(&b, "- %s\n", s)
		}
		b.WriteString("\n")
	}
	if len(r.Notes) > 0 {
		b.WriteString("## Pages to look at\n\nWhat these pages use that HotDog shows as plain text, or not at all:\n\n")
		var pages []string
		for p := range r.Notes {
			pages = append(pages, p)
		}
		sort.Strings(pages)
		for _, p := range pages {
			fmt.Fprintf(&b, "- `%s`: %s\n", p, strings.Join(r.Notes[p], "; "))
		}
		b.WriteString("\n")
	}
	if len(r.Skipped) > 0 {
		b.WriteString("## Skipped (already in the site; -overwrite replaces them)\n\n")
		for _, s := range r.Skipped {
			fmt.Fprintf(&b, "- %s\n", s)
		}
		b.WriteString("\n")
	}
	b.WriteString("Then: `hotdog-cms check`, which also finds links to pages that didn't come across.\n")
	return b.String()
}
