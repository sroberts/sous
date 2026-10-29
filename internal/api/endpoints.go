// Package api is the REST surface under /v1 (spec.md §8) and its OpenAPI
// document, both generated from one route table.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"reflect"
	"slices"
	"time"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/importer"
	"github.com/sroberts/sous/internal/recipes"
)

// Deps is what the REST handlers need.
type Deps struct {
	Service  *recipes.Service
	Resolver *identity.Resolver
	// JWKS is nil unless OUTPOST_AUTH=gateway; it fills Status.JWKS.
	JWKS    *identity.JWKS
	Log     *slog.Logger
	Version string // the binary's version, for /v1/status
	Auth    string // "none" or "gateway", for /v1/status
	// Importer reads recipes from links; nil answers import with 503.
	Importer *importer.Importer
}

const (
	jsonLimit  = 1 << 20
	imageLimit = 25 << 20
)

// endpoint is one /v1 route. New serves it and the OpenAPI document
// describes it, so the two cannot disagree.
type endpoint struct {
	Method  string
	Path    string
	OpID    string
	Summary string
	Query   reflect.Type // a struct with query tags, or nil
	Body    *payload     // the request body, or nil
	Resp    *payload     // the success body, or nil for none
	Status  int          // the success status; 0 means 200
	Errors  []int        // route-specific error statuses, for the document
	Write   bool         // nobody gets 401 who_are_you
	Limit   int64        // the request body limit; 0 when no body is read
	Handle  func(w http.ResponseWriter, r *http.Request, c identity.Caller) error
}

type api struct {
	d   Deps
	doc []byte
}

func (a *api) svc() *recipes.Service { return a.d.Service }

