package recipes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/sroberts/sous/internal/identity"
)

// RecipeGraph returns a recipe's step graph, what the checks find in it
// against the recipe and kitchen as they are now, and its step timing.
func (s *Service) RecipeGraph(ctx context.Context, c identity.Caller, recipeID string) (GraphView, error) {
	var v GraphView
	err := s.st.Read(ctx, func(tx *sql.Tx) error {
		var err error
		v, err = graphView(ctx, tx, recipeID, c.Username())
		return err
	})
	return v, err
}

// SetRecipeGraph saves a step graph for a recipe at recipeVersion, its
// current version, replacing the graph at version (0 when it has none).
// A graph saved as verified must pass the checks; saving it marks every
// label reviewed by the caller. Labels whose value is unchanged from the
// graph before keep their review.
func (s *Service) SetRecipeGraph(ctx context.Context, c identity.Caller, recipeID string, recipeVersion, version int, in GraphInput) (GraphView, error) {
	who, err := author(c)
	if err != nil {
		return GraphView{}, err
	}
	var out GraphView
	err = s.st.Write(ctx, func(tx *sql.Tx) error {
		r, err := loadRecipe(ctx, tx, recipeID, who)
		if err != nil {
			return err
		}
		if r.DeletedAt != nil {
			return Errorf(CodeDeleted, "this recipe is in the trash; restore it first")
		}
		if r.Version != recipeVersion {
			e := Errorf(CodeStale, "the recipe changed since (it's at version %d, not %d); read it and its graph again", r.Version, recipeVersion)
			e.Current = &r
			return e
		}
		prev, err := loadGraph(ctx, tx, recipeID)
		if err != nil {
			return err
		}
		switch {
		case prev == nil && version != 0:
			return Errorf(CodeInvalid, "the recipe has no step graph; send version 0 to make one")
		case prev != nil && prev.Version != version:
			cur, err := graphView(ctx, tx, recipeID, who)
			if err != nil {
				return err
			}
			e := Errorf(CodeStale, "someone changed the step graph since you read it")
			e.CurrentGraph = &cur
			return e
		}
		if err := s.saveGraph(ctx, tx, c, r, prev, in, nil, "Edited "+r.Title+": step graph"); err != nil {
			return err
		}
		out, err = graphView(ctx, tx, recipeID, who)
		return err
	})
	return out, err
}

// saveGraph normalises in, checks a verified one, stamps reviews (carried
// over from prev, every label when verified, and the labels in stamp) and
// stores it as the recipe's graph, logging summary. The caller has checked
// the recipe and graph versions.
func (s *Service) saveGraph(ctx context.Context, tx *sql.Tx, c identity.Caller, r Recipe, prev *RecipeGraph, in GraphInput,
	stamp map[graphLabelRef]bool, summary string) error {
	who, now := c.Username(), s.stamp()
	if err := normaliseGraph(&in, r); err != nil {
		return err
	}
	k, err := loadKitchen(ctx, tx)
	if err != nil {
		return err
	}
	g := RecipeGraph{
		RecipeID: r.ID, RecipeVersion: r.Version, OntologyVersion: OntologyVersion,
		Status: in.Status, ServeItemID: in.ServeItemID, Nodes: in.Nodes, Items: in.Items,
		CreatedAt: now, CreatedBy: who, UpdatedAt: now, UpdatedBy: who,
	}
	if in.Status == GraphVerified {
		if chk := checkGraph(&g, r, k.KitchenProfile); !chk.Valid {
			msgs := make([]string, 0, 3)
			for _, p := range chk.Errors[:min(3, len(chk.Errors))] {
				msgs = append(msgs, p.Message)
			}
			return Errorf(CodeInvalid, "a verified graph can't have errors, and this one has %d: %s", len(chk.Errors), strings.Join(msgs, "; "))
		}
	}
	stampReviews(&g, prev, in.Status == GraphVerified, who, now, stamp)
	if prev != nil && prev.RecipeVersion == r.Version {
		g.CreatedAt, g.CreatedBy = prev.CreatedAt, prev.CreatedBy
	}
	if err := tx.QueryRowContext(ctx, `SELECT coalesce(max(version), 0) + 1 FROM recipe_graphs WHERE recipe_id = ?`, r.ID).
		Scan(&g.Version); err != nil {
		return err
	}
	if err := storeGraph(ctx, tx, g); err != nil {
		return err
	}
	return s.logChange(ctx, tx, c, now, "recipe", r.ID, "update", summary)
}

