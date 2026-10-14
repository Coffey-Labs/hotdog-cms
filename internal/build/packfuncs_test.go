package build

import (
	"strings"
	"testing"
	"time"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
)

func TestSortBy(t *testing.T) {
	items := []any{
		map[string]any{"term": "starter"},
		map[string]any{"n": 1},
		map[string]any{"term": "Autolyse"},
		map[string]any{"term": 10},
		map[string]any{"term": 9},
	}
	got, err := sortBy("term", items)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, it := range got {
		names = append(names, str(it.(map[string]any)["term"]))
	}
	// Numbers as numbers (9 before 10), text without regard to case, missing last.
	if strings.Join(names, ",") != "9,10,Autolyse,starter," {
		t.Errorf("order: %v", names)
	}
	a := &site.Page{Title: "b", Date: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), Params: map[string]any{"category": "Shops"}}
	b := &site.Page{Title: "A", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Params: map[string]any{"category": "Food"}}
	for key, want := range map[string]string{"Title": "A", "Date": "A", "category": "A"} {
		got, _ := sortBy(key, []*site.Page{a, b})
		if got[0].(*site.Page).Title != want {
			t.Errorf("sortBy %s: %s first", key, got[0].(*site.Page).Title)
		}
	}
	if _, err := sortBy("x", "not a list"); err == nil {
		t.Error("sorted a string")
	}
}

func TestRatingsAndStars(t *testing.T) {
	r := ratings([]any{
		map[string]any{"rating": 5},
		map[string]any{"rating": 4},
		map[string]any{"rating": "4"},
		map[string]any{"rating": 9}, // out of range: left out
		map[string]any{"name": "x"}, // no rating
	})
	if r.Count != 3 || r.Average != 4.3 {
		t.Errorf("ratings: %+v", r)
	}
	for in, want := range map[any]string{5: "★★★★★", 4.6: "★★★★★", 4.4: "★★★★☆", 0: "☆☆☆☆☆", 7: "★★★★★", "3": "★★★☆☆"} {
		if got := stars(in); got != want {
			t.Errorf("stars(%v) = %s, want %s", in, got, want)
		}
	}
}

func TestIsoDuration(t *testing.T) {
	for in, want := range map[string]string{"2:14": "PT2M14S", "1:02:03": "PT1H2M3S", "0:45": "PT45S", "12": "", "a:b": "", "": ""} {
		if got := isoDuration(in); got != want {
			t.Errorf("isoDuration(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestProductReviewsData(t *testing.T) {
	s := &site.Site{Config: &site.Config{Name: "Bakery", URL: "https://example.org"}}
	p := &site.Page{Title: "Kit", Permalink: "https://example.org/kit/", Params: map[string]any{
		"price": 24,
		"reviews": []any{
			map[string]any{"name": "Sam", "rating": 5, "text": "Works **well**."},
			map[string]any{"name": "Lee", "rating": 4},
		},
	}}
	d := productData(s, p)
	ar, _ := d["aggregateRating"].(map[string]any)
	if ar == nil || ar["ratingValue"] != "4.5" || ar["reviewCount"] != 2 {
		t.Fatalf("aggregateRating: %v", d["aggregateRating"])
	}
	rv, _ := d["review"].([]any)
	if len(rv) != 2 || rv[0].(map[string]any)["reviewBody"] != "Works well." {
		t.Errorf("review: %v", d["review"])
	}
	delete(p.Params, "reviews")
	if _, ok := productData(s, p)["aggregateRating"]; ok {
		t.Error("a product without reviews has a rating")
	}
}
