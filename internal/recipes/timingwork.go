package recipes

import (
	"context"
	"database/sql"
	"strings"

	"github.com/sroberts/sous/internal/identity"
)

// Filling in step timing for recipes written before meal plans: which
// recipes still need it, a recipe's steps beside Sous's own reading of
// them, and a way to set timing that never touches a step's words. An
// assistant over MCP does the reading a regex can't.

// TimingTodo is a recipe with steps that have no timing yet.
type TimingTodo struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Version int    `json:"version"`
	Steps   int    `json:"steps" jsonschema:"how many steps it has"`
	Untimed int    `json:"untimed" jsonschema:"how many of them have no timing"`
}

// TimingTodos is a batch of recipes needing timing.
type TimingTodos struct {
	Items     []TimingTodo `json:"items"`
	Remaining int          `json:"remaining" jsonschema:"how many recipes need timing in all, this batch included"`
}

// TimingTodoQuery narrows RecipesNeedingTiming.
type TimingTodoQuery struct {
	MealID string `json:"meal_id,omitempty" jsonschema:"only the dishes on this meal's menu"`
	Limit  int    `json:"limit,omitempty" jsonschema:"at most this many recipes (default 20, at most 100)"`
}

// StepTimingRow is one step with its timing and Sous's guess.
type StepTimingRow struct {
	Number  int         `json:"number" jsonschema:"the step's number, from 1"`
	Section *string     `json:"section"`
	Text    string      `json:"text"`
	Timing  *StepTiming `json:"timing" jsonschema:"the timing set on the step, or null"`
	Guess   StepTiming  `json:"guess" jsonschema:"what Sous reads in the step's words: a starting point, often wrong about what needs the cook or what can be made ahead"`
}

// StepTimingView is a recipe's steps as meal planning sees them.
type StepTimingView struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	Version   int             `json:"version" jsonschema:"what set_step_timing needs"`
	YieldText *string         `json:"yield_text"`
	Notes     *string         `json:"notes"`
	Steps     []StepTimingRow `json:"steps"`
}

// RecipesNeedingTiming lists recipes, in the library or on one meal, with
// any step that has no timing, by title.
func (s *Service) RecipesNeedingTiming(ctx context.Context, c identity.Caller, q TimingTodoQuery) (TimingTodos, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		return TimingTodos{}, Errorf(CodeInvalid, "limit must be at most 100")
	}
	where := "r.deleted_at IS NULL"
	var args []any
	if q.MealID != "" {
		where += " AND r.id IN (SELECT recipe_id FROM meal_items WHERE meal_id = ?)"
		args = append(args, q.MealID)
	}
	out := TimingTodos{Items: []TimingTodo{}}
	err := s.st.Read(ctx, func(tx *sql.Tx) error {
		if q.MealID != "" {
			if _, err := loadMeal(ctx, tx, q.MealID); err != nil {
				return err
			}
		}
		base := `SELECT r.id, r.title, r.version, count(*), sum(st.timing IS NULL)
			FROM recipes r JOIN recipe_steps st ON st.recipe_id = r.id
			WHERE ` + where + ` GROUP BY r.id HAVING sum(st.timing IS NULL) > 0`
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM ("+base+")", args...).Scan(&out.Remaining); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, base+" ORDER BY r.title COLLATE NOCASE, r.id LIMIT ?", append(args, limit)...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var t TimingTodo
			if err := rows.Scan(&t.ID, &t.Title, &t.Version, &t.Steps, &t.Untimed); err != nil {
				return err
			}
			out.Items = append(out.Items, t)
		}
		return rows.Err()
	})
	return out, err
}

// StepTiming returns a recipe's steps with their timing and Sous's guess.
func (s *Service) StepTiming(ctx context.Context, c identity.Caller, id string) (StepTimingView, error) {
	r, err := s.Get(ctx, c, id)
	if err != nil {
		return StepTimingView{}, err
	}
	return timingView(r), nil
}

// SetStepTiming replaces a recipe's step timing, one entry per step (null
// clears one), leaving everything else, the steps' words included, as it is.
func (s *Service) SetStepTiming(ctx context.Context, c identity.Caller, id string, version int, timing []*StepTiming) (StepTimingView, error) {
	// One entry per step, exactly: a short list would quietly clear the rest.
	cur, err := s.Get(ctx, c, id)
	if err != nil {
		return StepTimingView{}, err
	}
	if len(timing) != len(cur.Steps) {
		return StepTimingView{}, Errorf(CodeInvalid, "timing has %d entries, but the recipe has %d steps; send one per step, null for none", len(timing), len(cur.Steps))
	}
	r, err := s.Update(ctx, c, id, version, RecipePatch{StepTiming: Some(timing)})
	if err != nil {
		return StepTimingView{}, err
	}
	return timingView(r), nil
}

func timingView(r Recipe) StepTimingView {
	texts := make([]string, len(r.Steps))
	for i, st := range r.Steps {
		texts[i] = st.Text
	}
	guess := SuggestTimings(texts)
	v := StepTimingView{ID: r.ID, Title: r.Title, Version: r.Version, YieldText: r.YieldText, Notes: r.Notes, Steps: make([]StepTimingRow, len(r.Steps))}
	for i, st := range r.Steps {
		v.Steps[i] = StepTimingRow{Number: i + 1, Section: st.Section, Text: strings.TrimSpace(st.Text), Timing: st.Timing, Guess: guess[i]}
	}
	return v
}
