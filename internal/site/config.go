// Package site loads a site directory: its configuration, its content and its
// data files. It knows nothing about templates or output; the build package
// turns what this produces into files.
package site

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is site.yaml.
type Config struct {
	Name        string                      `yaml:"name"`
	URL         string                      `yaml:"url"`
	Language    string                      `yaml:"language"`
	Description string                      `yaml:"description"`
	Author      string                      `yaml:"author"`
	Markdown    MarkdownConfig              `yaml:"markdown"`
	Collections map[string]CollectionConfig `yaml:"collections"`
	Taxonomies  []string                    `yaml:"taxonomies"`
	Menus       map[string][]MenuItem       `yaml:"menus"`
	// Paginate splits list and taxonomy pages into pages of this many items
	// (/articles/page/2/). 0 keeps every list on one page.
	Paginate int `yaml:"paginate"`
	// TermIndexMin marks taxonomy pages with fewer items than this noindex and
	// leaves them out of the sitemap: an archive with a thousand tags used once
	// would otherwise hand search engines a thousand thin pages.
	TermIndexMin int            `yaml:"term_index_min"`
	Redirects    []Redirect     `yaml:"redirects"`
	Outputs      OutputsConfig  `yaml:"outputs"`
	Check        CheckConfig    `yaml:"check"`
	Params       map[string]any `yaml:"params"`
}

// MarkdownConfig controls how page bodies are rendered.
type MarkdownConfig struct {
	// UnsafeHTML lets raw HTML in Markdown through, after a sanitizer. Off by
	// default: a site whose pages are written by several people should not
	// have to trust every one of them with script tags.
	UnsafeHTML bool `yaml:"unsafe_html"`
	// EmailOff wraps every mailto link in Cloudflare's <!--email_off-->
	// markers. Behind Cloudflare's proxy, Email Address Obfuscation otherwise
	// rewrites the address into a token that only a Cloudflare script turns
	// back into text, so it reads "[email protected]" with scripts off, to a
	// text browser and to a crawler. A privacy notice has to show its contact.
	EmailOff bool `yaml:"email_off"`
	// Autolink turns bare URLs into links. On unless set to false.
	Autolink *bool `yaml:"autolink"`
	// ExternalLinks: "new_tab" opens links to other sites in a new tab
	// (target=_blank rel="noopener noreferrer"), so authors needn't mark them.
	ExternalLinks string `yaml:"external_links"`
	// TrustHTML passes raw HTML through without the sanitizer. Only for a site
	// whose every author is trusted with script tags, such as a one-person
	// archive full of embeds; it overrides unsafe_html's sanitizer.
	TrustHTML bool `yaml:"trust_html"`
}

// AutolinkOn reports whether bare URLs become links.
func (m MarkdownConfig) AutolinkOn() bool { return m.Autolink == nil || *m.Autolink }

// Redirect sends one old address somewhere else.
type Redirect struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

// CollectionConfig is the per-folder setting for a collection such as
// content/articles/. Every folder of Markdown is a collection whether it is
// named here or not; this only changes the defaults.
type CollectionConfig struct {
	Layout   string `yaml:"layout"`    // layout for the collection's pages
	Sort     string `yaml:"sort"`      // date (newest first, the default), title or weight
	Feed     bool   `yaml:"feed"`      // write an Atom feed
	FeedPath string `yaml:"feed_path"` // where; default /<collection>/feed.xml
	FeedSize int    `yaml:"feed_size"` // entries in the feed; default 20
	// FeedFormat is atom (the default) or rss.
	FeedFormat string `yaml:"feed_format"`
	FeedTitle  string `yaml:"feed_title"`
	// FeedImage is the channel's logo in an RSS feed, a path in static/.
	FeedImage       string `yaml:"feed_image"`
	FeedDescription string `yaml:"feed_description"`
	// Paginate overrides the site's paginate for this collection's list.
	Paginate int `yaml:"paginate"`
	// Search writes /<collection>/search.json, an index of the collection's
	// pages for a search box to load when someone searches.
	Search bool `yaml:"search"`
}

