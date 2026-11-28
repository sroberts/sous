package recipes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"slices"
	"uuid"

	"github.com/sroberts/sous/internal/identity"
)

// ErrModelsUnavailable is what a Generator or Decider wraps when it can't
// reach its models at all, as opposed to a model answering badly.
var ErrModelsUnavailable = errors.New("the models for step graphs can't be reached")

// Upgrade modes: a draft is saved for review whatever the labels say; apply
// routes it to needs_review, or verified when every label is sure and the
// household allows that.
const (
	UpgradeDraft = "draft"
	UpgradeApply = "apply"
)

// UpgradeRequest is one recipe to upgrade.
type UpgradeRequest struct {
	ID    string `json:"id" jsonschema:"recipe ID"`
	Mode  string `json:"mode,omitempty" jsonschema:"draft (unset): save for review whatever the labels say; apply: needs_review, or verified when the household allows it and every label is sure"`
	Force bool   `json:"force,omitempty" jsonschema:"redo a recipe that already has a step graph, replacing it"`
}

// UpgradeResult is what an upgrade made.
type UpgradeResult struct {
	Graph    GraphView `json:"graph"`
	Attempts int       `json:"attempts" jsonschema:"1, or 2 when the first graph had errors and was sent back to be fixed"`
	Unplaced int       `json:"unplaced_quotes" jsonschema:"actions whose quoted words weren't found in their step; they point at the whole step"`
}

// unavailable turns a failure to reach the models into CodeUnavailable.
func unavailable(err error) error {
	if errors.Is(err, ErrModelsUnavailable) {
		return Errorf(CodeUnavailable, "%v", err)
	}
	return err
}

// UpgradeRecipe turns one recipe into a step graph now. It takes as long as
// the models do: tens of seconds on a laptop.
func (s *Service) UpgradeRecipe(ctx context.Context, c identity.Caller, in UpgradeRequest) (UpgradeResult, error) {
	if _, err := author(c); err != nil {
		return UpgradeResult{}, err
	}
	if s.up == nil {
		return UpgradeResult{}, Errorf(CodeUnavailable, "upgrades are off on this Sous (SOUS_OLLAMA_URL=off)")
	}
	if in.Mode == "" {
		in.Mode = UpgradeDraft
	}
	if in.Mode != UpgradeDraft && in.Mode != UpgradeApply {
		return UpgradeResult{}, Errorf(CodeInvalid, "mode must be draft or apply")
	}
	res, err := s.upgrade(ctx, c, in)
	return res, unavailable(err)
}

// noGraphModel is why Sous can't write a graph's structure itself.
const noGraphModel = "Sous has no graph model of its own (SOUS_GRAPH_MODEL); draft the graph's structure with draft_recipe_graph instead"

func (s *Service) upgrade(ctx context.Context, c identity.Caller, in UpgradeRequest) (UpgradeResult, error) {
	if s.up.Gen == nil {
		return UpgradeResult{}, Errorf(CodeUnavailable, noGraphModel)
	}
	return s.upgradeWith(ctx, c, in, 0, func(r Recipe, k KitchenProfile) (GraphInput, int, int, error) {
		return s.up.run(ctx, r, k)
	})
}

// structureFn makes a classified graph for a recipe: attempts and quotes
// it couldn't place come back with it.
type structureFn func(r Recipe, k KitchenProfile) (g GraphInput, attempts, unplaced int, err error)

