package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/recipes"
)

// Step graphs: a recipe's method as atomic actions joined by what they make
// and use, for planning several dishes at once. These tools read, write and
// check them; the recipe's own steps stay as they are.

type setGraphInput struct {
	ID            string `json:"id" jsonschema:"recipe ID"`
	RecipeVersion int    `json:"recipe_version" jsonschema:"the recipe's current version, from get_recipe or get_step_timing; spans are read against its words"`
	Version       int    `json:"version" jsonschema:"the graph's version from get_recipe_graph; 0 to make the recipe's first"`
	recipes.GraphInput
}

type deleteGraphInput struct {
	ID      string `json:"id" jsonschema:"recipe ID"`
	Version int    `json:"version" jsonschema:"the graph's version from get_recipe_graph"`
}

type validateGraphInput struct {
	ID    string              `json:"id" jsonschema:"recipe ID"`
	Graph *recipes.GraphInput `json:"graph,omitempty" jsonschema:"a proposed graph to check without saving; leave out to check the saved one"`
}

func registerGraph(s *sdk.Server, t *tools) {
	add(s, t, &sdk.Tool{
		Name:  "get_recipe_graph",
		Title: "Get a recipe's step graph",
		Description: "Get a recipe's step graph: its nodes (atomic actions, each pointing at the words of the step it comes " +
			"from, with action, duration range, attention, equipment, hold and timing anchor, each labelled with its source " +
			"and confidence), its items (ingredients, intermediates, the final dish), what the checks find in it against " +
			"the recipe and kitchen now, and the step timing it comes to. status is stale when the recipe's words have " +
			"changed under some nodes. Not found when the recipe has none.",
		Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: closedWorld},
	}, func(ctx context.Context, c identity.Caller, in recipeIDInput) (recipes.GraphView, error) {
		return t.svc.RecipeGraph(ctx, c, in.ID)
	})

	add(s, t, &sdk.Tool{
		Name:  "set_recipe_graph",
		Title: "Set a recipe's step graph",
		Description: "Save a recipe's whole step graph. Split each step into atomic actions; give each node the step number " +
			"(from 1) and the character span (from 0) of the words it comes from, an empty span only for an implied node " +
			"(a preheat, boiling water). Join nodes by items: a node's inputs are ingredients (with their line of the " +
			"ingredient list, from 1) or items another node outputs; every output must be used or be serve_item_id, the " +
			"final dish. Every oven node needs a setup node that preheats before it. Say each label's source and, for " +
			"model, a confidence. Status verified needs a graph with no errors and records the caller as reviewing every " +
			"label. Check a draft first with validate_recipe_graph. The recipe's steps don't change.",
		Annotations: &sdk.ToolAnnotations{DestructiveHint: ptr(true), IdempotentHint: true, OpenWorldHint: closedWorld},
	}, func(ctx context.Context, c identity.Caller, in setGraphInput) (recipes.GraphView, error) {
		return t.svc.SetRecipeGraph(ctx, c, in.ID, in.RecipeVersion, in.Version, in.GraphInput)
	})

	add(s, t, &sdk.Tool{
		Name:        "delete_recipe_graph",
		Title:       "Delete a recipe's step graph",
		Description: "Take a recipe's step graph away, so it plans by its step timing alone. Pass the graph's version from get_recipe_graph.",
		Annotations: &sdk.ToolAnnotations{DestructiveHint: ptr(true), OpenWorldHint: closedWorld},
	}, func(ctx context.Context, c identity.Caller, in deleteGraphInput) (struct{}, error) {
		return struct{}{}, t.svc.DeleteRecipeGraph(ctx, c, in.ID, in.Version)
	})

	add(s, t, &sdk.Tool{
		Name:  "validate_recipe_graph",
		Title: "Check a step graph",
		Description: "Run the step graph checks without saving: every input made by a node or from the ingredient list, " +
			"nothing made twice, no cycles, every output used or served, a preheat before every oven node, a check interval " +
			"on intermittent work, and (as warnings) equipment the kitchen lacks, steps with no nodes and changed words. " +
			"Pass graph to check a draft, or leave it out to check the saved graph.",
		Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: closedWorld},
	}, func(ctx context.Context, c identity.Caller, in validateGraphInput) (recipes.GraphCheck, error) {
		return t.svc.CheckRecipeGraph(ctx, c, in.ID, in.Graph)
	})
}
