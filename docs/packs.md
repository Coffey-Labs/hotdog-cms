# Packs

A pack is how sections, templates and styles are shared between sites. It's
a folder or a git repository of plain files. Installing it copies them into
the site, where they're the site's own: reviewed, versioned, changed and
published like everything else.

A pack carries **no code**. That's the point. WordPress plugins run with the
site's full rights, and that's where most of its security problems came
from. A pack's templates run through the same escaping, checks and
Content-Security-Policy as the site's own.

## Using one

    hotdog-cms pack add calendar                                       # one that comes with HotDog CMS
    hotdog-cms pack add ../faq-pack                                    # a folder
    hotdog-cms pack add https://git.example.org/packs/faq.git@v1.2.0   # a repository, at a tag
    hotdog-cms pack list
    hotdog-cms pack remove faq

- `add` installs the files and records the pack in `site.yaml`. Commit both,
  ideally through a review.
- Running `add` again with a newer version upgrades it. Files the new version
  no longer has are removed.
- A file already in the site that the pack didn't put there is never
  overwritten, unless you add `-force`. Two packs can't install the same
  file.
- `remove` takes the pack's files out and forgets it.
- `list` also names the packs that come with HotDog CMS:
  - **author-box**: who wrote an article, at its end, with a photo, bio
    and links, from `data/authors.yaml`;
  - **breadcrumbs**: a Home › Articles › This page trail above a page's
    title, and the same trail for search engines;
  - **calendar**: events, an upcoming-events section and a feed people
    subscribe to (see [events.md](events.md));
  - **changelog**: release notes, a page per release and all of them on
    one page, a feed to follow and a "What's new" section;
  - **coming-soon**: a holding page for before launch or during
    maintenance, with when you'll open and a sign-up form;
  - **contact-forms**: ready-made forms for a quote, a booking and a
    mailing list, and a section that puts any form beside your email,
    phone and address;
  - **directory**: a listing of businesses, members or places, grouped
    by category, with a page each;
  - **docs**: a documentation section with a sidebar, part pages and
    previous/next links, in the order `data/docs-nav.yaml` gives;
  - **downloads**: files to download, with their type and size;
  - **faq**: questions that open to show their answers;
  - **gallery**: a grid of pictures that open full size, with no script;
  - **glossary**: terms and what they mean, A to Z, each with its own
    link;
  - **jobs**: openings with closing dates; closed ones leave the list by
    themselves;
  - **link-in-bio**: one page of big buttons, for the link in a social
    profile;
  - **location-and-hours**: address, map link, phone, email and opening
    hours, from `data/location.yaml`, as a section or a partial;
  - **logo-wall**: a grid of logos, for customers, sponsors or "as seen
    in";
  - **menu-and-prices**: a price list in groups, and plans side by side;
  - **payment-links**: buy, book and donate buttons that open your payment
    provider's own checkout (Stripe, PayPal, Square, Ko-fi, Liberapay…);
  - **podcast**: episodes with a player, and the feed podcast apps
    subscribe to (`feed_format: podcast`);
  - **portfolio**: projects with a cover, details and pictures, a list page
    and a section;
  - **recipes**: ingredients, steps, times and servings;
  - **related-posts**: at the end of an article, the ones sharing the most
    tags with it;
  - **reviews**: star ratings from customers, with the average, as a
    section or under a product;
  - **series**: articles in parts, with "Part 2 of 5", every part listed
    and links to the previous and next part;
  - **share-links**: plain share links for Mastodon, Bluesky, LinkedIn and
    email, with no tracking;
  - **shop**: a product catalog with a page per product, each Buy button
    opening your payment provider's checkout (no cart);
  - **status-page**: whether your services are working, open and past
    incidents, and a feed;
  - **table-of-contents**: a long page's headings, linked, at its top;
  - **tables**: tables from a spreadsheet saved as CSV (or YAML), with a
    caption and row headings, that scroll on phones;
  - **team**: a page per person, a list of everyone and a section;
  - **testimonials**: quotes with a name and a role;
  - **timeline**: dates down a line, with what happened at each;
  - **videos**: a video library from your own files, with a page per
    video, a feed and a "Latest videos" section.
