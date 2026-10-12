# Related posts pack

"Keep reading" for a HotDog CMS site: at the end of an article, up to three
articles that share the most tags with it. It's worked out when the site is
built, from the tags you already give articles, so there's nothing to keep
up to date and nothing runs in visitors' browsers.

    hotdog-cms pack add related-posts

Add `{{ template "related-posts" . }}` to `layouts/article.html`, after the
content.
