# Sections and looks, for theme builders

(To share sections between sites, put them in a pack: see
[packs.md](packs.md).)

A theme decides what the editor offers people who'd rather not touch
templates: the **sections** a page can be built from, and the **look** they
can change. Both come from plain files in the site, so adding a section type
or a color is a matter of editing a template, not the editor.

## Sections

A section is a template in `sections/`. A page stacks sections in its front
matter, and the editor shows them as cards: added from the library,
reordered, duplicated, removed, each with a form for its fields.

```yaml
# content/index.md
sections:
  - hero:
      heading: Your site, in plain files
      buttons:
        - { text: Read the articles, href: /articles/, primary: true }
  - latest:
      collection: articles
```

### Declaring fields

Start a section template with a comment that says what it takes:

```
{{/* section
label: Hero
about: The big opening at the top of a page, with buttons.
fields:
  - { name: eyebrow, type: text, help: A short line above the heading. }
  - { name: heading, type: text, required: true }
  - { name: text, type: markdown }
  - name: buttons
    type: list
    fields:
      - { name: text, type: text, required: true }
      - { name: href, type: link, label: Goes to }
      - { name: primary, type: bool, label: Main button }
*/}}
<section class="hero">…
```

Each field has a `name`, and optionally a `type` (default `text`), a `label`,
`help`, `required` and a `default`. The types are:

| Type | The form shows |
|---|---|
| `text` | one line |
| `textarea` | several lines of plain text |
| `markdown` | Markdown (render it with `markdownify`) |
| `link` | an address on the site or elsewhere |
| `image` | a picture, picked from the media library |
| `bool` | a checkbox |
| `number` | a number |
| `select` | one of `options: [a, b, c]` |
| `list` | items with their own `fields`, or lines of text without them |
| `group` | a set of `fields` |
| `data` | a file in `data/` |
| `collection` | a collection, such as `articles` |

If the comment can't be read, the editor says why, in the section's card.

### Without a declaration

A template without the comment still gets a form: the editor reads the
fields it uses. It understands:
- `.Data.heading` and `$.Data.heading`;
- a variable holding `.Data` or one of its fields;
- `with` over a field (a group) and `range` over one (a list);
- `markdownify` (Markdown);
- `default 3 .Data.count` (a number with a default);
- a field only ever tested with `if` (a checkbox);
- names that say what they hold (`href`, `image`);
- partials handed `.Data` with `template`, followed into.

That's how the ported sites' sections work in the editor with no changes.
Declaring the fields still gives better labels, help and types.

## The look

`look.yaml` at the top of the site lists what can be changed from the
editor's Look panel, and the limits:

```yaml
tokens:
  - { name: accent, label: Accent, type: color, default: "#ba5019", dark: "#f2a65a" }
  - { name: bg, label: Background, type: color, default: "#fbfaf7", dark: "#12161c" }
  - name: font
    type: font
    default: inter
    options:
      inter: '"Inter", system-ui, sans-serif'
      serif: 'Charter, Georgia, serif'
  - name: radius
    label: Corners
    type: select
    default: round
    options: { sharp: 4px, round: 14px }
  - { name: logo, label: Logo, type: image }

contrast:
  - [text, bg]       # pairs that must stay readable
  - [accent, bg]
```

- **color:** a `default`, and a `dark` default if dark mode differs.
- **font and select:** named `options`, each standing for a CSS value.
- **image:** an address, read in templates with `look`.

A site's choices go in `site.yaml`:

```yaml
look:
  accent: "#0a7d55"                            # dark mode keeps the theme's
  text: { light: "#1b1b1b", dark: "#f0f0f0" }
  font: serif
  logo: /media/2026/10/logo.png
```

The build turns the choices into a small stylesheet of custom properties
(`--accent`, `--font`, …), with only what the site changes. Every color, font
and select token becomes `--<name>`, so the theme's CSS uses `var(--accent)`
and so on. Two template functions use it:

```
{{ with lookCSS }}<link rel="stylesheet" href="{{ . }}">{{ end }}   after the theme's stylesheet
{{ with look "logo" }}<img src="{{ . }}" alt="">{{ end }}           a token's value or its default
```

- **No inline styles:** the stylesheet is a fingerprinted file, so a strict
  Content-Security-Policy still holds.
