package site

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"gopkg.in/yaml.v3"
)

// Site is everything the templates can see.
type Site struct {
	Dir         string
	Config      *Config
	Pages       []*Page            // every page, in path order
	Collections map[string][]*Page // collection -> its pages, in the collection's order
	Lists       map[string]*Page   // collection -> its list page
	Taxonomies  map[string]map[string][]*Page
	Data        map[string]any // data/*.yaml, data/*.json and data/*.csv, by file name without extension

	bySource  map[string]*Page // PageFrom's index, built on first use
	BuildTime time.Time
	Dev       bool // built by `hotdog-cms serve`
	// NextScheduled is the earliest moment a rebuild will change the site
	// without a new commit: a publish_at still in the future, or an event
	// ending (it leaves the upcoming lists).
	NextScheduled time.Time
	// Now is the time the build judges against: scheduled pages, upcoming
	// events. Location is the site's time zone.
	Now      time.Time
	Location *time.Location
	// Banners are the notices from data/banners.yaml (banner.go).
	Banners  []Banner
	Markdown *Markdown
	terms    map[string]*termIndex
	termDocs map[string]map[string]*Page // taxonomy -> term ("" for the index) -> content/<taxonomy>/ page
}

// Options change what Load includes.
type Options struct {
	Drafts bool   // include pages with draft: true, and scheduled pages before their time
	URL    string // replace the configured URL (local preview)
	Dev    bool
	Now    time.Time // the time scheduled pages are judged against; default now
	// Overrides replaces the content of source files (paths under content/,
	// such as "about.md"), or adds files that don't exist yet: a page being
	// edited, before it is saved.
	Overrides map[string][]byte
	// Skip leaves these source files out (a page about to be moved).
	Skip map[string]bool
	// Look, if not nil, replaces site.yaml's look: (the Look panel's preview).
	Look map[string]any
}

// Load reads a whole site directory.
func Load(dir string, opt Options) (*Site, error) {
	if err := NoLinks(dir); err != nil {
		return nil, err
	}
	cfg, err := LoadConfig(dir)
	if err != nil {
		return nil, err
	}
	if opt.Look != nil {
		cfg.Look = opt.Look
	}
	host := ""
	if u, err := url.Parse(cfg.URL); err == nil {
		host = u.Hostname() // the real site's host, even in a preview
	}
	if opt.URL != "" {
		cfg.URL = strings.TrimRight(opt.URL, "/")
	}
	s := &Site{
		Dir:         dir,
		Config:      cfg,
		Collections: map[string][]*Page{},
		Lists:       map[string]*Page{},
		Taxonomies:  map[string]map[string][]*Page{},
		BuildTime:   time.Now().UTC(),
		Dev:         opt.Dev,
	}
	now := opt.Now
	if now.IsZero() {
		now = time.Now()
	}
	if s.Location, err = cfg.Location(); err != nil {
		return nil, err
	}
	s.Now = now
	if s.Banners, err = LoadBanners(dir, s.Location); err != nil {
		return nil, err
	}
	for _, b := range s.Banners {
		s.schedule(b.Start, now)
		s.schedule(b.End, now)
	}
	if s.Data, err = loadData(filepath.Join(dir, "data")); err != nil {
		return nil, err
	}
	md := NewMarkdown(cfg.Markdown, host)
	md.media = LoadMedia(filepath.Join(dir, "static"))
	s.Markdown = md
	content := filepath.Join(dir, "content")
	seen := map[string]bool{}
	// add takes one source file into the site, unless it is a draft or a
	// page scheduled for later and this build leaves those out.
	add := func(raw []byte, rel string) error {
		pg, err := loadPage(raw, rel, md)
		if err != nil {
			return fmt.Errorf("content/%s: %w", rel, err)
		}
		if pg.Draft && !opt.Drafts {
			return nil
		}
		if err := pg.resolveEvent(s.Location); err != nil {
			return fmt.Errorf("content/%s: %w", rel, err)
		}
		if pg.Event != nil {
			s.schedule(pg.Event.Ends(), now)
		}
		if !pg.PublishAt.IsZero() && pg.PublishAt.After(now) {
			s.schedule(pg.PublishAt, now)
			if !opt.Drafts {
				return nil
			}
		}
		pg.Site = s
		s.Pages = append(s.Pages, pg)
		return nil
	}
	err = filepath.WalkDir(content, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		r, _ := filepath.Rel(content, p)
		rel := filepath.ToSlash(r)
		if opt.Skip[rel] {
			return nil
		}
		if o, ok := opt.Overrides[rel]; ok {
			seen[rel] = true
			return add(o, rel)
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return add(raw, rel)
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no content/ folder in %s", dir)
		}
		return nil, err
	}
	// Overrides for files that don't exist yet: a new page, not yet saved.
	for rel, raw := range opt.Overrides {
		if seen[rel] {
			continue
		}
		clean := path.Clean("/" + rel)[1:]
		if clean != rel || !strings.HasSuffix(rel, ".md") {
			return nil, fmt.Errorf("override %q is not a page source", rel)
		}
		if err := add(raw, rel); err != nil {
			return nil, err
		}
	}
	s.placeCollections()
	s.resolveSourceLinks()
	s.assemble()
	if err := s.checkPaths(); err != nil {
		return nil, err
	}
	return s, nil
}

