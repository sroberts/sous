package recipes

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/sroberts/sous/internal/identity"
)

func TestSuggestTimings(t *testing.T) {
	got := SuggestTimings([]string{
		"Preheat the oven to 375°F.",
		"Chop the onions.",
		"Sauté the onions over medium heat for 8 to 10 minutes.",
		"Bake for 1 hour 15 minutes, until golden.",
		"Roast at 200°C for 45 minutes.",
		"Let rest for half an hour.",
		"Refrigerate overnight.",
		"The sauce can be made up to 3 days ahead; cover and refrigerate.",
		"Cook in the slow cooker on low for 6 hours.",
		"Simmer 10 minutes, then add the beans and simmer 5 minutes more.",
	})
	want := []StepTiming{
		{Minutes: ref(0), HandsOn: ref(false)},
		{},
		{Minutes: ref(10), Equipment: ref(EquipStovetop)},
		{Minutes: ref(75), HandsOn: ref(false), Equipment: ref(EquipOven), OvenTemp: ref(375)},
		{Minutes: ref(45), HandsOn: ref(false), Equipment: ref(EquipOven), OvenTemp: ref(390)},
		{Minutes: ref(30), HandsOn: ref(false)},
		{Minutes: ref(480), HandsOn: ref(false)},
		{HoldMinutes: ref(3 * 24 * 60), HoldIn: ref(HoldFridge)},
		{Minutes: ref(360), HandsOn: ref(false), Equipment: ref("slow cooker")},
		{Minutes: ref(15), HandsOn: ref(false), Equipment: ref(EquipStovetop)},
	}
	for i := range want {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Errorf("step %d:\n got  %s\n want %s", i, dump(got[i]), dump(want[i]))
		}
	}
}

func TestStepTimingNormalise(t *testing.T) {
	for _, c := range []struct {
		in   StepTiming
		code string
	}{
		{StepTiming{Minutes: ref(-1)}, CodeInvalid},
		{StepTiming{OvenTemp: ref(350)}, CodeInvalid}, // no oven
		{StepTiming{Equipment: ref(" Oven "), OvenTemp: ref(350)}, ""},
		{StepTiming{Equipment: ref("oven"), OvenTemp: ref(5000)}, CodeInvalid},
		{StepTiming{HoldIn: ref("fridge")}, CodeInvalid}, // no hold_minutes
		{StepTiming{HoldMinutes: ref(60), HoldIn: ref("cellar")}, CodeInvalid},
		{StepTiming{ReheatMinutes: ref(20)}, CodeInvalid},
		{StepTiming{HoldMinutes: ref(60), HoldIn: ref("FRIDGE"), ReheatMinutes: ref(20)}, ""},
		{StepTiming{Space: ref(0)}, CodeInvalid},
	} {
		in := c.in
		err := in.normalise("step")
		var e *Error
		switch {
		case c.code == "" && err != nil:
			t.Errorf("%s: %v", dump(c.in), err)
		case c.code != "" && (!errors.As(err, &e) || e.Code != c.code):
			t.Errorf("%s: err = %v, want %s", dump(c.in), err, c.code)
		}
	}
	tm := StepTiming{Equipment: ref("  Slow   Cooker "), HoldMinutes: ref(5), HoldIn: ref("Fridge")}
	if err := tm.normalise("x"); err != nil || *tm.Equipment != "slow cooker" || *tm.HoldIn != HoldFridge {
		t.Errorf("normalised = %s, %v", dump(tm), err)
	}
}

