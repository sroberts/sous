package recipes

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sroberts/sous/internal/identity"
)

// A fixture is a hand-built step graph for one of the sample recipes,
// written compactly: a node names the words it comes from by quoting them
// (or, for an implied node, some words of the step it serves), and an item
// names its ingredient line by some words of it. graphFixture turns it into
// the GraphInput a client would send, with spans counted in characters.
type fixture struct {
	Sample string        `json:"sample"`
	Serve  string        `json:"serve"`
	Items  []fixtureItem `json:"items"`
	Nodes  []fixtureNode `json:"nodes"`
}

type fixtureItem struct {
	ID       string  `json:"id"`
	Kind     string  `json:"kind"`
	Label    string  `json:"label"`
	Line     string  `json:"line"`
	Quantity *string `json:"quantity"`
}

type fixtureNode struct {
	ID            string           `json:"id"`
	Quote         string           `json:"quote"`
	ImpliedStep   string           `json:"implied_step"`
	Text          string           `json:"text"`
	Action        string           `json:"action"`
	Technique     *string          `json:"technique"`
	In            []string         `json:"in"`
	Out           []string         `json:"out"`
	Dur           [3]int           `json:"dur"`
	Att           string           `json:"att"`
	Check         int              `json:"check"`
	Equip         []EquipmentClaim `json:"equip"`
	Hold          *Hold            `json:"hold"`
	HoldSource    []any            `json:"hold_source"`
	Anchor        string           `json:"anchor"`
	Interruptible *bool            `json:"interruptible"`
	Implied       bool             `json:"implied"`
}

func label[T any](v T) Label[T] { return Label[T]{Value: v} }

// sampleRecipe saves one of the sample recipes, read from its file.
func sampleRecipe(t *testing.T, s *Service, c identity.Caller, name string) Recipe {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "samples", "data", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var in RecipeInput
	if err := json.Unmarshal(b, &in); err != nil {
		t.Fatal(err)
	}
	return create(t, s, c, in)
}

// graphFixture saves the fixture's sample recipe and returns it with the
// fixture as a GraphInput.
func graphFixture(t *testing.T, s *Service, c identity.Caller, file string) (Recipe, GraphInput) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "graphs", file))
	if err != nil {
		t.Fatal(err)
	}
	var f fixture
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	r := sampleRecipe(t, s, c, f.Sample)
	g := GraphInput{Status: GraphVerified, ServeItemID: f.Serve}
	for _, fi := range f.Items {
		it := GraphItem{ID: fi.ID, Kind: fi.Kind, Label: fi.Label, Quantity: fi.Quantity}
		if it.Kind == "" {
			it.Kind = "ingredient"
		}
		if fi.Line != "" {
			n := 0
			for i, ing := range r.Ingredients {
				if strings.Contains(ing.Raw, fi.Line) {
					line := i + 1
					it.Ingredient = &line
					n++
				}
			}
			if n != 1 {
				t.Fatalf("%s: item %s: %q matches %d ingredient lines", file, fi.ID, fi.Line, n)
			}
		}
		g.Items = append(g.Items, it)
	}
	for _, fn := range f.Nodes {
		n := ActionNode{ID: fn.ID, Text: fn.Text, Technique: fn.Technique, Inputs: fn.In, Outputs: fn.Out, Implied: fn.Implied,
			Action:    label(fn.Action),
			Duration:  label(Duration{fn.Dur[0], fn.Dur[1], fn.Dur[2]}),
			Attention: label(fn.Att)}
		find := fn.Quote
		if fn.Implied {
			find = fn.ImpliedStep
		}
		hits := 0
		for i, st := range r.Steps {
			if at := strings.Index(st.Text, find); at >= 0 {
				hits++
				start := len([]rune(st.Text[:at]))
				n.Source = SourceSpan{Step: i + 1, Start: start, End: start + len([]rune(find))}
				if fn.Implied {
					n.Source.End = start
				}
			}
		}
		if hits != 1 {
			t.Fatalf("%s: node %s: %q is in %d steps", file, fn.ID, find, hits)
		}
		if fn.Check > 0 {
			n.CheckInterval = &Label[int]{Value: fn.Check}
		}
		if fn.Equip != nil {
			n.Equipment = &Label[[]EquipmentClaim]{Value: fn.Equip}
		}
		if fn.Hold != nil {
			n.Hold = &Label[Hold]{Value: *fn.Hold}
			if fn.HoldSource != nil {
				conf := fn.HoldSource[1].(float64)
				n.Hold.Source, n.Hold.Confidence = fn.HoldSource[0].(string), &conf
			}
		}
		if fn.Anchor != "" {
			n.TimingAnchor = &Label[string]{Value: fn.Anchor}
		}
		if fn.Interruptible != nil {
			n.Interruptible = &Label[bool]{Value: *fn.Interruptible}
		}
		g.Nodes = append(g.Nodes, n)
	}
	return r, g
}

