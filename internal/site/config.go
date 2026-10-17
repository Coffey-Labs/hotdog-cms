// Package site loads a site directory: its configuration, its content and its
// data files. It knows nothing about templates or output; the build package
// turns what this produces into files.
package site

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is site.yaml.
type Config struct {
	Name        string `yaml:"name"`
	URL         string `yaml:"url"`
	Language    string `yaml:"language"`
	Description string `yaml:"description"`
	// Timezone is where the site's events happen (an IANA name such as
	// America/Phoenix): event times without a zone are read in it, and
	// pages show them in it. Default UTC.
	Timezone string `yaml:"timezone"`
	Author   string `yaml:"author"`
	// Copyright is who holds the copyright the footer shows; default the
	// site's name.
	Copyright string `yaml:"copyright"`
	// CopyrightURL, if set, links the copyright holder's name.
	CopyrightURL string `yaml:"copyright_url"`
	// Credit shows "Built with HotDog CMS" in the footer, linking to
	// hotdogcms.org.
	Credit      bool                        `yaml:"credit"`
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
	TermIndexMin int        `yaml:"term_index_min"`
	Redirects    []Redirect `yaml:"redirects"`
	// Analytics counts visits, only after a visitor agrees (consent.go in
	// the build). Privacy is who answers for the site's data, for the notice.
	Analytics *AnalyticsConfig `yaml:"analytics"`
	Privacy   PrivacyConfig    `yaml:"privacy"`
	// Verify holds search consoles' site-verification codes, written as
	// meta tags by {{ verificationTags }}. IndexNow tells search engines
	// which pages changed when the site is published (publish targets with
	// indexnow: true).
	Verify   VerifyConfig   `yaml:"verify"`
	IndexNow IndexNowConfig `yaml:"indexnow"`
	// Packs are the extensions installed in the site (internal/packs): what
	// each put where, so it can be upgraded or removed.
	Packs []PackRef `yaml:"packs"`
	// Fediverse is the author's handle (@name@instance), which Mastodon
	// credits in link previews: <meta name="fediverse:creator">.
	Fediverse string `yaml:"fediverse"`
	// Cart turns on the shop's cart (cart: true, or cart: { page: /basket/ }):
	// HotDog's cart script, kept in the visitor's browser, for the shop pack.
	Cart *CartConfig `yaml:"cart"`
	// Cards draws a social card for every page without its own image
	// (`cards: true`, or a map to choose the font, colors and logo).
	Cards *CardsConfig `yaml:"cards"`
	// Look is the site's choices for what its theme's look.yaml lets change:
	// colors, fonts, the logo (look.go).
	Look    map[string]any `yaml:"look"`
	Outputs OutputsConfig  `yaml:"outputs"`
	Check   CheckConfig    `yaml:"check"`
	Params  map[string]any `yaml:"params"`
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
	// ImageSizes is the sizes attribute given to pictures from media/, telling
	// browsers how wide they'll show so they fetch the right file. The default
	// suits a text column up to 50rem wide.
	ImageSizes string `yaml:"image_sizes"`
	// HeadingIDs picks how headings get their anchors: the default, or
	// mkdocs for the ones MkDocs gives (on-linux-bsd for "On Linux & BSD"),
	// so links into a site moved from it still land.
	HeadingIDs string `yaml:"heading_ids"`
	// Keys names keys for ++ctrl+k++ shortcuts, where the name shown isn't
	// the key's own capitalized: {hash: "#"} shows ++hash++ as #. MkDocs'
	// pymdownx.keys calls it key_map.
	Keys map[string]string `yaml:"keys"`
	// Highlight colors code blocks that name their language (```go) when the
	// site is built (highlight.go). On unless set to false.
	Highlight *bool `yaml:"highlight"`
}

// AutolinkOn reports whether bare URLs become links.
func (m MarkdownConfig) AutolinkOn() bool { return m.Autolink == nil || *m.Autolink }

// HighlightOn reports whether code blocks are colored.
func (m MarkdownConfig) HighlightOn() bool { return m.Highlight == nil || *m.Highlight }

// Redirect sends one old address somewhere else.
type Redirect struct {
	From string `yaml:"from" json:"from"`
	To   string `yaml:"to" json:"to"`
}

