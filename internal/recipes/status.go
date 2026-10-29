package recipes

import (
	"context"
	"database/sql"

	"github.com/sroberts/sous/internal/identity"
)

// Status fills SchemaVersion, Recipes (live count) and StorageBytes. The
// API layer fills Version, Auth and JWKS, which the service doesn't know.
func (s *Service) Status(ctx context.Context, c identity.Caller) (Status, error) {
	st := Status{SchemaVersion: s.st.SchemaVersion()}
	err := s.st.Read(ctx, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT count(*) FROM recipes WHERE deleted_at IS NULL`).Scan(&st.Recipes)
	})
	if err != nil {
		return Status{}, err
	}
	if st.StorageBytes, err = s.st.SizeBytes(); err != nil {
		return Status{}, err
	}
	return st, nil
}
