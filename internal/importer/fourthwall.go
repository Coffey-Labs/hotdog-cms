package importer

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/media"
)

// Fourthwall: a print-on-demand shop whose Storefront API hands out its
// catalog to anyone with the shop's storefront token. Each product becomes a
// page for the shop pack, with its photos brought into the site, and its
// sizes and colors as variants whose Buy goes to the shop's own checkout
// (cart/checkout?products=<variant>:1), so the site needs no cart and no
// script. Unlike the other imports this one is run again whenever the shop
// changes: Fourthwall stays the source, and a product's page is rewritten
// from it, keeping only the front matter the site added itself.

// FourthwallAPI is the Storefront API.
const FourthwallAPI = "https://storefront-api.fourthwall.com/v1"

// FourthwallOptions says which shop and where its products go.
type FourthwallOptions struct {
	SiteDir    string
	Token      string // the storefront token (ptkn_…), from Settings → For Developers
	Collection string // the shop's collection to bring in; "all" is every public product
	Dir        string // where under content/ products go: "products", the shop pack's collection
	Taxonomy   string // front matter key for the shop's other collections, e.g. "tags"; "" leaves them out
	Checkout   string // the checkout's address, e.g. https://shop.example.com; "" asks the shop
	Currency   string // prices in this currency; "" is the shop's own
	Photos     int    // at most this many photos a product (a color, when it has several); 0 is all of them
	Prune      bool   // delete pages of products no longer in the shop
	API        string // the API's address; "" is FourthwallAPI
	Client     *http.Client
	Log        io.Writer
}

// FourthwallReport is what a sync did.
type FourthwallReport struct {
	Written   []string // new or changed pages
	Unchanged int
	Pictures  int               // photos downloaded
	Gone      []string          // pages whose product left the shop
	Removed   []string          // …deleted, with Prune
	Failed    map[string]string // photo address → why
	Checkout  string
}

type fwPaged[T any] struct {
	Results []T `json:"results"`
	Paging  struct {
		HasNextPage bool `json:"hasNextPage"`
	} `json:"paging"`
}

type fwImage struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type fwProduct struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	State       struct {
		Type string `json:"type"`
	} `json:"state"`
	Images     []fwImage   `json:"images"`
	Variants   []fwVariant `json:"variants"`
	Additional []struct {
		Title    string `json:"title"`
		BodyHTML string `json:"bodyHtml"`
	} `json:"additionalInformation"`
	UpdatedAt string `json:"updatedAt"`
}

type fwVariant struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SKU       string `json:"sku"`
	UnitPrice struct {
		Value    float64 `json:"value"`
		Currency string  `json:"currency"`
	} `json:"unitPrice"`
	Attributes struct {
		Description string `json:"description"`
		Color       *struct {
			Name   string `json:"name"`
			Swatch string `json:"swatch"`
		} `json:"color"`
		Size *struct {
			Name string `json:"name"`
		} `json:"size"`
	} `json:"attributes"`
	Stock struct {
		Type    string `json:"type"`
		InStock *int   `json:"inStock"`
	} `json:"stock"`
	Images []fwImage `json:"images"`
}

// fwPhoto is a product photo and the color it shows, "" when it isn't one
// color's alone.
type fwPhoto struct {
	fwImage
	group string
}

// fwPhotos lists a product's photos. With more than one color, and photos
// that say which color they show, each color's come together (at most max
// of them, 0 for all) and carry its name; otherwise they're the product's
// own list, cut to max.
func fwPhotos(p fwProduct, colors []string, max int) []fwPhoto {
	var out []fwPhoto
	if len(colors) > 1 {
		owner := map[string]string{}
		for _, v := range p.Variants {
			c := v.Attributes.Color
			if c == nil || c.Name == "" {
				continue
			}
			for _, im := range v.Images {
				if o, ok := owner[im.ID]; ok && o != c.Name {
					owner[im.ID] = "" // two colors share it
				} else if !ok {
					owner[im.ID] = c.Name
				}
			}
		}
		if len(owner) > 0 {
			count := map[string]int{}
			for _, c := range colors {
				for _, im := range p.Images {
					if owner[im.ID] == c && (max <= 0 || count[c] < max) {
						out = append(out, fwPhoto{im, c})
						count[c]++
					}
				}
			}
			for _, im := range p.Images {
				if g, ok := owner[im.ID]; (!ok || g == "") && (max <= 0 || count[""] < max) {
					out = append(out, fwPhoto{im, ""})
					count[""]++
				}
			}
			return out
		}
	}
	for _, im := range p.Images {
		if max > 0 && len(out) == max {
			break
		}
		out = append(out, fwPhoto{im, ""})
	}
	return out
}

