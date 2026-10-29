// Package web embeds the built single-page app.
//
// `task build` copies web/build/client into dist/. Only dist/.gitkeep is committed, so a
// plain `go build` still compiles and the server answers with a notice instead of the app.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist returns the built app, rooted at the directory that holds index.html.
func Dist() (fs.FS, error) {
	return fs.Sub(dist, "dist")
}
