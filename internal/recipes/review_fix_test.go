package recipes

import "testing"

// A correction to a value label drops the alternatives the model weighed for
// the old value, as it does for the pointer labels.
func TestFillLabelDropsOldAlternatives(t *testing.T) {
	n := ActionNode{Action: Label[string]{Value: "heat", Source: "jev",
		Alternatives: []Alternative[string]{{Value: "combine", Probability: 0.3}}}}
	if err := fillLabel(&n, "action", `"prep"`, "user", 1, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if n.Action.Value != "prep" || n.Action.Source != "user" || n.Action.Alternatives != nil {
		t.Errorf("action = %+v, want prep from user with no alternatives", n.Action)
	}
}

// A graph whose words have changed leaves the review queue: its questions
// can't be answered until it's drafted again.
func TestReviewQueueSkipsStale(t *testing.T) {
	s := upgradeService(t, &fakeGen{drafts: []StructureDraft{toastDraft()}}, &fakeDec{answers: toastAnswers()})
	ann := person(t, s, "Ann")
	r := tinyRecipe(t, s, ann)
	if _, err := s.UpgradeRecipe(ctx, ann, UpgradeRequest{ID: r.ID}); err != nil {
		t.Fatal(err)
	}
	if page, _ := s.ReviewItems(ctx, ann, ReviewQuery{}); page.Remaining == 0 {
		t.Fatal("nothing to review to begin with")
	}
	r, _ = s.Get(ctx, ann, r.ID)
	if _, err := s.Update(ctx, ann, r.ID, r.Version, RecipePatch{Steps: Some([]string{"Heat the oven to 400°F.", "Grill the bread for 5 minutes, then butter it."})}); err != nil {
		t.Fatal(err)
	}
	if page, err := s.ReviewItems(ctx, ann, ReviewQuery{}); err != nil || page.Remaining != 0 {
		t.Errorf("stale graph still queued: %v %s", err, dump(page))
	}
}

// A confirmed label held by pointer (equipment) keeps its review when a
// later save answers something else: the "before" graph used to carry
// stamps over shared its labels with the one being saved.
func TestReviewStampSurvivesLaterSave(t *testing.T) {
	answers := toastAnswers()
	answers["Toast the"]["equipment"] = ranked{"oven", 0.5}
	s := upgradeService(t, &fakeGen{drafts: []StructureDraft{toastDraft()}}, &fakeDec{answers: answers})
	ann := person(t, s, "Ann")
	r := tinyRecipe(t, s, ann)
	res, err := s.UpgradeRecipe(ctx, ann, UpgradeRequest{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	var eq any
	for _, n := range res.Graph.Graph.Nodes {
		if n.ID == "toast" {
			eq = n.Equipment.Value
		}
	}
	out, err := s.ResolveReview(ctx, ann, ResolveRequest{RecipeID: r.ID, GraphVersion: res.Graph.Graph.Version,
		Items: []ResolveChoice{{NodeID: "toast", Field: "equipment", Value: eq}}})
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.ResolveReview(ctx, ann, ResolveRequest{RecipeID: r.ID, GraphVersion: out.Graph.Graph.Version,
		Items: []ResolveChoice{{NodeID: "toast", Field: "attention", Value: "passive"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range out.Graph.Graph.Nodes {
		if n.ID == "toast" && n.Equipment.ReviewedBy == nil {
			t.Errorf("the confirmed equipment lost its review: %s", dump(n.Equipment))
		}
	}
}
