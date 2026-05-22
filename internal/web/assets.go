// Package web embeds the built React SPA produced by `pnpm run build` and
// exposes it as an fs.FS rooted at the dist directory.
//
// The dist subdirectory is populated by `just build` (which runs `just
// fe-build` followed by `cp -R web/dist internal/web/dist`). Running
// `go build ./...` outside of `just build` succeeds — a committed
// `dist/.gitkeep` sentinel keeps the //go:embed directive resolvable — but
// the resulting binary will respond 404 to SPA routes because no real
// assets are present. Always go through `just build` for a usable binary.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// Dist returns the SPA static files rooted at the dist directory so callers
// can request paths like "index.html" and "assets/foo.js" without prefixing.
func Dist() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}
