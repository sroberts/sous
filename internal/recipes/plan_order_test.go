package recipes

import (
	"testing"
	"time"
)

// Make-ahead that was missed and is replanned from the small hours of the
// day still runs in its recipe's order: when the bake can't fit after the
// crust going forwards, it goes as late as it fits, but never before the
// crust is done.
func TestPlanMissedMakeAheadKeepsStepOrder(t *testing.T) {
	for _, c := range []struct {
		serve string
		h     int
	}{
		{"2026-11-26T22:00:00.000Z", 6}, {"2026-11-26T22:00:00.000Z", 11},
		{"2026-11-26T18:00:00.000Z", 6}, {"2026-11-26T17:00:00.000Z", 6}, {"2026-11-26T19:00:00.000Z", 5},
	} {
		h := c.h
		in := thanksgiving()
		in.meal.ServeAt = c.serve
		// h:00 UTC on the day: 1 AM to 7 AM in New York, the pie's make-ahead
		// (meant for the day before) not started.
		in.now = time.Date(2026, 11, 26, h, 0, 0, 0, time.UTC)
		p := mustPlan(t, in)
		// By step number, not by start: out of order is exactly what's checked.
		steps := map[string]map[int]PlanTask{}
		for _, task := range p.Tasks {
			if task.ItemID == nil || task.Step == nil {
				continue
			}
			if steps[*task.ItemID] == nil {
				steps[*task.ItemID] = map[int]PlanTask{}
			}
			steps[*task.ItemID][*task.Step] = task
		}
		for _, byStep := range steps {
			for n := 2; byStep[n].Key != ""; n++ {
				prev, cur := byStep[n-1], byStep[n]
				if prev.Key != "" && at(t, cur.Start).Before(at(t, prev.End)) {
					t.Errorf("now %s: %s step %d starts %s, before step %d ends at %s", in.now, *cur.RecipeTitle, n, cur.Start, n-1, prev.End)
				}
			}
		}
	}
}