// ReadFile reads a file inside the site folder: rel is cleaned so it can't
// climb out ("../../etc/passwd" reads <site>/etc/passwd), it must be a
// regular file (not a link, not /dev/zero), and it may not be larger than
// max bytes.
func ReadFile(siteDir, rel string, max int64) ([]byte, error) {
	clean := strings.TrimPrefix(path.Clean("/"+filepath.ToSlash(rel)), "/")
	if clean == "" {
		return nil, fmt.Errorf("%q isn't a file in the site", rel)
	}
	root, err := os.OpenRoot(siteDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	fi, err := root.Lstat(filepath.FromSlash(clean))
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s isn't a regular file", clean)
	}
	if fi.Size() > max {
		return nil, fmt.Errorf("%s is larger than %d MB", clean, max>>20)
	}
	f, err := root.Open(filepath.FromSlash(clean))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, max+1))
}

// NoLinks refuses a site that holds a symbolic link anywhere: a link
// committed to a site (static/x -> /etc/passwd, layouts/y -> ~/.ssh/id_ed25519)
// would let anyone who can write to the repository make a build read, and
// publish, files from the machine it runs on. The site's own folder may be
// reached through a link; nothing inside it may be one.
func NoLinks(dir string) error {
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return err
	}
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() {
			switch filepath.ToSlash(rel) {
			case ".git", "public", "node_modules":
				return fs.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symbolic link; HotDog CMS doesn't follow links in a site, since they can reach files outside it. Put the file itself there", filepath.ToSlash(rel))
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s isn't a regular file", filepath.ToSlash(rel))
		}
		return nil
	})
}

// schedule notes a moment the site changes by itself, if it's still ahead.
func (s *Site) schedule(t, now time.Time) {
	if t.After(now) && (s.NextScheduled.IsZero() || t.Before(s.NextScheduled)) {
		s.NextScheduled = t
	}
}

var leadImageRe = regexp.MustCompile(`\A\s*!\[([^\]]*)\]\(([^)\s]+)\)`)

var frontMatterRe = regexp.MustCompile(`(?s)\A---\r?\n(.*?)\r?\n---\r?\n?`)

