package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/recipes"
	"github.com/sroberts/sous/internal/storage"
)

type oneNodeGen struct{}

func (oneNodeGen) Generate(ctx context.Context, prompt string, schema any) ([]byte, error) {
	return json.Marshal(map[string]any{"serve": "toast",
		"items": []any{map[string]any{"id": "bread", "kind": "ingredient", "label": "bread", "line": 1},
			map[string]any{"id": "toast", "kind": "final", "label": "toast"}},
		"nodes": []any{map[string]any{"id": "toast", "step": 1, "quote": "Toast the bread.", "text": "Toast",
			"inputs": []string{"bread"}, "outputs": []string{"toast"}, "implied": false}}})
}

type unsureDec struct{}

func (unsureDec) Decide(ctx context.Context, state string, qs []recipes.Question) (map[string]recipes.Answer, error) {
	out := map[string]recipes.Answer{}
	for _, q := range qs {
		probs := map[string]float64{}
		for i, c := range q.Choices {
			probs[c.Label] = 0.1
			if i == 0 {
				probs[c.Label] = 0.6
			}
		}
		out[q.ID] = recipes.Answer{Probabilities: probs}
	}
	return out, nil
}

// upgradeHarness is newHarness with upgrades on, over stand-in models.
func upgradeHarness(t *testing.T) *harness {
	t.Helper()
	logs := &bytes.Buffer{}
	log := slog.New(slog.NewTextHandler(logs, nil))
	st, err := storage.Open(context.Background(), t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	svc := recipes.New(st, log)
	svc.SetUpgrader(&recipes.Upgrader{Gen: oneNodeGen{}, Dec: unsureDec{}})
	res := identity.NewResolver(identity.ResolverConfig{Directory: svc})
	h := New(Deps{Service: svc, Resolver: res, Log: log, Version: "test", Auth: "none"})
	return &harness{t: t, h: h, st: st, res: res, logs: logs, schema: newSchemaChecker(t)}
}

func TestUpgradeAPI(t *testing.T) {
	h := upgradeHarness(t)
	ann := h.person("Ann")
	rec := h.create(ann, map[string]any{"title": "Toast", "ingredients": []string{"bread"}, "steps": []string{"Toast the bread."}})
	id := rec["id"].(string)

	h.want(h.do("POST", "/v1/recipes/"+id+"/graph/upgrade", map[string]any{}), 401, "who_are_you")
	r := h.do("POST", "/v1/recipes/"+id+"/graph/upgrade", map[string]any{"mode": "apply"}, ann...)
	h.want(r, 200, "")
	up := r.json(t)
	g := up["graph"].(map[string]any)
	if g["graph"].(map[string]any)["status"] != "needs_review" || g["review_count"].(float64) < 1 {
		t.Fatalf("upgrade = %v", up)
	}
	h.want(h.do("POST", "/v1/recipes/"+id+"/graph/upgrade", map[string]any{}, ann...), 409, "exists")

	r = h.do("GET", "/v1/review?recipe_id="+id+"&limit=1", nil)
	h.want(r, 200, "")
	page := r.json(t)
	items := page["items"].([]any)
	if len(items) != 1 || page["next_cursor"] == nil {
		t.Fatalf("review = %v", page)
	}
	h.want(h.do("GET", "/v1/review?cursor=nonsense", nil), 400, "invalid_cursor")
	first := items[0].(map[string]any)
	r = h.do("POST", "/v1/review", map[string]any{"recipe_id": id, "graph_version": first["graph_version"],
		"items": []any{map[string]any{"node_id": first["node_id"], "field": first["field"], "value": first["value"]}}}, ann...)
	h.want(r, 200, "")
	if left := r.json(t)["remaining"].(float64); left != page["remaining"].(float64)-1 {
		t.Errorf("remaining %v after one of %v", left, page["remaining"])
	}

	r = h.do("POST", "/v1/upgrades", map[string]any{"ids": []string{id}, "force": true}, ann...)
	h.want(r, 201, "")
	job := r.json(t)
	h.want(h.do("GET", "/v1/upgrades/"+job["id"].(string), nil), 200, "")
	r = h.do("POST", "/v1/upgrades/"+job["id"].(string)+"/cancel", nil, ann...)
	h.want(r, 200, "")
	if r.json(t)["status"] != "cancelled" {
		t.Errorf("cancel = %s", r.body)
	}
	h.want(h.do("GET", "/v1/upgrades/nope", nil), 404, "not_found")

	r = h.do("GET", "/v1/graphs", nil)
	h.want(r, 200, "")
	o := r.json(t)
	if o["counts"].(map[string]any)["needs_review"] != 1.0 || o["upgrades"].(map[string]any)["graph_model"] != true {
		t.Errorf("overview = %s", r.body)
	}
}

// Without models, upgrading says so with a 503.
func TestUpgradeAPIOff(t *testing.T) {
	h := newHarness(t)
	ann := h.person("Ann")
	rec := h.create(ann, map[string]any{"title": "Toast", "steps": []string{"Toast."}})
	h.want(h.do("POST", "/v1/recipes/"+rec["id"].(string)+"/graph/upgrade", map[string]any{}, ann...), 503, "unavailable")
	h.want(h.do("POST", "/v1/upgrades", map[string]any{}, ann...), 503, "unavailable")
}

func TestDraftAPI(t *testing.T) {
	h := upgradeHarness(t)
	ann := h.person("Ann")
	rec := h.create(ann, map[string]any{"title": "Toast", "ingredients": []string{"bread"}, "steps": []string{"Toast the bread."}})
	id := rec["id"].(string)
	r := h.do("GET", "/v1/graphs/todo", nil)
	h.want(r, 200, "")
	if r.json(t)["remaining"] != 1.0 {
		t.Fatalf("todo = %s", r.body)
	}
	body := map[string]any{"recipe_version": rec["version"], "serve": "toast",
		"items": []any{map[string]any{"id": "bread", "kind": "ingredient", "label": "bread", "line": 1},
			map[string]any{"id": "toast", "kind": "final", "label": "toast"}},
		"nodes": []any{map[string]any{"id": "toast", "step": 1, "quote": "Toast the bread.", "text": "Toast",
			"inputs": []string{"bread"}, "outputs": []string{"toast"}, "implied": false}}}
	h.want(h.do("POST", "/v1/recipes/"+id+"/graph/draft", body), 401, "who_are_you")
	r = h.do("POST", "/v1/recipes/"+id+"/graph/draft", body, ann...)
	h.want(r, 200, "")
	if g := r.json(t)["graph"].(map[string]any)["graph"].(map[string]any); g["status"] != "draft" {
		t.Errorf("draft = %s", r.body)
	}
	h.want(h.do("GET", "/v1/graphs/todo?limit=0", nil), 200, "")
	h.want(h.do("POST", "/v1/recipes/"+id+"/graph/draft", body, ann...), 409, "exists")
}
