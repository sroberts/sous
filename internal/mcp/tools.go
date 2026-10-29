package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/recipes"
)

type idInput struct {
	ID string `json:"id" jsonschema:"recipe ID"`
}

type versionInput struct {
	ID      string `json:"id" jsonschema:"recipe ID"`
	Version int    `json:"version" jsonschema:"the recipe's version from get_recipe; a stale version is refused"`
}

type updateInput struct {
	ID      string `json:"id" jsonschema:"recipe ID"`
	Version int    `json:"version" jsonschema:"the recipe's version from get_recipe; a stale version is refused"`
	recipes.RecipePatch
}

type flagsInput struct {
	ID string `json:"id" jsonschema:"recipe ID"`
	recipes.MinePatch
}

func ptr[T any](v T) *T { return &v }

// Every tool works only on this household's recipes.
var closedWorld = ptr(false)

func register(s *sdk.Server, t *tools) {
	add(s, t, &sdk.Tool{
		Name:  "search_recipes",
		Title: "Search recipes",
		Description: "Search the household's recipes, excluding deleted ones. All filters are optional and combine with AND: " +
			"q is full-text over title, description, notes, ingredients and tags (words match as prefixes); " +
			"tag and ingredient each require every value given; favorite and want_to_cook use the caller's own flags. " +
			"Results are summaries sorted by title unless sort is updated or created. " +
			"When next_cursor is not null, pass it back as cursor for the next page. Use get_recipe for the full recipe.",
		Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: closedWorld},
	}, func(ctx context.Context, c identity.Caller, in recipes.ListQuery) (recipes.Page[recipes.Summary], error) {
		return t.svc.List(ctx, c, in)
	})

	add(s, t, &sdk.Tool{
		Name:  "get_recipe",
		Title: "Get a recipe",
		Description: "Get one recipe in full: parsed ingredients and steps (each with the section heading it sits under), " +
			"tags, times, notes, the caller's own flags, and its version, which update_recipe and delete_recipe need. " +
			"A recipe in the trash is still returned, with deleted_at set.",
		Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: closedWorld},
	}, func(ctx context.Context, c identity.Caller, in idInput) (recipes.Recipe, error) {
		return t.svc.Get(ctx, c, in.ID)
	})

	add(s, t, &sdk.Tool{
		Name:  "create_recipe",
		Title: "Create a recipe",
		Description: "Add a recipe. Only title is required. Ingredients and steps are lists of raw lines as a person would write them " +
			"(\"2 tbsp olive oil, divided\"); Sous parses quantities and units itself. A line starting with # is a section heading " +
			"(\"# For the sauce\") that groups the lines after it. The first tag is shown as the course. " +
			"servings is derived from yield_text. Returns the new recipe with its ID and version.",
		Annotations: &sdk.ToolAnnotations{DestructiveHint: ptr(false), OpenWorldHint: closedWorld},
	}, func(ctx context.Context, c identity.Caller, in recipes.RecipeInput) (recipes.Recipe, error) {
		return t.svc.Create(ctx, c, in)
	})

	add(s, t, &sdk.Tool{
		Name:  "update_recipe",
		Title: "Update a recipe",
		Description: "Change some fields of a recipe. Pass id and the version from get_recipe, plus only the fields to change: " +
			"an omitted field is left alone and null clears it. ingredients, steps, tags and images replace the whole list, " +
			"so send the complete new list (raw lines; a line starting with # is a section heading). " +
			"If the recipe changed since you read it, the result is a stale error that includes the current recipe; " +
			"reapply your change to it and retry with its version. Returns the updated recipe.",
		Annotations: &sdk.ToolAnnotations{DestructiveHint: ptr(true), OpenWorldHint: closedWorld},
	}, func(ctx context.Context, c identity.Caller, in updateInput) (recipes.Recipe, error) {
		return t.svc.Update(ctx, c, in.ID, in.Version, in.RecipePatch)
	})

	add(s, t, &sdk.Tool{
		Name:  "delete_recipe",
		Title: "Delete a recipe",
		Description: "Move a recipe to the trash. Pass id and the version from get_recipe. " +
			"The delete is soft: restore_recipe brings it back. Returns the recipe with deleted_at set.",
		Annotations: &sdk.ToolAnnotations{DestructiveHint: ptr(true), OpenWorldHint: closedWorld},
	}, func(ctx context.Context, c identity.Caller, in versionInput) (recipes.Recipe, error) {
		if err := t.svc.Delete(ctx, c, in.ID, in.Version); err != nil {
			return recipes.Recipe{}, err
		}
		return t.svc.Get(ctx, c, in.ID)
	})

	add(s, t, &sdk.Tool{
		Name:        "restore_recipe",
		Title:       "Restore a recipe",
		Description: "Bring a deleted recipe back from the trash. Restoring one that isn't deleted changes nothing. Returns the recipe.",
		Annotations: &sdk.ToolAnnotations{DestructiveHint: ptr(false), IdempotentHint: true, OpenWorldHint: closedWorld},
	}, func(ctx context.Context, c identity.Caller, in idInput) (recipes.Recipe, error) {
		return t.svc.Restore(ctx, c, in.ID)
	})

	add(s, t, &sdk.Tool{
		Name:  "set_my_recipe_flags",
		Title: "Set my recipe flags",
		Description: "Set the caller's own favorite and want_to_cook flags on a recipe. Each person has their own flags. " +
			"An omitted flag keeps its value. Needs no version and isn't recorded in the change log. Returns both flags.",
		Annotations: &sdk.ToolAnnotations{DestructiveHint: ptr(false), IdempotentHint: true, OpenWorldHint: closedWorld},
	}, func(ctx context.Context, c identity.Caller, in flagsInput) (recipes.Mine, error) {
		return t.svc.SetMine(ctx, c, in.ID, in.MinePatch)
	})

	add(s, t, &sdk.Tool{
		Name:        "list_tags",
		Title:       "List tags",
		Description: "List every tag in use on recipes that aren't deleted, with how many recipes carry it, most used first.",
		Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: closedWorld},
	}, func(ctx context.Context, c identity.Caller, _ struct{}) (recipes.Items[recipes.TagCount], error) {
		tags, err := t.svc.Tags(ctx, c)
		return recipes.Items[recipes.TagCount]{Items: tags}, err
	})

	add(s, t, &sdk.Tool{
		Name:  "recent_changes",
		Title: "Recent changes",
		Description: "Read the change log, newest first: who created, edited, deleted or restored which recipe, when, " +
			"and whether through the web UI, the API or MCP. Filter by entity_id (a recipe ID), person (a username) or via. " +
			"Use it to see what changed or to find a recipe to restore.",
		Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: closedWorld},
	}, func(ctx context.Context, c identity.Caller, in recipes.ChangeQuery) (recipes.ChangePage, error) {
		return t.svc.Changes(ctx, c, in)
	})
}