func loadPage(raw []byte, rel string, md *Markdown) (*Page, error) {
	var err error
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	fm := map[string]any{}
	body := raw
	var eventStart, eventEnd string
	if m := frontMatterRe.FindSubmatchIndex(raw); m != nil {
		if err := yaml.Unmarshal(raw[m[2]:m[3]], &fm); err != nil {
			simple, ok := simpleFrontMatter(string(raw[m[2]:m[3]]))
			if !ok {
				return nil, fmt.Errorf("front matter: %w", err)
			}
			fm = simple
		} else if fm["start"] != nil {
			// An event's times as written: YAML would read a bare
			// timestamp as UTC, but they're in the site's time zone.
			var ev struct{ Start, End yaml.Node }
			if yaml.Unmarshal(raw[m[2]:m[3]], &ev) == nil {
				eventStart, eventEnd = ev.Start.Value, ev.End.Value
			}
		}
		if fm == nil {
			fm = map[string]any{}
		}
		body = raw[m[1]:]
	}
	p := &Page{Kind: KindPage, Source: rel, Params: fm, Terms: map[string][]string{}, eventStart: eventStart, eventEnd: eventEnd}
	if p.eventStart == "" && fm["start"] != nil {
		p.eventStart, p.eventEnd = str(fm["start"]), str(fm["end"])
	}
	p.Title = str(fm["title"])
	p.Summary = str(fm["summary"])
	if p.Summary == "" {
		p.Summary = str(fm["description"])
	}
	p.Layout = str(fm["layout"])
	p.Image = str(fm["image"])
	p.ImageAlt = str(fm["image_alt"])
	// A product's link preview is its first photo, not a drawn card.
	if p.Image == "" {
		if ph, _ := fm["photos"].([]any); len(ph) > 0 {
			if m, _ := ph[0].(map[string]any); m != nil {
				p.Image = str(m["image"])
				if p.ImageAlt == "" {
					p.ImageAlt = str(m["alt"])
				}
			}
		}
	}
	p.RedirectTo = str(fm["redirect_to"])
	p.Draft = truthy(fm["draft"])
	p.NoIndex = truthy(fm["noindex"])
	if w, ok := fm["weight"].(int); ok {
		p.Weight = w
	}
	if p.Date, err = date(fm["date"]); err != nil {
		return nil, fmt.Errorf("date: %w", err)
	}
	if p.Updated, err = date(fm["updated"]); err != nil {
		return nil, fmt.Errorf("updated: %w", err)
	}
	if p.PublishAt, err = date(fm["publish_at"]); err != nil {
		return nil, fmt.Errorf("publish_at: %w", err)
	}
	if p.Sections, err = sections(fm["sections"]); err != nil {
		return nil, err
	}

	dir, name := path.Split(rel)
	dir = strings.TrimSuffix(dir, "/")
	base := strings.TrimSuffix(name, ".md")
	switch {
	case rel == "index.md":
		p.Path = "/"
	case rel == "404.md":
		p.Kind, p.Path, p.NoIndex = KindError, "/404.html", true
	case base == "_index":
		p.Kind, p.Path = KindList, "/"+dir+"/"
	case base == "index":
		p.Path = "/" + dir + "/"
	default:
		slug := base
		if s := str(fm["slug"]); s != "" {
			slug = s
		}
		p.Path = "/" + path.Join(dir, slug) + "/"
	}
	if u := str(fm["url"]); u != "" && p.Kind == KindPage {
		if !strings.HasPrefix(u, "/") || strings.Contains(u, "..") {
			return nil, fmt.Errorf("url %q must be a path starting with /", u)
		}
		if !strings.HasSuffix(u, "/") && !strings.HasSuffix(u, ".html") {
			u += "/"
		}
		p.Path = u
	}
	if dir != "" {
		p.Collection = strings.SplitN(dir, "/", 2)[0]
	}

	switch str(fm["format"]) {
	case "", "markdown":
		if p.Content, err = md.RenderTitled(body, p.Title); err != nil {
			return nil, err
		}
	case "html":
		if p.Content, err = md.HTML(body); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("format %q is not markdown or html", str(fm["format"]))
	}
	p.Raw = string(body)
	p.WordCount = CountWords(PlainMarkdown(p.Raw))
	if p.Title == "" {
		p.Title = firstHeading(p.Raw)
	}
	if m := leadImageRe.FindStringSubmatch(p.Raw); m != nil {
		p.LeadImage = &LeadImage{Src: m[2], Alt: m[1]}
	}
	if p.Summary == "" {
		p.Summary = firstLine(p.Raw)
	}
	if p.Title == "" {
		return nil, fmt.Errorf("title is required")
	}
	return p, nil
}

