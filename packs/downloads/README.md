# Downloads pack

A "Downloads" section for a HotDog CMS site: files people can download, such
as menus, forms, price lists or reports, each with a title, a line about it,
the file's type and its size.

    hotdog-cms pack add downloads

Put the files in `static/` (for example `static/files/menu.pdf`) and list
them in the section, or in a file in `data/`. The type and size are read
from the file when the site is built. A missing file is a broken link, which
`hotdog-cms check` reports before anything is published.
