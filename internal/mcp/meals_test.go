package mcp

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/sroberts/sous/internal/recipes"
)

func TestMealTools(t *testing.T) {
	url := serve(t, standIn(New(newService(t), slog.New(slog.DiscardHandler), "test")))
	alice := as(t, url, "alice")

	k := ok[recipes.Kitchen](t, alice, "get_kitchen", map[string]any{})
	k2 := ok[recipes.Kitchen](t, alice, "update_kitchen", map[string]any{
		"version": k.Version, "ovens": []any{map[string]any{"name": "Oven", "racks": 3}}, "burners": 6,
		"appliances": []any{}, "dishes": []any{map[string]any{"name": "Platter", "count": 1}},
		"day_start": "08:00", "day_end": "22:00",
	})
	if k2.Version != k.Version+1 || k2.Ovens[0].Racks != 3 {
		t.Fatalf("kitchen = %+v", k2)
	}
	if msg := fails(t, alice, "update_kitchen", map[string]any{
		"version": k.Version, "ovens": []any{}, "burners": 4, "appliances": []any{}, "dishes": []any{},
		"day_start": "08:00", "day_end": "22:00",
	}, "stale"); !strings.Contains(msg, `"racks":3`) {
		t.Errorf("stale kitchen should carry the current one: %q", msg)
	}

	roast := ok[recipes.Recipe](t, alice, "create_recipe", map[string]any{
		"title": "Roast chicken", "steps": []string{"Season.", "Roast."},
		"step_timing": []any{map[string]any{"minutes": 10}, map[string]any{"minutes": 75, "hands_on": false, "equipment": "oven", "oven_temp": 425}},
	})
	meal := ok[recipes.Meal](t, alice, "create_meal", map[string]any{
		"title": "Sunday lunch", "serve_at": "2026-12-06T13:00:00Z", "timezone": "Europe/London",
		"items": []any{map[string]any{"recipe_id": roast.ID, "dish": "Platter"}},
	})
	if len(ok[recipes.Items[recipes.MealSummary]](t, alice, "list_meals", map[string]any{}).Items) != 1 {
		t.Error("meal not listed")
	}
	plan := ok[recipes.Plan](t, alice, "plan_meal", map[string]any{"id": meal.ID, "now": "2026-12-01T09:00:00Z"})
	if !plan.OnTime || len(plan.Tasks) != 3 || plan.Tasks[len(plan.Tasks)-1].End != "2026-12-06T13:00:00.000Z" {
		t.Fatalf("plan = %+v", plan)
	}
	key := meal.Items[0].ID + ":0"
	plan = ok[recipes.Plan](t, alice, "update_meal_task", map[string]any{"id": meal.ID, "task": key, "done": true})
	for _, task := range plan.Tasks {
		if task.Key == key && task.Status != recipes.StatusDone {
			t.Errorf("task = %+v", task)
		}
	}

	m2 := ok[recipes.Meal](t, alice, "update_meal", map[string]any{"id": meal.ID, "version": meal.Version, "cooks": 2})
	if m2.Cooks != 2 {
		t.Errorf("updated = %+v", m2)
	}
	if msg := fails(t, alice, "update_meal", map[string]any{"id": meal.ID, "version": meal.Version, "cooks": 3}, "stale"); !strings.Contains(msg, `"cooks":2`) {
		t.Errorf("stale meal should carry the current one: %q", msg)
	}
	gone := ok[recipes.Meal](t, alice, "delete_meal", map[string]any{"id": meal.ID, "version": m2.Version})
	if gone.DeletedAt == nil {
		t.Error("not deleted")
	}
	if back := ok[recipes.Meal](t, alice, "restore_meal", map[string]any{"id": meal.ID}); back.DeletedAt != nil {
		t.Error("not restored")
	}
	if got := ok[recipes.Meal](t, alice, "get_meal", map[string]any{"id": meal.ID}); got.Title != "Sunday lunch" {
		t.Errorf("get = %+v", got)
	}
}