var simpleLineRe = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_-]*):\s*(.*)$`)

// simpleFrontMatter reads front matter that is one `key: value` per line but
// not valid YAML, which is how people write it by hand: `tags: [#Linux, #DevOps]`
// is a comment to YAML and a list of two tags to everyone else. Values in
// [brackets] are lists; quotes around a value are dropped. Anything more
// elaborate than that is left to YAML and its error message.
func simpleFrontMatter(src string) (map[string]any, bool) {
	out := map[string]any{}
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimRight(line, "\r ")
		if strings.TrimSpace(line) == "" {
			continue
		}
		m := simpleLineRe.FindStringSubmatch(line)
		if m == nil {
			return nil, false
		}
		key, val := m[1], strings.TrimSpace(m[2])
		if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") {
			var list []any
			for _, item := range strings.Split(val[1:len(val)-1], ",") {
				if item = unquote(strings.TrimSpace(item)); item != "" {
					list = append(list, item)
				}
			}
			out[key] = list
			continue
		}
		switch val = unquote(val); val {
		case "true":
			out[key] = true
		case "false":
			out[key] = false
		default:
			out[key] = val
		}
	}
	return out, true
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"' || s[0] == '\'' && s[len(s)-1] == '\'') {
		return s[1 : len(s)-1]
	}
	return s
}

// assemble works out what depends on the whole set: collection order,
// neighbours, taxonomies, permalinks, and the list pages nobody wrote.
func (s *Site) assemble() {
	cfg := s.Config
	s.takeTermDocs()
	for _, p := range s.Pages {
		p.Permalink = cfg.URL + p.Path
		switch p.Kind {
		case KindList:
			// content/docs/_index.md is the docs' own page; a folder's
			// _index.md under it (docs/install/_index.md) is that folder's.
			if !p.subList() {
				s.Lists[p.Collection] = p
			}
		case KindPage:
			if p.Collection != "" {
				s.Collections[p.Collection] = append(s.Collections[p.Collection], p)
			}
		}
	}
	for name, pages := range s.Collections {
		by := cfg.Collections[name].Sort
		if by == "" && cfg.Collections[name].Calendar {
			by = "start"
		}
		sortPages(pages, by)
		for i, p := range pages {
			if i > 0 {
				p.Prev = pages[i-1]
			}
			if i < len(pages)-1 {
				p.Next = pages[i+1]
			}
		}
		list, ok := s.Lists[name]
		if !ok {
			list = &Page{Kind: KindList, Title: titleCase(name), Path: s.CollectionPath(name), Collection: name,
				Params: map[string]any{}, Terms: map[string][]string{}, Site: s}
			list.Permalink = cfg.URL + list.Path
			s.Lists[name] = list
			s.Pages = append(s.Pages, list)
		}
		list.Pages = pages
	}
	// A folder's own page lists the pages in that folder.
	for _, p := range s.Pages {
		if p.Kind == KindList && p.subList() {
			dir := path.Dir(p.Source)
			for _, q := range s.Collections[p.Collection] {
				qdir := path.Dir(q.Source)
				if path.Base(q.Source) == "index.md" { // a page in a folder of its own
					qdir = path.Dir(qdir)
				}
				if qdir == dir {
					p.Pages = append(p.Pages, q)
				}
			}
		}
	}

	s.assembleTaxonomies()
	s.paginate()
	sort.Slice(s.Pages, func(i, j int) bool { return s.Pages[i].Path < s.Pages[j].Path })
}

