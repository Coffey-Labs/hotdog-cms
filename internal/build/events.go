package build

import (
	"crypto/sha256"
	"encoding/hex"
	"html/template"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
)

// Events: pages with a start: (site/event.go). A collection with
// calendar: true gets a feed at /<collection>/calendar.ics that calendar
// apps subscribe to, and every event page gets its own event.ics for "Add to
// calendar". Times are written in UTC, which every calendar reads the same
// way; all-day events are dates.

func eventFuncs(s *site.Site) map[string]any {
	limit := func(pages []*site.Page, n []int) []*site.Page {
		if len(n) > 0 && n[0] >= 0 && n[0] < len(pages) {
			return pages[:n[0]]
		}
		return pages
	}
	return map[string]any{
		// upcoming "events" [n]: events that haven't ended, soonest first.
		"upcoming": func(collection string, n ...int) []*site.Page {
			return limit(upcoming(s, collection), n)
		},
		// pastEvents "events" [n]: events that have ended, latest first.
		"pastEvents": func(collection string, n ...int) []*site.Page {
			var out []*site.Page
			for _, p := range s.Collection(collection) {
				if p.Event != nil && !p.Event.Ends().After(s.Now) {
					out = append(out, p)
				}
			}
			sort.SliceStable(out, func(i, j int) bool { return out[i].Event.Start.After(out[j].Event.Start) })
			return limit(out, n)
		},
		// calendarURL "events": the collection's feed, https://…/events/calendar.ics.
		"calendarURL": func(collection string) string { return s.Config.URL + calendarPath(collection) },
		// webcal "events": the same feed as a webcal:// link, which opens the
		// visitor's calendar app to subscribe. html/template allows only web
		// and mail links in href; this one is built here from the site's own
		// address, never from page text, so it's marked as a safe URL.
		"webcal": func(collection string) template.URL {
			if !nameRe.MatchString(collection) {
				return ""
			}
			u, err := url.Parse(s.Config.URL + calendarPath(collection))
			if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
				return ""
			}
			u.Scheme = "webcal"
			return template.URL(u.String())
		},
		// banners .: the notices showing on this page now (data/banners.yaml).
		"banners": func(p *site.Page) []site.Banner { return s.BannersFor(p) },
		// eventData .: the event as schema.org data, for a JSON-LD script tag.
		"eventData": func(p *site.Page) map[string]any { return eventData(p) },
	}
}

func init() {
	// For every server that's Go: serve, previews, the editor's live preview.
	_ = mime.AddExtensionType(".ics", "text/calendar; charset=utf-8")
}

var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

func calendarPath(collection string) string { return "/" + collection + "/calendar.ics" }

func upcoming(s *site.Site, collection string) []*site.Page {
	var out []*site.Page
	for _, p := range s.Collection(collection) {
		if p.Event != nil && p.Event.Ends().After(s.Now) {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Event.Start.Before(out[j].Event.Start) })
	return out
}

func eventData(p *site.Page) map[string]any {
	e := p.Event
	if e == nil {
		return nil
	}
	d := map[string]any{"@context": "https://schema.org", "@type": "Event", "name": p.Title, "url": p.Permalink}
	if e.AllDay {
		d["startDate"] = e.Start.Format("2006-01-02")
		if !e.End.IsZero() {
			d["endDate"] = e.End.Format("2006-01-02")
		}
	} else {
		d["startDate"] = e.Start.Format(time.RFC3339)
		if !e.End.IsZero() {
			d["endDate"] = e.End.Format(time.RFC3339)
		}
	}
	if p.Summary != "" {
		d["description"] = p.Summary
	}
	if e.Location != "" {
		d["location"] = map[string]any{"@type": "Place", "name": e.Location}
	}
	switch e.Status {
	case "cancelled":
		d["eventStatus"] = "https://schema.org/EventCancelled"
	case "postponed":
		d["eventStatus"] = "https://schema.org/EventPostponed"
	default:
		d["eventStatus"] = "https://schema.org/EventScheduled"
	}
	return d
}