// The phase 1 gate: five hand-built graphs of the sample recipes, covering
// a long rise, intermittent simmering, make-ahead holds, hidden
// intermediates (reserved cooking liquid) and an oven that changes
// temperature mid-bake, all pass the checks, save as verified and read back
// as they went in.
func TestStepGraphGate(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "graphs", "*.json"))
	if err != nil || len(files) < 5 {
		t.Fatalf("want 5 fixtures, got %v %v", files, err)
	}
	for _, path := range files {
		file := filepath.Base(path)
		t.Run(strings.TrimSuffix(file, ".json"), func(t *testing.T) {
			s := newTestService(t)
			ann := person(t, s, "Ann")
			r, in := graphFixture(t, s, ann, file)

			chk, err := s.CheckRecipeGraph(ctx, ann, r.ID, &in)
			if err != nil {
				t.Fatal(err)
			}
			if !chk.Valid {
				t.Fatalf("errors: %s", dump(chk.Errors))
			}
			for _, w := range chk.Warnings {
				// The sample kitchen lists no pans or containers; anything
				// else is a gap in the graph.
				if w.Code != "not_in_kitchen" {
					t.Errorf("warning: %s", dump(w))
				}
			}

			v, err := s.SetRecipeGraph(ctx, ann, r.ID, r.Version, 0, in)
			if err != nil {
				t.Fatal(err)
			}
			if v.Graph.Status != GraphVerified || v.Graph.Version != 1 || !v.Check.Valid {
				t.Fatalf("saved: status %s, version %d, valid %v", v.Graph.Status, v.Graph.Version, v.Check.Valid)
			}
			back, err := s.RecipeGraph(ctx, ann, r.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(back.Graph.Nodes) != len(in.Nodes) || len(back.Graph.Items) != len(in.Items) {
				t.Fatalf("read back %d nodes and %d items, saved %d and %d", len(back.Graph.Nodes), len(back.Graph.Items), len(in.Nodes), len(in.Items))
			}
			for i, n := range back.Graph.Nodes {
				want := in.Nodes[i]
				if n.ID != want.ID || n.Source != want.Source || !slices.Equal(n.Inputs, want.Inputs) || n.Duration.Value != want.Duration.Value {
					t.Errorf("node %d: got %s, want %s", i, dump(n), dump(want))
				}
				for _, l := range labelsOf(&n) {
					if *l.by == nil || **l.by != ann.Person.Username {
						t.Errorf("%s.%s not reviewed by ann: %v", n.ID, l.field, *l.by)
					}
				}
				if !n.Implied && n.Source.Quote == "" {
					t.Errorf("%s: no quote", n.ID)
				}
			}
			if len(back.Projection) != len(r.Steps) {
				t.Fatalf("projection has %d entries for %d steps", len(back.Projection), len(r.Steps))
			}
		})
	}
}