func (s *Site) checkPaths() error {
	seen := map[string]string{}
	for _, p := range s.Pages {
		src := p.Source
		if src == "" {
			src = "(generated " + p.Kind + " page)"
		}
		if !SafeURLPath(p.Path) && p.Path != "/404.html" {
			return fmt.Errorf("%s: the address %q isn't one a build can write safely (letters, digits, - _ . ~ and /, no ..)", src, p.Path)
		}
		if other, dup := seen[p.Path]; dup {
			return fmt.Errorf("%s and %s both want %s", other, src, p.Path)
		}
		seen[p.Path] = src
	}
	return nil
}

// PageAt returns the page at a URL path, or nil.
func (s *Site) PageAt(path string) *Page {
	for _, p := range s.Pages {
		if p.Path == path {
			return p
		}
	}
	return nil
}

// subList reports whether a list page is a folder's own page inside a
// collection (content/docs/install/_index.md) rather than the collection's.
func (p *Page) subList() bool {
	return p.Source != "" && path.Dir(p.Source) != p.Collection
}

// CollectionPath is the address of a collection's own page: /<name>/, or
// the path site.yaml gives it.
func (s *Site) CollectionPath(name string) string {
	if p := s.Config.Collections[name].Path; p != "" {
		return p
	}
	return "/" + name + "/"
}

// placeCollections moves the pages of a collection that has a path of its own
// there: content/docs/run/sizing.md is /run/sizing/ when docs has path: /.
// A page with its own url: stays where it says.
func (s *Site) placeCollections() {
	for _, p := range s.Pages {
		if p.Collection == "" || p.Params["url"] != nil {
			continue
		}
		from, to := "/"+p.Collection+"/", s.CollectionPath(p.Collection)
		if from != to && strings.HasPrefix(p.Path, from) {
			p.Path = to + strings.TrimPrefix(p.Path, from)
		}
	}
}

// Collection returns a collection's pages in order; nil if there is none.
func (s *Site) Collection(name string) []*Page { return s.Collections[name] }

// Menu returns a menu from site.yaml.
func (s *Site) Menu(name string) []MenuItem { return s.Config.Menus[name] }

func sortPages(pages []*Page, by string) {
	sort.SliceStable(pages, func(i, j int) bool {
		a, b := pages[i], pages[j]
		switch by {
		case "title":
			return strings.ToLower(a.Title) < strings.ToLower(b.Title)
		case "start": // events, soonest first; pages that aren't events after them
			if (a.Event == nil) != (b.Event == nil) {
				return a.Event != nil
			}
			if a.Event != nil && !a.Event.Start.Equal(b.Event.Start) {
				return a.Event.Start.Before(b.Event.Start)
			}
			return a.Path < b.Path
		case "weight":
			if a.Weight != b.Weight {
				return a.Weight < b.Weight
			}
			return strings.ToLower(a.Title) < strings.ToLower(b.Title)
		default:
			if !a.Date.Equal(b.Date) {
				return a.Date.After(b.Date)
			}
			return a.Path < b.Path
		}
	})
}

func loadData(dir string) (map[string]any, error) {
	out := map[string]any{}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := filepath.Ext(name)
		key := strings.TrimSuffix(name, ext)
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		var v any
		switch ext {
		case ".yaml", ".yml":
			err = yaml.Unmarshal(raw, &v)
		case ".json":
			err = json.Unmarshal(raw, &v)
		case ".csv":
			v, err = readCSV(raw)
		default:
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("data/%s: %w", name, err)
		}
		if _, dup := out[key]; dup {
			return nil, fmt.Errorf("data/%s: another file is already named %s", name, key)
		}
		out[key] = v
	}
	return out, nil
}