// MenuItem is one link in a menu from site.yaml.
type MenuItem struct {
	Name     string `yaml:"name"`
	URL      string `yaml:"url"`
	Icon     string `yaml:"icon"`
	Note     string `yaml:"note"`     // a second line under the name
	External bool   `yaml:"external"` // opens in a new tab
}

// OutputsConfig switches the generated files.
type OutputsConfig struct {
	Sitemap *bool `yaml:"sitemap"` // default on
	Robots  *bool `yaml:"robots"`  // default on, unless static/robots.txt exists
	LLMs    bool  `yaml:"llms"`    // llms.txt, default off
}

// CheckConfig tunes `hotdog-cms check`.
type CheckConfig struct {
	// Forbid lists regular expressions that must not appear in the source or
	// the built site: internal host names, personal addresses, and so on.
	Forbid []string `yaml:"forbid"`
	// Ignore turns rules off by id, e.g. [img-alt].
	Ignore []string `yaml:"ignore"`
	// AllowThirdParty lists hosts the site means to load from, such as
	// challenges.cloudflare.com for Turnstile. A tracker on this list is
	// still reported if it loads before consent.
	AllowThirdParty []string `yaml:"allow_third_party"`
	// Cloudflare: the site is served through Cloudflare's proxy, so email
	// addresses and fediverse handles outside email_off markers get mangled.
	Cloudflare bool `yaml:"cloudflare"`
	// CSP "strict" makes inline scripts and styles errors instead of warnings.
	CSP string `yaml:"csp"`
	// PrivacyPage is where the privacy notice lives; default /privacy/.
	PrivacyPage string `yaml:"privacy_page"`
}

func on(b *bool) bool { return b == nil || *b }

// SitemapOn reports whether sitemap.xml is written.
func (o OutputsConfig) SitemapOn() bool { return on(o.Sitemap) }

// RobotsOn reports whether robots.txt is written.
func (o OutputsConfig) RobotsOn() bool { return on(o.Robots) }

// LoadConfig reads and validates site.yaml in dir.
func LoadConfig(dir string) (*Config, error) {
	path := filepath.Join(dir, "site.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("no site here: %w", err)
	}
	var c Config
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("site.yaml: %w", err)
	}
	if c.Name == "" {
		return nil, fmt.Errorf("site.yaml: name is required")
	}
	if c.URL == "" {
		return nil, fmt.Errorf("site.yaml: url is required")
	}
	u, err := url.Parse(c.URL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, fmt.Errorf("site.yaml: url %q is not an http(s) address", c.URL)
	}
	if u.Path != "" && u.Path != "/" {
		return nil, fmt.Errorf("site.yaml: url %q has a path; a site lives at the root of its host", c.URL)
	}
	c.URL = strings.TrimRight(c.URL, "/")
	if c.Language == "" {
		c.Language = "en"
	}
	if c.Collections == nil {
		c.Collections = map[string]CollectionConfig{}
	}
	for name, cc := range c.Collections {
		switch cc.Sort {
		case "", "date", "title", "weight":
		default:
			return nil, fmt.Errorf("site.yaml: collection %s: sort %q is not date, title or weight", name, cc.Sort)
		}
		switch cc.FeedFormat {
		case "", "atom", "rss":
		default:
			return nil, fmt.Errorf("site.yaml: collection %s: feed_format %q is not atom or rss", name, cc.FeedFormat)
		}
	}
	switch c.Markdown.ExternalLinks {
	case "", "new_tab":
	default:
		return nil, fmt.Errorf("site.yaml: markdown.external_links %q: the only option is new_tab", c.Markdown.ExternalLinks)
	}
	for _, r := range c.Redirects {
		if !strings.HasPrefix(r.From, "/") || strings.Contains(r.From, "..") || r.To == "" {
			return nil, fmt.Errorf("site.yaml: redirect %q -> %q: from must be a path starting with /, and to must be set", r.From, r.To)
		}
	}
	for _, t := range c.Taxonomies {
		if t == "" || strings.ContainsAny(t, "/ ") {
			return nil, fmt.Errorf("site.yaml: taxonomy %q is not a plain name", t)
		}
	}
	return &c, nil
}
