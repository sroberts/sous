package recipes

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // a meal's timezone must load on a host without zoneinfo
	"uuid"

	"github.com/sroberts/sous/internal/identity"
)

// Meal is several recipes cooked for one sitting: a holiday dinner, say.
type Meal struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	ServeAt   string     `json:"serve_at" jsonschema:"when the meal is served (dishes with a serve offset come after), UTC RFC 3339"`
	Timezone  string     `json:"timezone" jsonschema:"IANA timezone of the kitchen, such as America/New_York: where its day begins and ends"`
	Cooks     int        `json:"cooks" jsonschema:"how many people can do hands-on work at once"`
	PrepDays  int        `json:"prep_days" jsonschema:"how many days before the meal make-ahead work may start"`
	Notes     *string    `json:"notes"`
	Items     []MealItem `json:"items" jsonschema:"the menu, in order"`
	Version   int        `json:"version" jsonschema:"incremented on every change; send it back as If-Match or version"`
	DeletedAt *string    `json:"deleted_at"`
	CreatedAt string     `json:"created_at"`
	CreatedBy string     `json:"created_by"`
	UpdatedAt string     `json:"updated_at"`
	UpdatedBy string     `json:"updated_by"`
}

// MealItem is one dish on a meal's menu.
type MealItem struct {
	ID                 string  `json:"id" jsonschema:"stays the same across edits; tasks in the plan are keyed by it"`
	RecipeID           string  `json:"recipe_id"`
	Title              string  `json:"title" jsonschema:"the recipe's title; read-only"`
	ServeOffsetMinutes int     `json:"serve_offset_minutes" jsonschema:"minutes after serve_at this dish is served (dessert at 90, say); 0 with the meal; negative before it (an appetiser)"`
	Dish               *string `json:"dish" jsonschema:"the serving dish it goes to the table in, by its name in the kitchen"`
}

// MealSummary is a meal as GET /v1/meals lists it.
type MealSummary struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	ServeAt   string `json:"serve_at"`
	Timezone  string `json:"timezone"`
	Items     int    `json:"items" jsonschema:"how many dishes are on the menu"`
	Version   int    `json:"version"`
	UpdatedAt string `json:"updated_at"`
}

// MealInput is the body of POST /v1/meals.
type MealInput struct {
	Title    string          `json:"title"`
	ServeAt  string          `json:"serve_at" jsonschema:"when the meal is served, RFC 3339 with an offset, e.g. 2026-11-26T17:00:00-05:00"`
	Timezone string          `json:"timezone" jsonschema:"IANA timezone, such as America/New_York"`
	Cooks    *int            `json:"cooks,omitempty" jsonschema:"1 to 8; default 1"`
	PrepDays *int            `json:"prep_days,omitempty" jsonschema:"0 to 7; default 1"`
	Notes    *string         `json:"notes,omitempty"`
	Items    []MealItemInput `json:"items,omitempty"`
}

// MealItemInput is one dish in a MealInput or MealPatch.
type MealItemInput struct {
	ID                 *string `json:"id,omitempty" jsonschema:"an existing item's id, to keep its progress; omit for a new dish"`
	RecipeID           string  `json:"recipe_id"`
	ServeOffsetMinutes int     `json:"serve_offset_minutes,omitempty" jsonschema:"minutes after serve_at, -180 (before the meal) to 720"`
	Dish               *string `json:"dish,omitempty" jsonschema:"serving dish, by its name in the kitchen"`
}

// MealPatch is the body of PATCH /v1/meals/{id}. Absent fields are left
// alone; sending items replaces the whole menu.
type MealPatch struct {
	Title    Optional[string]          `json:"title,omitzero"`
	ServeAt  Optional[string]          `json:"serve_at,omitzero" jsonschema:"RFC 3339 with an offset"`
	Timezone Optional[string]          `json:"timezone,omitzero"`
	Cooks    Optional[int]             `json:"cooks,omitzero"`
	PrepDays Optional[int]             `json:"prep_days,omitzero"`
	Notes    Optional[string]          `json:"notes,omitzero"`
	Items    Optional[[]MealItemInput] `json:"items,omitzero" jsonschema:"replaces the whole menu"`
}

