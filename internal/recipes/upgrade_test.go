package recipes

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sroberts/sous/internal/identity"
)

// fakeGen returns drafts in turn (the last one repeatedly) and keeps the
// prompts it was given.
type fakeGen struct {
	mu      sync.Mutex
	drafts  []StructureDraft
	prompts []string
	err     error
	during  func() // runs inside Generate, to change things mid-upgrade
}

func (f *fakeGen) Generate(ctx context.Context, prompt string, schema any) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.during != nil {
		f.during()
	}
	if f.err != nil {
		return nil, f.err
	}
	f.prompts = append(f.prompts, prompt)
	d := f.drafts[min(len(f.prompts)-1, len(f.drafts)-1)]
	return json.Marshal(d)
}

// fakeDec answers from a table keyed by a word in the action's words:
// answers[word][question] is the top label and its probability; anything
// unlisted gets defaults, sure of themselves.
type fakeDec struct {
	answers map[string]map[string]ranked
	err     error
	calls   int
	states  []string
}

var defaultAnswers = map[string]ranked{
	"action": {"prep", 0.9}, "attention": {"active", 0.95}, "check": {"5", 0.9},
	"anchor": {"asap_after_inputs", 0.9}, "hold": {"none", 0.95}, "where": {"counter", 0.9},
	"duration": {"5", 0.9}, "equipment": {"none", 0.95},
}

func (f *fakeDec) Decide(ctx context.Context, state string, qs []Question) (map[string]Answer, error) {
	f.calls++
	f.states = append(f.states, state)
	if f.err != nil {
		return nil, f.err
	}
	action := state // the action question reads only the action's words
	if _, after, ok := strings.Cut(state, "Action: "); ok {
		action = after
	}
	out := map[string]Answer{}
	for _, q := range qs {
		top := defaultAnswers[q.ID]
		for word, as := range f.answers {
			if a, ok := as[q.ID]; ok && strings.Contains(action, word) {
				top = a
			}
		}
		probs := map[string]float64{top.label: top.p}
		rest := (1 - top.p) / float64(max(len(q.Choices)-1, 1))
		for _, ch := range q.Choices {
			if ch.Label != top.label {
				probs[ch.Label] = rest
			}
		}
		out[q.ID] = Answer{Probabilities: probs}
	}
	return out, nil
}

// toastDraft is a good draft of tinyRecipe's graph, as a model writes it:
// loose ids, a quote with different spacing, an item it forgot to list.
func toastDraft() StructureDraft {
	return StructureDraft{Serve: "Buttered Toast",
		Items: []DraftItem{
			{ID: "Bread", Kind: "ingredient", Label: "bread", Line: 1},
			{ID: "butter", Kind: "ingredient", Label: "butter", Line: 2},
			{ID: "Buttered Toast", Kind: "final", Label: "buttered toast"},
		},
		Nodes: []DraftNode{
			{ID: "Preheat", Step: 1, Quote: "Heat the oven to 400°F.", Text: "Heat the oven", Outputs: []string{"hot oven"}},
			{ID: "toast", Step: 2, Quote: "toast the bread  for 5 minutes", Text: "Toast the bread", Inputs: []string{"Bread", "hot oven"}, Outputs: []string{"toasted"}},
			{ID: "butter", Step: 2, Quote: "then butter it.", Text: "Butter it", Inputs: []string{"toasted", "butter"}, Outputs: []string{"Buttered Toast"}},
		}}
}

// toastAnswers makes the preheat a setup in the oven and the toast a
// passive bake, unsure only of whether the toast needs watching.
func toastAnswers() map[string]map[string]ranked {
	return map[string]map[string]ranked{
		"Heat the oven": {"action": {"setup", 0.97}, "attention": {"passive", 0.9}, "equipment": {"oven", 0.96}, "duration": {"10", 0.9}},
		"Toast the":     {"action": {"heat", 0.95}, "attention": {"intermittent", 0.55}, "check": {"2", 0.7}, "equipment": {"oven", 0.9}},
		"butter it":     {"action": {"assemble", 0.92}, "anchor": {"just_before_serve", 0.92}},
	}
}

func upgradeService(t *testing.T, gen *fakeGen, dec *fakeDec) *Service {
	t.Helper()
	s := newTestService(t)
	s.SetUpgrader(&Upgrader{Gen: gen, Dec: dec})
	return s
}

