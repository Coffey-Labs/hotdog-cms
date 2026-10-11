package importer

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Ghost, Hugo and Jekyll. Hugo and Jekyll are already Markdown with front
// matter, so it's mostly mapping fields and working out the old addresses;
// Ghost's export carries each post's HTML.

// FromGhost reads a Ghost export (Settings → Labs → Export).
func FromGhost(r io.Reader, siteURL, posts string) ([]Item, error) {
	if posts == "" {
		posts = "articles"
	}
	var doc struct {
		DB []struct {
			Data struct {
				Posts []struct {
					ID, Title, Slug, Status, Type string
					HTML                          *string `json:"html"`
					PublishedAt                   string  `json:"published_at"`
					UpdatedAt                     string  `json:"updated_at"`
					CustomExcerpt                 *string `json:"custom_excerpt"`
					FeatureImage                  *string `json:"feature_image"`
					FeatureImageAlt               *string `json:"feature_image_alt"`
				} `json:"posts"`
				Tags []struct {
					ID, Name string
				} `json:"tags"`
				PostsTags []struct {
					PostID string `json:"post_id"`
					TagID  string `json:"tag_id"`
				} `json:"posts_tags"`
				Users []struct {
					ID, Name string
				} `json:"users"`
				PostsAuthors []struct {
					PostID   string `json:"post_id"`
					AuthorID string `json:"author_id"`
				} `json:"posts_authors"`
			} `json:"data"`
		} `json:"db"`
	}
	if err := json.NewDecoder(r).Decode(&doc); err != nil || len(doc.DB) == 0 {
		return nil, fmt.Errorf("not a Ghost export: %v", err)
	}
	d := doc.DB[0].Data
	tags := map[string]string{}
	for _, t := range d.Tags {
		tags[t.ID] = t.Name
	}
	users := map[string]string{}
	for _, u := range d.Users {
		users[u.ID] = u.Name
	}
	postTags := map[string][]string{}
	for _, pt := range d.PostsTags {
		if n := tags[pt.TagID]; n != "" && !strings.HasPrefix(n, "#") { // #internal tags are Ghost's own
			postTags[pt.PostID] = append(postTags[pt.PostID], n)
		}
	}
	author := map[string]string{}
	for _, pa := range d.PostsAuthors {
		if _, ok := author[pa.PostID]; !ok {
			author[pa.PostID] = users[pa.AuthorID]
		}
	}
	str := func(p *string) string {
		if p == nil {
			return ""
		}
		return strings.ReplaceAll(*p, "__GHOST_URL__", strings.TrimRight(siteURL, "/"))
	}
	var items []Item
	for _, p := range d.Posts {
		it := Item{Kind: p.Type, Title: p.Title, Slug: p.Slug, Draft: p.Status != "published", HTML: str(p.HTML), Summary: str(p.CustomExcerpt),
			Image: str(p.FeatureImage), ImageAlt: str(p.FeatureImageAlt), Tags: postTags[p.ID], Author: author[p.ID], OldURL: "/" + p.Slug + "/"}
		it.Date, _ = time.Parse(time.RFC3339, strings.Replace(p.PublishedAt, ".000Z", "Z", 1))
		it.Updated, _ = time.Parse(time.RFC3339, strings.Replace(p.UpdatedAt, ".000Z", "Z", 1))
		if p.Type == "post" {
			it.Dir = posts
		}
		if p.HTML == nil {
			it.Notes = append(it.Notes, "no HTML in the export (exported from an editor format only): the body is empty")
		}
		items = append(items, it)
	}
	return items, nil
}

// splitFront separates front matter (YAML ---, TOML +++ or JSON {}) from a body.
func splitFront(raw string) (map[string]any, string, error) {
	raw = strings.TrimPrefix(strings.ReplaceAll(raw, "\r\n", "\n"), "\ufeff")
	fm := map[string]any{}
	switch {
	case strings.HasPrefix(raw, "---\n"):
		end := strings.Index(raw[4:], "\n---")
		if end < 0 {
			return fm, raw, nil
		}
		err := yaml.Unmarshal([]byte(raw[4:4+end]), &fm)
		return fm, strings.TrimPrefix(raw[4+end+4:], "\n"), err
	case strings.HasPrefix(raw, "+++\n"):
		end := strings.Index(raw[4:], "\n+++")
		if end < 0 {
			return fm, raw, nil
		}
		return parseTOML(raw[4 : 4+end]), strings.TrimPrefix(raw[4+end+4:], "\n"), nil
	case strings.HasPrefix(raw, "{"):
		dec := json.NewDecoder(strings.NewReader(raw))
		if err := dec.Decode(&fm); err != nil {
			return fm, raw, err
		}
		rest, _ := io.ReadAll(dec.Buffered())
		return fm, strings.TrimLeft(string(rest)+raw[dec.InputOffset():], "\n"), nil
	}
	return fm, raw, nil
}

