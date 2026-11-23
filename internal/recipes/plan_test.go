package recipes

import (
	"strings"
	"testing"
	"time"
)

// timed is a step with timing set on the recipe.
func timed(text string, minutes int, handsOn bool, mod ...func(*StepTiming)) Step {
	t := &StepTiming{Minutes: ref(minutes), HandsOn: ref(handsOn)}
	for _, m := range mod {
		m(t)
	}
	return Step{Text: text, Timing: t}
}

func oven(temp, racks int) func(*StepTiming) {
	return func(t *StepTiming) { t.Equipment, t.OvenTemp, t.Space = ref(EquipOven), ref(temp), ref(racks) }
}

func stove(t *StepTiming) { t.Equipment = ref(EquipStovetop) }

func hold(minutes int, in string) func(*StepTiming) {
	return func(t *StepTiming) { t.HoldMinutes, t.HoldIn = ref(minutes), ref(in) }
}

var newYork, _ = time.LoadLocation("America/New_York")

// thanksgiving is five dishes for one oven with two racks, served at 5pm.
func thanksgiving() planInput {
	recipes := []Recipe{
		{ID: "turkey", Title: "Roast turkey", Steps: []Step{
			timed("Pat the turkey dry and season it.", 20, true),
			timed("Roast at 325°F.", 240, false, oven(325, 2)),
			timed("Rest under foil.", 30, false),
			timed("Carve.", 15, true),
		}},
		{ID: "stuffing", Title: "Stuffing", Steps: []Step{
			timed("Toss the bread with the vegetables and stock.", 30, true),
			timed("Bake at 375°F until crisp.", 45, false, oven(375, 1)),
		}},
		{ID: "cranberry", Title: "Cranberry sauce", Steps: []Step{
			timed("Simmer the cranberries with sugar.", 15, false, stove),
			timed("Cool, then refrigerate.", 60, false, hold(3*24*60, HoldFridge)),
		}},
		{ID: "pie", Title: "Pumpkin pie", Steps: []Step{
			timed("Make and roll out the crust.", 30, true),
			timed("Bake at 400°F.", 50, false, oven(400, 1)),
			timed("Cool on a rack.", 120, false, hold(24*60, HoldCounter)),
		}},
		{ID: "potatoes", Title: "Mashed potatoes", Steps: []Step{
			timed("Peel and cut the potatoes.", 15, true),
			timed("Boil until tender.", 25, false, stove),
			timed("Mash with butter and milk.", 10, true),
		}},
	}
	m := Meal{ID: "m1", Title: "Thanksgiving", ServeAt: "2026-11-26T22:00:00.000Z", Timezone: "America/New_York",
		Cooks: 1, PrepDays: 1, Version: 3}
	for i, r := range recipes {
		m.Items = append(m.Items, MealItem{ID: "i" + r.ID, RecipeID: r.ID, Title: r.Title})
		if r.ID == "pie" {
			m.Items[i].ServeOffsetMinutes = 90
		}
	}
	return planInput{
		meal:    m,
		recipes: recipes,
		kitchen: KitchenProfile{Ovens: []Oven{{Name: "Oven", Racks: 2}}, Burners: 4, DayStart: "08:00", DayEnd: "22:00"},
		now:     time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC),
	}
}

func mustPlan(t *testing.T, in planInput) Plan {
	t.Helper()
	p, err := buildPlan(in)
	if err != nil {
		t.Fatal(err)
	}
	checkInvariants(t, in, p)
	return p
}

