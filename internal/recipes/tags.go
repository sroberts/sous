package recipes

import (
	"context"

	"github.com/sroberts/sous/internal/identity"
)

// Tags returns every tag on a live recipe with its count, by count
// descending.
func (s *Service) Tags(ctx context.Context, c identity.Caller) ([]TagCount, error) {
	panic("TODO")
}
