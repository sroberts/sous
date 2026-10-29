package recipes

import (
	"bytes"
	"fmt"
	"slices"
	"testing"

	"github.com/sroberts/sous/internal/identity"
)

func titles(p Page[Summary]) []string {
	out := []string{}
	for _, s := range p.Items {
		out = append(out, s.Title)
	}
	return out
}

func list(t *testing.T, s *Service, c identity.Caller, q ListQuery) []string {
	t.Helper()
	p, err := s.List(ctx, c, q)
	if err != nil {
		t.Fatalf("List(%+v): %v", q, err)
	}
	if p.Total != len(p.Items) {
		t.Errorf("List(%+v): total %d, %d items", q, p.Total, len(p.Items))
	}
	return titles(p)
}

func TestListFilters(t *testing.T) {
	s := newTestService(t)
	scott := person(t, s, "Scott")
	alice := person(t, s, "Alice")
	pad := create(t, s, scott, RecipeInput{
		Title: "Pad Thai", Tags: []string{"Dinner", "Thai"},
		Ingredients: []string{"200 g Rice Noodles", "2 tbsp fish sauce", "1 lime"},
	})
	create(t, s, scott, RecipeInput{
		Title: "green curry", Description: sp("A Thai classic"), Tags: []string{"dinner"},
		Ingredients: []string{"400 ml coconut milk", "1 lime"},
	})
	crumble := create(t, s, scott, RecipeInput{
		Title: "Apple Crumble", Notes: sp("Crème fraîche on the side"), Tags: []string{"Dessert"},
		Ingredients: []string{"4 apples", "100 g butter"},
	})
	gone := create(t, s, scott, RecipeInput{Title: "Thai Basil Pork", Tags: []string{"Thai"}})
	if err := s.Delete(ctx, scott, gone.ID, 1); err != nil {
		t.Fatal(err)
	}
	yes := true
	s.SetMine(ctx, scott, pad.ID, MinePatch{Favorite: &yes})
	s.SetMine(ctx, scott, crumble.ID, MinePatch{WantToCook: &yes})
	s.SetMine(ctx, alice, crumble.ID, MinePatch{Favorite: &yes})

	for _, tc := range []struct {
		name string
		c    identity.Caller
		q    ListQuery
		want []string
	}{
		{"all by title", scott, ListQuery{}, []string{"Apple Crumble", "green curry", "Pad Thai"}},
		{"fts word", scott, ListQuery{Q: "thai"}, []string{"green curry", "Pad Thai"}},
		{"fts prefix", scott, ListQuery{Q: "noodl"}, []string{"Pad Thai"}},
		{"fts and", scott, ListQuery{Q: "thai lim"}, []string{"green curry", "Pad Thai"}},
		{"fts and narrows", scott, ListQuery{Q: "thai coco"}, []string{"green curry"}},
		{"fts diacritics", scott, ListQuery{Q: "creme"}, []string{"Apple Crumble"}},
		{"fts tag", scott, ListQuery{Q: "dessert"}, []string{"Apple Crumble"}},
		{"fts syntax is literal", scott, ListQuery{Q: `"pad" OR NEAR(`}, []string{}},
		{"fts punctuation only", scott, ListQuery{Q: " -*\" "}, []string{"Apple Crumble", "green curry", "Pad Thai"}},
		{"tag any case", scott, ListQuery{Tag: []string{"DINNER"}}, []string{"green curry", "Pad Thai"}},
		{"tag and", scott, ListQuery{Tag: []string{"dinner", "thai"}}, []string{"Pad Thai"}},
		{"tag deleted excluded", scott, ListQuery{Tag: []string{"Thai"}}, []string{"Pad Thai"}},
		{"ingredient substring", scott, ListQuery{Ingredient: []string{"LIME"}}, []string{"green curry", "Pad Thai"}},
		{"ingredient and", scott, ListQuery{Ingredient: []string{"lime", "noodle"}}, []string{"Pad Thai"}},
		{"ingredient none", scott, ListQuery{Ingredient: []string{"lime", "apple"}}, []string{}},
		{"favorite", scott, ListQuery{Favorite: true}, []string{"Pad Thai"}},
		{"favorite other person", alice, ListQuery{Favorite: true}, []string{"Apple Crumble"}},
		{"want to cook", scott, ListQuery{WantToCook: true}, []string{"Apple Crumble"}},
		{"favorite from nobody", identity.Caller{}, ListQuery{Favorite: true}, []string{}},
		{"sort updated", scott, ListQuery{Sort: "updated"}, []string{"Apple Crumble", "green curry", "Pad Thai"}},
		{"sort created", scott, ListQuery{Sort: "created"}, []string{"Apple Crumble", "green curry", "Pad Thai"}},
		{"q keeps title sort", scott, ListQuery{Q: "lime"}, []string{"green curry", "Pad Thai"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := list(t, s, tc.c, tc.q); !slices.Equal(got, tc.want) {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}

	// Editing Pad Thai moves it to the top by updated, not by created.
	if _, err := s.Update(ctx, scott, pad.ID, 1, RecipePatch{Notes: Some("x")}); err != nil {
		t.Fatal(err)
	}
	if got := list(t, s, scott, ListQuery{Sort: "updated"}); !slices.Equal(got, []string{"Pad Thai", "Apple Crumble", "green curry"}) {
		t.Errorf("updated after edit = %q", got)
	}
	if got := list(t, s, scott, ListQuery{Sort: "created"}); !slices.Equal(got, []string{"Apple Crumble", "green curry", "Pad Thai"}) {
		t.Errorf("created after edit = %q", got)
	}

	for _, q := range []ListQuery{{Sort: "rating"}, {Limit: -1}} {
		if _, err := s.List(ctx, scott, q); code(err) != CodeInvalid {
			t.Errorf("List(%+v) = %v", q, err)
		}
	}
}

func TestListSummary(t *testing.T) {
	s := newTestService(t)
	scott := person(t, s, "Scott")
	img, _ := s.UploadImage(ctx, scott, bytes.NewReader(pngBytes(t, 2)))
	img2, _ := s.UploadImage(ctx, scott, bytes.NewReader(pngBytes(t, 4)))
	r := create(t, s, scott, RecipeInput{
		Title: "Pad Thai", SourceURL: sp("https://www.example.com/x"), PrepMinutes: ip(10), CookMinutes: ip(20),
		TimeText: sp("about half an hour"), Tags: []string{"Dinner", "Thai"}, Images: []string{img.Hash, img2.Hash},
		Ingredients: []string{"# Sauce", "2 tbsp Fish Sauce", "200 g Rice Noodles"},
	})
	yes := true
	s.SetMine(ctx, scott, r.ID, MinePatch{Favorite: &yes})

	p, err := s.List(ctx, scott, ListQuery{})
	if err != nil || len(p.Items) != 1 {
		t.Fatalf("List = %+v, %v", p, err)
	}
	sm := p.Items[0]
	if sm.ID != r.ID || *sm.SourceHost != "example.com" || *sm.Cover != img.Hash || *sm.TotalMinutes != 30 ||
		*sm.TimeText != "about half an hour" || sm.Version != 1 || sm.UpdatedAt != r.UpdatedAt || sm.DeletedAt != nil {
		t.Errorf("summary = %+v", sm)
	}
	if !slices.Equal(sm.Tags, []string{"Dinner", "Thai"}) || !slices.Equal(sm.IngredientItems, []string{"fish sauce", "rice noodles"}) {
		t.Errorf("tags %q, items %q", sm.Tags, sm.IngredientItems)
	}
	if sm.Mine != (Mine{Favorite: true}) {
		t.Errorf("mine = %+v", sm.Mine)
	}
	if p.NextCursor != nil {
		t.Errorf("next cursor on last page")
	}

	bare := create(t, s, scott, RecipeInput{Title: "Toast"})
	p, _ = s.List(ctx, identity.Caller{}, ListQuery{Q: "toast"})
	if sm := p.Items[0]; sm.ID != bare.ID || sm.Cover != nil || sm.TotalMinutes != nil || sm.SourceHost != nil ||
		sm.Tags == nil || sm.IngredientItems == nil || sm.Mine != (Mine{}) {
		t.Errorf("bare summary = %+v", sm)
	}
}

func TestListPaging(t *testing.T) {
	s := newTestService(t)
	scott := person(t, s, "Scott")
	const n = 23
	for i := range n {
		// Repeated titles in mixed case exercise the id tie-break.
		title := fmt.Sprintf("Recipe %02d", i%7)
		if i%2 == 0 {
			title = fmt.Sprintf("recipe %02d", i%7)
		}
		create(t, s, scott, RecipeInput{Title: title})
	}
	for _, sort := range []string{"title", "updated", "created"} {
		var ids []string
		cur := ""
		pages := 0
		for {
			p, err := s.List(ctx, scott, ListQuery{Sort: sort, Cursor: cur, Limit: 4})
			if err != nil {
				t.Fatal(err)
			}
			if p.Total != n {
				t.Errorf("%s: total = %d", sort, p.Total)
			}
			for _, sm := range p.Items {
				ids = append(ids, sm.ID)
			}
			pages++
			if p.NextCursor == nil {
				break
			}
			cur = *p.NextCursor
		}
		all, err := s.List(ctx, scott, ListQuery{Sort: sort, Limit: 200})
		if err != nil {
			t.Fatal(err)
		}
		var want []string
		for _, sm := range all.Items {
			want = append(want, sm.ID)
		}
		if pages != 6 || !slices.Equal(ids, want) || len(want) != n {
			t.Errorf("%s: %d pages, paged %d ids, want %d in one page order", sort, pages, len(ids), len(want))
		}
		uniq := slices.Clone(ids)
		slices.Sort(uniq)
		if len(slices.Compact(uniq)) != n {
			t.Errorf("%s: duplicates in paging", sort)
		}
	}

	if _, err := s.List(ctx, scott, ListQuery{Cursor: "!!!"}); code(err) != CodeInvalidCursor {
		t.Errorf("garbage cursor = %v", err)
	}
	p, _ := s.List(ctx, scott, ListQuery{Sort: "updated", Limit: 2})
	if _, err := s.List(ctx, scott, ListQuery{Sort: "title", Cursor: *p.NextCursor}); code(err) != CodeInvalidCursor {
		t.Errorf("cursor from another sort = %v", err)
	}
	if p, _ := s.List(ctx, scott, ListQuery{Limit: 1000}); len(p.Items) != n {
		t.Errorf("limit over max = %d items", len(p.Items))
	}
	if p, _ := s.List(ctx, scott, ListQuery{}); len(p.Items) != n {
		t.Errorf("default limit = %d items", len(p.Items))
	}
}

func TestTrash(t *testing.T) {
	s := newTestService(t)
	scott := person(t, s, "Scott")
	var ids []string
	for _, title := range []string{"A", "B", "C", "D"} {
		ids = append(ids, create(t, s, scott, RecipeInput{Title: title}).ID)
	}
	// Delete C, A, D: newest deletion first is D, A, C.
	for _, i := range []int{2, 0, 3} {
		if err := s.Delete(ctx, scott, ids[i], 1); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	cur := ""
	for {
		p, err := s.Trash(ctx, scott, TrashQuery{Cursor: cur, Limit: 2})
		if err != nil {
			t.Fatal(err)
		}
		if p.Total != 3 {
			t.Errorf("total = %d", p.Total)
		}
		for _, sm := range p.Items {
			if sm.DeletedAt == nil {
				t.Errorf("%s: no deleted_at", sm.Title)
			}
			got = append(got, sm.Title)
		}
		if p.NextCursor == nil {
			break
		}
		cur = *p.NextCursor
	}
	if !slices.Equal(got, []string{"D", "A", "C"}) {
		t.Errorf("trash = %q", got)
	}
	if live := list(t, s, scott, ListQuery{}); !slices.Equal(live, []string{"B"}) {
		t.Errorf("live = %q", live)
	}
}
