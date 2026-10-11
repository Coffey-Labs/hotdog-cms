# Privacy, consent and statistics

HotDog CMS sites don't track anyone unless the site turns statistics on, and
then only after the visitor agrees.

## Visit statistics

```yaml
# site.yaml: one of
analytics: { provider: plausible, domain: example.org }
analytics: { provider: plausible, domain: example.org, src: "https://stats.example.org/js/script.js" }
analytics: { provider: matomo, url: "https://matomo.example.org/", site_id: "1" }
analytics: { provider: ga4, id: G-XXXXXXX }
```

With `analytics:` set, every page gets a small consent script and stylesheet
through `{{ consent }}` in the head. They're files, not inline code, so a
strict Content-Security-Policy still holds.
- **The question:** on a first visit, a banner asks whether the site may
  count the visit. It sits at the foot of the page, not over it, and the page
  works without answering. **Decline comes first and looks the same as
  Accept.**
- **Nothing is counted until someone accepts.** Only then is the counter's
  script added to the page.
- **Declining** (then or later) removes the counter's cookies.
- **Global Privacy Control:** a browser that sends it has already said no,
  and isn't asked.
- **Cookie settings:** any element with `data-hotdog-consent-open` reopens the
  choice. The starter puts a "Cookie settings" button in its footer.
- **Remembered** in the browser's local storage, not in a cookie.
- **CSP:** the counter's hosts ride on the consent script's tag, so `hotdog-cms
  check -csp` includes them in the policy it prints, and so do container
  images built with `csp: true`.

**Counting without asking** needs `gated: false` in `analytics:`. That's a
choice written into `site.yaml` where a reviewer sees it. The privacy notice
says so, and `check` warns (`analytics-ungated`) on every build.

The editor's **Privacy & statistics** page sets all of this up as a form, and
saves it to a draft like any other change.

## The privacy notice

`{{ privacyFacts }}` lists what the site uses that touches visitors' data,
from the site's own settings, each time it's built:
- the statistics, and whether they wait for consent;
- each form, where its submissions go, and Turnstile if it's on;
- every host in `check.allow_third_party`;
- anything listed under `privacy.extra`.

```yaml
privacy:
  controller: Coffey Labs LLC           # who answers for the data
  contact: privacy@example.org          # an address to write to
  updated: 10 October 2026              # when the notice last changed
  extra:
    - { what: Video embeds, who: YouTube, why: Videos on some pages, basis: Your consent }
```

The starter's `content/privacy.md` uses the `privacy` layout, which shows the
contact (protected from Cloudflare's address rewriting) and the list as a
table. Because the list comes from the configuration, the notice can't fall
behind what the site does.
