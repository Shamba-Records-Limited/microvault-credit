// Package static holds the admin's compiled stylesheet and its handful of scripts.
package static

import "embed"

// FS carries the assets served under /static.
//
//go:embed *.css *.js
var FS embed.FS
