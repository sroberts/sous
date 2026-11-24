package mcp

import (
	"context"
	"fmt"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/recipes"
)

// Step timing for recipes written before meal plans. Sous's own reading of
// a step is a regex; an assistant reads the words properly, so these tools
// let one fill timing in, recipe by recipe, without touching the steps.

type recipeIDInput struct {
	ID string `json:"id" jsonschema:"recipe ID"`
}

type setTimingInput struct {
	ID      string                `json:"id" jsonschema:"recipe ID"`
	Version int                   `json:"version" jsonschema:"the recipe's version from get_step_timing; a stale version is refused"`
	Timing  []*recipes.StepTiming `json:"timing" jsonschema:"one entry per step, in order, as numbered by get_step_timing; null leaves a step without timing"`
}

func registerTiming(s *sdk.Server, t *tools) {
	add(s, t, &sdk.Tool{
		Name:  "list_recipes_needing_timing",
		Title: "List recipes needing step timing",
		Description: "List recipes with steps that have no timing for meal plans yet, by title, in batches, with how many remain in all. " +
			"Pass meal_id for just the dishes on one meal. Fill each in with get_step_timing and set_step_timing; a recipe drops off " +
			"this list once every step has timing.",
		Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: closedWorld},
	}, func(ctx context.Context, c identity.Caller, in recipes.TimingTodoQuery) (recipes.TimingTodos, error) {
		return t.svc.RecipesNeedingTiming(ctx, c, in)
	})

	add(s, t, &sdk.Tool{
		Name:  "get_step_timing",
		Title: "Get a recipe's step timing",
		Description: "Get a recipe's steps, numbered, each with the timing set on it (or null) and Sous's guess from its words. " +
			"The guess is a regex's reading: a starting point, often wrong about what needs the cook, what can be made ahead, " +
			"and steps that say no time. Its version is what set_step_timing needs.",
		Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: closedWorld},
	}, func(ctx context.Context, c identity.Caller, in recipeIDInput) (recipes.StepTimingView, error) {
		return t.svc.StepTiming(ctx, c, in.ID)
	})

	add(s, t, &sdk.Tool{
		Name:  "set_step_timing",
		Title: "Set a recipe's step timing",
		Description: "Set the timing of every step of a recipe at once: one entry per step, in order, null to leave one without. " +
			"Only timing changes; the steps' words, and everything else in the recipe, stay as they are. Send the version from " +
			"get_step_timing. Returns the steps as get_step_timing does.",
		Annotations: &sdk.ToolAnnotations{DestructiveHint: ptr(false), IdempotentHint: true, OpenWorldHint: closedWorld},
	}, func(ctx context.Context, c identity.Caller, in setTimingInput) (recipes.StepTimingView, error) {
		return t.svc.SetStepTiming(ctx, c, in.ID, in.Version, in.Timing)
	})

	s.AddPrompt(&sdk.Prompt{
		Name:  "fill_step_timing",
		Title: "Fill in step timing",
		Description: "Read recipes' steps and set their timing for meal plans: minutes, whether they need the cook, " +
			"the oven and its temperature, and what can be made ahead. For the whole library, or one meal's dishes.",
		Arguments: []*sdk.PromptArgument{{
			Name:        "meal",
			Title:       "Meal",
			Description: "a meal's ID or name, for just its dishes; leave out for the whole library",
		}},
	}, func(ctx context.Context, req *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
		scope := "every recipe in the library that needs it"
		list := "list_recipes_needing_timing"
		if m := req.Params.Arguments["meal"]; m != "" {
			scope = fmt.Sprintf("the dishes on the meal %q", m)
			list = "list_recipes_needing_timing with that meal's meal_id (find it with list_meals if you were given a name)"
		}
		return &sdk.GetPromptResult{
			Description: "Fill in step timing for " + scope,
			Messages: []*sdk.PromptMessage{{
				Role:    "user",
				Content: &sdk.TextContent{Text: fmt.Sprintf(fillTimingPrompt, scope, list)},
			}},
		}, nil
	})
}

const fillTimingPrompt = `Fill in step timing in Sous for %s, so meal plans lay each recipe's steps out properly.

Work in batches: call %s, then for each recipe call get_step_timing, decide every step's timing, and save it with set_step_timing (the version from get_step_timing, one entry per step in order). Carry on until none remain, and tell me how many you did and anything you weren't sure of. Only timing changes; never rewrite a step.

For each step, read its words and the recipe as a whole:
- minutes: how long the step takes start to finish, as a cook would do it. A range takes its top. "Until golden" and the like: a sensible time for the amount. A step that's just a note can be 0.
- hands_on: true when the cook is busy throughout (chopping, stirring, searing); false when it looks after itself (roasting, simmering unattended, resting, chilling, rising, marinating). The guess often gets this wrong.
- equipment: "oven" with oven_temp in °F (convert °C, round to 5), "stovetop", or an appliance by its everyday name (slow cooker, grill, pressure cooker); leave out for none. Leave out a step that only preheats: Sous adds preheats itself.
- space: racks or burners taken at once, only when more than 1 (a turkey takes 2 racks; two pans, 2 burners).
- Make-ahead: on the step after which the dish can stop and wait, set hold_minutes (how long it keeps), hold_in (fridge, counter or warm), and, when it's brought back before serving, reheat_minutes and reheat_temp (null for out of the oven). Use the recipe's own words ("keeps 3 days", "make ahead") and kitchen sense; don't invent holds for dishes that must be served at once.

Sous's guess is a starting point, not an answer. If a recipe already has timing on some steps, keep what's there unless it's clearly wrong, and fill the rest.`