// parseTOML reads the flat part of TOML front matter: strings, numbers,
// booleans, dates and arrays of them. Tables ([params]) are skipped.
func parseTOML(s string) map[string]any {
	out := map[string]any{}
	inTable := false
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			inTable = true
			continue
		}
		if inTable {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.Trim(strings.TrimSpace(k), `"`)] = tomlValue(strings.TrimSpace(v))
	}
	return out
}

func tomlValue(v string) any {
	switch {
	case strings.HasPrefix(v, "["):
		var list []any
		for _, part := range regexp.MustCompile(`"[^"]*"|'[^']*'|[^,\[\]\s]+`).FindAllString(v, -1) {
			list = append(list, tomlValue(part))
		}
		return list
	case strings.HasPrefix(v, `"`) || strings.HasPrefix(v, `'`):
		if u, err := strconv.Unquote(`"` + strings.Trim(v, `"'`) + `"`); err == nil {
			return u
		}
		return strings.Trim(v, `"'`)
	case v == "true" || v == "false":
		return v == "true"
	}
	if n, err := strconv.Atoi(v); err == nil {
		return n
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t
	}
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t
	}
	return v
}

func fmString(fm map[string]any, keys ...string) string {
	for _, k := range keys {
		switch v := fm[k].(type) {
		case string:
			if v != "" {
				return v
			}
		case time.Time:
			return v.Format(time.RFC3339)
		}
	}
	return ""
}

func fmTime(fm map[string]any, keys ...string) time.Time {
	for _, k := range keys {
		switch v := fm[k].(type) {
		case time.Time:
			return v
		case string:
			for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05 -0700", "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02"} {
				if t, err := time.Parse(layout, v); err == nil {
					return t
				}
			}
		}
	}
	return time.Time{}
}

func fmList(fm map[string]any, keys ...string) []string {
	var out []string
	for _, k := range keys {
		switch v := fm[k].(type) {
		case []any:
			for _, x := range v {
				out = append(out, fmt.Sprint(x))
			}
		case string:
			if v != "" {
				out = append(out, strings.Fields(strings.ReplaceAll(v, ",", " "))...)
			}
		}
	}
	return out
}

var (
	hugoShortcode = regexp.MustCompile(`\{\{[<%]\s*/?\s*([\w-]+)`)
	liquidTag     = regexp.MustCompile(`\{%\s*(\w+)|\{\{\s*[\w.]+`)
	relImage      = regexp.MustCompile(`(!\[[^\]]*\]\()([^)\s:]+)(\s*(?:"[^"]*")?\))`)
)

// localImages points relative image links at the file beside the page, so
// the writer can bring them in.
func localImages(body, dir string) string {
	return relImage.ReplaceAllStringFunc(body, func(m string) string {
		sub := relImage.FindStringSubmatch(m)
		if strings.HasPrefix(sub[2], "/") || strings.HasPrefix(sub[2], "#") {
			return m
		}
		return sub[1] + "file://" + filepath.ToSlash(filepath.Join(dir, sub[2])) + sub[3]
	})
}

// FromHugo reads a Hugo site's content/ folder. rename maps a section to a
// collection here ("posts" to "articles"); its old addresses then redirect.
func FromHugo(siteDir string, rename map[string]string) ([]Item, error) {
	root := filepath.Join(siteDir, "content")
	var items []Item
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
			return err
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		fm, body, err := splitFront(string(raw))
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		dir, name := path.Split(rel)
		dir = strings.TrimSuffix(dir, "/")
		slug := strings.TrimSuffix(name, ".md")
		if slug == "index" { // a page bundle: the folder is the page
			dir, slug = path.Split(dir)
			dir = strings.TrimSuffix(dir, "/")
		}
		if slug == "_index" {
			return nil // a section's own page: the collection list here makes its own
		}
		if s := fmString(fm, "slug"); s != "" {
			slug = s
		}
		section := strings.SplitN(dir, "/", 2)[0]
		oldURL := "/" + path.Join(dir, slug) + "/"
		if u := fmString(fm, "url"); u != "" {
			oldURL = u
		}
		newDir := dir
		if to, ok := rename[section]; ok {
			newDir = to + strings.TrimPrefix(dir, section)
		}
		it := Item{Kind: "page", Title: fmString(fm, "title"), Slug: slug, Date: fmTime(fm, "date", "publishDate"), Updated: fmTime(fm, "lastmod"),
			Summary: fmString(fm, "description", "summary"), Tags: append(fmList(fm, "tags"), fmList(fm, "categories")...),
			Markdown: localImages(body, filepath.Dir(p)), OldURL: oldURL, Aliases: fmList(fm, "aliases"), Dir: newDir}
		if dir != "" {
			it.Kind = "post"
		}
		if b, ok := fm["draft"].(bool); ok && b {
			it.Draft = true
		}
		if imgs := fmList(fm, "images"); len(imgs) > 0 {
			it.Image = imgs[0]
		}
		for _, m := range hugoShortcode.FindAllStringSubmatch(body, -1) {
			it.Notes = append(it.Notes, "the Hugo shortcode "+m[1]+", which only Hugo could run; it's left as text")
		}
		it.Notes = dedupe(it.Notes)
		items = append(items, it)
		return nil
	})
	return items, err
}

