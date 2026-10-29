package recipes

import (
	"context"

	"github.com/sroberts/sous/internal/identity"
)

// Changes returns the change log, newest first, filtered by q.
func (s *Service) Changes(ctx context.Context, c identity.Caller, q ChangeQuery) (ChangePage, error) {
	panic("TODO")
}
