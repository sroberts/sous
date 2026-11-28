package ollama_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sroberts/sous/internal/config"
	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/ollama"
	"github.com/sroberts/sous/internal/recipes"
	"github.com/sroberts/sous/internal/storage"
)

// TestLiveUpgrade runs the upgrade pipeline against real models on the
// recipes with hand-built graphs (internal/recipes/testdata/graphs) and
// reports how close it comes. It only runs with SOUS_LIVE_MODELS=1, and
// SOUS_GRAPH_MODEL / SOUS_DECISION_MODEL / SOUS_OLLAMA_URL choose what to
// run against. It reports; it fails only if the pipeline errors.
func TestLiveUpgrade(t *testing.T) {
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
	oc := ollama.New(cfg.OllamaURL, cfg.GraphModel, cfg.DecisionModel)
	svc.SetUpgrader(&recipes.Upgrader{Gen: oc, Dec: oc})
	p, err := svc.CreatePerson(ctx, identity.Caller{Via: identity.ViaAPI}, "Eval")
	if err != nil {
		t.Fatal(err)
	}
	c := identity.Caller{Person: &identity.Person{Username: p.Username}, Route: identity.RouteDirect, Via: identity.ViaAPI}

	files, _ := filepath.Glob(filepath.Join("..", "recipes", "testdata", "graphs", "*.json"))
	if only := os.Getenv("SOUS_LIVE_ONLY"); only != "" {
		files = slices.DeleteFunc(files, func(f string) bool { return !strings.Contains(f, only) })
	}
	type tally struct{ sureRight, sure, unsureRight, unsure int }
	fields := map[string]*tally{}
	for _, f := range []string{"action", "attention", "equipment", "timing_anchor", "hold"} {
		fields[f] = &tally{}
	}
	t.Logf("graph model %s, decision model %s", cfg.GraphModel, cfg.DecisionModel)
	for _, file := range files {
		var fx struct {
			Sample string `json:"sample"`
			Nodes  []struct {
				Quote  string                   `json:"quote"`
				Action string                   `json:"action"`
				Att    string                   `json:"att"`
				Equip  []recipes.EquipmentClaim `json:"equip"`
				Anchor string                   `json:"anchor"`
				Hold   *recipes.Hold            `json:"hold"`
			} `json:"nodes"`
		}
		b, _ := os.ReadFile(file)
		if err := json.Unmarshal(b, &fx); err != nil {
			t.Fatal(err)
		}
		rb, _ := os.ReadFile(filepath.Join("..", "samples", "data", fx.Sample+".json"))
		var in recipes.RecipeInput
		if err := json.Unmarshal(rb, &in); err != nil {
			t.Fatal(err)
		}
		r, err := svc.Create(ctx, c, in)
		if err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		res, err := svc.UpgradeRecipe(ctx, c, recipes.UpgradeRequest{ID: r.ID})
		if err != nil {
			t.Errorf("%s: %v", fx.Sample, err)
			continue
		}
		g := res.Graph
		codes := map[string]int{}
		for _, e := range g.Check.Errors {
			codes[e.Code]++
		}
		implied := 0
		for _, n := range g.Graph.Nodes {
			if n.Implied {
				implied++
			}
		}
		t.Logf("%-24s %5.1fs  %2d nodes (%d implied; hand-built %d)  attempts %d  unplaced %d  to review %d  errors %v",
			fx.Sample, time.Since(start).Seconds(), len(g.Graph.Nodes), implied, len(fx.Nodes), res.Attempts, res.Unplaced, g.ReviewCount, codes)

		// Compare labels on actions whose words overlap a hand-built one.
		for _, h := range fx.Nodes {
			if h.Quote == "" {
				continue
			}
			var best *recipes.ActionNode
			for i, n := range g.Graph.Nodes {
				q := n.Source.Quote
				if q != "" && (strings.Contains(h.Quote, q) || strings.Contains(q, h.Quote)) {
					best = &g.Graph.Nodes[i]
					break
				}
			}
			if best == nil {
				continue
			}
			score := func(field string, ok bool, conf *float64) {
				tl := fields[field]
				sure := conf == nil || *conf >= threshold(field)
				switch {
				case sure && ok:
					tl.sure++
					tl.sureRight++
				case sure:
					tl.sure++
				case ok:
					tl.unsure++
					tl.unsureRight++
				default:
					tl.unsure++
				}
			}
			score("action", best.Action.Value == h.Action, best.Action.Confidence)
			score("attention", best.Attention.Value == h.Att, best.Attention.Confidence)
			if best.Equipment != nil {
				score("equipment", mainRes(best.Equipment.Value) == mainRes(h.Equip), best.Equipment.Confidence)
			}
			if best.TimingAnchor != nil {
				want := h.Anchor
				if want == "" {
					want = "asap_after_inputs"
				}
				score("timing_anchor", best.TimingAnchor.Value == want, best.TimingAnchor.Confidence)
			}
			var conf *float64
			if best.Hold != nil {
				conf = best.Hold.Confidence
			}
			score("hold", (best.Hold != nil) == (h.Hold != nil), conf)
		}
	}
	for _, f := range []string{"action", "attention", "equipment", "timing_anchor", "hold"} {
		tl := fields[f]
		t.Logf("%-14s sure (≥%.2f): %s   unsure: %s", f, threshold(f), frac(tl.sureRight, tl.sure), frac(tl.unsureRight, tl.unsure))
	}
}

func threshold(f string) float64 {
	if f == "action" {
		return 0.65
	}
	return 0.85
}

func mainRes(cs []recipes.EquipmentClaim) string {
	for _, c := range cs {
		if c.Resource == "oven" || c.Resource == "burner" || strings.HasPrefix(c.Resource, "appliance:") {
			return c.Resource
		}
	}
	return "none"
}

func frac(a, b int) string {
	if b == 0 {
		return "–"
	}
	return fmt.Sprintf("%d/%d right (%.0f%%)", a, b, 100*float64(a)/float64(b))
}
