# Share links pack

Plain "Share" links for a HotDog CMS site: Mastodon, Bluesky, LinkedIn and
email. Each is an ordinary link to that network's own share page, filled in
with the page's title and address. Unlike share buttons from those sites,
nothing loads from them until someone chooses a link, so nobody is tracked
and there's nothing to ask consent for.

    hotdog-cms pack add share-links

Add `{{ template "share-links" . }}` to a layout (such as
`layouts/article.html`, after the content), or use the "Share links" section
on any page. Set `params: { share_mastodon: your.server }` in `site.yaml` to
send the Mastodon link to your own server (default `mastodon.social`).
