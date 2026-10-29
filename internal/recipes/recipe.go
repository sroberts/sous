package recipes

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"uuid"

	"github.com/sroberts/sous/internal/identity"
)

// Get returns the full recipe with the caller's Mine flags. A deleted recipe
// is returned with DeletedAt set; an unknown id is CodeNotFound.
func (s *Service) Get(ctx context.Context, c identity.Caller, id string) (Recipe, error) {
	var r Recipe
	err := s.st.Read(ctx, func(tx *sql.Tx) error {
		var err error
		r, err = loadRecipe(ctx, tx, id, c.Username())
		return err
	})
	return r, err
}

// Create stores a new recipe at version 1 with a UUIDv7 id, parsing
// ingredients and steps, deriving servings, creating unknown tags and
// checking image hashes exist. A blank title is CodeInvalid.
func (s *Service) Create(ctx context.Context, c identity.Caller, in RecipeInput) (Recipe, error) {
	who, err := author(c)
	if err != nil {
		return Recipe{}, err
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return Recipe{}, Errorf(CodeInvalid, "title is required")
	}
	if err := checkMinutes(map[string]*int{
		"prep_minutes": in.PrepMinutes, "cook_minutes": in.CookMinutes, "total_minutes": in.TotalMinutes,
	}); err != nil {
		return Recipe{}, err
	}
	id := uuid.NewV7().String()
	now := s.stamp()
	var out Recipe
	err = s.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO recipes (id, title, source_url, description, notes, yield_text, servings,
			prep_minutes, cook_minutes, total_minutes, time_text, nutrition_text, created_at, created_by, updated_at, updated_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, title, in.SourceURL, in.Description, in.Notes, in.YieldText, servingsOf(in.YieldText),
			in.PrepMinutes, in.CookMinutes, in.TotalMinutes, in.TimeText, in.NutritionText, now, who, now, who)
		if err != nil {
			return err
		}
		if err := setTags(ctx, tx, id, in.Tags, who, now); err != nil {
			return err
		}
		if err := setImages(ctx, tx, id, in.Images); err != nil {
			return err
		}
		if err := setIngredients(ctx, tx, id, ParseIngredientLines(in.Ingredients)); err != nil {
			return err
		}
		if err := setSteps(ctx, tx, id, ParseStepLines(in.Steps)); err != nil {
			return err
		}
		if err := s.logChange(ctx, tx, c, now, "recipe", id, "create", "Added "+title); err != nil {
			return err
		}
		out, err = loadRecipe(ctx, tx, id, who)
		return err
	})
	return out, err
}

// Update applies patch if the recipe is still at version, bumping it.
// A stale version is CodeStale with Current set (read in the same
// transaction); a deleted recipe is CodeDeleted. The change summary names
// the changed fields: "Edited Pad Thai: ingredients, steps".
func (s *Service) Update(ctx context.Context, c identity.Caller, id string, version int, patch RecipePatch) (Recipe, error) {
	who, err := author(c)
	if err != nil {
		return Recipe{}, err
	}
	sets, args, err := patchColumns(patch)
	if err != nil {
		return Recipe{}, err
	}
	now := s.stamp()
	var out Recipe
	err = s.st.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE recipes SET version = version + 1, updated_at = ?, updated_by = ?
			WHERE id = ? AND version = ? AND deleted_at IS NULL`, now, who, id, version)
		if err != nil {
			return err
		}
		if err := checkClaimed(ctx, tx, res, id, who); err != nil {
			return err
		}
		before, err := loadRecipe(ctx, tx, id, who)
		if err != nil {
			return err
		}
		if len(sets) > 0 {
			q := "UPDATE recipes SET " + strings.Join(sets, ", ") + " WHERE id = ?"
			if _, err := tx.ExecContext(ctx, q, append(args, id)...); err != nil {
				return err
			}
		}
		if patch.Tags.Set {
			if err := setTags(ctx, tx, id, patch.Tags.Value, who, now); err != nil {
				return err
			}
		}
		if patch.Images.Set {
			if err := setImages(ctx, tx, id, patch.Images.Value); err != nil {
				return err
			}
		}
		if patch.Ingredients.Set {
			if err := setIngredients(ctx, tx, id, ParseIngredientLines(patch.Ingredients.Value)); err != nil {
				return err
			}
		}
		if patch.Steps.Set {
			if err := setSteps(ctx, tx, id, ParseStepLines(patch.Steps.Value)); err != nil {
				return err
			}
		}
		out, err = loadRecipe(ctx, tx, id, who)
		if err != nil {
			return err
		}
		summary := "Edited " + out.Title
		if changed := changedFields(before, out); len(changed) > 0 {
			summary += ": " + strings.Join(changed, ", ")
		}
		return s.logChange(ctx, tx, c, now, "recipe", id, "update", summary)
	})
	return out, err
}

// Delete soft-deletes the recipe at version, setting deleted_at and bumping
// the version. Stale is CodeStale with Current; already deleted is
// CodeDeleted.
func (s *Service) Delete(ctx context.Context, c identity.Caller, id string, version int) error {
	who, err := author(c)
	if err != nil {
		return err
	}
	now := s.stamp()
	return s.st.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE recipes SET version = version + 1, deleted_at = ?, updated_at = ?, updated_by = ?
			WHERE id = ? AND version = ? AND deleted_at IS NULL`, now, now, who, id, version)
		if err != nil {
			return err
		}
		if err := checkClaimed(ctx, tx, res, id, who); err != nil {
			return err
		}
		var title string
		if err := tx.QueryRowContext(ctx, `SELECT title FROM recipes WHERE id = ?`, id).Scan(&title); err != nil {
			return err
		}
		return s.logChange(ctx, tx, c, now, "recipe", id, "delete", "Deleted "+title)
	})
}

