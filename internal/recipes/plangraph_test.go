package recipes

import (
	"strings"
	"testing"
	"time"

	"github.com/sroberts/sous/internal/identity"
)

// stewRecipe simmers a stew and boils rice, then serves them together: in
// steps one after the other, in a graph side by side.
func stewRecipe(t *testing.T, s *Service, c identity.Caller) Recipe {
	t.Helper()
	return create(t, s, c, RecipeInput{Title: "Stew and rice", Ingredients: []string{"1 lb stew", "1 cup rice"},
		Steps: []string{"Simmer the stew for 30 minutes.", "Boil the rice for 20 minutes.", "Serve the stew with the rice."},
		StepTiming: []*StepTiming{
			{Minutes: ref(30), HandsOn: ref(false), Equipment: ref(EquipStovetop)},
			{Minutes: ref(20), HandsOn: ref(false), Equipment: ref(EquipStovetop)},
			{Minutes: ref(2)},
		}})
}

func burner() *Label[[]EquipmentClaim] {
	return &Label[[]EquipmentClaim]{Value: []EquipmentClaim{{Resource: "burner"}}}
}

func stewGraph(simmerAttention string) GraphInput {
	one, two := 1, 2
	return GraphInput{Status: GraphNeedsReview, ServeItemID: "dinner",
		Items: []GraphItem{
			{ID: "stew", Kind: "ingredient", Label: "stew", Ingredient: &one},
			{ID: "rice", Kind: "ingredient", Label: "rice", Ingredient: &two},
			{ID: "cooked-stew", Kind: "intermediate", Label: "stew"},
			{ID: "cooked-rice", Kind: "intermediate", Label: "rice"},
			{ID: "dinner", Kind: "final", Label: "dinner"},
		},
		Nodes: []ActionNode{
			{ID: "simmer", Source: SourceSpan{Step: 1, Start: 0, End: 31}, Text: "Simmer the stew", Action: label("heat"),
				Inputs: []string{"stew"}, Outputs: []string{"cooked-stew"}, Duration: label(Duration{30, 30, 30}),
				Attention: label(simmerAttention), CheckInterval: &Label[int]{Value: 10}, Equipment: burner()},
			{ID: "boil", Source: SourceSpan{Step: 2, Start: 0, End: 29}, Text: "Boil the rice", Action: label("heat"),
				Inputs: []string{"rice"}, Outputs: []string{"cooked-rice"}, Duration: label(Duration{20, 20, 20}),
				Attention: label("passive"), Equipment: burner()},
			{ID: "serve", Source: SourceSpan{Step: 3, Start: 0, End: 29}, Text: "Serve", Action: label("plate"),
				Inputs: []string{"cooked-stew", "cooked-rice"}, Outputs: []string{"dinner"}, Duration: label(Duration{2, 2, 2}),
				Attention: label("active")},
		}}
}

