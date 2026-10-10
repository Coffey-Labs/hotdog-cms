# Checks

`cl-cms check` builds the site and reads every page the way a careful
reviewer would. **Errors** fail it; **warnings** are reported. The same check
runs before anything is published: `cl-cms publish` and the pull agent
refuse a build with errors, and previews show the count for each branch.
There is no flag to skip it. A site that means to break a rule switches the
rule off in `site.yaml`, where the decision is written down and reviewed
like any other change:

```yaml
check:
  ignore: [img-alt]                         # rule ids, as printed
  allow_third_party: [challenges.cloudflare.com]
  cloudflare: true                          # served through Cloudflare's proxy
  csp: strict                               # inline scripts and styles become errors
  privacy_page: /privacy/
  forbid: ['(?i)internal\.example\.org']    # strings that must never be published
```

## Rules

| Rule | Level | What it catches |
|---|---|---|
| `tracker-before-consent` | error | Analytics or tracking that starts on page view (gtag.js, Facebook pixel, Hotjar, Clarity…), before anyone could agree. Load it from consent code instead. |
| `third-party` | warning | Scripts, frames or stylesheets from another site that isn't in `allow_third_party`. |
| `privacy-page` | error | The site loads third-party code but has no privacy notice. |
| `privacy-contact` | warning | The privacy notice has no email address to write to. |
| `inline-script`, `inline-style` | warning (error with `csp: strict`) | What a strict Content-Security-Policy would block. |
| `inline-handler` | error | `onclick=` and friends. |
| `mixed-content` | error | Scripts, frames, images or stylesheets over plain `http://`. |
| `broken-link` | error | Links and images that point at nothing in the site. |
| `forbidden-string` | error | Anything matching `forbid`, in the source or the build. |
| `cloudflare-email` | warning | With `cloudflare: true`: addresses and fediverse handles outside `email_off` markers, which Cloudflare rewrites to "[email protected]". `markdown.email_off: true` wraps them for you. |
| `missing-title` | error | An indexable page without a title. |
| `missing-description`, `description-length` | warning | No meta description, or one too long to show. |
| `missing-canonical` | warning | No canonical link. |
| `social-image`, `social-image-alt` | warning (error if the image is missing) | Link previews without a picture, or without alt text. |
| `duplicate-title`, `duplicate-description` | warning | Indexable pages that look the same to a search engine (later pages of a paginated list don't count). |
| `noindex-in-sitemap` | error | The sitemap lists a page that asks not to be indexed. |
| `robots-sitemap` | warning | robots.txt is missing, or doesn't point to the sitemap. |
| `security-txt` | warning, error if expired | `/.well-known/security.txt` missing, without `Expires:`, expired, or within 30 days of it. |
| `img-alt` | warning | Images without an `alt` attribute (`alt=""` is right for decoration). |

## The Content-Security-Policy a site needs

`cl-cms check -csp` prints the policy the built pages need: their own origin,
the hosts they load from, and `'unsafe-inline'` only if a page still has
inline code. A site with no inline scripts or styles gets a strict policy. It
can't see what scripts fetch at run time, so add those hosts to
`connect-src`. The `container` publish target can send it for you (`csp: true`).
