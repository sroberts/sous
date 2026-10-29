package api

import (
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"
	"testing"
)

// These replay what web/src/lib/api.js, session.svelte.js and
// routes/Editor.svelte actually send, and check the fields they read,
// including null versus absent, the way the mock server answers.

func keys(m map[string]any) []string {
	return slices.Sorted(maps.Keys(m))
}

func wantKeys(t *testing.T, what string, m map[string]any, want ...string) {
	t.Helper()
	slices.Sort(want)
	if got := keys(m); !slices.Equal(got, want) {
		t.Errorf("%s keys = %v, want %v", what, got, want)
	}
}

var (
	summaryKeys = []string{"id", "title", "source_host", "tags", "cover", "total_minutes", "time_text",
		"ingredient_items", "mine", "version", "updated_at"}
	recipeKeys = []string{"id", "title", "source_url", "description", "notes", "yield_text", "servings",
		"prep_minutes", "cook_minutes", "total_minutes", "time_text", "nutrition_text", "tags", "images",
		"ingredients", "steps", "version", "deleted_at", "created_at", "created_by", "updated_at", "updated_by",
		"source_host", "mine"}
	ingredientKeys = []string{"section", "raw", "quantity", "quantity_max", "unit", "item", "note", "parser_version"}
)

// ui is how the browser talks: always x-sous-via: ui, and once someone is
// chosen, the sous_person cookie and x-sous-person header.
func ui(username string) []string {
	h := []string{"X-Sous-Via", "ui"}
	if username != "" {
		h = append(h, "Cookie", "sous_person="+url.PathEscape(username), "X-Sous-Person", username)
	}
	return h
}

// allRecipes pages exactly as api.allRecipes does: the cursor is appended
// to the URL without encoding.
func allRecipes(t *testing.T, h *harness, hdr []string) []map[string]any {
	t.Helper()
	var items []map[string]any
	cursor := ""
	for pages := 0; ; pages++ {
		if pages > 50 {
			t.Fatal("paging never ended")
		}
		path := "/v1/recipes?limit=200"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		r := h.do("GET", path, nil, hdr...)
		h.want(r, 200, "")
		page := r.json(t)
		wantKeys(t, "page", page, "items", "total", "next_cursor")
		for _, it := range page["items"].([]any) {
			items = append(items, it.(map[string]any))
		}
		next, ok := page["next_cursor"].(string)
		if page["next_cursor"] != nil && !ok {
			t.Fatalf("next_cursor = %v", page["next_cursor"])
		}
		if next == "" {
			return items
		}
		cursor = next
	}
}

func TestUIBoot(t *testing.T) {
	h := newHarness(t)

	// boot(): people and the library, before anyone is chosen.
	r := h.do("GET", "/v1/people", nil, ui("")...)
	h.want(r, 200, "")
	if items := r.json(t)["items"].([]any); len(items) != 0 {
		t.Fatalf("people = %v", items)
	}
	if items := allRecipes(t, h, ui("")); len(items) != 0 {
		t.Fatalf("recipes = %v", items)
	}

	// The who's-cooking picker adds the first person with nobody chosen.
	r = h.do("POST", "/v1/people", map[string]any{"name": "Scott"}, ui("")...)
	h.want(r, 201, "")
	p := r.json(t)
	wantKeys(t, "person", p, "username", "name")
	scott := p["username"].(string)

	// A write before choosing gets who_are_you, which opens the picker.
	h.want(h.do("POST", "/v1/recipes", map[string]any{"title": "x"}, ui("")...), 401, "who_are_you")

	// More than one page at the UI's limit of 200, to follow next_cursor.
	for i := range 205 {
		h.create(ui(scott), map[string]any{"title": fmt.Sprintf("Recipe %03d", i)})
	}
	items := allRecipes(t, h, ui(scott))
	if len(items) != 205 {
		t.Fatalf("paged %d recipes, want 205", len(items))
	}
	ids := map[string]bool{}
	for _, it := range items {
		ids[it["id"].(string)] = true
	}
	if len(ids) != 205 {
		t.Errorf("%d distinct ids over the pages", len(ids))
	}
	s := items[0]
	wantKeys(t, "summary", s, summaryKeys...)
	wantKeys(t, "summary.mine", s["mine"].(map[string]any), "favorite", "want_to_cook")
	if s["cover"] != nil || s["source_host"] != nil || s["total_minutes"] != nil || s["time_text"] != nil {
		t.Errorf("empty summary fields should be null: %v", s)
	}
	if _, ok := s["tags"].([]any); !ok {
		t.Errorf("tags = %v, want []", s["tags"])
	}
	if _, ok := s["ingredient_items"].([]any); !ok {
		t.Errorf("ingredient_items = %v, want []", s["ingredient_items"])
	}
}

