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
