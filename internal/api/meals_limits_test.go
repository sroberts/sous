package api

import (
	"strconv"
	"testing"
)

// TestMealLimitsAPI covers the bounds that keep one meal from making the
// planner do unbounded work or leaving rows the plan never reads.
func TestMealLimitsAPI(t *testing.T) {
	h := newHarness(t)
	ann := h.person("Ann")
	steps := make([]string, 400)
	for i := range steps {
		steps[i] = "Stir " + strconv.Itoa(i) + "."
	}
	big := h.create(ann, map[string]any{"title": "Long", "steps": steps})
	small := h.create(ann, map[string]any{"title": "Short", "steps": []string{"Toast the bread."}})
	meal := func(items ...any) map[string]any {
		return map[string]any{"title": "M", "serve_at": "2026-11-26T17:00:00-05:00", "timezone": "America/New_York", "items": items}
	}

	// Two copies of a 400-step recipe is more than a meal plans.
	h.want(h.do("POST", "/v1/meals", meal(map[string]any{"recipe_id": big["id"]}, map[string]any{"recipe_id": big["id"]}), ann...), 400, "invalid")

	// An item id goes into task keys and URLs: only a safe alphabet.
	h.want(h.do("POST", "/v1/meals", meal(map[string]any{"recipe_id": small["id"], "id": "a/../b"}), ann...), 400, "invalid")

	r := h.do("POST", "/v1/meals", meal(map[string]any{"recipe_id": big["id"]}, map[string]any{"recipe_id": small["id"], "id": "toast_1"}), ann...)
	h.want(r, 201, "")
	m := r.json(t)
	id := m["id"].(string)
	h.want(h.do("GET", "/v1/meals/"+id+"/plan?now=2026-11-26T12:00:00Z", nil), 200, "")

	// A recipe that grows past the bound after joining the menu: the plan
	// refuses rather than computing.
	more := append(steps, steps[:250]...)
	h.want(h.do("PATCH", "/v1/recipes/"+big["id"].(string), map[string]any{"steps": more}, append(ann, "If-Match", `"1"`)...), 200, "")
	h.want(h.do("GET", "/v1/meals/"+id+"/plan", nil), 400, "invalid")

	// Task keys must name a step index, a reheat or a preheat of a real oven.
	for _, bad := range []string{"toast_1:x", "toast_1:-1", "toast_1:01", "toast_1:99999", "preheat:9:toast_1:0", "preheat:x:toast_1:0"} {
		h.want(h.do("PUT", "/v1/meals/"+id+"/tasks/"+bad, map[string]any{"done": true}, ann...), 404, "not_found")
	}

	// A pin far from the meal is refused; one on the day is fine.
	h.want(h.do("PATCH", "/v1/recipes/"+big["id"].(string), map[string]any{"steps": steps}, append(ann, "If-Match", `"2"`)...), 200, "")
	h.want(h.do("PUT", "/v1/meals/"+id+"/tasks/toast_1:0", map[string]any{"pinned_start": "9999-12-31T23:59:59Z"}, ann...), 400, "invalid")
	h.want(h.do("PUT", "/v1/meals/"+id+"/tasks/toast_1:0", map[string]any{"pinned_start": "0001-01-01T00:00:00Z"}, ann...), 400, "invalid")
	h.want(h.do("PUT", "/v1/meals/"+id+"/tasks/toast_1:0", map[string]any{"pinned_start": "2026-11-26T16:00:00-05:00"}, ann...), 200, "")
	h.want(h.do("PUT", "/v1/meals/"+id+"/tasks/preheat:0:toast_1:0", map[string]any{"started": true}, ann...), 200, "")
}