func (v fwVariant) soldOut() bool {
	return v.Stock.Type == "LIMITED" && v.Stock.InStock != nil && *v.Stock.InStock <= 0
}

type fwCollection struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// fourthwallKeys is the front matter a sync owns; anything else in a
// product's page was added on the site and is kept.
var fourthwallKeys = []string{"title", "summary", "price", "price_from", "currency", "weight", "photos", "checkout", "variants", "choose", "buy", "options", "sold_out", "sku", "fourthwall"}

// SyncFourthwall brings a Fourthwall shop's products into a site.
func SyncFourthwall(opt FourthwallOptions) (*FourthwallReport, error) {
	if opt.Token == "" {
		return nil, errors.New("no storefront token: make one in Fourthwall under Settings → For Developers → Storefront API")
	}
	if _, err := os.Stat(filepath.Join(opt.SiteDir, "site.yaml")); err != nil {
		return nil, fmt.Errorf("no site at %s (run hotdog-cms new first): %w", opt.SiteDir, err)
	}
	if opt.API == "" {
		opt.API = FourthwallAPI
	}
	if opt.Client == nil {
		opt.Client = &http.Client{Timeout: 30 * time.Second}
	}
	if opt.Log == nil {
		opt.Log = io.Discard
	}
	if opt.Collection == "" {
		opt.Collection = "all"
	}
	opt.Dir = safeDir(opt.Dir)
	if opt.Dir == "" {
		opt.Dir = "products"
	}
	get := func(p string, q url.Values, into any) error {
		if q == nil {
			q = url.Values{}
		}
		q.Set("storefront_token", opt.Token)
		if opt.Currency != "" {
			q.Set("currency", opt.Currency)
		}
		res, err := fwFresh(opt.Client, strings.TrimRight(opt.API, "/")+p+"?"+q.Encode())
		if err != nil {
			return err
		}
		defer res.Body.Close()
		if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
			return fmt.Errorf("Fourthwall turned the storefront token down (%s)", res.Status)
		}
		if res.StatusCode != http.StatusOK {
			return fmt.Errorf("Fourthwall: %s for %s", res.Status, p)
		}
		return json.NewDecoder(io.LimitReader(res.Body, 32<<20)).Decode(into)
	}
	products := func(collection string) ([]fwProduct, error) {
		var all []fwProduct
		for page := 0; page < 200; page++ {
			var pg fwPaged[fwProduct]
			q := url.Values{"page": {strconv.Itoa(page)}, "size": {"50"}}
			if err := get("/collections/"+url.PathEscape(collection)+"/products", q, &pg); err != nil {
				return nil, err
			}
			all = append(all, pg.Results...)
			if !pg.Paging.HasNextPage {
				break
			}
		}
		return all, nil
	}

	rep := &FourthwallReport{Failed: map[string]string{}}
	rep.Checkout = strings.TrimRight(opt.Checkout, "/")
	if rep.Checkout == "" {
		var shop struct {
			Domain       string `json:"domain"`
			PublicDomain string `json:"publicDomain"`
		}
		if err := get("/shop", nil, &shop); err != nil {
			return nil, err
		}
		if shop.PublicDomain == "" {
			return nil, errors.New("Fourthwall didn't say where the shop's checkout is; give it with -checkout")
		}
		rep.Checkout = "https://" + shop.PublicDomain
	}
	if u, err := url.Parse(rep.Checkout); err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, fmt.Errorf("the checkout address %q isn't an https:// address", rep.Checkout)
	}

	list, err := products(opt.Collection)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(opt.Log, "%d product(s) in %q\n", len(list), opt.Collection)

	// The shop's other collections, as terms of a taxonomy.
	terms := map[string][]string{} // product id → collection names
	if opt.Taxonomy != "" {
		for page := 0; page < 50; page++ {
			var pg fwPaged[fwCollection]
			if err := get("/collections", url.Values{"page": {strconv.Itoa(page)}, "size": {"50"}}, &pg); err != nil {
				return nil, err
			}
			for _, c := range pg.Results {
				if c.Slug == "all" || c.Slug == opt.Collection {
					continue
				}
				in, err := products(c.Slug)
				if err != nil {
					return nil, err
				}
				for _, p := range in {
					terms[p.ID] = append(terms[p.ID], c.Name)
				}
			}
			if !pg.Paging.HasNextPage {
				break
			}
		}
	}

	root, err := os.OpenRoot(opt.SiteDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	exists := func(rel string) bool {
		_, err := root.Stat(filepath.FromSlash(rel))
		return err == nil
	}

	// The pages a previous sync wrote, by product id.
	previous := map[string]string{}
	contentDir := path.Join("content", opt.Dir)
	if entries, err := os.ReadDir(filepath.Join(opt.SiteDir, filepath.FromSlash(contentDir))); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			rel := path.Join(contentDir, e.Name())
			raw, err := root.ReadFile(filepath.FromSlash(rel))
			if err != nil {
				continue
			}
			fm, _, err := splitFront(string(raw))
			if err != nil {
				continue
			}
			if fw, ok := fm["fourthwall"].(map[string]any); ok {
				if id := fmString(fw, "id"); id != "" {
					previous[id] = rel
				}
			}
		}
	}

	seen := map[string]bool{}
	for i, p := range list {
		if p.ID == "" || len(p.Variants) == 0 {
			continue
		}
		seen[p.ID] = true
		slug := slugify(p.Slug)
		if slug == "" {
			slug = slugify(p.Name)
		}
		if slug == "" {
			slug = "product"
		}
		rel := path.Join(contentDir, slug+".md")
		if old, ok := previous[p.ID]; ok {
			rel = old // a product renamed in the shop keeps its page
		}

		// What the site already has: its own front matter, and the photos a
		// previous sync brought in, by Fourthwall's id.
		kept := map[string]any{}
		havePhoto := map[string]string{}
		if raw, err := root.ReadFile(filepath.FromSlash(rel)); err == nil {
			fm, _, err := splitFront(string(raw))
			if err == nil {
				if _, ours := fm["fourthwall"]; !ours {
					fmt.Fprintf(opt.Log, "  %s is already there and isn't from Fourthwall; left alone\n", rel)
					continue
				}
				for k, v := range fm {
					if !slices.Contains(fourthwallKeys, k) && k != opt.Taxonomy {
						kept[k] = v
					}
				}
				for _, ph := range listOf(fm["photos"]) {
					m, _ := ph.(map[string]any)
					if id, img := fmString(m, "source"), fmString(m, "image"); id != "" && img != "" && exists(path.Join("static", img)) {
						havePhoto[id] = img
					}
				}
			}
		}

		// The colors, in the shop's order, and whether there are sizes.
		var colors []string
		sized := false
		for _, v := range p.Variants {
			if c := v.Attributes.Color; c != nil && c.Name != "" && !slices.Contains(colors, c.Name) {
				colors = append(colors, c.Name)
			}
			if v.Attributes.Size != nil && v.Attributes.Size.Name != "" {
				sized = true
			}
		}

		// Photos: with several colors, each one's own (at most -photos of
		// each), marked with the color so the page can show the chosen one's.
		mediaDir := path.Join("shop", slug)
		images := fwPhotos(p, colors, opt.Photos)
		of := map[string]int{}
		for _, im := range images {
			of[im.group]++
		}
		var photos []map[string]any
		using := map[string]bool{}
		seen := map[string]int{}
		for n, im := range images {
			seen[im.group]++
			alt := p.Name
			if im.group != "" {
				alt += ", " + im.group
			}
			if of[im.group] > 1 {
				alt = fmt.Sprintf("%s, photo %d of %d", alt, seen[im.group], of[im.group])
			}
			photo := map[string]any{"alt": alt, "source": im.ID}
			if im.group != "" {
				photo["group"] = im.group
			}
			if img, ok := havePhoto[im.ID]; ok {
				photo["image"] = img
				photos = append(photos, photo)
				using[img] = true
				continue
			}
			data, err := fetchWith(opt.Client, im.URL)
			if err != nil {
				rep.Failed[im.URL] = err.Error()
				continue
			}
			img, err := media.Process(data, strconv.Itoa(n+1)+path.Ext(strings.SplitN(im.URL, "?", 2)[0]), mediaDir, func(p string) bool { return exists(p) })
			if err != nil {
				rep.Failed[im.URL] = err.Error()
				continue
			}
			for _, f := range img.Files {
				if err := writeInSite(opt.SiteDir, path.Join("static", f.Path), f.Data); err != nil {
					return nil, err
				}
			}
			rep.Pictures++
			photo["image"] = img.URL
			photos = append(photos, photo)
			using[img.URL] = true
		}
		// Photos the product no longer has go, with their smaller sizes.
		if entries, err := os.ReadDir(filepath.Join(opt.SiteDir, "static", "media", filepath.FromSlash(mediaDir))); err == nil {
			for _, e := range entries {
				name := e.Name()
				main := name
				if m := media.VariantRe.FindStringSubmatch(name); m != nil {
					main = m[1]
				}
				main = strings.TrimSuffix(main, path.Ext(main))
				inUse := false
				for u := range using {
					if strings.TrimSuffix(path.Base(u), path.Ext(u)) == main {
						inUse = true
					}
				}
				if !inUse {
					_ = root.Remove(filepath.Join("static", "media", filepath.FromSlash(mediaDir), name))
				}
			}
		}

		// Variants: one per color and size, each with its own checkout.
		// Colors head groups of sizes only when there's more than one.
		cur := ""
		lowest, highest := p.Variants[0].UnitPrice.Value, p.Variants[0].UnitPrice.Value
		var variants []map[string]any
		allSold := true
		for _, v := range p.Variants {
			if cur == "" {
				cur = v.UnitPrice.Currency
			}
			lowest = min(lowest, v.UnitPrice.Value)
			highest = max(highest, v.UnitPrice.Value)
			name := ""
			if v.Attributes.Size != nil {
				name = v.Attributes.Size.Name
			}
			m := map[string]any{"price": v.UnitPrice.Value, "id": v.ID, "buy": fwBuy(rep.Checkout, v.ID, cur)}
			if c := v.Attributes.Color; c != nil && c.Name != "" {
				if len(colors) > 1 {
					m["group"] = c.Name
				}
				if c.Swatch != "" {
					m["swatch"] = c.Swatch
				}
				if name == "" {
					name = c.Name
				}
			}
			if name == "" {
				name = v.Attributes.Description
			}
			if name == "" {
				name = v.Name
			}
			m["name"] = name
			if v.SKU != "" {
				m["sku"] = v.SKU
			}
			if v.soldOut() {
				m["sold_out"] = true
			} else {
				allSold = false
			}
			variants = append(variants, m)
		}

		fm := map[string]any{}
		for k, v := range kept {
			fm[k] = v
		}
		fm["title"] = p.Name
		if s := plainFirstParagraph(p.Description); s != "" {
			fm["summary"] = shortSummary(s)
		}
		fm["price"] = lowest
		if highest > lowest {
			fm["price_from"] = true
		} else {
			delete(fm, "price_from")
		}
		fm["currency"] = cur
		if _, ok := kept["weight"]; !ok {
			fm["weight"] = i + 1 // the shop's own order
		}
		if len(photos) > 0 {
			fm["photos"] = photos
		}
		// The checkout takes any number of variants, as id:quantity,…: one
		// with Buy now, all the cart's with a cart.
		fm["checkout"] = map[string]any{"action": rep.Checkout + "/cart/checkout", "field": "products", "item": "{id}:{qty}", "join": ",", "fields": map[string]any{"currency": cur}}
		fm["variants"] = variants
		if sized {
			fm["choose"] = "Size"
		} else {
			delete(fm, "choose")
		}
		if p.State.Type == "SOLD_OUT" || allSold {
			fm["sold_out"] = true
		}
		if opt.Taxonomy != "" && len(terms[p.ID]) > 0 {
			fm[opt.Taxonomy] = terms[p.ID]
		}
		fm["fourthwall"] = map[string]any{"id": p.ID, "slug": p.Slug, "updated": p.UpdatedAt}

		body := ToMarkdown(p.Description).Markdown
		for _, a := range p.Additional {
			if t := strings.TrimSpace(a.Title); t != "" && strings.TrimSpace(a.BodyHTML) != "" {
				body += "\n\n## " + t + "\n\n" + ToMarkdown(a.BodyHTML).Markdown
			}
		}
		doc, err := frontMatterIn(fm, []string{"title", "summary", "price", "price_from", "currency", "weight", "sold_out", "photos", "choose", "variants", "checkout", "fourthwall"})
		if err != nil {
			return nil, err
		}
		out := "---\n" + doc + "---\n\n" + strings.TrimSpace(body) + "\n"
		if old, err := root.ReadFile(filepath.FromSlash(rel)); err == nil && string(old) == out {
			rep.Unchanged++
			continue
		}
		if err := writeInSite(opt.SiteDir, rel, []byte(out)); err != nil {
			return nil, err
		}
		rep.Written = append(rep.Written, rel)
		fmt.Fprintf(opt.Log, "  %s\n", rel)
	}

	// Products that have left the shop.
	for id, rel := range previous {
		if seen[id] {
			continue
		}
		if !opt.Prune {
			rep.Gone = append(rep.Gone, rel)
			continue
		}
		if err := root.Remove(filepath.FromSlash(rel)); err == nil {
			rep.Removed = append(rep.Removed, rel)
			slug := strings.TrimSuffix(path.Base(rel), ".md")
			_ = root.RemoveAll(filepath.Join("static", "media", "shop", slug))
		}
	}
	slices.Sort(rep.Gone)
	slices.Sort(rep.Removed)
	return rep, nil
}