// Restore undeletes a recipe, bumping its version. Restoring a recipe that
// isn't deleted is a no-op that returns it unchanged.
func (s *Service) Restore(ctx context.Context, c identity.Caller, id string) (Recipe, error) {
	who, err := author(c)
	if err != nil {
		return Recipe{}, err
	}
	now := s.stamp()
	var out Recipe
	err = s.st.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE recipes SET version = version + 1, deleted_at = NULL, updated_at = ?, updated_by = ?
			WHERE id = ? AND deleted_at IS NOT NULL`, now, who, id)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		out, err = loadRecipe(ctx, tx, id, who)
		if err != nil || n == 0 {
			return err
		}
		return s.logChange(ctx, tx, c, now, "recipe", id, "restore", "Restored "+out.Title)
	})
	return out, err
}

// SetMine merges p into the caller's flags on the recipe and returns the
// result. It is not recorded in the change log.
func (s *Service) SetMine(ctx context.Context, c identity.Caller, id string, p MinePatch) (Mine, error) {
	who, err := author(c)
	if err != nil {
		return Mine{}, err
	}
	now := s.stamp()
	var m Mine
	err = s.st.Write(ctx, func(tx *sql.Tx) error {
		var one int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM recipes WHERE id = ?`, id).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound()
		} else if err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, `INSERT INTO person_recipe (person_id, recipe_id, favorite, want_to_cook, updated_at)
			VALUES (?1, ?2, coalesce(?3, 0), coalesce(?4, 0), ?5)
			ON CONFLICT (person_id, recipe_id) DO UPDATE SET
				favorite = coalesce(?3, favorite), want_to_cook = coalesce(?4, want_to_cook), updated_at = ?5
			RETURNING favorite, want_to_cook`, who, id, p.Favorite, p.WantToCook, now).Scan(&m.Favorite, &m.WantToCook)
	})
	return m, err
}

func notFound() *Error {
	return Errorf(CodeNotFound, "that recipe does not exist")
}

// checkClaimed turns a guarded UPDATE that matched no row into the reason:
// the recipe doesn't exist, is in the trash, or has moved past the caller's
// version. Being in the same transaction, Current is exactly what beat it.
func checkClaimed(ctx context.Context, tx *sql.Tx, res sql.Result, id, who string) error {
	n, err := res.RowsAffected()
	if err != nil || n > 0 {
		return err
	}
	var deletedAt *string
	err = tx.QueryRowContext(ctx, `SELECT deleted_at FROM recipes WHERE id = ?`, id).Scan(&deletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound()
	} else if err != nil {
		return err
	}
	if deletedAt != nil {
		return Errorf(CodeDeleted, "that recipe is in the trash; restore it first")
	}
	cur, err := loadRecipe(ctx, tx, id, who)
	if err != nil {
		return err
	}
	e := Errorf(CodeStale, "someone changed this recipe since you opened it")
	e.Current = &cur
	return e
}

func checkMinutes(fields map[string]*int) error {
	for _, name := range []string{"prep_minutes", "cook_minutes", "total_minutes"} {
		if v := fields[name]; v != nil && *v < 0 {
			return Errorf(CodeInvalid, "%s must be 0 or more", name)
		}
	}
	return nil
}

func servingsOf(yieldText *string) *int {
	if yieldText == nil {
		return nil
	}
	return Servings(*yieldText)
}

