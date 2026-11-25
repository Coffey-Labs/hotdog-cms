package site

import "testing"

// Markdown read away from the site has nothing to be relative to: every
// link is made absolute, source-file links become page addresses, and code
// shows links as written.
func TestAbsoluteMarkdown(t *testing.T) {
	guide := &Page{Source: "docs/install/fresh.md", Path: "/install/fresh/", Permalink: "https://example.org/install/fresh/"}
	server := &Page{Source: "docs/install/server.md", Path: "/install/server/", Permalink: "https://example.org/install/server/"}
	guide.Raw = "See [the server](server.md#ports), [home](/), ![a map](map.png \"Map\"), " +
		"[elsewhere](https://other.example/x.md), [here](#top) and <a href=\"../run/\">run</a>.\n" +
		"Write `[x](server.md)` like that.\n\n```md\n[x](server.md)\n```\n"
	s := &Site{Pages: []*Page{guide, server}}
	got := s.AbsoluteMarkdown()(guide)
	want := "See [the server](https://example.org/install/server/#ports), [home](https://example.org/), " +
		"![a map](https://example.org/install/fresh/map.png \"Map\"), [elsewhere](https://other.example/x.md), " +
		"[here](#top) and <a href=\"https://example.org/install/run/\">run</a>.\n" +
		"Write `[x](server.md)` like that.\n\n```md\n[x](server.md)\n```\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}