// TaskPatch is the body of PUT /v1/meals/{id}/tasks/{task}. Absent fields
// are left alone.
type TaskPatch struct {
	PinnedStart Optional[string] `json:"pinned_start,omitzero" jsonschema:"RFC 3339: fix the task to start then, and plan around it; null unpins"`
	Started     Optional[bool]   `json:"started,omitzero" jsonschema:"true starts the task now (its timer runs from here); false takes it back"`
	Done        Optional[bool]   `json:"done,omitzero" jsonschema:"true ticks it off now; false takes it back"`
}

// PlanQuery is GET /v1/meals/{id}/plan.
type PlanQuery struct {
	Now string `query:"now" json:"now,omitempty" jsonschema:"plan as if it were this time (RFC 3339) instead of now"`
}

// Limits on a meal.
const (
	maxMealItems   = 40
	maxServeOffset = 12 * 60
	minServeOffset = -3 * 60 // an appetiser before the meal
	// maxMealSteps bounds the steps across a menu, so one plan (and the
	// alternatives a late plan is checked against) is a bounded amount of
	// work: the scheduler's cost grows much faster than the task count.
	maxMealSteps = 600
	// maxPinDays is how far from serving a task may be pinned: the prep
	// window and the days before it the planner looks back over.
	maxPinDays = 15
)

func tooManySteps(n int) *Error {
	return Errorf(CodeInvalid, "the menu has %d steps in all; a meal can plan at most %d", n, maxMealSteps)
}

func mealNotFound() *Error { return Errorf(CodeNotFound, "that meal does not exist") }

// ListMeals returns live meals, soonest first.
func (s *Service) ListMeals(ctx context.Context, c identity.Caller) ([]MealSummary, error) {
	out := []MealSummary{}
	err := s.st.Read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT m.id, m.title, m.serve_at, m.timezone,
			(SELECT count(*) FROM meal_items i WHERE i.meal_id = m.id), m.version, m.updated_at
			FROM meals m WHERE m.deleted_at IS NULL ORDER BY m.serve_at, m.id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m MealSummary
			if err := rows.Scan(&m.ID, &m.Title, &m.ServeAt, &m.Timezone, &m.Items, &m.Version, &m.UpdatedAt); err != nil {
				return err
			}
			out = append(out, m)
		}
		return rows.Err()
	})
	return out, err
}

// GetMeal returns a meal; a deleted one has DeletedAt set.
func (s *Service) GetMeal(ctx context.Context, c identity.Caller, id string) (Meal, error) {
	var m Meal
	err := s.st.Read(ctx, func(tx *sql.Tx) error {
		var err error
		m, err = loadMeal(ctx, tx, id)
		return err
	})
	return m, err
}

// CreateMeal stores a new meal at version 1.
func (s *Service) CreateMeal(ctx context.Context, c identity.Caller, in MealInput) (Meal, error) {
	who, err := author(c)
	if err != nil {
		return Meal{}, err
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return Meal{}, Errorf(CodeInvalid, "title is required")
	}
	serve, err := parseServeAt(in.ServeAt)
	if err != nil {
		return Meal{}, err
	}
	if err := checkTimezone(in.Timezone); err != nil {
		return Meal{}, err
	}
	cooks, prep := 1, 1
	if in.Cooks != nil {
		cooks = *in.Cooks
	}
	if in.PrepDays != nil {
		prep = *in.PrepDays
	}
	if err := checkMealCounts(cooks, prep); err != nil {
		return Meal{}, err
	}
	now := s.stamp()
	id := uuid.NewV7().String()
	var out Meal
	err = s.st.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO meals (id, title, serve_at, timezone, cooks, prep_days, notes,
			created_at, created_by, updated_at, updated_by) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, title, serve, in.Timezone, cooks, prep, blankNil(in.Notes), now, who, now, who); err != nil {
			return err
		}
		if err := setMealItems(ctx, tx, id, in.Items); err != nil {
			return err
		}
		if err := s.logChange(ctx, tx, c, now, "meal", id, "create", "Planned "+title); err != nil {
			return err
		}
		out, err = loadMeal(ctx, tx, id)
		return err
	})
	return out, err
}

