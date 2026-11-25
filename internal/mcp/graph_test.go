package mcp

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/sroberts/sous/internal/recipes"
)

func TestGraphTools(t *testing.T) {
	url := serve(t, standIn(New(newService(t), slog.New(slog.DiscardHandler), "test")))
	alice := as(t, url, "alice")
	r := ok[recipes.Recipe](t, alice, "create_recipe", map[string]any{
		"title": "Toast", "ingredients": []string{"2 slices bread"},
		"steps": []string{"Heat the oven to 400°F.", "Toast the bread for 5 minutes."},
	})
	oven := map[string]any{"value": []any{map[string]any{"resource": "oven", "temp_f": 400}}}
	graph := map[string]any{
		"status": "draft", "serve_item_id": "toast",
		"items": []any{
			map[string]any{"id": "bread", "kind": "ingredient", "label": "bread", "ingredient": 1},
			map[string]any{"id": "hot-oven", "kind": "intermediate", "label": "hot oven"},
			map[string]any{"id": "toast", "kind": "final", "label": "toast"},
		},
		"nodes": []any{
			map[string]any{"id": "preheat", "source": map[string]any{"step": 1, "start": 0, "end": 23}, "text": "Heat the oven",
				"action": map[string]any{"value": "setup", "source": "model", "confidence": 0.9}, "inputs": []any{}, "outputs": []any{"hot-oven"},
				"duration":  map[string]any{"value": map[string]any{"min": 10, "typical": 15, "max": 15}, "source": "model", "confidence": 0.8},
				"attention": map[string]any{"value": "passive", "source": "model", "confidence": 0.9}, "equipment": oven, "implied": false},
			map[string]any{"id": "toast", "source": map[string]any{"step": 2, "start": 0, "end": 15}, "text": "Toast the bread",
				"action": map[string]any{"value": "heat"}, "inputs": []any{"bread"}, "outputs": []any{"toast"},
				"duration":  map[string]any{"value": map[string]any{"min": 4, "typical": 5, "max": 6}},
				"attention": map[string]any{"value": "passive"}, "equipment": oven, "implied": false},
		},
	}
	chk := ok[recipes.GraphCheck](t, alice, "validate_recipe_graph", map[string]any{"id": r.ID, "graph": graph})
	if chk.Valid || len(chk.Errors) != 2 || chk.Errors[0].Code != "unused_output" {
		t.Fatalf("check = %+v", chk)
	}
	graph["nodes"].([]any)[1].(map[string]any)["inputs"] = []any{"bread", "hot-oven"}
	if chk := ok[recipes.GraphCheck](t, alice, "validate_recipe_graph", map[string]any{"id": r.ID, "graph": graph}); !chk.Valid {
		t.Fatalf("fixed check = %+v", chk)
	}

	set := map[string]any{"id": r.ID, "recipe_version": r.Version, "version": 0}
	for k, v := range graph {
		set[k] = v
	}
	v := ok[recipes.GraphView](t, alice, "set_recipe_graph", set)
	if v.Graph.Version != 1 || v.Graph.Nodes[0].Action.Source != "model" || !v.Check.Valid {
		t.Fatalf("set = %+v", v)
	}
	if msg := fails(t, alice, "set_recipe_graph", set, "stale"); !strings.Contains(msg, `"version":1`) {
		t.Errorf("stale set should carry the current graph: %q", msg)
	}
	got := ok[recipes.GraphView](t, alice, "get_recipe_graph", map[string]any{"id": r.ID})
	if got.Graph.Version != 1 || len(got.Graph.Nodes) != 2 {
		t.Errorf("get = %+v", got)
	}
	if saved := ok[recipes.GraphCheck](t, alice, "validate_recipe_graph", map[string]any{"id": r.ID}); !saved.Valid {
		t.Errorf("saved check = %+v", saved)
	}
	ok[struct{}](t, alice, "delete_recipe_graph", map[string]any{"id": r.ID, "version": 1})
	fails(t, alice, "get_recipe_graph", map[string]any{"id": r.ID}, "not_found")
}