// CollectionConfig is the per-folder setting for a collection such as
// content/articles/. Every folder of Markdown is a collection whether it is
// named here or not; this only changes the defaults.
type CollectionConfig struct {
	Layout string `yaml:"layout"` // layout for the collection's pages
	Sort   string `yaml:"sort"`   // date (newest first, the default), title or weight
	// Path is where the collection's pages live: /docs/ by default for
	// content/docs/, / for a docs site whose pages sit at the root of its
	// host. Its search index, feed and calendar stay at /<collection>/.
	Path     string `yaml:"path"`
	Feed     bool   `yaml:"feed"`      // write an Atom feed
	FeedPath string `yaml:"feed_path"` // where; default /<collection>/feed.xml
	FeedSize int    `yaml:"feed_size"` // entries in the feed; default 20
	// FeedFormat is atom (the default), rss, or podcast: RSS with the
	// iTunes fields podcast apps read, an enclosure per episode's audio.
	FeedFormat string `yaml:"feed_format"`
	// For feed_format: podcast. The category is one of Apple's (Arts,
	// Business, Comedy, Education, ...); email is where directories write to
	// the show's owner; explicit marks the whole show as explicit.
	PodcastCategory string `yaml:"podcast_category"`
	PodcastEmail    string `yaml:"podcast_email"`
	PodcastExplicit bool   `yaml:"podcast_explicit"`
	FeedTitle       string `yaml:"feed_title"`
	// FeedImage is the channel's logo in an RSS feed, a path in static/.
	FeedImage       string `yaml:"feed_image"`
	FeedDescription string `yaml:"feed_description"`
	// Paginate overrides the site's paginate for this collection's list; 0
	// keeps it on one page, for a list nobody pages through (the docs').
	Paginate *int `yaml:"paginate"`
	// ListLayout is the layout for the collection's own page (/<collection>/);
	// default list.
	ListLayout string `yaml:"list_layout"`
	// Calendar writes /<collection>/calendar.ics, a feed of the collection's
	// events that calendar apps subscribe to (webcal://). Its pages are
	// sorted by start unless sort says otherwise.
	Calendar     bool   `yaml:"calendar"`
	CalendarName string `yaml:"calendar_name"` // what calendar apps call it; default "<site>: <collection>"
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
	if c.Cart.On() && c.Cart.Page != "" && (!strings.HasPrefix(c.Cart.Page, "/") || strings.HasPrefix(c.Cart.Page, "//")) {
		return nil, fmt.Errorf("site.yaml: cart page %q is an address on the site, starting with /", c.Cart.Page)
	}
	if err := c.Analytics.Validate(); err != nil {
		return nil, fmt.Errorf("site.yaml: analytics: %w", err)
	}
	if k := c.IndexNow.Key; k != "" && !indexNowKey.MatchString(k) {
		return nil, fmt.Errorf("site.yaml: indexnow.key should be 8 to 128 letters, digits or dashes")
	}
	for name, v := range map[string]string{"google": c.Verify.Google, "bing": c.Verify.Bing, "yandex": c.Verify.Yandex} {
		if v != "" && !verifyCode.MatchString(v) {
			return nil, fmt.Errorf("site.yaml: verify.%s should be the code only (the content of the meta tag), not the whole tag", name)
		}
	}
	if c.Fediverse != "" && !fediverseRe.MatchString(c.Fediverse) {
		return nil, fmt.Errorf("site.yaml: fediverse %q should be a handle like @name@instance.social", c.Fediverse)
	}
	if c.Language == "" {
		c.Language = "en"
	}
	if c.Copyright == "" {
		c.Copyright = c.Name
	}
	if c.CopyrightURL != "" && !strings.HasPrefix(c.CopyrightURL, "https://") && !strings.HasPrefix(c.CopyrightURL, "http://") && !strings.HasPrefix(c.CopyrightURL, "/") {
		return nil, fmt.Errorf("site.yaml: copyright_url %q should be a web address", c.CopyrightURL)
	}
	if c.Collections == nil {
		c.Collections = map[string]CollectionConfig{}
	}
	for name, cc := range c.Collections {
		switch cc.Sort {
		case "", "date", "title", "weight", "start":
		default:
			return nil, fmt.Errorf("site.yaml: collection %s: sort %q is not date, title, weight or start", name, cc.Sort)
		}
		if cc.Path != "" && (!strings.HasPrefix(cc.Path, "/") || !strings.HasSuffix(cc.Path, "/") || !SafeURLPath(cc.Path)) {
			return nil, fmt.Errorf("site.yaml: collection %s: path %q should be a folder on the site, like / or /%s/", name, cc.Path, name)
		}
		if cc.FeedPath != "" && !SafeURLPath(cc.FeedPath) {
			return nil, fmt.Errorf("site.yaml: collection %s: feed_path %q should be a path on the site, like /%s/feed.xml", name, cc.FeedPath, name)
		}
		switch cc.FeedFormat {
		case "", "atom", "rss", "podcast":
		default:
			return nil, fmt.Errorf("site.yaml: collection %s: feed_format %q is not atom, rss or podcast", name, cc.FeedFormat)
		}
	}
	switch c.Markdown.HeadingIDs {
	case "", "default", "mkdocs":
	default:
		return nil, fmt.Errorf("site.yaml: markdown.heading_ids %q is not default or mkdocs", c.Markdown.HeadingIDs)
	}
	switch c.Markdown.ExternalLinks {
	case "", "new_tab":
	default:
		return nil, fmt.Errorf("site.yaml: markdown.external_links %q: the only option is new_tab", c.Markdown.ExternalLinks)
	}
	for _, r := range c.Redirects {
		if !SafeURLPath(r.From) || !safeRedirectTo(r.To) {
			return nil, fmt.Errorf("site.yaml: redirect %q -> %q: from must be a path on the site (/old-page/), and to a path or a web address", r.From, r.To)
		}
	}
	for _, t := range c.Taxonomies {
		if t == "" || strings.ContainsAny(t, "/ ") {
			return nil, fmt.Errorf("site.yaml: taxonomy %q is not a plain name", t)
		}
	}
	return &c, nil
}

