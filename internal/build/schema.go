package build

import (
	"fmt"
	"html/template"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
)

// Structured data for search engines from what packs put on a page: the
// questions in an faq section, the business in a location section, and the
// person, recipe or job a page is about. A template writes each one as
// <script type="application/ld+json">{{ . }}</script>, where html/template
// encodes it as JSON, so nothing in a page can break out of the tag.
//
// Packs carry no code, so this lives here; the shapes follow the fields the
// bundled packs document.

// pageData is every schema.org object for a page, in a stable order.
func pageData(s *site.Site, p *site.Page) []map[string]any {
	var out []map[string]any
	for _, sec := range p.Sections {
		switch sec.Type {
		case "faq":
			if d := faqData(s, sec.Data); d != nil {
				out = append(out, d)
			}
		case "location":
			if d := businessData(s, sec.Data); d != nil {
				out = append(out, d)
			}
		}
	}
	layout := p.Layout
	if layout == "" && p.Collection != "" {
		layout = s.Config.Collections[p.Collection].Layout
	}
	switch layout {
	case "person":
		out = append(out, personData(s, p))
	case "recipe":
		out = append(out, recipeData(s, p))
	case "product":
		out = append(out, productData(s, p))
	case "job":
		// A closed opening isn't advertised: search engines drop listings
		// whose validThrough has passed, and say so.
		if !closed(s, p.Params["closes"]) {
			out = append(out, jobData(s, p))
		}
	}
	return out
}

func ldBase(typ string) map[string]any {
	return map[string]any{"@context": "https://schema.org", "@type": typ}
}

func str(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func list(v any) []any {
	l, _ := v.([]any)
	return l
}

// lines is a list field or a block of text, one entry per line.
func lines(v any) []string {
	var out []string
	switch t := v.(type) {
	case []any:
		for _, x := range t {
			if s := str(x); s != "" {
				out = append(out, s)
			}
		}
	case string:
		for _, l := range strings.Split(t, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				out = append(out, l)
			}
		}
	}
	return out
}

func abs(s *site.Site, p string) string {
	if p == "" || strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
		return p
	}
	return strings.TrimRight(s.Config.URL, "/") + "/" + strings.TrimLeft(p, "/")
}

// sectionItems is a section's own list, or the data file it names with from:.
func sectionItems(s *site.Site, d map[string]any, key string) []any {
	if f := str(d["from"]); f != "" {
		return list(s.Data[f])
	}
	return list(d[key])
}

func faqData(s *site.Site, d map[string]any) map[string]any {
	var qs []any
	for _, it := range sectionItems(s, d, "items") {
		m, _ := it.(map[string]any)
		q, a := str(m["question"]), str(m["answer"])
		if q == "" || a == "" {
			continue
		}
		qs = append(qs, map[string]any{"@type": "Question", "name": q,
			"acceptedAnswer": map[string]any{"@type": "Answer", "text": a}})
	}
	if len(qs) == 0 {
		return nil
	}
	out := ldBase("FAQPage")
	out["mainEntity"] = qs
	return out
}

func businessData(s *site.Site, d map[string]any) map[string]any {
	src := d
	if str(d["address"]) == "" && d["hours"] == nil {
		f := str(d["from"])
		if f == "" {
			f = "location"
		}
		src, _ = s.Data[f].(map[string]any)
	}
	if src == nil || (str(src["name"]) == "" && str(src["address"]) == "") {
		return nil
	}
	typ := str(src["type"])
	if typ == "" {
		typ = "LocalBusiness"
	}
	out := ldBase(typ)
	out["name"] = str(src["name"])
	if out["name"] == "" {
		out["name"] = s.Config.Name
	}
	out["url"] = s.Config.URL
	if a := str(src["address"]); a != "" {
		out["address"] = map[string]any{"@type": "PostalAddress", "streetAddress": strings.Join(lines(a), ", ")}
	}
	if v := str(src["phone"]); v != "" {
		out["telephone"] = v
	}
	if v := str(src["email"]); v != "" {
		out["email"] = v
	}
	var hours []string
	for _, h := range list(src["hours"]) {
		m, _ := h.(map[string]any)
		if days, times := str(m["days"]), str(m["times"]); days != "" && times != "" {
			hours = append(hours, days+": "+times)
		}
	}
	if len(hours) > 0 {
		// Free text, as the pack lets people write hours; schema.org's
		// openingHours wants a strict form, so this goes in the description.
		out["description"] = "Opening hours: " + strings.Join(hours, "; ")
	}
	return out
}

func personData(s *site.Site, p *site.Page) map[string]any {
	out := ldBase("Person")
	out["name"] = p.Title
	out["url"] = p.Permalink
	if v := str(p.Params["role"]); v != "" {
		out["jobTitle"] = v
	}
	if v := str(p.Params["photo"]); v != "" {
		out["image"] = abs(s, v)
	}
	if p.Summary != "" {
		out["description"] = p.Summary
	}
	var same []string
	for _, l := range list(p.Params["links"]) {
		m, _ := l.(map[string]any)
		if u := str(m["url"]); u != "" {
			same = append(same, u)
		}
	}
	if len(same) > 0 {
		out["sameAs"] = same
	}
	out["worksFor"] = map[string]any{"@type": "Organization", "name": s.Config.Name, "url": s.Config.URL}
	return out
}