// The pie's projection: the preheat step comes to nothing (planners add
// their own), and the bake step sums its two bakes, keeps the oven at the
// temperature it holds longest, and needs no hands.
func TestStepGraphProjection(t *testing.T) {
	s := newTestService(t)
	ann := person(t, s, "Ann")
	r, in := graphFixture(t, s, ann, "pumpkin-pie.json")
	v, err := s.SetRecipeGraph(ctx, ann, r.ID, r.Version, 0, in)
	if err != nil {
		t.Fatal(err)
	}
	step := func(prefix string) int {
		for i, st := range r.Steps {
			if strings.HasPrefix(st.Text, prefix) {
				return i
			}
		}
		t.Fatalf("no step %q", prefix)
		return 0
	}
	if p := v.Projection[step("Heat the oven")]; p != nil {
		t.Errorf("preheat step projects to %s, want null", dump(p))
	}
	bake := v.Projection[step("Bake for 15")]
	if bake == nil || *bake.Minutes != 60 || *bake.Equipment != EquipOven || *bake.OvenTemp != 350 || *bake.HandsOn {
		t.Errorf("bake step projects to %s, want 60 minutes hands-off in the oven at 350", dump(bake))
	}
	crust := v.Projection[step("Whisk the flour")]
	// 1 + 5 + 3 + 1 active, 60 chilling: hands-off, with the chill's hold.
	if crust == nil || *crust.Minutes != 70 || *crust.HandsOn || crust.HoldMinutes == nil || *crust.HoldMinutes != 2880 || *crust.HoldIn != HoldFridge {
		t.Errorf("crust step projects to %s", dump(crust))
	}
	fill := v.Projection[step("Whisk the pumpkin")]
	if fill == nil || *fill.Minutes != 5 || !*fill.HandsOn || fill.Equipment != nil {
		t.Errorf("filling step projects to %s", dump(fill))
	}
}

// tinyRecipe is a two-step recipe for checking one rule at a time.
func tinyRecipe(t *testing.T, s *Service, c identity.Caller) Recipe {
	t.Helper()
	return create(t, s, c, RecipeInput{Title: "Toast", Ingredients: []string{"2 slices bread", "1 tbsp butter"},
		Steps: []string{"Heat the oven to 400°F.", "Toast the bread for 5 minutes, then butter it."}})
}

// tinyGraph is a valid graph for tinyRecipe.
func tinyGraph() GraphInput {
	one, two := 1, 2
	oven := func(temp int) *Label[[]EquipmentClaim] {
		return &Label[[]EquipmentClaim]{Value: []EquipmentClaim{{Resource: "oven", TempF: &temp}}}
	}
	return GraphInput{Status: GraphDraft, ServeItemID: "toast",
		Items: []GraphItem{
			{ID: "bread", Kind: "ingredient", Label: "bread", Ingredient: &one},
			{ID: "butter", Kind: "ingredient", Label: "butter", Ingredient: &two},
			{ID: "hot-oven", Kind: "intermediate", Label: "hot oven"},
			{ID: "toasted", Kind: "intermediate", Label: "toasted bread"},
			{ID: "toast", Kind: "final", Label: "buttered toast"},
		},
		Nodes: []ActionNode{
			{ID: "preheat", Source: SourceSpan{Step: 1, Start: 0, End: 23}, Text: "Heat the oven", Action: label("setup"),
				Outputs: []string{"hot-oven"}, Duration: label(Duration{10, 15, 15}), Attention: label("passive"), Equipment: oven(400)},
			{ID: "toast", Source: SourceSpan{Step: 2, Start: 0, End: 31}, Text: "Toast the bread", Action: label("heat"),
				Inputs: []string{"bread", "hot-oven"}, Outputs: []string{"toasted"}, Duration: label(Duration{4, 5, 6}),
				Attention: label("passive"), Equipment: oven(400)},
			{ID: "butter", Source: SourceSpan{Step: 2, Start: 33, End: 46}, Text: "Butter it", Action: label("assemble"),
				Inputs: []string{"toasted", "butter"}, Outputs: []string{"toast"}, Duration: label(Duration{1, 1, 1}), Attention: label("active")},
		}}
}

func codes(ps []GraphProblem) []string {
	out := []string{}
	for _, p := range ps {
		out = append(out, p.Code)
	}
	return out
}

