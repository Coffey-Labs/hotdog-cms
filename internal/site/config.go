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
	Outputs     OutputsConfig               `yaml:"outputs"`
	Check       CheckConfig                 `yaml:"check"`
	Params      map[string]any              `yaml:"params"`
}

// MarkdownConfig controls how page bodies are rendered.
type MarkdownConfig struct {
	// UnsafeHTML lets raw HTML in Markdown through, after a sanitizer. Off by
	// default: a site whose pages are written by several people should not
	// have to trust every one of them with script tags.
	UnsafeHTML bool `yaml:"unsafe_html"`
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
}

// MenuItem is one link in a menu from site.yaml.
type MenuItem struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
	Icon string `yaml:"icon"`
}

// OutputsConfig switches the generated files.
type OutputsConfig struct {
	Sitemap *bool `yaml:"sitemap"` // default on
	Robots  *bool `yaml:"robots"`  // default on, unless static/robots.txt exists
	LLMs    bool  `yaml:"llms"`    // llms.txt, default off
}

// CheckConfig is what `cl-cms check` refuses.
type CheckConfig struct {
	// Forbid lists regular expressions that must not appear in the source or
	// the built site: internal host names, personal addresses, and so on.
	Forbid []string `yaml:"forbid"`
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
	}
	for _, t := range c.Taxonomies {
		if t == "" || strings.ContainsAny(t, "/ ") {
			return nil, fmt.Errorf("site.yaml: taxonomy %q is not a plain name", t)
		}
	}
	return &c, nil
}
