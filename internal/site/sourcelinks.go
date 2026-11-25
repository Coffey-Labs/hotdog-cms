package site

import (
	"html/template"
	"net/url"
	"path"
	"regexp"
	"strings"
)

// srcLinkRe finds links written to a page's source file, the way MkDocs
// sites and most Markdown editors write them: about.md,
// ../install/linux.md#docker. Only relative paths: an address with a scheme
// is somewhere else.
var srcLinkRe = regexp.MustCompile(`(<a\s[^>]*?href=")([^"#?:]+\.md)((?:#[^"]*)?)"`)

// resolveSourceLinks turns each link to another page's source file into that
// page's address, keeping any #fragment. Paths are relative to the linking
// page's own source, as on disk; a path starting with / is from content/. A
// section's index.md also finds its _index.md, MkDocs' name for what HotDog
// calls _index.md. A link to a .md file no page came from is left as written,
// for the link check to report.
func (s *Site) resolveSourceLinks() {
	bySource := s.sourceIndex()
	for _, p := range s.Pages {
		if p.Source == "" || !strings.Contains(string(p.Content), ".md") {
			continue
		}
		dir := path.Dir(p.Source)
		p.Content = template.HTML(srcLinkRe.ReplaceAllStringFunc(string(p.Content), func(m string) string {
			sub := srcLinkRe.FindStringSubmatch(m)
			to := sourceTarget(bySource, dir, sub[2])
			if to == nil {
				return m
			}
			return sub[1] + to.Path + sub[3] + `"`
		}))
	}
}

func (s *Site) sourceIndex() map[string]*Page {
	out := map[string]*Page{}
	for _, p := range s.Pages {
		if p.Source != "" {
			out[p.Source] = p
		}
	}
	return out
}

// sourceTarget is the page a link to a source file (about.md,
// ../install/linux.md, /guide.md) written in a file in dir points at, or nil.
func sourceTarget(bySource map[string]*Page, dir, target string) *Page {
	var rel string
	if strings.HasPrefix(target, "/") {
		rel = path.Clean(strings.TrimPrefix(target, "/"))
	} else {
		rel = path.Clean(path.Join(dir, target))
	}
	if strings.HasPrefix(rel, "../") || rel == ".." {
		return nil
	}
	to := bySource[rel]
	if to == nil && path.Base(rel) == "index.md" {
		to = bySource[path.Join(path.Dir(rel), "_index.md")]
	}
	if to == nil && path.Base(rel) == "README.md" {
		to = bySource[path.Join(path.Dir(rel), "_index.md")]
	}
	return to
}

// mdLinkRe finds a Markdown link or image target, ](target) or ](target
// "title"); attrLinkRe an href or src in HTML written into the Markdown.
var (
	mdLinkRe   = regexp.MustCompile(`(\]\()([^)\s]+)((?:\s+"[^"]*")?\))`)
	attrLinkRe = regexp.MustCompile(`(\s(?:href|src)=")([^"]+)(")`)
)

// AbsoluteMarkdown returns a function that gives a page's Markdown with every
// relative link made absolute: a link to a source file becomes that page's
// address, and any other relative address is resolved against the page's
// own. For Markdown read away from the site (llms-full.txt, a page's
// index.md), where a relative link has nothing to be relative to. Code
// blocks and code spans are left as written: they show links, not use them.
func (s *Site) AbsoluteMarkdown() func(p *Page) string {
	bySource := s.sourceIndex()
	return func(p *Page) string {
		base, err := url.Parse(p.Permalink)
		if err != nil || p.Raw == "" {
			return p.Raw
		}
		dir := path.Dir(p.Source)
		fix := func(target string) string {
			if target == "" || strings.HasPrefix(target, "#") || strings.HasPrefix(target, "//") {
				return target
			}
			if u, err := url.Parse(target); err != nil || u.Scheme != "" {
				return target
			}
			file, frag, _ := strings.Cut(target, "#")
			if strings.HasSuffix(file, ".md") && !strings.ContainsAny(file, "?:") {
				if to := sourceTarget(bySource, dir, file); to != nil {
					if frag != "" {
						return to.Permalink + "#" + frag
					}
					return to.Permalink
				}
			}
			ref, err := url.Parse(target)
			if err != nil {
				return target
			}
			return base.ResolveReference(ref).String()
		}
		rewrite := func(text string) string {
			text = mdLinkRe.ReplaceAllStringFunc(text, func(m string) string {
				sub := mdLinkRe.FindStringSubmatch(m)
				return sub[1] + fix(sub[2]) + sub[3]
			})
			return attrLinkRe.ReplaceAllStringFunc(text, func(m string) string {
				sub := attrLinkRe.FindStringSubmatch(m)
				return sub[1] + fix(sub[2]) + sub[3]
			})
		}
		return outsideCode(p.Raw, rewrite)
	}
}

// outsideCode applies f to the parts of Markdown that aren't code: fenced
// blocks (``` or ~~~) and `code spans` pass through untouched.
func outsideCode(md string, f func(string) string) string {
	var out, prose strings.Builder
	flush := func() {
		if prose.Len() > 0 {
			out.WriteString(outsideSpans(prose.String(), f))
			prose.Reset()
		}
	}
	fence := ""
	for _, line := range strings.SplitAfter(md, "\n") {
		trim := strings.TrimLeft(line, " \t")
		switch {
		case fence != "":
			out.WriteString(line)
			if strings.HasPrefix(trim, fence) {
				fence = ""
			}
			continue
		case strings.HasPrefix(trim, "```"):
			fence = "```"
		case strings.HasPrefix(trim, "~~~"):
			fence = "~~~"
		}
		if fence != "" {
			flush()
			out.WriteString(line)
			continue
		}
		prose.WriteString(line)
	}
	flush()
	return out.String()
}

// outsideSpans applies f between `code spans`, which pass through untouched.
func outsideSpans(text string, f func(string) string) string {
	parts := strings.Split(text, "`")
	if len(parts)%2 == 0 { // an unmatched backtick: no spans to trust
		return f(text)
	}
	for i := 0; i < len(parts); i += 2 {
		parts[i] = f(parts[i])
	}
	return strings.Join(parts, "`")
}
