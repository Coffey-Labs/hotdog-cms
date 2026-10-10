---
title: About
summary: What this site is and how it's made.
---

This page is `content/about.md`. Its address, `/about/`, comes from its file name.

## Writing a page

Every page starts with front matter between two `---` lines, then Markdown:

```markdown
---
title: About
summary: One line for search results, cards and link previews.
---

The page itself, in Markdown.
```

## Pages made of sections

The front page has no Markdown body. It's a stack of **sections** listed in its
front matter: a hero, a set of cards, the latest articles and a closing band.
Each section is one file in `sections/`, and its fields are what you write under
its name.