// DeleteRecipeGraph takes a recipe's step graph away (into the trash with
// its earlier ones), so the recipe plans by its step timing alone.
func (s *Service) DeleteRecipeGraph(ctx context.Context, c identity.Caller, recipeID string, version int) error {
	who, err := author(c)
	if err != nil {
		return err
	}
	now := s.stamp()
	return s.st.Write(ctx, func(tx *sql.Tx) error {
		r, err := loadRecipe(ctx, tx, recipeID, who)
		if err != nil {
			return err
		}
		prev, err := loadGraph(ctx, tx, recipeID)
		if err != nil {
			return err
		}
		if prev == nil {
			return Errorf(CodeNotFound, "this recipe has no step graph")
		}
		if prev.Version != version {
			cur, err := graphView(ctx, tx, recipeID, who)
			if err != nil {
				return err
			}
			e := Errorf(CodeStale, "someone changed the step graph since you read it")
			e.CurrentGraph = &cur
			return e
		}
		if _, err := tx.ExecContext(ctx, `UPDATE recipe_graphs SET deleted_at = ?, updated_at = ?, updated_by = ?
			WHERE recipe_id = ? AND deleted_at IS NULL`, now, now, who, recipeID); err != nil {
			return err
		}
		return s.logChange(ctx, tx, c, now, "recipe", recipeID, "update", "Edited "+r.Title+": removed its step graph")
	})
}

// CheckRecipeGraph runs the checks on a proposed graph for a recipe, or on
// its saved one when in is nil, without saving anything.
func (s *Service) CheckRecipeGraph(ctx context.Context, c identity.Caller, recipeID string, in *GraphInput) (GraphCheck, error) {
	var out GraphCheck
	err := s.st.Read(ctx, func(tx *sql.Tx) error {
		if in == nil {
			v, err := graphView(ctx, tx, recipeID, c.Username())
			out = v.Check
			return err
		}
		r, err := loadRecipe(ctx, tx, recipeID, c.Username())
		if err != nil {
			return err
		}
		k, err := loadKitchen(ctx, tx)
		if err != nil {
			return err
		}
		if err := normaliseGraph(in, r); err != nil {
			return err
		}
		g := RecipeGraph{Status: in.Status, ServeItemID: in.ServeItemID, Nodes: in.Nodes, Items: in.Items}
		out = checkGraph(&g, r, k.KitchenProfile)
		return nil
	})
	return out, err
}

// graphView loads a recipe's graph with its checks and projection.
func graphView(ctx context.Context, tx *sql.Tx, recipeID, username string) (GraphView, error) {
	r, err := loadRecipe(ctx, tx, recipeID, username)
	if err != nil {
		return GraphView{}, err
	}
	g, err := loadGraph(ctx, tx, recipeID)
	if err != nil {
		return GraphView{}, err
	}
	if g == nil {
		return GraphView{}, Errorf(CodeNotFound, "this recipe has no step graph yet")
	}
	k, err := loadKitchen(ctx, tx)
	if err != nil {
		return GraphView{}, err
	}
	if markStale(g, r) {
		g.Status = GraphStale
	}
	return GraphView{Graph: *g, ReviewCount: reviewCount(g.Nodes), Check: checkGraph(g, r, k.KitchenProfile), Projection: project(*g, len(r.Steps))}, nil
}

// stampReviews carries review stamps over from prev for labels whose value
// hasn't changed, and stamps every label still unreviewed with verify, or
// those named in stamp.
func stampReviews(g *RecipeGraph, prev *RecipeGraph, verify bool, who, now string, stamp map[graphLabelRef]bool) {
	type kept struct {
		value  string
		by, at *string
	}
	old := map[graphLabelRef]kept{}
	if prev != nil {
		for i := range prev.Nodes {
			for _, l := range labelsOf(&prev.Nodes[i]) {
				if *l.by != nil {
					old[graphLabelRef{prev.Nodes[i].ID, l.field}] = kept{l.json(), *l.by, *l.at}
				}
			}
		}
	}
	for i := range g.Nodes {
		for _, l := range labelsOf(&g.Nodes[i]) {
			if o, ok := old[graphLabelRef{g.Nodes[i].ID, l.field}]; ok && o.value == l.json() {
				*l.by, *l.at = o.by, o.at
			} else if verify || stamp[graphLabelRef{g.Nodes[i].ID, l.field}] {
				by, at := who, now
				*l.by, *l.at = &by, &at
			}
		}
	}
}