func TestStepGraphChecks(t *testing.T) {
	s := newTestService(t)
	ann := person(t, s, "Ann")
	r := tinyRecipe(t, s, ann)
	chk, err := s.CheckRecipeGraph(ctx, ann, r.ID, ptrTo(tinyGraph()))
	if err != nil || !chk.Valid || len(chk.Warnings) != 0 {
		t.Fatalf("tiny graph: %v, %s", err, dump(chk))
	}
	cases := []struct {
		name  string
		edit  func(g *GraphInput)
		want  string
		error bool
	}{
		{"dangling input", func(g *GraphInput) { g.Nodes[2].Inputs = []string{"toasted", "butter", "jam"} }, "unknown_item", true},
		{"nothing makes it", func(g *GraphInput) { g.Nodes[1].Outputs = nil }, "dangling_input", true},
		{"made twice", func(g *GraphInput) { g.Nodes[2].Outputs = []string{"toast", "toasted"} }, "made_twice", true},
		{"makes an ingredient", func(g *GraphInput) { g.Nodes[1].Outputs = []string{"toasted", "butter"} }, "makes_ingredient", true},
		{"cycle", func(g *GraphInput) {
			g.Nodes[1].Inputs = append(g.Nodes[1].Inputs, "toast")
		}, "cycle", true},
		{"unused output", func(g *GraphInput) {
			g.Items = append(g.Items, GraphItem{ID: "crumbs", Kind: "intermediate", Label: "crumbs"})
			g.Nodes[1].Outputs = append(g.Nodes[1].Outputs, "crumbs")
		}, "unused_output", true},
		{"no preheat", func(g *GraphInput) { g.Nodes[1].Inputs = []string{"bread"}; g.Nodes[0].Outputs = nil }, "no_preheat", true},
		{"preheat at another temperature", func(g *GraphInput) {
			temp := 300
			g.Nodes[0].Equipment.Value[0].TempF = &temp
		}, "no_preheat", true},
		{"intermittent without a check interval", func(g *GraphInput) { g.Nodes[1].Attention = label("intermittent") }, "no_check_interval", true},
		{"served item isn't final", func(g *GraphInput) { g.ServeItemID = "toasted" }, "serve_not_final", true},
		{"unknown appliance", func(g *GraphInput) {
			g.Nodes[2].Equipment = &Label[[]EquipmentClaim]{Value: []EquipmentClaim{{Resource: "appliance:toaster"}}}
		}, "not_in_kitchen", false},
		{"step without nodes", func(g *GraphInput) {
			g.Nodes[1].Source = SourceSpan{Step: 1, Start: 0, End: 4}
			g.Nodes[2].Source = SourceSpan{Step: 1, Start: 5, End: 8}
		}, "step_uncovered", false},
		{"unused ingredient line", func(g *GraphInput) { g.Items[1].Ingredient = nil }, "unused_ingredient", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := tinyGraph()
			tc.edit(&g)
			chk, err := s.CheckRecipeGraph(ctx, ann, r.ID, &g)
			if err != nil {
				t.Fatal(err)
			}
			got := codes(chk.Warnings)
			if tc.error {
				got = codes(chk.Errors)
			}
			if !slices.Contains(got, tc.want) || chk.Valid == tc.error {
				t.Errorf("want %s (error %v), got valid %v, %s", tc.want, tc.error, chk.Valid, dump(chk))
			}
		})
	}
}