// minutes turns prep_minutes: 15 into PT15M (and 90 into PT1H30M).
func minutes(v any) string {
	n, err := strconv.Atoi(str(v))
	if err != nil || n <= 0 {
		return ""
	}
	if n >= 60 {
		if n%60 == 0 {
			return fmt.Sprintf("PT%dH", n/60)
		}
		return fmt.Sprintf("PT%dH%dM", n/60, n%60)
	}
	return fmt.Sprintf("PT%dM", n)
}

func recipeData(s *site.Site, p *site.Page) map[string]any {
	out := ldBase("Recipe")
	out["name"] = p.Title
	out["url"] = p.Permalink
	if p.Summary != "" {
		out["description"] = p.Summary
	}
	if v := str(p.Params["photo"]); v != "" {
		out["image"] = abs(s, v)
	}
	if !p.Date.IsZero() {
		out["datePublished"] = p.Date.Format("2006-01-02")
	}
	out["author"] = map[string]any{"@type": "Person", "name": s.Config.Author}
	prep, cook := p.Params["prep_minutes"], p.Params["cook_minutes"]
	if v := minutes(prep); v != "" {
		out["prepTime"] = v
	}
	if v := minutes(cook); v != "" {
		out["cookTime"] = v
	}
	a, _ := strconv.Atoi(str(prep))
	b, _ := strconv.Atoi(str(cook))
	if v := minutes(a + b); v != "" {
		out["totalTime"] = v
	}
	if v := str(p.Params["servings"]); v != "" {
		out["recipeYield"] = v
	}
	if v := str(p.Params["category"]); v != "" {
		out["recipeCategory"] = v
	}
	if v := str(p.Params["cuisine"]); v != "" {
		out["recipeCuisine"] = v
	}
	if l := lines(p.Params["ingredients"]); len(l) > 0 {
		out["recipeIngredient"] = l
	}
	var steps []any
	for _, l := range lines(p.Params["steps"]) {
		steps = append(steps, map[string]any{"@type": "HowToStep", "text": l})
	}
	if len(steps) > 0 {
		out["recipeInstructions"] = steps
	}
	return out
}

// employment maps the words people write to schema.org's employment types.
var employment = map[string]string{
	"full-time": "FULL_TIME", "full time": "FULL_TIME", "part-time": "PART_TIME", "part time": "PART_TIME",
	"contract": "CONTRACTOR", "contractor": "CONTRACTOR", "temporary": "TEMPORARY", "temp": "TEMPORARY",
	"internship": "INTERN", "intern": "INTERN", "volunteer": "VOLUNTEER", "per diem": "PER_DIEM", "seasonal": "TEMPORARY",
}

func jobData(s *site.Site, p *site.Page) map[string]any {
	out := ldBase("JobPosting")
	out["title"] = p.Title
	out["url"] = p.Permalink
	desc := p.Summary
	if body := strings.TrimSpace(p.Raw); body != "" {
		desc = strings.TrimSpace(desc + "\n\n" + body)
	}
	out["description"] = desc
	if !p.Date.IsZero() {
		out["datePosted"] = p.Date.Format("2006-01-02")
	}
	if t, ok := dayOf(s, p.Params["closes"]); ok {
		out["validThrough"] = endOfDay(t).Format(time.RFC3339)
	}
	if v, ok := employment[strings.ToLower(str(p.Params["employment"]))]; ok {
		out["employmentType"] = v
	}
	out["hiringOrganization"] = map[string]any{"@type": "Organization", "name": s.Config.Name, "sameAs": s.Config.URL}
	loc := str(p.Params["location"])
	if strings.EqualFold(loc, "remote") {
		out["jobLocationType"] = "TELECOMMUTE"
	} else if loc != "" {
		out["jobLocation"] = map[string]any{"@type": "Place", "address": map[string]any{"@type": "PostalAddress", "streetAddress": loc}}
	}
	return out
}

// dayOf reads a date from front matter (2026-11-01, or a time YAML parsed)
// as that day in the site's time zone.
func dayOf(s *site.Site, v any) (time.Time, bool) {
	loc := s.Location
	if loc == nil {
		loc = time.UTC
	}
	switch t := v.(type) {
	case time.Time:
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc), true
	case string:
		d, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(t), loc)
		return d, err == nil
	}
	return time.Time{}, false
}

func endOfDay(t time.Time) time.Time { return t.AddDate(0, 0, 1).Add(-time.Second) }

// closed says whether a date (a job's closes:, say) has passed, at the end
// of that day in the site's time zone, when the site is built.
func closed(s *site.Site, v any) bool {
	t, ok := dayOf(s, v)
	return ok && s.BuildTime.After(endOfDay(t))
}

