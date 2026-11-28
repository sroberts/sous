package ollama_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/sroberts/sous/internal/config"
	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/ollama"
	"github.com/sroberts/sous/internal/recipes"
	"github.com/sroberts/sous/internal/storage"
)

// TestLiveClassify measures the decision model alone: it sends the
// hand-built structure of each gate recipe through draft_recipe_graph, so
// the model labels exactly the actions labelled by hand, and compares
// every field. SOUS_LIVE_MODELS=1 runs it; SOUS_LIVE_VERBOSE=1 lists each
// miss.
func TestLiveClassify(t *testing.T) {
	if os.Getenv("SOUS_LIVE_MODELS") != "1" {
		t.Skip("set SOUS_LIVE_MODELS=1 to run against local models")
	}
	cfg, err := config.LoadUpgrade(os.Getenv)
	if err != nil || !cfg.On() {
		t.Fatalf("config: %v", err)
	}
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := storage.Open(ctx, t.TempDir(), log)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := recipes.New(st, log)
	oc := ollama.New(cfg.OllamaURL, "", cfg.DecisionModel)
	svc.SetUpgrader(&recipes.Upgrader{Dec: oc})
	p, _ := svc.CreatePerson(ctx, identity.Caller{Via: identity.ViaAPI}, "Eval")
	c := identity.Caller{Person: &identity.Person{Username: p.Username}, Route: identity.RouteDirect, Via: identity.ViaAPI}
	verbose := os.Getenv("SOUS_LIVE_VERBOSE") == "1"

	type tally struct{ sureRight, sure, unsureRight, unsure int }
	fields := []string{"action", "attention", "equipment", "hold", "interruptible"}
	tallies := map[string]*tally{}
	for _, f := range fields {
		tallies[f] = &tally{}
	}
	confusion := map[string]int{}
	files, _ := filepath.Glob(filepath.Join("..", "recipes", "testdata", "graphs", "*.json"))
	for _, file := range files {
		var fx struct {
			Sample string `json:"sample"`
			Serve  string `json:"serve"`
			Items  []struct {
				ID, Kind, Label, Line string
			} `json:"items"`
			Nodes []struct {
				ID          string                   `json:"id"`
				Quote       string                   `json:"quote"`
				ImpliedStep string                   `json:"implied_step"`
				Text        string                   `json:"text"`
				Action      string                   `json:"action"`
				In          []string                 `json:"in"`
				Out         []string                 `json:"out"`
				Att         string                   `json:"att"`
				Equip       []recipes.EquipmentClaim `json:"equip"`
				Hold        *recipes.Hold            `json:"hold"`
				Interrupt   *bool                    `json:"interruptible"`
				Implied     bool                     `json:"implied"`
			} `json:"nodes"`
		}
		b, _ := os.ReadFile(file)
		if err := json.Unmarshal(b, &fx); err != nil {
			t.Fatal(err)
		}
		rb, _ := os.ReadFile(filepath.Join("..", "samples", "data", fx.Sample+".json"))
		var in recipes.RecipeInput
		_ = json.Unmarshal(rb, &in)
		r, err := svc.Create(ctx, c, in)
		if err != nil {
			t.Fatal(err)
		}
		d := recipes.StructureDraft{Serve: fx.Serve}
		for _, it := range fx.Items {
			di := recipes.DraftItem{ID: it.ID, Kind: it.Kind, Label: it.Label}
			if di.Kind == "" {
				di.Kind = "ingredient"
			}
			for i, ing := range r.Ingredients {
				if it.Line != "" && strings.Contains(ing.Raw, it.Line) {
					di.Line = i + 1
				}
			}
			d.Items = append(d.Items, di)
		}
		for _, n := range fx.Nodes {
			find := n.Quote
			if n.Implied {
				find = n.ImpliedStep
			}
			step := 0
			for i, st := range r.Steps {
				if strings.Contains(st.Text, find) {
					step = i + 1
				}
			}
			q := n.Quote
			if n.Implied {
				q = ""
			}
			d.Nodes = append(d.Nodes, recipes.DraftNode{ID: n.ID, Step: step, Quote: q, Text: n.Text, Inputs: n.In, Outputs: n.Out, Implied: n.Implied})
		}
		res, err := svc.DraftRecipeGraph(ctx, c, recipes.DraftRequest{ID: r.ID, RecipeVersion: r.Version, StructureDraft: d})
		if err != nil {
			t.Fatalf("%s: %v", fx.Sample, err)
		}
		got := map[string]recipes.ActionNode{}
		for _, n := range res.Graph.Graph.Nodes {
			got[n.ID] = n
		}
		for _, h := range fx.Nodes {
			n := got[h.ID]
			score := func(field, want, have string, conf *float64) {
				tl := tallies[field]
				ok := want == have
				sure := conf != nil && *conf >= thresholdFor(field)
				if sure {
					tl.sure++
				} else {
					tl.unsure++
				}
				if ok && sure {
					tl.sureRight++
				} else if ok {
					tl.unsureRight++
				}
				if !ok && field == "action" {
					confusion[want+" → "+have]++
				}
				if !ok && verbose {
					c := 0.0
					if conf != nil {
						c = *conf
					}
					t.Logf("  %-24s %-12s %-14s want %-12s got %-12s (%.2f)  %s", fx.Sample, h.ID, field, want, have, c, cmpText(h.Quote, h.Text))
				}
			}
			score("action", h.Action, n.Action.Value, n.Action.Confidence)
			score("attention", h.Att, n.Attention.Value, n.Attention.Confidence)
			if n.Equipment != nil {
				score("equipment", mainRes(h.Equip), mainRes(n.Equipment.Value), n.Equipment.Confidence)
			}
			hasHold := func(x bool) string { return map[bool]string{true: "hold", false: "none"}[x] }
			var hc *float64
			if n.Hold != nil {
				hc = n.Hold.Confidence
			}
			score("hold", hasHold(h.Hold != nil), hasHold(n.Hold != nil), hc)
			if h.Interrupt != nil && n.Interruptible != nil {
				score("interruptible", fmt.Sprint(*h.Interrupt), fmt.Sprint(n.Interruptible.Value), n.Interruptible.Confidence)
			}
		}
	}
	for _, f := range fields {
		tl := tallies[f]
		t.Logf("%-14s all %s   sure (≥%.2f): %s   unsure: %s", f, frac(tl.sureRight+tl.unsureRight, tl.sure+tl.unsure),
			thresholdFor(f), frac(tl.sureRight, tl.sure), frac(tl.unsureRight, tl.unsure))
	}
	var keys []string
	for k := range confusion {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return confusion[keys[i]] > confusion[keys[j]] })
	for _, k := range keys {
		t.Logf("  action miss %-24s ×%d", k, confusion[k])
	}
}

func thresholdFor(f string) float64 {
	if f == "action" || f == "interruptible" {
		return 0.65
	}
	return 0.85
}

func cmpText(a, b string) string {
	if a != "" {
		return a
	}
	return "(implied) " + b
}
