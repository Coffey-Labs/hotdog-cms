package packs

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/build"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/check"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
)

// Every bundled pack installs into the starter, builds with real content,
// passes the checks with no errors, and renders what it promises.
func TestBundledPacksBuildAndPass(t *testing.T) {
	picture := func() []byte {
		img := image.NewRGBA(image.Rect(0, 0, 8, 6))
		for x := 0; x < 8; x++ {
			for y := 0; y < 6; y++ {
				img.Set(x, y, color.RGBA{200, 120, 40, 255})
			}
		}
		var b bytes.Buffer
		png.Encode(&b, img)
		return b.Bytes()
	}()
	type page struct{ path, body string }
	cases := []struct {
		pack  string
		files []page
		want  map[string][]string // built file -> what it must contain
	}{
		{"faq", []page{
			{"content/help.md", "---\ntitle: Help\nsummary: Questions people ask.\nsections:\n  - faq:\n      heading: Questions\n      open: true\n      items:\n        - question: Do you deliver?\n          answer: Yes, within **five** miles.\n        - question: Can I pay by card?\n          answer: Yes.\n---\n"},
			{"data/faq.yaml", "- question: Shared question?\n  answer: A shared answer.\n"},
			{"content/shared.md", "---\ntitle: Shared\nsummary: Questions from a file.\nsections:\n  - faq: { from: faq }\n---\n"},
		}, map[string][]string{
			"help/index.html":   {`<details class="faq-item" open>`, "Do you deliver?", "<strong>five</strong>", "packs/faq/faq."},
			"shared/index.html": {"Shared question?", "A shared answer."},
		}},
		{"testimonials", []page{
			{"content/kind-words.md", "---\ntitle: Kind words\nsummary: What people say.\nsections:\n  - testimonials:\n      items:\n        - { quote: The best sourdough in town., name: Maria Lopez, role: Customer since 2019 }\n---\n"},
		}, map[string][]string{
			"kind-words/index.html": {`class="tm-item"`, "The best sourdough in town.", "Maria Lopez", "Customer since 2019"},
		}},
		{"team", []page{
			{"content/people/_index.md", "---\ntitle: Our team\nsummary: The people who bake.\n---\n"},
			{"content/people/sam-rivera.md", "---\ntitle: Sam Rivera\nsummary: Head baker.\nrole: Head baker\nweight: 1\nemail: sam@example.org\nlinks:\n  - { name: Mastodon, url: \"https://example.social/@sam\" }\n---\nSam bakes.\n"},
			{"content/people/emile-durand.md", "---\ntitle: Émile Durand\nsummary: Pastry.\nrole: Pastry\nweight: 2\n---\n"},
			{"content/people/ana.md", "---\ntitle: Ana\nsummary: Front of house.\nphoto: /media/a.png\nphoto_alt: Ana at the counter\nweight: 3\n---\n"},
			{"content/about-us.md", "---\ntitle: About us\nsummary: Who we are.\nsections:\n  - team: { heading: The team }\n---\n"},
		}, map[string][]string{
			"people/index.html":            {"Our team", `href="/people/sam-rivera/"`, "Head baker"},
			"people/sam-rivera/index.html": {"mailto:sam@example.org", `rel="me"`, "team-initial", ">S<"},
			"about-us/index.html":          {`href="/people/emile-durand/"`, ">É<", `src="/media/a.png" alt="Ana at the counter"`},
		}},
		{"location-and-hours", []page{
			{"data/location.yaml", "name: Bakery on Main\naddress: |\n  12 Main Street\n  Springfield, IL 62701\nphone: \"(217) 555-0142\"\nemail: hello@example.org\nhours:\n  - { days: Monday to Friday, times: 7am to 6pm }\n  - { days: Sunday, times: Closed }\nnote: Closed on public holidays.\n"},
			{"content/visit.md", "---\ntitle: Visit\nsummary: Where and when.\nsections:\n  - location: { heading: Visit us }\n---\n"},
			{"content/market.md", "---\ntitle: Market stall\nsummary: Saturdays.\nsections:\n  - location:\n      address: Town Square\n      hours: [{ days: Saturday, times: 8am to noon }]\n---\n"},
		}, map[string][]string{
			"visit/index.html":  {"Bakery on Main", "https://www.openstreetmap.org/search?query=12&#43;Main&#43;Street", "tel:%28217%29555-0142", "Monday to Friday", "Closed on public holidays."},
			"market/index.html": {"Town Square", "Saturday", "8am to noon"},
		}},
		{"gallery", []page{
			{"content/shop.md", "---\ntitle: The shop\nsummary: Pictures of the shop.\nsections:\n  - gallery:\n      heading: Inside\n      items:\n        - { image: /media/a.png, alt: The counter, caption: Fresh at 7am. }\n        - { image: /media/b.png, alt: The ovens }\n  - gallery:\n      columns: \"2\"\n      items:\n        - { image: /media/a.png, alt: The counter again }\n---\n"},
		}, map[string][]string{
			"shop/index.html": {`id="gallery-0-1"`, `href="#gallery-0-1"`, `id="gallery-1-0"`, "gal-cols-2", "2 of 2", "Fresh at 7am.", `alt="The ovens"`},
		}},
		{"menu-and-prices", []page{
			{"content/menu.md", "---\ntitle: Menu\nsummary: What we bake.\nsections:\n  - price-list:\n      groups:\n        - name: Bread\n          items:\n            - { name: Sourdough, description: Our own starter, price: $8 }\n            - { name: Rye, price: $7, tags: [vegan] }\n      note: Ask us about allergies.\n  - plans:\n      items:\n        - name: Small\n          price: $25\n          period: a month\n          features: \"One loaf a week\\nPick up on Saturday\"\n          button: { text: Sign up, href: /contact/ }\n        - { name: Large, price: $45, featured: true }\n---\n"},
		}, map[string][]string{
			"menu/index.html": {"Sourdough", "$8", `class="pl-tag">vegan`, "Ask us about allergies.", "<li>One loaf a week</li>", "<li>Pick up on Saturday</li>", "plan-featured", "Most popular", `href="/contact/">Sign up`},
		}},
		{"downloads", []page{
			{"static/files/menu.pdf", strings.Repeat("x", 14400)},
			{"content/files.md", "---\ntitle: Files\nsummary: Menus and forms.\nsections:\n  - downloads:\n      items:\n        - { title: Catering menu, file: /files/menu.pdf, description: Every season., updated: March 2026 }\n---\n"},
		}, map[string][]string{
			"files/index.html": {`href="/files/menu.pdf" download`, "PDF · 14 KB · March 2026", "Every season."},
		}},
		{"portfolio", []page{
			{"content/work/_index.md", "---\ntitle: Our work\nsummary: Projects.\n---\n"},
			{"content/work/garden-room.md", "---\ntitle: Garden room\ndate: 2026-05-14\nsummary: A timber garden office.\ncover: /media/a.png\ncover_alt: The garden room at dusk\nclient: The Lee family\nservices: Design, build\nlink: https://studio.example/case\npictures:\n  - { image: /media/b.png, alt: The frame going up, caption: Week two. }\n---\nHow it went.\n"},
			{"content/index.md", "---\ntitle: Home\nsummary: Builders.\nlayout: landing\nsections:\n  - portfolio: { count: 3 }\n---\n"},
		}, map[string][]string{
			"work/index.html":             {"Our work", `href="/work/garden-room/"`, `alt="The garden room at dusk"`},
			"work/garden-room/index.html": {"The Lee family", "Design, build", "May 2026", ">studio.example<", "Week two.", `alt="The frame going up"`},
			"index.html":                  {`href="/work/garden-room/"`, "All our work"},
		}},
		{"share-links", []page{
			{"content/news.md", "---\ntitle: Big news\nsummary: News.\nsections:\n  - share: { heading: Tell a friend }\n---\n"},
			{"data/x.yaml", "a: 1\n"},
		}, map[string][]string{
			"news/index.html": {"Tell a friend", "https://mastodon.social/share?text=Big&#43;news&#43;https%3A%2F%2Fexample.org%2Fnews%2F", "https://bsky.app/intent/compose?text=", "linkedin.com/sharing/share-offsite/?url=https%3A%2F%2Fexample.org%2Fnews%2F", "mailto:?subject=Big&#43;news"},
		}},
		{"docs", []page{
			{"data/docs-nav.yaml", "- part: start\n  title: Start here\n  blurb: The basics.\n  pages: [one, two]\n- part: more\n  title: More\n  blurb: The rest.\n  pages: [three]\n"},
			{"content/docs/_index.md", "---\ntitle: Documentation\nsummary: All of it.\nlayout: doc-home\n---\n"},
			{"content/docs/start/_index.md", "---\ntitle: Start here\nsummary: The basics.\n---\n"},
			{"content/docs/start/one.md", "---\ntitle: Page one\nsummary: First.\nweight: 1\n---\nOne.\n"},
			{"content/docs/start/two.md", "---\ntitle: Page two\nsummary: Second.\nweight: 2\n---\nTwo.\n"},
			{"content/docs/more/_index.md", "---\ntitle: More\nsummary: The rest.\n---\n"},
			{"content/docs/more/three.md", "---\ntitle: Page three\nsummary: Third.\n---\nThree.\n"},
		}, map[string][]string{
			"docs/index.html":           {`class="partgrid"`, `href="/docs/start/"`, "The basics."},
			"docs/start/index.html":     {`class="pagelist"`, `href="/docs/start/one/"`, "First."},
			"docs/start/two/index.html": {`<span>Previous</span>Page one`, `<span>Next</span>Page three`, `aria-current="page">Page two`},
			"docs/search.json":          {`"/docs/start/one/"`},
		}},
	}
	for _, c := range cases {
		t.Run(c.pack, func(t *testing.T) {
			if !Bundled(c.pack) {
				t.Fatalf("%s isn't bundled", c.pack)
			}
			dir := starterSite(t)
			if _, err := Add(dir, c.pack, false); err != nil {
				t.Fatal(err)
			}
			os.MkdirAll(filepath.Join(dir, "static", "media"), 0o755)
			os.WriteFile(filepath.Join(dir, "static", "media", "a.png"), picture, 0o644)
			os.WriteFile(filepath.Join(dir, "static", "media", "b.png"), picture, 0o644)
			for _, f := range c.files {
				p := filepath.Join(dir, filepath.FromSlash(f.path))
				os.MkdirAll(filepath.Dir(p), 0o755)
				if err := os.WriteFile(p, []byte(f.body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			res, err := build.Run(build.Options{SiteDir: dir})
			if err != nil {
				t.Fatal(err)
			}
			for file, wants := range c.want {
				out, err := os.ReadFile(filepath.Join(res.Out, filepath.FromSlash(file)))
				if err != nil {
					t.Errorf("%s: %v", file, err)
					continue
				}
				for _, w := range wants {
					if !strings.Contains(string(out), w) {
						t.Errorf("%s lacks %q", file, w)
					}
				}
			}
			cfg, err := site.LoadConfig(dir)
			if err != nil {
				t.Fatal(err)
			}
			rep, err := check.Run(dir, res.Out, cfg)
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range rep.Problems {
				if p.Level == "error" {
					t.Errorf("check: %s %s: %s", p.Rule, p.File, p.What)
				}
			}
		})
	}
}
