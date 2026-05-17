// Package webembed holds the embedded React SPA.
// internal/webembed/dist is populated by `pnpm run build` in web/ (see vite outDir).
// FS is nil until a real build exists (index.html); the server may then fall back
// to os.DirFS("internal/webembed/dist") — see cmd/server/main.go.
package webembed

import (
	"embed"
	"io/fs"
)

//go:embed dist
var distFS embed.FS

// FS is the sub-filesystem rooted at dist/ (stripped of the "dist/" prefix).
// It is nil if there is no built SPA (no index.html in the embed).
var FS fs.FS

func init() {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return
	}
	entries, err := fs.ReadDir(sub, ".")
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.Name() == "index.html" {
			FS = sub
			return
		}
	}
}
