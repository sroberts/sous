package api

import (
	"context"
	"log/slog"
	"testing"

	"github.com/sroberts/sous/internal/recipes"
	"github.com/sroberts/sous/internal/samples"
)

// TestSamplesThroughAPI: a seeded library shows the samples by Sous, but
// Sous is no one the API will act as or list.
func TestSamplesThroughAPI(t *testing.T) {
	h := newHarness(t)
	all, err := samples.All()
	if err != nil {
		t.Fatal(err)
	}
	n, err := recipes.New(h.st, slog.New(slog.DiscardHandler)).SeedSamples(context.Background(), all)
	if err != nil || n != len(all) {
		t.Fatalf("SeedSamples = %d, %v", n, err)
	}

	r := h.do("GET", "/v1/recipes", nil)
	h.want(r, 200, "")
	items := r.json(t)["items"].([]any)
	if len(items) != len(all) {
		t.Fatalf("%d recipes, want %d", len(items), len(all))
	}
	id := items[0].(map[string]any)["id"].(string)
	r = h.do("GET", "/v1/recipes/"+id, nil)
	h.want(r, 200, "")
	if m := r.json(t); m["created_by"] != "_sous" || m["updated_by"] != "_sous" {
		t.Errorf("created_by %v, updated_by %v", m["created_by"], m["updated_by"])
	}

	r = h.do("GET", "/v1/people", nil)
	h.want(r, 200, "")
	if string(r.body) != `{"items":[]}`+"\n" {
		t.Errorf("people = %s", r.body)
	}
	h.want(h.do("GET", "/v1/me", nil, "X-Sous-Person", "_sous"), 401, "unknown_person")
	h.want(h.do("POST", "/v1/recipes", map[string]any{"title": "X"}, "X-Sous-Person", "_sous"), 401, "unknown_person")
	h.want(h.do("GET", "/v1/me", nil, "Cookie", "sous_person=_sous"), 401, "who_are_you")

	r = h.do("GET", "/v1/changes?via=system", nil)
	h.want(r, 200, "")
	changes := r.json(t)["items"].([]any)
	if len(changes) != len(all) {
		t.Fatalf("%d system changes, want %d", len(changes), len(all))
	}
	for _, c := range changes {
		if c := c.(map[string]any); c["via"] != "system" || c["person_id"] != "_sous" || c["action"] != "create" {
			t.Errorf("change = %v", c)
		}
	}
	h.want(h.do("GET", "/v1/changes?via=fax", nil), 400, "invalid")

	// Someone who joins can edit and delete a sample like any recipe.
	scott := h.person("Scott")
	r = h.do("PATCH", "/v1/recipes/"+id, map[string]any{"notes": "Ours now."}, append(scott, "If-Match", `"1"`)...)
	h.want(r, 200, "")
	if m := r.json(t); m["created_by"] != "_sous" || m["updated_by"] != "scott" {
		t.Errorf("after edit: created_by %v, updated_by %v", m["created_by"], m["updated_by"])
	}
	h.want(h.do("DELETE", "/v1/recipes/"+id, nil, append(scott, "If-Match", `"2"`)...), 204, "")
	r = h.do("GET", "/v1/people", nil)
	if items := r.json(t)["items"].([]any); len(items) != 1 {
		t.Errorf("people = %v", items)
	}
}
