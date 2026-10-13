package site

import (
	"reflect"
	"testing"
)

func TestHeadings(t *testing.T) {
	md := NewMarkdown(MarkdownConfig{}, "example.org")
	body, err := md.Render([]byte("# Title\n\nIntro.\n\n## Install it\n\n### On *Linux* & BSD\n\n#### Too deep\n\n## Run it {#run}\n\n!!! note \"A note\"\n\n    ## Inside a note\n"))
	if err != nil {
		t.Fatal(err)
	}
	got := headings(body)
	want := []Heading{
		{2, "install-it", "Install it"},
		{3, "on-linux--bsd", "On Linux & BSD"},
		{2, "run", "Run it"},
		{2, "inside-a-note", "Inside a note"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("headings:\n got %+v\nwant %+v", got, want)
	}
}
