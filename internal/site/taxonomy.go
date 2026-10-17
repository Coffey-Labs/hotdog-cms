package site

import (
	"slices"
	"sort"
	"strings"
	"unicode"
)

// Taxonomy terms are matched and addressed by a slug, and shown the way
// people spelled them. Spellings that differ only in case, a leading #, or
// hyphens are one subject: "open source", "#OpenSource" and "open-source" share
// a page, named for whichever spelling is used most.

// TermSlug makes a term's URL segment: lower case, no leading #, spaces and
// underscores become hyphens, other punctuation goes ("node.js" -> "nodejs").
// Letters in any script are kept.
func TermSlug(v string) string {
	s := strings.TrimLeft(strings.ToLower(strings.TrimSpace(v)), "#")
	var b strings.Builder
	dash := false
	for _, r := range s {
		switch {
		case unicode.IsSpace(r) || r == '_' || r == '-':
			if !dash {
				b.WriteByte('-')
				dash = true
			}
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			dash = false
		}
	}
	return strings.Trim(b.String(), "-")
}

// TermInfo is one term of a taxonomy.
type TermInfo struct {
	Slug  string
	Label string
	Count int // how many times it is used across pages
	URL   string
	Page  *Page
}

type termIndex struct {
	canon   map[string]string // hyphen-blind key -> slug used most
	infos   map[string]*TermInfo
	order   []string // by count, then first appearance
	byCount []TermInfo
	byLabel []TermInfo
}

func hyphenBlind(slug string) string { return strings.ReplaceAll(slug, "-", "") }

// mostUsed returns the key with the highest count; ties go to the one seen first.
func mostUsed(counts map[string]int, seen []string) string {
	best, n := "", -1
	for _, k := range seen {
		if counts[k] > n {
			best, n = k, counts[k]
		}
	}
	return best
}

// A folder in content/ named for a taxonomy describes its terms instead of
// being a collection: content/tags/linux.md (or tags/linux/index.md) gives
// /tags/linux/ a title, words, a picture, a layout and any other front
// matter, and content/tags/_index.md does the same for /tags/. The term's
// pages are still listed by the taxonomy.
func (s *Site) takeTermDocs() {
	s.termDocs = map[string]map[string]*Page{}
	kept := s.Pages[:0]
	for _, p := range s.Pages {
		tax := p.Collection
		if tax == "" || !slices.Contains(s.Config.Taxonomies, tax) || (p.Kind != KindPage && p.Kind != KindList) {
			kept = append(kept, p)
			continue
		}
		if s.termDocs[tax] == nil {
			s.termDocs[tax] = map[string]*Page{}
		}
		term := ""
		if p.Kind == KindPage {
			term = strings.Trim(strings.TrimPrefix(p.Path, "/"+tax+"/"), "/")
		}
		s.termDocs[tax][term] = p
	}
	s.Pages = kept
}

// describe gives a generated term or terms page what its content/ file says.
func (p *Page) describe(d *Page) {
	if d == nil {
		return
	}
	p.Source, p.Title, p.Summary = d.Source, d.Title, d.Summary
	p.Content, p.Raw, p.WordCount, p.LeadImage = d.Content, d.Raw, d.WordCount, d.LeadImage
	p.Params, p.Sections, p.Layout, p.Weight = d.Params, d.Sections, d.Layout, d.Weight
	p.Image, p.ImageAlt = d.Image, d.ImageAlt
	p.NoIndex = p.NoIndex || d.NoIndex
}

