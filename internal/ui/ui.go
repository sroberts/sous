// Package ui serves the built web UI, embedded from dist/.
package ui

import (
	"embed"
	"log/slog"
	"net/http"
)

// Embedding * rather than dist keeps go build working before the UI has
// been built; only the dist subtree is served.
//
//go:embed *
var files embed.FS

// Handler serves dist: /assets/* with an immutable cache header, a missing
// path with a file extension as 404, and anything else as index.html with
// no-cache for client-side routing. Without dist it serves a "UI not built"
// page.
func Handler(log *slog.Logger) http.Handler {
	panic("TODO")
}
