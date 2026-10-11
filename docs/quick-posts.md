# Quick posts

For what has to go up in minutes: a closure, holiday hours, an offer, an
event, a change on the front page. Open a site in the editor and choose
**Quick post** (or type "quick" in the command palette).

| | |
|---|---|
| **Banner or alert** | A line across the top of every page, or the home page only. An alert stands out. It can start later and come down by itself: at the end of today, tomorrow, in a week, or at a time you pick. |
| **News post** | A short announcement, dated today, in the collection you choose (news, if the site has one). |
| **Event** | Something on a date, with a time or all day, where, and a tickets link. It goes into the site's calendar and its feed, so subscribers see it in their calendar app. Shown when the site has a calendar (`hotdog-cms pack add calendar`; see [events.md](events.md)). |
| **Front page** | The home page's sections, from the section library: change the words and buttons, add the upcoming events. |

## Post now

**Post now** does in one go what the rest of the editor does in steps:
1. commits the change on a new draft;
2. opens a review for it;
3. runs the checks;
4. publishes it (merges the review), if the platform lets you merge on this
   site.

If the checks find a problem, nothing is published, and the review stays
open with the problem listed. If the platform doesn't let you merge, the
review waits for someone who can. **Send for review** stops after step 3.

A quick post is an ordinary change, made as you, in git, so History can undo
it like any other. The live site updates when the site's publishing pipeline
runs, as it does for every change.

## Banners

Banners are kept in `data/banners.yaml`, which the quick post writes and
anyone can edit by hand:

```yaml
- text: Closed Thursday for Thanksgiving.
  link: /news/holiday-hours/   # optional: a page on the site, or a web address
  link_text: Holiday hours     # default "More"
  tone: alert                  # info (the default) or alert
  show: home                   # all (the default) or home
  from: 2026-11-20 00:00       # optional, in the site's timezone
  until: 2026-11-27 00:00      # optional
```

- A banner is one line, up to 280 characters. Put the rest on a page and
  link to it.
- Times are in the site's `timezone:` (`site.yaml`). The site changes at
  those times only when it's rebuilt: the pull agent rebuilds then by
  itself, and the CI files from `hotdog-cms ci` rebuild hourly.
- Writing a banner tidies away the ones that are over. **Take down** removes
  one straight away (as a published change, like everything else).
- Themes show banners with `{{ range banners . }}`; the starter's
  `partials/banner.html` is an example. Each has `.Text`, `.Link`,
  `.LinkText`, `.Tone` and `.Show`.
