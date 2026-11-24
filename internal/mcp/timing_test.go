package mcp

import (
	"log/slog"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sroberts/sous/internal/recipes"
)

func TestTimingTools(t *testing.T) {
	url := serve(t, standIn(New(newService(t), slog.New(slog.DiscardHandler), "test")))
	alice := as(t, url, "alice")

	stew := ok[recipes.Recipe](t, alice, "create_recipe", map[string]any{
		"title": "Beef stew", "steps": []string{"Brown the beef in batches.", "Cover and braise in the oven at 325°F for 2 to 3 hours."},
	})
	timed := ok[recipes.Recipe](t, alice, "create_recipe", map[string]any{
		"title": "Toast", "steps": []string{"Toast the bread."}, "step_timing": []any{map[string]any{"minutes": 3}},
	})
	meal := ok[recipes.Meal](t, alice, "create_meal", map[string]any{
		"title": "Supper", "serve_at": "2026-12-06T19:00:00Z", "timezone": "Europe/London",
		"items": []any{map[string]any{"recipe_id": stew.ID}, map[string]any{"recipe_id": timed.ID}},
	})

	todo := ok[recipes.TimingTodos](t, alice, "list_recipes_needing_timing", map[string]any{"meal_id": meal.ID})
	if todo.Remaining != 1 || len(todo.Items) != 1 || todo.Items[0].ID != stew.ID || todo.Items[0].Untimed != 2 {
		t.Fatalf("needing timing on the meal = %+v", todo)
	}

	v := ok[recipes.StepTimingView](t, alice, "get_step_timing", map[string]any{"id": stew.ID})
	if len(v.Steps) != 2 || v.Steps[1].Timing != nil || v.Steps[1].Guess.OvenTemp == nil || *v.Steps[1].Guess.OvenTemp != 325 {
		t.Fatalf("step timing = %+v", v)
	}

	set := ok[recipes.StepTimingView](t, alice, "set_step_timing", map[string]any{
		"id": stew.ID, "version": v.Version,
		"timing": []any{map[string]any{"minutes": 20}, map[string]any{"minutes": 180, "hands_on": false, "equipment": "oven", "oven_temp": 325}},
	})
	if set.Version != v.Version+1 || set.Steps[0].Text != "Brown the beef in batches." || set.Steps[1].Timing == nil || *set.Steps[1].Timing.Minutes != 180 {
		t.Fatalf("after set = %+v", set)
	}
	if left := ok[recipes.TimingTodos](t, alice, "list_recipes_needing_timing", map[string]any{"meal_id": meal.ID}); left.Remaining != 0 {
		t.Errorf("still needing timing = %+v", left)
	}
	fails(t, alice, "set_step_timing", map[string]any{"id": stew.ID, "version": v.Version, "timing": []any{nil, nil}}, "stale")
	fails(t, alice, "set_step_timing", map[string]any{"id": stew.ID, "version": set.Version, "timing": []any{nil}}, "invalid")
}

func TestFillTimingPrompt(t *testing.T) {
	url := serve(t, standIn(New(newService(t), slog.New(slog.DiscardHandler), "test")))
	cs := as(t, url, "alice")
	res, err := cs.GetPrompt(ctx, &sdk.GetPromptParams{Name: "fill_step_timing", Arguments: map[string]string{"meal": "Thanksgiving"}})
	if err != nil {
		t.Fatal(err)
	}
	text := res.Messages[0].Content.(*sdk.TextContent).Text
	for _, want := range []string{`"Thanksgiving"`, "set_step_timing", "never rewrite a step"} {
		if !strings.Contains(text, want) {
			t.Errorf("prompt lacks %q:\n%s", want, text)
		}
	}
}