// writeCalendars writes every calendar collection's feed and every event's
// own file.
func writeCalendars(s *site.Site, out string) error {
	host := ""
	if u, err := url.Parse(s.Config.URL); err == nil {
		host = u.Hostname()
	}
	for name, cc := range s.Config.Collections {
		if !cc.Calendar {
			continue
		}
		var events []*site.Page
		for _, p := range s.Collection(name) {
			if p.Event != nil {
				events = append(events, p)
			}
		}
		title := cc.CalendarName
		if title == "" {
			list := s.Lists[name]
			coll := name
			if list != nil && list.Title != "" {
				coll = list.Title
			}
			title = s.Config.Name + ": " + coll
		}
		if err := writeICS(filepath.Join(out, name, "calendar.ics"), ics(s, host, title, events)); err != nil {
			return err
		}
	}
	for _, p := range s.Pages {
		if f := p.EventICS(); f != "" {
			if err := writeICS(filepath.Join(out, filepath.FromSlash(strings.TrimPrefix(f, "/"))), ics(s, host, p.Title, []*site.Page{p})); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeICS(file, body string) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	return os.WriteFile(file, []byte(body), 0o644)
}

// ics writes an iCalendar (RFC 5545) calendar of events.
func ics(s *site.Site, host, name string, events []*site.Page) string {
	var b strings.Builder
	line := func(l string) { b.WriteString(fold(l)) }
	line("BEGIN:VCALENDAR")
	line("VERSION:2.0")
	line("PRODID:-//Coffey Labs//HotDog CMS//EN")
	line("CALSCALE:GREGORIAN")
	line("METHOD:PUBLISH")
	line("X-WR-CALNAME:" + text(name))
	line("REFRESH-INTERVAL;VALUE=DURATION:PT6H")
	line("X-PUBLISHED-TTL:PT6H")
	for _, p := range events {
		e := p.Event
		sum := sha256.Sum256([]byte(p.Path))
		line("BEGIN:VEVENT")
		line("UID:" + hex.EncodeToString(sum[:])[:20] + "@" + host)
		stamp := p.Updated
		if stamp.IsZero() {
			stamp = p.Date
		}
		if stamp.IsZero() {
			stamp = e.Start
		}
		line("DTSTAMP:" + utc(stamp))
		if e.AllDay {
			last := e.End
			if last.IsZero() {
				last = e.Start
			}
			y, m, d := last.Date()
			line("DTSTART;VALUE=DATE:" + e.Start.Format("20060102"))
			line("DTEND;VALUE=DATE:" + time.Date(y, m, d+1, 0, 0, 0, 0, time.UTC).Format("20060102"))
		} else {
			line("DTSTART:" + utc(e.Start))
			if !e.End.IsZero() {
				line("DTEND:" + utc(e.End))
			}
		}
		line("SUMMARY:" + text(p.Title))
		desc := p.Summary
		if e.Link != "" {
			desc = strings.TrimSpace(desc + "\n\n" + absolute(s, e.Link))
		}
		desc = strings.TrimSpace(desc + "\n\n" + p.Permalink)
		line("DESCRIPTION:" + text(desc))
		if e.Location != "" {
			line("LOCATION:" + text(e.Location))
		}
		line("URL:" + p.Permalink)
		switch e.Status {
		case "cancelled":
			line("STATUS:CANCELLED")
		case "postponed":
			line("STATUS:TENTATIVE")
		default:
			line("STATUS:CONFIRMED")
		}
		line("END:VEVENT")
	}
	line("END:VCALENDAR")
	return b.String()
}

func absolute(s *site.Site, link string) string {
	if strings.HasPrefix(link, "/") {
		return s.Config.URL + link
	}
	return link
}

func utc(t time.Time) string { return t.UTC().Format("20060102T150405Z") }

// text escapes a TEXT value: backslash, semicolon, comma and line breaks.
func text(v string) string {
	r := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`, "\r", `\n`)
	return r.Replace(v)
}

// fold ends a content line with CRLF, folding it at 75 octets without
// splitting a character.
func fold(l string) string {
	var b strings.Builder
	n := 0
	for _, r := range l {
		w := utf8.RuneLen(r)
		if n+w > 75 {
			b.WriteString("\r\n ")
			n = 1
		}
		b.WriteRune(r)
		n += w
	}
	b.WriteString("\r\n")
	return b.String()
}