// SafeURLPath accepts an address on the site that a build can safely turn
// into a file: absolute, clean, no "..", and only characters that need no
// escaping in a web server's configuration.
func SafeURLPath(p string) bool {
	if !strings.HasPrefix(p, "/") || strings.Contains(p, "..") || strings.Contains(p, "//") {
		return false
	}
	clean := path.Clean(p)
	if strings.HasSuffix(p, "/") && clean != "/" {
		clean += "/"
	}
	return clean == p && urlPathRe.MatchString(p)
}

var (
	urlPathRe = regexp.MustCompile(`^/[\p{L}\p{N}._~%/@+=-]*$`)
	// Redirect targets go into nginx and Caddy configuration: a path, or an
	// http(s) address, with nothing a config file would read as syntax.
	redirectToRe = regexp.MustCompile(`^(https?://[A-Za-z0-9.-]+(:[0-9]+)?)?/[A-Za-z0-9._~%/@+=?&:-]*$|^https?://[A-Za-z0-9.-]+(:[0-9]+)?$`)
)

func safeRedirectTo(to string) bool {
	return redirectToRe.MatchString(to) && !strings.Contains(to, "..")
}

// CardsConfig sets up social cards. Colors default to the site's look (its
// band and band text), the fonts to the Go fonts, the logo to look's logo.
type CardsConfig struct {
	Enabled    bool   `yaml:"enabled"`
	Font       string `yaml:"font"`       // a .ttf or .otf in the site, for the title
	FontSmall  string `yaml:"font_small"` // for the site's name
	Background string `yaml:"background"`
	Text       string `yaml:"text"`
	Accent     string `yaml:"accent"`
	Logo       string `yaml:"logo"` // a PNG or JPEG in static/ or assets/
}

// UnmarshalYAML takes `cards: true` as well as the map form.
func (c *CardsConfig) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		return n.Decode(&c.Enabled)
	}
	type plain CardsConfig
	c.Enabled = true
	return n.Decode((*plain)(c))
}

// On reports whether cards are drawn.
func (c *CardsConfig) On() bool { return c != nil && c.Enabled }

var fediverseRe = regexp.MustCompile(`^@[^@\s]+@[^@\s]+\.[^@\s]+$`)

// AnalyticsConfig sets up visit counting. It starts only after a visitor
// agrees, through the consent banner, unless gated is set to false: an
// explicit choice, which `hotdog-cms check` still warns about.
type AnalyticsConfig struct {
	Provider string `yaml:"provider" json:"provider"`        // plausible, matomo or ga4
	Domain   string `yaml:"domain" json:"domain,omitempty"`  // plausible: the site's domain as Plausible knows it
	Src      string `yaml:"src" json:"src,omitempty"`        // plausible: the script, if self-hosted
	URL      string `yaml:"url" json:"url,omitempty"`        // matomo: where Matomo runs
	SiteID   string `yaml:"site_id" json:"siteId,omitempty"` // matomo
	ID       string `yaml:"id" json:"id,omitempty"`          // ga4: G-…
	Gated    *bool  `yaml:"gated" json:"gated,omitempty"`    // default true
}

