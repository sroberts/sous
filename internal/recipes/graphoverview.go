package recipes

import (
	"context"
	"database/sql"

	"github.com/sroberts/sous/internal/identity"
)

// GraphOverview is the household's step graphs at a glance, for the app:
// what Sous can do, how many recipes are in each state, and each graph's
// status and review count.
type GraphOverview struct {
	Upgrades GraphUpgrades    `json:"upgrades"`
	Counts   map[string]int   `json:"counts" jsonschema:"recipes with steps by graph status: none, draft, needs_review, verified, stale"`
	Items    []GraphStatusRow `json:"items" jsonschema:"every recipe with a step graph, by title"`
}

// GraphUpgrades says what this Sous can do with its models.
type GraphUpgrades struct {
	Labels     bool `json:"labels" jsonschema:"a decision model is set up, so drafts can be labelled"`
	GraphModel bool `json:"graph_model" jsonschema:"Sous can also draft a graph's structure itself (upgrade_recipe and background jobs)"`
	AutoVerify bool `json:"auto_verify"`
}

// GraphStatusRow is one recipe's graph.
type GraphStatusRow struct {
	RecipeID    string `json:"recipe_id"`
	Title       string `json:"title"`
	Status      string `json:"status" jsonschema:"draft, needs_review, verified or stale"`
	Version     int    `json:"version" jsonschema:"the graph's version"`
	ReviewCount int    `json:"review_count"`
}

// GraphOverview lists every live recipe's graph status. Staleness and
// review counts come from SQL, so it stays quick across a whole library.
func (s *Service) GraphOverview(ctx context.Context, c identity.Caller) (GraphOverview, error) {
	out := GraphOverview{Counts: map[string]int{"none": 0, GraphDraft: 0, GraphNeedsReview: 0, GraphVerified: 0, GraphStale: 0},
		Items: []GraphStatusRow{}}
	if s.up != nil {
		out.Upgrades = GraphUpgrades{Labels: s.up.Dec != nil, GraphModel: s.up.Gen != nil, AutoVerify: s.up.AutoVerify}
	}
	err := s.st.Read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT r.id, r.title, g.status, g.version,
			EXISTS (SELECT 1 FROM graph_nodes n LEFT JOIN recipe_steps st ON st.recipe_id = n.recipe_id AND st.position = n.step - 1
				WHERE n.recipe_id = g.recipe_id AND n.recipe_version = g.recipe_version
				AND (st.text IS NULL OR substr(st.text, n.span_start + 1, n.span_end - n.span_start) IS NOT n.quote)),
			(SELECT count(*) FROM graph_labels l JOIN graph_nodes n ON n.recipe_id = l.recipe_id AND n.recipe_version = l.recipe_version AND n.id = l.node_id
				WHERE l.recipe_id = g.recipe_id AND l.recipe_version = g.recipe_version AND `+reviewWhere+`)
			FROM recipes r JOIN recipe_graphs g ON g.recipe_id = r.id AND g.deleted_at IS NULL
			WHERE r.deleted_at IS NULL ORDER BY r.title COLLATE NOCASE, r.id`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var row GraphStatusRow
			var stale bool
			if err := rows.Scan(&row.RecipeID, &row.Title, &row.Status, &row.Version, &stale, &row.ReviewCount); err != nil {
				rows.Close()
				return err
			}
			if stale {
				row.Status = GraphStale
			}
			out.Counts[row.Status]++
			out.Items = append(out.Items, row)
		}
		if err := closeRows(rows); err != nil {
			return err
		}
		var none int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM recipes r WHERE r.deleted_at IS NULL
			AND EXISTS (SELECT 1 FROM recipe_steps WHERE recipe_id = r.id)
			AND NOT EXISTS (SELECT 1 FROM recipe_graphs g WHERE g.recipe_id = r.id AND g.deleted_at IS NULL)`).Scan(&none); err != nil {
			return err
		}
		out.Counts["none"] = none
		return nil
	})
	return out, err
}
