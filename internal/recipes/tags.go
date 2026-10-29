package recipes

import (
	"context"
	"database/sql"

	"github.com/sroberts/sous/internal/identity"
)

// Tags returns every tag on a live recipe with its count, by count
// descending, then name.
func (s *Service) Tags(ctx context.Context, c identity.Caller) ([]TagCount, error) {
	out := []TagCount{}
	err := s.st.Read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT t.name, count(*) AS n
			FROM recipe_tags rt JOIN tags t ON t.id = rt.tag_id JOIN recipes r ON r.id = rt.recipe_id
			WHERE r.deleted_at IS NULL
			GROUP BY t.id ORDER BY n DESC, t.name COLLATE NOCASE`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var tc TagCount
			if err := rows.Scan(&tc.Name, &tc.Count); err != nil {
				return err
			}
			out = append(out, tc)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
