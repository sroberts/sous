package api

import (
	"testing"
)

func TestKitchenAPI(t *testing.T) {
	h := newHarness(t)
	ann := h.person("Ann")

	r := h.do("GET", "/v1/kitchen", nil)
	h.want(r, 200, "")
	k := r.json(t)
	if r.header.Get("ETag") != `"1"` || k["burners"] != 4.0 {
		t.Fatalf("kitchen = %v, ETag %q", k, r.header.Get("ETag"))
	}
	profile := map[string]any{
		"ovens":   []any{map[string]any{"name": "Main", "racks": 2}, map[string]any{"name": "Wall", "racks": 1}},
		"burners": 5, "appliances": []any{map[string]any{"name": "Slow cooker", "count": 1}},
		"dishes":    []any{map[string]any{"name": "Platter", "count": 1}},
		"day_start": "07:30", "day_end": "21:00",
	}
	h.want(h.do("PUT", "/v1/kitchen", profile, ann...), 428, "version_required")
	h.want(h.do("PUT", "/v1/kitchen", profile), 401, "who_are_you")
	r = h.do("PUT", "/v1/kitchen", profile, append(ann, "If-Match", `"1"`)...)
	h.want(r, 200, "")
	if k := r.json(t); k["version"] != 2.0 || len(k["ovens"].([]any)) != 2 {
		t.Errorf("updated = %v", k)
	}
	r = h.do("PUT", "/v1/kitchen", profile, append(ann, "If-Match", `"1"`)...)
	h.want(r, 412, "stale")
	if cur := r.json(t)["current"].(map[string]any); cur["version"] != 2.0 {
		t.Errorf("stale current = %v", cur)
	}
}

func TestSuggestAPI(t *testing.T) {
	h := newHarness(t)
	r := h.do("POST", "/v1/steps/suggest", map[string]any{"steps": []string{"Preheat to 425°F.", "Roast 40 minutes."}})
	h.want(r, 200, "")
	items := r.json(t)["items"].([]any)
	roast := items[1].(map[string]any)
	if roast["minutes"] != 40.0 || roast["oven_temp"] != 425.0 || roast["equipment"] != "oven" {
		t.Errorf("suggested = %v", items)
	}
}

func TestMealAPI(t *testing.T) {
	h := newHarness(t)
	ann := h.person("Ann")
	turkey := h.create(ann, map[string]any{
		"title": "Turkey",
		"steps": []string{"Season.", "Roast.", "Carve."},
		"step_timing": []any{
			map[string]any{"minutes": 20},
			map[string]any{"minutes": 240, "hands_on": false, "equipment": "oven", "oven_temp": 325, "space": 2},
			map[string]any{"minutes": 15},
		},
	})
	steps := turkey["steps"].([]any)
	if tm, ok := steps[1].(map[string]any)["timing"].(map[string]any); !ok || tm["oven_temp"] != 325.0 {
		t.Fatalf("steps = %v", steps)
	}
	pie := h.create(ann, map[string]any{"title": "Pie", "steps": []string{"Bake at 400°F for 50 minutes.", "Cool; it keeps up to 1 day ahead."}})

	h.want(h.do("POST", "/v1/meals", map[string]any{"title": "T", "serve_at": "2026-11-26T17:00:00-05:00", "timezone": "America/New_York"}), 401, "who_are_you")
	h.want(h.do("POST", "/v1/meals", map[string]any{"title": "T", "serve_at": "soon", "timezone": "America/New_York"}, ann...), 400, "invalid")
	r := h.do("POST", "/v1/meals", map[string]any{
		"title": "Thanksgiving", "serve_at": "2026-11-26T17:00:00-05:00", "timezone": "America/New_York",
		"items": []any{map[string]any{"recipe_id": turkey["id"]}, map[string]any{"recipe_id": pie["id"], "serve_offset_minutes": 90}},
	}, ann...)
	h.want(r, 201, "")
	meal := r.json(t)
	id := meal["id"].(string)
	items := meal["items"].([]any)
	turkeyItem := items[0].(map[string]any)["id"].(string)

	r = h.do("GET", "/v1/meals", nil)
	h.want(r, 200, "")
	if list := r.json(t)["items"].([]any); len(list) != 1 {
		t.Errorf("meals = %v", list)
	}

	r = h.do("GET", "/v1/meals/"+id+"/plan?now=2026-11-20T12:00:00Z", nil)
	h.want(r, 200, "")
	plan := r.json(t)
	if plan["on_time"] != true || len(plan["tasks"].([]any)) < 5 {
		t.Fatalf("plan = %v", plan)
	}
	h.want(h.do("GET", "/v1/meals/"+id+"/plan?now=later", nil), 400, "invalid")
	h.want(h.do("GET", "/v1/meals/nope/plan", nil), 404, "not_found")

	// Tick off a task; the plan comes back with it done.
	r = h.do("PUT", "/v1/meals/"+id+"/tasks/"+turkeyItem+":0", map[string]any{"done": true}, ann...)
	h.want(r, 200, "")
	var done bool
	for _, x := range r.json(t)["tasks"].([]any) {
		task := x.(map[string]any)
		if task["key"] == turkeyItem+":0" {
			done = task["status"] == "done" && task["done_by"] == "ann"
		}
	}
	if !done {
		t.Errorf("task not done")
	}
	h.want(h.do("PUT", "/v1/meals/"+id+"/tasks/nope:1", map[string]any{"done": true}, ann...), 404, "not_found")
	h.want(h.do("POST", "/v1/meals/"+id+"/reset", nil, ann...), 200, "")

	// Edit, with concurrency.
	h.want(h.do("PATCH", "/v1/meals/"+id, map[string]any{"cooks": 2}, ann...), 428, "version_required")
	r = h.do("PATCH", "/v1/meals/"+id, map[string]any{"cooks": 2, "prep_days": 2}, append(ann, "If-Match", `"1"`)...)
	h.want(r, 200, "")
	if m := r.json(t); m["cooks"] != 2.0 || m["version"] != 2.0 {
		t.Errorf("patched = %v", m)
	}
	r = h.do("PATCH", "/v1/meals/"+id, map[string]any{"cooks": 3}, append(ann, "If-Match", `"1"`)...)
	h.want(r, 412, "stale")
	if cur := r.json(t)["current"].(map[string]any); cur["cooks"] != 2.0 {
		t.Errorf("stale current = %v", cur)
	}

	// Delete and restore.
	h.want(h.do("DELETE", "/v1/meals/"+id, nil, append(ann, "If-Match", `"2"`)...), 204, "")
	h.want(h.do("PATCH", "/v1/meals/"+id, map[string]any{"cooks": 3}, append(ann, "If-Match", `"3"`)...), 409, "deleted")
	r = h.do("POST", "/v1/meals/"+id+"/restore", nil, ann...)
	h.want(r, 200, "")
	if r.json(t)["deleted_at"] != nil {
		t.Errorf("still deleted")
	}

	// A step whose text is unchanged keeps its timing when steps are edited.
	r = h.do("PATCH", "/v1/recipes/"+turkey["id"].(string), map[string]any{"steps": []string{"Season well.", "Roast.", "Carve."}},
		append(ann, "If-Match", `"1"`)...)
	h.want(r, 200, "")
	steps = r.json(t)["steps"].([]any)
	if _, has := steps[0].(map[string]any)["timing"]; has {
		t.Errorf("an edited step kept its timing: %v", steps[0])
	}
	if _, has := steps[1].(map[string]any)["timing"]; !has {
		t.Errorf("an unchanged step lost its timing: %v", steps[1])
	}
}