func at(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// checkInvariants: no oven holds two temperatures at once or more dishes
// than racks, no more hands-on tasks run at once than there are cooks, no
// more burners are used than exist, and each dish's steps run in order.
func checkInvariants(t *testing.T, in planInput, p Plan) {
	t.Helper()
	type span struct {
		s, e time.Time
		pt   PlanTask
	}
	var spans []span
	for _, pt := range p.Tasks {
		if pt.Kind == KindPreheat {
			continue
		}
		spans = append(spans, span{at(t, pt.Start), at(t, pt.End), pt})
	}
	space := func(pt PlanTask) int {
		for i, it := range in.meal.Items {
			if pt.ItemID != nil && *pt.ItemID == it.ID && pt.Step != nil {
				if tm := in.recipes[i].Steps[*pt.Step-1].Timing; tm != nil && tm.Space != nil {
					return *tm.Space
				}
			}
		}
		return 1
	}
	for _, a := range spans {
		hands, burners, racks := 0, 0, 0
		for _, b := range spans {
			if !(b.s.Compare(a.s) <= 0 && b.e.After(a.s)) || b.pt.Minutes == 0 {
				continue
			}
			if b.pt.HandsOn {
				hands++
			}
			if b.pt.Equipment != nil && *b.pt.Equipment == EquipStovetop {
				burners++
			}
			if b.pt.Oven != nil && a.pt.Oven != nil && *b.pt.Oven == *a.pt.Oven {
				racks += space(b.pt)
				if a.pt.OvenTemp != nil && b.pt.OvenTemp != nil && abs(*a.pt.OvenTemp-*b.pt.OvenTemp) > tempTolerance {
					t.Errorf("%s at %d°F and %s at %d°F share the oven at %s", a.pt.Key, *a.pt.OvenTemp, b.pt.Key, *b.pt.OvenTemp, a.s)
				}
			}
		}
		if hands > in.meal.Cooks {
			t.Errorf("%d hands-on tasks at %s, with %d cooks", hands, a.s, in.meal.Cooks)
		}
		if burners > in.kitchen.Burners {
			t.Errorf("%d burners in use at %s", burners, a.s)
		}
		if a.pt.Oven != nil && racks > in.kitchen.Ovens[0].Racks {
			t.Errorf("%d racks in use at %s", racks, a.s)
		}
	}
	end := map[string]time.Time{}
	for _, sp := range spans {
		if sp.pt.ItemID == nil {
			continue
		}
		if prev, ok := end[*sp.pt.ItemID]; ok && sp.s.Before(prev) && sp.pt.Status != StatusDone {
			t.Errorf("%s starts at %s, before the step before it ends at %s", sp.pt.Key, sp.s, prev)
		}
		end[*sp.pt.ItemID] = sp.e
	}
}

func taskByKey(t *testing.T, p Plan, key string) PlanTask {
	t.Helper()
	for _, pt := range p.Tasks {
		if pt.Key == key {
			return pt
		}
	}
	t.Fatalf("no task %s in %d tasks", key, len(p.Tasks))
	return PlanTask{}
}

func TestPlanThanksgiving(t *testing.T) {
	in := thanksgiving()
	p := mustPlan(t, in)
	serve := at(t, p.ServeAt)
	if !p.OnTime || p.BehindMinutes != 0 {
		t.Errorf("on_time = %v, behind %d; warnings %+v", p.OnTime, p.BehindMinutes, p.Warnings)
	}

	// Day-of dishes land as they're served, or just before: one cook
	// can't carve and mash at once, so one of them finishes a little early.
	for _, key := range []string{"iturkey:3", "ipotatoes:2", "istuffing:1"} {
		got := at(t, taskByKey(t, p, key).End)
		if got.After(serve) || serve.Sub(got) > 30*time.Minute {
			t.Errorf("%s ends %s, want within half an hour before %s", key, got.In(newYork), serve.In(newYork))
		}
	}

	// The turkey takes the whole oven, so the stuffing (at another
	// temperature) can't bake beside it; it bakes while the turkey rests
	// only if there's time to reheat the oven, and otherwise before.
	roast, bake := taskByKey(t, p, "iturkey:1"), taskByKey(t, p, "istuffing:1")
	if at(t, bake.Start).Before(at(t, roast.End)) && at(t, bake.End).After(at(t, roast.Start)) {
		t.Errorf("stuffing %s–%s overlaps the roast %s–%s", bake.Start, bake.End, roast.Start, roast.End)
	}

	// Make-ahead dishes move to the prep day, the day before.
	serveDay := serve.In(newYork).Day()
	for _, key := range []string{"icranberry:0", "icranberry:1", "ipie:0", "ipie:1", "ipie:2"} {
		pt := taskByKey(t, p, key)
		if !pt.MakeAhead {
			t.Errorf("%s isn't make-ahead", key)
		}
		if d := at(t, pt.Start).In(newYork).Day(); d != serveDay-1 {
			t.Errorf("%s starts on the %d, want the day before (%d)", key, d, serveDay-1)
		}
	}
	if h := taskByKey(t, p, "ipie:2").Hold; h == nil || h.In != HoldCounter {
		t.Errorf("pie hold = %+v", h)
	}
	// Hands-on work keeps to the kitchen's day.
	for _, pt := range p.Tasks {
		if !pt.HandsOn || pt.Minutes == 0 {
			continue
		}
		if h := at(t, pt.Start).In(newYork).Hour(); h < 8 || at(t, pt.End).In(newYork).Hour() > 22 {
			t.Errorf("%s is hands-on at %s", pt.Key, at(t, pt.Start).In(newYork))
		}
	}

	// Every oven task at a new temperature is preheated first.
	preheats := 0
	for _, pt := range p.Tasks {
		if pt.Kind == KindPreheat {
			preheats++
			if pt.OvenTemp == nil || !strings.HasPrefix(pt.Text, "Preheat the oven to") {
				t.Errorf("preheat = %+v", pt)
			}
		}
	}
	if preheats < 3 {
		t.Errorf("%d preheats, want one for each of 325, 375 and 400", preheats)
	}
	if len(p.Ovens) != 1 || len(p.Ovens[0].Uses) < 3 {
		t.Errorf("ovens = %+v", p.Ovens)
	}
	// To the table in time order: the pie, served later, comes last.
	if len(p.Serving) != 5 || p.Serving[4].RecipeTitle != "Pumpkin pie" || p.Serving[4].ServeAt != "2026-11-26T23:30:00.000Z" {
		t.Errorf("serving = %+v", p.Serving)
	}
}

func TestPlanTwoCooksShorterDay(t *testing.T) {
	one := mustPlan(t, thanksgiving())
	in := thanksgiving()
	in.meal.Cooks = 2
	two := mustPlan(t, in)
	if !two.OnTime {
		t.Fatalf("not on time with two cooks: %+v", two.Warnings)
	}
	// Two pairs of hands never need to start earlier than one.
	if at(t, *two.Start).Before(at(t, *one.Start)) {
		t.Errorf("two cooks start %s, before one cook's %s", *two.Start, *one.Start)
	}
	// And they can carve and mash at once, so both land as served.
	serve := at(t, two.ServeAt)
	for _, key := range []string{"iturkey:3", "ipotatoes:2"} {
		if got := at(t, taskByKey(t, two, key).End); !got.Equal(serve) {
			t.Errorf("two cooks: %s ends %s, want %s", key, got, serve)
		}
	}
}

func TestPlanFallingBehind(t *testing.T) {
	in := thanksgiving()
	// 1:30pm on the day, and nothing has been started: the turkey should
	// have gone in at about 11.
	in.now = time.Date(2026, 11, 26, 18, 30, 0, 0, time.UTC)
	p := mustPlan(t, in)
	if p.OnTime || p.BehindMinutes < 60 {
		t.Fatalf("on_time %v, behind %d; want well behind", p.OnTime, p.BehindMinutes)
	}
	roast := taskByKey(t, p, "iturkey:1")
	if roast.Status != StatusLate || at(t, roast.Start).Before(in.now) {
		t.Errorf("roast = %s from %s, want late and not before now", roast.Status, roast.Start)
	}
	// The carving moved only because the roast did; it isn't late.
	if carve := taskByKey(t, p, "iturkey:3"); carve.Status != StatusPending {
		t.Errorf("carve = %s, want pending", carve.Status)
	}
	found := false
	for _, w := range p.Warnings {
		found = found || w.Code == "behind"
	}
	if !found {
		t.Errorf("no behind warning in %+v", p.Warnings)
	}
}

func TestPlanProgressHoldsItsPlace(t *testing.T) {
	in := thanksgiving()
	in.now = time.Date(2026, 11, 26, 16, 0, 0, 0, time.UTC) // 11am
	started := time.Date(2026, 11, 26, 15, 10, 0, 0, time.UTC)
	done := time.Date(2026, 11, 26, 15, 5, 0, 0, time.UTC)
	in.progress = map[string]taskProgress{
		"iturkey:0": {started: ref(done.Add(-20 * time.Minute)), done: &done, doneBy: ref("ann")},
		"iturkey:1": {started: &started},
	}
	p := mustPlan(t, in)
	roast := taskByKey(t, p, "iturkey:1")
	if roast.Status != StatusActive || !at(t, roast.Start).Equal(started) || roast.StartedAt == nil {
		t.Errorf("roast = %+v", roast)
	}
	prep := taskByKey(t, p, "iturkey:0")
	if prep.Status != StatusDone || prep.DoneBy == nil || *prep.DoneBy != "ann" {
		t.Errorf("prep = %+v", prep)
	}
	// Started 10:10 for 4 hours, it rests and is carved by 2:55, well
	// before 5: the carving waits for serving rather than the roast moving.
	if carve := taskByKey(t, p, "iturkey:3"); at(t, carve.End).After(at(t, p.ServeAt)) {
		t.Errorf("carve ends %s, after serving", carve.End)
	}
}

func TestPlanPinned(t *testing.T) {
	in := thanksgiving()
	pin := time.Date(2026, 11, 26, 14, 0, 0, 0, time.UTC) // 9am
	in.progress = map[string]taskProgress{"ipotatoes:0": {pinned: &pin}}
	p := mustPlan(t, in)
	peel := taskByKey(t, p, "ipotatoes:0")
	if !peel.Pinned || !at(t, peel.Start).Equal(pin) {
		t.Errorf("peel = %+v", peel)
	}
}

func TestPlanReheatAndGuesses(t *testing.T) {
	in := thanksgiving()
	in.recipes = append(in.recipes, Recipe{ID: "gratin", Title: "Gratin", Steps: []Step{
		{Text: "Slice the potatoes thinly and layer with cream."},
		{Text: "Bake at 350°F for 1 hour, then cool and refrigerate for up to 2 days.",
			Timing: &StepTiming{Minutes: ref(60), HandsOn: ref(false), Equipment: ref(EquipOven), OvenTemp: ref(350),
				HoldMinutes: ref(2 * 24 * 60), HoldIn: ref(HoldFridge), ReheatMinutes: ref(30), ReheatTemp: ref(350)}},
	}})
	in.meal.Items = append(in.meal.Items, MealItem{ID: "igratin", RecipeID: "gratin", Title: "Gratin"})
	p := mustPlan(t, in)
	rh := taskByKey(t, p, "igratin:reheat")
	if rh.Kind != KindReheat || rh.OvenTemp == nil || *rh.OvenTemp != 350 || !at(t, rh.End).Equal(at(t, p.ServeAt)) {
		t.Errorf("reheat = %+v", rh)
	}
	slice := taskByKey(t, p, "igratin:0")
	if !slice.Estimated || slice.Minutes != defaultStep {
		t.Errorf("slice = %+v, want an estimated %d minutes", slice, defaultStep)
	}
	found := false
	for _, w := range p.Warnings {
		found = found || (w.Code == "estimated" && w.ItemID != nil && *w.ItemID == "igratin")
	}
	if !found {
		t.Errorf("no estimated warning: %+v", p.Warnings)
	}
}

func TestPlanDishes(t *testing.T) {
	in := thanksgiving()
	in.kitchen.Dishes = []Dish{{Name: "Big platter", Count: 1}, {Name: "Bowl", Count: 3}}
	in.meal.Items[0].Dish = ref("big platter")
	in.meal.Items[1].Dish = ref("Big platter")
	in.meal.Items[2].Dish = ref("Gravy boat")
	p := mustPlan(t, in)
	codes := map[string]int{}
	for _, w := range p.Warnings {
		codes[w.Code]++
	}
	if codes["dish_short"] != 1 || codes["dish_unknown"] != 1 || codes["no_dish"] != 2 {
		t.Errorf("warnings = %+v", p.Warnings)
	}
}

func TestPlanUnknownApplianceAndNoSteps(t *testing.T) {
	in := thanksgiving()
	in.recipes[4].Steps[1] = timed("Cook in the slow cooker.", 240, false, func(t *StepTiming) { t.Equipment = ref("slow cooker") })
	in.recipes = append(in.recipes, Recipe{ID: "rolls", Title: "Rolls"})
	in.meal.Items = append(in.meal.Items, MealItem{ID: "irolls", RecipeID: "rolls"})
	p := mustPlan(t, in)
	codes := map[string]int{}
	for _, w := range p.Warnings {
		codes[w.Code]++
	}
	if codes["unknown_appliance"] != 1 || codes["no_steps"] != 1 {
		t.Errorf("warnings = %+v", p.Warnings)
	}
	in.kitchen.Appliances = []Appliance{{Name: "slow cooker", Count: 1}}
	in.recipes[1].Steps[1] = timed("Keep warm in the slow cooker.", 240, false, func(t *StepTiming) { t.Equipment = ref("slow cooker") })
	p = mustPlan(t, in)
	a, b := taskByKey(t, p, "ipotatoes:1"), taskByKey(t, p, "istuffing:1")
	if at(t, a.Start).Before(at(t, b.End)) && at(t, b.Start).Before(at(t, a.End)) {
		t.Errorf("one slow cooker used twice at once: %s–%s and %s–%s", a.Start, a.End, b.Start, b.End)
	}
}

func TestPlanEmptyMeal(t *testing.T) {
	in := thanksgiving()
	in.meal.Items, in.recipes = nil, nil
	p := mustPlan(t, in)
	if p.Start != nil || len(p.Tasks) != 0 || !p.OnTime || p.Warnings == nil || p.Serving == nil {
		t.Errorf("plan = %+v", p)
	}
}

func TestPlanMissedMakeAhead(t *testing.T) {
	in := thanksgiving()
	// Noon on the day: the pie and cranberry sauce were for yesterday.
	in.now = time.Date(2026, 11, 26, 17, 0, 0, 0, time.UTC)
	p := mustPlan(t, in)
	for _, key := range []string{"ipie:0", "icranberry:0"} {
		if got := taskByKey(t, p, key).Status; got != StatusMissed {
			t.Errorf("%s = %s, want missed", key, got)
		}
	}
	// Day-of work that should have started is late, not missed.
	prep := taskByKey(t, p, "iturkey:0")
	if prep.Status != StatusLate {
		t.Errorf("turkey prep = %s, want late", prep.Status)
	}
	// It says when it was meant to start, before the clock moved it.
	if prep.PlannedFor == nil || !at(t, *prep.PlannedFor).Before(at(t, prep.Start)) {
		t.Errorf("turkey prep planned_for = %v, start %s", prep.PlannedFor, prep.Start)
	}
	if len(p.LateDishes) == 0 || p.Warnings[0].Code != "behind" {
		t.Fatalf("late dishes %+v, first warning %+v", p.LateDishes, p.Warnings[0])
	}
	causes := map[string]string{}
	for _, d := range p.LateDishes {
		causes[d.ItemID] = d.Cause
	}
	if causes["ipie"] != "missed" {
		t.Errorf("pie cause = %q, want missed (%+v)", causes["ipie"], p.LateDishes)
	}
	if causes["iturkey"] != "late_start" {
		t.Errorf("turkey cause = %q, want late_start", causes["iturkey"])
	}
	if p.LateDishes[0].Minutes < p.LateDishes[len(p.LateDishes)-1].Minutes {
		t.Errorf("late dishes not latest first: %+v", p.LateDishes)
	}
}

func TestPlanAppetiserBeforeTheMeal(t *testing.T) {
	in := thanksgiving()
	in.meal.Items[2].ServeOffsetMinutes = -60
	p := mustPlan(t, in)
	if p.Serving[0].RecipeTitle != "Cranberry sauce" || p.Serving[0].ServeAt != "2026-11-26T21:00:00.000Z" {
		t.Errorf("serving = %+v", p.Serving)
	}
}

func TestHumanMinutes(t *testing.T) {
	for m, want := range map[int]string{45: "45 min", 60: "1 hr", 120: "2 hr", 100: "1 hr 40 min"} {
		if got := humanMinutes(m); got != want {
			t.Errorf("humanMinutes(%d) = %q, want %q", m, got, want)
		}
	}
}

// Five hours before serving with the prep day missed: the turkey, whose
// four-hour roast is the meal's critical path, gets the oven before the
// pie, which is served later and can wait.
func TestPlanUrgentDishFirst(t *testing.T) {
	in := thanksgiving()
	in.now = time.Date(2026, 11, 26, 17, 0, 0, 0, time.UTC) // noon
	p := mustPlan(t, in)
	roast, bake := taskByKey(t, p, "iturkey:1"), taskByKey(t, p, "ipie:1")
	if !at(t, roast.Start).Before(at(t, bake.Start)) {
		t.Errorf("roast starts %s, after the pie bakes at %s", roast.Start, bake.Start)
	}
	var turkey LateDish
	for _, d := range p.LateDishes {
		if d.ItemID == "iturkey" {
			turkey = d
		}
	}
	// Started at noon it needs 4h55m for a 5pm meal: under an hour late.
	if turkey.Minutes > 60 {
		t.Errorf("turkey late by %d minutes, want under an hour (%+v)", turkey.Minutes, p.LateDishes)
	}
	// Nothing is planned before now but what's already started or done.
	for _, task := range p.Tasks {
		if task.Kind != KindPreheat && at(t, task.Start).Before(in.now.Add(-time.Minute)) && task.Status != StatusDone && task.Status != StatusActive {
			t.Errorf("%s planned at %s, before now", task.Key, task.Start)
		}
	}
}
