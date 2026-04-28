//go:build desktop

// Package assets provides embedded web assets for desktop builds.
package assets

import "embed"

//go:embed web
var FS embed.FS
