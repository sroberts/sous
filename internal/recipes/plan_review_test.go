package recipes

import (
	"database/sql"
	"strings"
	"testing"
	"time"
)

// A once-only warning (an unknown appliance, no oven) survives the extra
// passes a live plan makes.
func TestPlanLiveKeepsKitchenWarnings(t *testing.T) {
	in := thanksgiving()
	in.recipes[4].Steps[1] = timed("Cook in the slow cooker.", 240, false, func(t *StepTiming) { t.Equipment = ref("slow cooker") })
	in.now = time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC) // the day itself: live
	p := mustPlan(t, in)
	n := 0
	for _, w := range p.Warnings {
		if w.Code == "unknown_appliance" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("unknown_appliance warnings = %d, want 1: %+v", n, p.Warnings)
	}
}

// The prep window opens at day_start by the wall clock, even on the day
// the clocks go back.
func TestPlanPrepWindowAcrossDST(t *testing.T) {
	in := thanksgiving()
	in.meal.ServeAt = "2026-11-02T22:00:00.000Z" // the day after US clocks go back
	in.now = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	p := mustPlan(t, in)
	from := at(t, p.PrepFrom).In(newYork)
	if from.Day() != 1 || from.Hour() != 8 || from.Minute() != 0 {
		t.Errorf("prep_from = %s, want 8:00 AM on November 1", from)
	}
}

// Confirming an oven is on survives an edit to the menu that keeps the dish.
func TestMealPreheatProgressKeptOnMenuEdit(t *testing.T) {
	s := newTestService(t)
	ann := person(t, s, "Ann")
	pie := create(t, s, ann, RecipeInput{Title: "Pie", Steps: []string{"Bake at 400°F for 50 minutes."}})
	m, err := s.CreateMeal(ctx, ann, MealInput{Title: "Dinner", ServeAt: "2026-11-26T17:00:00-05:00", Timezone: "America/New_York",
		Items: []MealItemInput{{RecipeID: pie.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	p, err := s.MealPlan(ctx, ann, m.ID, when)
	if err != nil {
		t.Fatal(err)
	}
	key := ""
	for _, task := range p.Tasks {
		if strings.HasPrefix(task.Key, "preheat:") {
			key = task.Key
		}
	}
	if key == "" {
		t.Fatalf("no preheat in %s", dump(p.Tasks))
	}
	if _, err := s.UpdateTask(ctx, ann, m.ID, key, TaskPatch{Started: Some(true)}); err != nil {
		t.Fatal(err)
	}
	m, err = s.UpdateMeal(ctx, ann, m.ID, m.Version, MealPatch{
		Items: Some([]MealItemInput{{ID: ref(m.Items[0].ID), RecipeID: pie.ID, Dish: ref("Pie dish")}}),
	})
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.MealPlan(ctx, ann, m.ID, when)
	if err != nil {
		t.Fatal(err)
	}
	if got := taskByKey(t, p, key); got.StartedAt == nil {
		t.Errorf("preheat confirmation lost on a menu edit: %s", dump(got))
	}
	// Taking the dish off the menu takes its preheat's progress with it.
	if _, err := s.UpdateMeal(ctx, ann, m.ID, m.Version, MealPatch{Items: Some([]MealItemInput{})}); err != nil {
		t.Fatal(err)
	}
	prog := map[string]taskProgress{}
	err = s.st.Read(ctx, func(tx *sql.Tx) error {
		var err error
		prog, err = loadProgress(ctx, tx, m.ID)
		return err
	})
	if err != nil || len(prog) != 0 {
		t.Errorf("progress left after the dish went: %v, %v", prog, err)
	}
}
