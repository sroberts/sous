package mcp

import (
	"context"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/recipes"
)

type mealIDInput struct {
	ID string `json:"id" jsonschema:"meal ID"`
}

type mealVersionInput struct {
	ID      string `json:"id" jsonschema:"meal ID"`
	Version int    `json:"version" jsonschema:"the meal's version from get_meal; a stale version is refused"`
}

type updateMealInput struct {
	ID      string `json:"id" jsonschema:"meal ID"`
	Version int    `json:"version" jsonschema:"the meal's version from get_meal; a stale version is refused"`
	recipes.MealPatch
}

type planInput struct {
	ID  string `json:"id" jsonschema:"meal ID"`
	Now string `json:"now,omitempty" jsonschema:"plan as if it were this time (RFC 3339) instead of now"`
}

type taskInput struct {
	ID   string `json:"id" jsonschema:"meal ID"`
	Task string `json:"task" jsonschema:"the task's key from plan_meal"`
	recipes.TaskPatch
}

type kitchenInput struct {
	Version int `json:"version" jsonschema:"the kitchen's version from get_kitchen; a stale version is refused"`
	recipes.KitchenProfile
}

func registerMeals(s *sdk.Server, t *tools) {
	add(s, t, &sdk.Tool{
		Name:  "get_kitchen",
		Title: "Get the kitchen",
		Description: "Get the household's kitchen as meal planning sees it: ovens and how many racks each holds, burners, " +
			"other appliances by the name steps use (slow cooker, grill), serving dishes, and the hours of the day hands-on " +
			"work is planned in. Its version is what update_kitchen needs.",
		Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: closedWorld},
	}, func(ctx context.Context, c identity.Caller, _ struct{}) (recipes.Kitchen, error) {
		return t.svc.GetKitchen(ctx, c)
	})

	add(s, t, &sdk.Tool{
		Name:  "update_kitchen",
		Title: "Update the kitchen",
		Description: "Replace the kitchen: send every field (start from get_kitchen) with its version. An appliance's name " +
			"must match the equipment steps name, such as \"slow cooker\". Returns the kitchen.",
		Annotations: &sdk.ToolAnnotations{DestructiveHint: ptr(true), IdempotentHint: true, OpenWorldHint: closedWorld},
	}, func(ctx context.Context, c identity.Caller, in kitchenInput) (recipes.Kitchen, error) {
		return t.svc.UpdateKitchen(ctx, c, in.Version, in.KitchenProfile)
	})

	add(s, t, &sdk.Tool{
		Name:        "list_meals",
		Title:       "List meals",
		Description: "List planned meals (a holiday dinner, say: several recipes served together), soonest first, excluding deleted ones.",
		Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: closedWorld},
	}, func(ctx context.Context, c identity.Caller, _ struct{}) (recipes.Items[recipes.MealSummary], error) {
		ms, err := t.svc.ListMeals(ctx, c)
		return recipes.Items[recipes.MealSummary]{Items: ms}, err
	})

	add(s, t, &sdk.Tool{
		Name:        "get_meal",
		Title:       "Get a meal",
		Description: "Get one meal: when it's served, the kitchen's timezone, how many cooks, how many prep days, and its menu with each dish's serving offset and serving dish. Its version is what update_meal and delete_meal need.",
		Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: closedWorld},
	}, func(ctx context.Context, c identity.Caller, in mealIDInput) (recipes.Meal, error) {
		return t.svc.GetMeal(ctx, c, in.ID)
	})

	add(s, t, &sdk.Tool{
		Name:  "create_meal",
		Title: "Plan a meal",
		Description: "Plan a meal: several recipes served together. serve_at is RFC 3339 with an offset; timezone is the " +
			"kitchen's IANA name. Each item is a recipe_id, with serve_offset_minutes for a dish served later (dessert at 90) " +
			"and a serving dish named as in the kitchen. Then call plan_meal for the timeline.",
		Annotations: &sdk.ToolAnnotations{DestructiveHint: ptr(false), OpenWorldHint: closedWorld},
	}, func(ctx context.Context, c identity.Caller, in recipes.MealInput) (recipes.Meal, error) {
		return t.svc.CreateMeal(ctx, c, in)
	})

	add(s, t, &sdk.Tool{
		Name:  "update_meal",
		Title: "Update a meal",
		Description: "Change some fields of a meal. Pass id and version from get_meal, plus only the fields to change. items " +
			"replaces the whole menu: pass each existing item's id back to keep its progress. A stale version returns the " +
			"meal as it stands; reapply your change and retry.",
		Annotations: &sdk.ToolAnnotations{DestructiveHint: ptr(true), OpenWorldHint: closedWorld},
	}, func(ctx context.Context, c identity.Caller, in updateMealInput) (recipes.Meal, error) {
		return t.svc.UpdateMeal(ctx, c, in.ID, in.Version, in.MealPatch)
	})

	add(s, t, &sdk.Tool{
		Name:        "delete_meal",
		Title:       "Delete a meal",
		Description: "Move a meal to the trash. Pass id and version from get_meal. restore_meal brings it back.",
		Annotations: &sdk.ToolAnnotations{DestructiveHint: ptr(true), OpenWorldHint: closedWorld},
	}, func(ctx context.Context, c identity.Caller, in mealVersionInput) (recipes.Meal, error) {
		if err := t.svc.DeleteMeal(ctx, c, in.ID, in.Version); err != nil {
			return recipes.Meal{}, err
		}
		return t.svc.GetMeal(ctx, c, in.ID)
	})

	add(s, t, &sdk.Tool{
		Name:        "restore_meal",
		Title:       "Restore a meal",
		Description: "Bring a deleted meal back from the trash. Restoring one that isn't deleted changes nothing.",
		Annotations: &sdk.ToolAnnotations{DestructiveHint: ptr(false), IdempotentHint: true, OpenWorldHint: closedWorld},
	}, func(ctx context.Context, c identity.Caller, in mealIDInput) (recipes.Meal, error) {
		return t.svc.RestoreMeal(ctx, c, in.ID)
	})

	add(s, t, &sdk.Tool{
		Name:  "plan_meal",
		Title: "Plan a meal's timeline",
		Description: "Lay a meal out in time: every step of every dish with its start and end, fitted around the cooks' hands, " +
			"each oven's racks and single temperature (with preheats added), burners and appliances. Day-of work lands just " +
			"as each dish is served; make-ahead work (steps with hold_minutes) moves into the prep days. Tasks already " +
			"started or done stay put, and anything late moves to now (on_time and behind_minutes say how it stands). " +
			"Read warnings: guessed timings, clashes, serving dishes. Times are UTC; give them to people in the meal's timezone.",
		Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: closedWorld},
	}, func(ctx context.Context, c identity.Caller, in planInput) (recipes.Plan, error) {
		now := time.Now()
		if in.Now != "" {
			v, err := time.Parse(time.RFC3339, in.Now)
			if err != nil {
				return recipes.Plan{}, recipes.Errorf(recipes.CodeInvalid, "now must be an RFC 3339 time")
			}
			now = v
		}
		return t.svc.MealPlan(ctx, c, in.ID, now)
	})

	add(s, t, &sdk.Tool{
		Name:  "update_meal_task",
		Title: "Update a meal task",
		Description: "Start, tick off or pin one task of a meal, by its key from plan_meal: started true starts it now, done " +
			"true ticks it off now (false takes either back), and pinned_start fixes when it starts so the plan works around " +
			"it (null unpins). Returns the new plan.",
		Annotations: &sdk.ToolAnnotations{DestructiveHint: ptr(false), IdempotentHint: true, OpenWorldHint: closedWorld},
	}, func(ctx context.Context, c identity.Caller, in taskInput) (recipes.Plan, error) {
		return t.svc.UpdateTask(ctx, c, in.ID, in.Task, in.TaskPatch)
	})
}
