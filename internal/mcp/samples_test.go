package mcp

import (
	"log/slog"
	"testing"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/recipes"
	"github.com/sroberts/sous/internal/samples"
	"github.com/sroberts/sous/internal/storage"
)

// TestSamplesThroughMCP: the samples are searchable over MCP, and the
// internal caller header can't name Sous, even on a bare handler.
func TestSamplesThroughMCP(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	all, err := samples.All()
	if err != nil {
		t.Fatal(err)
	}
	// Not newService: adding Alice and Bob logs changes, and a used library
	// is never seeded.
	st, err := storage.Open(ctx, t.TempDir(), log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	svc := recipes.New(st, log)
	if n, err := svc.SeedSamples(ctx, all); err != nil || n != len(all) {
		t.Fatalf("SeedSamples = %d, %v", n, err)
	}
	h := New(svc, log, "test")

	cs := as(t, serve(t, standIn(h)), "")
	page := ok[recipes.Page[recipes.Summary]](t, cs, "search_recipes", map[string]any{"tag": []string{"Historic"}})
	if page.Total != len(all) {
		t.Fatalf("Historic recipes = %d, want %d", page.Total, len(all))
	}
	changes := ok[recipes.ChangePage](t, cs, "recent_changes", map[string]any{"via": "system"})
	if len(changes.Items) != len(all) || *changes.Items[0].PersonID != identity.SystemUsername {
		t.Errorf("system changes = %+v", changes)
	}

	bare := connect(t, serve(t, h), map[string]string{HeaderCaller: identity.SystemUsername})
	fails(t, bare, "create_recipe", map[string]any{"title": "Forged"}, identity.CodeUnknownPerson)
	fails(t, as(t, serve(t, standIn(h)), identity.SystemUsername), "create_recipe",
		map[string]any{"title": "Forged"}, identity.CodeUnknownPerson)
}