// IsGated reports whether analytics waits for consent.
func (a *AnalyticsConfig) IsGated() bool { return a == nil || a.Gated == nil || *a.Gated }

// Validate says what's wrong with the settings, or nothing.
func (a *AnalyticsConfig) Validate() error {
	if a == nil {
		return nil
	}
	https := func(name, v string) error {
		if v != "" && !strings.HasPrefix(v, "https://") {
			return fmt.Errorf("%s %q should start with https://", name, v)
		}
		return nil
	}
	switch a.Provider {
	case "plausible":
		if a.Domain == "" {
			return fmt.Errorf("plausible needs domain:, the site as Plausible knows it")
		}
		return https("src", a.Src)
	case "matomo":
		if a.URL == "" || a.SiteID == "" {
			return fmt.Errorf("matomo needs url: and site_id:")
		}
		return https("url", a.URL)
	case "ga4":
		if !regexp.MustCompile(`^G-[A-Z0-9]+$`).MatchString(a.ID) {
			return fmt.Errorf("ga4 needs id: like G-ABC123")
		}
		return nil
	default:
		return fmt.Errorf("provider %q isn't plausible, matomo or ga4", a.Provider)
	}
}

// PrivacyConfig is what the privacy notice says about who's responsible.
type PrivacyConfig struct {
	Controller string        `yaml:"controller"` // the person or organization responsible
	Contact    string        `yaml:"contact"`    // an email address to write to
	Updated    string        `yaml:"updated"`    // when the notice last changed, as you'd write it
	Extra      []PrivacyFact `yaml:"extra"`      // anything else the site uses
}

// PrivacyFact is one thing a site uses that touches visitors' data.
type PrivacyFact struct {
	What  string `yaml:"what"`
	Who   string `yaml:"who"`
	Why   string `yaml:"why"`
	Basis string `yaml:"basis"` // the lawful basis: consent, legitimate interest, contract…
}

// VerifyConfig is what each search console gave for its meta-tag method.
type VerifyConfig struct {
	Google string `yaml:"google" json:"google,omitempty"` // google-site-verification
	Bing   string `yaml:"bing" json:"bing,omitempty"`     // msvalidate.01
	Yandex string `yaml:"yandex" json:"yandex,omitempty"` // yandex-verification
}

// IndexNowConfig holds the site's IndexNow key; the build publishes it as
// /<key>.txt, which is how search engines know a ping is the site's own.
type IndexNowConfig struct {
	Key string `yaml:"key" json:"key,omitempty"`
}

var (
	indexNowKey = regexp.MustCompile(`^[A-Za-z0-9-]{8,128}$`)
	verifyCode  = regexp.MustCompile(`^[A-Za-z0-9_.=+/-]{4,200}$`)
)

// PackRef records an installed pack.
type PackRef struct {
	Name    string   `yaml:"name" json:"name"`
	Version string   `yaml:"version" json:"version"`
	Source  string   `yaml:"source,omitempty" json:"source,omitempty"`
	Commit  string   `yaml:"commit,omitempty" json:"commit,omitempty"`
	Files   []string `yaml:"files" json:"files"`

	// From the last install, for showing; never written to site.yaml.
	AddedCollections []string `yaml:"-" json:"-"`
	Notes            string   `yaml:"-" json:"-"`
	Seeded           []string `yaml:"-" json:"-"` // forms copied in, now the site's own
	Kept             []string `yaml:"-" json:"-"` // forms the site already had, left as they were
}

// CartConfig turns the cart on and says where its page is.
type CartConfig struct {
	Enabled bool   `yaml:"enabled"`
	Page    string `yaml:"page"` // the cart page's address; /cart/ unless set
}

// UnmarshalYAML takes `cart: true` as well as the map form.
func (c *CartConfig) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		return n.Decode(&c.Enabled)
	}
	type plain CartConfig
	c.Enabled = true
	return n.Decode((*plain)(c))
}

// On reports whether the cart is on.
func (c *CartConfig) On() bool { return c != nil && c.Enabled }

// PagePath is the cart page's address.
func (c *CartConfig) PagePath() string {
	if c == nil || c.Page == "" {
		return "/cart/"
	}
	return c.Page
}
