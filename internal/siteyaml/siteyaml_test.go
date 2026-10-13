package siteyaml

import (
	"strings"
	"testing"
)

func TestAddEntry(t *testing.T) {
	in := "name: Test\n\n# Collections.\ncollections:\n  articles:\n    layout: article\n    # paginate: 20\n\nmenus:\n  main: []\n"
	got, err := AddEntry(in, "collections", "events", map[string]any{"layout": "event", "calendar": true})
	if err != nil {
		t.Fatal(err)
	}
	want := "name: Test\n\n# Collections.\ncollections:\n  articles:\n    layout: article\n    # paginate: 20\n  events:\n    calendar: true\n    layout: event\n\nmenus:\n  main: []\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	got, err = AddEntry("name: Test\n", "collections", "events", map[string]any{"calendar": true})
	if err != nil || got != "name: Test\ncollections:\n  events:\n    calendar: true\n" {
		t.Errorf("new block: %q %v", got, err)
	}
	if _, err := AddEntry("collections: {a: {}}\n", "collections", "events", map[string]any{"calendar": true}); err == nil {
		t.Error("changed a one-line mapping")
	}
}

func TestSetEntry(t *testing.T) {
	in := "name: T\ncollections:\n  articles:\n    layout: article\n    # paginate: 20\n\n  # The docs.\n  docs:\n    layout: doc\n    search: true\nmenus:\n  main: []\n"
	out, err := SetEntry(in, "collections", "docs", map[string]any{"layout": "doc", "path": "/"})
	if err != nil {
		t.Fatal(err)
	}
	want := "name: T\ncollections:\n  articles:\n    layout: article\n    # paginate: 20\n\n  # The docs.\n  docs:\n    layout: doc\n    path: /\nmenus:\n  main: []\n"
	if out != want {
		t.Errorf("got\n%s\nwant\n%s", out, want)
	}
	out, _ = SetEntry(in, "collections", "articles", map[string]any{"layout": "post"})
	if !strings.Contains(out, "  articles:\n    layout: post\n\n  # The docs.\n  docs:\n") {
		t.Errorf("first entry, comment before the next kept:\n%s", out)
	}
	out, _ = SetEntry(in, "collections", "events", map[string]any{"calendar": true})
	if !strings.Contains(out, "    search: true\n  events:\n    calendar: true\nmenus:") {
		t.Errorf("a new entry is added:\n%s", out)
	}
}