// patchColumns returns the SET clauses and arguments for the scalar fields
// patch carries. Null clears a column; the title can't be cleared.
func patchColumns(p RecipePatch) ([]string, []any, error) {
	var sets []string
	var args []any
	add := func(col string, v any) {
		sets = append(sets, col+" = ?")
		args = append(args, v)
	}
	if p.Title.Set {
		t := strings.TrimSpace(p.Title.Value)
		if p.Title.Null || t == "" {
			return nil, nil, Errorf(CodeInvalid, "title is required and can't be cleared")
		}
		add("title", t)
	}
	if err := checkMinutes(map[string]*int{
		"prep_minutes": p.PrepMinutes.Ptr(), "cook_minutes": p.CookMinutes.Ptr(), "total_minutes": p.TotalMinutes.Ptr(),
	}); err != nil {
		return nil, nil, err
	}
	for _, f := range []struct {
		col string
		o   Optional[string]
	}{
		{"source_url", p.SourceURL}, {"description", p.Description}, {"notes", p.Notes},
		{"yield_text", p.YieldText}, {"time_text", p.TimeText}, {"nutrition_text", p.NutritionText},
	} {
		if f.o.Set {
			add(f.col, f.o.Ptr())
		}
	}
	if p.YieldText.Set {
		add("servings", servingsOf(p.YieldText.Ptr()))
	}
	for _, f := range []struct {
		col string
		o   Optional[int]
	}{
		{"prep_minutes", p.PrepMinutes}, {"cook_minutes", p.CookMinutes}, {"total_minutes", p.TotalMinutes},
	} {
		if f.o.Set {
			add(f.col, f.o.Ptr())
		}
	}
	return sets, args, nil
}

// changedFields names, in display order, the fields that differ.
func changedFields(a, b Recipe) []string {
	fields := []struct {
		name string
		x, y any
	}{
		{"title", a.Title, b.Title},
		{"source", a.SourceURL, b.SourceURL},
		{"description", a.Description, b.Description},
		{"notes", a.Notes, b.Notes},
		{"yield", a.YieldText, b.YieldText},
		{"prep time", a.PrepMinutes, b.PrepMinutes},
		{"cook time", a.CookMinutes, b.CookMinutes},
		{"total time", a.TotalMinutes, b.TotalMinutes},
		{"time", a.TimeText, b.TimeText},
		{"nutrition", a.NutritionText, b.NutritionText},
		{"tags", a.Tags, b.Tags},
		{"images", a.Images, b.Images},
		{"ingredients", a.Ingredients, b.Ingredients},
		{"steps", a.Steps, b.Steps},
	}
	var out []string
	for _, f := range fields {
		if !reflect.DeepEqual(f.x, f.y) {
			out = append(out, f.name)
		}
	}
	return out
}

// setTags replaces the recipe's tags, keeping their order. Each name
// matches an existing tag case-insensitively (the first spelling stored
// wins) or creates one; blanks and repeats are dropped.
func setTags(ctx context.Context, tx *sql.Tx, id string, names []string, who, now string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM recipe_tags WHERE recipe_id = ?`, id); err != nil {
		return err
	}
	seen := map[int64]bool{}
	pos := 0
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		var tagID int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM tags WHERE name = ?`, name).Scan(&tagID)
		if errors.Is(err, sql.ErrNoRows) {
			err = tx.QueryRowContext(ctx, `INSERT INTO tags (name, created_at, created_by) VALUES (?, ?, ?) RETURNING id`,
				name, now, who).Scan(&tagID)
		}
		if err != nil {
			return err
		}
		if seen[tagID] {
			continue
		}
		seen[tagID] = true
		if _, err := tx.ExecContext(ctx, `INSERT INTO recipe_tags (recipe_id, tag_id, position) VALUES (?, ?, ?)`,
			id, tagID, pos); err != nil {
			return err
		}
		pos++
	}
	return nil
}

