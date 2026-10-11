package site

import (
	"fmt"
	"strings"
	"testing"
)

func TestPageLinks(t *testing.T) {
	all := make([]*Page, 10)
	for i := range all {
		all[i] = &Page{Path: fmt.Sprintf("/a/page/%d/", i+1)}
	}
	row := func(n int) string {
		var b []string
		for _, l := range (&Paginator{Number: n, TotalPages: 10, All: all}).Links(1) {
			switch {
			case l.Gap:
				b = append(b, "…")
			case l.Current:
				b = append(b, fmt.Sprintf("[%d]", l.Number))
			default:
				b = append(b, fmt.Sprint(l.Number))
			}
		}
		return strings.Join(b, " ")
	}
	for n, want := range map[int]string{
		1:  "[1] 2 … 10",
		5:  "1 … 4 [5] 6 … 10",
		3:  "1 2 [3] 4 … 10",
		10: "1 … 9 [10]",
	} {
		if got := row(n); got != want {
			t.Errorf("page %d: %q, want %q", n, got, want)
		}
	}
}
