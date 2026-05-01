//go:build !desktop

// Package assets provides embedded web assets for server (CLI) builds.
package assets

import "embed"

//go:embed web
var FS embed.FS