// What can't be stored at all is refused outright, before the checks.
func TestStepGraphRefused(t *testing.T) {
	s := newTestService(t)
	ann := person(t, s, "Ann")
	r := tinyRecipe(t, s, ann)
	cases := []struct {
		name string
		edit func(g *GraphInput)
		want string
	}{
		{"span past the step", func(g *GraphInput) { g.Nodes[0].Source.End = 99 }, "outside step"},
		{"empty span on a stated node", func(g *GraphInput) { g.Nodes[0].Source.End = 0 }, "only an implied node"},
		{"no such step", func(g *GraphInput) { g.Nodes[0].Source.Step = 3 }, "isn't one of"},
		{"bad action", func(g *GraphInput) { g.Nodes[0].Action = label("bake") }, "action must be"},
		{"model label without confidence", func(g *GraphInput) { g.Nodes[0].Attention.Source = "model" }, "needs a confidence"},
		{"confidence over 1", func(g *GraphInput) { c := 1.5; g.Nodes[0].Attention.Confidence = &c }, "0 to 1"},
		{"duration out of order", func(g *GraphInput) { g.Nodes[0].Duration = label(Duration{10, 5, 20}) }, "duration needs"},
		{"duplicate id", func(g *GraphInput) { g.Nodes[1].ID = "preheat" }, "used twice"},
		{"bad resource", func(g *GraphInput) {
			g.Nodes[2].Equipment = &Label[[]EquipmentClaim]{Value: []EquipmentClaim{{Resource: "toaster"}}}
		}, "resource"},
		{"temperature off the oven", func(g *GraphInput) {
			temp := 400
			g.Nodes[2].Equipment = &Label[[]EquipmentClaim]{Value: []EquipmentClaim{{Resource: "burner", TempF: &temp}}}
		}, "only the oven"},
		{"ingredient line out of range", func(g *GraphInput) { n := 3; g.Items[0].Ingredient = &n }, "isn't a line"},
		{"bad status", func(g *GraphInput) { g.Status = "stale" }, "status"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := tinyGraph()
			tc.edit(&g)
			_, err := s.SetRecipeGraph(ctx, ann, r.ID, r.Version, 0, g)
			if !isCode(err, CodeInvalid) || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("got %v, want invalid mentioning %q", err, tc.want)
			}
		})
	}
	// A verified graph has to pass the checks; a draft needn't.
	g := tinyGraph()
	g.Nodes[1].Attention = label("intermittent")
	g.Status = GraphVerified
	if _, err := s.SetRecipeGraph(ctx, ann, r.ID, r.Version, 0, g); !isCode(err, CodeInvalid) || !strings.Contains(err.Error(), "check") {
		t.Errorf("verified with errors: %v", err)
	}
	g.Status = GraphDraft
	v, err := s.SetRecipeGraph(ctx, ann, r.ID, r.Version, 0, g)
	if err != nil || v.Check.Valid {
		t.Errorf("draft with errors: %v, valid %v", err, v.Check.Valid)
	}
}

