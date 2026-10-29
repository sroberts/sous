// Package recipes is the domain layer: people, recipes, tags, images and
// the change log, plus the ingredient parser. REST and MCP both call it, so
// they cannot drift apart.
package recipes

import (
	"log/slog"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/storage"
)

// Service is the domain layer over a Store. Every write runs in one
// Store.Write transaction that also appends to the change log, and every
// write from nobody returns CodeWhoAreYou.
type Service struct {
	st  *storage.Store
	log *slog.Logger
}

// Service is the people directory the identity resolver consults.
var _ identity.Directory = (*Service)(nil)

// New returns a Service over st.
func New(st *storage.Store, log *slog.Logger) *Service {
	return &Service{st: st, log: log}
}