func (a *api) endpoints() []endpoint {
	return []endpoint{
		{Method: "GET", Path: "/v1/status", OpID: "getStatus",
			Summary: "System status: version, schema version, auth mode, recipe count, storage size and gateway key health",
			Resp:    jsonOf[recipes.Status](), Handle: a.status},
		{Method: "GET", Path: "/v1/me", OpID: "getMe",
			Summary: "The person making the request, and whether Outpost signed them in; 401 who_are_you when nobody is",
			Resp:    jsonOf[recipes.Me](), Handle: a.me},
		{Method: "GET", Path: "/v1/people", OpID: "listPeople",
			Summary: "Everyone in the household",
			Resp:    jsonOf[recipes.Items[recipes.Person]](), Handle: a.listPeople},
		{Method: "POST", Path: "/v1/people", OpID: "createPerson",
			Summary: "Add a person; the username is derived from the name. Needs no person",
			Body:    jsonOf[recipes.NewPerson](), Resp: jsonOf[recipes.Person](), Status: http.StatusCreated,
			Errors: []int{409}, Limit: jsonLimit, Handle: a.createPerson},
		{Method: "GET", Path: "/v1/recipes", OpID: "listRecipes",
			Summary: "List and search recipes as summaries, a page at a time",
			Query:   reflect.TypeFor[recipes.ListQuery](), Resp: jsonOf[recipes.Page[recipes.Summary]](), Handle: a.listRecipes},
		{Method: "POST", Path: "/v1/recipes", OpID: "createRecipe",
			Summary: "Create a recipe. Ingredients and steps are raw lines; '# Heading' starts a section",
			Body:    jsonOf[recipes.RecipeInput](), Resp: jsonOf[recipes.Recipe](), Status: http.StatusCreated,
			Write: true, Limit: jsonLimit, Handle: a.createRecipe},
		{Method: "POST", Path: "/v1/recipes/import/preview", OpID: "previewImport",
			Summary: "Read a recipe from a link (or pasted text) into a draft to review; nothing is saved but its photo. " +
				"POST the draft to /v1/recipes to save it",
			Body: jsonOf[recipes.ImportRequest](), Resp: jsonOf[recipes.ImportPreview](),
			Errors: []int{422}, Write: true, Limit: jsonLimit, Handle: a.previewImport},
		{Method: "GET", Path: "/v1/recipes/{id}", OpID: "getRecipe",
			Summary: "The full recipe, with the caller's flags. A deleted recipe has deleted_at set",
			Resp:    jsonOf[recipes.Recipe](), Errors: []int{404}, Handle: a.getRecipe},
		{Method: "PATCH", Path: "/v1/recipes/{id}", OpID: "updateRecipe",
			Summary: "Change some fields of a recipe. Needs If-Match with the version being edited",
			Body:    jsonOf[recipes.RecipePatch](), Resp: jsonOf[recipes.Recipe](),
			Errors: []int{404, 409, 412, 428}, Write: true, Limit: jsonLimit, Handle: a.updateRecipe},
		{Method: "DELETE", Path: "/v1/recipes/{id}", OpID: "deleteRecipe",
			Summary: "Move a recipe to the trash. Needs If-Match with the version being deleted",
			Status:  http.StatusNoContent, Errors: []int{404, 409, 412, 428}, Write: true, Handle: a.deleteRecipe},
		{Method: "POST", Path: "/v1/recipes/{id}/restore", OpID: "restoreRecipe",
			Summary: "Take a recipe out of the trash. Restoring one that isn't deleted changes nothing",
			Resp:    jsonOf[recipes.Recipe](), Errors: []int{404}, Write: true, Handle: a.restoreRecipe},
		{Method: "PUT", Path: "/v1/recipes/{id}/mine", OpID: "setMine",
			Summary: "Set the caller's favorite and want_to_cook flags; an absent flag keeps its value",
			Body:    jsonOf[recipes.MinePatch](), Resp: jsonOf[recipes.Mine](),
			Errors: []int{404}, Write: true, Limit: jsonLimit, Handle: a.setMine},
		{Method: "GET", Path: "/v1/trash", OpID: "listTrash",
			Summary: "Deleted recipes, most recently deleted first",
			Query:   reflect.TypeFor[recipes.TrashQuery](), Resp: jsonOf[recipes.Page[recipes.Summary]](), Handle: a.listTrash},
		{Method: "GET", Path: "/v1/tags", OpID: "listTags",
			Summary: "Tags on live recipes with their counts, most used first",
			Resp:    jsonOf[recipes.Items[recipes.TagCount]](), Handle: a.listTags},
		{Method: "POST", Path: "/v1/ingredients/parse", OpID: "parseIngredients",
			Summary: "Parse ingredient lines without saving, one item per line. Needs no person",
			Body:    jsonOf[recipes.ParseRequest](), Resp: jsonOf[recipes.Items[recipes.ParsedLine]](),
			Limit: jsonLimit, Handle: a.parseIngredients},
		{Method: "POST", Path: "/v1/images", OpID: "uploadImage",
			Summary: "Upload a JPEG, PNG, WebP or HEIC image as the raw body; returns its hash",
			Body:    imagePayload, Resp: jsonOf[recipes.ImageRef](), Status: http.StatusCreated,
			Errors: []int{415}, Write: true, Limit: imageLimit, Handle: a.uploadImage},
		{Method: "GET", Path: "/v1/images/{hash}", OpID: "getImage",
			Summary: "An uploaded image. Its content never changes under a hash",
			Resp:    imagePayload, Errors: []int{404}, Handle: a.getImage},
		{Method: "GET", Path: "/v1/changes", OpID: "listChanges",
			Summary: "The change log, newest first",
			Query:   reflect.TypeFor[recipes.ChangeQuery](), Resp: jsonOf[recipes.ChangePage](), Handle: a.listChanges},
		{Method: "GET", Path: "/v1/openapi.json", OpID: "getOpenAPI",
			Summary: "This document",
			Resp:    openapiPayload, Handle: a.openapi},
	}
}

// New returns the handler for every /v1 route, registered from the route
// table. It resolves identity (via from the x-sous-via header), enforces
// who_are_you on writes, body limits, cross-origin protection on unsafe
// methods, and answers unknown /v1 paths and methods with JSON 404 and 405.
func New(d Deps) http.Handler {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.Auth == "" {
		d.Auth = "none"
	}
	a := &api{d: d}
	eps := a.endpoints()
	doc, err := json.Marshal(buildDocument(eps, d.Version))
	if err != nil {
		panic("api: openapi document: " + err.Error())
	}
	a.doc = doc

	mux := http.NewServeMux()
	allow := map[string][]string{}
	for _, ep := range eps {
		mux.Handle(ep.Method+" "+ep.Path, a.serve(ep))
		allow[ep.Path] = append(allow[ep.Path], ep.Method)
	}
	for path, methods := range allow {
		if slices.Contains(methods, "GET") {
			methods = append(methods, "HEAD")
		}
		slices.Sort(methods)
		mux.Handle(path, methodNotAllowed(methods))
	}
	mux.HandleFunc("/v1/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, recipes.CodeNotFound, "no route for "+r.Method+" "+r.URL.Path)
	})
	return crossOrigin(mux)
}

func (a *api) status(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	st, err := a.svc().Status(r.Context(), c)
	if err != nil {
		return err
	}
	st.Version, st.Auth = a.d.Version, a.d.Auth
	if a.d.JWKS != nil {
		js := a.d.JWKS.Status()
		st.JWKS = &js
	}
	writeJSON(w, http.StatusOK, st)
	return nil
}

