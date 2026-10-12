# Team pack

The people behind a HotDog CMS site: a page for each person with their photo,
role, a few lines about them and their links, a page listing everyone at
/people/, and a "Team" section for any page.

    hotdog-cms pack add team

People are Markdown files in `content/people/`, ordered by `weight`, with
`role`, `photo` and `photo_alt` (what the photo shows), and optionally `email`
and `links`. A person without a photo gets their initial in a circle. Only publish photos and
details people agreed to share.
