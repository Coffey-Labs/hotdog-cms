# Podcast pack

A podcast on a HotDog CMS site: a page for each episode with a player and
show notes, a page at /episodes/ listing them, an "Episodes" section for any
page, and the feed podcast apps subscribe to (RSS with the iTunes fields
Apple Podcasts, Spotify and the others read).

    hotdog-cms pack add podcast

Audio files live in `static/`, so they're published with the site; for a big
back catalogue, `audio:` can be a full address on a file host instead. The
player is the browser's own `<audio>`, with no script. Give each episode
show notes, and a transcript if you can.