- **Dark-mode values** are written for the system setting (unless the reader
  picked light) and for `data-theme="dark"`, the convention the starter
  follows.
- **Invalid choices stop the build:** a color that isn't one, or an option
  the theme doesn't list, with the reason in words.
- **Contrast:** every `contrast` pair is checked in light and dark mode.
  `hotdog-cms check` refuses to publish one under 4.5:1 (`look-contrast`),
  and the Look panel shows each ratio as colors are picked.

## Long lists

A collection's page and a tag's page list everything in them. Set
`paginate:` in `site.yaml` to split long lists into pages of that many:
`/articles/`, `/articles/page/2/`, and so on. A collection can set its own
under `collections:`.

Every list page has a `.Paginator`, split or not, so a template ranges over
`.Paginator.Items` either way:

```
<ul class="cards">
  {{ range .Paginator.Items }}{{ template "card" . }}{{ end }}
</ul>
{{ template "pagination" . }}
```

| Field | What it is |
|---|---|
| `.Number`, `.TotalPages`, `.TotalItems` | Where this page is, and how much there is |
| `.Items` | The pages listed on this one |
| `.Prev`, `.Next` | The neighbouring pages (newer, older), or nothing at either end |
| `.First`, `.All` | Page 1, and every page in order |
| `.Links 2` | A row of numbered links: the first, the last, two either side of this one, and gaps (`.Gap`) for the rest; `.Current` marks this page |

The starter's `partials/pagination.html` draws newer and older links and the
numbers, marks the current page for screen readers, and its head adds
`rel="prev"` and `rel="next"` and "page 2" to later pages' titles. Later
pages keep their own canonical address. The duplicate-title check leaves
them out.

## On this page

`headings .Content` lists a page's h2 and h3 headings, in order, each with
its `.Level` (2 or 3), `.ID` (the anchor) and `.Text`. A contents list for
long pages is a few lines:

```
{{ $h := headings .Content }}
{{ if gt (len $h) 1 }}
<nav aria-label="On this page"><ul>
  {{ range $h }}<li class="h{{ .Level }}"><a href="#{{ .ID }}">{{ .Text }}</a></li>{{ end }}
</ul></nav>
{{ end }}
```

The table-of-contents pack puts one at the top of a page; the docs pack
draws one beside each page. Search indexes carry the headings too (`h`, as
`[anchor, text]` pairs), so a search box can send someone to the section a
word is in rather than the top of the page.

## Navigation trees

`nav "docs-nav" .` reads a navigation tree from `data/docs-nav.yaml` and
places the page in it. Each entry is a page's source file, a heading over
more entries (with a page of its own or not), or a link elsewhere:

```
- docs/_index.md
- title: Run it
  page: docs/run/_index.md
  children:
    - docs/run/sizing.md
    - title: Compliance
      children: [docs/run/audit-log.md, docs/run/legal-holds.md]
- title: Back to the main site
  url: https://example.org/
```

What comes back has `.Items` (each with `.Title`, `.URL`, `.Page`,
`.Children`, `.External`, `.Here` for the page being shown and `.Open` for
the headings above it), `.Here`, `.Trail` (those headings, from the top),
and `.Prev` and `.Next` in reading order: every entry with a page, depth
first, a heading's page before what's under it. An entry whose page isn't
in the build, a draft say, is left out. The docs pack draws its sidebar,
breadcrumb and previous and next links from it.

A collection's pages live under `/<collection>/`; `path:` in its
`collections:` entry puts them elsewhere. `path: /` puts a docs site's pages
at the root of its own host. Its search index and feed stay at
`/<collection>/`.

## What the starter offers

The starter's sections are hero (with an optional picture beside the text),
cards, latest, text, closing band and video. Its Look panel sets the accent,
background, text, hero and closing-band colors (each with a dark-mode
value), the font (Inter, system, serif, rounded or Recursive, which ships
with the starter under the SIL Open Font License), corners and the logo.

## The footer

`site.yaml`'s `copyright:` names who holds the copyright the footer shows
(the site's name if it's left out), `copyright_url:` links that name, and `credit: true` puts "Built with
HotDog CMS" in the middle of the footer, linking to hotdogcms.org. New sites
start with the credit on; set `credit: false` to take it off.