// upgradeWith reads the recipe, has produce make its graph, routes it and
// saves it, refusing if the recipe or its graph changed meanwhile.
// recipeVersion, when not 0, is the version the structure was written
// against.
func (s *Service) upgradeWith(ctx context.Context, c identity.Caller, in UpgradeRequest, recipeVersion int, produce structureFn) (UpgradeResult, error) {
	var r Recipe
	var k Kitchen
	var prev *RecipeGraph
	err := s.st.Read(ctx, func(tx *sql.Tx) error {
		var err error
		if r, err = loadRecipe(ctx, tx, in.ID, c.Username()); err != nil {
			return err
		}
		if k, err = loadKitchen(ctx, tx); err != nil {
			return err
		}
		prev, err = loadGraph(ctx, tx, in.ID)
		return err
	})
	if err != nil {
		return UpgradeResult{}, err
	}
	if r.DeletedAt != nil {
		return UpgradeResult{}, Errorf(CodeDeleted, "this recipe is in the trash; restore it first")
	}
	if prev != nil && !in.Force {
		return UpgradeResult{}, Errorf(CodeExists, "this recipe already has a step graph; pass force to redo it")
	}
	if len(r.Steps) == 0 {
		return UpgradeResult{}, Errorf(CodeInvalid, "this recipe has no steps to upgrade")
	}
	if recipeVersion != 0 && recipeVersion != r.Version {
		e := Errorf(CodeStale, "the recipe changed since you read it (it's at version %d, not %d); read it again and redraft", r.Version, recipeVersion)
		e.Current = &r
		return UpgradeResult{}, e
	}

	g, attempts, missed, err := produce(r, k.KitchenProfile)
	if err != nil {
		return UpgradeResult{}, err
	}
	g.Status = GraphDraft
	if in.Mode == UpgradeApply {
		g.Status = GraphNeedsReview
		chk := checkGraph(&RecipeGraph{ServeItemID: g.ServeItemID, Nodes: g.Nodes, Items: g.Items}, r, k.KitchenProfile)
		if s.up.AutoVerify && chk.Valid && reviewCount(g.Nodes) == 0 {
			g.Status = GraphVerified
		}
	}

	// Save against the recipe and graph as they were read: the models
	// took a while, and someone may have changed either since.
	var out UpgradeResult
	err = s.st.Write(ctx, func(tx *sql.Tx) error {
		cur, err := loadRecipe(ctx, tx, in.ID, c.Username())
		if err != nil {
			return err
		}
		if cur.Version != r.Version {
			e := Errorf(CodeStale, "the recipe changed while it was being upgraded; upgrade it again")
			e.Current = &cur
			return e
		}
		now, err := loadGraph(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		if (now == nil) != (prev == nil) || (now != nil && now.Version != prev.Version) {
			return Errorf(CodeExists, "someone saved a step graph for this recipe while it was being upgraded")
		}
		if err := s.saveGraph(ctx, tx, c, r, now, g, nil, "Upgraded "+r.Title+" to a step graph"); err != nil {
			return err
		}
		out.Graph, err = graphView(ctx, tx, in.ID, c.Username())
		return err
	})
	out.Attempts, out.Unplaced = attempts, missed
	return out, err
}

// run is the pipeline: generate, assemble, check the structure (sending it
// back once with the problems), then classify.
func (u *Upgrader) run(ctx context.Context, r Recipe, k KitchenProfile) (GraphInput, int, int, error) {
	var g GraphInput
	var missed int
	var prev *StructureDraft
	var problems []GraphProblem
	attempts := 0
	for attempts < 2 {
		attempts++
		d, err := u.generate(ctx, r, prev, problems)
		if err != nil {
			return GraphInput{}, attempts, 0, err
		}
		var unplaced []string
		g, unplaced = assemble(d, r)
		missed = len(unplaced)
		if len(g.Nodes) == 0 {
			problems = []GraphProblem{{Code: "no_nodes", Message: "the graph has no actions"}}
			prev = &d
			continue
		}
		problems = structureProblems(g, r, k)
		if len(problems) == 0 {
			break
		}
		prev = &d
	}
	if len(g.Nodes) == 0 {
		return GraphInput{}, attempts, missed, fmt.Errorf("the graph model made no actions for this recipe")
	}
	if err := u.classify(ctx, r, k, &g); err != nil {
		return GraphInput{}, attempts, missed, err
	}
	return g, attempts, missed, nil
}

// structureProblems checks a graph's shape before it's classified: the
// errors that come from how actions and items join, with stand-in labels.
// (A missing preheat is found by the full checks after classifying, when
// the oven claims are known.)
func structureProblems(g GraphInput, r Recipe, k KitchenProfile) []GraphProblem {
	probe := RecipeGraph{ServeItemID: g.ServeItemID, Items: g.Items}
	for _, n := range g.Nodes {
		n.Action = Label[string]{Value: "prep"}
		n.Attention = Label[string]{Value: AttentionActive}
		words := []rune(r.Steps[n.Source.Step-1].Text)
		n.Source.Quote = string(words[n.Source.Start:n.Source.End])
		probe.Nodes = append(probe.Nodes, n)
	}
	return checkGraph(&probe, r, k).Errors
}

// DraftRequest is a graph's structure written by an assistant, for Sous to
// classify and save.
type DraftRequest struct {
	ID            string `json:"id" jsonschema:"recipe ID"`
	RecipeVersion int    `json:"recipe_version" jsonschema:"the recipe's version from get_recipe; the quotes are read against its words"`
	Mode          string `json:"mode,omitempty" jsonschema:"draft (unset): save for review whatever the labels say; apply: needs_review, or verified when the household allows it and every label is sure"`
	Force         bool   `json:"force,omitempty" jsonschema:"replace the recipe's step graph if it has one"`
	StructureDraft
}

// DraftRecipeGraph takes a graph's structure written by someone else (an
// assistant), checks it, has the decision model classify every action and
// saves it as an upgrade does. A structure with problems (a quote not in
// its step, an input nothing makes, an output nothing uses, a cycle) is
// refused with all of them listed, to fix and send again.
func (s *Service) DraftRecipeGraph(ctx context.Context, c identity.Caller, in DraftRequest) (UpgradeResult, error) {
	if _, err := author(c); err != nil {
		return UpgradeResult{}, err
	}
	if s.up == nil {
		return UpgradeResult{}, Errorf(CodeUnavailable, "upgrades are off on this Sous (SOUS_OLLAMA_URL=off)")
	}
	if in.Mode == "" {
		in.Mode = UpgradeDraft
	}
	if in.Mode != UpgradeDraft && in.Mode != UpgradeApply {
		return UpgradeResult{}, Errorf(CodeInvalid, "mode must be draft or apply")
	}
	if in.RecipeVersion < 1 {
		return UpgradeResult{}, Errorf(CodeInvalid, "recipe_version is required: the version the quotes were read from")
	}
	res, err := s.upgradeWith(ctx, c, UpgradeRequest{ID: in.ID, Mode: in.Mode, Force: in.Force}, in.RecipeVersion,
		func(r Recipe, k KitchenProfile) (GraphInput, int, int, error) {
			g, unplaced := assemble(in.StructureDraft, r)
			if len(g.Nodes) == 0 {
				return GraphInput{}, 1, 0, Errorf(CodeInvalid, "the draft has no actions")
			}
			var msgs []string
			for _, id := range unplaced {
				msgs = append(msgs, fmt.Sprintf("%s: its quote isn't in step %d, or any step; copy the words exactly", id, stepOf(g, id)))
			}
			for _, p := range structureProblems(g, r, k) {
				msgs = append(msgs, p.Message)
			}
			if len(msgs) > 0 {
				return GraphInput{}, 1, len(unplaced), Errorf(CodeInvalid, "the draft has %d problems; fix them and send the whole draft again:\n- %s",
					len(msgs), strings.Join(msgs[:min(len(msgs), 30)], "\n- "))
			}
			return g, 1, 0, s.up.classify(ctx, r, k, &g)
		})
	return res, unavailable(err)
}

func stepOf(g GraphInput, node string) int {
	for _, n := range g.Nodes {
		if n.ID == node {
			return n.Source.Step
		}
	}
	return 0
}

// GraphTodo is a recipe without a step graph.
type GraphTodo struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Version int    `json:"version"`
	Steps   int    `json:"steps"`
}