// fwBuy is a variant's checkout: one of it, straight to the shop's checkout.
func fwBuy(checkout, variant, cur string) string {
	q := url.Values{"products": {variant + ":1"}}
	if cur != "" {
		q.Set("currency", cur)
	}
	return checkout + "/cart/checkout?" + q.Encode()
}

// fwStaleWait is how long to give Fourthwall's cache to fetch a fresh copy.
var fwStaleWait = 2 * time.Second

// fwFresh gets u, and gets it again when the answer is a stale copy.
// Fourthwall's API sits behind a cache that, after a quiet spell, answers
// with the copy it has, however old, and fetches a fresh one behind it: the
// first sync after an edit could miss it. A copy older than its max-age is
// asked for again, a few times, before it's taken as it is.
func fwFresh(c *http.Client, u string) (*http.Response, error) {
	for try := 0; ; try++ {
		res, err := c.Get(u)
		if err != nil || res.StatusCode != http.StatusOK || try == 3 || !staleCopy(res.Header) {
			return res, err
		}
		res.Body.Close()
		time.Sleep(fwStaleWait)
	}
}

// staleCopy says whether a cached answer is older than its max-age.
func staleCopy(h http.Header) bool {
	age, err := strconv.Atoi(strings.TrimSpace(h.Get("Age")))
	if err != nil {
		return false
	}
	for _, d := range strings.Split(h.Get("Cache-Control"), ",") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(strings.ToLower(d)), "max-age="); ok {
			if max, err := strconv.Atoi(v); err == nil {
				return age > max
			}
		}
	}
	return false
}

