package api

import (
	"net/http"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/recipes"
)

// staleGraphBody is the 412 for a step graph: the error plus the graph as
// it now stands.
type staleGraphBody struct {
	Error   errorDetail       `json:"error"`
	Current recipes.GraphView `json:"current" jsonschema:"the step graph as it now stands; its version is the one to send next"`
}

// graphPut is the body of PUT /v1/recipes/{id}/graph.
type graphPut struct {
	RecipeVersion int `json:"recipe_version" jsonschema:"the recipe's current version: the graph's spans are read against its words"`
	Version       int `json:"version" jsonschema:"the graph's version being replaced, from GET; 0 to make the recipe's first"`
	recipes.GraphInput
}

func (a *api) graphEndpoints() []endpoint {
	stale := jsonOf[staleGraphBody]()
	return []endpoint{
		{Method: "GET", Path: "/v1/recipes/{id}/graph", OpID: "getRecipeGraph",
			Summary: "A recipe's step graph (atomic actions joined by what they make and use, each field labelled with its source " +
				"and confidence), what the checks find in it now, and the step timing it comes to",
			Resp: jsonOf[recipes.GraphView](), Errors: []int{404}, Handle: a.getRecipeGraph},
		{Method: "PUT", Path: "/v1/recipes/{id}/graph", OpID: "setRecipeGraph",
			Summary: "Save a recipe's step graph, replacing the one at version (0 for its first). A verified graph must pass the checks",
			Body:    jsonOf[graphPut](), Resp: jsonOf[recipes.GraphView](), Stale: stale,
			Errors: []int{404, 409, 412}, Write: true, Limit: jsonLimit, Handle: a.setRecipeGraph},
		{Method: "DELETE", Path: "/v1/recipes/{id}/graph", OpID: "deleteRecipeGraph",
			Summary: "Take a recipe's step graph away, so it plans by step timing alone. Needs If-Match with the graph's version",
			Status:  http.StatusNoContent, Stale: stale,
			Errors: []int{404, 412, 428}, Write: true, Handle: a.deleteRecipeGraph},
		{Method: "POST", Path: "/v1/recipes/{id}/graph/check", OpID: "checkRecipeGraph",
			Summary: "Run the step graph checks on a proposed graph for a recipe without saving it",
			Body:    jsonOf[recipes.GraphInput](), Resp: jsonOf[recipes.GraphCheck](),
			Errors: []int{404}, Limit: jsonLimit, Handle: a.checkRecipeGraph},
	}
}

func (a *api) getRecipeGraph(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	v, err := a.svc().RecipeGraph(r.Context(), c, r.PathValue("id"))
	if err != nil {
		return err
	}
	writeVersioned(w, http.StatusOK, v.Graph.Version, v)
	return nil
}

func (a *api) setRecipeGraph(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	var in graphPut
	if err := decodeJSON(r, &in); err != nil {
		return err
	}
	v, err := a.svc().SetRecipeGraph(r.Context(), c, r.PathValue("id"), in.RecipeVersion, in.Version, in.GraphInput)
	if err != nil {
		return err
	}
	writeVersioned(w, http.StatusOK, v.Graph.Version, v)
	return nil
}

func (a *api) deleteRecipeGraph(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	version, err := ifMatch(r)
	if err != nil {
		return err
	}
	if err := a.svc().DeleteRecipeGraph(r.Context(), c, r.PathValue("id"), version); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (a *api) checkRecipeGraph(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	var in recipes.GraphInput
	if err := decodeJSON(r, &in); err != nil {
		return err
	}
	chk, err := a.svc().CheckRecipeGraph(r.Context(), c, r.PathValue("id"), &in)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, chk)
	return nil
}
