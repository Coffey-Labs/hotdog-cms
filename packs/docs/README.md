# Documentation pack

A documentation section for a HotDog CMS site, at /docs/: a sidebar listing
every part and the pages of the part being read, a page for each part with
its pages in order, a front page with a card for each part, and previous and
next links at the foot of every page.

    hotdog-cms pack add docs

The reading order lives in one file, `data/docs-nav.yaml`, so moving a page
is one line. The docs collection gets a search index (`/docs/search.json`);
a pack can't carry scripts, so adding a search box is up to the site.
hotdogcms.org/docs is built with this pack.