func (s *Site) assembleTaxonomies() {
	cfg := s.Config
	s.terms = map[string]*termIndex{}
	// Newest first, so ties between spellings go to the more recent writing.
	pages := make([]*Page, 0, len(s.Pages))
	for _, p := range s.Pages {
		if p.Kind == KindPage {
			pages = append(pages, p)
		}
	}
	sortPages(pages, "date")
	for _, tax := range cfg.Taxonomies {
		// Pass 1: which slug each hyphen-blind key settles on.
		slugCounts := map[string]map[string]int{}
		slugSeen := map[string][]string{}
		for _, p := range pages {
			for _, v := range strList(p.Params[tax]) {
				slug := TermSlug(v)
				if slug == "" {
					continue
				}
				k := hyphenBlind(slug)
				if slugCounts[k] == nil {
					slugCounts[k] = map[string]int{}
				}
				if slugCounts[k][slug] == 0 {
					slugSeen[k] = append(slugSeen[k], slug)
				}
				slugCounts[k][slug]++
			}
		}
		ti := &termIndex{canon: map[string]string{}, infos: map[string]*TermInfo{}}
		for k := range slugCounts {
			ti.canon[k] = mostUsed(slugCounts[k], slugSeen[k])
		}
		// A term with a page in content/ but on no page yet still gets its page.
		docs := map[string]*Page{}
		for term, d := range s.termDocs[tax] {
			if term == "" {
				continue
			}
			slug := ti.Slug(term)
			if slug == "" {
				continue
			}
			docs[slug] = d
			if _, used := ti.canon[hyphenBlind(slug)]; !used {
				ti.canon[hyphenBlind(slug)] = slug
				ti.infos[slug] = &TermInfo{Slug: slug, Label: d.Title, URL: "/" + tax + "/" + slug + "/"}
			}
		}
		// Pass 2: counts, labels and the pages under each term.
		labelCounts := map[string]map[string]int{}
		labelSeen := map[string][]string{}
		termPages := map[string][]*Page{}
		for _, p := range pages {
			onPage := map[string]bool{}
			for _, v := range strList(p.Params[tax]) {
				slug := ti.Slug(v)
				if slug == "" {
					continue
				}
				info := ti.infos[slug]
				if info == nil {
					info = &TermInfo{Slug: slug, URL: "/" + tax + "/" + slug + "/"}
					ti.infos[slug] = info
					ti.order = append(ti.order, slug)
					labelCounts[slug] = map[string]int{}
				}
				info.Count++
				label := strings.TrimLeft(strings.TrimSpace(v), "#")
				if labelCounts[slug][label] == 0 {
					labelSeen[slug] = append(labelSeen[slug], label)
				}
				labelCounts[slug][label]++
				if !onPage[slug] {
					onPage[slug] = true
					p.Terms[tax] = append(p.Terms[tax], slug)
					termPages[slug] = append(termPages[slug], p)
				}
			}
		}
		sort.SliceStable(ti.order, func(i, j int) bool { return ti.infos[ti.order[i]].Count > ti.infos[ti.order[j]].Count })
		index := &Page{Kind: KindTerms, Title: titleCase(tax), Path: "/" + tax + "/", Taxonomy: tax, Term: tax,
			Params: map[string]any{}, Terms: map[string][]string{}, Site: s}
		index.describe(s.termDocs[tax][""])
		index.Permalink = cfg.URL + index.Path
		for slug, info := range ti.infos {
			if info.Count == 0 { // described but not used: a page, but not in the lists
				tp := &Page{Kind: KindTerm, Title: info.Label, Path: info.URL, Taxonomy: tax, Term: slug,
					Params: map[string]any{}, Terms: map[string][]string{}, Site: s}
				tp.describe(docs[slug])
				tp.Permalink = cfg.URL + tp.Path
				info.Page = tp
				s.Pages = append(s.Pages, tp)
			}
		}
		for _, slug := range ti.order {
			info := ti.infos[slug]
			info.Label = mostUsed(labelCounts[slug], labelSeen[slug])
			tp := &Page{Kind: KindTerm, Title: info.Label, Path: info.URL, Taxonomy: tax, Term: slug,
				Pages: termPages[slug], Params: map[string]any{}, Terms: map[string][]string{}, Site: s}
			tp.Permalink = cfg.URL + tp.Path
			tp.NoIndex = cfg.TermIndexMin > 0 && len(tp.Pages) < cfg.TermIndexMin
			tp.describe(docs[slug])
			info.Page = tp
			s.Pages = append(s.Pages, tp)
			index.Pages = append(index.Pages, tp)
		}
		s.terms[tax] = ti
		s.Pages = append(s.Pages, index)
	}
}

// Slug returns the page-sharing slug for a spelling, or "" if it has none.
func (ti *termIndex) Slug(v string) string {
	slug := TermSlug(v)
	if slug == "" {
		return ""
	}
	if c, ok := ti.canon[hyphenBlind(slug)]; ok {
		return c
	}
	return slug
}

// Terms lists a taxonomy's terms, most used first. The list is shared by
// every page of a build; don't change it.
func (s *Site) Terms(tax string) []TermInfo {
	ti := s.terms[tax]
	if ti == nil {
		return nil
	}
	if ti.byCount == nil {
		ti.byCount = make([]TermInfo, 0, len(ti.order))
		for _, slug := range ti.order {
			ti.byCount = append(ti.byCount, *ti.infos[slug])
		}
	}
	return ti.byCount
}

// TermPage is the page of the term a spelling belongs to, or nil if there is
// none: term "tags" "Linux" has the title, words and front matter of
// content/tags/linux.md, when there is one, and the pages tagged Linux.
func (s *Site) TermPage(tax, v string) *Page {
	ti := s.terms[tax]
	if ti == nil {
		return nil
	}
	if info := ti.infos[ti.Slug(v)]; info != nil {
		return info.Page
	}
	return nil
}

// Tagged keeps the pages that carry a term, in the order given: tagged
// "tags" "Linux" (collection "products") is the products tagged Linux, in
// the shop's order. Any spelling of the term matches, as on its page.
func (s *Site) Tagged(tax, v string, pages []*Page) []*Page {
	ti := s.terms[tax]
	if ti == nil {
		return nil
	}
	slug := ti.Slug(v)
	var out []*Page
	for _, p := range pages {
		if slices.Contains(p.Terms[tax], slug) {
			out = append(out, p)
		}
	}
	return out
}

// TermsByLabel lists a taxonomy's terms in label order.
func (s *Site) TermsByLabel(tax string) []TermInfo {
	ti := s.terms[tax]
	if ti == nil {
		return nil
	}
	if ti.byLabel == nil {
		ti.byLabel = append([]TermInfo{}, s.Terms(tax)...)
		sort.SliceStable(ti.byLabel, func(i, j int) bool { return ti.byLabel[i].Label < ti.byLabel[j].Label })
	}
	return ti.byLabel
}

// TermURL is the address of the page a spelling of a term belongs to, or ""
// if the spelling has no letters or digits to make an address of.
func (s *Site) TermURL(tax, v string) string {
	ti := s.terms[tax]
	if ti == nil {
		return ""
	}
	slug := ti.Slug(v)
	if slug == "" {
		return ""
	}
	return "/" + tax + "/" + slug + "/"
}
