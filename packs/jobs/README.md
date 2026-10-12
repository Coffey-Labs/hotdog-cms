# Jobs pack

Job openings for a HotDog CMS site: a page for each, a list at /jobs/ of
what's open, and a "Jobs" section for any page. Each opening has where it
is, what kind it is, the pay if you give it, when applications close, and
how to apply.

    hotdog-cms pack add jobs

Closed openings leave the list, and their pages say so, from the first
build after the closing date: the pipeline files `hotdog-cms ci` writes
rebuild hourly. Open ones are written as schema.org JobPosting data, which
search engines use for job listings.
