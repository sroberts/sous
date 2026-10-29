// Package ui serves the built web UI, embedded from dist/.
package ui

import (
	"bytes"
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"
)

// Embedding * rather than dist keeps go build working before the UI has
// been built; only the dist subtree is served.
//
//go:embed *
var files embed.FS

const notBuiltPage = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Sous</title></head>
<body><h1>UI not built</h1>
<p>Build it, then rebuild the binary: <code>cd web &amp;&amp; npm ci &amp;&amp; npm run build</code></p>
<p>The REST API under <code>/v1</code> and MCP under <code>/mcp</code> work without it.</p>
</body></html>
`

// Handler serves dist: /assets/* with an immutable cache header, a missing
// path with a file extension as 404, and anything else as index.html with
// no-cache for client-side routing. Without dist it serves a "UI not built"
// page.
func Handler(log *slog.Logger) http.Handler {
	dist, err := fs.Sub(files, "dist")
	if err != nil {
		panic(err) // fs.Sub fails only on an invalid path
	}
	return newHandler(dist, log)
}

func newHandler(dist fs.FS, log *slog.Logger) http.Handler {
	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		log.Warn("ui not built", "hint", "cd web && npm ci && npm run build")
		index = []byte(notBuiltPage)
	}
	files := http.FileServerFS(dist)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		// index.html goes through the SPA branch: FileServer would redirect it to /.
		if name != "" && name != "index.html" {
			if fi, err := fs.Stat(dist, name); err == nil && !fi.IsDir() {
				if strings.HasPrefix(name, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
			if path.Ext(name) != "" {
				http.NotFound(w, r)
				return
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(index))
	})
}