// GraphTodos is a batch of recipes without a step graph, and how many
// there are in all.
type GraphTodos struct {
	Items     []GraphTodo `json:"items"`
	Remaining int         `json:"remaining" jsonschema:"recipes without a step graph in all, this batch included"`
}

// GraphTodoQuery narrows RecipesWithoutGraph.
type GraphTodoQuery struct {
	MealID string `json:"meal_id,omitempty" jsonschema:"only the dishes on this meal"`
	Limit  int    `json:"limit,omitempty" jsonschema:"how many to list; default 20, at most 100"`
}

// RecipesWithoutGraph lists recipes with steps and no step graph, by title.
func (s *Service) RecipesWithoutGraph(ctx context.Context, c identity.Caller, q GraphTodoQuery) (GraphTodos, error) {
	limit := q.Limit
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 {
		return GraphTodos{}, Errorf(CodeInvalid, "limit must be 1 to 100")
	}
	where := `r.deleted_at IS NULL AND NOT EXISTS (SELECT 1 FROM recipe_graphs g WHERE g.recipe_id = r.id AND g.deleted_at IS NULL)`
	var args []any
	if q.MealID != "" {
		where += " AND r.id IN (SELECT recipe_id FROM meal_items WHERE meal_id = ?)"
		args = append(args, q.MealID)
	}
	out := GraphTodos{Items: []GraphTodo{}}
	err := s.st.Read(ctx, func(tx *sql.Tx) error {
		if q.MealID != "" {
			if _, err := loadMeal(ctx, tx, q.MealID); err != nil {
				return err
			}
		}
		base := `SELECT r.id, r.title, r.version, count(*) FROM recipes r JOIN recipe_steps st ON st.recipe_id = r.id
			WHERE ` + where + ` GROUP BY r.id`
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM ("+base+")", args...).Scan(&out.Remaining); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, base+" ORDER BY r.title COLLATE NOCASE, r.id LIMIT ?", append(args, limit)...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var t GraphTodo
			if err := rows.Scan(&t.ID, &t.Title, &t.Version, &t.Steps); err != nil {
				rows.Close()
				return err
			}
			out.Items = append(out.Items, t)
		}
		return closeRows(rows)
	})
	return out, err
}

// Jobs.

// UpgradeJobRequest chooses recipes to upgrade in the background: by id, or
// by tag or meal, by default only those without a graph.
type UpgradeJobRequest struct {
	IDs          []string `json:"ids,omitempty" jsonschema:"recipe IDs; or leave out and filter"`
	Tag          string   `json:"tag,omitempty" jsonschema:"only recipes with this tag"`
	MealID       string   `json:"meal_id,omitempty" jsonschema:"only the dishes on this meal"`
	WithoutGraph *bool    `json:"without_graph,omitempty" jsonschema:"only recipes with no step graph yet; unset is true"`
	Mode         string   `json:"mode,omitempty" jsonschema:"draft (unset) or apply, as for upgrade_recipe"`
	Force        bool     `json:"force,omitempty" jsonschema:"redo recipes that already have a graph"`
	Limit        int      `json:"limit,omitempty" jsonschema:"at most this many recipes, by title; unset is all that match, up to 1000"`
}

