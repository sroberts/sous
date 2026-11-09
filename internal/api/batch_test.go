package api

import (
	"fmt"
	"reflect"
	"testing"
)

func TestBatchRecipes(t *testing.T) {
	h := newHarness(t)
	scott := h.person("Scott")
	a := h.create(scott, map[string]any{"title": "Toast", "ingredients": []string{"2 slices bread"}})["id"].(string)
	b := h.create(scott, map[string]any{"title": "Eggs"})["id"].(string)
	gone := h.create(scott, map[string]any{"title": "Gone"})["id"].(string)
	h.want(h.do("DELETE", "/v1/recipes/"+gone, nil, append(scott, "If-Match", `"1"`)...), 204, "")
	h.want(h.do("PUT", "/v1/recipes/"+b+"/mine", map[string]any{"favorite": true}, scott...), 200, "")

	ids := func(r *result) (out []string) {
		for _, it := range r.json(t)["items"].([]any) {
			out = append(out, it.(map[string]any)["id"].(string))
		}
		return out
	}

	// In the order asked, once each, without the deleted or unknown ones.
	// Reading needs no person.
	r := h.do("POST", "/v1/recipes/batch", map[string]any{"ids": []string{b, gone, "nope", a, b}})
	h.want(r, 200, "")
	if got := ids(r); !reflect.DeepEqual(got, []string{b, a}) {
		t.Errorf("ids = %v, want [%s %s]", got, b, a)
	}

	// Full recipes, with the caller's flags.
	r = h.do("POST", "/v1/recipes/batch", map[string]any{"ids": []string{a, b}}, scott...)
	items := r.json(t)["items"].([]any)
	toast, eggs := items[0].(map[string]any), items[1].(map[string]any)
	wantKeys(t, "batch recipe", toast, recipeKeys...)
	if ing := toast["ingredients"].([]any); len(ing) != 1 || ing[0].(map[string]any)["item"] != "bread" {
		t.Errorf("ingredients = %v", toast["ingredients"])
	}
	if eggs["mine"].(map[string]any)["favorite"] != true || toast["mine"].(map[string]any)["favorite"] != false {
		t.Errorf("mine = %v, %v", eggs["mine"], toast["mine"])
	}

	h.want(h.do("POST", "/v1/recipes/batch", map[string]any{"ids": []string{}}), 200, "")
	if string(h.do("POST", "/v1/recipes/batch", map[string]any{}).body) != `{"items":[]}`+"\n" {
		t.Error("no ids should answer an empty list")
	}
	many := make([]string, 101)
	for i := range many {
		many[i] = fmt.Sprint(i)
	}
	h.want(h.do("POST", "/v1/recipes/batch", map[string]any{"ids": many}), 400, "invalid")
	h.want(h.do("POST", "/v1/recipes/batch", map[string]any{"ids": "x"}), 400, "invalid")
}
