package importer

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/media/webpenc"
)

// fakeFourthwall is a Storefront API with a tee (two prices, a sold-out
// size) and a cap, and a collection holding the cap.
func fakeFourthwall(t *testing.T, products *[]map[string]any) *httptest.Server {
	t.Helper()
	cut := image.NewRGBA(image.Rect(0, 0, 600, 800))
	cut.Set(300, 400, color.RGBA{10, 10, 10, 255})
	var pic bytes.Buffer
	if err := webpenc.Encode(&pic, cut, webpenc.Options{Lossless: true}); err != nil {
		t.Fatal(err)
	}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/img/") {
			w.Write(pic.Bytes())
			return
		}
		if r.URL.Query().Get("storefront_token") != "ptkn_test" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		paged := func(v any) {
			json.NewEncoder(w).Encode(map[string]any{"results": v, "paging": map[string]any{"hasNextPage": false}})
		}
		switch r.URL.Path {
		case "/shop":
			json.NewEncoder(w).Encode(map[string]any{"publicDomain": "shop.example"})
		case "/collections":
			paged([]any{map[string]any{"name": "All", "slug": "all"}, map[string]any{"name": "Hats", "slug": "hats"}})
		case "/collections/all/products":
			paged(*products)
		case "/collections/hats/products":
			var hats []map[string]any
			for _, p := range *products {
				if p["slug"] == "cap" {
					hats = append(hats, p)
				}
			}
			paged(hats)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func fwVariantJSON(id, color, size string, price float64, stock any) map[string]any {
	attrs := map[string]any{"color": map[string]any{"name": color, "swatch": "#000000"}}
	if size != "" {
		attrs["size"] = map[string]any{"name": size}
	}
	st := map[string]any{"type": "UNLIMITED"}
	if stock != nil {
		st = map[string]any{"type": "LIMITED", "inStock": stock}
	}
	return map[string]any{"id": id, "name": "x", "sku": "SKU-" + id, "unitPrice": map[string]any{"value": price, "currency": "USD"}, "attributes": attrs, "stock": st}
}

func TestFourthwallSync(t *testing.T) {
	var products []map[string]any
	srv := fakeFourthwall(t, &products)
	img := func(id string) map[string]any {
		return map[string]any{"id": id, "url": srv.URL + "/img/" + id + ".webp"}
	}
	products = []map[string]any{
		{"id": "p1", "name": "Plain T-Shirt", "slug": "plain-t-shirt", "description": "<p>A logo on a <em>white</em> tee &amp; more.</p><ul><li>Cotton</li></ul>",
			"state": map[string]any{"type": "AVAILABLE"}, "images": []any{img("a"), img("b"), img("c")},
			"variants":              []any{fwVariantJSON("v1", "White", "S", 24.4, nil), fwVariantJSON("v2", "White", "2XL", 26.4, nil), fwVariantJSON("v3", "White", "3XL", 28.4, 0)},
			"additionalInformation": []any{map[string]any{"title": "More details", "bodyHtml": "<ul><li>Relaxed fit</li></ul>"}}},
		{"id": "p2", "name": "Cap", "slug": "cap", "description": "<p>A cap.</p>",
			"state": map[string]any{"type": "AVAILABLE"}, "images": []any{img("d")},
			"variants": []any{fwVariantJSON("c1", "Black", "One size", 21.6, nil)}},
	}
	dir := starterSite(t)
	opt := FourthwallOptions{SiteDir: dir, Token: "ptkn_test", API: srv.URL, Taxonomy: "tags", Photos: 2}
	rep, err := SyncFourthwall(opt)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Written) != 2 || rep.Pictures != 3 || rep.Checkout != "https://shop.example" {
		t.Fatalf("report: %+v", rep)
	}
	tee := read(t, filepath.Join(dir, "content/products/plain-t-shirt.md"))
	for _, want := range []string{
		"title: Plain T-Shirt\nsummary: A logo on a white tee & more.\nprice: 24.4\nprice_from: true\ncurrency: USD\nweight: 1\n",
		"image: /media/shop/plain-t-shirt/1.webp", "alt: Plain T-Shirt, photo 2 of 2", "source: b",
		"choose: Size", "name: 2XL", "id: v2", "sold_out: true", "item: '{id}:{qty}'",
		"action: https://shop.example/cart/checkout",
		"buy: https://shop.example/cart/checkout?currency=USD&products=v1%3A1",
		"A logo on a *white* tee & more.", "## More details", "- Relaxed fit",
	} {
		if !strings.Contains(tee, want) {
			t.Errorf("tee page lacks %q:\n%s", want, tee)
		}
	}
	if strings.Contains(tee, "group:") || strings.Contains(tee, "photo 3") {
		t.Errorf("one color needs no groups, and -photos 2 keeps two:\n%s", tee)
	}
	cap := read(t, filepath.Join(dir, "content/products/cap.md"))
	if !strings.Contains(cap, "tags: [Hats]") || strings.Contains(cap, "choose:") == false {
		t.Errorf("cap page:\n%s", cap)
	}
	if _, err := os.Stat(filepath.Join(dir, "static/media/shop/plain-t-shirt/2-480w.webp")); err != nil {
		t.Fatal("smaller sizes missing")
	}

	// Run again: nothing changes, nothing is fetched. A key the site added
	// stays; a photo the shop dropped goes.
	os.WriteFile(filepath.Join(dir, "content/products/cap.md"), []byte(strings.Replace(cap, "title: Cap\n", "title: Cap\nfeatured: true\n", 1)), 0o644)
	if rep, err = SyncFourthwall(opt); err != nil || len(rep.Written) != 1 || rep.Unchanged != 1 || rep.Pictures != 0 {
		t.Fatalf("second run: %v %+v", err, rep)
	}
	if !strings.Contains(read(t, filepath.Join(dir, "content/products/cap.md")), "featured: true") {
		t.Fatal("the site's own front matter was dropped")
	}
	products[0]["images"] = []any{img("a")}
	if _, err = SyncFourthwall(opt); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "static/media/shop/plain-t-shirt/2.webp")); !os.IsNotExist(err) {
		t.Fatal("a dropped photo stayed")
	}

	// A product leaves the shop: reported, then deleted with Prune. A page
	// the site wrote itself is never touched.
	os.WriteFile(filepath.Join(dir, "content/products/mine.md"), []byte("---\ntitle: Mine\n---\n"), 0o644)
	products = products[:1]
	if rep, err = SyncFourthwall(opt); err != nil || len(rep.Gone) != 1 || rep.Gone[0] != "content/products/cap.md" {
		t.Fatalf("gone: %v %+v", err, rep)
	}
	opt.Prune = true
	if rep, err = SyncFourthwall(opt); err != nil || len(rep.Removed) != 1 {
		t.Fatalf("prune: %v %+v", err, rep)
	}
	if _, err := os.Stat(filepath.Join(dir, "content/products/mine.md")); err != nil {
		t.Fatal("pruned the site's own page")
	}
	if _, err := os.Stat(filepath.Join(dir, "static/media/shop/cap")); !os.IsNotExist(err) {
		t.Fatal("the pruned product's photos stayed")
	}

	opt.Token = "wrong"
	if _, err := SyncFourthwall(opt); err == nil || !strings.Contains(err.Error(), "turned the storefront token down") {
		t.Fatalf("bad token: %v", err)
	}
}