// related is up to n pages from p's collection that share the most tags
// with it, then the newest. Pages sharing none are left out.
func related(s *site.Site, p *site.Page, n int) []*site.Page {
	if p.Collection == "" {
		return nil
	}
	mine := map[string]bool{}
	for _, t := range tagsOf(p) {
		mine[t] = true
	}
	if len(mine) == 0 {
		return nil
	}
	type scored struct {
		p *site.Page
		n int
	}
	var hits []scored
	for _, q := range s.Collection(p.Collection) {
		if q == p || q.Draft || q.NoIndex {
			continue
		}
		k := 0
		for _, t := range tagsOf(q) {
			if mine[t] {
				k++
			}
		}
		if k > 0 {
			hits = append(hits, scored{q, k})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].n != hits[j].n {
			return hits[i].n > hits[j].n
		}
		return hits[i].p.Date.After(hits[j].p.Date)
	})
	var out []*site.Page
	for i := 0; i < len(hits) && i < n; i++ {
		out = append(out, hits[i].p)
	}
	return out
}

func tagsOf(p *site.Page) []string {
	var out []string
	for _, t := range lines(p.Params["tags"]) {
		out = append(out, site.TermSlug(t))
	}
	return out
}

// Heading is one heading of a rendered page, for a table of contents.
type Heading struct {
	Level int
	ID    string
	Text  string
}

// headings lists the h2 and h3 headings with ids in rendered HTML.
func headings(content template.HTML) []Heading {
	var out []Heading
	z := html.NewTokenizer(strings.NewReader(string(content)))
	var cur *Heading
	var text strings.Builder
	for {
		switch z.Next() {
		case html.ErrorToken:
			return out
		case html.StartTagToken:
			name, hasAttr := z.TagName()
			if (string(name) == "h2" || string(name) == "h3") && cur == nil {
				h := &Heading{Level: int(name[1] - '0')}
				for hasAttr {
					var k, v []byte
					k, v, hasAttr = z.TagAttr()
					if string(k) == "id" {
						h.ID = string(v)
					}
				}
				if h.ID != "" {
					cur = h
					text.Reset()
				}
			}
		case html.TextToken:
			if cur != nil {
				text.Write(z.Text())
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			if cur != nil && (string(name) == "h2" || string(name) == "h3") {
				cur.Text = strings.Join(strings.Fields(text.String()), " ")
				out = append(out, *cur)
				cur = nil
			}
		}
	}
}

// amount reads a price written as a number (24, 24.5, "24.50").
func amount(v any) (float64, bool) {
	switch t := v.(type) {
	case int:
		return float64(t), true
	case float64:
		return t, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimLeft(t, "$€£¥")), 64)
		return f, err == nil
	}
	return 0, false
}

var currencySigns = map[string]string{"USD": "$", "CAD": "CA$", "AUD": "A$", "NZD": "NZ$", "EUR": "€", "GBP": "£", "JPY": "¥", "INR": "₹", "MXN": "MX$", "BRL": "R$"}

// money writes a price the way people read it: 24 "USD" is $24.00, 24 "SEK"
// is 24.00 SEK. Whole yen have no decimals.
func money(v any, currency string) string {
	f, ok := amount(v)
	if !ok {
		return str(v)
	}
	cur := strings.ToUpper(strings.TrimSpace(currency))
	if cur == "" {
		cur = "USD"
	}
	n := strconv.FormatFloat(f, 'f', 2, 64)
	if cur == "JPY" {
		n = strconv.FormatFloat(f, 'f', 0, 64)
	}
	if sign, ok := currencySigns[cur]; ok {
		return sign + n
	}
	return n + " " + cur
}

func productData(s *site.Site, p *site.Page) map[string]any {
	out := ldBase("Product")
	out["name"] = p.Title
	out["url"] = p.Permalink
	if p.Summary != "" {
		out["description"] = p.Summary
	}
	var images []string
	for _, ph := range list(p.Params["photos"]) {
		m, _ := ph.(map[string]any)
		if im := str(m["image"]); im != "" {
			images = append(images, abs(s, im))
		}
	}
	if len(images) > 0 {
		out["image"] = images
	}
	if v := str(p.Params["sku"]); v != "" {
		out["sku"] = v
	}
	if f, ok := amount(p.Params["price"]); ok {
		cur := strings.ToUpper(str(p.Params["currency"]))
		if cur == "" {
			cur = "USD"
		}
		avail := "https://schema.org/InStock"
		if str(p.Params["sold_out"]) == "true" {
			avail = "https://schema.org/OutOfStock"
		}
		out["offers"] = map[string]any{"@type": "Offer", "price": strconv.FormatFloat(f, 'f', 2, 64), "priceCurrency": cur, "availability": avail, "url": p.Permalink}
	}
	out["brand"] = map[string]any{"@type": "Brand", "name": s.Config.Name}
	return out
}