// UpdateMeal applies patch if the meal is still at version.
func (s *Service) UpdateMeal(ctx context.Context, c identity.Caller, id string, version int, p MealPatch) (Meal, error) {
	who, err := author(c)
	if err != nil {
		return Meal{}, err
	}
	var sets []string
	var args []any
	add := func(col string, v any) {
		sets = append(sets, col+" = ?")
		args = append(args, v)
	}
	if p.Title.Set {
		t := strings.TrimSpace(p.Title.Value)
		if t == "" {
			return Meal{}, Errorf(CodeInvalid, "title is required and can't be cleared")
		}
		add("title", t)
	}
	if p.ServeAt.Set {
		serve, err := parseServeAt(p.ServeAt.Value)
		if err != nil {
			return Meal{}, err
		}
		add("serve_at", serve)
	}
	if p.Timezone.Set {
		if err := checkTimezone(p.Timezone.Value); err != nil {
			return Meal{}, err
		}
		add("timezone", p.Timezone.Value)
	}
	if p.Cooks.Set {
		if p.Cooks.Null || checkMealCounts(p.Cooks.Value, 1) != nil {
			return Meal{}, Errorf(CodeInvalid, "cooks must be between 1 and 8")
		}
		add("cooks", p.Cooks.Value)
	}
	if p.PrepDays.Set {
		if p.PrepDays.Null || checkMealCounts(1, p.PrepDays.Value) != nil {
			return Meal{}, Errorf(CodeInvalid, "prep_days must be between 0 and 7")
		}
		add("prep_days", p.PrepDays.Value)
	}
	if p.Notes.Set {
		add("notes", blankNil(p.Notes.Ptr()))
	}
	now := s.stamp()
	var out Meal
	err = s.st.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE meals SET version = version + 1, updated_at = ?, updated_by = ?
			WHERE id = ? AND version = ? AND deleted_at IS NULL`, now, who, id, version)
		if err != nil {
			return err
		}
		if err := checkMealClaimed(ctx, tx, res, id); err != nil {
			return err
		}
		if len(sets) > 0 {
			if _, err := tx.ExecContext(ctx, "UPDATE meals SET "+strings.Join(sets, ", ")+" WHERE id = ?", append(args, id)...); err != nil {
				return err
			}
		}
		if p.Items.Set {
			if err := setMealItems(ctx, tx, id, p.Items.Value); err != nil {
				return err
			}
		}
		if out, err = loadMeal(ctx, tx, id); err != nil {
			return err
		}
		return s.logChange(ctx, tx, c, now, "meal", id, "update", "Edited the meal "+out.Title)
	})
	return out, err
}

// DeleteMeal moves a meal to the trash.
func (s *Service) DeleteMeal(ctx context.Context, c identity.Caller, id string, version int) error {
	who, err := author(c)
	if err != nil {
		return err
	}
	now := s.stamp()
	return s.st.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE meals SET version = version + 1, deleted_at = ?, updated_at = ?, updated_by = ?
			WHERE id = ? AND version = ? AND deleted_at IS NULL`, now, now, who, id, version)
		if err != nil {
			return err
		}
		if err := checkMealClaimed(ctx, tx, res, id); err != nil {
			return err
		}
		var title string
		if err := tx.QueryRowContext(ctx, `SELECT title FROM meals WHERE id = ?`, id).Scan(&title); err != nil {
			return err
		}
		return s.logChange(ctx, tx, c, now, "meal", id, "delete", "Deleted the meal "+title)
	})
}

