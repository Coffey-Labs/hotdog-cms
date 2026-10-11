# Events and calendars

Any page with a `start:` is an event. A collection of them can be a
calendar that people subscribe to in their calendar app, so new events
appear there by themselves.

The quickest way in is the calendar pack that comes with HotDog CMS:

    hotdog-cms pack add calendar

It adds an `events` collection to `site.yaml`, a page for each event, a list
of what's coming up at `/events/`, and an "Upcoming events" section for any
page (the home page, say).

## An event

`content/events/open-house.md`:

```yaml
---
title: Spring open house
summary: Tours, coffee, and a look at what's new.
start: 2026-11-07 18:00
end: 2026-11-07 20:00            # optional
location: The Grange Hall, 12 Main St
link: https://tickets.example.org/open-house   # optional: tickets, sign-up
status: cancelled                # optional: cancelled or postponed
---
What's happening, in Markdown.
```

- **Times are in the site's time zone,** set once in `site.yaml`:
  `timezone: America/Phoenix` (any IANA name; UTC if unset). A time written
  with its own zone (`2026-11-07T18:00-07:00`) is taken as written.
- **A date without a time is all day:** `start: 2026-12-04` and
  `end: 2026-12-06` is three days. Give both start and end a time, or
  neither.
- An event with no `end:` stays on the upcoming list until the end of its
  day.
- A cancelled event stays listed, marked as cancelled, so people who saw it
  find out. Calendar apps show it as cancelled too.

## The calendar feed

```yaml
# site.yaml
collections:
  events:
    calendar: true                # write /events/calendar.ics
    calendar_name: Grange events  # what calendar apps call it (default "<site>: Events")
```

- `/events/calendar.ics` is an iCalendar feed of the collection. People
  subscribe with a `webcal://` link (`{{ webcal "events" }}`), which opens
  their calendar app, or paste its address (`{{ calendarURL "events" }}`)
  into Google Calendar's "From URL". Apps check it for changes every few
  hours.
- Every event also gets its own `event.ics` beside its page, for "Add to my
  calendar" (`{{ .EventICS }}`).
- Times are written in UTC, which every calendar reads the same way, and
  all-day events as dates. The files are checked against Mozilla's ical.js
  (Thunderbird's) and Python's icalendar.
- Web servers should send `.ics` as `text/calendar`. HotDog CMS's container
  images and its own servers do. For nginx elsewhere, add
  `location ~ \.ics$ { default_type "text/calendar; charset=utf-8"; }`.

## In templates

| | |
|---|---|
| `upcoming "events" 3` | Events that haven't ended, soonest first (the number is optional) |
| `pastEvents "events" 10` | Events that have ended, latest first |
| `.Event` | Set on an event page: `.Start`, `.End`, `.AllDay`, `.Location`, `.Link`, `.Status`, `.Cancelled` |
| `.Event.When` | "Saturday, November 7, 2026, 6:00–8:00 pm MST", in the site's time zone |
| `.EventICS` | The page's own `event.ics` |
| `webcal "events"`, `calendarURL "events"` | The feed, to subscribe or to paste |
| `eventData .` | The event as schema.org data, for `<script type="application/ld+json">`; the starter's schema partial uses it, so search engines can show the event |

A calendar collection is sorted by start, and `list_layout:` picks the
layout for its own page (the pack's is `events`).

## Keeping "upcoming" current

What counts as upcoming depends on when the site was built. The pull agent
rebuilds when an event ends, as it does for scheduled pages, and the CI
files `hotdog-cms ci` writes rebuild hourly. A site published only by hand
needs publishing again for past events to move down the list. The feed
itself is unaffected: calendar apps work out what's past.

Repeating events (every Tuesday) aren't supported yet: add each one.