// Editing a step's words marks the nodes from it stale, and only those.
func TestStepGraphStale(t *testing.T) {
	s := newTestService(t)
	ann := person(t, s, "Ann")
	r := tinyRecipe(t, s, ann)
	if _, err := s.SetRecipeGraph(ctx, ann, r.ID, r.Version, 0, tinyGraph()); err != nil {
		t.Fatal(err)
	}
	r, err := s.Update(ctx, ann, r.ID, r.Version, RecipePatch{Title: Some("Hot toast")})
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.RecipeGraph(ctx, ann, r.ID)
	if err != nil || v.Graph.Status != GraphDraft {
		t.Fatalf("after a title edit: %v, status %s", err, v.Graph.Status)
	}
	r, err = s.Update(ctx, ann, r.ID, r.Version, RecipePatch{Steps: Some([]string{"Heat the oven to 400°F.", "Grill the bread for 5 minutes, then butter it."})})
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.RecipeGraph(ctx, ann, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Graph.Status != GraphStale {
		t.Errorf("status %s, want stale", v.Graph.Status)
	}
	stale := map[string]bool{}
	for _, n := range v.Graph.Nodes {
		stale[n.ID] = n.Stale
	}
	if stale["preheat"] || !stale["toast"] || stale["butter"] {
		t.Errorf("stale nodes = %v, want only toast", stale)
	}
	if !slices.Contains(codes(v.Check.Warnings), "stale") {
		t.Errorf("no stale warning: %s", dump(v.Check))
	}
}

func TestStepGraphVersions(t *testing.T) {
	s := newTestService(t)
	ann := person(t, s, "Ann")
	r := tinyRecipe(t, s, ann)
	if _, err := s.RecipeGraph(ctx, ann, r.ID); !isCode(err, CodeNotFound) {
		t.Fatalf("no graph yet: %v", err)
	}
	if _, err := s.SetRecipeGraph(ctx, identity.Caller{}, r.ID, r.Version, 0, tinyGraph()); !isCode(err, CodeWhoAreYou) {
		t.Errorf("nobody: %v", err)
	}
	if _, err := s.SetRecipeGraph(ctx, ann, r.ID, r.Version, 3, tinyGraph()); !isCode(err, CodeInvalid) {
		t.Errorf("version 3 with no graph: %v", err)
	}
	v, err := s.SetRecipeGraph(ctx, ann, r.ID, r.Version, 0, tinyGraph())
	if err != nil {
		t.Fatal(err)
	}
	// A second save from a stale read gets the graph as it stands.
	_, err = s.SetRecipeGraph(ctx, ann, r.ID, r.Version, 0, tinyGraph())
	var e *Error
	if !isCode(err, CodeStale) || !asError(err, &e) || e.CurrentGraph == nil || e.CurrentGraph.Graph.Version != v.Graph.Version {
		t.Errorf("stale graph version: %v", err)
	}
	// So does one against an old version of the recipe, with the recipe.
	r2, err := s.Update(ctx, ann, r.ID, r.Version, RecipePatch{Title: Some("Hot toast")})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SetRecipeGraph(ctx, ann, r.ID, r.Version, v.Graph.Version, tinyGraph())
	if !isCode(err, CodeStale) || !asError(err, &e) || e.Current == nil || e.Current.Version != r2.Version {
		t.Errorf("stale recipe version: %v", err)
	}
	// Saving against the new version makes version 2, and the graph moves on.
	v2, err := s.SetRecipeGraph(ctx, ann, r.ID, r2.Version, v.Graph.Version, tinyGraph())
	if err != nil || v2.Graph.Version != 2 || v2.Graph.RecipeVersion != r2.Version {
		t.Fatalf("resave: %v, %d", err, v2.Graph.Version)
	}

	if err := s.DeleteRecipeGraph(ctx, ann, r.ID, 1); !isCode(err, CodeStale) {
		t.Errorf("delete stale: %v", err)
	}
	if err := s.DeleteRecipeGraph(ctx, ann, r.ID, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecipeGraph(ctx, ann, r.ID); !isCode(err, CodeNotFound) {
		t.Errorf("after delete: %v", err)
	}
	if err := s.DeleteRecipeGraph(ctx, ann, r.ID, 2); !isCode(err, CodeNotFound) {
		t.Errorf("delete again: %v", err)
	}
	// A new graph after a delete takes the next version.
	v3, err := s.SetRecipeGraph(ctx, ann, r.ID, r2.Version, 0, tinyGraph())
	if err != nil || v3.Graph.Version != 3 {
		t.Errorf("after delete: %v, version %d", err, v3.Graph.Version)
	}

	page, err := s.Changes(ctx, ann, ChangeQuery{EntityID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	var graphChanges []string
	for _, ch := range page.Items {
		if strings.Contains(ch.Summary, "step graph") {
			graphChanges = append(graphChanges, ch.Summary)
		}
	}
	if len(graphChanges) != 4 {
		t.Errorf("graph changes logged: %v", graphChanges)
	}

	// The trash: a deleted recipe's graph can be read but not changed.
	r3, err := s.Get(ctx, ann, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, ann, r.ID, r3.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetRecipeGraph(ctx, ann, r.ID, r3.Version, 3, tinyGraph()); !isCode(err, CodeDeleted) {
		t.Errorf("trashed recipe: %v", err)
	}
	if _, err := s.RecipeGraph(ctx, ann, r.ID); err != nil {
		t.Errorf("reading a trashed recipe's graph: %v", err)
	}
}

// A verified save marks every label reviewed by the saver. Labels keep
// their review through a later save while their value doesn't change; a
// changed one loses it until the graph is verified again.
func TestStepGraphReviews(t *testing.T) {
	s := newTestService(t)
	ann, bob := person(t, s, "Ann"), person(t, s, "Bob")
	r := tinyRecipe(t, s, ann)
	g := tinyGraph()
	conf := 0.6
	g.Nodes[1].Attention = Label[string]{Value: "passive", Source: "model", Confidence: &conf}
	by := "mallory"
	g.Nodes[1].Action.ReviewedBy = &by // ignored: Sous sets reviews
	v, err := s.SetRecipeGraph(ctx, ann, r.ID, r.Version, 0, g)
	if err != nil {
		t.Fatal(err)
	}
	if n := v.Graph.Nodes[1]; n.Action.ReviewedBy != nil || n.Attention.Source != "model" || *n.Attention.Confidence != 0.6 || *n.Action.Confidence != 1 {
		t.Fatalf("draft labels: %s", dump(n))
	}

	g.Status = GraphVerified
	v, err = s.SetRecipeGraph(ctx, ann, r.ID, r.Version, v.Graph.Version, g)
	if err != nil {
		t.Fatal(err)
	}
	if n := v.Graph.Nodes[1]; n.Attention.ReviewedBy == nil || *n.Attention.ReviewedBy != "ann" || n.Attention.ReviewedAt == nil {
		t.Fatalf("verified labels: %s", dump(n))
	}

	// Bob changes the toasting time as a draft: that label loses its
	// review; the rest keep Ann's.
	g.Status = GraphNeedsReview
	g.Nodes[1].Duration = label(Duration{6, 8, 10})
	v, err = s.SetRecipeGraph(ctx, bob, r.ID, r.Version, v.Graph.Version, g)
	if err != nil {
		t.Fatal(err)
	}
	n := v.Graph.Nodes[1]
	if n.Duration.ReviewedBy != nil || n.Attention.ReviewedBy == nil || *n.Attention.ReviewedBy != "ann" {
		t.Errorf("after bob's draft: %s", dump(n))
	}
	if v.Graph.CreatedBy != "ann" || v.Graph.UpdatedBy != "bob" {
		t.Errorf("created by %s, updated by %s", v.Graph.CreatedBy, v.Graph.UpdatedBy)
	}
}

// Pans and containers and counter space are kept when an update leaves
// them out, so a client that predates them doesn't wipe them.
func TestKitchenVesselsKept(t *testing.T) {
	s := newTestService(t)
	ann := person(t, s, "Ann")
	k, err := s.GetKitchen(ctx, ann)
	if err != nil {
		t.Fatal(err)
	}
	if k.Vessels == nil || k.CounterZones == nil || *k.CounterZones != 0 {
		t.Fatalf("defaults: %s", dump(k))
	}
	p := k.KitchenProfile
	two := 2
	p.Vessels = []Vessel{{Name: "  Dutch   Oven ", Count: 1}}
	p.CounterZones = &two
	k, err = s.UpdateKitchen(ctx, ann, k.Version, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(k.Vessels) != 1 || k.Vessels[0].Name != "dutch oven" || *k.CounterZones != 2 {
		t.Fatalf("saved: %s", dump(k))
	}
	old := k.KitchenProfile
	old.Vessels, old.CounterZones = nil, nil
	old.Burners = 6
	k, err = s.UpdateKitchen(ctx, ann, k.Version, old)
	if err != nil {
		t.Fatal(err)
	}
	if len(k.Vessels) != 1 || *k.CounterZones != 2 || k.Burners != 6 {
		t.Errorf("left out: %s", dump(k))
	}
	p = k.KitchenProfile
	p.Vessels = []Vessel{}
	k, err = s.UpdateKitchen(ctx, ann, k.Version, p)
	if err != nil || len(k.Vessels) != 0 {
		t.Errorf("emptied: %v %s", err, dump(k))
	}
	for _, bad := range []KitchenProfile{
		{Vessels: []Vessel{{Name: "pan", Count: 0}}},
		{Vessels: []Vessel{{Name: "pan", Count: 1}, {Name: "Pan", Count: 2}}},
		{CounterZones: ptrTo(21)},
	} {
		if _, err := s.UpdateKitchen(ctx, ann, k.Version, bad); !isCode(err, CodeInvalid) {
			t.Errorf("%s: %v", dump(bad), err)
		}
	}
}

func ptrTo[T any](v T) *T { return &v }

func asError(err error, e **Error) bool { return errors.As(err, e) }
