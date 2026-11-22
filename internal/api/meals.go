package api

import (
	"net/http"
	"reflect"
	"time"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/recipes"
)

// staleMealBody and staleKitchenBody are the 412s for a meal and the
// kitchen: the error plus what now stands.
type staleMealBody struct {
	Error   errorDetail  `json:"error"`
	Current recipes.Meal `json:"current" jsonschema:"the meal as it now stands; its version is the one to send next"`
}

type staleKitchenBody struct {
	Error   errorDetail     `json:"error"`
	Current recipes.Kitchen `json:"current" jsonschema:"the kitchen as it now stands; its version is the one to send next"`
}

// SuggestRequest is the body of POST /v1/steps/suggest.
type suggestRequest struct {
	Steps []string `json:"steps" jsonschema:"step texts, in order: an oven temperature carries forward from one to the next"`
}

func (a *api) mealEndpoints() []endpoint {
	staleMeal := jsonOf[staleMealBody]()
	staleKitchen := jsonOf[staleKitchenBody]()
	return []endpoint{
		{Method: "GET", Path: "/v1/kitchen", OpID: "getKitchen",
			Summary: "The household's kitchen for meal planning: ovens and racks, burners, appliances, serving dishes and the hours of its day",
			Resp:    jsonOf[recipes.Kitchen](), Handle: a.getKitchen},
		{Method: "PUT", Path: "/v1/kitchen", OpID: "updateKitchen",
			Summary: "Replace the kitchen. Needs If-Match with the version being edited",
			Body:    jsonOf[recipes.KitchenProfile](), Resp: jsonOf[recipes.Kitchen](), IfMatch: true, Stale: staleKitchen,
			Errors: []int{412, 428}, Write: true, Limit: jsonLimit, Handle: a.updateKitchen},
		{Method: "POST", Path: "/v1/steps/suggest", OpID: "suggestTiming",
			Summary: "Guess each step's timing from its words (durations, oven temperature, equipment, make-ahead). Saves nothing and needs no person",
			Body:    jsonOf[suggestRequest](), Resp: jsonOf[recipes.Items[recipes.StepTiming]](),
			Limit: jsonLimit, Handle: a.suggestTiming},
		{Method: "GET", Path: "/v1/meals", OpID: "listMeals",
			Summary: "Planned meals that aren't deleted, soonest first",
			Resp:    jsonOf[recipes.Items[recipes.MealSummary]](), Handle: a.listMeals},
		{Method: "POST", Path: "/v1/meals", OpID: "createMeal",
			Summary: "Plan a meal: several recipes served together at one time",
			Body:    jsonOf[recipes.MealInput](), Resp: jsonOf[recipes.Meal](), Status: http.StatusCreated,
			Write: true, Limit: jsonLimit, Handle: a.createMeal},
		{Method: "GET", Path: "/v1/meals/{id}", OpID: "getMeal",
			Summary: "A meal and its menu. A deleted meal has deleted_at set",
			Resp:    jsonOf[recipes.Meal](), Errors: []int{404}, Handle: a.getMeal},
		{Method: "PATCH", Path: "/v1/meals/{id}", OpID: "updateMeal",
			Summary: "Change some fields of a meal; items replace the whole menu. Needs If-Match with the version being edited",
			Body:    jsonOf[recipes.MealPatch](), Resp: jsonOf[recipes.Meal](), Stale: staleMeal,
			Errors: []int{404, 409, 412, 428}, Write: true, Limit: jsonLimit, Handle: a.updateMeal},
		{Method: "DELETE", Path: "/v1/meals/{id}", OpID: "deleteMeal",
			Summary: "Move a meal to the trash. Needs If-Match with the version being deleted",
			Status:  http.StatusNoContent, Stale: staleMeal,
			Errors: []int{404, 409, 412, 428}, Write: true, Handle: a.deleteMeal},
		{Method: "POST", Path: "/v1/meals/{id}/restore", OpID: "restoreMeal",
			Summary: "Take a meal out of the trash. Restoring one that isn't deleted changes nothing",
			Resp:    jsonOf[recipes.Meal](), Errors: []int{404}, Write: true, Handle: a.restoreMeal},
		{Method: "GET", Path: "/v1/meals/{id}/plan", OpID: "getMealPlan",
			Summary: "The meal laid out in time: every step of every dish, fitted around the cooks, ovens, burners and appliances, " +
				"with make-ahead work moved earlier and anything already late moved to now",
			Query: reflect.TypeFor[recipes.PlanQuery](), Resp: jsonOf[recipes.Plan](), Errors: []int{404}, Handle: a.getMealPlan},
		{Method: "PUT", Path: "/v1/meals/{id}/tasks/{task}", OpID: "updateMealTask",
			Summary: "Start, tick off or pin one task of a meal; returns the plan as it now stands",
			Body:    jsonOf[recipes.TaskPatch](), Resp: jsonOf[recipes.Plan](),
			Errors: []int{404, 409}, Write: true, Limit: jsonLimit, Handle: a.updateMealTask},
		{Method: "POST", Path: "/v1/meals/{id}/reset", OpID: "resetMeal",
			Summary: "Forget what has been started and ticked off on a meal, keeping pins, to cook it again; returns the plan",
			Resp:    jsonOf[recipes.Plan](), Errors: []int{404}, Write: true, Handle: a.resetMeal},
	}
}

