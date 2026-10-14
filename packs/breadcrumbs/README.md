# Breadcrumbs pack

A trail above a page's title, from the home page down to the page:
**Home › Recipes › Bread › Rye**. Each step links to the page at that
address, so a visitor deep in a site can get back up, and search engines show
the trail in results instead of the bare address.

    hotdog-cms pack add breadcrumbs

Then add `{{ template "breadcrumbs" . }}` to the layouts that should show it.
The search engine data (BreadcrumbList) is written for every page once the
pack is installed. The docs pack has a trail of its own, so leave it out of
the docs layouts.
