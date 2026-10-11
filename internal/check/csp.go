package check

import (
	"net/url"
	"path/filepath"
	"sort"
	"strings"
)

// policy works out the Content-Security-Policy the built pages need: their
// own origin, the hosts they load scripts, styles, images and frames from,
// and 'unsafe-inline' only where a page still has inline code or styles. It
// can't see what scripts fetch at run time (connect-src), so a site adds
// those hosts itself.
func policy(pages []*page) string {
	sets := map[string]map[string]bool{}
	add := func(dir, src string) {
		if sets[dir] == nil {
			sets[dir] = map[string]bool{}
		}
		sets[dir][src] = true
	}
	origin := func(ref string) string {
		u, err := url.Parse(ref)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "") {
			return ""
		}
		scheme := u.Scheme
		if scheme == "" {
			scheme = "https"
		}
		return scheme + "://" + strings.ToLower(u.Host)
	}
	for _, pg := range pages {
		if pg.redirect {
			continue
		}
		for _, s := range pg.scripts {
			if o := origin(s); o != "" {
				add("script-src", o)
			}
		}
		for _, s := range pg.styles {
			if o := origin(s); o != "" {
				add("style-src", o)
			}
		}
		for _, s := range pg.images {
			if strings.HasPrefix(s, "data:") {
				add("img-src", "data:")
			} else if o := origin(s); o != "" {
				add("img-src", o)
			}
		}
		for _, s := range pg.frames {
			if o := origin(s); o != "" {
				add("frame-src", o)
			}
		}
		// A counter the consent script loads (only after consent): its own
		// script, where it reports, and its tracking pixel.
		for _, h := range pg.consentHosts {
			add("script-src", h)
			add("connect-src", h)
			add("img-src", h)
		}
		if len(pg.inlineJS) > 0 || len(pg.handlers) > 0 {
			add("script-src", "'unsafe-inline'")
		}
		if pg.inlineStyles > 0 {
			add("style-src", "'unsafe-inline'")
		}
	}
	list := func(dir string, base ...string) string {
		var extra []string
		for s := range sets[dir] {
			extra = append(extra, s)
		}
		sort.Strings(extra)
		return dir + " " + strings.Join(append(base, extra...), " ")
	}
	parts := []string{
		"default-src 'self'",
		list("script-src", "'self'"),
		list("style-src", "'self'"),
		list("img-src", "'self'"),
		"font-src 'self'",
		list("connect-src", "'self'"),
	}
	if len(sets["frame-src"]) > 0 {
		parts = append(parts, list("frame-src"))
	} else {
		parts = append(parts, "frame-src 'none'")
	}
	parts = append(parts, "object-src 'none'", "base-uri 'self'", "form-action 'self'", "frame-ancestors 'self'")
	return strings.Join(parts, "; ")
}

// Policy is the Content-Security-Policy for a built site, from its pages
// alone (for publishers that write it into a server config).
func Policy(out string) string {
	c := &checker{out: out, siteDir: filepath.Dir(out)}
	pages, err := c.readPages()
	if err != nil {
		return ""
	}
	return policy(pages)
}