// labelRow is one label of a node, reached through pointers so it can be
// read, stamped and stored without knowing its value's type.
type labelRow struct {
	field  string
	value  any
	alts   any
	source *string
	conf   **float64
	by, at **string
}

func (l labelRow) json() string {
	b, _ := json.Marshal(l.value)
	return string(b)
}

func rowOf[T any](field string, l *Label[T]) labelRow {
	var alts any
	if len(l.Alternatives) > 0 {
		alts = l.Alternatives
	}
	return labelRow{field, l.Value, alts, &l.Source, &l.Confidence, &l.ReviewedBy, &l.ReviewedAt}
}

// labelsOf lists a node's labels that are set.
func labelsOf(n *ActionNode) []labelRow {
	rows := []labelRow{rowOf("action", &n.Action), rowOf("duration", &n.Duration), rowOf("attention", &n.Attention)}
	if n.CheckInterval != nil {
		rows = append(rows, rowOf("check_interval_min", n.CheckInterval))
	}
	if n.Interruptible != nil {
		rows = append(rows, rowOf("interruptible", n.Interruptible))
	}
	if n.Equipment != nil {
		rows = append(rows, rowOf("equipment", n.Equipment))
	}
	if n.Hold != nil {
		rows = append(rows, rowOf("hold", n.Hold))
	}
	if n.TimingAnchor != nil {
		rows = append(rows, rowOf("timing_anchor", n.TimingAnchor))
	}
	return rows
}

// storeGraph writes g in place of any graph at the same recipe version.
func storeGraph(ctx context.Context, tx *sql.Tx, g RecipeGraph) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM recipe_graphs WHERE recipe_id = ? AND recipe_version = ?`, g.RecipeID, g.RecipeVersion); err != nil {
		return err
	}
	// Older graphs give way: the newest is the recipe's graph, and history
	// keeps the rest.
	if _, err := tx.ExecContext(ctx, `UPDATE recipe_graphs SET deleted_at = ? WHERE recipe_id = ? AND deleted_at IS NULL`,
		g.UpdatedAt, g.RecipeID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO recipe_graphs (recipe_id, recipe_version, version, ontology_version, status,
		serve_item_id, created_at, created_by, updated_at, updated_by) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		g.RecipeID, g.RecipeVersion, g.Version, g.OntologyVersion, g.Status, g.ServeItemID,
		g.CreatedAt, g.CreatedBy, g.UpdatedAt, g.UpdatedBy); err != nil {
		return err
	}
	for i, it := range g.Items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO graph_items (recipe_id, recipe_version, id, position, kind, label, ingredient, quantity)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, g.RecipeID, g.RecipeVersion, it.ID, i, it.Kind, it.Label, it.Ingredient, it.Quantity); err != nil {
			return err
		}
	}
	for i := range g.Nodes {
		n := &g.Nodes[i]
		ins, _ := json.Marshal(n.Inputs)
		outs, _ := json.Marshal(n.Outputs)
		if _, err := tx.ExecContext(ctx, `INSERT INTO graph_nodes (recipe_id, recipe_version, id, position, step, span_start, span_end,
			quote, text, technique, inputs, outputs, implied) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			g.RecipeID, g.RecipeVersion, n.ID, i, n.Source.Step, n.Source.Start, n.Source.End, n.Source.Quote,
			n.Text, n.Technique, string(ins), string(outs), n.Implied); err != nil {
			return err
		}
		for _, l := range labelsOf(n) {
			var alts *string
			if l.alts != nil {
				b, _ := json.Marshal(l.alts)
				a := string(b)
				alts = &a
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO graph_labels (recipe_id, recipe_version, node_id, field, value, source,
				confidence, reviewed_by, reviewed_at, alternatives) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				g.RecipeID, g.RecipeVersion, n.ID, l.field, l.json(), *l.source, **l.conf, *l.by, *l.at, alts); err != nil {
				return err
			}
		}
	}
	return nil
}