func TestFourthwallPhotosByColor(t *testing.T) {
	im := func(ids ...string) []fwImage {
		var out []fwImage
		for _, id := range ids {
			out = append(out, fwImage{ID: id})
		}
		return out
	}
	variant := func(color string, ids ...string) fwVariant {
		var v fwVariant
		v.Attributes.Color = &struct {
			Name   string `json:"name"`
			Swatch string `json:"swatch"`
		}{Name: color}
		v.Images = im(ids...)
		return v
	}
	p := fwProduct{Images: im("a", "b", "c", "d", "e"), Variants: []fwVariant{
		variant("Red", "a", "b", "e"), variant("Red", "a", "b", "e"), variant("Blue", "c", "d", "e"),
	}}
	got := func(ps []fwPhoto) string {
		var s []string
		for _, p := range ps {
			s = append(s, p.ID+"="+p.group)
		}
		return strings.Join(s, " ")
	}
	if g := got(fwPhotos(p, []string{"Red", "Blue"}, 0)); g != "a=Red b=Red c=Blue d=Blue e=" {
		t.Errorf("every photo, by color, a shared one last: %s", g)
	}
	if g := got(fwPhotos(p, []string{"Red", "Blue"}, 1)); g != "a=Red c=Blue e=" {
		t.Errorf("one of each color: %s", g)
	}
	if g := got(fwPhotos(p, []string{"Red"}, 2)); g != "a= b=" {
		t.Errorf("one color, the product's own list: %s", g)
	}
}

func TestFourthwallStaleCopy(t *testing.T) {
	fwStaleWait = 0
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Cache-Control", "max-age=60,public")
		if calls == 1 {
			w.Header().Set("Age", "2037") // the cache's old copy
			w.Write([]byte("old"))
			return
		}
		w.Header().Set("Age", "0")
		w.Write([]byte("new"))
	}))
	defer srv.Close()
	res, err := fwFresh(srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if string(b) != "new" || calls != 2 {
		t.Fatalf("got %q after %d calls", b, calls)
	}
	for h, want := range map[[2]string]bool{
		{"61", "max-age=60"}: true, {"60", "public, max-age=60"}: false, {"", "max-age=60"}: false, {"999", "no-store"}: false,
	} {
		if got := staleCopy(http.Header{"Age": {h[0]}, "Cache-Control": {h[1]}}); got != want {
			t.Errorf("Age %q, %q: stale %v", h[0], h[1], got)
		}
	}
}
