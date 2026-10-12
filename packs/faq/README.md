# Questions and answers pack

A "Questions and answers" section for any page of a HotDog CMS site. Each
question opens to show its answer and closes again, with HTML's own
`<details>` element: no script, keyboard and screen-reader friendly, and the
answers are in the page for search engines.

    hotdog-cms pack add faq

The questions can be written in the section, or kept in a file in `data/` and
shared between pages (`from: faq` reads `data/faq.yaml`).