func fetchWith(c *http.Client, u string) ([]byte, error) {
	p, err := url.Parse(u)
	if err != nil || (p.Scheme != "https" && p.Scheme != "http") {
		return nil, fmt.Errorf("not a web address")
	}
	res, err := c.Get(u)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s", res.Status)
	}
	return io.ReadAll(io.LimitReader(res.Body, media.MaxBytes+1))
}

var (
	firstPara = regexp.MustCompile(`(?is)<p[^>]*>(.*?)</p>`)
	anyTag    = regexp.MustCompile(`<[^>]+>`)
	fwSpaces  = regexp.MustCompile(`\s+`)
)

// plainFirstParagraph is a description's first paragraph as plain text.
func plainFirstParagraph(h string) string {
	s := h
	if m := firstPara.FindStringSubmatch(h); m != nil {
		s = m[1]
	}
	s = html.UnescapeString(anyTag.ReplaceAllString(s, " "))
	return strings.TrimSpace(fwSpaces.ReplaceAllString(s, " "))
}

func listOf(v any) []any {
	l, _ := v.([]any)
	return l
}

// Markdown is the sync's report, for the person who ran it.
func (r *FourthwallReport) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d page(s) written, %d unchanged, %d photo(s) brought in. Buy goes to %s.\n", len(r.Written), r.Unchanged, r.Pictures, r.Checkout)
	if len(r.Gone) > 0 {
		fmt.Fprintf(&b, "\nNo longer in the shop (run again with -prune to delete them):\n")
		for _, g := range r.Gone {
			fmt.Fprintf(&b, "  %s\n", g)
		}
	}
	for _, g := range r.Removed {
		fmt.Fprintf(&b, "  deleted %s\n", g)
	}
	if len(r.Failed) > 0 {
		fmt.Fprintf(&b, "\nPhotos that couldn't be brought in:\n")
		for _, u := range sortedFailed(r.Failed) {
			fmt.Fprintf(&b, "  %s: %s\n", u, r.Failed[u])
		}
	}
	return b.String()
}

func sortedFailed(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
