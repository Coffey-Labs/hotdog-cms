// Package editor is the hotdog-cms editor: a separate server, never on the host
// that serves the sites, where people sign in with their git platform and
// work on sites through it. It holds no accounts and no content of its own:
// who someone is and what they may do comes from the platform, every change
// is a commit made with their own token, and sessions are encrypted cookies.
package editor

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/forge"
)

// Config is editor.yaml.
type Config struct {
	Listen    string        `yaml:"listen"`     // default 127.0.0.1:8190
	PublicURL string        `yaml:"public_url"` // how browsers reach the editor; the OAuth redirect is built from it
	Cache     string        `yaml:"cache"`      // where site checkouts live; default under the user cache folder
	Forges    []ForgeConfig `yaml:"forges"`
	Sites     []SiteConfig  `yaml:"sites"`
	Live      LiveConfig    `yaml:"live"` // the live preview of pages being edited
}

// ForgeConfig is an OAuth application on a git platform.
type ForgeConfig struct {
	Host            string     `yaml:"host"` // git.example.org, github.com, ...
	Kind            forge.Kind `yaml:"kind"` // gitea, forgejo, github, gitlab, bitbucket
	ClientID        string     `yaml:"client_id"`
	ClientSecretEnv string     `yaml:"client_secret_env"` // the variable holding the client secret
	// Base is the platform's web address if it isn't https://<host>.
	Base string `yaml:"base"`
}

// SiteConfig is a site the editor can open.
type SiteConfig struct {
	ID      string        `yaml:"id"` // in URLs; default from the name
	Name    string        `yaml:"name"`
	Repo    string        `yaml:"repo"`   // https clone URL
	Branch  string        `yaml:"branch"` // the branch the site publishes from; default main
	Subdir  string        `yaml:"subdir"`
	Preview PreviewConfig `yaml:"preview"`
}

// PreviewConfig says where `hotdog-cms preview` serves this repository's branches.
type PreviewConfig struct {
	Domain string `yaml:"domain"` // previews are <branch>.<domain>
	Port   string `yaml:"port"`
	Scheme string `yaml:"scheme"`
}

func (p PreviewConfig) url(slug string) string {
	if p.Domain == "" {
		return ""
	}
	scheme := p.Scheme
	if scheme == "" {
		scheme = "https"
	}
	host := slug + "." + p.Domain
	if p.Port != "" {
		host += ":" + p.Port
	}
	return scheme + "://" + host + "/"
}

// LoadConfig reads editor.yaml.
func LoadConfig(file string) (*Config, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	if c.Listen == "" {
		c.Listen = "127.0.0.1:8190"
	}
	if c.PublicURL == "" {
		c.PublicURL = "http://" + c.Listen
	}
	c.PublicURL = strings.TrimRight(c.PublicURL, "/")
	if u, err := url.Parse(c.PublicURL); err != nil || u.Host == "" {
		return nil, fmt.Errorf("%s: public_url %q is not a URL", file, c.PublicURL)
	}
	if c.Cache == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return nil, err
		}
		c.Cache = filepath.Join(base, "hotdog-cms", "editor")
	}
	hosts := map[string]bool{}
	for i, f := range c.Forges {
		switch f.Kind {
		case forge.Gitea, forge.Forgejo, forge.GitHub, forge.GitLab, forge.Bitbucket:
		default:
			return nil, fmt.Errorf("%s: forges[%d]: kind %q is not gitea, forgejo, github, gitlab or bitbucket", file, i, f.Kind)
		}
		if f.Host == "" || f.ClientID == "" || f.ClientSecretEnv == "" {
			return nil, fmt.Errorf("%s: forges[%d]: host, client_id and client_secret_env are required", file, i)
		}
		hosts[strings.ToLower(f.Host)] = true
	}
	ids := map[string]bool{}
	for i := range c.Sites {
		s := &c.Sites[i]
		if s.Branch == "" {
			s.Branch = "main"
		}
		if s.ID == "" {
			s.ID = slug(s.Name)
		}
		if s.ID == "" || ids[s.ID] {
			return nil, fmt.Errorf("%s: sites[%d]: needs a name, and an id no other site has", file, i)
		}
		ids[s.ID] = true
		if s.Subdir != "" && (filepath.IsAbs(s.Subdir) || strings.Contains(s.Subdir, "..")) {
			return nil, fmt.Errorf("%s: sites[%d]: subdir must be relative, inside the repository", file, i)
		}
		u, err := url.Parse(s.Repo)
		if err != nil || u.Scheme != "https" && u.Scheme != "http" {
			return nil, fmt.Errorf("%s: sites[%d]: repo must be an https clone URL", file, i)
		}
		if !hosts[strings.ToLower(u.Hostname())] {
			return nil, fmt.Errorf("%s: sites[%d]: no forge configured for %s, so nobody could sign in to it", file, i, u.Hostname())
		}
	}
	return &c, nil
}

func slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if b.Len() > 0 && !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func (c *Config) forge(host string) *ForgeConfig {
	for i := range c.Forges {
		if strings.EqualFold(c.Forges[i].Host, host) {
			return &c.Forges[i]
		}
	}
	return nil
}

func (f *ForgeConfig) base() string {
	if f.Base != "" {
		return strings.TrimRight(f.Base, "/")
	}
	return "https://" + f.Host
}