- `-site` picks the site folder (default: the current one).

**Search engines.** The faq, location-and-hours, team, recipes, jobs and shop
packs are also written as schema.org data (FAQPage, LocalBusiness, Person,
Recipe, JobPosting, Product), and so are the breadcrumbs pack's trail
(BreadcrumbList, on every page once the pack is installed), a glossary
section's terms (DefinedTermSet), a video page (VideoObject) and a product's
reviews (AggregateRating), which search engines use for richer results. A pack can't carry the
`<script type="application/ld+json">` that holds it, so HotDog CMS
builds it (`pageData` in templates) and the starter's `partials/schema.html`
writes it. Sites started before HotDog CMS 0.2 can add the same two lines to
theirs:

    {{ range pageData . }}<script type="application/ld+json">{{ . }}</script>
    {{ end }}

```yaml
# site.yaml, written by hotdog-cms pack
packs:
  - name: faq
    version: 1.2.0
    source: https://git.example.org/packs/faq.git@v1.2.0
    commit: 4f2c9e1…
    files:
      - assets/packs/faq/faq.css
      - sections/faq.html
```

In the editor, a pack's sections appear in the section library like any
other, marked with the pack they came from.

## Making one

```
faq-pack/
  pack.yaml
  sections/faq.html
  assets/faq.css
  README.md
  LICENSE
```

```yaml
# pack.yaml
name: faq                # lowercase letters, digits and dashes
version: 1.0.0
title: FAQ
description: Questions and answers that open and close.
license: MIT
author: Example Studio
# Settings its layouts expect, added to site.yaml's collections: on install
# unless the site already configures that collection.
collections:
  faq:
    layout: faq-page
# Shown after installing: what to do next.
notes: |
  Add questions as content/faq/<name>.md.
```

Collection settings are data, not code, and are checked against what
`site.yaml` accepts. Removing a pack leaves them, since they may have been
changed and do nothing without the pack's layouts.

What a pack can hold:
- **Templates:** `.html` files directly in `sections/`, `partials/` or
  `layouts/`, installed at the same place in the site. Sections declare their
  fields the usual way (see [themes.md](themes.md)).
- **Forms:** `.yaml` files directly in `forms/` (see [forms.md](forms.md)).
  A form is copied in once and is the site's own from then on, since the
  site sets where it sends: upgrading or removing the pack leaves it alone,
  and a site's own form of the same name is kept. A pack's form must send
  to a placeholder address such as `you@example.org`, never one the pack's
  author chose, and can't turn on Turnstile, which loads another site's
  script. `hotdog-cms check` warns until the site sets its own address.
- **Assets:** stylesheets, pictures (PNG, JPEG, WebP, GIF, AVIF) and fonts
  in `assets/`. They're installed under `assets/packs/<name>/`, so two packs
  never collide. Refer to them that way:
  `{{ asset "packs/faq/faq.css" }}`.
- `README`, `LICENSE` and `CHANGELOG` files stay in the pack.

What a pack can't hold, and `hotdog-cms pack check` says so:
- JavaScript;
- SVG, which can carry scripts (use PNG or WebP);
- a template with a `<script>`, an event handler (`onclick=` and the
  like), a `javascript:` link, or an `<iframe>`, `<object>` or `<embed>`;
- symbolic links, or anything outside the folders above.

    hotdog-cms pack check ./faq-pack   # what it would install, or what's wrong

## What isn't a pack

A step that has to *run* something, such as a search index or an image
pipeline, isn't a pack. It's an operator's `before` or `after` step on a
publish target (see [publishing.md](publishing.md)). Those are trusted,
because the operator chose them. They live only in the operator's targets
file, never in `site.yaml`.
