# Changelog pack

Release notes for a HotDog CMS site: a page per release, every release on
one page, a feed people can follow, and a "What's new" section.

    hotdog-cms pack add changelog

Each release is a page in `content/changelog/`, with its version as the
title, a date, a one-line summary and the notes in Markdown. Mark one
`security: true` or `breaking: true` and it says so wherever it's listed.

- `/changelog/`: every release, newest first, with its notes, each at its own
  anchor (`/changelog/#1-4-0`).
- `/changelog/1-4-0/`: one release, with links to the older and newer ones.
- `/changelog/feed.xml`: an Atom feed of new releases.
- **What's new** section: the latest few releases on any page.
- `{{ template "latest-release" . }}`: a "Latest 1.4.0" link for a hero or a
  footer.

Notes grouped under `### Added`, `### Changed`, `### Fixed` and so on (the
headings keepachangelog.com uses) show as labels.