// RestoreMeal takes a meal out of the trash; one that isn't deleted is
// returned unchanged.
func (s *Service) RestoreMeal(ctx context.Context, c identity.Caller, id string) (Meal, error) {
	who, err := author(c)
	if err != nil {
		return Meal{}, err
	}
	now := s.stamp()
	var out Meal
	err = s.st.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE meals SET version = version + 1, deleted_at = NULL, updated_at = ?, updated_by = ?
			WHERE id = ? AND deleted_at IS NOT NULL`, now, who, id)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if out, err = loadMeal(ctx, tx, id); err != nil || n == 0 {
			return err
		}
		return s.logChange(ctx, tx, c, now, "meal", id, "restore", "Restored the meal "+out.Title)
	})
	return out, err
}

// UpdateTask records progress on one task of a meal, or pins it, and
// returns the plan as it now stands. Progress isn't in the change log: it
// is the state of a cook, not a change to the menu.
func (s *Service) UpdateTask(ctx context.Context, c identity.Caller, mealID, task string, p TaskPatch) (Plan, error) {
	who, err := author(c)
	if err != nil {
		return Plan{}, err
	}
	var pinned *string
	var pinAt time.Time
	if p.PinnedStart.Set && !p.PinnedStart.Null {
		t, err := time.Parse(time.RFC3339, p.PinnedStart.Value)
		if err != nil {
			return Plan{}, Errorf(CodeInvalid, "pinned_start must be an RFC 3339 time")
		}
		pinAt = t
		pinned = ref(t.UTC().Format(timeFormat))
	}
	now := s.now().UTC()
	stamp := now.Format(timeFormat)
	err = s.st.Write(ctx, func(tx *sql.Tx) error {
		m, err := loadMeal(ctx, tx, mealID)
		if err != nil {
			return err
		}
		if m.DeletedAt != nil {
			return Errorf(CodeDeleted, "that meal is in the trash; restore it first")
		}
		if !taskKnown(m, task) {
			return Errorf(CodeNotFound, "the meal has no task %q", task)
		}
		if pinned != nil {
			serve, err := time.Parse(time.RFC3339, m.ServeAt)
			if err != nil {
				return err
			}
			if d := pinAt.Sub(serve); d < -maxPinDays*24*time.Hour || d > maxPinDays*24*time.Hour {
				return Errorf(CodeInvalid, "pinned_start must be within %d days of the meal", maxPinDays)
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO meal_tasks (meal_id, task, updated_at) VALUES (?, ?, ?)
			ON CONFLICT (meal_id, task) DO NOTHING`, mealID, task, stamp); err != nil {
			return err
		}
		var sets []string
		var args []any
		add := func(q string, a ...any) {
			sets = append(sets, q)
			args = append(args, a...)
		}
		if p.PinnedStart.Set {
			add("pinned_start = ?", pinned)
		}
		if p.Started.Set {
			if p.Started.Value {
				add("started_at = coalesce(started_at, ?)", stamp)
			} else {
				add("started_at = NULL, done_at = NULL, done_by = NULL")
			}
		}
		if p.Done.Set {
			if p.Done.Value {
				add("done_at = coalesce(done_at, ?), done_by = coalesce(done_by, ?)", stamp, who)
			} else {
				add("done_at = NULL, done_by = NULL")
			}
		}
		add("updated_at = ?", stamp)
		_, err = tx.ExecContext(ctx, "UPDATE meal_tasks SET "+strings.Join(sets, ", ")+" WHERE meal_id = ? AND task = ?",
			append(args, mealID, task)...)
		return err
	})
	if err != nil {
		return Plan{}, err
	}
	return s.MealPlan(ctx, c, mealID, now)
}

