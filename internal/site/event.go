package site

import (
	"fmt"
	"strings"
	"time"

	_ "time/tzdata" // time zones work on hosts and containers without a zone database
)

// Event is what makes a page an event: a start: in its front matter.
//
//	start: 2026-11-07 18:00     # in the site's timezone: (site.yaml)
//	end: 2026-11-07 20:00       # optional
//	location: The Grange Hall, 12 Main St
//	link: https://tickets.example.org/open-house   # optional: tickets, sign-up
//	status: cancelled           # optional
//
// A date without a time (start: 2026-11-07) is an all-day event, and end: is
// then its last day. A time written with a zone (2026-11-07T18:00-07:00) is
// taken as written.
type Event struct {
	Start    time.Time // in the site's time zone
	End      time.Time // timed: when it ends (zero if not given); all-day: its last day
	AllDay   bool
	Location string
	Link     string
	Status   string // "", "cancelled" or "postponed"
}

// Ends is when the event stops being upcoming: its end, or for an event
// without one, the end of the day it starts (so today's events stay listed
// all day).
func (e *Event) Ends() time.Time {
	switch {
	case e.AllDay:
		last := e.End
		if last.IsZero() {
			last = e.Start
		}
		return midnightAfter(last)
	case !e.End.IsZero():
		return e.End
	}
	return midnightAfter(e.Start)
}

func midnightAfter(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d+1, 0, 0, 0, 0, t.Location())
}

// Cancelled says whether the event is off.
func (e *Event) Cancelled() bool { return e.Status == "cancelled" }

// When says when the event is, in plain English, in the site's time zone:
// "Saturday, November 7, 2026, 6:00–8:00 pm MST".
func (e *Event) When() string {
	s := e.Start
	if e.AllDay {
		last := e.End
		if last.IsZero() || !last.After(s) {
			return s.Format("Monday, January 2, 2006")
		}
		switch {
		case last.Year() != s.Year():
			return s.Format("January 2, 2006") + " – " + last.Format("January 2, 2006")
		case last.Month() != s.Month():
			return s.Format("January 2") + " – " + last.Format("January 2, 2006")
		}
		return s.Format("January 2") + "–" + last.Format("2, 2006")
	}
	day := s.Format("Monday, January 2, 2006")
	zone := s.Format("MST")
	if e.End.IsZero() {
		return fmt.Sprintf("%s, %s %s", day, clock(s, true), zone)
	}
	end := e.End
	if sameDay(s, end) {
		// "6:00–8:00 pm", or "11:00 am – 1:00 pm" across noon.
		if s.Format("pm") == end.Format("pm") {
			return fmt.Sprintf("%s, %s–%s %s", day, clock(s, false), clock(end, true), zone)
		}
		return fmt.Sprintf("%s, %s – %s %s", day, clock(s, true), clock(end, true), zone)
	}
	return fmt.Sprintf("%s, %s – %s, %s %s", s.Format("Mon, Jan 2, 2006"), clock(s, true), end.Format("Mon, Jan 2, 2006"), clock(end, true), zone)
}

func clock(t time.Time, ampm bool) string {
	if ampm {
		return t.Format("3:04 pm")
	}
	return t.Format("3:04")
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

var (
	zonedLayouts = []string{time.RFC3339, "2006-01-02T15:04Z07:00", "2006-01-02 15:04:05Z07:00", "2006-01-02 15:04Z07:00"}
	localLayouts = []string{"2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02 15:04"}
)

// eventTime reads a start: or end: as written, in loc unless it names a zone.
func eventTime(s string, loc *time.Location) (t time.Time, dateOnly bool, err error) {
	s = strings.TrimSpace(s)
	for _, l := range zonedLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.In(loc), false, nil
		}
	}
	for _, l := range localLayouts {
		if t, err := time.ParseInLocation(l, s, loc); err == nil {
			return t, false, nil
		}
	}
	if t, err := time.ParseInLocation("2006-01-02", s, loc); err == nil {
		return t, true, nil
	}
	return time.Time{}, false, fmt.Errorf("%q is not a date and time (use 2026-11-07 18:00, or 2026-11-07 for all day)", s)
}

// resolveEvent turns a page's start: and end: into its Event.
func (p *Page) resolveEvent(loc *time.Location) error {
	if p.eventStart == "" {
		return nil
	}
	start, allDay, err := eventTime(p.eventStart, loc)
	if err != nil {
		return fmt.Errorf("start: %w", err)
	}
	ev := &Event{Start: start, AllDay: allDay, Location: str(p.Params["location"]), Link: str(p.Params["link"])}
	if p.eventEnd != "" {
		end, endDateOnly, err := eventTime(p.eventEnd, loc)
		if err != nil {
			return fmt.Errorf("end: %w", err)
		}
		if endDateOnly != allDay {
			return fmt.Errorf("end: give both start and end a time, or neither (for an all-day event)")
		}
		if end.Before(start) {
			return fmt.Errorf("end: %s is before the start", p.eventEnd)
		}
		ev.End = end
	}
	switch st := strings.ToLower(str(p.Params["status"])); st {
	case "", "cancelled", "postponed":
		ev.Status = st
	case "canceled":
		ev.Status = "cancelled"
	default:
		return fmt.Errorf("status: %q should be cancelled or postponed (or left out)", st)
	}
	if ev.Link != "" && !strings.HasPrefix(ev.Link, "https://") && !strings.HasPrefix(ev.Link, "http://") && !strings.HasPrefix(ev.Link, "/") {
		return fmt.Errorf("link: %q should be a web address", ev.Link)
	}
	p.Event = ev
	return nil
}

// EventICS is where the page's own calendar file is: "Add to calendar".
func (p *Page) EventICS() string {
	if p.Event == nil || !strings.HasSuffix(p.Path, "/") {
		return ""
	}
	return p.Path + "event.ics"
}

// Location is the site's time zone, from timezone: in site.yaml (UTC if
// unset).
func (c *Config) Location() (*time.Location, error) {
	if c.Timezone == "" {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return nil, fmt.Errorf("timezone: %q is not a time zone name (like America/Phoenix or Europe/Berlin)", c.Timezone)
	}
	return loc, nil
}
