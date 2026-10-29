package recipes

import (
	"context"
	"database/sql"
	"slices"
	"strings"
	"unicode"

	"github.com/sroberts/sous/internal/identity"
)

// List returns live recipe summaries matching q, paged by opaque cursor.
// Repeated tags AND together case-insensitively; the default sort is title
// even with q set; favorite or want_to_cook from nobody matches nothing.
// A bad sort or limit is CodeInvalid; a bad cursor is CodeInvalidCursor.
func (s *Service) List(ctx context.Context, c identity.Caller, q ListQuery) (Page[Summary], error) {
	limit, err := pageLimit(q.Limit)
	if err != nil {
		return Page[Summary]{}, err
	}
	sp := pageSpec{where: []string{"r.deleted_at IS NULL"}}
	switch q.Sort {
	case "", "title":
		sp.kind, sp.key, sp.nocase = "title", "r.title", true
	case "updated":
		sp.kind, sp.key, sp.desc = "updated", "r.updated_at", true
	case "created":
		sp.kind, sp.key, sp.desc = "created", "r.created_at", true
	default:
		return Page[Summary]{}, Errorf(CodeInvalid, "sort must be title, updated or created")
	}
	if (q.Favorite || q.WantToCook) && c.Person == nil {
		return Page[Summary]{Items: []Summary{}}, nil
	}

	if m := ftsQuery(q.Q); m != "" {
		sp.filter("r.seq IN (SELECT rowid FROM recipe_search WHERE recipe_search MATCH ?)", m)
	}
	for _, t := range q.Tag {
		if t = strings.TrimSpace(t); t != "" {
			// fold, not NOCASE: NOCASE ignores case for ASCII only.
			sp.filter(`EXISTS (SELECT 1 FROM recipe_tags rt JOIN tags t ON t.id = rt.tag_id
				WHERE rt.recipe_id = r.id AND fold(t.name) = ?)`, strings.ToLower(t))
		}
	}
	for _, ing := range q.Ingredient {
		if ing = strings.TrimSpace(ing); ing != "" {
			sp.filter(`EXISTS (SELECT 1 FROM recipe_ingredients ri
				WHERE ri.recipe_id = r.id AND instr(fold(ri.item), ?) > 0)`, strings.ToLower(ing))
		}
	}
	if q.Favorite {
		sp.filter(`EXISTS (SELECT 1 FROM person_recipe f WHERE f.recipe_id = r.id AND f.person_id = ? AND f.favorite = 1)`,
			c.Person.Username)
	}
	if q.WantToCook {
		sp.filter(`EXISTS (SELECT 1 FROM person_recipe w WHERE w.recipe_id = r.id AND w.person_id = ? AND w.want_to_cook = 1)`,
			c.Person.Username)
	}
	return s.page(ctx, c.Username(), sp, q.Cursor, limit)
}

// Trash returns deleted recipes, most recently deleted first, with
// DeletedAt set on each summary.
func (s *Service) Trash(ctx context.Context, c identity.Caller, q TrashQuery) (Page[Summary], error) {
	limit, err := pageLimit(q.Limit)
	if err != nil {
		return Page[Summary]{}, err
	}
	sp := pageSpec{kind: "trash", key: "r.deleted_at", desc: true, where: []string{"r.deleted_at IS NOT NULL"}}
	return s.page(ctx, c.Username(), sp, q.Cursor, limit)
}

// ftsQuery turns free text into an FTS5 query: every word must appear,
// matched as a prefix. Quoting each token keeps FTS5 syntax out of user
// input. It returns "" when q has no words.
func ftsQuery(q string) string {
	words := strings.FieldsFunc(q, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r) && !unicode.IsMark(r)
	})
	for i, w := range words {
		words[i] = `"` + strings.ReplaceAll(w, `"`, `""`) + `"*`
	}
	return strings.Join(words, " AND ")
}

// pageSpec is a keyset-paged summary query: the filters, and the sort key
// column, tie-broken by id in the same direction.
type pageSpec struct {
	kind   string // names the ordering inside cursors
	key    string
	desc   bool
	nocase bool
	where  []string
	args   []any
}

func (p *pageSpec) filter(cond string, args ...any) {
	p.where = append(p.where, cond)
	p.args = append(p.args, args...)
}

func (p pageSpec) keyExpr() string {
	if p.nocase {
		return p.key + " COLLATE NOCASE"
	}
	return p.key
}

func (p pageSpec) after() string {
	op := ">"
	if p.desc {
		op = "<"
	}
	k := p.keyExpr()
	return "(" + k + " " + op + " ? OR (" + k + " = ? AND r.id " + op + " ?))"
}

