package mcp

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sroberts/sous/internal/recipes"
)

// stubGen writes a fixed draft; stubDec is sure of everything except
// whether toasting needs watching.
type stubGen struct{}

func (stubGen) Generate(ctx context.Context, prompt string, schema any) ([]byte, error) {
	return json.Marshal(map[string]any{
		"serve": "toast",
		"items": []any{
			map[string]any{"id": "bread", "kind": "ingredient", "label": "bread", "line": 1},
			map[string]any{"id": "toast", "kind": "final", "label": "toast"},
		},
		"nodes": []any{
			map[string]any{"id": "toast", "step": 1, "quote": "Toast the bread for 5 minutes.", "text": "Toast the bread",
				"inputs": []string{"bread"}, "outputs": []string{"toast"}, "implied": false},
		},
	})
}

type stubDec struct{}

func (stubDec) Decide(ctx context.Context, state string, qs []recipes.Question) (map[string]recipes.Answer, error) {
	top := map[string]string{"action": "heat", "attention": "intermittent", "check": "2", "interruptible": "no",
		"anchor": "just_before_serve", "hold": "none", "where": "counter", "duration": "5", "equipment": "none"}
	out := map[string]recipes.Answer{}
	for _, q := range qs {
		p := 0.95
		if q.ID == "attention" {
			p = 0.5
		}
		probs := map[string]float64{}
		for _, c := range q.Choices {
			probs[c.Label] = (1 - p) / float64(len(q.Choices)-1)
		}
		probs[top[q.ID]] = p
		out[q.ID] = recipes.Answer{Probabilities: probs}
	}
	return out, nil
}

func TestUpgradeTools(t *testing.T) {
	svc := newService(t)
	svc.SetUpgrader(&recipes.Upgrader{Gen: stubGen{}, Dec: stubDec{}})
	url := serve(t, standIn(New(svc, slog.New(slog.DiscardHandler), "test")))
	alice := as(t, url, "alice")
	r := ok[recipes.Recipe](t, alice, "create_recipe", map[string]any{
		"title": "Toast", "ingredients": []string{"2 slices bread"}, "steps": []string{"Toast the bread for 5 minutes."},
	})

	res := ok[recipes.UpgradeResult](t, alice, "upgrade_recipe", map[string]any{"id": r.ID})
	if res.Graph.Graph.Status != "draft" || res.Graph.ReviewCount != 1 || len(res.Graph.Graph.Nodes) != 1 {
		t.Fatalf("upgrade = %+v", res)
	}
	if msg := fails(t, alice, "upgrade_recipe", map[string]any{"id": r.ID}, "exists"); !strings.Contains(msg, "force") {
		t.Errorf("again: %q", msg)
	}

	page := ok[recipes.ReviewPage](t, alice, "list_review_items", map[string]any{"recipe_id": r.ID})
	if page.Remaining != 1 || page.Items[0].Field != "attention" || page.Items[0].Value != "intermittent" {
		t.Fatalf("review = %+v", page)
	}
	out := ok[recipes.ResolveResult](t, alice, "resolve_review_items", map[string]any{
		"recipe_id": r.ID, "graph_version": page.Items[0].GraphVersion, "accept": true,
		"items": []any{map[string]any{"node_id": "toast", "field": "attention", "value": "active"}},
	})
	if out.Remaining != 0 || out.Graph.Graph.Status != "verified" {
		t.Errorf("resolve = %+v", out)
	}

	job := ok[recipes.UpgradeJob](t, alice, "upgrade_recipes", map[string]any{"ids": []string{r.ID}, "force": true})
	if job.Status != "queued" || len(job.Items) != 1 {
		t.Fatalf("job = %+v", job)
	}
	if got := ok[recipes.UpgradeJob](t, alice, "get_upgrade_job", map[string]any{"id": job.ID}); got.ID != job.ID {
		t.Errorf("get job = %+v", got)
	}
	if got := ok[recipes.UpgradeJob](t, alice, "cancel_upgrade_job", map[string]any{"id": job.ID}); got.Status != "cancelled" {
		t.Errorf("cancel = %+v", got)
	}
	fails(t, alice, "get_upgrade_job", map[string]any{"id": "nope"}, "not_found")
}

