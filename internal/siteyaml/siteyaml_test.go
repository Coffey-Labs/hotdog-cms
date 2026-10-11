package siteyaml

import "testing"

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
