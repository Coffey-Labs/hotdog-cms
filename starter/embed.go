// Package starter holds the site that `cl-cms new` writes out.
package starter

import "embed"

// Files is the starter site, under site/.
//
//go:embed all:site
var Files embed.FS