// UpgradeJob is a bulk upgrade and how far it has got.
type UpgradeJob struct {
	ID         string           `json:"id"`
	Status     string           `json:"status" jsonschema:"queued, running, done or cancelled"`
	Mode       string           `json:"mode"`
	Force      bool             `json:"force"`
	CreatedAt  string           `json:"created_at"`
	CreatedBy  string           `json:"created_by"`
	FinishedAt *string          `json:"finished_at"`
	Counts     map[string]int   `json:"counts" jsonschema:"recipes by status: pending, running, done, needs_review, failed, skipped"`
	Items      []UpgradeJobItem `json:"items"`
}

// UpgradeJobItem is one recipe in a job.
type UpgradeJobItem struct {
	RecipeID    string  `json:"recipe_id"`
	Title       string  `json:"title"`
	Status      string  `json:"status" jsonschema:"pending, running, done (saved, nothing to review), needs_review, failed or skipped (deleted, or already has a graph)"`
	ReviewCount *int    `json:"review_count"`
	Error       *string `json:"error"`
}

const maxJobRecipes = 1000

// QueueUpgrades starts a bulk upgrade, worked through in the background.
func (s *Service) QueueUpgrades(ctx context.Context, c identity.Caller, in UpgradeJobRequest) (UpgradeJob, error) {
	who, err := author(c)
	if err != nil {
		return UpgradeJob{}, err
	}
	if s.up == nil {
		return UpgradeJob{}, Errorf(CodeUnavailable, "upgrades are off on this Sous (SOUS_OLLAMA_URL=off)")
	}
	if s.up.Gen == nil {
		return UpgradeJob{}, Errorf(CodeUnavailable, noGraphModel)
	}
	if in.Mode == "" {
		in.Mode = UpgradeDraft
	}
	if in.Mode != UpgradeDraft && in.Mode != UpgradeApply {
		return UpgradeJob{}, Errorf(CodeInvalid, "mode must be draft or apply")
	}
	if in.Limit < 0 || in.Limit > maxJobRecipes {
		return UpgradeJob{}, Errorf(CodeInvalid, "limit must be 1 to %d", maxJobRecipes)
	}
	if len(in.IDs) > maxJobRecipes {
		return UpgradeJob{}, Errorf(CodeInvalid, "ids: at most %d", maxJobRecipes)
	}
	limit := in.Limit
	if limit == 0 {
		limit = maxJobRecipes
	}
	without := in.WithoutGraph == nil || *in.WithoutGraph

	where := []string{"r.deleted_at IS NULL"}
	var args []any
	if len(in.IDs) > 0 {
		where = append(where, "r.id IN ("+placeholders(len(in.IDs))+")")
		for _, id := range in.IDs {
			args = append(args, id)
		}
	}
	if in.Tag != "" {
		where = append(where, "r.id IN (SELECT rt.recipe_id FROM recipe_tags rt JOIN tags t ON t.id = rt.tag_id WHERE t.name = ? COLLATE NOCASE)")
		args = append(args, in.Tag)
	}
	if in.MealID != "" {
		where = append(where, "r.id IN (SELECT recipe_id FROM meal_items WHERE meal_id = ?)")
		args = append(args, in.MealID)
	}
	if without && !in.Force {
		where = append(where, "NOT EXISTS (SELECT 1 FROM recipe_graphs g WHERE g.recipe_id = r.id AND g.deleted_at IS NULL)")
	}
	id := uuid.NewV7().String()
	now := s.stamp()
	err = s.st.Write(ctx, func(tx *sql.Tx) error {
		if in.MealID != "" {
			if _, err := loadMeal(ctx, tx, in.MealID); err != nil {
				return err
			}
		}
		rows, err := tx.QueryContext(ctx, `SELECT r.id FROM recipes r WHERE `+strings.Join(where, " AND ")+
			` ORDER BY r.title COLLATE NOCASE, r.id LIMIT ?`, append(args, limit)...)
		if err != nil {
			return err
		}
		var ids []string
		for rows.Next() {
			var rid string
			if err := rows.Scan(&rid); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, rid)
		}
		if err := closeRows(rows); err != nil {
			return err
		}
		if len(ids) == 0 {
			return Errorf(CodeInvalid, "no recipes match")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO upgrade_jobs (id, status, mode, force, via, created_at, created_by)
			VALUES (?, 'queued', ?, ?, ?, ?, ?)`, id, in.Mode, in.Force, viaOf(c), now, who); err != nil {
			return err
		}
		for i, rid := range ids {
			if _, err := tx.ExecContext(ctx, `INSERT INTO upgrade_job_items (job_id, position, recipe_id, status) VALUES (?, ?, ?, 'pending')`,
				id, i, rid); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return UpgradeJob{}, err
	}
	s.wake()
	return s.UpgradeJob(ctx, c, id)
}

// UpgradeJob reports a job's progress.
func (s *Service) UpgradeJob(ctx context.Context, c identity.Caller, id string) (UpgradeJob, error) {
	var j UpgradeJob
	err := s.st.Read(ctx, func(tx *sql.Tx) error {
		var err error
		j, err = loadJob(ctx, tx, id)
		return err
	})
	return j, err
}

// CancelUpgradeJob stops a job: recipes not yet started are skipped.
func (s *Service) CancelUpgradeJob(ctx context.Context, c identity.Caller, id string) (UpgradeJob, error) {
	if _, err := author(c); err != nil {
		return UpgradeJob{}, err
	}
	now := s.stamp()
	err := s.st.Write(ctx, func(tx *sql.Tx) error {
		if _, err := loadJob(ctx, tx, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE upgrade_job_items SET status = 'skipped', error = 'cancelled', finished_at = ?
			WHERE job_id = ? AND status = 'pending'`, now, id); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE upgrade_jobs SET status = 'cancelled', finished_at = coalesce(finished_at, ?)
			WHERE id = ? AND status IN ('queued', 'running')`, now, id)
		return err
	})
	if err != nil {
		return UpgradeJob{}, err
	}
	return s.UpgradeJob(ctx, c, id)
}

func loadJob(ctx context.Context, tx *sql.Tx, id string) (UpgradeJob, error) {
	j := UpgradeJob{ID: id, Counts: map[string]int{}, Items: []UpgradeJobItem{}}
	err := tx.QueryRowContext(ctx, `SELECT status, mode, force, created_at, created_by, finished_at FROM upgrade_jobs WHERE id = ?`, id).
		Scan(&j.Status, &j.Mode, &j.Force, &j.CreatedAt, &j.CreatedBy, &j.FinishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return j, Errorf(CodeNotFound, "no upgrade job %s", id)
	} else if err != nil {
		return j, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT i.recipe_id, r.title, i.status, i.review_count, i.error
		FROM upgrade_job_items i JOIN recipes r ON r.id = i.recipe_id WHERE i.job_id = ? ORDER BY i.position`, id)
	if err != nil {
		return j, err
	}
	for rows.Next() {
		var it UpgradeJobItem
		if err := rows.Scan(&it.RecipeID, &it.Title, &it.Status, &it.ReviewCount, &it.Error); err != nil {
			rows.Close()
			return j, err
		}
		j.Counts[it.Status]++
		j.Items = append(j.Items, it)
	}
	return j, closeRows(rows)
}

