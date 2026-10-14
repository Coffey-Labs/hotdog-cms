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
			"help/index.html":   {`<details class="faq-item" open>`, "Do you deliver?", "<strong>five</strong>", "packs/faq/faq.", `"@type":"FAQPage"`, `"@type":"Question","acceptedAnswer"`},
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
			"people/sam-rivera/index.html": {`"@type":"Person"`, `"jobTitle":"Head baker"`, "mailto:sam@example.org", `rel="me"`, "team-initial", ">S<"},
			"about-us/index.html":          {`href="/people/emile-durand/"`, ">É<", `src="/media/a.png" alt="Ana at the counter"`},
		}},
		{"location-and-hours", []page{
			{"data/location.yaml", "name: Bakery on Main\naddress: |\n  12 Main Street\n  Springfield, IL 62701\nphone: \"(217) 555-0142\"\nemail: hello@example.org\nhours:\n  - { days: Monday to Friday, times: 7am to 6pm }\n  - { days: Sunday, times: Closed }\nnote: Closed on public holidays.\n"},
			{"content/visit.md", "---\ntitle: Visit\nsummary: Where and when.\nsections:\n  - location: { heading: Visit us }\n---\n"},
			{"content/market.md", "---\ntitle: Market stall\nsummary: Saturdays.\nsections:\n  - location:\n      address: Town Square\n      hours: [{ days: Saturday, times: 8am to noon }]\n---\n"},
		}, map[string][]string{
			"visit/index.html":  {`"@type":"LocalBusiness"`, `"telephone":"(217) 555-0142"`, "Bakery on Main", "https://www.openstreetmap.org/search?query=12&#43;Main&#43;Street", "tel:%28217%29555-0142", "Monday to Friday", "Closed on public holidays."},
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
			"docs/start/two/index.html": {`<span>Previous</span>Page one`, `<span>Next</span>More`, `aria-current="page">Page two`, `<p class="crumb"><a href="/docs/start/">Start here</a></p>`},
			"docs/more/index.html":      {`<span>Previous</span>Page two`, `<span>Next</span>Page three`, `href="/docs/more/three/"`},
			"docs/search.json":          {`"/docs/start/one/"`},
		}},
		{"recipes", []page{
			{"content/recipes/sourdough.md", "---\ntitle: Weekend sourdough\ndate: 2026-10-01\nsummary: A simple loaf.\nphoto: /media/a.png\nphoto_alt: A round loaf\nprep_minutes: 30\ncook_minutes: 45\nservings: One loaf\ningredients:\n  - 500 g flour\n  - 350 g water\nsteps:\n  - Mix and rest.\n  - Bake at 250°C.\n---\nTips.\n"},
		}, map[string][]string{
			"recipes/sourdough/index.html": {"<li>500 g flour</li>", "<li>Bake at 250°C.</li>", "75 min", `"@type":"Recipe"`, `"prepTime":"PT30M"`, `"totalTime":"PT1H15M"`, `"recipeIngredient":["500 g flour","350 g water"]`},
			"recipes/index.html":           {`href="/recipes/sourdough/"`, "75 minutes"},
		}},
		{"jobs", []page{
			{"content/jobs/baker.md", "---\ntitle: Morning baker\ndate: 2026-10-10\nsummary: Bake from 4am.\nlocation: Springfield, IL\nemployment: Part-time\npay: $19 an hour\ncloses: 2099-11-15\napply: mailto:jobs@example.org\n---\nThe job.\n"},
			{"content/jobs/old.md", "---\ntitle: Summer help\ndate: 2020-05-01\nsummary: Gone.\ncloses: 2020-06-01\napply: mailto:jobs@example.org\n---\n"},
		}, map[string][]string{
			"jobs/index.html":       {"Morning baker", "Apply by November 15"},
			"jobs/baker/index.html": {`"@type":"JobPosting"`, `"employmentType":"PART_TIME"`, `"validThrough":"2099-11-15T23:59:59`, "Sunday, November 15, 2099", `href="mailto:jobs@example.org">Apply`},
			"jobs/old/index.html":   {"Applications for this job have closed."},
		}},
		{"related-posts", []page{
			{"content/articles/a.md", "---\ntitle: Article A\ndate: 2026-01-01\nsummary: A.\ntags: [bread, ovens]\n---\nA.\n"},
			{"content/articles/b.md", "---\ntitle: Article B\ndate: 2026-02-01\nsummary: B.\ntags: [bread, ovens]\n---\nB.\n"},
			{"content/articles/c.md", "---\ntitle: Article C\ndate: 2026-03-01\nsummary: C.\ntags: [bread]\n---\nC.\n"},
			{"content/articles/d.md", "---\ntitle: Article D\ndate: 2026-04-01\nsummary: D.\ntags: [cake]\n---\nD.\n"},
		}, map[string][]string{
			"articles/a/index.html": {"Keep reading", `href="/articles/b/">Article B`, `href="/articles/c/">Article C`},
		}},
		{"table-of-contents", []page{
			{"content/articles/long.md", "---\ntitle: Long\ndate: 2026-01-01\nsummary: Long.\n---\n## First part\n\nx\n\n### A detail\n\nx\n\n## Second part\n\nx\n"},
			{"content/articles/short.md", "---\ntitle: Short\ndate: 2026-01-02\nsummary: Short.\n---\n## Only one\n\nx\n"},
		}, map[string][]string{
			"articles/long/index.html": {"On this page", `<li class="toc-l2"><a href="#first-part">First part</a>`, `<li class="toc-l3"><a href="#a-detail">A detail</a>`},
		}},
		{"podcast", []page{
			{"static/media/ep1.mp3", strings.Repeat("x", 5000)},
			{"content/episodes/_index.md", "---\ntitle: The Bakery Hour\nsummary: A podcast.\n---\n"},
			{"content/episodes/one.md", "---\ntitle: \"1: Why we bake at 4am\"\ndate: 2026-10-01\nsummary: Starters.\naudio: /media/ep1.mp3\nduration: \"34:12\"\nepisode: 1\n---\nNotes.\n"},
		}, map[string][]string{
			"episodes/one/index.html": {`<audio controls preload="none" src="/media/ep1.mp3">`, "34:12", "(5 KB)"},
			"episodes/index.html":     {"Subscribe in a podcast app", "https://example.org/episodes/feed.xml"},
			"episodes/feed.xml":       {`xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd"`, "<title>The Bakery Hour</title>", `<enclosure url="https://example.org/media/ep1.mp3" length="5000" type="audio/mpeg">`, "<itunes:duration>34:12</itunes:duration>", "<itunes:episode>1</itunes:episode>"},
		}},
		{"payment-links", []page{
			{"content/shop.md", "---\ntitle: Gift cards\nsummary: Gift cards.\nsections:\n  - buy:\n      items:\n        - { name: $25 gift card, price: $25, image: /media/a.png, image_alt: A gift card, link: \"https://buy.stripe.com/test_123\", button: Buy now }\n  - donate:\n      heading: Support the bakery\n      link: \"https://liberapay.com/bakery\"\n      amounts:\n        - { label: $5, link: \"https://liberapay.com/bakery/donate?amount=5\" }\n---\n"},
		}, map[string][]string{
			"shop/index.html": {`href="https://buy.stripe.com/test_123" rel="noopener">Buy now`, `alt="A gift card"`, "Support the bakery", `href="https://liberapay.com/bakery" rel="noopener">Donate`, ">$5<"},
		}},
		{"shop", []page{
			{"content/products/_index.md", "---\ntitle: Shop\nsummary: Things to buy.\n---\n"},
			{"content/products/kit.md", "---\ntitle: Starter kit\nsummary: A jar of starter.\nprice: 24\ncurrency: USD\nweight: 1\nphotos:\n  - { image: /media/a.png, alt: A jar }\n  - { image: /media/b.png, alt: The card }\nbuy: \"https://buy.stripe.com/test_1\"\noptions:\n  - { name: With a basket, price: 39.5, buy: \"https://buy.stripe.com/test_2\" }\n---\nWhat's in the box.\n"},
			{"content/products/mug.md", "---\ntitle: Mug\nsummary: A mug.\nprice: 12\ncurrency: EUR\nweight: 2\nsold_out: true\n---\n"},
		}, map[string][]string{
			"products/index.html":     {`href="/products/kit/"`, "$24.00", "€12.00 · <em>Sold out</em>"},
			"products/kit/index.html": {`href="https://buy.stripe.com/test_1" rel="noopener">Buy`, "With a basket · $39.50", `alt="The card"`, `"@type":"Product"`, `"priceCurrency":"USD"`, `"price":"24.00"`, "InStock"},
			"products/mug/index.html": {"Sold out", "OutOfStock"},
		}},
		{"contact-forms", []page{
			{"content/quote.md", "---\ntitle: Get a quote\nsummary: Tell us what you need.\nsections:\n  - contact-form:\n      heading: Ask for a quote\n      form: quote\n      email: hello@example.org\n      phone: (217) 555-0142\n      address: |\n        12 Main Street\n        Springfield\n      note: We answer within **one** working day.\n---\n"},
			{"content/news.md", "---\ntitle: News\nsummary: Sign up.\nsections:\n  - contact-form: { heading: Hear from us, form: signup }\n  - contact-form: { form: booking }\n---\n"},
		}, map[string][]string{
			"quote/index.html": {"Ask for a quote", `action="/_hotdog/forms/quote"`, `name="service"`, "<option>A change or repair</option>", ">Ask for a quote</button>", `class="cf-details"`, `href="mailto:hello@example.org"`, "tel:%28217%29555-0142", "12 Main Street", "<strong>one</strong>", "packs/contact-forms/contact-forms."},
			"news/index.html":  {"Hear from us", `action="/_hotdog/forms/signup"`, `type="checkbox" required`, "cf-grid cf-solo", `action="/_hotdog/forms/booking"`, "<option>Evening</option>"},
		}},
		{"tables", []page{
			{"data/prices.csv", "\ufeffItem,Small,Large\nSourdough,$6,$8\n\"Rye, dark\",$5,$7\n[Gift card](/prices/),<b>$25</b>,\n"},
			{"data/staff.yaml", "- { name: Sam, role: Baker, start: 2019 }\n- { name: Ana, role: Front of house }\n"},
			{"content/prices.md", "---\ntitle: Prices\nsummary: What things cost.\nsections:\n  - table:\n      heading: Prices\n      from: prices\n      caption: Tax included.\n      numbers: true\n      row_headers: true\n      note: Prices from **January**.\n  - table:\n      from: staff\n      columns: [name, start]\n  - table:\n      no_header: true\n      compact: true\n      rows:\n        - Monday | Closed\n        - Tuesday | 7am to 6pm\n---\n"},
		}, map[string][]string{
			"prices/index.html": {"<caption>Tax included.</caption>", `<th scope="col">Item</th>`, `<th scope="col">Large</th>`, `<th scope="row">Sourdough</th><td>$6</td>`, `<th scope="row">Rye, dark</th>`, `<a href="/prices/">Gift card</a>`, "<td><!-- raw HTML omitted -->$25<!-- raw HTML omitted --></td>", "tbl tbl-numbers tbl-rowheads", `aria-label="Tax included."`, "<strong>January</strong>", `<th scope="col">name</th><th scope="col">start</th>`, "<td>Sam</td><td>2019</td>", "<td>Ana</td><td></td>", "tbl tbl-compact", "<td>Monday</td><td>Closed</td>", "packs/tables/tables."},
		}},
		{"changelog", []page{
			{"content/changelog/_index.md", "---\ntitle: Release notes\nsummary: What changed, release by release.\n---\n"},
			{"content/changelog/1-0-0.md", "---\ntitle: \"1.0.0\"\ndate: 2026-09-01\nsummary: The first release.\n---\n### Added\n\n- Everything.\n"},
			{"content/changelog/1-1-0.md", "---\ntitle: \"1.1.0\"\ndate: 2026-10-01\nsummary: Faster.\nbreaking: true\n---\n### Changed\n\n- Faster builds.\n"},
			{"content/changelog/1-1-1.md", "---\ntitle: \"1.1.1\"\ndate: 2026-10-12\nsummary: A security fix.\nsecurity: true\n---\n### Security\n\n- Closed a hole.\n"},
			{"content/index.md", "---\ntitle: Home\nsummary: Software.\nlayout: landing\nsections:\n  - changelog: { count: 2, notes: true }\n---\n"},
		}, map[string][]string{
			"changelog/index.html":       {"Release notes", `id="1-1-1"`, `href="/changelog/1-0-0/">1.0.0`, "Security fix", "Breaking changes", "<h3", "Faster builds.", `href="/changelog/feed.xml">Follow new releases`, "October 12, 2026"},
			"changelog/1-1-0/index.html": {"<h1>1.1.0</h1>", "Breaking changes", `class="rel-older" href="/changelog/1-0-0/"`, `class="rel-newer" href="/changelog/1-1-1/"`, "What changed"},
			"changelog/1-1-1/index.html": {`class="rel-older" href="/changelog/1-1-0/"`, `class="rel-latest" href="/changelog/1-1-1/"`},
			"changelog/feed.xml":         {"1.1.1", "1.0.0"},
			"index.html":                 {`href="/changelog/1-1-1/">1.1.1`, `href="/changelog/1-1-0/">1.1.0`, "Closed a hole.", "Every release"},
		}},
		{"breadcrumbs", []page{
			{"content/recipes/_index.md", "---\ntitle: Recipes\nsummary: All of them.\n---\n"},
			{"content/recipes/bread/_index.md", "---\ntitle: Bread\nsummary: Loaves.\n---\n"},
			{"content/recipes/bread/rye.md", "---\ntitle: Dark rye\ndate: 2026-10-01\nsummary: A dense loaf.\n---\nRye.\n"},
		}, map[string][]string{
			"recipes/bread/rye/index.html": {`<nav class="crumbs" aria-label="Breadcrumb">`, `<li><a href="/">Home</a></li><li><a href="/recipes/">Recipes</a></li><li><a href="/recipes/bread/">Bread</a></li><li><span aria-current="page">Dark rye</span></li>`, `"@type":"BreadcrumbList"`, `"position":4`, `"item":"https://example.org/recipes/bread/"`, "packs/breadcrumbs/breadcrumbs."},
			"about/index.html":             {`<li><a href="/">Home</a></li><li><span aria-current="page">About</span></li>`},
		}},
		{"author-box", []page{
			{"data/authors.yaml", "sam:\n  name: Sam Rivera\n  role: Head baker\n  bio: Bakes from **4am**.\n  photo: /media/a.png\n  photo_alt: Sam at the oven\n  page: /about/\n  links:\n    - { name: Mastodon, url: \"https://example.social/@sam\" }\nana:\n  name: Ana Lopez\nme:\n  name: Your Name\n  role: Owner\n"},
			{"content/articles/one.md", "---\ntitle: One\ndate: 2026-10-01\nsummary: One.\nauthor: sam\n---\nOne.\n"},
			{"content/articles/two.md", "---\ntitle: Two\ndate: 2026-10-02\nsummary: Two.\nauthors: [sam, Ana Lopez]\n---\nTwo.\n"},
			{"content/articles/three.md", "---\ntitle: Three\ndate: 2026-10-03\nsummary: Three.\n---\nThree.\n"},
		}, map[string][]string{
			"articles/one/index.html":   {`aria-label="About the author"`, `src="/media/a.png" alt="Sam at the oven"`, `<a href="/about/" rel="author">Sam Rivera</a>`, "· Head baker", "<strong>4am</strong>", `href="https://example.social/@sam">Mastodon`, "packs/author-box/author-box."},
			"articles/two/index.html":   {`aria-label="About the authors"`, "Sam Rivera", `<span class="ab-photo ab-initial" aria-hidden="true">A</span>`, "Ana Lopez"},
			"articles/three/index.html": {"Your Name", "· Owner"},
		}},
		{"series", []page{
			{"content/articles/a.md", "---\ntitle: Starter\ndate: 2026-09-01\nsummary: A.\nseries: Sourdough basics\n---\nA.\n"},
			{"content/articles/b.md", "---\ntitle: Feeding\ndate: 2026-09-08\nsummary: B.\nseries: Sourdough basics\n---\nB.\n"},
			{"content/articles/c.md", "---\ntitle: Baking\ndate: 2026-09-15\nsummary: C.\nseries: sourdough basics\n---\nC.\n"},
			{"content/articles/x.md", "---\ntitle: Ovens two\ndate: 2026-08-01\nsummary: X.\nseries: Ovens\npart: 2\n---\nX.\n"},
			{"content/articles/y.md", "---\ntitle: Ovens one\ndate: 2026-08-20\nsummary: Y.\nseries: Ovens\npart: 1\n---\nY.\n"},
			{"content/articles/alone.md", "---\ntitle: Alone\ndate: 2026-07-01\nsummary: Z.\nseries: Just me\n---\nZ.\n"},
		}, map[string][]string{
			"articles/b/index.html": {"Part 2 of 3</span> in <span class=\"series-name\">Sourdough basics</span>", `<li><a href="/articles/a/">Starter</a></li><li><span aria-current="page">Feeding</span></li><li><a href="/articles/c/">Baking</a></li>`, `class="series-prev" href="/articles/a/"`, `class="series-next" href="/articles/c/"`, "packs/series/series."},
			"articles/a/index.html": {"Part 1 of 3"},
			"articles/x/index.html": {"Part 2 of 2", `class="series-prev" href="/articles/y/"`},
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
			if c.pack == "breadcrumbs" {
				for _, l := range []string{"page.html", "article.html", "list.html"} {
					lp := filepath.Join(dir, "layouts", l)
					raw, _ := os.ReadFile(lp)
					os.WriteFile(lp, []byte(strings.Replace(string(raw), "<h1", `{{ template "breadcrumbs" . }}<h1`, 1)), 0o644)
				}
			}
			if c.pack == "author-box" || c.pack == "series" {
				lp := filepath.Join(dir, "layouts", "article.html")
				raw, _ := os.ReadFile(lp)
				os.WriteFile(lp, []byte(strings.Replace(string(raw), "{{ .Content }}", `{{ .Content }}{{ template "`+c.pack+`" . }}`, 1)), 0o644)
			}
			if c.pack == "changelog" {
				lp := filepath.Join(dir, "partials", "footer.html")
				raw, _ := os.ReadFile(lp)
				os.WriteFile(lp, []byte(strings.Replace(string(raw), "</footer>", `{{ template "latest-release" . }}</footer>`, 1)), 0o644)
			}
			if c.pack == "related-posts" || c.pack == "table-of-contents" {
				lp := filepath.Join(dir, "layouts", "article.html")
				raw, _ := os.ReadFile(lp)
				tpl := `{{ template "related-posts" . }}`
				if c.pack == "table-of-contents" {
					tpl = `{{ template "table-of-contents" . }}`
				}
				os.WriteFile(lp, []byte(strings.Replace(string(raw), "{{ .Content }}", tpl+"{{ .Content }}", 1)), 0o644)
			}
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