// ResetProgress clears what has been started and ticked off on a meal,
// keeping pins, so it can be cooked again.
func (s *Service) ResetProgress(ctx context.Context, c identity.Caller, mealID string) (Plan, error) {
	if _, err := author(c); err != nil {
		return Plan{}, err
	}
	err := s.st.Write(ctx, func(tx *sql.Tx) error {
		m, err := loadMeal(ctx, tx, mealID)
		if err != nil {
			return err
		}
		if m.DeletedAt != nil {
			return Errorf(CodeDeleted, "that meal is in the trash; restore it first")
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM meal_tasks WHERE meal_id = ? AND pinned_start IS NULL`, mealID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE meal_tasks SET started_at = NULL, done_at = NULL, done_by = NULL, updated_at = ?
			WHERE meal_id = ?`, s.stamp(), mealID)
		return err
	})
	if err != nil {
		return Plan{}, err
	}
	return s.MealPlan(ctx, c, mealID, s.now())
}

// taskKnown reports whether task names a step (or the reheat) of an item:
// "<item>:<step index>" or "<item>:reheat", or a preheat for one of those.
// Anything else would only leave a row the plan never reads.
func taskKnown(m Meal, task string) bool {
	// A preheat is "preheat:<oven>:<the task it heats for>".
	if after, ok := strings.CutPrefix(task, "preheat:"); ok {
		oven, heated, ok := strings.Cut(after, ":")
		if n, err := strconv.Atoi(oven); !ok || err != nil || n < 0 || n >= maxOvens || strconv.Itoa(n) != oven {
			return false
		}
		task = heated
	}
	item, rest, ok := strings.Cut(task, ":")
	if !ok {
		return false
	}
	if rest != "reheat" {
		n, err := strconv.Atoi(rest)
		if err != nil || n < 0 || n >= maxMealSteps || strconv.Itoa(n) != rest {
			return false
		}
	}
	for _, it := range m.Items {
		if it.ID == item {
			return true
		}
	}
	return false
}

func checkMealClaimed(ctx context.Context, tx *sql.Tx, res sql.Result, id string) error {
	n, err := res.RowsAffected()
	if err != nil || n > 0 {
		return err
	}
	cur, err := loadMeal(ctx, tx, id)
	if err != nil {
		return err
	}
	if cur.DeletedAt != nil {
		return Errorf(CodeDeleted, "that meal is in the trash; restore it first")
	}
	e := Errorf(CodeStale, "someone changed this meal since you opened it")
	e.CurrentMeal = &cur
	return e
}

