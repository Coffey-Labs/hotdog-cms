package check

// Fixes says, for each rule, what usually puts it right: shown beside every
// finding, in the editor and by `hotdog-cms check`, so a finding is never
// just a complaint.
var Fixes = map[string]string{
	"tracker-before-consent": "Load analytics only after a visitor agrees: set analytics: in site.yaml (it's gated by the consent banner), or start it from your own consent code.",
	"third-party":            "If the site really should load it, add the host to check.allow_third_party in site.yaml and name it in the privacy notice; otherwise host the file yourself.",
	"privacy-page":           "Add a privacy notice at /privacy/ (or set check.privacy_page): content/privacy.md, which can list what the site loads with {{ range privacyFacts }}.",
	"privacy-contact":        "Put an email address on the privacy notice, so people can reach whoever is responsible for their data.",
	"inline-script":          "Move the script into a file under assets/ and load it with <script src=\"{{ asset \"x.js\" }}\">.",
	"inline-style":           "Move the styles into assets/styles.css, or a class; for a site's own colors, use look.yaml.",
	"inline-handler":         "Attach the handler from a script file with addEventListener instead of an on… attribute.",
	"mixed-content":          "Load it over https://, or host it on the site.",
	"broken-link":            "Fix the address, add the missing page, or add a redirect (redirects: in site.yaml) from the old address.",
	"forbidden-string":       "Remove it from the page or file; it matches a pattern in check.forbid that must never be published.",
	"cloudflare-email":       "Turn on markdown.email_off in site.yaml, or wrap the address in <!--email_off--> and <!--/email_off-->.",
	"missing-title":          "Give the page a title in its front matter.",
	"missing-description":    "Add a summary to the page's front matter; it becomes the description search results show.",
	"description-length":     "Shorten the summary to about 160 characters, the most a search result shows.",
	"missing-canonical":      "Have the layout's head include <link rel=\"canonical\" href=\"{{ .Permalink }}\">.",
	"social-image":           "Set image: in the front matter, or turn on social cards (cards: in site.yaml) so every page gets one.",
	"social-image-alt":       "Set image_alt: in the front matter to describe the picture.",
	"duplicate-title":        "Give each page its own title, so search results can tell them apart.",
	"duplicate-description":  "Give each page its own summary.",
	"noindex-in-sitemap":     "Leave the page out of the sitemap, or let it be indexed.",
	"robots-sitemap":         "Add static/robots.txt with a line: Sitemap: https://your.site/sitemap.xml",
	"security-txt":           "Add static/.well-known/security.txt with Contact: and an Expires: date within a year (RFC 9116).",
	"img-alt":                "Add alt text to the image, or alt=\"\" if it's only decoration.",
	"image-location":         "Upload the picture through the editor (it removes location data), or strip it: exiftool -gps:all= FILE",
	"image-weight":           "Upload the picture through the editor, which resizes it and makes WebP copies, or shrink it before committing.",
	"analytics-ungated":      "Remove gated: false from analytics: in site.yaml, so the consent banner asks first; if you keep it, say so in the privacy notice.",
	"look-contrast":          "Pick colors further apart in the Look panel (it shows the ratio as you go); text needs 4.5:1.",
}
