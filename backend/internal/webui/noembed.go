//go:build !embedstatic

package webui

import "io/fs"

// FS returns nil when the binary is built without embedded frontend assets.
func FS() fs.FS {
	return nil
}