func TestRecipeStepTiming(t *testing.T) {
	s := newTestService(t)
	ann := person(t, s, "Ann")
	r := create(t, s, ann, RecipeInput{
		Title: "Gravy",
		Steps: []string{"# Base", "Make a roux.", "Whisk in stock and simmer."},
		StepTiming: []*StepTiming{
			{Minutes: ref(10), Equipment: ref("Stovetop")},
			nil,
		},
	})
	if r.Steps[0].Timing == nil || *r.Steps[0].Timing.Equipment != EquipStovetop || r.Steps[1].Timing != nil {
		t.Fatalf("steps = %s", dump(r.Steps))
	}

	// Editing the words of one step keeps the timing on the others.
	r, err := s.Update(ctx, ann, r.ID, r.Version, RecipePatch{Steps: Some([]string{"Make a roux.", "Whisk in hot stock; simmer."})})
	if err != nil {
		t.Fatal(err)
	}
	if r.Steps[0].Timing == nil || *r.Steps[0].Timing.Minutes != 10 {
		t.Errorf("timing lost on an unchanged step: %s", dump(r.Steps))
	}

	// Timing alone goes on the steps as they are, and reads as "timing".
	r, err = s.Update(ctx, ann, r.ID, r.Version, RecipePatch{StepTiming: Some([]*StepTiming{nil, {Minutes: ref(15), HandsOn: ref(false)}})})
	if err != nil {
		t.Fatal(err)
	}
	if r.Steps[0].Timing != nil || r.Steps[1].Timing == nil || *r.Steps[1].Timing.Minutes != 15 {
		t.Errorf("steps = %s", dump(r.Steps))
	}
	page, err := s.Changes(ctx, ann, ChangeQuery{EntityID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got := page.Items[0].Summary; got != "Edited Gravy: timing" {
		t.Errorf("summary = %q", got)
	}

	// Null clears it; too many entries is invalid.
	r, err = s.Update(ctx, ann, r.ID, r.Version, RecipePatch{StepTiming: Null[[]*StepTiming]()})
	if err != nil || r.Steps[1].Timing != nil {
		t.Errorf("cleared = %s, %v", dump(r.Steps), err)
	}
	_, err = s.Update(ctx, ann, r.ID, r.Version, RecipePatch{StepTiming: Some([]*StepTiming{nil, nil, nil})})
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeInvalid {
		t.Errorf("err = %v, want invalid", err)
	}
}

func TestKitchen(t *testing.T) {
	s := newTestService(t)
	ann := person(t, s, "Ann")
	k, err := s.GetKitchen(ctx, identity.Caller{})
	if err != nil {
		t.Fatal(err)
	}
	if k.Version != 1 || len(k.Ovens) != 1 || k.Burners != 4 || k.UpdatedBy != nil || k.Dishes == nil {
		t.Fatalf("default kitchen = %s", dump(k))
	}
	if _, err := s.UpdateKitchen(ctx, identity.Caller{}, 1, k.KitchenProfile); !isCode(err, CodeWhoAreYou) {
		t.Errorf("nobody: %v", err)
	}
	p := k.KitchenProfile
	p.Ovens = append(p.Ovens, Oven{Racks: 1})
	p.Appliances = []Appliance{{Name: " Slow Cooker ", Count: 1}, {Name: ""}}
	p.Dishes = []Dish{{Name: "Platter", Count: 2}}
	k2, err := s.UpdateKitchen(ctx, ann, 1, p)
	if err != nil {
		t.Fatal(err)
	}
	if k2.Version != 2 || k2.Ovens[1].Name != "Oven 2" || len(k2.Appliances) != 1 || k2.Appliances[0].Name != "slow cooker" ||
		k2.UpdatedBy == nil || *k2.UpdatedBy != ann.Username() {
		t.Errorf("kitchen = %s", dump(k2))
	}
	_, err = s.UpdateKitchen(ctx, ann, 1, p)
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeStale || e.CurrentKitchen == nil || e.CurrentKitchen.Version != 2 {
		t.Errorf("stale: %v", err)
	}
	p.DayEnd = "07:00"
	if _, err := s.UpdateKitchen(ctx, ann, 2, p); !isCode(err, CodeInvalid) {
		t.Errorf("day_end before day_start: %v", err)
	}
}

func TestMealLifecycle(t *testing.T) {
	s := newTestService(t)
	ann := person(t, s, "Ann")
	turkey := create(t, s, ann, RecipeInput{Title: "Turkey", Steps: []string{"Roast at 325°F for 4 hours.", "Carve."}})
	pie := create(t, s, ann, RecipeInput{Title: "Pie", Steps: []string{"Bake at 400°F for 50 minutes.", "Cool for up to 1 day ahead."}})

	if _, err := s.CreateMeal(ctx, ann, MealInput{Title: "x", ServeAt: "tomorrow", Timezone: "UTC"}); !isCode(err, CodeInvalid) {
		t.Errorf("bad serve_at: %v", err)
	}
	if _, err := s.CreateMeal(ctx, ann, MealInput{Title: "x", ServeAt: "2026-11-26T17:00:00-05:00", Timezone: "Mars/Olympus"}); !isCode(err, CodeInvalid) {
		t.Errorf("bad timezone: %v", err)
	}
	m, err := s.CreateMeal(ctx, ann, MealInput{
		Title: " Thanksgiving ", ServeAt: "2026-11-26T17:00:00-05:00", Timezone: "America/New_York",
		Items: []MealItemInput{{RecipeID: turkey.ID, Dish: ref("Platter")}, {RecipeID: pie.ID, ServeOffsetMinutes: 90}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.Title != "Thanksgiving" || m.ServeAt != "2026-11-26T22:00:00.000Z" || m.Cooks != 1 || m.PrepDays != 1 ||
		len(m.Items) != 2 || m.Items[0].Title != "Turkey" || *m.Items[0].Dish != "Platter" {
		t.Fatalf("meal = %s", dump(m))
	}
	list, err := s.ListMeals(ctx, identity.Caller{})
	if err != nil || len(list) != 1 || list[0].Items != 2 {
		t.Errorf("list = %s, %v", dump(list), err)
	}

	// Tick off a step; reordering the menu keeps it with its dish.
	turkeyItem := m.Items[0].ID
	p, err := s.UpdateTask(ctx, ann, m.ID, turkeyItem+":0", TaskPatch{Done: Some(true)})
	if err != nil {
		t.Fatal(err)
	}
	if got := taskByKey(t, p, turkeyItem+":0"); got.Status != StatusDone || *got.DoneBy != ann.Username() {
		t.Errorf("task = %s", dump(got))
	}
	if _, err := s.UpdateTask(ctx, ann, m.ID, "nope:0", TaskPatch{Done: Some(true)}); !isCode(err, CodeNotFound) {
		t.Errorf("unknown task: %v", err)
	}
	m, err = s.UpdateMeal(ctx, ann, m.ID, m.Version, MealPatch{
		Cooks: Some(2),
		Items: Some([]MealItemInput{{RecipeID: pie.ID, ID: ref(m.Items[1].ID)}, {RecipeID: turkey.ID, ID: ref(turkeyItem)}}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.Cooks != 2 || m.Items[1].ID != turkeyItem {
		t.Errorf("meal = %s", dump(m))
	}
	p, err = s.MealPlan(ctx, ann, m.ID, time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if got := taskByKey(t, p, turkeyItem+":0"); got.Status != StatusDone {
		t.Errorf("progress lost on reorder: %s", dump(got))
	}

	// Dropping the dish drops its progress; reset clears the rest.
	if _, err := s.UpdateMeal(ctx, ann, m.ID, m.Version, MealPatch{Title: Some("")}); !isCode(err, CodeInvalid) {
		t.Errorf("blank title: %v", err)
	}
	_, err = s.UpdateMeal(ctx, ann, m.ID, m.Version-1, MealPatch{Cooks: Some(3)})
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeStale || e.CurrentMeal == nil {
		t.Errorf("stale: %v", err)
	}
	p, err = s.ResetProgress(ctx, ann, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := taskByKey(t, p, turkeyItem+":0"); got.Status == StatusDone {
		t.Errorf("reset kept progress: %s", dump(got))
	}

	// Delete and restore.
	if err := s.DeleteMeal(ctx, ann, m.ID, m.Version); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListMeals(ctx, ann); len(list) != 0 {
		t.Errorf("deleted meal listed")
	}
	if _, err := s.UpdateTask(ctx, ann, m.ID, turkeyItem+":0", TaskPatch{Done: Some(true)}); !isCode(err, CodeDeleted) {
		t.Errorf("task on deleted meal: %v", err)
	}
	m, err = s.RestoreMeal(ctx, ann, m.ID)
	if err != nil || m.DeletedAt != nil {
		t.Errorf("restore = %s, %v", dump(m), err)
	}
	changes, _ := s.Changes(ctx, ann, ChangeQuery{EntityID: m.ID})
	if len(changes.Items) != 4 || changes.Items[0].Summary != "Restored the meal Thanksgiving" {
		t.Errorf("changes = %s", dump(changes.Items))
	}
}

func isCode(err error, code string) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == code
}