func TestUIEditFlow(t *testing.T) {
	h := newHarness(t)
	scott := h.person("Scott")[1]
	jo := h.person("Jo")[1]

	// Editor.svelte's body() for a new recipe: blanks are null, lists trimmed.
	r := h.do("POST", "/v1/recipes", map[string]any{
		"title":         "Pad Thai",
		"source_url":    "https://www.example.com/pad-thai",
		"description":   nil,
		"yield_text":    "Serves 2",
		"prep_minutes":  15,
		"cook_minutes":  nil,
		"total_minutes": nil,
		"notes":         nil,
		"tags":          []string{"Dinner", "Thai"},
		"images":        []string{},
		"ingredients":   []string{"# Sauce", "2 tbsp fish sauce", "1 tbsp sugar", "# Noodles", "200 g rice noodles"},
		"steps":         []string{"Soak the noodles.", "# Finish", "Toss everything."},
	}, ui(scott)...)
	h.want(r, 201, "")
	if et := r.header.Get("ETag"); et != `"1"` {
		t.Errorf("create ETag = %q", et)
	}
	rec := r.json(t)
	wantKeys(t, "recipe", rec, recipeKeys...)
	id := rec["id"].(string)
	if rec["version"] != 1.0 || rec["deleted_at"] != nil || rec["created_by"] != scott || rec["source_host"] != "example.com" {
		t.Errorf("created = %v", rec)
	}
	if !slices.Equal(rec["tags"].([]any), []any{"Dinner", "Thai"}) {
		t.Errorf("tags lost their order: %v", rec["tags"])
	}
	ings := rec["ingredients"].([]any)
	if len(ings) != 3 {
		t.Fatalf("ingredients = %v", ings)
	}
	first := ings[0].(map[string]any)
	wantKeys(t, "ingredient", first, ingredientKeys...)
	if first["section"] != "Sauce" || first["unit"] != "tbsp" || first["quantity"] != 2.0 || first["item"] != "fish sauce" {
		t.Errorf("ingredient = %v", first)
	}
	steps := rec["steps"].([]any)
	s0, s1 := steps[0].(map[string]any), steps[1].(map[string]any)
	wantKeys(t, "step", s0, "section", "text")
	if v, ok := s0["section"]; !ok || v != nil || s1["section"] != "Finish" {
		t.Errorf("steps = %v", steps)
	}

	// The recipe page loads it and keeps the ETag's version.
	r = h.do("GET", "/v1/recipes/"+id, nil, ui(scott)...)
	h.want(r, 200, "")
	if et := r.header.Get("ETag"); et != `"1"` {
		t.Errorf("GET ETag = %q", et)
	}
	if r.json(t)["version"] != 1.0 {
		t.Errorf("version = %v", r.json(t)["version"])
	}

	// Jo saves first; Scott's save at version 1 is a conflict.
	h.want(h.do("PATCH", "/v1/recipes/"+id, map[string]any{"notes": "Lime."}, append(ui(jo), "If-Match", `"1"`)...), 200, "")
	r = h.do("PATCH", "/v1/recipes/"+id, map[string]any{"title": "Pad Thai (mine)"}, append(ui(scott), "If-Match", `"1"`)...)
	h.want(r, 412, "stale")
	body := r.json(t)
	wantKeys(t, "412 body", body, "error", "current")
	wantKeys(t, "412 error", body["error"].(map[string]any), "code", "message")
	cur := body["current"].(map[string]any)
	wantKeys(t, "current", cur, recipeKeys...)
	if cur["updated_by"] != jo || cur["version"] != 2.0 || cur["notes"] != "Lime." {
		t.Errorf("current = %v", cur)
	}

	// "Save mine over theirs" resends with current.version.
	r = h.do("PATCH", "/v1/recipes/"+id, map[string]any{"title": "Pad Thai (mine)"},
		append(ui(scott), "If-Match", fmt.Sprintf(`"%v"`, cur["version"]))...)
	h.want(r, 200, "")
	if r.header.Get("ETag") != `"3"` || r.json(t)["updated_by"] != scott {
		t.Errorf("save over: ETag %q, %s", r.header.Get("ETag"), r.body)
	}

	// Trash, then Undo from the toast.
	r = h.do("DELETE", "/v1/recipes/"+id, nil, append(ui(scott), "If-Match", `"3"`)...)
	h.want(r, 204, "")
	if len(r.body) != 0 {
		t.Errorf("204 with a body: %q", r.body)
	}
	r = h.do("POST", "/v1/recipes/"+id+"/restore", nil, ui(scott)...)
	h.want(r, 200, "")
	rec = r.json(t)
	wantKeys(t, "restored", rec, recipeKeys...)
	if v, ok := rec["deleted_at"]; !ok || v != nil {
		t.Errorf("restored deleted_at = %v (present %v)", v, ok)
	}

	// The heart and bookmark buttons.
	r = h.do("PUT", "/v1/recipes/"+id+"/mine", map[string]any{"favorite": true}, ui(scott)...)
	h.want(r, 200, "")
	mine := r.json(t)
	wantKeys(t, "mine", mine, "favorite", "want_to_cook")
	if mine["favorite"] != true || mine["want_to_cook"] != false {
		t.Errorf("mine = %v", mine)
	}
	if got := h.do("GET", "/v1/recipes/"+id, nil, ui(jo)...).json(t)["mine"].(map[string]any); got["favorite"] != false {
		t.Errorf("Jo sees Scott's favourite: %v", got)
	}

	r = h.do("GET", "/v1/tags", nil, ui(scott)...)
	h.want(r, 200, "")
	tags := r.json(t)["items"].([]any)
	if len(tags) != 2 {
		t.Fatalf("tags = %v", tags)
	}
	wantKeys(t, "tag", tags[0].(map[string]any), "name", "count")
}

