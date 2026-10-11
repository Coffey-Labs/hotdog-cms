package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEvents(t *testing.T) {
	dir := writeStarter(t)
	cfg, _ := os.ReadFile(filepath.Join(dir, "site.yaml"))
	cfg = []byte(strings.Replace(string(cfg), "language: en\n", "language: en\ntimezone: America/Phoenix\n", 1))
	cfg = []byte(strings.Replace(string(cfg), "collections:\n", "collections:\n  events:\n    calendar: true\n", 1))
	os.WriteFile(filepath.Join(dir, "site.yaml"), cfg, 0o644)
	os.MkdirAll(filepath.Join(dir, "content", "events"), 0o755)
	write := func(name, fm string) {
		os.WriteFile(filepath.Join(dir, "content", "events", name), []byte("---\n"+fm+"---\nAbout it.\n"), 0o644)
	}
	write("open-house.md", "title: Open house\nstart: 2026-11-07 18:00\nend: 2026-11-07 20:00\nlocation: Hall, 12 Main St; Mesa\n")
	write("fair.md", "title: Fair\nstart: 2026-12-04\nend: 2026-12-06\n")
	write("zoned.md", "title: Zoned\nstart: 2026-11-20T09:30:00+01:00\n")
	write("swap.md", "title: Swap\nstart: 2026-11-14 11:00\nstatus: cancelled\n")
	write("picnic.md", "title: Picnic\nstart: 2026-07-04 12:00\nend: 2026-07-04 15:00\n")

	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	res, err := Run(Options{SiteDir: dir, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	read := func(p string) string { b, _ := os.ReadFile(filepath.Join(res.Out, p)); return string(b) }
	feed := read("events/calendar.ics")
	for _, want := range []string{
		"X-WR-CALNAME:Test: Events\r\n",
		"SUMMARY:Open house\r\nDESCRIPTION:",
		"DTSTART:20261108T010000Z\r\nDTEND:20261108T030000Z\r\n", // 6 pm in Phoenix (UTC-7, no daylight saving)
		"DTSTART;VALUE=DATE:20261204\r\nDTEND;VALUE=DATE:20261207\r\n",
		"DTSTART:20261120T083000Z\r\n", // its own zone, kept
		"DTSTART:20260704T190000Z\r\n",
		`LOCATION:Hall\, 12 Main St\; Mesa` + "\r\n",
		"STATUS:CANCELLED\r\n",
	} {
		if !strings.Contains(feed, want) {
			t.Errorf("feed lacks %q", want)
		}
	}
	for _, l := range strings.Split(feed, "\r\n") {
		if len(l) > 75 {
			t.Errorf("line over 75 octets: %q", l)
		}
	}
	if !strings.Contains(read("events/open-house/event.ics"), "SUMMARY:Open house") {
		t.Error("no event.ics beside the event")
	}
	page := read("events/open-house/index.html")
	if !strings.Contains(page, `"@type":"Event"`) || !strings.Contains(page, `"startDate":"2026-11-07T18:00:00-07:00"`) {
		t.Errorf("event page lacks its structured data:\n%s", page)
	}
	// The next moment the site changes by itself: the open house ending.
	if want := time.Date(2026, 11, 8, 3, 0, 0, 0, time.UTC); !res.Site.NextScheduled.Equal(want) {
		t.Errorf("next scheduled %v, want %v", res.Site.NextScheduled, want)
	}
	// Upcoming, as templates see it: today's open house still on, the
	// picnic gone.
	up := upcoming(res.Site, "events")
	var names []string
	for _, p := range up {
		names = append(names, p.Title)
	}
	if got := strings.Join(names, ", "); got != "Open house, Swap, Zoned, Fair" {
		t.Errorf("upcoming: %s", got)
	}
	if w := up[0].Event.When(); w != "Saturday, November 7, 2026, 6:00–8:00 pm MST" {
		t.Errorf("when: %s", w)
	}
	if w := up[3].Event.When(); w != "December 4–6, 2026" {
		t.Errorf("when: %s", w)
	}

	write("bad.md", "title: Bad\nstart: 2026-11-07 18:00\nend: 2026-11-07\n")
	if _, err := Run(Options{SiteDir: dir, Now: now}); err == nil || !strings.Contains(err.Error(), "give both start and end a time") {
		t.Errorf("mixed all-day and timed: %v", err)
	}
}

func TestBanners(t *testing.T) {
	dir := writeStarter(t)
	os.WriteFile(filepath.Join(dir, "data", "banners.yaml"), []byte(`- text: Closed Thursday for Thanksgiving.
  link: /about/
  link_text: Hours
  tone: alert
  from: 2026-11-20 00:00
  until: 2026-11-27 00:00
- text: New on the home page only.
  show: home
- text: Over already.
  until: 2026-11-01 00:00
`), 0o644)
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	res, err := Run(Options{SiteDir: dir, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	read := func(p string) string { b, _ := os.ReadFile(filepath.Join(res.Out, p)); return string(b) }
	home, about := read("index.html"), read("about/index.html")
	if !strings.Contains(home, `<strong>Closed Thursday for Thanksgiving.</strong> <a href="/about/">Hours</a>`) || !strings.Contains(home, "New on the home page only.") {
		t.Errorf("home banners:\n%s", home)
	}
	if !strings.Contains(about, "Closed Thursday") || strings.Contains(about, "home page only") || strings.Contains(home+about, "Over already") {
		t.Error("banners on the wrong pages")
	}
	if want := time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC); !res.Site.NextScheduled.Equal(want) {
		t.Errorf("next scheduled %v, want %v (the banner coming down)", res.Site.NextScheduled, want)
	}
	os.WriteFile(filepath.Join(dir, "data", "banners.yaml"), []byte("- text: \"\"\n"), 0o644)
	if _, err := Run(Options{SiteDir: dir, Now: now}); err == nil {
		t.Error("an empty banner built")
	}
}

func TestFeedBeforeFirstPost(t *testing.T) {
	dir := writeStarter(t)
	os.MkdirAll(filepath.Join(dir, "content", "news"), 0o755)
	os.WriteFile(filepath.Join(dir, "content", "news", "_index.md"), []byte("---\ntitle: News\n---\n"), 0o644)
	cfg, _ := os.ReadFile(filepath.Join(dir, "site.yaml"))
	os.WriteFile(filepath.Join(dir, "site.yaml"), []byte(strings.Replace(string(cfg), "collections:\n", "collections:\n  news:\n    feed: true\n", 1)), 0o644)
	res, err := Run(Options{SiteDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	feed, err := os.ReadFile(filepath.Join(res.Out, "news", "feed.xml"))
	if err != nil || !strings.Contains(string(feed), "<feed") || strings.Contains(string(feed), "<entry") {
		t.Errorf("empty news feed: %v\n%s", err, feed)
	}
}
