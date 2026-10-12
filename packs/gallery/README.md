# Gallery pack

A "Gallery" section for a HotDog CMS site: a grid of pictures, each opening
full size with its caption, with links to the next and previous picture. It
works with no script at all (it uses the page's own `#` links), so it's
light, private and fine under a strict Content-Security-Policy.

    hotdog-cms pack add gallery

Give every picture a description (`alt`): the gallery is a list of links,
and the description is what each link says to people who can't see it.