// loadGraph returns a recipe's newest graph that isn't deleted, or nil.
func loadGraph(ctx context.Context, tx *sql.Tx, recipeID string) (*RecipeGraph, error) {
	g := RecipeGraph{RecipeID: recipeID, Nodes: []ActionNode{}, Items: []GraphItem{}}
	err := tx.QueryRowContext(ctx, `SELECT recipe_version, version, ontology_version, status, serve_item_id,
		created_at, created_by, updated_at, updated_by FROM recipe_graphs
		WHERE recipe_id = ? AND deleted_at IS NULL ORDER BY version DESC LIMIT 1`, recipeID).
		Scan(&g.RecipeVersion, &g.Version, &g.OntologyVersion, &g.Status, &g.ServeItemID,
			&g.CreatedAt, &g.CreatedBy, &g.UpdatedAt, &g.UpdatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	key := []any{recipeID, g.RecipeVersion}

	rows, err := tx.QueryContext(ctx, `SELECT id, kind, label, ingredient, quantity FROM graph_items
		WHERE recipe_id = ? AND recipe_version = ? ORDER BY position`, key...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var it GraphItem
		if err := rows.Scan(&it.ID, &it.Kind, &it.Label, &it.Ingredient, &it.Quantity); err != nil {
			rows.Close()
			return nil, err
		}
		g.Items = append(g.Items, it)
	}
	if err := closeRows(rows); err != nil {
		return nil, err
	}

	byID := map[string]int{}
	rows, err = tx.QueryContext(ctx, `SELECT id, step, span_start, span_end, quote, text, technique, inputs, outputs, implied
		FROM graph_nodes WHERE recipe_id = ? AND recipe_version = ? ORDER BY position`, key...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var n ActionNode
		var ins, outs string
		if err := rows.Scan(&n.ID, &n.Source.Step, &n.Source.Start, &n.Source.End, &n.Source.Quote, &n.Text,
			&n.Technique, &ins, &outs, &n.Implied); err != nil {
			rows.Close()
			return nil, err
		}
		if err := errors.Join(json.Unmarshal([]byte(ins), &n.Inputs), json.Unmarshal([]byte(outs), &n.Outputs)); err != nil {
			rows.Close()
			return nil, err
		}
		byID[n.ID] = len(g.Nodes)
		g.Nodes = append(g.Nodes, n)
	}
	if err := closeRows(rows); err != nil {
		return nil, err
	}

	rows, err = tx.QueryContext(ctx, `SELECT node_id, field, value, source, confidence, reviewed_by, reviewed_at, alternatives
		FROM graph_labels WHERE recipe_id = ? AND recipe_version = ?`, key...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var node, field, value, source string
		var conf float64
		var by, at, alts *string
		if err := rows.Scan(&node, &field, &value, &source, &conf, &by, &at, &alts); err != nil {
			rows.Close()
			return nil, err
		}
		i, ok := byID[node]
		if !ok {
			continue
		}
		if err := fillLabel(&g.Nodes[i], field, value, source, conf, by, at, alts); err != nil {
			rows.Close()
			return nil, err
		}
	}
	if err := closeRows(rows); err != nil {
		return nil, err
	}
	return &g, nil
}

func closeRows(rows *sql.Rows) error {
	return errors.Join(rows.Close(), rows.Err())
}

// fillLabel sets one stored label on its node.
func fillLabel(n *ActionNode, field, value, source string, conf float64, by, at, alts *string) error {
	switch field {
	case "action":
		return setLabel(&n.Action, value, source, conf, by, at, alts)
	case "duration":
		return setLabel(&n.Duration, value, source, conf, by, at, alts)
	case "attention":
		return setLabel(&n.Attention, value, source, conf, by, at, alts)
	case "check_interval_min":
		n.CheckInterval = new(Label[int])
		return setLabel(n.CheckInterval, value, source, conf, by, at, alts)
	case "interruptible":
		n.Interruptible = new(Label[bool])
		return setLabel(n.Interruptible, value, source, conf, by, at, alts)
	case "equipment":
		n.Equipment = new(Label[[]EquipmentClaim])
		return setLabel(n.Equipment, value, source, conf, by, at, alts)
	case "hold":
		n.Hold = new(Label[Hold])
		return setLabel(n.Hold, value, source, conf, by, at, alts)
	case "timing_anchor":
		n.TimingAnchor = new(Label[string])
		return setLabel(n.TimingAnchor, value, source, conf, by, at, alts)
	}
	return nil
}

func setLabel[T any](l *Label[T], value, source string, conf float64, by, at, alts *string) error {
	l.Source, l.Confidence, l.ReviewedBy, l.ReviewedAt = source, &conf, by, at
	// A value label (action, duration, attention) is set in place, so the
	// alternatives the model weighed for its old value go with it.
	l.Alternatives = nil
	if alts != nil {
		if err := json.Unmarshal([]byte(*alts), &l.Alternatives); err != nil {
			return err
		}
	}
	return json.Unmarshal([]byte(value), &l.Value)
}
