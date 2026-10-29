package recipes

import (
	"context"

	"github.com/sroberts/sous/internal/identity"
)

// List returns live recipe summaries matching q, paged by opaque cursor.
// Repeated tags AND together case-insensitively; the default sort is title
// even with q set; favorite or want_to_cook from nobody matches nothing.
// A bad sort, cursor or limit is CodeInvalid.
func (s *Service) List(ctx context.Context, c identity.Caller, q ListQuery) (Page[Summary], error) {
	panic("TODO")
}

// Trash returns deleted recipes, most recently deleted first, with
// DeletedAt set on each summary.
func (s *Service) Trash(ctx context.Context, c identity.Caller, q TrashQuery) (Page[Summary], error) {
	panic("TODO")
}
