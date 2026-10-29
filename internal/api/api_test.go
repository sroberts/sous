package api

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/recipes"
	"github.com/sroberts/sous/internal/storage"
)

// harness serves the API over a real store in a temp dir. Every JSON
// response it sees is validated against the OpenAPI document.
type harness struct {
	t      *testing.T
	h      http.Handler
	st     *storage.Store
	res    *identity.Resolver
	logs   *bytes.Buffer
	schema *schemaChecker
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	logs := &bytes.Buffer{}
	log := slog.New(slog.NewTextHandler(logs, nil))
	st, err := storage.Open(context.Background(), t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	svc := recipes.New(st, log)
	res := identity.NewResolver(identity.ResolverConfig{Directory: svc})
	h := New(Deps{Service: svc, Resolver: res, Log: log, Version: "test", Auth: "none"})
	return &harness{t: t, h: h, st: st, res: res, logs: logs, schema: newSchemaChecker(t)}
}

type result struct {
	status int
	header http.Header
	body   []byte
}

// json decodes the body into a generic value, as a JS client would see it.
func (r *result) json(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(r.body, &m); err != nil {
		t.Fatalf("body is not a JSON object: %v: %s", err, r.body)
	}
	return m
}

func (r *result) code(t *testing.T) string {
	t.Helper()
	e, _ := r.json(t)["error"].(map[string]any)
	code, _ := e["code"].(string)
	return code
}

// do sends a request. body is nil, []byte (sent raw) or a value sent as
// JSON. hdr is name, value pairs.
func (h *harness) do(method, path string, body any, hdr ...string) *result {
	h.t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case []byte:
		rd = bytes.NewReader(b)
	case string:
		rd = strings.NewReader(b)
	default:
		j, err := json.Marshal(b)
		if err != nil {
			h.t.Fatal(err)
		}
		rd = bytes.NewReader(j)
	}
	req := httptest.NewRequest(method, path, rd)
	if body != nil {
		if _, raw := body.([]byte); !raw {
			req.Header.Set("Content-Type", "application/json")
		}
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Add(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	h.h.ServeHTTP(rec, req)
	r := &result{status: rec.Code, header: rec.Header(), body: rec.Body.Bytes()}
	h.schema.check(h.t, req, r)
	return r
}

func (h *harness) want(r *result, status int, code string) {
	h.t.Helper()
	if r.status != status {
		h.t.Fatalf("status %d, want %d: %s", r.status, status, r.body)
	}
	if code != "" {
		if got := r.code(h.t); got != code {
			h.t.Fatalf("code %q, want %q: %s", got, code, r.body)
		}
	}
}

// person creates someone and returns the header pair that names them.
func (h *harness) person(name string) []string {
	h.t.Helper()
	r := h.do("POST", "/v1/people", map[string]any{"name": name})
	h.want(r, 201, "")
	return []string{"X-Sous-Person", r.json(h.t)["username"].(string)}
}

func (h *harness) create(as []string, in map[string]any) map[string]any {
	h.t.Helper()
	r := h.do("POST", "/v1/recipes", in, as...)
	h.want(r, 201, "")
	return r.json(h.t)
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestStatus(t *testing.T) {
	h := newHarness(t)
	r := h.do("GET", "/v1/status", nil)
	h.want(r, 200, "")
	m := r.json(t)
	if m["version"] != "test" || m["auth"] != "none" || m["recipes"] != 0.0 {
		t.Errorf("status = %v", m)
	}
	if v, ok := m["jwks"]; !ok || v != nil {
		t.Errorf("jwks = %v, %v; want present and null", v, ok)
	}
	if m["schema_version"].(float64) < 1 {
		t.Errorf("schema_version = %v", m["schema_version"])
	}
}

func TestMe(t *testing.T) {
	h := newHarness(t)
	h.want(h.do("GET", "/v1/me", nil), 401, "who_are_you")
	scott := h.person("Scott")
	r := h.do("GET", "/v1/me", nil, scott...)
	h.want(r, 200, "")
	if m := r.json(t); m["username"] != "scott" || m["name"] != "Scott" {
		t.Errorf("me = %v", m)
	}
	h.want(h.do("GET", "/v1/me", nil, "X-Sous-Person", "nobody"), 401, "unknown_person")
	// A stale cookie is nobody, not an error.
	h.want(h.do("GET", "/v1/me", nil, "Cookie", "sous_person=gone"), 401, "who_are_you")
	h.want(h.do("GET", "/v1/me", nil, "Cookie", "sous_person=scott"), 200, "")
}

func TestPeople(t *testing.T) {
	h := newHarness(t)
	r := h.do("GET", "/v1/people", nil)
	h.want(r, 200, "")
	if string(r.body) != `{"items":[]}`+"\n" {
		t.Errorf("empty people = %s", r.body)
	}
	r = h.do("POST", "/v1/people", map[string]any{"name": "Mary Ann"})
	h.want(r, 201, "")
	if m := r.json(t); m["username"] != "mary-ann" || m["name"] != "Mary Ann" {
		t.Errorf("created = %v", m)
	}
	h.want(h.do("POST", "/v1/people", map[string]any{"name": "mary ann"}), 409, "exists")
	h.want(h.do("POST", "/v1/people", map[string]any{"name": "  "}), 400, "invalid")
	h.want(h.do("POST", "/v1/people", "{"), 400, "invalid")
	h.want(h.do("POST", "/v1/people", ""), 400, "invalid")
	h.want(h.do("POST", "/v1/people", `{"name":"A"} {"name":"B"}`), 400, "invalid")
	h.want(h.do("POST", "/v1/people", `{"name":3}`), 400, "invalid")

	r = h.do("POST", "/v1/people", map[string]any{"name": "Al", "admin": true})
	h.want(r, 400, "invalid")
	if msg := r.json(t)["error"].(map[string]any)["message"].(string); !strings.Contains(msg, `"admin"`) {
		t.Errorf("unknown field message %q doesn't name the field", msg)
	}

	big := `{"name":"` + strings.Repeat("x", jsonLimit) + `"}`
	h.want(h.do("POST", "/v1/people", big), 413, "too_large")

	r = h.do("GET", "/v1/people", nil)
	if items := r.json(t)["items"].([]any); len(items) != 1 {
		t.Errorf("people = %v", items)
	}
}

func TestRecipeLifecycle(t *testing.T) {
	h := newHarness(t)
	h.want(h.do("POST", "/v1/recipes", map[string]any{"title": "Soup"}), 401, "who_are_you")
	scott := h.person("Scott")
	jo := h.person("Jo")
	h.want(h.do("POST", "/v1/recipes", map[string]any{"title": " "}, scott...), 400, "invalid")
	h.want(h.do("POST", "/v1/recipes", map[string]any{"title": "Soup", "images": []string{strings.Repeat("a", 64)}}, scott...), 400, "unknown_image")

	r := h.do("POST", "/v1/recipes", map[string]any{
		"title":       "Pad Thai",
		"source_url":  "https://www.example.com/pad-thai",
		"yield_text":  "Serves 4",
		"tags":        []string{"Dinner", "Thai"},
		"ingredients": []string{"# Sauce", "2 tbsp fish sauce", "200 g rice noodles"},
		"steps":       []string{"Soak the noodles.", "Fry."},
	}, scott...)
	h.want(r, 201, "")
	if r.header.Get("ETag") != `"1"` {
		t.Errorf("create ETag = %q", r.header.Get("ETag"))
	}
	rec := r.json(t)
	id := rec["id"].(string)
	if rec["servings"] != 4.0 || rec["source_host"] != "example.com" || rec["created_by"] != "scott" {
		t.Errorf("created = %v", rec)
	}

	r = h.do("GET", "/v1/recipes/"+id, nil)
	h.want(r, 200, "")
	if r.header.Get("ETag") != `"1"` {
		t.Errorf("get ETag = %q", r.header.Get("ETag"))
	}
	h.want(h.do("GET", "/v1/recipes/nope", nil), 404, "not_found")

	path := "/v1/recipes/" + id
	h.want(h.do("PATCH", path, map[string]any{"title": "x"}, scott...), 428, "version_required")
	h.want(h.do("PATCH", path, map[string]any{"title": "x"}, append(scott, "If-Match", "*")...), 400, "invalid")
	h.want(h.do("PATCH", path, map[string]any{"title": "x"}), 401, "who_are_you")
	h.want(h.do("PATCH", path, map[string]any{"colour": "red"}, append(scott, "If-Match", `"1"`)...), 400, "invalid")
	h.want(h.do("PATCH", "/v1/recipes/nope", map[string]any{"title": "x"}, append(scott, "If-Match", `"1"`)...), 404, "not_found")

	r = h.do("PATCH", path, map[string]any{"description": "Quick.", "source_url": nil}, append(scott, "If-Match", `W/"1"`)...)
	h.want(r, 200, "")
	if r.header.Get("ETag") != `"2"` {
		t.Errorf("patch ETag = %q", r.header.Get("ETag"))
	}
	if m := r.json(t); m["description"] != "Quick." || m["source_url"] != nil || m["version"] != 2.0 {
		t.Errorf("patched = %v", m)
	}

	r = h.do("PATCH", path, map[string]any{"title": "Mine"}, append(jo, "If-Match", `"1"`)...)
	h.want(r, 412, "stale")
	if cur := r.json(t)["current"].(map[string]any); cur["version"] != 2.0 || cur["updated_by"] != "scott" {
		t.Errorf("current = %v", cur)
	}
	h.want(h.do("DELETE", path, nil, append(jo, "If-Match", `"1"`)...), 412, "stale")
	h.want(h.do("DELETE", path, nil, jo...), 428, "version_required")

	r = h.do("DELETE", path, nil, append(jo, "If-Match", `"2"`)...)
	h.want(r, 204, "")
	if len(r.body) != 0 {
		t.Errorf("DELETE body = %q", r.body)
	}
	h.want(h.do("PATCH", path, map[string]any{"title": "x"}, append(jo, "If-Match", `"3"`)...), 409, "deleted")
	h.want(h.do("DELETE", path, nil, append(jo, "If-Match", `"3"`)...), 409, "deleted")

	r = h.do("GET", path, nil)
	h.want(r, 200, "")
	if r.json(t)["deleted_at"] == nil {
		t.Error("deleted recipe has no deleted_at")
	}
	if items := h.do("GET", "/v1/recipes", nil).json(t)["items"].([]any); len(items) != 0 {
		t.Errorf("list after delete = %v", items)
	}
	r = h.do("GET", "/v1/trash", nil)
	h.want(r, 200, "")
	if items := r.json(t)["items"].([]any); len(items) != 1 || items[0].(map[string]any)["deleted_at"] == nil {
		t.Errorf("trash = %v", items)
	}

	h.want(h.do("POST", path+"/restore", nil), 401, "who_are_you")
	h.want(h.do("POST", "/v1/recipes/nope/restore", nil, jo...), 404, "not_found")
	r = h.do("POST", path+"/restore", nil, jo...)
	h.want(r, 200, "")
	if m := r.json(t); m["deleted_at"] != nil || m["version"] != 4.0 {
		t.Errorf("restored = %v", m)
	}
}

func TestListRecipes(t *testing.T) {
	h := newHarness(t)
	scott := h.person("Scott")
	for _, in := range []map[string]any{
		{"title": "Apple pie", "tags": []string{"Dessert"}, "ingredients": []string{"4 apples"}, "prep_minutes": 20, "cook_minutes": 40},
		{"title": "Bean stew", "tags": []string{"Dinner"}, "ingredients": []string{"1 can beans"}},
		{"title": "Carrot cake", "tags": []string{"dessert", "Baking"}, "ingredients": []string{"3 carrots"}},
	} {
		h.create(scott, in)
	}

	r := h.do("GET", "/v1/recipes?limit=2", nil)
	h.want(r, 200, "")
	m := r.json(t)
	items := m["items"].([]any)
	if len(items) != 2 || m["total"] != 3.0 || m["next_cursor"] == nil {
		t.Fatalf("page 1 = %v", m)
	}
	first := items[0].(map[string]any)
	if first["title"] != "Apple pie" || first["total_minutes"] != 60.0 || first["cover"] != nil {
		t.Errorf("summary = %v", first)
	}
	r = h.do("GET", "/v1/recipes?limit=2&cursor="+m["next_cursor"].(string), nil)
	h.want(r, 200, "")
	m = r.json(t)
	if items := m["items"].([]any); len(items) != 1 || m["next_cursor"] != nil {
		t.Errorf("page 2 = %v", m)
	}

	titles := func(query string) []string {
		t.Helper()
		r := h.do("GET", "/v1/recipes?"+query, nil, scott...)
		h.want(r, 200, "")
		var out []string
		for _, it := range r.json(t)["items"].([]any) {
			out = append(out, it.(map[string]any)["title"].(string))
		}
		return out
	}
	if got := titles("tag=DESSERT"); !slices.Equal(got, []string{"Apple pie", "Carrot cake"}) {
		t.Errorf("tag=DESSERT: %v", got)
	}
	if got := titles("tag=dessert&tag=baking"); !slices.Equal(got, []string{"Carrot cake"}) {
		t.Errorf("tag AND: %v", got)
	}
	if got := titles("q=bean"); !slices.Equal(got, []string{"Bean stew"}) {
		t.Errorf("q=bean: %v", got)
	}
	if got := titles("ingredient=carrot&sort=created"); !slices.Equal(got, []string{"Carrot cake"}) {
		t.Errorf("ingredient: %v", got)
	}
	if got := titles("favorite=true"); len(got) != 0 {
		t.Errorf("favorite with none set: %v", got)
	}

	h.want(h.do("GET", "/v1/recipes?limit=many", nil), 400, "invalid")
	h.want(h.do("GET", "/v1/recipes?favorite=maybe", nil), 400, "invalid")
	h.want(h.do("GET", "/v1/recipes?sort=x", nil), 400, "invalid")
	h.want(h.do("GET", "/v1/recipes?q=a&q=b", nil), 400, "invalid")
	h.want(h.do("GET", "/v1/recipes?cursor=bogus", nil), 400, "invalid_cursor")
	h.want(h.do("GET", "/v1/trash?limit=-1", nil), 400, "invalid")
}

func TestMineAndTags(t *testing.T) {
	h := newHarness(t)
	scott := h.person("Scott")
	id := h.create(scott, map[string]any{"title": "Toast", "tags": []string{"Breakfast", "Quick"}})["id"].(string)
	h.create(scott, map[string]any{"title": "Eggs", "tags": []string{"Breakfast"}})

	h.want(h.do("PUT", "/v1/recipes/"+id+"/mine", map[string]any{"favorite": true}), 401, "who_are_you")
	h.want(h.do("PUT", "/v1/recipes/nope/mine", map[string]any{"favorite": true}, scott...), 404, "not_found")
	h.want(h.do("PUT", "/v1/recipes/"+id+"/mine", map[string]any{"favourite": true}, scott...), 400, "invalid")
	r := h.do("PUT", "/v1/recipes/"+id+"/mine", map[string]any{"favorite": true}, scott...)
	h.want(r, 200, "")
	if string(r.body) != `{"favorite":true,"want_to_cook":false}`+"\n" {
		t.Errorf("mine = %s", r.body)
	}
	r = h.do("PUT", "/v1/recipes/"+id+"/mine", map[string]any{"want_to_cook": true}, scott...)
	if string(r.body) != `{"favorite":true,"want_to_cook":true}`+"\n" {
		t.Errorf("mine merge = %s", r.body)
	}
	if got := h.do("GET", "/v1/recipes?favorite=true", nil, scott...).json(t)["items"].([]any); len(got) != 1 {
		t.Errorf("favorites = %v", got)
	}

	r = h.do("GET", "/v1/tags", nil)
	h.want(r, 200, "")
	if string(r.body) != `{"items":[{"name":"Breakfast","count":2},{"name":"Quick","count":1}]}`+"\n" {
		t.Errorf("tags = %s", r.body)
	}
}

func TestImages(t *testing.T) {
	h := newHarness(t)
	img := pngBytes(t)
	h.want(h.do("POST", "/v1/images", img, "Content-Type", "image/png"), 401, "who_are_you")
	scott := h.person("Scott")
	r := h.do("POST", "/v1/images", img, append(scott, "Content-Type", "image/png")...)
	h.want(r, 201, "")
	hash := r.json(t)["hash"].(string)
	if len(hash) != 64 {
		t.Fatalf("hash = %q", hash)
	}
	h.want(h.do("POST", "/v1/images", []byte("not an image"), append(scott, "Content-Type", "image/png")...), 415, "unsupported_media_type")

	r = h.do("GET", "/v1/images/"+hash, nil)
	h.want(r, 200, "")
	if !bytes.Equal(r.body, img) {
		t.Error("image bytes differ")
	}
	if ct := r.header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q", ct)
	}
	if cc := r.header.Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q", cc)
	}
	h.want(h.do("GET", "/v1/images/"+strings.Repeat("0", 64), nil), 404, "not_found")
	h.want(h.do("GET", "/v1/images/..%2f..%2fsous.db", nil), 404, "not_found")

	rec := h.create(scott, map[string]any{"title": "Pie", "images": []string{hash}})
	if rec["images"].([]any)[0] != hash {
		t.Errorf("images = %v", rec["images"])
	}
}

func TestChanges(t *testing.T) {
	h := newHarness(t)
	scott := h.person("Scott")
	id := h.create(append(scott, "X-Sous-Via", "ui"), map[string]any{"title": "Soup"})["id"].(string)
	h.do("PATCH", "/v1/recipes/"+id, map[string]any{"notes": "Salt."}, append(scott, "If-Match", `"1"`)...)

	r := h.do("GET", "/v1/changes", nil)
	h.want(r, 200, "")
	items := r.json(t)["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("changes = %v", items)
	}
	newest := items[0].(map[string]any)
	if newest["action"] != "update" || newest["via"] != "api" || newest["person_id"] != "scott" {
		t.Errorf("newest = %v", newest)
	}
	r = h.do("GET", "/v1/changes?via=ui&entity_id="+id, nil)
	h.want(r, 200, "")
	if items := r.json(t)["items"].([]any); len(items) != 1 || items[0].(map[string]any)["action"] != "create" {
		t.Errorf("via=ui = %v", items)
	}
	h.want(h.do("GET", "/v1/changes?via=fax", nil), 400, "invalid")
	h.want(h.do("GET", "/v1/changes?limit=2&cursor=bogus", nil), 400, "invalid_cursor")
}

func TestParse(t *testing.T) {
	h := newHarness(t)
	r := h.do("POST", "/v1/ingredients/parse", map[string]any{"lines": []string{"# Dough", "2 cups flour"}})
	h.want(r, 200, "")
	items := r.json(t)["items"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["section"] != "Dough" || items[1].(map[string]any)["unit"] != "cup" {
		t.Errorf("parse = %s", r.body)
	}
	h.want(h.do("POST", "/v1/ingredients/parse", map[string]any{"lines": "flour"}), 400, "invalid")
}

func TestRouting(t *testing.T) {
	h := newHarness(t)
	h.want(h.do("GET", "/v1/nope", nil), 404, "not_found")
	h.want(h.do("GET", "/v1/recipes/a/b/c", nil), 404, "not_found")
	r := h.do("DELETE", "/v1/tags", nil)
	h.want(r, 405, "method_not_allowed")
	if a := r.header.Get("Allow"); a != "GET, HEAD" {
		t.Errorf("Allow = %q", a)
	}
	r = h.do("PUT", "/v1/recipes/x", nil)
	h.want(r, 405, "method_not_allowed")
	if a := r.header.Get("Allow"); a != "DELETE, GET, HEAD, PATCH" {
		t.Errorf("Allow = %q", a)
	}

	r = h.do("GET", "/v1/openapi.json", nil)
	h.want(r, 200, "")
	if m := r.json(t); m["openapi"] != "3.1.0" {
		t.Errorf("openapi = %v", m["openapi"])
	}
}

func TestCrossOrigin(t *testing.T) {
	h := newHarness(t)
	h.want(h.do("POST", "/v1/people", map[string]any{"name": "Eve"}, "Sec-Fetch-Site", "cross-site"), 403, "cross_origin")
	h.want(h.do("POST", "/v1/people", map[string]any{"name": "Eve"}, "Origin", "https://evil.example"), 403, "cross_origin")
	h.want(h.do("GET", "/v1/people", nil, "Sec-Fetch-Site", "cross-site"), 200, "")
	h.want(h.do("POST", "/v1/people", map[string]any{"name": "Ann"}, "Sec-Fetch-Site", "same-origin"), 201, "")
}

// callerRecorder stands in for cmd/sous's request log writer.
type callerRecorder struct {
	*httptest.ResponseRecorder
	caller identity.Caller
	set    bool
}

func (c *callerRecorder) SetCaller(cl identity.Caller) { c.caller, c.set = cl, true }

func TestCallerHook(t *testing.T) {
	h := newHarness(t)
	h.person("Scott")
	req := httptest.NewRequest("GET", "/v1/people", nil)
	req.Header.Set("X-Sous-Person", "scott")
	req.Header.Set("X-Sous-Via", "ui")
	w := &callerRecorder{ResponseRecorder: httptest.NewRecorder()}
	h.h.ServeHTTP(w, req)
	if !w.set || w.caller.Username() != "scott" || w.caller.Route != identity.RouteDirect || w.caller.Via != identity.ViaUI {
		t.Errorf("caller = %+v (set %v)", w.caller, w.set)
	}

	req = httptest.NewRequest("GET", "/v1/people", nil)
	req.Header.Set("X-Sous-Via", "agent")
	w = &callerRecorder{ResponseRecorder: httptest.NewRecorder()}
	h.h.ServeHTTP(w, req)
	if !w.set || w.caller.Person != nil || w.caller.Route != identity.RouteNone || w.caller.Via != identity.ViaAPI {
		t.Errorf("nobody caller = %+v (set %v)", w.caller, w.set)
	}
}

func TestWithIdentity(t *testing.T) {
	h := newHarness(t)
	h.person("Scott")
	var gotHeader []string
	var gotCaller identity.Caller
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Values(CallerHeader)
		gotCaller, _ = identity.CallerFrom(r.Context())
	})
	mw := WithIdentity(h.res, identity.ViaMCP, next)

	req := httptest.NewRequest("POST", "/mcp", nil)
	req.Header.Set("X-Sous-Person", "scott")
	req.Header.Set(CallerHeader, "mallory")
	w := &callerRecorder{ResponseRecorder: httptest.NewRecorder()}
	mw.ServeHTTP(w, req)
	if !slices.Equal(gotHeader, []string{"scott"}) {
		t.Errorf("caller header = %q", gotHeader)
	}
	if gotCaller.Username() != "scott" || gotCaller.Via != identity.ViaMCP || !w.set {
		t.Errorf("caller = %+v, hook set %v", gotCaller, w.set)
	}
	if req.Header.Get(CallerHeader) != "mallory" {
		t.Error("WithIdentity changed the caller's request headers")
	}

	req = httptest.NewRequest("POST", "/mcp", nil)
	req.Header.Set(CallerHeader, "mallory")
	mw.ServeHTTP(httptest.NewRecorder(), req)
	if len(gotHeader) != 0 || gotCaller.Person != nil || gotCaller.Via != identity.ViaMCP {
		t.Errorf("nobody: header %q, caller %+v", gotHeader, gotCaller)
	}

	req = httptest.NewRequest("POST", "/mcp", nil)
	req.Header.Set("X-Sous-Person", "ghost")
	rec := httptest.NewRecorder()
	gotCaller = identity.Caller{}
	mw.ServeHTTP(rec, req)
	if rec.Code != 401 || !strings.Contains(rec.Body.String(), `"code":"unknown_person"`) || rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("unknown person: %d %s", rec.Code, rec.Body)
	}
	if gotCaller.Via != "" {
		t.Error("next ran after a failed resolve")
	}
}

func TestInternalErrorsHideCause(t *testing.T) {
	h := newHarness(t)
	h.st.Close()
	r := h.do("GET", "/v1/tags", nil)
	h.want(r, 503, "unavailable")
	if msg := r.json(t)["error"].(map[string]any)["message"]; msg != "Sous can't reach its storage right now" {
		t.Errorf("message = %v", msg)
	}
	if !strings.Contains(h.logs.String(), "request failed") {
		t.Errorf("cause not logged: %s", h.logs)
	}
}
