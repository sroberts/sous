// Package recipes is the domain layer: people, recipes, tags, images and
// the change log, plus the ingredient parser. REST and MCP both call it, so
// they cannot drift apart.
package recipes

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"time"
	"uuid"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/storage"
)

// Error codes the service adds to those in errors.go.
const (
	CodeUnknownImage  = "unknown_image"  // 400: a recipe names an image hash that was never uploaded
	CodeInvalidCursor = "invalid_cursor" // 400: cursor is not one this list issued
)

// timeFormat is fixed-width so stored times sort as strings.
const timeFormat = "2006-01-02T15:04:05.000Z"

// Service is the domain layer over a Store. Every write runs in one
// Store.Write transaction that also appends to the change log, and every
// write from nobody returns CodeWhoAreYou.
type Service struct {
	st  *storage.Store
	log *slog.Logger
	now func() time.Time
}

// Service is the people directory the identity resolver consults.
var _ identity.Directory = (*Service)(nil)

// New returns a Service over st.
func New(st *storage.Store, log *slog.Logger) *Service {
	return &Service{st: st, log: log, now: time.Now}
}

func (s *Service) stamp() string {
	return s.now().UTC().Format(timeFormat)
}

// author returns the caller's username, or CodeWhoAreYou for nobody.
func author(c identity.Caller) (string, error) {
	if c.Person == nil || c.Person.Username == "" {
		return "", Errorf(CodeWhoAreYou, "choose who is cooking before making changes")
	}
	return c.Person.Username, nil
}

func viaOf(c identity.Caller) string {
	switch c.Via {
	case identity.ViaUI, identity.ViaMCP, identity.ViaSystem:
		return string(c.Via)
	}
	return string(identity.ViaAPI)
}

// logChange appends to the change log inside the caller's transaction, so
// the entry commits or rolls back with the change it describes.
func (s *Service) logChange(ctx context.Context, tx *sql.Tx, c identity.Caller, at, entity, entityID, action, summary string) error {
	var person *string
	if u := c.Username(); u != "" {
		person = &u
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO changes (id, at, person_id, via, entity, entity_id, action, summary)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewV7().String(), at, person, viaOf(c), entity, entityID, action, summary)
	return err
}

// placeholders returns "?, ?, …" for n arguments.
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}

func pageLimit(n int) (int, error) {
	switch {
	case n < 0:
		return 0, Errorf(CodeInvalid, "limit must be between 1 and 200")
	case n == 0:
		return 50, nil
	case n > 200:
		return 200, nil
	}
	return n, nil
}
