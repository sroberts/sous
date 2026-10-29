package recipes

import (
	"context"

	"github.com/sroberts/sous/internal/identity"
)

// Status fills SchemaVersion, Recipes (live count) and StorageBytes. The
// API layer fills Version, Auth and JWKS, which the service doesn't know.
func (s *Service) Status(ctx context.Context, c identity.Caller) (Status, error) {
	panic("TODO")
}
