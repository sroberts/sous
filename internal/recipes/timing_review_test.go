package recipes

import "testing"

// A step that preheats and also does real work is read for the work, not
// dropped as a preheat; one that only preheats still is.
func TestSuggestPreheatWithOtherWork(t *testing.T) {
	g := SuggestTimings([]string{
		"Heat the oven to 325°F (165°C) with a rack in the lower third. Pat the beef very dry and season with the salt and pepper.",
		"Braise in the oven for 2 to 3 hours.",
		"Preheat the oven to 350°F.",
	})
	if g[0].Minutes != nil && *g[0].Minutes == 0 {
		t.Errorf("mixed step read as a preheat: %+v", g[0])
	}
	if g[0].Equipment != nil {
		t.Errorf("patting the beef dry needs no %s", *g[0].Equipment)
	}
	if g[2].Minutes == nil || *g[2].Minutes != 0 {
		t.Errorf("a step that only preheats: %+v, want 0 minutes", g[2])
	}
	// 325°F, or 165°C read back as about the same.
	if g[1].OvenTemp == nil || abs(*g[1].OvenTemp-325) > 10 {
		t.Errorf("the temperature carries forward from the mixed step: %+v", g[1])
	}
}

// An appliance offered as an alternative isn't needed.
func TestSuggestApplianceAlternative(t *testing.T) {
	g := SuggestTimings([]string{
		"Melt the chocolate over barely simmering water, or in the microwave in short bursts.",
		"Microwave the butter until melted, about 1 minute.",
	})
	if g[0].Equipment != nil && *g[0].Equipment == "microwave" {
		t.Errorf("an alternative microwave read as needed: %+v", g[0])
	}
	if g[1].Equipment == nil || *g[1].Equipment != "microwave" {
		t.Errorf("a microwave step: %+v", g[1])
	}
}
