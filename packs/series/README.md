# Series pack

Articles written in parts. Each part shows where it falls ("Part 2 of 5 in
Sourdough basics"), lists every part with the one you're reading marked, and
links to the previous and next part.

    hotdog-cms pack add series

Give the articles the same `series:` in their front matter (and `part:`
numbers if they shouldn't simply go oldest first), and add
`{{ template "series" . }}` to `layouts/article.html`. Add `series` to
`taxonomies:` in `site.yaml` for a page per series, which the box links to.
