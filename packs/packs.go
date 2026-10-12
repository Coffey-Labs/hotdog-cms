// Package packs holds the packs that come with HotDog CMS, so
// `hotdog-cms pack add calendar` works without a download. Each is an
// ordinary pack (see docs/packs.md), MIT licensed like the starter.
package packs

import "embed"

//go:embed all:calendar all:docs all:downloads all:faq all:gallery all:location-and-hours all:menu-and-prices all:portfolio all:share-links all:team all:testimonials
var FS embed.FS
