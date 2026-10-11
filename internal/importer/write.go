package importer

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/media"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/siteyaml"
)

// Item is one post or page, from wherever it came.
type Item struct {
	Kind     string // "post" or "page"
	Title    string
	Slug     string
	Date     time.Time
	Updated  time.Time
	Draft    bool
	Summary  string
	Tags     []string
	Author   string
	HTML     string         // the body as HTML (WordPress, Ghost)
	Markdown string         // the body as Markdown already (Hugo, Jekyll)
	Extra    map[string]any // other front matter worth keeping
	OldURL   string         // where it lived, so its address keeps working
	Aliases  []string       // other old addresses (Hugo aliases, Jekyll redirect_from)
	Dir      string         // where under content/ it goes ("articles", "about/team"); "" for the top
	Index    bool           // a page with pages under it: it becomes its folder's own page (_index.md)
	Image    string         // its featured picture
	ImageAlt string
	Notes    []string // what the source already knows didn't come across (shortcodes…)
}

// Options says where and how items are written.
type Options struct {
	SiteDir   string
	Overwrite bool                           // replace pages already there
	Fetch     func(u string) ([]byte, error) // download a picture; default plain HTTP
	Base      string                         // the old site's address, for relative image links
	LocalRoot string                         // file:// pictures are read only from inside this folder (Hugo, Jekyll)
	Log       io.Writer
}

// Report is what happened, page by page.
type Report struct {
	Written   []string
	Skipped   []string // already there
	KeptHTML  map[string][]string
	Notes     map[string][]string
	Pictures  int
	Failed    map[string]string // picture address → why
	Redirects []site.Redirect
}

