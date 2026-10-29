package mcp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/recipes"
	"github.com/sroberts/sous/internal/storage"
)

var ctx = context.Background()

func newService(t *testing.T) *recipes.Service {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := storage.Open(ctx, t.TempDir(), log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	svc := recipes.New(st, log)
	for _, name := range []string{"Alice", "Bob"} {
		if _, err := svc.CreatePerson(ctx, identity.Caller{Via: identity.ViaAPI}, name); err != nil {
			t.Fatal(err)
		}
	}
	return svc
}

// standIn plays api.WithIdentity's part for these tests: it drops any
// client-sent HeaderCaller and sets it from X-Sous-Person, unverified.
func standIn(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del(HeaderCaller)
		if p := r.Header.Get(identity.HeaderPerson); p != "" {
			r.Header.Set(HeaderCaller, p)
		}
		next.ServeHTTP(w, r)
	})
}

func serve(t *testing.T, h http.Handler) string {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv.URL
}

type headerTransport struct{ h http.Header }

func (ht headerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	for k, v := range ht.h {
		r.Header[k] = v
	}
	return http.DefaultTransport.RoundTrip(r)
}

// connect opens a client session that sends extra headers on every request.
func connect(t *testing.T, url string, headers map[string]string) *sdk.ClientSession {
	t.Helper()
	h := http.Header{}
	for k, v := range headers {
		h.Set(k, v)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, &sdk.StreamableClientTransport{
		Endpoint:             url,
		HTTPClient:           &http.Client{Transport: headerTransport{h}},
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func as(t *testing.T, url, person string) *sdk.ClientSession {
	t.Helper()
	if person == "" {
		return connect(t, url, nil)
	}
	return connect(t, url, map[string]string{identity.HeaderPerson: person})
}

func call(t *testing.T, cs *sdk.ClientSession, name string, args any) *sdk.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res
}

func text(res *sdk.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*sdk.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// ok calls a tool, requires success, and decodes its structured content.
func ok[T any](t *testing.T, cs *sdk.ClientSession, name string, args any) T {
	t.Helper()
	res := call(t, cs, name, args)
	if res.IsError {
		t.Fatalf("%s: unexpected error: %s", name, text(res))
	}
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out T
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("%s: decoding %s: %v", name, b, err)
	}
	return out
}

// fails calls a tool and requires an error result whose text starts with code.
func fails(t *testing.T, cs *sdk.ClientSession, name string, args any, code string) string {
	t.Helper()
	res := call(t, cs, name, args)
	if !res.IsError {
		t.Fatalf("%s: want error %s, got success: %s", name, code, text(res))
	}
	if got := text(res); !strings.HasPrefix(got, code+": ") {
		t.Fatalf("%s: want %s, got %q", name, code, got)
	}
	return text(res)
}

func TestListTools(t *testing.T) {
	url := serve(t, standIn(New(newService(t), slog.New(slog.DiscardHandler), "test")))
	res, err := as(t, url, "").ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	tools := map[string]*sdk.Tool{}
	for _, tool := range res.Tools {
		tools[tool.Name] = tool
	}
	want := []string{"search_recipes", "get_recipe", "create_recipe", "update_recipe", "delete_recipe",
		"restore_recipe", "set_my_recipe_flags", "list_tags", "recent_changes"}
	if len(tools) != len(want) {
		t.Errorf("got %d tools, want %d", len(tools), len(want))
	}
	for _, name := range want {
		tool := tools[name]
		if tool == nil {
			t.Errorf("missing tool %s", name)
			continue
		}
		if tool.Description == "" || tool.OutputSchema == nil || tool.Annotations == nil {
			t.Errorf("%s: want description, output schema and annotations", name)
		}
	}

	upd := tools["update_recipe"].InputSchema.(map[string]any)
	props := upd["properties"].(map[string]any)
	for field, typ := range map[string]string{"title": "string", "prep_minutes": "integer", "ingredients": "array"} {
		types, _ := props[field].(map[string]any)["type"].([]any)
		if !slices.Contains(types, "null") || !slices.Contains(types, any(typ)) {
			t.Errorf("update_recipe.%s type = %v, want [null %s]", field, props[field], typ)
		}
	}
	req, _ := upd["required"].([]any)
	if !slices.Contains(req, "id") || !slices.Contains(req, "version") || slices.Contains(req, "title") {
		t.Errorf("update_recipe required = %v, want id and version only", req)
	}
	del := tools["delete_recipe"].InputSchema.(map[string]any)
	if req, _ := del["required"].([]any); !slices.Contains(req, "version") {
		t.Errorf("delete_recipe required = %v, want version", req)
	}

	if a := tools["delete_recipe"].Annotations; a.DestructiveHint == nil || !*a.DestructiveHint {
		t.Error("delete_recipe should be destructive")
	}
	if a := tools["search_recipes"].Annotations; !a.ReadOnlyHint {
		t.Error("search_recipes should be read-only")
	}
	if a := tools["create_recipe"].Annotations; a.ReadOnlyHint || a.IdempotentHint {
		t.Error("create_recipe is neither read-only nor idempotent")
	}
}

func TestRecipeLifecycle(t *testing.T) {
	url := serve(t, standIn(New(newService(t), slog.New(slog.DiscardHandler), "test")))
	alice := as(t, url, "alice")

	created := ok[recipes.Recipe](t, alice, "create_recipe", map[string]any{
		"title":       "Pad Thai",
		"yield_text":  "Serves 4",
		"tags":        []string{"Main", "Thai"},
		"ingredients": []string{"# Sauce", "2 tbsp fish sauce", "200 g rice noodles"},
		"steps":       []string{"Soak the noodles.", "Fry everything."},
	})
	if created.ID == "" || created.Version != 1 || created.CreatedBy != "alice" {
		t.Fatalf("created = %+v", created)
	}
	if s := created.Ingredients[0].Section; s == nil || *s != "Sauce" || created.Ingredients[0].Unit == nil {
		t.Errorf("ingredient 0 = %+v, want parsed under section Sauce", created.Ingredients[0])
	}

	got := ok[recipes.Recipe](t, alice, "get_recipe", map[string]any{"id": created.ID})
	if got.Title != "Pad Thai" || got.Version != 1 {
		t.Errorf("get = %+v", got)
	}

	v2 := ok[recipes.Recipe](t, alice, "update_recipe", map[string]any{"id": created.ID, "version": 1, "notes": "Use tamarind."})
	if v2.Version != 2 || v2.Notes == nil {
		t.Fatalf("update = %+v", v2)
	}

	msg := fails(t, alice, "update_recipe", map[string]any{"id": created.ID, "version": 1, "title": "Stale"}, "stale")
	if !strings.Contains(msg, `"version":2`) || !strings.Contains(msg, "Use tamarind.") {
		t.Errorf("stale error should carry the current recipe: %q", msg)
	}

	v3 := ok[recipes.Recipe](t, alice, "update_recipe", map[string]any{
		"id": created.ID, "version": 2, "title": "Pad Thai Gai", "notes": nil, "prep_minutes": 20,
	})
	if v3.Version != 3 || v3.Title != "Pad Thai Gai" || v3.Notes != nil || v3.PrepMinutes == nil || *v3.PrepMinutes != 20 {
		t.Errorf("update = %+v", v3)
	}
	if len(v3.Ingredients) != 2 || len(v3.Tags) != 2 {
		t.Errorf("omitted lists should be untouched: %+v", v3)
	}

	if res := call(t, alice, "delete_recipe", map[string]any{"id": created.ID}); !res.IsError {
		t.Error("delete_recipe without version should be refused")
	}
	deleted := ok[recipes.Recipe](t, alice, "delete_recipe", map[string]any{"id": created.ID, "version": 3})
	if deleted.DeletedAt == nil || deleted.Version != 4 {
		t.Errorf("deleted = %+v", deleted)
	}
	page := ok[recipes.Page[recipes.Summary]](t, alice, "search_recipes", map[string]any{"q": "pad"})
	if page.Total != 0 {
		t.Errorf("search found a deleted recipe: %+v", page)
	}

	restored := ok[recipes.Recipe](t, alice, "restore_recipe", map[string]any{"id": created.ID})
	if restored.DeletedAt != nil || restored.Version != 5 {
		t.Errorf("restored = %+v", restored)
	}

	fails(t, alice, "get_recipe", map[string]any{"id": "nope"}, recipes.CodeNotFound)
}

func TestFlagsAndSearch(t *testing.T) {
	url := serve(t, standIn(New(newService(t), slog.New(slog.DiscardHandler), "test")))
	alice, bob := as(t, url, "alice"), as(t, url, "bob")
	r := ok[recipes.Recipe](t, alice, "create_recipe", map[string]any{"title": "Dal", "tags": []string{"Main"}})
	ok[recipes.Recipe](t, alice, "create_recipe", map[string]any{"title": "Flapjack"})

	mine := ok[recipes.Mine](t, alice, "set_my_recipe_flags", map[string]any{"id": r.ID, "favorite": true})
	if !mine.Favorite || mine.WantToCook {
		t.Errorf("mine = %+v", mine)
	}
	mine = ok[recipes.Mine](t, alice, "set_my_recipe_flags", map[string]any{"id": r.ID, "want_to_cook": true})
	if !mine.Favorite || !mine.WantToCook {
		t.Errorf("omitted flag should keep its value: %+v", mine)
	}

	page := ok[recipes.Page[recipes.Summary]](t, alice, "search_recipes", map[string]any{"favorite": true})
	if page.Total != 1 || page.Items[0].ID != r.ID || !page.Items[0].Mine.Favorite {
		t.Errorf("alice's favourites = %+v", page)
	}
	if page := ok[recipes.Page[recipes.Summary]](t, bob, "search_recipes", map[string]any{"favorite": true}); page.Total != 0 {
		t.Errorf("bob's favourites = %+v", page)
	}
	if page := ok[recipes.Page[recipes.Summary]](t, bob, "search_recipes", map[string]any{}); page.Total != 2 {
		t.Errorf("all recipes = %+v", page)
	}

	tags := ok[recipes.Items[recipes.TagCount]](t, bob, "list_tags", nil)
	if len(tags.Items) != 1 || tags.Items[0] != (recipes.TagCount{Name: "Main", Count: 1}) {
		t.Errorf("tags = %+v", tags)
	}
}

func TestRecentChangesAttributesMCP(t *testing.T) {
	url := serve(t, standIn(New(newService(t), slog.New(slog.DiscardHandler), "test")))
	bob := as(t, url, "bob")
	r := ok[recipes.Recipe](t, bob, "create_recipe", map[string]any{"title": "Soda Bread"})

	page := ok[recipes.ChangePage](t, bob, "recent_changes", map[string]any{"entity_id": r.ID})
	if len(page.Items) != 1 {
		t.Fatalf("changes = %+v", page)
	}
	c := page.Items[0]
	if c.Via != "mcp" || c.PersonID == nil || *c.PersonID != "bob" || c.Action != "create" {
		t.Errorf("change = %+v, want create by bob via mcp", c)
	}
	if page := ok[recipes.ChangePage](t, bob, "recent_changes", map[string]any{"via": "mcp", "person": "bob"}); len(page.Items) != 1 {
		t.Errorf("filtered changes = %+v", page)
	}
}

func TestWriteFromNobody(t *testing.T) {
	url := serve(t, standIn(New(newService(t), slog.New(slog.DiscardHandler), "test")))
	nobody := as(t, url, "")
	msg := fails(t, nobody, "create_recipe", map[string]any{"title": "Ghost Soup"}, recipes.CodeWhoAreYou)
	if !strings.Contains(msg, "X-Sous-Person") {
		t.Errorf("who_are_you should explain X-Sous-Person: %q", msg)
	}
	if page := ok[recipes.Page[recipes.Summary]](t, nobody, "search_recipes", nil); page.Total != 0 {
		t.Errorf("the write went through: %+v", page)
	}
}

// The caller can also arrive on the request context, as api.WithIdentity
// may store it; the SDK carries that context into handlers in stateless
// mode. Via is forced to mcp whatever the context says.
func TestCallerFromContext(t *testing.T) {
	svc := newService(t)
	h := New(svc, slog.New(slog.DiscardHandler), "test")
	url := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := &identity.Person{Username: "alice", Name: "Alice"}
		c := identity.Caller{Person: p, Route: identity.RouteGateway, Via: identity.ViaAPI}
		h.ServeHTTP(w, r.WithContext(identity.WithCaller(r.Context(), c)))
	}))
	cs := as(t, url, "")
	r := ok[recipes.Recipe](t, cs, "create_recipe", map[string]any{"title": "Ragu"})
	page := ok[recipes.ChangePage](t, cs, "recent_changes", map[string]any{"entity_id": r.ID})
	if c := page.Items[0]; c.Via != "mcp" || *c.PersonID != "alice" {
		t.Errorf("change = %+v", c)
	}
}

