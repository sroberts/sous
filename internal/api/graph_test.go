package api

import (
	"testing"
)

// toastGraph is a valid step graph, as JSON, for a two-step toast recipe.
func toastGraph(recipeVersion, version int, status string) map[string]any {
	oven := map[string]any{"value": []any{map[string]any{"resource": "oven", "temp_f": 400}}}
	return map[string]any{
		"recipe_version": recipeVersion, "version": version, "status": status, "serve_item_id": "toast",
		"items": []any{
			map[string]any{"id": "bread", "kind": "ingredient", "label": "bread", "ingredient": 1},
			map[string]any{"id": "hot-oven", "kind": "intermediate", "label": "hot oven"},
			map[string]any{"id": "toast", "kind": "final", "label": "toast"},
		},
		"nodes": []any{
			map[string]any{"id": "preheat", "source": map[string]any{"step": 1, "start": 0, "end": 23}, "text": "Heat the oven",
				"action": map[string]any{"value": "setup"}, "outputs": []any{"hot-oven"},
				"duration":  map[string]any{"value": map[string]any{"min": 10, "typical": 15, "max": 15}},
				"attention": map[string]any{"value": "passive"}, "equipment": oven},
			map[string]any{"id": "toast", "source": map[string]any{"step": 2, "start": 0, "end": 15}, "text": "Toast the bread",
				"action": map[string]any{"value": "heat"}, "inputs": []any{"bread", "hot-oven"}, "outputs": []any{"toast"},
				"duration":  map[string]any{"value": map[string]any{"min": 4, "typical": 5, "max": 6}},
				"attention": map[string]any{"value": "passive", "source": "model", "confidence": 0.7}, "equipment": oven},
		},
	}
}

func TestRecipeGraphAPI(t *testing.T) {
	h := newHarness(t)
	ann := h.person("Ann")
	rec := h.create(ann, map[string]any{"title": "Toast", "ingredients": []string{"2 slices bread"},
		"steps": []string{"Heat the oven to 400°F.", "Toast the bread for 5 minutes."}})
	id := rec["id"].(string)
	rv := int(rec["version"].(float64))

	h.want(h.do("GET", "/v1/recipes/"+id+"/graph", nil), 404, "not_found")
	h.want(h.do("PUT", "/v1/recipes/"+id+"/graph", toastGraph(rv, 0, "verified")), 401, "who_are_you")

	// Checking a draft saves nothing and needs no person.
	r := h.do("POST", "/v1/recipes/"+id+"/graph/check", toastGraph(rv, 0, "draft"))
	h.want(r, 400, "invalid") // recipe_version and version aren't part of a graph
	draft := toastGraph(rv, 0, "draft")
	delete(draft, "recipe_version")
	delete(draft, "version")
	r = h.do("POST", "/v1/recipes/"+id+"/graph/check", draft)
	h.want(r, 200, "")
	if c := r.json(t); c["valid"] != true {
		t.Fatalf("check = %v", c)
	}

	r = h.do("PUT", "/v1/recipes/"+id+"/graph", toastGraph(rv, 0, "verified"), ann...)
	h.want(r, 200, "")
	if r.header.Get("ETag") != `"1"` {
		t.Errorf("ETag %q", r.header.Get("ETag"))
	}
	v := r.json(t)
	g := v["graph"].(map[string]any)
	node := g["nodes"].([]any)[1].(map[string]any)
	att := node["attention"].(map[string]any)
	if g["status"] != "verified" || att["confidence"] != 0.7 || att["reviewed_by"] != "ann" || node["source"].(map[string]any)["quote"] != "Toast the bread" {
		t.Errorf("saved = %v", v)
	}
	proj := v["projection"].([]any)
	if proj[0] != nil || proj[1].(map[string]any)["minutes"] != 5.0 {
		t.Errorf("projection = %v", proj)
	}

	r = h.do("GET", "/v1/recipes/"+id+"/graph", nil)
	h.want(r, 200, "")
	r = h.do("PUT", "/v1/recipes/"+id+"/graph", toastGraph(rv, 0, "draft"), ann...)
	h.want(r, 412, "stale")
	if cur := r.json(t)["current"].(map[string]any)["graph"].(map[string]any); cur["version"] != 1.0 || r.header.Get("ETag") != `"1"` {
		t.Errorf("stale current = %v", cur)
	}

	h.want(h.do("DELETE", "/v1/recipes/"+id+"/graph", nil, ann...), 428, "version_required")
	h.want(h.do("DELETE", "/v1/recipes/"+id+"/graph", nil, append(ann, "If-Match", `"1"`)...), 204, "")
	h.want(h.do("GET", "/v1/recipes/"+id+"/graph", nil), 404, "not_found")
}