func mealOf(t *testing.T, s *Service, c identity.Caller, ids ...string) Meal {
	t.Helper()
	var items []MealItemInput
	for _, id := range ids {
		items = append(items, MealItemInput{RecipeID: id})
	}
	m, err := s.CreateMeal(ctx, c, MealInput{Title: "Dinner", ServeAt: "2026-12-06T18:00:00-05:00", Timezone: "America/New_York", Items: items})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

var planNow = time.Date(2026, 12, 1, 12, 0, 0, 0, time.UTC)

func firstStart(t *testing.T, p Plan) time.Time {
	t.Helper()
	first := ""
	for _, task := range p.Tasks {
		if task.Kind != KindPreheat && (first == "" || task.Start < first) {
			first = task.Start
		}
	}
	return at(t, first)
}

// Planned from its graph, the rice boils while the stew simmers, so the
// dish starts 20 minutes later than when its steps run one after another.
func TestPlanFromGraphOverlaps(t *testing.T) {
	s := newTestService(t)
	ann := person(t, s, "Ann")
	r := stewRecipe(t, s, ann)
	m := mealOf(t, s, ann, r.ID)
	steps, err := s.MealPlan(ctx, ann, m.ID, planNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetRecipeGraph(ctx, ann, r.ID, r.Version, 0, stewGraph("intermittent")); err != nil {
		t.Fatal(err)
	}
	graph, err := s.MealPlan(ctx, ann, m.ID, planNow)
	if err != nil {
		t.Fatal(err)
	}
	serve := at(t, m.ServeAt)
	if got := serve.Sub(firstStart(t, steps)); got < 52*time.Minute {
		t.Errorf("from steps it starts %v before serving, want at least 52m", got)
	}
	if got := serve.Sub(firstStart(t, graph)); got > 35*time.Minute || got < 32*time.Minute {
		t.Errorf("from its graph it starts %v before serving, want about 32m: %s", got, dump(graph.Tasks))
	}
	keys := map[string]PlanTask{}
	for _, task := range graph.Tasks {
		keys[task.Key] = task
	}
	item := m.Items[0].ID
	simmer, boil := keys[item+":g:simmer"], keys[item+":g:boil"]
	if simmer.Key == "" || boil.Key == "" || *simmer.Step != 1 || *boil.Step != 2 || simmer.Text != "Simmer the stew for 30 minutes." {
		t.Fatalf("graph tasks: %s", dump(graph.Tasks))
	}
	if !(boil.Start < simmer.End && simmer.Start < boil.End) {
		t.Errorf("rice and stew don't overlap: %s / %s", dump(simmer), dump(boil))
	}
	if simmer.HandsOn || !keys[item+":g:serve"].HandsOn {
		t.Errorf("hands-on: simmer %v, serve %v", simmer.HandsOn, keys[item+":g:serve"].HandsOn)
	}

	// A graph action can be started and ticked off like a step.
	if _, err := s.UpdateTask(ctx, ann, m.ID, item+":g:simmer", TaskPatch{Started: Some(true)}); err != nil {
		t.Errorf("start a graph action: %v", err)
	}
	if _, err := s.UpdateTask(ctx, ann, m.ID, item+":g:no such!", TaskPatch{Started: Some(true)}); !isCode(err, CodeInvalid) && !isCode(err, CodeNotFound) {
		t.Errorf("a bad action key: %v", err)
	}
}

// A cook can keep an eye on two simmers at once, but can't do two things
// at once with their hands.
func TestPlanGraphAttentionShares(t *testing.T) {
	for _, tc := range []struct {
		attention string
		overlap   bool
	}{{"intermittent", true}, {"active", false}} {
		t.Run(tc.attention, func(t *testing.T) {
			s := newTestService(t)
			ann := person(t, s, "Ann")
			a, b := stewRecipe(t, s, ann), stewRecipe(t, s, ann)
			for _, r := range []Recipe{a, b} {
				if _, err := s.SetRecipeGraph(ctx, ann, r.ID, r.Version, 0, stewGraph(tc.attention)); err != nil {
					t.Fatal(err)
				}
			}
			m := mealOf(t, s, ann, a.ID, b.ID)
			p, err := s.MealPlan(ctx, ann, m.ID, planNow)
			if err != nil {
				t.Fatal(err)
			}
			var simmers []PlanTask
			for _, task := range p.Tasks {
				if strings.HasSuffix(task.Key, ":g:simmer") {
					simmers = append(simmers, task)
				}
			}
			if len(simmers) != 2 {
				t.Fatalf("simmers: %s", dump(p.Tasks))
			}
			overlap := simmers[0].Start < simmers[1].End && simmers[1].Start < simmers[0].End
			if overlap != tc.overlap {
				t.Errorf("%s simmers overlap = %v, want %v: %s", tc.attention, overlap, tc.overlap, dump(simmers))
			}
		})
	}
}

// The gate's pumpkin pie, planned from its graph: its own preheat gives
// way to the planner's, the bake still waits for the crust, and as the
// cooled pie keeps eight hours, the whole pie is made ahead, as it would be
// from steps with that hold on the last.
func TestPlanGraphPie(t *testing.T) {
	s := newTestService(t)
	ann := person(t, s, "Ann")
	r, in := graphFixture(t, s, ann, "pumpkin-pie.json")
	if _, err := s.SetRecipeGraph(ctx, ann, r.ID, r.Version, 0, in); err != nil {
		t.Fatal(err)
	}
	m := mealOf(t, s, ann, r.ID)
	p, err := s.MealPlan(ctx, ann, m.ID, planNow)
	if err != nil {
		t.Fatal(err)
	}
	item := m.Items[0].ID
	keys := map[string]PlanTask{}
	preheats := 0
	for _, task := range p.Tasks {
		keys[task.Key] = task
		if task.Kind == KindPreheat {
			preheats++
		}
	}
	if _, ok := keys[item+":g:preheat"]; ok {
		t.Error("the graph's own preheat was planned")
	}
	if preheats == 0 {
		t.Error("no preheat planned")
	}
	if keys[item+":g:bake-hot"].Start < keys[item+":g:lattice"].End || keys[item+":g:lattice"].Start < keys[item+":g:crimp"].End {
		t.Errorf("out of order: %s", dump(p.Tasks))
	}
	if !keys[item+":g:chill-dough"].MakeAhead || !keys[item+":g:mix-dry"].MakeAhead || !keys[item+":g:mix-filling"].MakeAhead {
		t.Errorf("make-ahead: chill %v, mix-dry %v, filling %v", keys[item+":g:chill-dough"].MakeAhead,
			keys[item+":g:mix-dry"].MakeAhead, keys[item+":g:mix-filling"].MakeAhead)
	}
	for _, w := range p.Warnings {
		if w.Code == "conflict" {
			t.Errorf("conflict: %s", w.Message)
		}
	}
}

// A meal planning some dishes from graphs says which aren't, and why; a
// draft, or a graph whose words have changed, isn't used.
func TestPlanGraphMixed(t *testing.T) {
	s := newTestService(t)
	ann := person(t, s, "Ann")
	a, b, c := stewRecipe(t, s, ann), stewRecipe(t, s, ann), tinyRecipe(t, s, ann)
	if _, err := s.SetRecipeGraph(ctx, ann, a.ID, a.Version, 0, stewGraph("passive")); err != nil {
		t.Fatal(err)
	}
	draft := stewGraph("passive")
	draft.Status = GraphDraft
	if _, err := s.SetRecipeGraph(ctx, ann, b.ID, b.Version, 0, draft); err != nil {
		t.Fatal(err)
	}
	m := mealOf(t, s, ann, a.ID, b.ID, c.ID)
	p, err := s.MealPlan(ctx, ann, m.ID, planNow)
	if err != nil {
		t.Fatal(err)
	}
	var msgs []string
	for _, w := range p.Warnings {
		if w.Code == "no_graph" {
			msgs = append(msgs, w.Message)
		}
	}
	if len(msgs) != 2 || !strings.Contains(msgs[0]+msgs[1], "still a draft") || !strings.Contains(msgs[0]+msgs[1], "has no step graph") {
		t.Errorf("warnings: %q", msgs)
	}
	graphTasks := 0
	for _, task := range p.Tasks {
		if strings.Contains(task.Key, ":g:") {
			graphTasks++
			if *task.ItemID != m.Items[0].ID {
				t.Errorf("a graph task for the wrong dish: %s", dump(task))
			}
		}
	}
	if graphTasks != 3 {
		t.Errorf("%d graph tasks, want 3", graphTasks)
	}

	// Its words change: back to its steps.
	a, _ = s.Get(ctx, ann, a.ID)
	if _, err := s.Update(ctx, ann, a.ID, a.Version, RecipePatch{Steps: Some([]string{"Braise the stew for 30 minutes.", "Boil the rice for 20 minutes.", "Serve the stew with the rice."})}); err != nil {
		t.Fatal(err)
	}
	p, err = s.MealPlan(ctx, ann, m.ID, planNow)
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range p.Tasks {
		if strings.Contains(task.Key, ":g:") {
			t.Errorf("a stale graph was used: %s", dump(task))
		}
	}
	// A graph that can't be used is named even when no dish plans from a
	// graph, so the stew isn't quietly planned from its steps; dishes with
	// no graph at all aren't, as nothing in the meal uses graphs.
	var named []string
	for _, w := range p.Warnings {
		if w.Code == "no_graph" {
			named = append(named, w.Message)
		}
	}
	if len(named) != 2 || !strings.Contains(strings.Join(named, " "), "words have changed") || !strings.Contains(strings.Join(named, " "), "still a draft") {
		t.Errorf("no_graph warnings: %q", named)
	}
}

// The curry's simmered base keeps two days: it and everything before it
// are made ahead, it's reheated on the day, and the rice, cream and lemon
// come after the reheat. Planned live and late, it still keeps its order.
func TestPlanGraphCurryAheadAndLive(t *testing.T) {
	s := newTestService(t)
	ann := person(t, s, "Ann")
	r, in := graphFixture(t, s, ann, "chicken-curry.json")
	if _, err := s.SetRecipeGraph(ctx, ann, r.ID, r.Version, 0, in); err != nil {
		t.Fatal(err)
	}
	m := mealOf(t, s, ann, r.ID)
	item := m.Items[0].ID
	check := func(p Plan) map[string]PlanTask {
		t.Helper()
		keys := map[string]PlanTask{}
		for _, task := range p.Tasks {
			keys[task.Key] = task
		}
		before := func(a, b string) {
			t.Helper()
			if keys[item+a].End > keys[item+b].Start {
				t.Errorf("%s ends %s, after %s starts %s", a, keys[item+a].End, b, keys[item+b].Start)
			}
		}
		before(":g:simmer", ":reheat")
		before(":reheat", ":g:cream")
		before(":g:cream", ":g:finish")
		before(":g:juice", ":g:finish")
		before(":g:boil-rice", ":g:serve")
		before(":g:finish", ":g:serve")
		return keys
	}
	p, err := s.MealPlan(ctx, ann, m.ID, planNow)
	if err != nil {
		t.Fatal(err)
	}
	keys := check(p)
	if !keys[item+":g:simmer"].MakeAhead || !keys[item+":g:melt"].MakeAhead || keys[item+":g:boil-rice"].MakeAhead || keys[item+":reheat"].Key == "" {
		t.Errorf("make-ahead split: %s", dump(p.Tasks))
	}
	// The day itself, with nothing done and too little time: replanned
	// from now, still in order.
	late := at(t, m.ServeAt).Add(-40 * time.Minute)
	p, err = s.MealPlan(ctx, ann, m.ID, late)
	if err != nil {
		t.Fatal(err)
	}
	check(p)
	if p.OnTime {
		t.Errorf("planned 40 minutes out with a day's make-ahead undone, yet on time")
	}
}
