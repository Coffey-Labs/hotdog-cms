# Status page pack

A status page: whether each of your services is working, an overall line
that follows from them, the incidents still open, and the past ones, with a
page per incident and a feed people can follow.

    hotdog-cms pack add status-page

Components and how they're doing live in `data/status.yaml`; incidents are
pages in `content/incidents/`, updated as they go. The page changes when
the site is published, so publish after each update. Keep it on a different
host from the services it reports on.