func TestUpgradeRecipe(t *testing.T) {
	gen := &fakeGen{drafts: []StructureDraft{toastDraft()}}
	dec := &fakeDec{answers: toastAnswers()}
	s := upgradeService(t, gen, dec)
	ann := person(t, s, "Ann")
	r := tinyRecipe(t, s, ann)

	res, err := s.UpgradeRecipe(ctx, ann, UpgradeRequest{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	g := res.Graph.Graph
	if g.Status != GraphDraft || res.Attempts != 1 || res.Unplaced != 0 || !res.Graph.Check.Valid {
		t.Fatalf("result: status %s, attempts %d, unplaced %d, check %s", g.Status, res.Attempts, res.Unplaced, dump(res.Graph.Check))
	}
	if !strings.Contains(gen.prompts[0], "1. 2 slices bread") || !strings.Contains(gen.prompts[0], "2. Toast the bread for 5 minutes") {
		t.Errorf("prompt: %s", gen.prompts[0])
	}
	byID := map[string]ActionNode{}
	for _, n := range g.Nodes {
		byID[n.ID] = n
	}
	toast, pre := byID["toast"], byID["preheat"]
	if toast.Source.Quote != "Toast the bread for 5 minutes" {
		t.Errorf("quote found as %q", toast.Source.Quote)
	}
	// Stated time wins, and the oven takes the step's temperature.
	if toast.Duration.Source != "recipe_text" || toast.Duration.Value != (Duration{5, 5, 5}) {
		t.Errorf("toast duration %s", dump(toast.Duration))
	}
	if c := toast.Equipment.Value; len(c) != 1 || c[0].Resource != ResOven || c[0].TempF == nil || *c[0].TempF != 400 {
		t.Errorf("toast equipment %s", dump(toast.Equipment))
	}
	if pre.Action.Value != "setup" || pre.Duration.Source != "jev" || pre.Duration.Value != (Duration{5, 10, 20}) {
		t.Errorf("preheat %s", dump(pre))
	}
	// The action question reads only the action's words; interruptible
	// is left to follow attention.
	if !slices.Contains(dec.states, `"Toast the bread for 5 minutes"`) || toast.Interruptible != nil {
		t.Errorf("action states %q, interruptible %s", dec.states, dump(toast.Interruptible))
	}
	if toast.CheckInterval == nil || toast.CheckInterval.Value != 2 || len(toast.Attention.Alternatives) != 2 {
		t.Errorf("toast attention %s, check %s", dump(toast.Attention), dump(toast.CheckInterval))
	}
	// Only the toast's attention (0.55, under 0.85) and check interval
	// (0.7 but not critical: kept) — attention alone is under its bar.
	if res.Graph.ReviewCount != 1 {
		t.Errorf("review count %d, want 1", res.Graph.ReviewCount)
	}
	// Items it forgot are added, and ids are made valid.
	ids := map[string]string{}
	for _, it := range g.Items {
		ids[it.ID] = it.Kind
	}
	if ids["hot-oven"] != "intermediate" || ids["toasted"] != "intermediate" || ids["buttered-toast"] != "final" || g.ServeItemID != "buttered-toast" {
		t.Errorf("items %v, serve %s", ids, g.ServeItemID)
	}

	if _, err := s.UpgradeRecipe(ctx, ann, UpgradeRequest{ID: r.ID}); !isCode(err, CodeExists) {
		t.Errorf("again without force: %v", err)
	}
	res, err = s.UpgradeRecipe(ctx, ann, UpgradeRequest{ID: r.ID, Mode: UpgradeApply, Force: true})
	if err != nil || res.Graph.Graph.Status != GraphNeedsReview || res.Graph.Graph.Version != 2 {
		t.Fatalf("apply with force: %v, %s", err, res.Graph.Graph.Status)
	}
}

// With auto-verify, an apply that's sure of everything saves as verified;
// without it, or with anything to review, it waits for a person.
func TestUpgradeAutoVerify(t *testing.T) {
	sure := toastAnswers()
	sure["Toast the"]["attention"] = ranked{"passive", 0.97}
	gen := &fakeGen{drafts: []StructureDraft{toastDraft()}}
	s := upgradeService(t, gen, &fakeDec{answers: sure})
	s.up.AutoVerify = true
	ann := person(t, s, "Ann")
	r := tinyRecipe(t, s, ann)
	res, err := s.UpgradeRecipe(ctx, ann, UpgradeRequest{ID: r.ID, Mode: UpgradeApply})
	if err != nil || res.Graph.Graph.Status != GraphVerified || res.Graph.ReviewCount != 0 {
		t.Fatalf("auto-verify: %v, %s, %d to review", err, res.Graph.Graph.Status, res.Graph.ReviewCount)
	}
	if by := res.Graph.Graph.Nodes[0].Action.ReviewedBy; by == nil || *by != "ann" {
		t.Errorf("verified labels reviewed by %v", by)
	}
}

// A draft with errors goes back once with the problems, and the fixed one
// is used.
func TestUpgradeRepairs(t *testing.T) {
	bad := toastDraft()
	bad.Nodes[1].Outputs = []string{"toast-bits"} // butter's "toasted" is now made by nothing
	gen := &fakeGen{drafts: []StructureDraft{bad, toastDraft()}}
	s := upgradeService(t, gen, &fakeDec{answers: toastAnswers()})
	ann := person(t, s, "Ann")
	r := tinyRecipe(t, s, ann)
	res, err := s.UpgradeRecipe(ctx, ann, UpgradeRequest{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if res.Attempts != 2 || !res.Graph.Check.Valid || len(gen.prompts) != 2 {
		t.Fatalf("attempts %d, valid %v", res.Attempts, res.Graph.Check.Valid)
	}
	if !strings.Contains(gen.prompts[1], "no node makes it") || !strings.Contains(gen.prompts[1], `"toast-bits"`) {
		t.Errorf("repair prompt: %s", gen.prompts[1])
	}

	// Still broken after the repair: saved anyway, with its errors, for a
	// person to sort out.
	gen2 := &fakeGen{drafts: []StructureDraft{bad}}
	s2 := upgradeService(t, gen2, &fakeDec{answers: toastAnswers()})
	bob := person(t, s2, "Bob")
	r2 := tinyRecipe(t, s2, bob)
	res, err = s2.UpgradeRecipe(ctx, bob, UpgradeRequest{ID: r2.ID, Mode: UpgradeApply})
	if err != nil || res.Graph.Check.Valid || res.Graph.Graph.Status != GraphNeedsReview || res.Attempts != 2 {
		t.Errorf("unrepaired: %v, valid %v, %s", err, res.Graph.Check.Valid, res.Graph.Graph.Status)
	}
}

func TestUpgradeRefusals(t *testing.T) {
	s := newTestService(t)
	ann := person(t, s, "Ann")
	r := tinyRecipe(t, s, ann)
	if _, err := s.UpgradeRecipe(ctx, ann, UpgradeRequest{ID: r.ID}); !isCode(err, CodeUnavailable) {
		t.Errorf("no upgrader: %v", err)
	}
	if _, err := s.QueueUpgrades(ctx, ann, UpgradeJobRequest{}); !isCode(err, CodeUnavailable) {
		t.Errorf("no upgrader, queue: %v", err)
	}

	gen := &fakeGen{drafts: []StructureDraft{toastDraft()}}
	dec := &fakeDec{answers: toastAnswers(), err: fmt.Errorf("%w: connection refused", ErrModelsUnavailable)}
	s.SetUpgrader(&Upgrader{Gen: gen, Dec: dec})
	if _, err := s.UpgradeRecipe(ctx, ann, UpgradeRequest{ID: r.ID}); !isCode(err, CodeUnavailable) {
		t.Errorf("unreachable: %v", err)
	}
	dec.err = nil
	if _, err := s.UpgradeRecipe(ctx, ann, UpgradeRequest{ID: r.ID, Mode: "maybe"}); !isCode(err, CodeInvalid) {
		t.Errorf("bad mode: %v", err)
	}
	if _, err := s.UpgradeRecipe(ctx, nobody, UpgradeRequest{ID: r.ID}); !isCode(err, CodeWhoAreYou) {
		t.Errorf("nobody: %v", err)
	}

	// Someone edits the recipe while the models work: refused, not saved
	// against words it never read.
	gen.during = func() {
		gen.during = nil
		cur, _ := s.Get(ctx, ann, r.ID)
		if _, err := s.Update(ctx, ann, r.ID, cur.Version, RecipePatch{Title: Some("Hot toast")}); err != nil {
			t.Error(err)
		}
	}
	if _, err := s.UpgradeRecipe(ctx, ann, UpgradeRequest{ID: r.ID}); !isCode(err, CodeStale) {
		t.Errorf("edited mid-upgrade: %v", err)
	}
	if _, err := s.RecipeGraph(ctx, ann, r.ID); !isCode(err, CodeNotFound) {
		t.Errorf("a graph was saved: %v", err)
	}
}

var nobody = identity.Caller{}

func waitABit() { time.Sleep(10 * time.Millisecond) }

func TestUpgradeJobs(t *testing.T) {
	gen := &fakeGen{drafts: []StructureDraft{toastDraft()}}
	dec := &fakeDec{answers: toastAnswers()}
	s := upgradeService(t, gen, dec)
	ann := person(t, s, "Ann")
	a := tinyRecipe(t, s, ann)
	b := create(t, s, ann, RecipeInput{Title: "Butter toast", Tags: []string{"Breakfast"}, Ingredients: []string{"2 slices bread", "1 tbsp butter"},
		Steps: []string{"Heat the oven to 400°F.", "Toast the bread for 5 minutes, then butter it."}})
	gone := create(t, s, ann, RecipeInput{Title: "Zzz gone", Steps: []string{"Nothing."}})

	// Only Breakfast: one recipe.
	j, err := s.QueueUpgrades(ctx, ann, UpgradeJobRequest{Tag: "breakfast"})
	if err != nil || len(j.Items) != 1 || j.Items[0].RecipeID != b.ID || j.Status != "queued" {
		t.Fatalf("by tag: %v %s", err, dump(j))
	}
	if _, err := s.QueueUpgrades(ctx, ann, UpgradeJobRequest{Tag: "nothing"}); !isCode(err, CodeInvalid) {
		t.Errorf("no match: %v", err)
	}
	// Everything without a graph, in title order; one trashed before its turn.
	all, err := s.QueueUpgrades(ctx, ann, UpgradeJobRequest{Mode: UpgradeApply})
	if err != nil || len(all.Items) != 3 {
		t.Fatalf("all: %v %s", err, dump(all))
	}
	if err := s.Delete(ctx, ann, gone.ID, gone.Version); err != nil {
		t.Fatal(err)
	}

	// The models go away: the recipe goes back to pending, not failed.
	dec.err = fmt.Errorf("%w: down", ErrModelsUnavailable)
	if worked, wait := s.upgradeNext(ctx); worked || wait == 0 {
		t.Errorf("unreachable: worked %v, wait %v", worked, wait)
	}
	j, _ = s.UpgradeJob(ctx, ann, j.ID)
	if j.Items[0].Status != "pending" {
		t.Errorf("after unreachable: %s", dump(j))
	}
	dec.err = nil
	for i := 0; ; i++ {
		worked, _ := s.upgradeNext(ctx)
		if !worked {
			break
		}
		if i > 10 {
			t.Fatal("worker never ran dry")
		}
	}
	j, _ = s.UpgradeJob(ctx, ann, j.ID)
	if j.Status != "done" || j.Counts["needs_review"] != 1 || j.FinishedAt == nil {
		t.Errorf("first job: %s", dump(j))
	}
	all, _ = s.UpgradeJob(ctx, ann, all.ID)
	// b was done by the first job, so the second skips it; a is upgraded;
	// the trashed one is skipped.
	got := map[string]string{}
	for _, it := range all.Items {
		got[it.RecipeID] = it.Status
	}
	if got[a.ID] != "needs_review" || got[b.ID] != "skipped" || got[gone.ID] != "skipped" || all.Status != "done" {
		t.Errorf("second job: %s", dump(all))
	}
	if v, err := s.RecipeGraph(ctx, ann, a.ID); err != nil || v.Graph.Status != GraphNeedsReview || v.Graph.CreatedBy != "ann" {
		t.Errorf("a's graph: %v %s", err, v.Graph.Status)
	}

	// Cancelling skips what hasn't started.
	c, err := s.QueueUpgrades(ctx, ann, UpgradeJobRequest{IDs: []string{a.ID, b.ID}, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	c, err = s.CancelUpgradeJob(ctx, ann, c.ID)
	if err != nil || c.Status != "cancelled" || c.Counts["skipped"] != 2 {
		t.Errorf("cancel: %v %s", err, dump(c))
	}
	if worked, _ := s.upgradeNext(ctx); worked {
		t.Error("worked on a cancelled job")
	}
	if _, err := s.UpgradeJob(ctx, ann, "nope"); !isCode(err, CodeNotFound) {
		t.Errorf("unknown job: %v", err)
	}

	// A job left running by a stop is picked up again.
	r, err := s.QueueUpgrades(ctx, ann, UpgradeJobRequest{IDs: []string{a.ID}, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE upgrade_job_items SET status = 'running' WHERE job_id = ?`, r.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	run, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { s.RunUpgrades(run); close(done) }()
	for range 200 {
		if j, _ := s.UpgradeJob(ctx, ann, r.ID); j.Status == "done" {
			break
		}
		waitABit()
	}
	stop()
	<-done
	if j, _ := s.UpgradeJob(ctx, ann, r.ID); j.Status != "done" {
		t.Errorf("resumed job: %s", dump(j))
	}
}

func TestReviewQueue(t *testing.T) {
	gen := &fakeGen{drafts: []StructureDraft{toastDraft()}}
	answers := toastAnswers()
	answers["butter it"]["hold"] = ranked{"15", 0.7}
	s := upgradeService(t, gen, &fakeDec{answers: answers})
	ann := person(t, s, "Ann")
	r := tinyRecipe(t, s, ann)
	res, err := s.UpgradeRecipe(ctx, ann, UpgradeRequest{ID: r.ID, Mode: UpgradeApply})
	if err != nil {
		t.Fatal(err)
	}
	page, err := s.ReviewItems(ctx, ann, ReviewQuery{RecipeID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if page.Remaining != 2 || len(page.Items) != 2 {
		t.Fatalf("queue: %s", dump(page))
	}
	// Least sure first: toast's attention at 0.55, then butter's hold at 0.7
	// (every hold the model proposes is asked about).
	if it := page.Items[0]; it.NodeID != "toast" || it.Field != "attention" || it.Value != "intermittent" || it.Alternatives == nil {
		t.Errorf("first: %s", dump(it))
	}
	if it := page.Items[1]; it.NodeID != "butter" || it.Field != "hold" || !it.Critical || it.GraphVersion != res.Graph.Graph.Version {
		t.Errorf("second: %s", dump(it))
	}
	p1, err := s.ReviewItems(ctx, ann, ReviewQuery{Limit: 1})
	if err != nil || len(p1.Items) != 1 || p1.NextCursor == nil {
		t.Fatalf("page 1: %v %s", err, dump(p1))
	}
	p2, err := s.ReviewItems(ctx, ann, ReviewQuery{Limit: 1, Cursor: *p1.NextCursor})
	if err != nil || len(p2.Items) != 1 || p2.NextCursor != nil || p2.Items[0].NodeID != "butter" {
		t.Errorf("page 2: %v %s", err, dump(p2))
	}
	if _, err := s.ReviewItems(ctx, ann, ReviewQuery{Field: "attention", Limit: 101}); !isCode(err, CodeInvalid) {
		t.Errorf("limit: %v", err)
	}

	// Accepting with things left is refused.
	v := res.Graph.Graph.Version
	if _, err := s.ResolveReview(ctx, ann, ResolveRequest{RecipeID: r.ID, GraphVersion: v, Accept: true}); !isCode(err, CodeInvalid) {
		t.Errorf("accept early: %v", err)
	}
	// Correct the attention, confirm the hold.
	out, err := s.ResolveReview(ctx, ann, ResolveRequest{RecipeID: r.ID, GraphVersion: v, Items: []ResolveChoice{
		{NodeID: "toast", Field: "attention", Value: "passive"},
		{NodeID: "butter", Field: "hold", Value: map[string]any{"max_min": 15, "where": "counter"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Remaining != 0 || out.Graph.Graph.Status != GraphNeedsReview {
		t.Fatalf("resolved: %d left, %s", out.Remaining, out.Graph.Graph.Status)
	}
	for _, n := range out.Graph.Graph.Nodes {
		switch n.ID {
		case "toast":
			if n.Attention.Value != "passive" || n.Attention.Source != "user" || *n.Attention.Confidence != 1 || n.Attention.ReviewedBy == nil {
				t.Errorf("corrected attention: %s", dump(n.Attention))
			}
		case "butter":
			if n.Hold.Source != "jev" || *n.Hold.Confidence != 0.7 || n.Hold.ReviewedBy == nil || *n.Hold.ReviewedBy != "ann" {
				t.Errorf("confirmed hold: %s", dump(n.Hold))
			}
		}
	}
	var corrections int
	var field, model, user string
	if err := s.st.Read(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRow(`SELECT count(*) FROM review_corrections`).Scan(&corrections); err != nil {
			return err
		}
		return tx.QueryRow(`SELECT field, model_value, user_value FROM review_corrections`).Scan(&field, &model, &user)
	}); err != nil {
		t.Fatal(err)
	}
	if corrections != 1 || field != "attention" || model != `"intermittent"` || user != `"passive"` {
		t.Errorf("corrections: %d %s %s %s", corrections, field, model, user)
	}

	// "Use at once" takes a hold away, and counts as a correction.
	v = out.Graph.Graph.Version
	out, err = s.ResolveReview(ctx, ann, ResolveRequest{RecipeID: r.ID, GraphVersion: v, Items: []ResolveChoice{{NodeID: "butter", Field: "hold", Value: nil}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range out.Graph.Graph.Nodes {
		if n.ID == "butter" && n.Hold != nil {
			t.Errorf("hold kept: %s", dump(n.Hold))
		}
	}
	v = out.Graph.Graph.Version

	// Stale version, bad field, bad value.
	if _, err := s.ResolveReview(ctx, ann, ResolveRequest{RecipeID: r.ID, GraphVersion: v - 1, Accept: true}); !isCode(err, CodeStale) {
		t.Errorf("stale: %v", err)
	}
	for _, bad := range []ResolveChoice{
		{NodeID: "toast", Field: "colour", Value: "red"},
		{NodeID: "toast", Field: "attention", Value: "sometimes"},
		{NodeID: "toast", Field: "duration", Value: "long"},
		{NodeID: "nope", Field: "attention", Value: "active"},
	} {
		if _, err := s.ResolveReview(ctx, ann, ResolveRequest{RecipeID: r.ID, GraphVersion: v, Items: []ResolveChoice{bad}}); !isCode(err, CodeInvalid) {
			t.Errorf("%s: %v", dump(bad), err)
		}
	}
	out, err = s.ResolveReview(ctx, ann, ResolveRequest{RecipeID: r.ID, GraphVersion: v, Accept: true})
	if err != nil || out.Graph.Graph.Status != GraphVerified {
		t.Errorf("accept: %v, %s", err, out.Graph.Graph.Status)
	}
	if page, _ := s.ReviewItems(ctx, ann, ReviewQuery{}); page.Remaining != 0 {
		t.Errorf("left after accepting: %s", dump(page))
	}
}

// Only attention and equipment under 0.7, every proposed hold and implied
// actions are asked about; a weak hold is dropped, and words that agree
// with the model raise its confidence.
func TestReviewPolicy(t *testing.T) {
	answers := toastAnswers()
	answers["Toast the"]["attention"] = ranked{"passive", 0.6}  // "toast" isn't in the words' passive list: asked
	answers["Toast the"]["equipment"] = ranked{"oven", 0.6}     // nor is "toast" an oven word: asked
	answers["Heat the oven"]["equipment"] = ranked{"oven", 0.6} // the words say oven: raised to 0.8, not asked
	answers["butter it"]["hold"] = ranked{"60", 0.5}            // too weak: dropped
	answers["butter it"]["action"] = ranked{"assemble", 0.3}    // unsure, but action isn't asked
	s := upgradeService(t, &fakeGen{drafts: []StructureDraft{toastDraft()}}, &fakeDec{answers: answers})
	ann := person(t, s, "Ann")
	r := tinyRecipe(t, s, ann)
	res, err := s.UpgradeRecipe(ctx, ann, UpgradeRequest{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]ActionNode{}
	for _, n := range res.Graph.Graph.Nodes {
		byID[n.ID] = n
	}
	if c := *byID["preheat"].Equipment.Confidence; c != 0.8 {
		t.Errorf("agreeing words: confidence %v, want 0.8", c)
	}
	if byID["butter"].Hold != nil {
		t.Errorf("weak hold kept: %s", dump(byID["butter"].Hold))
	}
	page, err := s.ReviewItems(ctx, ann, ReviewQuery{RecipeID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, it := range page.Items {
		got = append(got, it.NodeID+"."+it.Field)
	}
	slices.Sort(got)
	if !slices.Equal(got, []string{"toast.attention", "toast.equipment"}) || res.Graph.ReviewCount != 2 {
		t.Errorf("asked about %v (count %d)", got, res.Graph.ReviewCount)
	}
}

func TestAssemble(t *testing.T) {
	r := Recipe{Title: "Purée", Ingredients: []Ingredient{{Raw: "1 lb carrots"}},
		Steps: []Step{{Text: "Boil the carrots\nuntil soft, 20 to 25 minutes."}, {Text: "Purée with a stick blender."}}}
	d := StructureDraft{Serve: "Purée",
		Items: []DraftItem{{ID: "Carrots", Kind: "ingredient", Label: "carrots", Line: 1}, {ID: "carrots", Kind: "spice", Label: ""},
			{ID: "x", Kind: "ingredient", Line: 7}},
		Nodes: []DraftNode{
			{ID: "boil", Step: 1, Quote: "boil the carrots until soft, 20 to 25 minutes", Inputs: []string{"Carrots"}, Outputs: []string{"soft carrots"}},
			{ID: "boil", Step: 1, Quote: "Purée with a stick blender.", Inputs: []string{"soft carrots"}, Outputs: []string{"Purée"}},
			{ID: "water", Step: 9, Implied: true, Quote: "anything", Text: "Boil water", Outputs: []string{"boiling water"}},
			{ID: "lost", Step: 2, Quote: "words that aren't there", Outputs: []string{"mystery"}},
		}}
	g, missed := assemble(d, r)
	if len(missed) != 1 || missed[0] != "lost" || g.ServeItemID != "puree" {
		t.Fatalf("missed %v, serve %s", missed, g.ServeItemID)
	}
	n := g.Nodes
	if n[0].Source != (SourceSpan{Step: 1, Start: 0, End: 45}) {
		t.Errorf("fuzzy quote: %s", dump(n[0].Source))
	}
	if n[1].ID != "boil-2" || n[1].Source.Step != 2 || n[1].Source.Start != 0 || n[1].Source.End != 27 {
		t.Errorf("quote in another step: %s", dump(n[1]))
	}
	if n[2].Source != (SourceSpan{Step: 2, Start: 0, End: 0}) {
		t.Errorf("implied clamps its step and empties its span: %s", dump(n[2].Source))
	}
	if n[3].Source != (SourceSpan{Step: 2, Start: 0, End: 27}) {
		t.Errorf("missing quote points at the whole step: %s", dump(n[3].Source))
	}
	items := map[string]GraphItem{}
	for _, it := range g.Items {
		items[it.ID] = it
	}
	// A second "carrots" with an unknown kind is kept apart as an
	// intermediate; a line past the list is dropped.
	if len(g.Items) != 7 || items["carrots"].Ingredient == nil || items["carrots-2"].Kind != "intermediate" || items["x"].Ingredient != nil ||
		items["puree"].Kind != "final" || items["soft-carrots"].Label != "soft carrots" || items["carrots"].Kind != "ingredient" {
		t.Errorf("items: %s", dump(g.Items))
	}
	if graphSlug("Œufs à la Neige!") != "oeufs-a-la-neige" || graphSlug("***") != "x" {
		t.Errorf("slugs: %s %s", graphSlug("Œufs à la Neige!"), graphSlug("***"))
	}
}

// A temperature an oven can't be ("cool to 105°F") is never an oven's, and
// doesn't carry to later steps.
func TestUpgradeIgnoresNonOvenTemps(t *testing.T) {
	d := StructureDraft{Serve: "rolls",
		Items: []DraftItem{{ID: "milk", Kind: "ingredient", Line: 1}, {ID: "rolls", Kind: "final"}},
		Nodes: []DraftNode{
			{ID: "cool", Step: 1, Quote: "Cool the milk to 105°F.", Text: "Cool the milk", Inputs: []string{"milk"}, Outputs: []string{"warm"}},
			{ID: "bake", Step: 2, Quote: "Bake the rolls.", Text: "Bake the rolls", Inputs: []string{"warm"}, Outputs: []string{"rolls"}},
		}}
	answers := map[string]map[string]ranked{"Cool the milk": {"equipment": {"oven", 0.9}}, "Bake the": {"equipment": {"oven", 0.9}}}
	s := upgradeService(t, &fakeGen{drafts: []StructureDraft{d}}, &fakeDec{answers: answers})
	ann := person(t, s, "Ann")
	r := create(t, s, ann, RecipeInput{Title: "Rolls", Ingredients: []string{"1 cup milk"}, Steps: []string{"Cool the milk to 105°F.", "Bake the rolls."}})
	res, err := s.UpgradeRecipe(ctx, ann, UpgradeRequest{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range res.Graph.Graph.Nodes {
		if c := n.Equipment.Value; len(c) != 1 || c[0].TempF != nil {
			t.Errorf("%s: %s", n.ID, dump(c))
		}
	}
}

// A hosted model's JSON in a code fence is unwrapped.
func TestGenerateFenced(t *testing.T) {
	u := &Upgrader{Gen: fencedGen{}}
	d, err := u.generate(ctx, Recipe{Title: "x", Steps: []Step{{Text: "a"}}}, nil, nil)
	if err != nil || d.Serve != "toast" {
		t.Errorf("fenced: %+v %v", d, err)
	}
}

type fencedGen struct{}

func (fencedGen) Generate(ctx context.Context, prompt string, schema any) ([]byte, error) {
	return []byte("```json\n{\"serve\":\"toast\",\"items\":[],\"nodes\":[]}\n```"), nil
}

func TestStatedDuration(t *testing.T) {
	for _, tc := range []struct {
		text string
		want Duration
		ok   bool
	}{
		{"bake until set, 40 to 45 minutes", Duration{40, 45, 45}, true},
		{"chill for 1 hour", Duration{60, 60, 60}, true},
		{"rise 1 to 1 1/2 hours", Duration{60, 90, 90}, true},
		{"simmer 10 minutes, then 5 minutes more", Duration{15, 15, 15}, true},
		{"leave overnight", Duration{480, 720, 720}, true},
		{"cook for half an hour", Duration{30, 30, 30}, true},
		{"beat with a fork for about 30 seconds", Duration{1, 1, 1}, true},
		{"leave for 10 to 15 seconds", Duration{0, 1, 1}, true},
		{"cook 2 minutes, then 30 seconds more", Duration{3, 3, 3}, true},
		{"keeps up to 3 days", Duration{}, false},
		{"stir well", Duration{}, false},
	} {
		got, ok := statedDuration(tc.text)
		if got != tc.want || ok != tc.ok {
			t.Errorf("%q: got %v %v, want %v %v", tc.text, got, ok, tc.want, tc.ok)
		}
	}
}

// An assistant drafts the structure; Sous checks it, classifies it and
// saves it. Problems come back listed, to fix and resend.
func TestDraftRecipeGraph(t *testing.T) {
	s := newTestService(t)
	s.SetUpgrader(&Upgrader{Dec: &fakeDec{answers: toastAnswers()}}) // no graph model of its own
	ann := person(t, s, "Ann")
	r := tinyRecipe(t, s, ann)

	if _, err := s.UpgradeRecipe(ctx, ann, UpgradeRequest{ID: r.ID}); !isCode(err, CodeUnavailable) || !strings.Contains(err.Error(), "draft_recipe_graph") {
		t.Errorf("upgrade without a graph model: %v", err)
	}
	if _, err := s.QueueUpgrades(ctx, ann, UpgradeJobRequest{}); !isCode(err, CodeUnavailable) {
		t.Errorf("queue without a graph model: %v", err)
	}

	bad := toastDraft()
	bad.Nodes[1].Quote = "Grill the bread"
	bad.Nodes[2].Inputs = []string{"jam", "butter"}
	_, err := s.DraftRecipeGraph(ctx, ann, DraftRequest{ID: r.ID, RecipeVersion: r.Version, StructureDraft: bad})
	if !isCode(err, CodeInvalid) || !strings.Contains(err.Error(), "toast: its quote isn't in step 2") || !strings.Contains(err.Error(), "jam") {
		t.Fatalf("bad draft: %v", err)
	}
	if _, err := s.RecipeGraph(ctx, ann, r.ID); !isCode(err, CodeNotFound) {
		t.Errorf("a bad draft was saved: %v", err)
	}
	if _, err := s.DraftRecipeGraph(ctx, ann, DraftRequest{ID: r.ID, StructureDraft: toastDraft()}); !isCode(err, CodeInvalid) {
		t.Errorf("no recipe version: %v", err)
	}
	if _, err := s.DraftRecipeGraph(ctx, ann, DraftRequest{ID: r.ID, RecipeVersion: r.Version + 1, StructureDraft: toastDraft()}); !isCode(err, CodeStale) {
		t.Errorf("wrong recipe version: %v", err)
	}

	res, err := s.DraftRecipeGraph(ctx, ann, DraftRequest{ID: r.ID, RecipeVersion: r.Version, Mode: UpgradeApply, StructureDraft: toastDraft()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Graph.Graph.Status != GraphNeedsReview || !res.Graph.Check.Valid || res.Graph.ReviewCount != 1 {
		t.Errorf("draft saved: %s, valid %v, %d to review", res.Graph.Graph.Status, res.Graph.Check.Valid, res.Graph.ReviewCount)
	}
	if _, err := s.DraftRecipeGraph(ctx, ann, DraftRequest{ID: r.ID, RecipeVersion: r.Version, StructureDraft: toastDraft()}); !isCode(err, CodeExists) {
		t.Errorf("again without force: %v", err)
	}
	s.up = nil
	if _, err := s.DraftRecipeGraph(ctx, ann, DraftRequest{ID: r.ID, RecipeVersion: r.Version, StructureDraft: toastDraft()}); !isCode(err, CodeUnavailable) {
		t.Errorf("upgrades off: %v", err)
	}
}

func TestRecipesWithoutGraph(t *testing.T) {
	s := newTestService(t)
	s.SetUpgrader(&Upgrader{Dec: &fakeDec{answers: toastAnswers()}})
	ann := person(t, s, "Ann")
	a := tinyRecipe(t, s, ann)
	b := create(t, s, ann, RecipeInput{Title: "Apple", Steps: []string{"Eat it."}})
	create(t, s, ann, RecipeInput{Title: "Nothing to do"})
	got, err := s.RecipesWithoutGraph(ctx, ann, GraphTodoQuery{})
	if err != nil || got.Remaining != 2 || got.Items[0].ID != b.ID || got.Items[1].Steps != 2 {
		t.Fatalf("todo: %v %s", err, dump(got))
	}
	if _, err := s.DraftRecipeGraph(ctx, ann, DraftRequest{ID: a.ID, RecipeVersion: a.Version, StructureDraft: toastDraft()}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.RecipesWithoutGraph(ctx, ann, GraphTodoQuery{Limit: 1})
	if got.Remaining != 1 || len(got.Items) != 1 || got.Items[0].ID != b.ID {
		t.Errorf("after one: %s", dump(got))
	}
	if _, err := s.RecipesWithoutGraph(ctx, ann, GraphTodoQuery{Limit: 101}); !isCode(err, CodeInvalid) {
		t.Errorf("limit: %v", err)
	}
	if _, err := s.RecipesWithoutGraph(ctx, ann, GraphTodoQuery{MealID: "nope"}); !isCode(err, CodeNotFound) {
		t.Errorf("unknown meal: %v", err)
	}
}

// The overview agrees with what each graph says of itself.
func TestGraphOverview(t *testing.T) {
	s := newTestService(t)
	ann := person(t, s, "Ann")
	o, err := s.GraphOverview(ctx, ann)
	if err != nil || o.Upgrades.Labels || o.Counts["none"] != 0 {
		t.Fatalf("empty, upgrades off: %v %s", err, dump(o))
	}
	s.SetUpgrader(&Upgrader{Dec: &fakeDec{answers: toastAnswers()}, AutoVerify: true})
	a := tinyRecipe(t, s, ann)
	b := create(t, s, ann, RecipeInput{Title: "Butter toast", Ingredients: []string{"2 slices bread", "1 tbsp butter"},
		Steps: []string{"Heat the oven to 400°F.", "Toast the bread for 5 minutes, then butter it."}})
	create(t, s, ann, RecipeInput{Title: "Apple", Steps: []string{"Eat it."}})
	create(t, s, ann, RecipeInput{Title: "No steps"})
	ra, err := s.DraftRecipeGraph(ctx, ann, DraftRequest{ID: a.ID, RecipeVersion: a.Version, Mode: UpgradeApply, StructureDraft: toastDraft()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DraftRecipeGraph(ctx, ann, DraftRequest{ID: b.ID, RecipeVersion: b.Version, StructureDraft: toastDraft()}); err != nil {
		t.Fatal(err)
	}
	// b's toasting words change under its graph.
	if _, err := s.Update(ctx, ann, b.ID, b.Version, RecipePatch{Steps: Some([]string{"Heat the oven to 400°F.", "Grill the bread for 5 minutes, then butter it."})}); err != nil {
		t.Fatal(err)
	}
	o, err = s.GraphOverview(ctx, ann)
	if err != nil {
		t.Fatal(err)
	}
	if !o.Upgrades.Labels || o.Upgrades.GraphModel || !o.Upgrades.AutoVerify {
		t.Errorf("upgrades: %s", dump(o.Upgrades))
	}
	if o.Counts["none"] != 1 || o.Counts[GraphNeedsReview] != 1 || o.Counts[GraphStale] != 1 || len(o.Items) != 2 {
		t.Fatalf("counts: %s", dump(o))
	}
	got := map[string]GraphStatusRow{}
	for _, it := range o.Items {
		got[it.RecipeID] = it
	}
	if got[a.ID].ReviewCount != ra.Graph.ReviewCount || got[a.ID].Version != ra.Graph.Graph.Version || got[b.ID].Status != GraphStale {
		t.Errorf("rows: %s, want a's review count %d", dump(o.Items), ra.Graph.ReviewCount)
	}
	if v, _ := s.RecipeGraph(ctx, ann, b.ID); v.Graph.Status != GraphStale {
		t.Errorf("the graph itself says %s", v.Graph.Status)
	}
}