// Write puts items into a site.
func Write(items []Item, opt Options) (*Report, error) {
	cfgPath := filepath.Join(opt.SiteDir, "site.yaml")
	cfgRaw, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("no site at %s (run hotdog-cms new first): %w", opt.SiteDir, err)
	}
	cfg, err := site.LoadConfig(opt.SiteDir)
	if err != nil {
		return nil, err
	}
	if opt.Fetch == nil {
		opt.Fetch = func(u string) ([]byte, error) { return fetch(u, opt.LocalRoot) }
	}
	if opt.Log == nil {
		opt.Log = io.Discard
	}
	rep := &Report{KeptHTML: map[string][]string{}, Notes: map[string][]string{}, Failed: map[string]string{}}
	pictures := map[string]string{} // old address → new
	taken := func(p string) bool {
		_, err := os.Stat(filepath.Join(opt.SiteDir, filepath.FromSlash(p)))
		return err == nil
	}
	keptHTML := false
	redirects := map[string]string{}
	for _, it := range items {
		slug := it.Slug
		if slug == "" {
			slug = slugify(it.Title)
		}
		if slug == "" {
			slug = "untitled"
		}
		rel := path.Join("content", it.Dir, slug+".md")
		if it.Index {
			rel = path.Join("content", it.Dir, slug, "_index.md")
		}
		newURL := "/" + path.Join(it.Dir, slug) + "/"
		if !opt.Overwrite && taken(rel) {
			rep.Skipped = append(rep.Skipped, rel)
			continue
		}
		month := it.Date
		if month.IsZero() {
			month = time.Now()
		}
		mediaDir := month.Format("2006/01")
		// Pictures: brought in through the same re-encoding as uploads.
		bring := func(u string) string {
			if u == "" || strings.HasPrefix(u, "data:") {
				return u
			}
			abs := u
			if base, err := url.Parse(opt.Base); err == nil && opt.Base != "" {
				if ref, err := url.Parse(u); err == nil {
					abs = base.ResolveReference(ref).String()
				}
			}
			if done, ok := pictures[abs]; ok {
				return done
			}
			data, err := opt.Fetch(abs)
			if err != nil {
				rep.Failed[abs] = err.Error()
				return u
			}
			name := path.Base(strings.SplitN(abs, "?", 2)[0])
			img, err := media.Process(data, name, mediaDir, taken)
			if err != nil {
				rep.Failed[abs] = err.Error()
				return u
			}
			for _, f := range img.Files {
				p := filepath.Join(opt.SiteDir, "static", filepath.FromSlash(f.Path))
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					rep.Failed[abs] = err.Error()
					return u
				}
				if err := os.WriteFile(p, f.Data, 0o644); err != nil {
					rep.Failed[abs] = err.Error()
					return u
				}
			}
			rep.Pictures++
			pictures[abs] = img.URL
			return img.URL
		}

		fm := map[string]any{}
		var body string
		format := ""
		switch {
		case it.HTML != "":
			conv := ToMarkdown(it.HTML)
			if len(conv.Lossy) > 0 {
				body = cleanHTML(it.HTML)
				format = "html"
				keptHTML = true
				rep.KeptHTML[rel] = conv.Lossy
				for _, u := range imageAddresses(body) {
					body = strings.ReplaceAll(body, u, bring(u))
				}
			} else {
				body = conv.Markdown
				for _, u := range conv.Images {
					if n := bring(u); n != u {
						body = strings.ReplaceAll(body, "]("+u, "]("+n)
					}
				}
			}
		default:
			body = it.Markdown
			for _, m := range mdImage.FindAllStringSubmatch(body, -1) {
				if n := bring(m[2]); n != m[2] {
					body = strings.ReplaceAll(body, "]("+m[2], "]("+n)
				}
			}
		}
		if len(it.Notes) > 0 {
			rep.Notes[rel] = it.Notes
		}
		fm["title"] = it.Title
		if !it.Date.IsZero() {
			fm["date"] = it.Date.Format("2006-01-02")
		}
		if !it.Updated.IsZero() && it.Updated.Format("2006-01-02") != it.Date.Format("2006-01-02") {
			fm["updated"] = it.Updated.Format("2006-01-02")
		}
		if it.Summary != "" {
			fm["summary"] = it.Summary
		}
		if len(it.Tags) > 0 {
			fm["tags"] = it.Tags
		}
		if it.Author != "" {
			fm["author"] = it.Author
		}
		if it.Draft {
			fm["draft"] = true
		}
		if it.Image != "" {
			fm["image"] = bring(it.Image)
			if it.ImageAlt != "" {
				fm["image_alt"] = it.ImageAlt
			}
		}
		if format != "" {
			fm["format"] = format
		}
		for k, v := range it.Extra {
			if _, ok := fm[k]; !ok {
				fm[k] = v
			}
		}
		doc, err := frontMatter(fm)
		if err != nil {
			return nil, err
		}
		p := filepath.Join(opt.SiteDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(p, []byte("---\n"+doc+"---\n\n"+strings.TrimLeft(body, "\n")), 0o644); err != nil {
			return nil, err
		}
		rep.Written = append(rep.Written, rel)
		fmt.Fprintf(opt.Log, "  %s\n", rel)
		for _, old := range append([]string{it.OldURL}, it.Aliases...) {
			if from := oldPath(old); from != "" && from != newURL {
				redirects[from] = newURL
			}
		}
	}

	// Every old address keeps working.
	text := string(cfgRaw)
	if len(redirects) > 0 {
		all := map[string]string{}
		for _, r := range cfg.Redirects {
			all[r.From] = r.To
		}
		for from, to := range redirects {
			if _, ok := all[from]; !ok {
				all[from] = to
				rep.Redirects = append(rep.Redirects, site.Redirect{From: from, To: to})
			}
		}
		froms := make([]string, 0, len(all))
		for f := range all {
			froms = append(froms, f)
		}
		sort.Strings(froms)
		list := []map[string]string{}
		for _, f := range froms {
			list = append(list, map[string]string{"from": f, "to": all[f]})
		}
		if text, err = siteyaml.SetTopLevel(text, "redirects", list); err != nil {
			return nil, err
		}
	}
	// Pages kept as HTML need the site to allow (sanitized) HTML.
	if keptHTML && !cfg.Markdown.UnsafeHTML && !cfg.Markdown.TrustHTML {
		md := map[string]any{}
		var doc yaml.Node
		if yaml.Unmarshal([]byte(text), &doc) == nil && len(doc.Content) == 1 {
			m := doc.Content[0]
			for i := 0; i+1 < len(m.Content); i += 2 {
				if m.Content[i].Value == "markdown" {
					_ = m.Content[i+1].Decode(&md)
				}
			}
		}
		md["unsafe_html"] = true
		if text, err = siteyaml.SetTopLevel(text, "markdown", md); err != nil {
			return nil, err
		}
	}
	if text != string(cfgRaw) {
		if err := os.WriteFile(cfgPath, []byte(text), 0o644); err != nil {
			return nil, err
		}
	}
	sort.Slice(rep.Redirects, func(i, j int) bool { return rep.Redirects[i].From < rep.Redirects[j].From })
	return rep, nil
}

var mdImage = regexp.MustCompile(`!\[([^\]]*)\]\(\s*<?([^)\s>]+)`)

