package api

import (
	"net/http"
	"reflect"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/recipes"
)

// upgradeBody is the body of POST /v1/recipes/{id}/graph/upgrade.
type upgradeBody struct {
	Mode  string `json:"mode,omitempty" jsonschema:"draft (unset) or apply"`
	Force bool   `json:"force,omitempty" jsonschema:"redo a recipe that already has a step graph"`
}

// draftBody is the body of POST /v1/recipes/{id}/graph/draft.
type draftBody struct {
	RecipeVersion int    `json:"recipe_version" jsonschema:"the recipe's version the quotes were read from"`
	Mode          string `json:"mode,omitempty" jsonschema:"draft (unset) or apply"`
	Force         bool   `json:"force,omitempty" jsonschema:"replace the recipe's step graph if it has one"`
	recipes.StructureDraft
}

// graphTodoQuery is GET /v1/graphs/todo's query.
type graphTodoQuery struct {
	MealID string `query:"meal_id" jsonschema:"only the dishes on this meal"`
	Limit  int    `query:"limit" jsonschema:"default 20, at most 100"`
}

func (a *api) upgradeEndpoints() []endpoint {
	return []endpoint{
		{Method: "POST", Path: "/v1/recipes/{id}/graph/upgrade", OpID: "upgradeRecipe",
			Summary: "Turn a recipe into a step graph with Sous's local models. Takes tens of seconds; 503 when the models are off or unreachable",
			Body:    jsonOf[upgradeBody](), Resp: jsonOf[recipes.UpgradeResult](),
			Errors: []int{404, 409, 412, 503}, Write: true, Limit: jsonLimit, Handle: a.upgradeRecipe},
		{Method: "POST", Path: "/v1/recipes/{id}/graph/draft", OpID: "draftRecipeGraph",
			Summary: "Send a step graph's structure for Sous to check, classify with its decision model and save; a draft with problems is 400 with them listed",
			Body:    jsonOf[draftBody](), Resp: jsonOf[recipes.UpgradeResult](),
			Errors: []int{404, 409, 412, 503}, Write: true, Limit: jsonLimit, Handle: a.draftRecipeGraph},
		{Method: "GET", Path: "/v1/graphs/todo", OpID: "listRecipesWithoutGraph",
			Summary: "Recipes with steps and no step graph, by title, with how many remain",
			Query:   reflect.TypeFor[graphTodoQuery](), Resp: jsonOf[recipes.GraphTodos](), Errors: []int{404}, Handle: a.listRecipesWithoutGraph},
		{Method: "POST", Path: "/v1/upgrades", OpID: "queueUpgrades",
			Summary: "Queue recipes to be upgraded to step graphs in the background",
			Body:    jsonOf[recipes.UpgradeJobRequest](), Resp: jsonOf[recipes.UpgradeJob](), Status: http.StatusCreated,
			Errors: []int{503}, Write: true, Limit: jsonLimit, Handle: a.queueUpgrades},
		{Method: "GET", Path: "/v1/upgrades/{id}", OpID: "getUpgradeJob",
			Summary: "A background upgrade and how far it has got",
			Resp:    jsonOf[recipes.UpgradeJob](), Errors: []int{404}, Handle: a.getUpgradeJob},
		{Method: "POST", Path: "/v1/upgrades/{id}/cancel", OpID: "cancelUpgradeJob",
			Summary: "Stop a background upgrade; recipes not started are skipped",
			Resp:    jsonOf[recipes.UpgradeJob](), Errors: []int{404}, Write: true, Handle: a.cancelUpgradeJob},
		{Method: "GET", Path: "/v1/review", OpID: "listReviewItems",
			Summary: "Step graph labels to confirm or correct, most consequential first",
			Query:   reflect.TypeFor[recipes.ReviewQuery](), Resp: jsonOf[recipes.ReviewPage](), Handle: a.listReviewItems},
		{Method: "POST", Path: "/v1/review", OpID: "resolveReviewItems",
			Summary: "Confirm or correct labels of one recipe's step graph, and optionally accept it as verified",
			Body:    jsonOf[recipes.ResolveRequest](), Resp: jsonOf[recipes.ResolveResult](),
			Errors: []int{404, 409, 412}, Write: true, Limit: jsonLimit, Handle: a.resolveReviewItems},
	}
}

func (a *api) upgradeRecipe(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	var in upgradeBody
	if err := decodeJSON(r, &in); err != nil {
		return err
	}
	res, err := a.svc().UpgradeRecipe(r.Context(), c, recipes.UpgradeRequest{ID: r.PathValue("id"), Mode: in.Mode, Force: in.Force})
	if err != nil {
		return err
	}
	writeVersioned(w, http.StatusOK, res.Graph.Graph.Version, res)
	return nil
}

func (a *api) queueUpgrades(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	var in recipes.UpgradeJobRequest
	if err := decodeJSON(r, &in); err != nil {
		return err
	}
	j, err := a.svc().QueueUpgrades(r.Context(), c, in)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, j)
	return nil
}

func (a *api) getUpgradeJob(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	j, err := a.svc().UpgradeJob(r.Context(), c, r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, j)
	return nil
}

func (a *api) cancelUpgradeJob(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	j, err := a.svc().CancelUpgradeJob(r.Context(), c, r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, j)
	return nil
}

func (a *api) listReviewItems(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	var q recipes.ReviewQuery
	if err := decodeQuery(r.URL.Query(), &q); err != nil {
		return err
	}
	p, err := a.svc().ReviewItems(r.Context(), c, q)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, p)
	return nil
}

func (a *api) resolveReviewItems(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	var in recipes.ResolveRequest
	if err := decodeJSON(r, &in); err != nil {
		return err
	}
	res, err := a.svc().ResolveReview(r.Context(), c, in)
	if err != nil {
		return err
	}
	writeVersioned(w, http.StatusOK, res.Graph.Graph.Version, res)
	return nil
}

func (a *api) draftRecipeGraph(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	var in draftBody
	if err := decodeJSON(r, &in); err != nil {
		return err
	}
	res, err := a.svc().DraftRecipeGraph(r.Context(), c, recipes.DraftRequest{ID: r.PathValue("id"), RecipeVersion: in.RecipeVersion,
		Mode: in.Mode, Force: in.Force, StructureDraft: in.StructureDraft})
	if err != nil {
		return err
	}
	writeVersioned(w, http.StatusOK, res.Graph.Graph.Version, res)
	return nil
}

func (a *api) listRecipesWithoutGraph(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	var q graphTodoQuery
	if err := decodeQuery(r.URL.Query(), &q); err != nil {
		return err
	}
	out, err := a.svc().RecipesWithoutGraph(r.Context(), c, recipes.GraphTodoQuery{MealID: q.MealID, Limit: q.Limit})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}
