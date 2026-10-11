package check

import (
	"bufio"
	"encoding/xml"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/media"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
)

// Hosts whose scripts track visitors. Loading one on page view means the
// tracking starts before anyone could agree to it. A site that asks first
// loads it from its consent code instead, which this check doesn't see.
var trackers = []string{
	"googletagmanager.com", "google-analytics.com", "doubleclick.net", "googleadservices.com",
	"connect.facebook.net", "facebook.net", "analytics.tiktok.com", "snap.licdn.com",
	"static.hotjar.com", "script.hotjar.com", "clarity.ms", "cdn.segment.com",
	"static.cloudflareinsights.com", "js.hs-scripts.com", "matomo.cloud", "cdn.amplitude.com",
	"mc.yandex.ru", "bat.bing.com",
}

func isTracker(h string) bool {
	for _, t := range trackers {
		if h == t || strings.HasSuffix(h, "."+t) {
			return true
		}
	}
	return false
}

var paginated = regexp.MustCompile(`/page/\d+/$`)

func (c *checker) checkPage(pg *page) {
	if pg.redirect {
		return
	}
	strict := c.cfg.Check.CSP == "strict"
	inlineLevel := Warning
	if strict {
		inlineLevel = Error
	}
	// Tracking and third parties.
	seen := map[string]bool{}
	for _, ref := range append(append(append([]string{}, pg.scripts...), pg.frames...), pg.styles...) {
		h := c.host(ref)
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		switch {
		case isTracker(h):
			c.add(Error, "tracker-before-consent", pg.file, "%s loads on page view, before any consent; load it from your consent code instead", h)
		case !c.allow[h]:
			c.add(Warning, "third-party", pg.file, "loads from %s; list it in check.allow_third_party if that's intended, and in the privacy notice", h)
		}
	}
	for _, js := range pg.inlineJS {
		if strings.Contains(js, "gtag(") || strings.Contains(js, "dataLayer") || strings.Contains(js, "fbq(") || strings.Contains(js, "_paq") {
			c.add(Error, "tracker-before-consent", pg.file, "inline tracking code runs on page view: %s", js)
		}
	}
	// What a strict Content-Security-Policy would refuse.
	if len(pg.inlineJS) > 0 {
		c.add(inlineLevel, "inline-script", pg.file, "%d inline script(s), first: %s", len(pg.inlineJS), pg.inlineJS[0])
	}
	if pg.inlineStyles > 0 {
		c.add(inlineLevel, "inline-style", pg.file, "%d inline style(s) (style attributes or <style>)", pg.inlineStyles)
	}
	for _, h := range pg.handlers {
		c.add(Error, "inline-handler", pg.file, "event handler attribute %s; attach it from a script file", h)
	}
	for _, u := range pg.insecure {
		c.add(Error, "mixed-content", pg.file, "loads %s over plain http", u)
	}
	if pg.imgNoAlt > 0 {
		c.add(Warning, "img-alt", pg.file, "%d image(s) without an alt attribute (use alt=\"\" for decoration)", pg.imgNoAlt)
	}
	// Broken links.
	for _, ref := range pg.links {
		target, ok := c.internal(ref, pg.url)
		if !ok {
			continue
		}
		if !exists(c.out, target) {
			c.add(Error, "broken-link", pg.file, "links to %s, which is not in the site", ref)
		}
	}
	// Cloudflare's Email Address Obfuscation.
	if c.cfg.Check.Cloudflare {
		for _, e := range uniq(pg.exposed) {
			c.add(Warning, "cloudflare-email", pg.file, "%s is outside <!--email_off--> markers; Cloudflare will rewrite it", e)
		}
	}
	// Search and social metadata, for pages meant to be found.
	if pg.noindex || pg.static || strings.HasSuffix(pg.file, "/404.html") {
		return
	}
	if strings.TrimSpace(pg.title) == "" {
		c.add(Error, "missing-title", pg.file, "no <title>")
	}
	if pg.description == "" {
		c.add(Warning, "missing-description", pg.file, "no meta description")
	} else if n := len([]rune(pg.description)); n > 300 {
		c.add(Warning, "description-length", pg.file, "meta description is %d characters; search results show about 160", n)
	}
	if pg.canonical == "" {
		c.add(Warning, "missing-canonical", pg.file, "no canonical link")
	}
	if pg.ogImage == "" {
		c.add(Warning, "social-image", pg.file, "no og:image; links to this page will preview without a picture")
	} else {
		if !pg.ogImageAlt {
			c.add(Warning, "social-image-alt", pg.file, "og:image has no og:image:alt")
		}
		if target, ok := c.internal(pg.ogImage, pg.url); ok && !exists(c.out, target) {
			c.add(Error, "social-image", pg.file, "og:image %s is not in the site", pg.ogImage)
		}
	}
}