func (a *api) me(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	if c.Person == nil {
		return recipes.Errorf(recipes.CodeWhoAreYou, "no one is selected on this device")
	}
	src := recipes.SourceDevice
	if c.Route == identity.RouteGateway {
		src = recipes.SourceOutpost
	}
	writeJSON(w, http.StatusOK, recipes.Me{Person: recipes.Person{Username: c.Person.Username, Name: c.Person.Name}, Source: src})
	return nil
}

func (a *api) listPeople(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	people, err := a.svc().ListPeople(r.Context(), c)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, recipes.Items[recipes.Person]{Items: people})
	return nil
}

func (a *api) createPerson(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	var in recipes.NewPerson
	if err := decodeJSON(r, &in); err != nil {
		return err
	}
	p, err := a.svc().CreatePerson(r.Context(), c, in.Name)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, p)
	return nil
}

func (a *api) listRecipes(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	var q recipes.ListQuery
	if err := decodeQuery(r.URL.Query(), &q); err != nil {
		return err
	}
	page, err := a.svc().List(r.Context(), c, q)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, page)
	return nil
}

func (a *api) createRecipe(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	var in recipes.RecipeInput
	if err := decodeJSON(r, &in); err != nil {
		return err
	}
	rec, err := a.svc().Create(r.Context(), c, in)
	if err != nil {
		return err
	}
	writeRecipe(w, http.StatusCreated, rec)
	return nil
}

func (a *api) previewImport(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	if a.d.Importer == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "importing from links is not available")
		return nil
	}
	var in recipes.ImportRequest
	if err := decodeJSON(r, &in); err != nil {
		return err
	}
	p, err := a.d.Importer.Preview(r.Context(), c, in)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, p)
	return nil
}

func (a *api) getRecipe(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	rec, err := a.svc().Get(r.Context(), c, r.PathValue("id"))
	if err != nil {
		return err
	}
	writeRecipe(w, http.StatusOK, rec)
	return nil
}

func (a *api) updateRecipe(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	version, err := ifMatch(r)
	if err != nil {
		return err
	}
	var p recipes.RecipePatch
	if err := decodeJSON(r, &p); err != nil {
		return err
	}
	rec, err := a.svc().Update(r.Context(), c, r.PathValue("id"), version, p)
	if err != nil {
		return err
	}
	writeRecipe(w, http.StatusOK, rec)
	return nil
}

func (a *api) deleteRecipe(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	version, err := ifMatch(r)
	if err != nil {
		return err
	}
	if err := a.svc().Delete(r.Context(), c, r.PathValue("id"), version); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (a *api) restoreRecipe(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	rec, err := a.svc().Restore(r.Context(), c, r.PathValue("id"))
	if err != nil {
		return err
	}
	writeRecipe(w, http.StatusOK, rec)
	return nil
}

func (a *api) setMine(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	var p recipes.MinePatch
	if err := decodeJSON(r, &p); err != nil {
		return err
	}
	m, err := a.svc().SetMine(r.Context(), c, r.PathValue("id"), p)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, m)
	return nil
}

func (a *api) listTrash(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	var q recipes.TrashQuery
	if err := decodeQuery(r.URL.Query(), &q); err != nil {
		return err
	}
	page, err := a.svc().Trash(r.Context(), c, q)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, page)
	return nil
}

func (a *api) listTags(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	tags, err := a.svc().Tags(r.Context(), c)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, recipes.Items[recipes.TagCount]{Items: tags})
	return nil
}

func (a *api) parseIngredients(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	var in recipes.ParseRequest
	if err := decodeJSON(r, &in); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, recipes.Items[recipes.ParsedLine]{Items: recipes.PreviewLines(in.Lines)})
	return nil
}

func (a *api) uploadImage(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	ref, err := a.svc().UploadImage(r.Context(), c, r.Body)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, ref)
	return nil
}

func (a *api) getImage(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	f, meta, err := a.svc().Image(r.Context(), c, r.PathValue("hash"))
	if err != nil {
		return err
	}
	defer f.Close()
	h := w.Header()
	h.Set("Content-Type", meta.ContentType)
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
	h.Set("ETag", `"`+meta.Hash+`"`)
	h.Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "", time.Time{}, f)
	return nil
}

func (a *api) listChanges(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	var q recipes.ChangeQuery
	if err := decodeQuery(r.URL.Query(), &q); err != nil {
		return err
	}
	page, err := a.svc().Changes(r.Context(), c, q)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, page)
	return nil
}

func (a *api) openapi(w http.ResponseWriter, r *http.Request, c identity.Caller) error {
	w.Header().Set("Content-Type", "application/json")
	w.Write(a.doc)
	return nil
}