// setImages replaces the recipe's images in order. Every hash must already
// be uploaded; repeats are dropped.
func setImages(ctx context.Context, tx *sql.Tx, id string, hashes []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM recipe_images WHERE recipe_id = ?`, id); err != nil {
		return err
	}
	seen := map[string]bool{}
	pos := 0
	for _, h := range hashes {
		if seen[h] {
			continue
		}
		seen[h] = true
		var one int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM images WHERE hash = ?`, h).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return Errorf(CodeUnknownImage, "images: no uploaded image has hash %q", h)
		} else if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO recipe_images (recipe_id, position, image_hash) VALUES (?, ?, ?)`,
			id, pos, h); err != nil {
			return err
		}
		pos++
	}
	return nil
}

func setIngredients(ctx context.Context, tx *sql.Tx, id string, ings []Ingredient) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM recipe_ingredients WHERE recipe_id = ?`, id); err != nil {
		return err
	}
	for i, ing := range ings {
		_, err := tx.ExecContext(ctx, `INSERT INTO recipe_ingredients
			(recipe_id, position, section, raw, quantity, quantity_max, unit, item, note, parser_version)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, i, ing.Section, ing.Raw, ing.Quantity, ing.QuantityMax, ing.Unit, ing.Item, ing.Note, ing.ParserVersion)
		if err != nil {
			return err
		}
	}
	return nil
}

func setSteps(ctx context.Context, tx *sql.Tx, id string, steps []Step) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM recipe_steps WHERE recipe_id = ?`, id); err != nil {
		return err
	}
	for i, st := range steps {
		if _, err := tx.ExecContext(ctx, `INSERT INTO recipe_steps (recipe_id, position, section, text) VALUES (?, ?, ?, ?)`,
			id, i, st.Section, st.Text); err != nil {
			return err
		}
	}
	return nil
}

// loadRecipe reads the full recipe with username's flags (none for "").
func loadRecipe(ctx context.Context, tx *sql.Tx, id, username string) (Recipe, error) {
	var r Recipe
	err := tx.QueryRowContext(ctx, `SELECT id, title, source_url, description, notes, yield_text, servings,
		prep_minutes, cook_minutes, total_minutes, time_text, nutrition_text, version, deleted_at,
		created_at, created_by, updated_at, updated_by
		FROM recipes WHERE id = ?`, id).Scan(&r.ID, &r.Title, &r.SourceURL, &r.Description, &r.Notes, &r.YieldText,
		&r.Servings, &r.PrepMinutes, &r.CookMinutes, &r.TotalMinutes, &r.TimeText, &r.NutritionText, &r.Version,
		&r.DeletedAt, &r.CreatedAt, &r.CreatedBy, &r.UpdatedAt, &r.UpdatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return Recipe{}, notFound()
	} else if err != nil {
		return Recipe{}, err
	}
	if r.SourceURL != nil {
		r.SourceHost = SourceHost(*r.SourceURL)
	}
	if r.Tags, err = queryStrings(ctx, tx, `SELECT t.name FROM recipe_tags rt JOIN tags t ON t.id = rt.tag_id
		WHERE rt.recipe_id = ? ORDER BY rt.position`, id); err != nil {
		return Recipe{}, err
	}
	if r.Images, err = queryStrings(ctx, tx, `SELECT image_hash FROM recipe_images WHERE recipe_id = ? ORDER BY position`, id); err != nil {
		return Recipe{}, err
	}

	r.Ingredients = []Ingredient{}
	rows, err := tx.QueryContext(ctx, `SELECT section, raw, quantity, quantity_max, unit, item, note, parser_version
		FROM recipe_ingredients WHERE recipe_id = ? ORDER BY position`, id)
	if err != nil {
		return Recipe{}, err
	}
	for rows.Next() {
		var i Ingredient
		if err := rows.Scan(&i.Section, &i.Raw, &i.Quantity, &i.QuantityMax, &i.Unit, &i.Item, &i.Note, &i.ParserVersion); err != nil {
			rows.Close()
			return Recipe{}, err
		}
		r.Ingredients = append(r.Ingredients, i)
	}
	if err := rows.Close(); err != nil {
		return Recipe{}, err
	}
	if err := rows.Err(); err != nil {
		return Recipe{}, err
	}

	r.Steps = []Step{}
	rows, err = tx.QueryContext(ctx, `SELECT section, text FROM recipe_steps WHERE recipe_id = ? ORDER BY position`, id)
	if err != nil {
		return Recipe{}, err
	}
	for rows.Next() {
		var st Step
		if err := rows.Scan(&st.Section, &st.Text); err != nil {
			rows.Close()
			return Recipe{}, err
		}
		r.Steps = append(r.Steps, st)
	}
	if err := rows.Close(); err != nil {
		return Recipe{}, err
	}
	if err := rows.Err(); err != nil {
		return Recipe{}, err
	}

	err = tx.QueryRowContext(ctx, `SELECT favorite, want_to_cook FROM person_recipe WHERE person_id = ? AND recipe_id = ?`,
		username, id).Scan(&r.Mine.Favorite, &r.Mine.WantToCook)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Recipe{}, err
	}
	return r, nil
}

// queryStrings returns the single string column of q's rows, never nil.
func queryStrings(ctx context.Context, tx *sql.Tx, q string, args ...any) ([]string, error) {
	rows, err := tx.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