func TestUpgradeOff(t *testing.T) {
	url := serve(t, standIn(New(newService(t), slog.New(slog.DiscardHandler), "test")))
	alice := as(t, url, "alice")
	r := ok[recipes.Recipe](t, alice, "create_recipe", map[string]any{"title": "Toast", "steps": []string{"Toast."}})
	if msg := fails(t, alice, "upgrade_recipe", map[string]any{"id": r.ID}, "unavailable"); !strings.Contains(msg, "off") {
		t.Errorf("off: %q", msg)
	}
}

// Without a graph model, an assistant drafts the structure and Sous
// labels it.
func TestDraftTools(t *testing.T) {
	svc := newService(t)
	svc.SetUpgrader(&recipes.Upgrader{Dec: stubDec{}})
	url := serve(t, standIn(New(svc, slog.New(slog.DiscardHandler), "test")))
	alice := as(t, url, "alice")
	r := ok[recipes.Recipe](t, alice, "create_recipe", map[string]any{
		"title": "Toast", "ingredients": []string{"2 slices bread"}, "steps": []string{"Toast the bread for 5 minutes."},
	})
	todo := ok[recipes.GraphTodos](t, alice, "list_recipes_without_graph", map[string]any{})
	if todo.Remaining != 1 || todo.Items[0].ID != r.ID {
		t.Fatalf("todo = %+v", todo)
	}
	if msg := fails(t, alice, "upgrade_recipe", map[string]any{"id": r.ID}, "unavailable"); !strings.Contains(msg, "draft_recipe_graph") {
		t.Errorf("upgrade without a graph model: %q", msg)
	}
	draft := map[string]any{
		"id": r.ID, "recipe_version": r.Version, "serve": "toast",
		"items": []any{
			map[string]any{"id": "bread", "kind": "ingredient", "label": "bread", "line": 1},
			map[string]any{"id": "toast", "kind": "final", "label": "toast"},
		},
		"nodes": []any{map[string]any{"id": "toast", "step": 1, "quote": "Grill the bread", "text": "Toast the bread",
			"inputs": []string{"bread"}, "outputs": []string{"toast"}, "implied": false}},
	}
	if msg := fails(t, alice, "draft_recipe_graph", draft, "invalid"); !strings.Contains(msg, "quote isn't in step 1") {
		t.Errorf("bad draft: %q", msg)
	}
	draft["nodes"].([]any)[0].(map[string]any)["quote"] = "Toast the bread for 5 minutes."
	res := ok[recipes.UpgradeResult](t, alice, "draft_recipe_graph", draft)
	if res.Graph.Graph.Status != "draft" || res.Graph.ReviewCount != 1 || res.Graph.Graph.Nodes[0].Action.Source != "jev" {
		t.Errorf("draft = %+v", res)
	}

	cs := as(t, url, "alice")
	p, err := cs.GetPrompt(context.Background(), &sdk.GetPromptParams{Name: "upgrade_step_graphs", Arguments: map[string]string{"meal": "Thanksgiving"}})
	if err != nil {
		t.Fatal(err)
	}
	text := p.Messages[0].Content.(*sdk.TextContent).Text
	if !strings.Contains(text, `the meal "Thanksgiving"`) || !strings.Contains(text, "draft_recipe_graph") || !strings.Contains(text, "plan_meal and tell me") {
		t.Errorf("prompt: %s", text)
	}
	p, err = cs.GetPrompt(context.Background(), &sdk.GetPromptParams{Name: "upgrade_step_graphs"})
	if err != nil || strings.Contains(p.Messages[0].Content.(*sdk.TextContent).Text, "plan_meal and tell me") {
		t.Errorf("library prompt: %v", err)
	}
}
