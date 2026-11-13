//go:build embedstatic

package webui

import (
	"embed"
	"io/fs"
)

// dist holds the compiled frontend bundle staged by the build script.
//
//go:embed all:dist
var dist embed.FS

// FS returns the embedded frontend assets, or nil when they are unavailable.
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil
	}
	return sub
}
