// Package packs holds the packs that come with HotDog CMS, so
// `hotdog-cms pack add calendar` works without a download. Each is an
// ordinary pack (see docs/packs.md), MIT licensed like the starter.
package packs

import "embed"

//go:embed all:author-box all:breadcrumbs all:calendar all:changelog all:coming-soon all:contact-forms all:directory all:docs all:downloads all:faq all:gallery all:glossary all:jobs all:link-in-bio all:location-and-hours all:logo-wall all:menu-and-prices all:payment-links all:podcast all:portfolio all:recipes all:related-posts all:reviews all:series all:share-links all:shop all:status-page all:table-of-contents all:tables all:team all:testimonials all:timeline all:videos
var FS embed.FS