func imageAddresses(h string) []string {
	var out []string
	for _, m := range regexp.MustCompile(`<img[^>]+src="([^"]+)"`).FindAllStringSubmatch(h, -1) {
		out = append(out, m[1])
	}
	return out
}

// cleanHTML drops WordPress's block markers from HTML that's kept as it is.
func cleanHTML(h string) string {
	h = regexp.MustCompile(`<!-- /?wp:[^>]*-->\n?`).ReplaceAllString(h, "")
	return strings.TrimSpace(h) + "\n"
}

func oldPath(u string) string {
	if u == "" {
		return ""
	}
	p, err := url.Parse(u)
	if err != nil {
		return ""
	}
	if p.Path == "" {
		return ""
	}
	return p.Path
}

func frontMatter(fm map[string]any) (string, error) {
	order := []string{"title", "date", "updated", "summary", "tags", "author", "draft", "image", "image_alt", "format"}
	m := &yaml.Node{Kind: yaml.MappingNode}
	add := func(k string, v any) error {
		var n yaml.Node
		if err := n.Encode(v); err != nil {
			return err
		}
		if _, ok := v.([]string); ok {
			n.Style = yaml.FlowStyle
		}
		m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: k}, &n)
		return nil
	}
	for _, k := range order {
		if v, ok := fm[k]; ok {
			if err := add(k, v); err != nil {
				return "", err
			}
			delete(fm, k)
		}
	}
	rest := make([]string, 0, len(fm))
	for k := range fm {
		rest = append(rest, k)
	}
	sort.Strings(rest)
	for _, k := range rest {
		if err := add(k, fm[k]); err != nil {
			return "", err
		}
	}
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(m); err != nil {
		return "", err
	}
	return b.String(), nil
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	return strings.Trim(slugRe.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

// fetch downloads a picture over http(s), or reads it from beside the page
// being imported (file://, only inside localRoot), at most media.MaxBytes.
func fetch(u, localRoot string) ([]byte, error) {
	p, err := url.Parse(u)
	if err == nil && p.Scheme == "file" {
		abs, err1 := filepath.Abs(filepath.FromSlash(p.Path))
		root, err2 := filepath.Abs(localRoot)
		if localRoot == "" || err1 != nil || err2 != nil || !strings.HasPrefix(abs, root+string(filepath.Separator)) {
			return nil, fmt.Errorf("a local file outside the folder being imported")
		}
		f, err := os.Open(abs)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return io.ReadAll(io.LimitReader(f, media.MaxBytes+1))
	}
	if err != nil || (p.Scheme != "https" && p.Scheme != "http") {
		return nil, fmt.Errorf("not a web address")
	}
	res, err := (&http.Client{Timeout: 30 * time.Second}).Get(u)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s", res.Status)
	}
	return io.ReadAll(io.LimitReader(res.Body, media.MaxBytes+1))
}

// WriteReport writes what happened as Markdown, for the person who ran it.
func (r *Report) Markdown(source string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Import from %s\n\n", source)
	fmt.Fprintf(&b, "%d page(s) written, %d skipped (already there), %d picture(s) brought in, %d redirect(s) added.\n\n", len(r.Written), len(r.Skipped), r.Pictures, len(r.Redirects))
	if len(r.KeptHTML) > 0 {
		b.WriteString("## Kept as HTML\n\nMarkdown would have lost something in these, so their HTML was kept (`format: html`). It's sanitized when the site is built, so embeds and scripts are dropped; check how they look.\n\n")
		for _, k := range sortedKeys(r.KeptHTML) {
			fmt.Fprintf(&b, "- `%s`: %s\n", k, strings.Join(r.KeptHTML[k], ", "))
		}
		b.WriteString("\n")
	}
	if len(r.Notes) > 0 {
		b.WriteString("## To look at\n\n")
		for _, k := range sortedKeys(r.Notes) {
			fmt.Fprintf(&b, "- `%s`: %s\n", k, strings.Join(r.Notes[k], "; "))
		}
		b.WriteString("\n")
	}
	if len(r.Failed) > 0 {
		b.WriteString("## Pictures that didn't come across\n\nThese still point at the old site.\n\n")
		keys := make([]string, 0, len(r.Failed))
		for k := range r.Failed {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "- %s: %s\n", k, r.Failed[k])
		}
		b.WriteString("\n")
	}
	if len(r.Skipped) > 0 {
		b.WriteString("## Skipped\n\nA page was already there; run again with -overwrite to replace them.\n\n")
		for _, s := range r.Skipped {
			fmt.Fprintf(&b, "- `%s`\n", s)
		}
	}
	return b.String()
}

func sortedKeys(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
