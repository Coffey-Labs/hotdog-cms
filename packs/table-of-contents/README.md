# Table of contents pack

A table of contents for long pages on a HotDog CMS site: the page's headings
(## and ###), each linking to its part of the page, in a box at the top that
folds away. It's made when the site is built, from the headings themselves,
so it's always right.

    hotdog-cms pack add table-of-contents

Add `{{ template "table-of-contents" . }}` before the content in a layout.
Pages with fewer than three headings don't show one; `toc: false` in a
page's front matter leaves it out too.
