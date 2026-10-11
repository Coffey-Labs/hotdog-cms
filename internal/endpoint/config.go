// Package endpoint is the one part of hotdog-cms that runs per request: it takes
// form submissions and answers searches for the sites it is configured with.
// It holds no content of its own and needs no database. It is not the CMS:
// it never edits a site, and the CMS never runs here.
package endpoint

import (
	"bytes"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/about"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/forms"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
)

// Config is endpoint.yaml.
type Config struct {
	Listen string `yaml:"listen"`
	// TrustedProxies are the peers whose X-Forwarded-For is believed, such
	// as the reverse proxy in front. Anyone else is identified by their own
	// address, whatever headers they send.
	TrustedProxies []string     `yaml:"trusted_proxies"`
	SMTP           SMTPConfig   `yaml:"smtp"`
	RateLimit      RateLimit    `yaml:"rate_limit"`
	Sites          []SiteConfig `yaml:"sites"`
	// Viewer lets the editor read stored submissions (viewer.go).
	Viewer ViewerConfig `yaml:"viewer"`
	// Source is where people using the endpoint can get its source (the
	// AGPL's section 13), answered at /_hotdog/source. Leave it out when
	// running HotDog CMS as released; set it if you've changed the code.
	Source string `yaml:"source"`
}

// SMTPConfig is how submissions are emailed.
type SMTPConfig struct {
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	Username    string `yaml:"username"`
	PasswordEnv string `yaml:"password_env"` // the variable holding the password; default HOTDOG_SMTP_PASSWORD
	From        string `yaml:"from"`
	// TLS: starttls (the default), tls (port 465 style), or none, which is
	// only allowed to a server on this machine.
	TLS string `yaml:"tls"`
}

// RateLimit caps submissions per visitor address.
type RateLimit struct {
	Count  int           `yaml:"count"`  // default 5
	Window time.Duration `yaml:"window"` // default 10m
}

// SiteConfig is one site the endpoint serves.
type SiteConfig struct {
	Dir    string   `yaml:"dir"`    // the site's source: site.yaml, forms/, layouts/
	Public string   `yaml:"public"` // the built site, for search indexes
	Hosts  []string `yaml:"hosts"`  // default: the host in site.yaml's url, and www.
	Store  string   `yaml:"store"`  // folder for forms with store: true; JSON lines, one file per form
	// Recipients are the only addresses this site's forms may email:
	// "hello@example.org", or "@example.org" for any address there. The
	// forms themselves live in the site, which anyone who can write to it
	// can change; this list is yours.
	Recipients []string `yaml:"recipients"`
}

// allowed says whether a form may send to addr.
func (sc SiteConfig) allowed(addr string) bool {
	if len(sc.Recipients) == 0 {
		return true
	}
	addr = strings.ToLower(strings.TrimSpace(addr))
	for _, r := range sc.Recipients {
		r = strings.ToLower(strings.TrimSpace(r))
		if addr == r || (strings.HasPrefix(r, "@") && strings.HasSuffix(addr, r)) {
			return true
		}
	}
	return false
}

type siteRuntime struct {
	cfg   SiteConfig
	site  *site.Config
	forms map[string]*forms.Form
	hosts []string
}

// LoadConfig reads endpoint.yaml and every site it names.
func LoadConfig(file string) (*Config, map[string]*siteRuntime, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, nil, err
	}
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", file, err)
	}
	if c.Listen == "" {
		c.Listen = "127.0.0.1:8180"
	}
	if c.Source != "" && !about.CheckURL(c.Source) {
		return nil, nil, fmt.Errorf("%s: source %q is not a web address", file, c.Source)
	}
	if c.RateLimit.Count <= 0 {
		c.RateLimit.Count = 5
	}
	if c.RateLimit.Window <= 0 {
		c.RateLimit.Window = 10 * time.Minute
	}
	if c.SMTP.PasswordEnv == "" {
		c.SMTP.PasswordEnv = "HOTDOG_SMTP_PASSWORD"
	}
	switch c.SMTP.TLS {
	case "":
		c.SMTP.TLS = "starttls"
	case "starttls", "tls":
	case "none":
		if !isLoopbackHost(c.SMTP.Host) {
			return nil, nil, fmt.Errorf("%s: smtp tls: none is only allowed to a server on this machine", file)
		}
	default:
		return nil, nil, fmt.Errorf("%s: smtp tls must be starttls, tls or none", file)
	}
	for _, p := range c.TrustedProxies {
		if _, _, err := net.ParseCIDR(p); err != nil {
			return nil, nil, fmt.Errorf("%s: trusted_proxies %q is not a network like 127.0.0.1/32", file, p)
		}
	}
	sites := map[string]*siteRuntime{}
	for i, sc := range c.Sites {
		sc.Dir = abs(file, sc.Dir)
		if sc.Public != "" {
			sc.Public = abs(file, sc.Public)
		}
		if sc.Store != "" {
			sc.Store = abs(file, sc.Store)
		}
		scfg, err := site.LoadConfig(sc.Dir)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: sites[%d]: %w", file, i, err)
		}
		fs, err := forms.Load(sc.Dir)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: sites[%d]: %w", file, i, err)
		}
		hosts := sc.Hosts
		if len(hosts) == 0 {
			u, _ := url.Parse(scfg.URL)
			h := strings.TrimPrefix(u.Hostname(), "www.")
			hosts = []string{h, "www." + h}
		}
		for _, f := range fs {
			if f.To != "" && !sc.allowed(f.To) {
				return nil, nil, fmt.Errorf("%s: %s: form %s sends to %s, which isn't in this site's recipients", file, sc.Dir, f.Name, f.To)
			}
			if f.Store && sc.Store == "" {
				return nil, nil, fmt.Errorf("%s: %s: form %s has store: true but the site has no store folder", file, sc.Dir, f.Name)
			}
		}
		rt := &siteRuntime{cfg: sc, site: scfg, forms: fs, hosts: hosts}
		for _, h := range hosts {
			h = strings.ToLower(h)
			if _, dup := sites[h]; dup {
				return nil, nil, fmt.Errorf("%s: host %s is claimed by two sites", file, h)
			}
			sites[h] = rt
		}
	}
	if len(sites) == 0 {
		return nil, nil, fmt.Errorf("%s: no sites", file)
	}
	if err := c.Viewer.check(); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", file, err)
	}
	return &c, sites, nil
}

func abs(configFile, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(filepath.Dir(configFile), p)
}

func isLoopbackHost(h string) bool {
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}