func (p pageSpec) order() string {
	if p.desc {
		return p.keyExpr() + " DESC, r.id DESC"
	}
	return p.keyExpr() + ", r.id"
}

func (s *Service) page(ctx context.Context, username string, p pageSpec, cur string, limit int) (Page[Summary], error) {
	var after *cursor
	if cur != "" {
		c, err := decodeCursor(cur, p.kind)
		if err != nil {
			return Page[Summary]{}, err
		}
		after = &c
	}
	out := Page[Summary]{Items: []Summary{}}
	err := s.st.Read(ctx, func(tx *sql.Tx) error {
		where := strings.Join(p.where, " AND ")
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM recipes r WHERE `+where, p.args...).Scan(&out.Total); err != nil {
			return err
		}
		args := slices.Concat([]any{username}, p.args)
		if after != nil {
			where += " AND " + p.after()
			args = append(args, after.Key, after.Key, after.ID)
		}
		args = append(args, limit+1)
		rows, err := tx.QueryContext(ctx, `SELECT r.id, r.title, r.source_url, r.prep_minutes, r.cook_minutes,
			r.total_minutes, r.time_text, r.version, r.updated_at, r.deleted_at, `+p.key+`,
			coalesce(pr.favorite, 0), coalesce(pr.want_to_cook, 0)
			FROM recipes r LEFT JOIN person_recipe pr ON pr.recipe_id = r.id AND pr.person_id = ?
			WHERE `+where+` ORDER BY `+p.order()+` LIMIT ?`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		var keys []string
		for rows.Next() {
			var sm Summary
			var src *string
			var prep, cook *int
			var key string
			if err := rows.Scan(&sm.ID, &sm.Title, &src, &prep, &cook, &sm.TotalMinutes, &sm.TimeText, &sm.Version,
				&sm.UpdatedAt, &sm.DeletedAt, &key, &sm.Mine.Favorite, &sm.Mine.WantToCook); err != nil {
				return err
			}
			if src != nil {
				sm.SourceHost = SourceHost(*src)
			}
			sm.TotalMinutes = summaryMinutes(sm.TotalMinutes, prep, cook)
			sm.Tags, sm.IngredientItems = []string{}, []string{}
			out.Items = append(out.Items, sm)
			keys = append(keys, key)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(out.Items) > limit {
			out.Items = out.Items[:limit]
			out.NextCursor = encodeCursor(cursor{Kind: p.kind, Key: keys[limit-1], ID: out.Items[limit-1].ID})
		}
		return decorate(ctx, tx, out.Items)
	})
	if err != nil {
		return Page[Summary]{}, err
	}
	return out, nil
}

// summaryMinutes is the mock's total ?? (prep || cook ? prep + cook : null),
// where a zero counts as unset.
func summaryMinutes(total, prep, cook *int) *int {
	if total != nil {
		return total
	}
	p, c := 0, 0
	if prep != nil {
		p = *prep
	}
	if cook != nil {
		c = *cook
	}
	if p == 0 && c == 0 {
		return nil
	}
	sum := p + c
	return &sum
}

// decorate fills tags, cover and ingredient items for a page of summaries
// with one query each, rather than per recipe.
func decorate(ctx context.Context, tx *sql.Tx, items []Summary) error {
	if len(items) == 0 {
		return nil
	}
	idx := make(map[string]*Summary, len(items))
	ids := make([]any, len(items))
	for i := range items {
		idx[items[i].ID] = &items[i]
		ids[i] = items[i].ID
	}
	in := "(" + placeholders(len(ids)) + ")"
	each := func(q string, fn func(sm *Summary, v string)) error {
		rows, err := tx.QueryContext(ctx, q, ids...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id, v string
			if err := rows.Scan(&id, &v); err != nil {
				return err
			}
			fn(idx[id], v)
		}
		return rows.Err()
	}
	if err := each(`SELECT rt.recipe_id, t.name FROM recipe_tags rt JOIN tags t ON t.id = rt.tag_id
		WHERE rt.recipe_id IN `+in+` ORDER BY rt.recipe_id, rt.position`,
		func(sm *Summary, v string) { sm.Tags = append(sm.Tags, v) }); err != nil {
		return err
	}
	if err := each(`SELECT recipe_id, image_hash FROM recipe_images WHERE position = 0 AND recipe_id IN `+in,
		func(sm *Summary, v string) { sm.Cover = &v }); err != nil {
		return err
	}
	return each(`SELECT recipe_id, item FROM recipe_ingredients WHERE recipe_id IN `+in+` ORDER BY recipe_id, position`,
		func(sm *Summary, v string) { sm.IngredientItems = append(sm.IngredientItems, strings.ToLower(v)) })
}