func TestUIParsePreview(t *testing.T) {
	h := newHarness(t)
	// Editor.svelte sends blank lines as " " so indexes line up; a heading
	// line and an empty line are covered too. No person is needed.
	r := h.do("POST", "/v1/ingredients/parse", map[string]any{
		"lines": []string{"# For the sauce", "", " ", "2 tbsp fish sauce", "a pinch of salt"},
	}, ui("")...)
	h.want(r, 200, "")
	body := r.json(t)
	wantKeys(t, "parse body", body, "items")
	items := body["items"].([]any)
	if len(items) != 5 {
		t.Fatalf("items = %v", items)
	}
	lineKeys := []string{"raw", "quantity", "quantity_max", "unit", "item", "note", "parser_version"}

	heading := items[0].(map[string]any)
	wantKeys(t, "heading", heading, "section", "raw")
	if heading["section"] != "For the sauce" || heading["raw"] != "# For the sauce" {
		t.Errorf("heading = %v", heading)
	}
	for i := 1; i <= 2; i++ {
		blank := items[i].(map[string]any)
		wantKeys(t, "blank", blank, lineKeys...)
		if blank["raw"] != "" || blank["item"] != "" || blank["quantity"] != nil {
			t.Errorf("blank %d = %v", i, blank)
		}
	}
	line := items[3].(map[string]any)
	wantKeys(t, "line", line, lineKeys...)
	if line["quantity"] != 2.0 || line["unit"] != "tbsp" || line["item"] != "fish sauce" || line["note"] != nil {
		t.Errorf("line = %v", line)
	}
	if !strings.Contains(string(r.body), `"raw":"a pinch of salt"`) {
		t.Errorf("unparsed line missing: %s", r.body)
	}
}