// New trusts HeaderCaller as given: mounted bare, a client can name anyone.
// The trust boundary is api.WithIdentity, which deletes the client's copy
// (standIn does the same) before setting it from the resolved identity.
func TestInternalHeaderTrustBoundary(t *testing.T) {
	svc := newService(t)
	h := New(svc, slog.New(slog.DiscardHandler), "test")

	bare := connect(t, serve(t, h), map[string]string{HeaderCaller: "bob"})
	r := ok[recipes.Recipe](t, bare, "create_recipe", map[string]any{"title": "Forged"})
	if r.CreatedBy != "bob" {
		t.Errorf("bare handler: created_by = %q, want the forged bob", r.CreatedBy)
	}
	fails(t, connect(t, serve(t, h), map[string]string{HeaderCaller: "mallory"}),
		"create_recipe", map[string]any{"title": "x"}, identity.CodeUnknownPerson)

	wrapped := connect(t, serve(t, standIn(h)), map[string]string{HeaderCaller: "bob"})
	fails(t, wrapped, "create_recipe", map[string]any{"title": "Forged"}, recipes.CodeWhoAreYou)
}

func TestCrossOriginDenied(t *testing.T) {
	url := serve(t, standIn(New(newService(t), slog.New(slog.DiscardHandler), "test")))
	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("Origin", "https://evil.example")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body struct{ Error struct{ Code string } }
	json.NewDecoder(resp.Body).Decode(&body)
	if resp.StatusCode != http.StatusForbidden || body.Error.Code != "cross_origin" {
		t.Errorf("got %d %+v, want 403 cross_origin", resp.StatusCode, body)
	}
}