// readCSV reads a spreadsheet saved as CSV into a list of rows, each a list
// of cells, the way a YAML list of lists reads: the tables pack takes either.
// Every row must have as many cells as the first. A byte order mark, which
// Excel writes, is dropped.
func readCSV(raw []byte) (any, error) {
	r := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))))
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	rows := make([]any, len(records))
	for i, rec := range records {
		cells := make([]any, len(rec))
		for j, c := range rec {
			cells[j] = c
		}
		rows[i] = cells
	}
	return rows, nil
}

func sections(v any) ([]Section, error) {
	if v == nil {
		return nil, nil
	}
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("sections: expected a list")
	}
	var out []Section
	for i, item := range list {
		switch it := item.(type) {
		case string:
			out = append(out, Section{Type: it, Data: map[string]any{}})
		case map[string]any:
			if len(it) != 1 {
				return nil, fmt.Errorf("sections[%d]: each section is one name with its fields under it", i)
			}
			for name, fields := range it {
				data, _ := fields.(map[string]any)
				if fields != nil && data == nil {
					return nil, fmt.Errorf("sections[%d] (%s): fields must be a map", i, name)
				}
				if data == nil {
					data = map[string]any{}
				}
				out = append(out, Section{Type: name, Data: data})
			}
		default:
			return nil, fmt.Errorf("sections[%d]: expected a name or a map", i)
		}
	}
	return out, nil
}

func str(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case nil:
		return ""
	default:
		return fmt.Sprint(t)
	}
}

func truthy(v any) bool {
	b, ok := v.(bool)
	return ok && b
}

func strList(v any) []string {
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			if s := str(x); s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		var out []string
		for _, s := range strings.Split(t, ",") {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

var dateLayouts = []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02"}

// ParseDate reads a date as front matter's date: does: 2026-10-10, or with
// a time, 2026-10-10 08:30.
func ParseDate(v any) (time.Time, error) { return date(v) }

func date(v any) (time.Time, error) {
	switch t := v.(type) {
	case nil:
		return time.Time{}, nil
	case time.Time:
		return t, nil
	case string:
		for _, l := range dateLayouts {
			if d, err := time.Parse(l, strings.TrimSpace(t)); err == nil {
				return d, nil
			}
		}
		return time.Time{}, fmt.Errorf("%q is not a date (use 2026-10-10)", t)
	}
	return time.Time{}, fmt.Errorf("%v is not a date", v)
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

func plain(h string) string { return html.UnescapeString(tagRe.ReplaceAllString(h, " ")) }

// firstHeading returns the text of the body's first "# " heading.
func firstHeading(md string) string {
	for _, line := range strings.Split(md, "\n") {
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "# ") {
			return strings.TrimSpace(t[2:])
		}
	}
	return ""
}

// firstLine is a summary for a page that has none: the first line of prose,
// skipping headings, code, images, quotes and lists, as plain text cut to 220
// characters.
func firstLine(md string) string {
	for _, line := range strings.Split(md, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		skip := false
		for _, pre := range []string{"#", "```", "!", ">", "-", "*"} {
			if strings.HasPrefix(t, pre) {
				skip = true
				break
			}
		}
		if !skip {
			return Excerpt(220, t)
		}
	}
	return ""
}

var paraRe = regexp.MustCompile(`(?s)<p>(.*?)</p>`)

func firstParagraph(h string) string {
	for _, m := range paraRe.FindAllStringSubmatch(h, -1) {
		t := strings.Join(strings.Fields(plain(m[1])), " ")
		if t == "" {
			continue
		}
		if r := []rune(t); len(r) > 200 {
			cut := string(r[:200])
			if i := strings.LastIndex(cut, " "); i > 120 {
				cut = cut[:i]
			}
			return cut + "…"
		}
		return t
	}
	return ""
}

// Slugify makes a URL segment: lower case, letters and digits, hyphens.
func Slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			dash = false
		case b.Len() > 0 && !dash:
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

func titleCase(s string) string {
	s = strings.ReplaceAll(s, "-", " ")
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