func uniq(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

func (c *checker) sitewide(pages []*page) {
	byURL := map[string]*page{}
	titles := map[string][]string{}
	descs := map[string][]string{}
	thirdParty := false
	for _, pg := range pages {
		byURL[pg.url] = pg
		if pg.redirect {
			continue
		}
		for _, ref := range append(append([]string{}, pg.scripts...), pg.frames...) {
			if c.host(ref) != "" {
				thirdParty = true
			}
		}
		if pg.noindex || pg.static || paginated.MatchString(pg.url) || strings.HasSuffix(pg.file, "/404.html") {
			continue
		}
		if t := strings.TrimSpace(pg.title); t != "" {
			titles[t] = append(titles[t], pg.file)
		}
		if pg.description != "" {
			descs[pg.description] = append(descs[pg.description], pg.file)
		}
	}
	dup := func(rule, what string, m map[string][]string) {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if files := m[k]; len(files) > 1 {
				sort.Strings(files)
				c.add(Warning, rule, files[0], "same %s as %d other page(s), e.g. %s", what, len(files)-1, files[1])
			}
		}
	}
	dup("duplicate-title", "title", titles)
	dup("duplicate-description", "description", descs)

	// The sitemap lists only pages meant to be indexed, and robots.txt points to it.
	sitemap := filepath.Join(c.out, "sitemap.xml")
	if raw, err := os.ReadFile(sitemap); err == nil {
		var sm struct {
			URLs []struct {
				Loc string `xml:"loc"`
			} `xml:"url"`
		}
		if xml.Unmarshal(raw, &sm) == nil {
			for _, u := range sm.URLs {
				path := strings.TrimPrefix(u.Loc, strings.TrimRight(c.cfg.URL, "/"))
				if pg := byURL[path]; pg != nil && (pg.noindex || pg.redirect) {
					c.add(Error, "noindex-in-sitemap", "public/sitemap.xml", "lists %s, which asks not to be indexed", path)
				}
			}
		}
		robots, err := os.ReadFile(filepath.Join(c.out, "robots.txt"))
		if err != nil {
			c.add(Warning, "robots-sitemap", "public/robots.txt", "no robots.txt, so crawlers aren't told where the sitemap is")
		} else if !strings.Contains(strings.ToLower(string(robots)), "sitemap:") {
			c.add(Warning, "robots-sitemap", "public/robots.txt", "has no Sitemap: line")
		}
	}

	// security.txt (RFC 9116): present, and not past its Expires date.
	sec := filepath.Join(c.out, ".well-known", "security.txt")
	if f, err := os.Open(sec); err != nil {
		c.add(Warning, "security-txt", "public/.well-known/security.txt", "missing; it tells researchers how to report a vulnerability")
	} else {
		var expires time.Time
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if v, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "Expires:"); ok {
				expires, _ = time.Parse(time.RFC3339, strings.TrimSpace(v))
			}
		}
		f.Close()
		switch {
		case expires.IsZero():
			c.add(Error, "security-txt", "public/.well-known/security.txt", "no valid Expires: field (required)")
		case expires.Before(c.now):
			c.add(Error, "security-txt", "public/.well-known/security.txt", "expired on %s", expires.Format("2006-01-02"))
		case expires.Before(c.now.AddDate(0, 0, 30)):
			c.add(Warning, "security-txt", "public/.well-known/security.txt", "expires on %s, within 30 days", expires.Format("2006-01-02"))
		}
	}

	// A site that loads anything from elsewhere owes its visitors a privacy
	// notice, and the notice owes them a way to reach a person.
	if thirdParty {
		pp := c.cfg.Check.PrivacyPage
		if pp == "" {
			pp = "/privacy/"
		}
		pg := byURL[pp]
		switch {
		case pg == nil || pg.redirect:
			c.add(Error, "privacy-page", pp, "the site loads third-party code but has no privacy notice at %s (check.privacy_page)", pp)
		case len(pg.mailtos) == 0:
			c.add(Warning, "privacy-contact", pg.file, "the privacy notice has no email address to write to")
		}
	}
}

// Published pictures: a photo straight off a phone carries the place it was
// taken, which on a home page is the author's address. Pictures uploaded
// through the editor are re-encoded and never have it; this catches the ones
// committed by hand.
func (c *checker) images() {
	_ = filepath.WalkDir(c.out, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(p))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" && ext != ".gif" {
			return nil
		}
		rel, _ := filepath.Rel(c.out, p)
		file := "public/" + filepath.ToSlash(rel)
		info, err := d.Info()
		if err == nil && info.Size() > imageHeavy {
			c.add(Warning, "image-weight", file, "is %d KB; pictures over %d KB slow pages down (the editor resizes uploads)", info.Size()>>10, imageHeavy>>10)
		}
		if ext != ".jpg" && ext != ".jpeg" {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		head := make([]byte, 256<<10) // EXIF sits at the start
		n, _ := io.ReadFull(f, head)
		f.Close()
		if media.ReadJPEGMeta(head[:n]).GPS {
			c.add(Error, "image-location", file, "carries the GPS location where it was taken; upload it through the editor, or remove it (exiftool -gps:all= FILE)")
		}
		return nil
	})
}

const imageHeavy = 500 << 10

// The look: every pair the theme says must stay readable, with the site's
// choices applied, in light and in dark mode.
func (c *checker) look() {
	lf, err := site.LoadLook(c.siteDir)
	if err != nil || lf == nil {
		return // a broken look.yaml fails the build first
	}
	vals, _ := lf.Values(c.cfg.Look)
	for _, r := range lf.Contrasts(vals) {
		for _, m := range []struct {
			mode  string
			ratio float64
		}{{"light", r.Light}, {"dark", r.Dark}} {
			if m.ratio < site.MinContrast {
				c.add(Error, "look-contrast", "site.yaml", "%s on %s is %.2f:1 in %s mode; text needs at least %.1f:1", r.Fore, r.Back, m.ratio, m.mode, site.MinContrast)
			}
		}
	}
}
