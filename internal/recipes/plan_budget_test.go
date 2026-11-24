package recipes

import (
	"strconv"
	"testing"
	"time"
)

// TestPlanBudget: a menu built so that most slots fail (one cook, one
// single-rack oven swinging between temperatures, make-ahead holds) is
// still planned in bounded time, with what didn't fit flagged.
func TestPlanBudget(t *testing.T) {
	var steps []Step
	for j := 0; j < 15; j++ {
		if j%3 == 2 {
			steps = append(steps, timed("Bake "+strconv.Itoa(j), 120, false, oven(300, 1), hold(3*24*60, HoldFridge)))
		} else {
			steps = append(steps, timed("Bake "+strconv.Itoa(j), 60, true, oven(200+(j%2)*400, 1)))
		}
	}
	r := Recipe{ID: "r", Title: "Big", Steps: steps}
	in := planInput{
		meal:    Meal{ID: "m", Title: "M", ServeAt: "2026-11-26T22:00:00.000Z", Timezone: "America/New_York", Cooks: 1, PrepDays: 7, Version: 1},
		kitchen: KitchenProfile{Ovens: []Oven{{Name: "Oven", Racks: 1}}, Burners: 1, DayStart: "08:00", DayEnd: "22:00"},
		now:     time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC),
	}
	for i := 0; i < maxMealItems; i++ {
		in.meal.Items = append(in.meal.Items, MealItem{ID: "i" + strconv.Itoa(i), RecipeID: "r", Title: "Big"})
		in.recipes = append(in.recipes, r)
	}
	start := time.Now()
	p, err := buildPlan(in)
	if err != nil {
		t.Fatal(err)
	}
	if !p.OnTime {
		lateOptions(in, p)
	}
	// Unbounded, this took about 12 seconds; bounded, well under one.
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("planning took %v", d)
	}
	if len(p.Tasks) < len(in.meal.Items)*len(steps) {
		t.Errorf("only %d tasks planned", len(p.Tasks))
	}
}

func TestTaskKnown(t *testing.T) {
	m := Meal{Items: []MealItem{{ID: "a"}}}
	for key, want := range map[string]bool{
		"a:0": true, "a:599": true, "a:reheat": true, "preheat:0:a:3": true, "preheat:5:a:reheat": true,
		"a:600": false, "a:-1": false, "a:01": false, "a:x": false, "a:": false, "b:0": false,
		"preheat:6:a:0": false, "preheat:-1:a:0": false, "preheat:00:a:0": false, "preheat:0:b:0": false,
	} {
		if got := taskKnown(m, key); got != want {
			t.Errorf("taskKnown(%q) = %v, want %v", key, got, want)
		}
	}
}
