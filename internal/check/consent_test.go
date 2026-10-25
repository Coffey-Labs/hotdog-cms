package check

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/build"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/starter"
)

func starterSite(t *testing.T, extra string) string {
	t.Helper()
	dir := t.TempDir()
	err := fs.WalkDir(starter.Files, "site", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dir, strings.TrimPrefix(p, "site"))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, _ := starter.Files.ReadFile(p)
		return os.WriteFile(target, []byte(strings.ReplaceAll(string(data), "{{SITE_NAME}}", "Test")), 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	f, _ := os.OpenFile(filepath.Join(dir, "site.yaml"), os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString(extra)
	f.Close()
	return dir
}

func TestConsentAndPrivacy(t *testing.T) {
	dir := starterSite(t, "analytics: { provider: plausible, domain: example.org, src: \"https://stats.example.net/js/script.js\" }\nprivacy: { controller: Test Co, contact: privacy@example.org }\n")
	res, err := build.Run(build.Options{SiteDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.ReadFile(filepath.Join(res.Out, "index.html"))
	h := string(home)
	for _, want := range []string{`data-hotdog-consent data-provider="plausible" data-domain="example.org" data-src="https://stats.example.net/js/script.js" data-privacy="/privacy/" data-hosts="https://stats.example.net"`, `/hotdog-consent.`, `data-hotdog-consent-open>Cookie settings</button>`} {
		if !strings.Contains(h, want) {
			t.Errorf("home missing %s", want)
		}
	}
	if strings.Contains(h, `<script src="https://stats.example.net`) {
		t.Error("the counter loads on page view")
	}
	priv, _ := os.ReadFile(filepath.Join(res.Out, "privacy", "index.html"))
	p := string(priv)
	for _, want := range []string{"Visit statistics", "Plausible (stats.example.net)", "Your consent", "The Contact form: what you write in it", "Test Co", `<!--email_off--><a href="mailto:privacy@example.org">`} {
		if !strings.Contains(p, want) {
			t.Errorf("privacy page missing %q", want)
		}
	}
	rep, err := Run(dir, res.Out, res.Site.Config)
	if err != nil {
		t.Fatal(err)
	}
	for _, pr := range rep.Problems {
		if pr.Rule == "tracker-before-consent" || pr.Rule == "analytics-ungated" || pr.Rule == "privacy-page" || pr.Rule == "privacy-contact" {
			t.Errorf("unexpected: %v", pr)
		}
	}
	if !strings.Contains(rep.CSP, "script-src 'self' https://stats.example.net") || !strings.Contains(rep.CSP, "connect-src 'self' https://stats.example.net") {
		t.Errorf("CSP: %s", rep.CSP)
	}

	// Counting without asking is a recorded choice, and still a warning.
	dir = starterSite(t, "analytics: { provider: ga4, id: G-ABC123, gated: false }\n")
	res, err = build.Run(build.Options{SiteDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	rep, _ = Run(dir, res.Out, res.Site.Config)
	n := 0
	for _, pr := range rep.Problems {
		if pr.Rule == "analytics-ungated" {
			n++
			if pr.Level != Warning {
				t.Errorf("level %s", pr.Level)
			}
		}
	}
	if n == 0 {
		t.Error("no analytics-ungated warning")
	}

	// Measuring Google ads: the banner and the notice say so, and the policy
	// lets Google's ad hosts through.
	dir = starterSite(t, "analytics: { provider: ga4, id: G-ABC123, ad_conversions: true }\n")
	res, err = build.Run(build.Options{SiteDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	home, _ = os.ReadFile(filepath.Join(res.Out, "index.html"))
	if !strings.Contains(string(home), `data-ads="true"`) {
		t.Error("home missing data-ads")
	}
	priv, _ = os.ReadFile(filepath.Join(res.Out, "privacy", "index.html"))
	if !strings.Contains(string(priv), "Google Ads, through Google Analytics") {
		t.Error("privacy page doesn't mention Google Ads")
	}
	rep, _ = Run(dir, res.Out, res.Site.Config)
	if !strings.Contains(rep.CSP, "https://www.googleadservices.com") {
		t.Errorf("CSP: %s", rep.CSP)
	}

	// A bad analytics setting stops the build, saying why.
	for setting, want := range map[string]string{
		"analytics: { provider: ga4, id: UA-1 }\n":                                         "G-ABC123",
		"analytics: { provider: ga4, id: G-ABC123, gated: false, ad_conversions: true }\n": "needs consent",
		"analytics: { provider: plausible, domain: example.org, ad_conversions: true }\n":  "is for ga4",
	} {
		dir = starterSite(t, setting)
		if _, err := build.Run(build.Options{SiteDir: dir}); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", setting, err)
		}
	}
}
