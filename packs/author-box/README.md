# Author box pack

Who wrote an article, at its end: a photo (or their initial), their name and
role, a short bio and their links, from `data/authors.yaml`.

    hotdog-cms pack add author-box

Add `{{ template "author-box" . }}` to `layouts/article.html` after
`{{ .Content }}`. An article shows the author its front matter names
(`author: sam`, or `authors: [sam, ana]`); without one, the author whose name
is the site's `author:`, so a site with one writer needs no front matter.
Authors with a `page:` link to it, so the box pairs well with the team pack.