func setMealItems(ctx context.Context, tx *sql.Tx, mealID string, items []MealItemInput) error {
	if len(items) > maxMealItems {
		return Errorf(CodeInvalid, "items: at most %d dishes", maxMealItems)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM meal_items WHERE meal_id = ?`, mealID); err != nil {
		return err
	}
	seen := map[string]bool{}
	steps := 0
	for i, it := range items {
		if it.ServeOffsetMinutes < minServeOffset || it.ServeOffsetMinutes > maxServeOffset {
			return Errorf(CodeInvalid, "items[%d]: serve_offset_minutes must be between %d and %d", i, minServeOffset, maxServeOffset)
		}
		var n int
		err := tx.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM recipe_steps s WHERE s.recipe_id = r.id)
			FROM recipes r WHERE r.id = ?`, it.RecipeID).Scan(&n)
		if errors.Is(err, sql.ErrNoRows) {
			return Errorf(CodeInvalid, "items[%d]: no recipe has id %q", i, it.RecipeID)
		} else if err != nil {
			return err
		}
		if steps += n; steps > maxMealSteps {
			return tooManySteps(steps)
		}
		id := ""
		if it.ID != nil {
			id = strings.TrimSpace(*it.ID)
		}
		if id == "" || seen[id] {
			id = uuid.NewV7().String()
		}
		if !itemID.MatchString(id) {
			return Errorf(CodeInvalid, "items[%d]: id must be at most 64 letters, digits, '-' or '_'", i)
		}
		seen[id] = true
		if _, err := tx.ExecContext(ctx, `INSERT INTO meal_items (meal_id, id, position, recipe_id, serve_offset_minutes, dish)
			VALUES (?, ?, ?, ?, ?, ?)`, mealID, id, i, it.RecipeID, it.ServeOffsetMinutes, blankNil(it.Dish)); err != nil {
			return err
		}
	}
	// Progress on a dish that left the menu goes with it. A preheat's key
	// starts "preheat:", so it is matched by the dish it heats for.
	menu := Meal{}
	for id := range seen {
		menu.Items = append(menu.Items, MealItem{ID: id})
	}
	rows, err := tx.QueryContext(ctx, `SELECT task FROM meal_tasks WHERE meal_id = ?`, mealID)
	if err != nil {
		return err
	}
	var gone []string
	for rows.Next() {
		var task string
		if err := rows.Scan(&task); err != nil {
			rows.Close()
			return err
		}
		if !taskKnown(menu, task) {
			gone = append(gone, task)
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, task := range gone {
		if _, err := tx.ExecContext(ctx, `DELETE FROM meal_tasks WHERE meal_id = ? AND task = ?`, mealID, task); err != nil {
			return err
		}
	}
	return nil
}

func loadMeal(ctx context.Context, tx *sql.Tx, id string) (Meal, error) {
	var m Meal
	err := tx.QueryRowContext(ctx, `SELECT id, title, serve_at, timezone, cooks, prep_days, notes, version, deleted_at,
		created_at, created_by, updated_at, updated_by FROM meals WHERE id = ?`, id).Scan(&m.ID, &m.Title, &m.ServeAt,
		&m.Timezone, &m.Cooks, &m.PrepDays, &m.Notes, &m.Version, &m.DeletedAt, &m.CreatedAt, &m.CreatedBy, &m.UpdatedAt, &m.UpdatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return Meal{}, mealNotFound()
	} else if err != nil {
		return Meal{}, err
	}
	m.Items = []MealItem{}
	rows, err := tx.QueryContext(ctx, `SELECT i.id, i.recipe_id, r.title, i.serve_offset_minutes, i.dish
		FROM meal_items i JOIN recipes r ON r.id = i.recipe_id WHERE i.meal_id = ? ORDER BY i.position`, id)
	if err != nil {
		return Meal{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var it MealItem
		if err := rows.Scan(&it.ID, &it.RecipeID, &it.Title, &it.ServeOffsetMinutes, &it.Dish); err != nil {
			return Meal{}, err
		}
		m.Items = append(m.Items, it)
	}
	return m, rows.Err()
}

// itemID is what a menu item's id may be: it goes into task keys and URLs.
var itemID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func parseServeAt(s string) (string, error) {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
	if err != nil {
		return "", Errorf(CodeInvalid, "serve_at must be an RFC 3339 time with an offset, e.g. 2026-11-26T17:00:00-05:00")
	}
	return t.UTC().Format(timeFormat), nil
}

func checkTimezone(tz string) error {
	if strings.TrimSpace(tz) == "" || tz == "Local" {
		return Errorf(CodeInvalid, "timezone must be an IANA name such as America/New_York")
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return Errorf(CodeInvalid, "timezone %q isn't one Sous knows; use an IANA name such as America/New_York", tz)
	}
	return nil
}

func checkMealCounts(cooks, prep int) error {
	if cooks < 1 || cooks > 8 {
		return Errorf(CodeInvalid, "cooks must be between 1 and 8")
	}
	if prep < 0 || prep > 7 {
		return Errorf(CodeInvalid, "prep_days must be between 0 and 7")
	}
	return nil
}

func blankNil(s *string) *string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	v := strings.TrimSpace(*s)
	return &v
}