// FromJekyll reads a Jekyll site: _posts/, _drafts/ and its pages.
func FromJekyll(siteDir, posts string) ([]Item, error) {
	if posts == "" {
		posts = "articles"
	}
	permalink := "date"
	var conf map[string]any
	if raw, err := os.ReadFile(filepath.Join(siteDir, "_config.yml")); err == nil {
		_ = yaml.Unmarshal(raw, &conf)
		if p, ok := conf["permalink"].(string); ok && p != "" {
			permalink = p
		}
	}
	pattern := map[string]string{
		"date":    "/:categories/:year/:month/:day/:title:output_ext",
		"pretty":  "/:categories/:year/:month/:day/:title/",
		"ordinal": "/:categories/:year/:y_day/:title:output_ext",
		"none":    "/:categories/:title:output_ext",
	}[permalink]
	if pattern == "" {
		pattern = permalink
	}
	dated := regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})-(.+)\.(md|markdown|html)$`)
	var items []Item
	read := func(p string, draft bool) error {
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		fm, body, err := splitFront(string(raw))
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		name := filepath.Base(p)
		it := Item{Kind: "post", Dir: posts, Draft: draft, Title: fmString(fm, "title"), Summary: fmString(fm, "excerpt", "description"),
			Tags: append(fmList(fm, "tags"), fmList(fm, "categories", "category")...), Aliases: fmList(fm, "redirect_from"), Author: fmString(fm, "author")}
		if m := dated.FindStringSubmatch(name); m != nil {
			it.Slug = m[4]
			it.Date, _ = time.Parse("2006-01-02", m[1]+"-"+m[2]+"-"+m[3])
		} else {
			it.Slug = strings.TrimSuffix(name, filepath.Ext(name))
		}
		if t := fmTime(fm, "date"); !t.IsZero() {
			it.Date = t
		}
		if b, ok := fm["published"].(bool); ok && !b {
			it.Draft = true
		}
		if strings.HasSuffix(name, ".html") {
			it.HTML = body
		} else {
			it.Markdown = localImages(body, filepath.Dir(p))
		}
		cats := strings.Join(fmList(fm, "categories", "category"), "/")
		old := fmString(fm, "permalink")
		if old == "" && !draft {
			r := strings.NewReplacer(":categories", cats, ":year", it.Date.Format("2006"), ":month", it.Date.Format("01"), ":day", it.Date.Format("02"),
				":y_day", strconv.Itoa(it.Date.YearDay()), ":title", it.Slug, ":slug", it.Slug, ":output_ext", ".html")
			old = regexp.MustCompile(`/+`).ReplaceAllString(r.Replace(pattern), "/")
		}
		it.OldURL = old
		for _, m := range liquidTag.FindAllStringSubmatch(body, -1) {
			tag := strings.TrimSpace(strings.Trim(m[0], "{%"))
			it.Notes = append(it.Notes, "Liquid ("+tag+"), which only Jekyll could run; it's left as text")
		}
		it.Notes = dedupe(it.Notes)
		items = append(items, it)
		return nil
	}
	for _, folder := range []struct {
		dir   string
		draft bool
	}{{"_posts", false}, {"_drafts", true}} {
		entries, _ := os.ReadDir(filepath.Join(siteDir, folder.dir))
		for _, e := range entries {
			if !e.IsDir() && regexp.MustCompile(`\.(md|markdown|html)$`).MatchString(e.Name()) {
				if err := read(filepath.Join(siteDir, folder.dir, e.Name()), folder.draft); err != nil {
					return nil, err
				}
			}
		}
	}
	// Pages: Markdown files outside the _folders.
	err := filepath.WalkDir(siteDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(siteDir, p)
		if d.IsDir() {
			if rel != "." && (strings.HasPrefix(d.Name(), "_") || strings.HasPrefix(d.Name(), ".") || d.Name() == "assets" || d.Name() == "vendor" || d.Name() == "node_modules") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".md") || strings.EqualFold(d.Name(), "README.md") {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		fm, body, err := splitFront(string(raw))
		if err != nil || len(fm) == 0 {
			return nil // without front matter Jekyll doesn't publish it
		}
		dir, name := path.Split(filepath.ToSlash(rel))
		slug := strings.TrimSuffix(name, ".md")
		it := Item{Kind: "page", Title: fmString(fm, "title"), Slug: slug, Dir: strings.TrimSuffix(dir, "/"), Markdown: localImages(body, filepath.Dir(p)),
			Summary: fmString(fm, "description"), Aliases: fmList(fm, "redirect_from")}
		it.OldURL = fmString(fm, "permalink")
		if it.OldURL == "" {
			it.OldURL = "/" + path.Join(it.Dir, slug) + ".html"
		}
		if slug == "index" && it.Dir == "" {
			return nil // the home page: the site's own front page stays
		}
		items = append(items, it)
		return nil
	})
	return items, err
}