func (a *api) getKitchen(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	k, err := a.svc().GetKitchen(r.Context(), c)
	if err != nil {
		return err
	}
	writeVersioned(w, http.StatusOK, k.Version, k)
	return nil
}

func (a *api) updateKitchen(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	version, err := ifMatch(r)
	if err != nil {
		return err
	}
	var in recipes.KitchenProfile
	if err := decodeJSON(r, &in); err != nil {
		return err
	}
	k, err := a.svc().UpdateKitchen(r.Context(), c, version, in)
	if err != nil {
		return err
	}
	writeVersioned(w, http.StatusOK, k.Version, k)
	return nil
}

func (a *api) suggestTiming(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	var in suggestRequest
	if err := decodeJSON(r, &in); err != nil {
		return err
	}
	if len(in.Steps) > 500 {
		return recipes.Errorf(recipes.CodeInvalid, "steps: at most 500")
	}
	writeJSON(w, http.StatusOK, recipes.Items[recipes.StepTiming]{Items: recipes.SuggestTimings(in.Steps)})
	return nil
}

func (a *api) listMeals(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	ms, err := a.svc().ListMeals(r.Context(), c)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, recipes.Items[recipes.MealSummary]{Items: ms})
	return nil
}

func (a *api) createMeal(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	var in recipes.MealInput
	if err := decodeJSON(r, &in); err != nil {
		return err
	}
	m, err := a.svc().CreateMeal(r.Context(), c, in)
	if err != nil {
		return err
	}
	writeVersioned(w, http.StatusCreated, m.Version, m)
	return nil
}

func (a *api) getMeal(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	m, err := a.svc().GetMeal(r.Context(), c, r.PathValue("id"))
	if err != nil {
		return err
	}
	writeVersioned(w, http.StatusOK, m.Version, m)
	return nil
}

func (a *api) updateMeal(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	version, err := ifMatch(r)
	if err != nil {
		return err
	}
	var p recipes.MealPatch
	if err := decodeJSON(r, &p); err != nil {
		return err
	}
	m, err := a.svc().UpdateMeal(r.Context(), c, r.PathValue("id"), version, p)
	if err != nil {
		return err
	}
	writeVersioned(w, http.StatusOK, m.Version, m)
	return nil
}

func (a *api) deleteMeal(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	version, err := ifMatch(r)
	if err != nil {
		return err
	}
	if err := a.svc().DeleteMeal(r.Context(), c, r.PathValue("id"), version); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (a *api) restoreMeal(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	m, err := a.svc().RestoreMeal(r.Context(), c, r.PathValue("id"))
	if err != nil {
		return err
	}
	writeVersioned(w, http.StatusOK, m.Version, m)
	return nil
}

func (a *api) getMealPlan(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	var q recipes.PlanQuery
	if err := decodeQuery(r.URL.Query(), &q); err != nil {
		return err
	}
	now := time.Now()
	if q.Now != "" {
		t, err := time.Parse(time.RFC3339, q.Now)
		if err != nil {
			return recipes.Errorf(recipes.CodeInvalid, "now must be an RFC 3339 time")
		}
		now = t
	}
	p, err := a.svc().MealPlan(r.Context(), c, r.PathValue("id"), now)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, p)
	return nil
}

func (a *api) updateMealTask(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	var p recipes.TaskPatch
	if err := decodeJSON(r, &p); err != nil {
		return err
	}
	plan, err := a.svc().UpdateTask(r.Context(), c, r.PathValue("id"), r.PathValue("task"), p)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, plan)
	return nil
}

func (a *api) resetMeal(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	plan, err := a.svc().ResetProgress(r.Context(), c, r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, plan)
	return nil
}

// writeVersioned sends a versioned record with its version as the ETag.
func writeVersioned(w http.ResponseWriter, status, version int, v any) {
	w.Header().Set("ETag", etag(version))
	writeJSON(w, status, v)
}