func (s *Service) wake() {
	if s.job == nil {
		return
	}
	select {
	case s.job <- struct{}{}:
	default:
	}
}

// RunUpgrades works through queued upgrade jobs, one recipe at a time,
// until ctx ends. Recipes left running by a stop are picked up again. If
// the models can't be reached it waits and tries the same recipe again
// rather than failing the rest.
func (s *Service) RunUpgrades(ctx context.Context) {
	if s.up == nil || s.up.Gen == nil {
		return
	}
	if err := s.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE upgrade_job_items SET status = 'pending' WHERE status = 'running'`)
		return err
	}); err != nil {
		s.log.Warn("upgrade worker: requeue failed", "error", err.Error())
	}
	idle := 30 * time.Second
	for {
		worked, wait := s.upgradeNext(ctx)
		if ctx.Err() != nil {
			return
		}
		if worked {
			continue
		}
		if wait == 0 {
			wait = idle
		}
		select {
		case <-ctx.Done():
			return
		case <-s.job:
		case <-time.After(wait):
		}
	}
}

// upgradeNext upgrades the next pending recipe, if any. It reports whether
// it did one, and how long to wait when the models were unreachable.
func (s *Service) upgradeNext(ctx context.Context) (bool, time.Duration) {
	var jobID, recipeID, mode, via, by string
	var pos int
	var force bool
	err := s.st.Write(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `SELECT i.job_id, i.position, i.recipe_id, j.mode, j.force, j.via, j.created_by
			FROM upgrade_job_items i JOIN upgrade_jobs j ON j.id = i.job_id
			WHERE i.status = 'pending' AND j.status IN ('queued', 'running')
			ORDER BY j.created_at, i.position LIMIT 1`).Scan(&jobID, &pos, &recipeID, &mode, &force, &via, &by)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE upgrade_job_items SET status = 'running' WHERE job_id = ? AND position = ?`, jobID, pos); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE upgrade_jobs SET status = 'running' WHERE id = ? AND status = 'queued'`, jobID)
		return err
	})
	if errors.Is(err, sql.ErrNoRows) {
		return false, 0
	}
	if err != nil {
		if ctx.Err() == nil {
			s.log.Warn("upgrade worker: picking the next recipe failed", "error", err.Error())
		}
		return false, time.Minute
	}

	c := identity.Caller{Person: &identity.Person{Username: by}, Route: identity.RouteDirect, Via: identity.Via(via)}
	res, err := s.upgrade(ctx, c, UpgradeRequest{ID: recipeID, Mode: mode, Force: force})
	status, count := "done", (*int)(nil)
	var msg *string
	switch {
	case errors.Is(err, ErrModelsUnavailable), ctx.Err() != nil:
		// Not the recipe's fault: put it back and wait.
		_ = s.st.Write(context.WithoutCancel(ctx), func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `UPDATE upgrade_job_items SET status = 'pending' WHERE job_id = ? AND position = ?`, jobID, pos)
			return err
		})
		if ctx.Err() == nil {
			s.log.Warn("upgrade worker: models unreachable; waiting", "error", err.Error())
		}
		return false, time.Minute
	case hasCode(err, CodeExists), hasCode(err, CodeDeleted), hasCode(err, CodeNotFound):
		status, msg = "skipped", ref(err.Error())
	case err != nil:
		status, msg = "failed", ref(err.Error())
		s.log.Warn("upgrade worker: recipe failed", "recipe", recipeID, "error", err.Error())
	default:
		n := res.Graph.ReviewCount
		count = &n
		if n > 0 || !res.Graph.Check.Valid {
			status = "needs_review"
		}
	}
	now := s.stamp()
	err = s.st.Write(context.WithoutCancel(ctx), func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE upgrade_job_items SET status = ?, review_count = ?, error = ?, finished_at = ?
			WHERE job_id = ? AND position = ?`, status, count, msg, now, jobID, pos); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE upgrade_jobs SET status = 'done', finished_at = ?
			WHERE id = ? AND status = 'running' AND NOT EXISTS (
				SELECT 1 FROM upgrade_job_items WHERE job_id = ? AND status IN ('pending', 'running'))`, now, jobID, jobID)
		return err
	})
	if err != nil {
		s.log.Warn("upgrade worker: recording a result failed", "error", err.Error())
	}
	return true, 0
}

func hasCode(err error, code string) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == code
}

// Review.

// ReviewQuery narrows the review queue.
type ReviewQuery struct {
	RecipeID string `json:"recipe_id,omitempty" query:"recipe_id" jsonschema:"only this recipe"`
	MealID   string `json:"meal_id,omitempty" query:"meal_id" jsonschema:"only the dishes on this meal"`
	Field    string `json:"field,omitempty" query:"field" jsonschema:"only this field: action, duration, attention, check_interval_min, interruptible, equipment, hold or timing_anchor"`
	Limit    int    `json:"limit,omitempty" query:"limit" jsonschema:"at most this many; default 20, at most 100"`
	Cursor   string `json:"cursor,omitempty" query:"cursor" jsonschema:"from the last page's next_cursor"`
}

// ReviewItem is one label to confirm or correct.
type ReviewItem struct {
	RecipeID     string  `json:"recipe_id"`
	RecipeTitle  string  `json:"recipe_title"`
	GraphVersion int     `json:"graph_version" jsonschema:"what resolve_review_items needs"`
	NodeID       string  `json:"node_id"`
	NodeText     string  `json:"node_text"`
	Quote        string  `json:"quote" jsonschema:"the recipe's words the action comes from; empty for an implied action"`
	Implied      bool    `json:"implied"`
	Field        string  `json:"field"`
	Critical     bool    `json:"critical" jsonschema:"whether the field decides when things happen: attention, hold, timing_anchor or equipment"`
	Value        any     `json:"value" jsonschema:"the label's value, in the field's own shape"`
	Source       string  `json:"source"`
	Confidence   float64 `json:"confidence"`
	Alternatives any     `json:"alternatives" jsonschema:"other answers weighed, most likely first, each {value, probability}; null for none"`
}

// ReviewPage is a page of the review queue.
type ReviewPage struct {
	Items      []ReviewItem `json:"items"`
	Remaining  int          `json:"remaining" jsonschema:"labels to review in all that match, this page included"`
	NextCursor *string      `json:"next_cursor"`
}

// reviewWhere is the SQL for a label that needs review.
var reviewWhere = reviewSQL()

// ReviewItems lists labels to review across recipes, most consequential
// first: dishes on meals still to come, then the caller's favourites, then
// the rest; within those the fields that decide timing, least sure first.
func (s *Service) ReviewItems(ctx context.Context, c identity.Caller, q ReviewQuery) (ReviewPage, error) {
	limit := q.Limit
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 {
		return ReviewPage{}, Errorf(CodeInvalid, "limit must be 1 to 100")
	}
	offset := 0
	if q.Cursor != "" {
		cur, err := decodeCursor(q.Cursor, "review")
		if err != nil {
			return ReviewPage{}, err
		}
		offset, _ = strconv.Atoi(cur.Key)
	}
	where := []string{reviewWhere}
	var args []any
	if q.RecipeID != "" {
		where = append(where, "l.recipe_id = ?")
		args = append(args, q.RecipeID)
	}
	if q.MealID != "" {
		where = append(where, "l.recipe_id IN (SELECT recipe_id FROM meal_items WHERE meal_id = ?)")
		args = append(args, q.MealID)
	}
	if q.Field != "" {
		where = append(where, "l.field = ?")
		args = append(args, q.Field)
	}
	from := `FROM graph_labels l
		JOIN recipe_graphs g ON g.recipe_id = l.recipe_id AND g.recipe_version = l.recipe_version AND g.deleted_at IS NULL
		JOIN graph_nodes n ON n.recipe_id = l.recipe_id AND n.recipe_version = l.recipe_version AND n.id = l.node_id
		JOIN recipes r ON r.id = l.recipe_id AND r.deleted_at IS NULL
		WHERE ` + strings.Join(where, " AND ")
	page := ReviewPage{Items: []ReviewItem{}}
	err := s.st.Read(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT count(*) "+from, args...).Scan(&page.Remaining); err != nil {
			return err
		}
		order := ` ORDER BY
			EXISTS (SELECT 1 FROM meal_items mi JOIN meals m ON m.id = mi.meal_id
				WHERE mi.recipe_id = l.recipe_id AND m.deleted_at IS NULL AND m.serve_at >= ?) DESC,
			EXISTS (SELECT 1 FROM person_recipe pr WHERE pr.recipe_id = l.recipe_id AND pr.person_id = ? AND pr.favorite) DESC,
			l.field IN ('attention', 'hold', 'timing_anchor', 'equipment') DESC,
			l.confidence, r.title COLLATE NOCASE, l.recipe_id, n.position, l.field
			LIMIT ? OFFSET ?`
		rows, err := tx.QueryContext(ctx, `SELECT l.recipe_id, r.title, g.version, l.node_id, n.text, n.quote, n.implied, l.field,
			l.value, l.source, l.confidence, l.alternatives `+from+order,
			append(args, s.stamp(), c.Username(), limit, offset)...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var it ReviewItem
			var value string
			var alts *string
			if err := rows.Scan(&it.RecipeID, &it.RecipeTitle, &it.GraphVersion, &it.NodeID, &it.NodeText, &it.Quote, &it.Implied,
				&it.Field, &value, &it.Source, &it.Confidence, &alts); err != nil {
				rows.Close()
				return err
			}
			it.Critical = slices.Contains(criticalFields, it.Field)
			_ = json.Unmarshal([]byte(value), &it.Value)
			if alts != nil {
				_ = json.Unmarshal([]byte(*alts), &it.Alternatives)
			}
			page.Items = append(page.Items, it)
		}
		return closeRows(rows)
	})
	if err != nil {
		return ReviewPage{}, err
	}
	if offset+len(page.Items) < page.Remaining {
		page.NextCursor = encodeCursor(cursor{Kind: "review", Key: strconv.Itoa(offset + len(page.Items))})
	}
	return page, nil
}

// ResolveRequest confirms or corrects labels of one recipe's graph.
type ResolveRequest struct {
	RecipeID     string          `json:"recipe_id"`
	GraphVersion int             `json:"graph_version" jsonschema:"from list_review_items or get_recipe_graph; a stale one is refused"`
	Items        []ResolveChoice `json:"items"`
	Accept       bool            `json:"accept,omitempty" jsonschema:"also save the graph as verified, which needs nothing left to review and no errors"`
}

// ResolveChoice is one label's answer: the value to keep, in the field's
// own shape. Sending the value it has confirms it.
type ResolveChoice struct {
	NodeID string `json:"node_id"`
	Field  string `json:"field"`
	Value  any    `json:"value" jsonschema:"the value in the field's own shape: a string for action, attention and timing_anchor; {min, typical, max} for duration; a number for check_interval_min; true or false for interruptible; a list of claims for equipment; {max_min, where, …} for hold"`
}

// ResolveResult is the graph after review.
type ResolveResult struct {
	Graph     GraphView `json:"graph"`
	Remaining int       `json:"remaining" jsonschema:"labels on this graph still to review"`
}

// ResolveReview sets the chosen labels as the caller's own, reviewed by
// them, and records each one that differs from what a model said, to learn
// from.
func (s *Service) ResolveReview(ctx context.Context, c identity.Caller, in ResolveRequest) (ResolveResult, error) {
	who, err := author(c)
	if err != nil {
		return ResolveResult{}, err
	}
	if len(in.Items) == 0 && !in.Accept {
		return ResolveResult{}, Errorf(CodeInvalid, "items: send at least one, or accept")
	}
	if len(in.Items) > maxGraphNodes {
		return ResolveResult{}, Errorf(CodeInvalid, "items: at most %d", maxGraphNodes)
	}
	var out ResolveResult
	err = s.st.Write(ctx, func(tx *sql.Tx) error {
		r, err := loadRecipe(ctx, tx, in.RecipeID, who)
		if err != nil {
			return err
		}
		if r.DeletedAt != nil {
			return Errorf(CodeDeleted, "this recipe is in the trash; restore it first")
		}
		g, err := loadGraph(ctx, tx, in.RecipeID)
		if err != nil {
			return err
		}
		if g == nil {
			return Errorf(CodeNotFound, "this recipe has no step graph")
		}
		if g.Version != in.GraphVersion {
			cur, err := graphView(ctx, tx, in.RecipeID, who)
			if err != nil {
				return err
			}
			e := Errorf(CodeStale, "someone changed the step graph since you read it")
			e.CurrentGraph = &cur
			return e
		}
		prev := *g
		prev.Nodes = slices.Clone(g.Nodes)
		if markStale(g, r) {
			return Errorf(CodeInvalid, "the recipe's words have changed under this graph; upgrade it again before reviewing")
		}
		idx := map[string]int{}
		for i, n := range g.Nodes {
			idx[n.ID] = i
		}
		stamp := map[graphLabelRef]bool{}
		now := s.stamp()
		for _, ch := range in.Items {
			i, ok := idx[ch.NodeID]
			if !ok {
				return Errorf(CodeInvalid, "no node %s in this graph", ch.NodeID)
			}
			if !slices.Contains(labelFields, ch.Field) {
				return Errorf(CodeInvalid, "field must be one of %s", strings.Join(labelFields, ", "))
			}
			n := &g.Nodes[i]
			// What it was, by value: the label's fields are about to change.
			var had bool
			var beforeJSON, beforeSource string
			var beforeConf float64
			for _, l := range labelsOf(n) {
				if l.field == ch.Field {
					had, beforeJSON, beforeSource, beforeConf = true, l.json(), *l.source, 1
					if *l.conf != nil {
						beforeConf = **l.conf
					}
				}
			}
			b, err := json.Marshal(ch.Value)
			if err != nil {
				return Errorf(CodeInvalid, "%s.%s: %v", ch.NodeID, ch.Field, err)
			}
			if err := fillLabel(n, ch.Field, string(b), "user", 1, nil, nil, nil); err != nil {
				return Errorf(CodeInvalid, "%s.%s: the value isn't the field's shape: %v", ch.NodeID, ch.Field, err)
			}
			stamp[graphLabelRef{n.ID, ch.Field}] = true
			var afterJSON string
			for _, l := range labelsOf(n) {
				if l.field == ch.Field {
					afterJSON = l.json()
				}
			}
			if had && beforeJSON == afterJSON {
				// Confirmed: keep its source and confidence, only reviewed.
				if err := fillLabel(n, ch.Field, beforeJSON, beforeSource, beforeConf, nil, nil, nil); err != nil {
					return err
				}
				continue
			}
			if had && (beforeSource == "jev" || beforeSource == "model") {
				if _, err := tx.ExecContext(ctx, `INSERT INTO review_corrections (id, at, person, recipe_id, node_id, node_text, field,
					model_value, model_source, confidence, user_value) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					uuid.NewV7().String(), now, who, r.ID, n.ID, cmpOr(n.Source.Quote, n.Text), ch.Field,
					beforeJSON, beforeSource, beforeConf, afterJSON); err != nil {
					return err
				}
			}
		}
		in2 := GraphInput{Status: g.Status, ServeItemID: g.ServeItemID, Nodes: g.Nodes, Items: g.Items}
		if in2.Status == GraphStale {
			in2.Status = GraphNeedsReview
		}
		if in.Accept {
			left := 0
			for i := range g.Nodes {
				for _, l := range labelsOf(&g.Nodes[i]) {
					if !stamp[graphLabelRef{g.Nodes[i].ID, l.field}] && needsReview(g.Nodes[i], l) {
						left++
					}
				}
			}
			if left > 0 {
				return Errorf(CodeInvalid, "%d labels still need review before this graph can be accepted", left)
			}
			in2.Status = GraphVerified
		}
		summary := fmt.Sprintf("Reviewed %d labels of %s's step graph", len(in.Items), r.Title)
		if err := s.saveGraph(ctx, tx, c, r, &prev, in2, stamp, summary); err != nil {
			return err
		}
		out.Graph, err = graphView(ctx, tx, in.RecipeID, who)
		out.Remaining = out.Graph.ReviewCount
		return err
	})
	return out, err
}

var labelFields = []string{"action", "duration", "attention", "check_interval_min", "interruptible", "equipment", "hold", "timing_anchor"}
