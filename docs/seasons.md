# Seasons

`seasons:` in site.yaml dresses a site for holidays through the year. Each
visitor's browser works out the day from its own date every time a page
loads, so nothing is scheduled and nothing is rebuilt when a holiday starts
or ends. The user guide is on hotdogcms.org (Writing › Holidays); this page
is how it's put together.

```yaml
seasons: true            # or:
seasons:
  anchor: header.top     # the strip hangs under this; default the window's top
  logo: .brand-logo      # the day's prop is laid over this
  art:                   # or swap a picture for your own drawings
    - { image: logo.svg, art: /img/season/logo-{prop}.svg }
  skip: footer           # pictures in here keep their usual look
  only: [advent, pride]  # or except: [halloween]
  greetings: false       # no greeting on the strip
```

`{{ seasons }}` in the head writes the tags (the starter has it). With
seasons off it writes nothing and no file is added.

## The pieces

- `internal/seasons/src/core.js`: the dates. Holidays (`HOLIDAYS`, one strip
  and prop each; the first that matches dresses the page, all are greeted)
  and December's observances (`SCENES`, drawn side by side at the end of the
  strip). Lunar dates come from the browser's `Intl` calendars (Chinese,
  Hebrew, Islamic, Persian); where a browser lacks one, those holidays don't
  show. Easter is computed (Western and Orthodox), Holi and Diwali from the
  moon's phases, Dongzhi from the solstice. Advent alone runs on UTC.
- `src/render.js`: the strips and scenes, drawn as SVG in the page.
- `src/run.js`: places the strip, swaps pictures, lays props, handles
  `?season=`.
- `src/season.css`: the strip's look. The build links it, and adds where each
  kind of prop sits on a logo (`.season-prop-head`, `-eyes`, `-ear`, `-pin`,
  `-side`), which a site can restyle.
- `props/*.svg`: each prop on its own, fingerprinted into
  `hotdog-seasons/<prop>.<hash>.svg`.

The build checks `only:` and `except:` against the holiday names, and that a
site's own `art:` has a drawing for every prop (`static/` plus the address,
with `{prop}` filled in).

## Previewing

`?season=<id>` shows one holiday, `?season=YYYY-MM-DD` everything that date
shows, `?season=off` nothing and `?season=auto` today again; each lasts for
the browser tab's session. `TZ=UTC node internal/seasons/src/dates-test.js
2026 2027` lists every holiday's days for those years.
