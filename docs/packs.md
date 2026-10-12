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
  - **calendar**: events, an upcoming-events section and a feed people
    subscribe to (see [events.md](events.md));
  - **docs**: a documentation section with a sidebar, part pages and
    previous/next links, in the order `data/docs-nav.yaml` gives;
  - **downloads**: files to download, with their type and size;
  - **faq**: questions that open to show their answers;
  - **gallery**: a grid of pictures that open full size, with no script;
  - **location-and-hours**: address, map link, phone, email and opening
    hours, from `data/location.yaml`, as a section or a partial;
  - **menu-and-prices**: a price list in groups, and plans side by side;
  - **portfolio**: projects with a cover, details and pictures, a list page
    and a section;
  - **share-links**: plain share links for Mastodon, Bluesky, LinkedIn and
    email, with no tracking;
  - **team**: a page per person, a list of everyone and a section;
  - **testimonials**: quotes with a name and a role.
- `-site` picks the site folder (default: the current one).

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
