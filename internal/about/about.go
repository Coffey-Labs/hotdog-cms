// Package about says what this program is, who makes it, and where its
// source is. HotDog CMS is under the GNU Affero General Public License: anyone
// who uses the editor or the endpoint over a network may have the source of
// the version they're using (section 13). The editor shows a link to it on
// every screen, and the endpoint answers /_hotdog/source.
//
// Someone running a modified copy must point the link at their own source:
// `source:` in editor.yaml and in the endpoint's configuration.
package about

import (
	"net/url"
	"runtime/debug"
	"strings"
)

const (
	Name          = "HotDog CMS"
	Copyright     = "© 2026 Coffey Labs LLC"
	CompanyURL    = "https://coffeylabs.org"
	DefaultSource = "https://git.coffeylabs.org/coffey-labs/hotdog-cms"
	License       = "AGPL-3.0-or-later"
	LicenseURL    = "https://www.gnu.org/licenses/agpl-3.0.html"
)

// Version is set at release: -ldflags "-X git.coffeylabs.org/coffey-labs/hotdog-cms/internal/about.Version=v1.2.3".
var Version = "dev"

// Revision is the commit this binary was built from, if go build recorded
// it, and whether the working tree had changes that commit doesn't hold.
func Revision() (rev string, modified bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	return rev, modified
}

// Info is what the editor shows in its footer.
type Info struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	Revision   string `json:"revision,omitempty"`
	Modified   bool   `json:"modified,omitempty"`
	Copyright  string `json:"copyright"`
	CompanyURL string `json:"companyURL"`
	Source     string `json:"source"`
	License    string `json:"license"`
	LicenseURL string `json:"licenseURL"`
	Links      []Link `json:"links,omitempty"`
}

// Link is one of the operator's own links (privacy notice, terms, help).
type Link struct {
	Name string `yaml:"name" json:"name"`
	URL  string `yaml:"url" json:"url"`
}

// Get describes this binary. source is the operator's source address, or
// empty for ours; with ours, the link goes to the exact commit when it's
// known and unmodified.
func Get(source string, links []Link) Info {
	rev, mod := Revision()
	i := Info{Name: Name, Version: Version, Revision: rev, Modified: mod, Copyright: Copyright, CompanyURL: CompanyURL,
		Source: source, License: License, LicenseURL: LicenseURL, Links: links}
	if i.Source == "" {
		i.Source = DefaultSource
		if rev != "" && !mod {
			i.Source += "/src/commit/" + rev
		}
	}
	return i
}

// CheckURL accepts an https (or http) address for a link.
func CheckURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && !strings.ContainsAny(s, " \n\"'<>")
}
