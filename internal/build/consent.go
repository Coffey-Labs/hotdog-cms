package build

import (
	_ "embed"
	"html/template"
	"net/url"
	"sort"
	"strings"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/forms"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
)

// Consent and the privacy notice. With analytics: in site.yaml, the build
// publishes a small consent script and stylesheet (no inline code, so a
// strict CSP holds), and templates get:
//
//	{{ consent }}        the two tags, in <head>
//	{{ consentActive }}  whether to offer "Cookie settings"
//	{{ privacyFacts }}   what the site uses that touches visitors' data, for
//	                     the privacy notice: built from the configuration, so
//	                     the notice can't fall behind it
//
// The consent script's tag carries the counter's hosts (data-hosts), so the
// checks and the Content-Security-Policy see them in the page.

var (
	//go:embed static/hotdog-consent.js
	consentScript []byte
	//go:embed static/hotdog-consent.css
	consentStyle []byte
)

var providerNames = map[string]string{"plausible": "Plausible", "matomo": "Matomo", "ga4": "Google Analytics"}

func origin(u string) string {
	p, err := url.Parse(u)
	if err != nil || p.Host == "" {
		return ""
	}
	return p.Scheme + "://" + p.Host
}

// analyticsHosts are the origins a counter loads from and reports to.
func analyticsHosts(a *site.AnalyticsConfig) []string {
	switch a.Provider {
	case "plausible":
		if a.Src != "" {
			return []string{origin(a.Src)}
		}
		return []string{"https://plausible.io"}
	case "matomo":
		return []string{origin(a.URL)}
	case "ga4":
		return []string{"https://www.googletagmanager.com", "https://www.google-analytics.com", "https://*.google-analytics.com", "https://*.analytics.google.com"}
	}
	return nil
}

func privacyPage(cfg *site.Config) string {
	if cfg.Check.PrivacyPage != "" {
		return cfg.Check.PrivacyPage
	}
	return "/privacy/"
}

// consentTags is what {{ consent }} writes.
func consentTags(cfg *site.Config, a *Assets) template.HTML {
	an := cfg.Analytics
	if an == nil {
		return ""
	}
	js, _ := a.URL("hotdog-consent.js")
	css, _ := a.URL("hotdog-consent.css")
	attrs := [][2]string{
		{"data-provider", an.Provider}, {"data-domain", an.Domain}, {"data-src", an.Src}, {"data-url", an.URL},
		{"data-site", an.SiteID}, {"data-id", an.ID}, {"data-privacy", privacyPage(cfg)},
		{"data-hosts", strings.Join(analyticsHosts(an), " ")},
	}
	if !an.IsGated() {
		attrs = append(attrs, [2]string{"data-gated", "false"})
	}
	var b strings.Builder
	b.WriteString(`<link rel="stylesheet" href="` + template.HTMLEscapeString(css) + `">`)
	b.WriteString(`<script src="` + template.HTMLEscapeString(js) + `" defer data-hotdog-consent`)
	for _, kv := range attrs {
		if kv[1] != "" {
			b.WriteString(" " + kv[0] + `="` + template.HTMLEscapeString(kv[1]) + `"`)
		}
	}
	b.WriteString(`></script>`)
	return template.HTML(b.String())
}

// privacyFacts lists what the site uses that touches visitors' data.
func privacyFacts(cfg *site.Config, siteForms map[string]*forms.Form) []site.PrivacyFact {
	var out []site.PrivacyFact
	if an := cfg.Analytics; an != nil {
		basis := "Your consent, asked for on your first visit; change it any time with Cookie settings."
		if !an.IsGated() {
			basis = "Legitimate interest in knowing which pages are read."
		}
		hosts := analyticsHosts(an)
		who := providerNames[an.Provider]
		if len(hosts) > 0 {
			who += " (" + strings.TrimPrefix(hosts[0], "https://") + ")"
		}
		out = append(out, site.PrivacyFact{What: "Visit statistics", Who: who, Why: "To count visits and see which pages are read.", Basis: basis})
	}
	names := make([]string, 0, len(siteForms))
	for n := range siteForms {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		f := siteForms[n]
		title := f.Title
		if title == "" {
			title = n
		}
		who := "Sent by email to the site's owner."
		if f.Store {
			who = "Sent by email to the site's owner, and kept on the site's server."
		}
		if f.Turnstile != "" {
			who += " Cloudflare Turnstile checks that a person sent it."
		}
		out = append(out, site.PrivacyFact{What: "The " + title + " form: what you write in it", Who: who, Why: "To answer you.", Basis: "Your request: nothing is sent unless you send it."})
	}
	if cfg.Cart.On() {
		out = append(out, site.PrivacyFact{What: "What you put in your cart", Who: "Kept in your own browser (local storage); it reaches the shop only when you check out.", Why: "To remember your cart between pages.", Basis: "Your request: you added it."})
	}
	for _, h := range cfg.Check.AllowThirdParty {
		out = append(out, site.PrivacyFact{What: "Content from " + h, Who: h, Why: "Part of what some pages show.", Basis: "Legitimate interest; " + h + " sees your address when your browser fetches it."})
	}
	return append(out, cfg.Privacy.Extra...)
}
