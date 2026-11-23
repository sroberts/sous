package recipes

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sroberts/sous/internal/identity"
)

// Plan is a meal laid out in time: every step of every dish, when it
// starts, what it occupies, and what can't be made to fit.
type Plan struct {
	MealID        string        `json:"meal_id"`
	MealVersion   int           `json:"meal_version"`
	ServeAt       string        `json:"serve_at"`
	Now           string        `json:"now" jsonschema:"the time the plan was made for"`
	PrepFrom      string        `json:"prep_from" jsonschema:"the start of the prep window: prep_days before the meal, at the start of the kitchen's day"`
	Start         *string       `json:"start" jsonschema:"when the first task starts; null for an empty menu"`
	OnTime        bool          `json:"on_time" jsonschema:"false when, as things stand, a dish can't be ready when it's served"`
	BehindMinutes int           `json:"behind_minutes" jsonschema:"how late the latest dish would be; 0 when on time"`
	LateDishes    []LateDish    `json:"late_dishes" jsonschema:"each dish that won't be ready when it's served, latest first, and why"`
	Tasks         []PlanTask    `json:"tasks" jsonschema:"in order of start"`
	Ovens         []OvenUse     `json:"ovens"`
	Serving       []ServingDish `json:"serving"`
	Warnings      []PlanWarning `json:"warnings"`
}

// PlanTask is one thing to do: a recipe step, a reheat, or a preheat.
type PlanTask struct {
	Key         string    `json:"key" jsonschema:"\"<item id>:<step index>\", \"<item id>:reheat\", or for a preheat \"preheat:...\"; pass it to update a task"`
	Kind        string    `json:"kind" jsonschema:"step, reheat or preheat"`
	ItemID      *string   `json:"item_id"`
	RecipeID    *string   `json:"recipe_id"`
	RecipeTitle *string   `json:"recipe_title"`
	Step        *int      `json:"step" jsonschema:"the step's number in its recipe, from 1"`
	Text        string    `json:"text"`
	Start       string    `json:"start"`
	End         string    `json:"end"`
	Minutes     int       `json:"minutes"`
	HandsOn     bool      `json:"hands_on"`
	Equipment   *string   `json:"equipment"`
	Oven        *string   `json:"oven" jsonschema:"which oven, by name, for an oven task"`
	OvenTemp    *int      `json:"oven_temp"`
	MakeAhead   bool      `json:"make_ahead" jsonschema:"part of a dish made ahead and held"`
	Hold        *TaskHold `json:"hold" jsonschema:"after this task the dish waits: where, and until when"`
	Estimated   bool      `json:"estimated" jsonschema:"its time was guessed from the step's words, not set on the recipe"`
	Pinned      bool      `json:"pinned"`
	Status      string    `json:"status" jsonschema:"pending, active (started), done, late (should have started by now), or missed (make-ahead work whose time has passed; it is replanned from now)"`
	StartedAt   *string   `json:"started_at"`
	DoneAt      *string   `json:"done_at"`
	DoneBy      *string   `json:"done_by"`
}

// LateDish is a dish that won't be ready in time.
type LateDish struct {
	ItemID      string `json:"item_id"`
	RecipeTitle string `json:"recipe_title"`
	ServeAt     string `json:"serve_at" jsonschema:"when it is meant to be served"`
	ReadyAt     string `json:"ready_at" jsonschema:"when, as things stand, it will be ready"`
	Minutes     int    `json:"minutes" jsonschema:"how late it will be"`
	Cause       string `json:"cause" jsonschema:"missed (its make-ahead work's time passed), late_start (work that should have started hasn't), or busy (the kitchen has no room for it sooner)"`
	Task        string `json:"task" jsonschema:"the task most to blame"`
}

// TaskHold is where a made-ahead dish waits after its last make-ahead step.
type TaskHold struct {
	In    string `json:"in" jsonschema:"fridge, counter or warm"`
	Until string `json:"until"`
}

// OvenUse is one oven's timeline.
type OvenUse struct {
	Name  string     `json:"name"`
	Racks int        `json:"racks"`
	Uses  []OvenSpan `json:"uses"`
}

// OvenSpan is a stretch of time an oven holds one temperature.
type OvenSpan struct {
	Start   string   `json:"start"`
	End     string   `json:"end"`
	Temp    *int     `json:"temp" jsonschema:"°F; the lowest, when dishes within 25°F share it"`
	TempMax *int     `json:"temp_max" jsonschema:"°F, the highest, when dishes at different temperatures share it; null otherwise"`
	Tasks   []string `json:"tasks" jsonschema:"task keys"`
}

// ServingDish is a dish going to the table.
type ServingDish struct {
	ItemID      string  `json:"item_id"`
	RecipeTitle string  `json:"recipe_title"`
	Dish        *string `json:"dish"`
	ServeAt     string  `json:"serve_at"`
}

// PlanWarning is something the cook should know about the plan.
type PlanWarning struct {
	Code    string  `json:"code" jsonschema:"estimated, no_steps, in_trash, conflict, waits, early, no_oven, no_burners, unknown_appliance, no_dish, dish_unknown, dish_short or behind"`
	Message string  `json:"message"`
	Task    *string `json:"task"`
	ItemID  *string `json:"item_id"`
}

// Task kinds and statuses.
const (
	KindStep    = "step"
	KindReheat  = "reheat"
	KindPreheat = "preheat"

	StatusPending = "pending"
	StatusActive  = "active"
	StatusDone    = "done"
	StatusLate    = "late"
	StatusMissed  = "missed"
)

const (
	gridMin       = 5   // tasks start on a five-minute grid
	tempTolerance = 25  // °F: dishes this close share an oven
	preheatMin    = 15  // minutes an oven takes to change temperature
	defaultStep   = 5   // minutes for a step whose time can't be guessed
	waitWarnMin   = 30  // a cooked step waiting longer than this is flagged
	lookBackDays  = 7   // how far before the prep window a task may be pushed
	liveAfterMin  = 360 // minutes after serving that a plan stops being live
)

// MealPlan lays out the meal as of now.
func (s *Service) MealPlan(ctx context.Context, c identity.Caller, id string, now time.Time) (Plan, error) {
	var in planInput
	err := s.st.Read(ctx, func(tx *sql.Tx) error {
		m, err := loadMeal(ctx, tx, id)
		if err != nil {
			return err
		}
		in.meal = m
		for _, it := range m.Items {
			r, err := loadRecipe(ctx, tx, it.RecipeID, "")
			if err != nil {
				return err
			}
			in.recipes = append(in.recipes, r)
		}
		k, err := loadKitchen(ctx, tx)
		if err != nil {
			return err
		}
		in.kitchen = k.KitchenProfile
		in.progress, err = loadProgress(ctx, tx, id)
		return err
	})
	if err != nil {
		return Plan{}, err
	}
	in.now = now
	return buildPlan(in)
}

// taskProgress is a meal_tasks row.
type taskProgress struct {
	pinned, started, done *time.Time
	doneBy                *string
}

func loadProgress(ctx context.Context, tx *sql.Tx, mealID string) (map[string]taskProgress, error) {
	rows, err := tx.QueryContext(ctx, `SELECT task, pinned_start, started_at, done_at, done_by FROM meal_tasks WHERE meal_id = ?`, mealID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]taskProgress{}
	for rows.Next() {
		var key string
		var pinned, started, done *string
		var p taskProgress
		if err := rows.Scan(&key, &pinned, &started, &done, &p.doneBy); err != nil {
			return nil, err
		}
		p.pinned, p.started, p.done = parseStamp(pinned), parseStamp(started), parseStamp(done)
		out[key] = p
	}
	return out, rows.Err()
}

func parseStamp(s *string) *time.Time {
	if s == nil {
		return nil
	}
	t, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		return nil
	}
	return &t
}

// planInput is everything buildPlan needs; recipes[i] is meal.Items[i]'s.
type planInput struct {
	meal     Meal
	recipes  []Recipe
	kitchen  KitchenProfile
	progress map[string]taskProgress
	now      time.Time
}

// ptask is a task while it is being placed. Times are minutes from the
// planner's origin.
type ptask struct {
	key, kind string
	item      int // index into the menu; -1 for a preheat
	step      int // index into the recipe's steps; -1 for a reheat
	text      string
	dur       int
	handsOn   bool
	equip     string
	temp      int
	space     int
	estimated bool
	makeAhead bool
	holdMax   int
	holdIn    string
	prev      *ptask

	fixed, pinned, late, missed bool
	ideal                       int // where the plan made without the clock put it
	prog                        taskProgress
	start, end                  int
	oven                        int
	placed                      bool
}

type planner struct {
	in       planInput
	loc      *time.Location
	origin   time.Time
	dayStart int
	dayEnd   int
	appCap   map[string]int
	placed   []*ptask
	lf       map[*ptask]int // each day-of task's latest finish, in the backward pass
	floor    int            // the earliest a task may be placed: now, once live
	stuck    map[int]bool   // dishes with a task that fitted nowhere, in this pass
	warnings []PlanWarning
	warned   map[string]bool
}

func (p *planner) at(m int) time.Time { return p.origin.Add(time.Duration(m) * time.Minute) }

func (p *planner) minutes(t time.Time) int { return int(math.Floor(t.Sub(p.origin).Minutes())) }

func (p *planner) stamp(m int) string { return p.at(m).UTC().Format(timeFormat) }

func (p *planner) warn(code, msg string, task *ptask, once string) {
	if once != "" {
		if p.warned[once] {
			return
		}
		p.warned[once] = true
	}
	w := PlanWarning{Code: code, Message: msg}
	if task != nil {
		if task.kind != KindPreheat {
			w.Task = ref(task.key)
		}
		if task.item >= 0 {
			w.ItemID = ref(p.in.meal.Items[task.item].ID)
		}
	}
	p.warnings = append(p.warnings, w)
}

// buildPlan is the scheduler. Day-of work is placed backwards from each
// dish's serving time, as late as the kitchen allows, so food is hot when
// it lands; make-ahead work is placed forwards from the start of the prep
// window, so it is out of the way before the day. Both respect the cooks'
// hands, each oven's racks and single temperature, the burners and the
// appliances, and keep hands-on work inside the kitchen's day when they
// can. Tasks already started, done or pinned stay where they are, and once
// the plan is live, anything that should have started already is moved
// to now and the dishes behind it slip.
func buildPlan(in planInput) (Plan, error) {
	loc, err := time.LoadLocation(in.meal.Timezone)
	if err != nil {
		return Plan{}, err
	}
	serve, err := time.Parse(time.RFC3339, in.meal.ServeAt)
	if err != nil {
		return Plan{}, err
	}
	k := in.kitchen
	k.fillDefaults()
	in.kitchen = k
	p := &planner{in: in, loc: loc, appCap: map[string]int{}, lf: map[*ptask]int{}, warned: map[string]bool{}}
	p.dayStart, _ = clockMinutes(k.DayStart)
	p.dayEnd, _ = clockMinutes(k.DayEnd)
	if p.dayEnd <= p.dayStart {
		p.dayStart, p.dayEnd = 8*60, 22*60
	}
	for _, a := range k.Appliances {
		p.appCap[a.Name] += a.Count
	}

	// The prep window opens prep_days before the meal at the start of the
	// day, and never later than three hours before serving.
	ls := serve.In(loc)
	prepFrom := time.Date(ls.Year(), ls.Month(), ls.Day()-in.meal.PrepDays, 0, 0, 0, 0, loc).
		Add(time.Duration(p.dayStart) * time.Minute)
	if latest := serve.Add(-3 * time.Hour); prepFrom.After(latest) {
		prepFrom = latest.Truncate(gridMin * time.Minute)
	}
	p.origin = prepFrom.AddDate(0, 0, -lookBackDays)
	prepMin := p.minutes(prepFrom)

	tasks, dayOf, ahead, deadlines := p.buildTasks(serve)

	// Fixed tasks first: they hold their place whatever else happens.
	nowMin := p.minutes(in.now)
	for _, t := range tasks {
		p.fix(t, nowMin)
	}
	base := slices.Clone(p.placed)
	baseWarnings := len(p.warnings)

	// The plan as it should be: nothing in it is held back by the clock.
	none := make([]bool, len(dayOf))
	p.pass(tasks, dayOf, ahead, deadlines, prepMin, none, -1, base, baseWarnings)
	for _, t := range tasks {
		t.ideal = t.start
	}

	// Live: anything that should have started by now has to happen from now
	// on. The dishes in trouble are replanned first, most urgent first, and
	// the rest fit around them; a dish that then can't fit in time joins
	// them, until the plan holds.
	live := !in.now.Before(prepFrom.Add(-24*time.Hour)) && in.now.Before(p.at(maxDeadline(deadlines)+liveAfterMin))
	if live {
		floor := nowMin + mod(-nowMin, gridMin)
		trouble := slices.Clone(none)
		// First every dish is fitted as usual but from now on; only those
		// that then can't be done in time are in trouble.
		p.pass(tasks, dayOf, ahead, deadlines, prepMin, trouble, floor, base, baseWarnings)
		for range len(dayOf) + 1 {
			changed := false
			for _, t := range tasks {
				if !t.fixed && (t.start < nowMin || p.stuck[t.item]) && !trouble[t.item] {
					trouble[t.item], changed = true, true
				}
			}
			if !changed {
				break
			}
			p.pass(tasks, dayOf, ahead, deadlines, prepMin, trouble, floor, base, baseWarnings)
		}
		for _, t := range tasks {
			// Late means it should have started by now; a task that only
			// moves because the one before it did is just later. Make-ahead
			// work whose time has passed is missed: the cook decides whether
			// to do it now or say it's done.
			if !t.fixed && t.ideal < nowMin {
				t.late = !t.makeAhead
				t.missed = t.makeAhead
			}
		}
	}

	return p.result(serve, prepFrom, tasks, dayOf, ahead, deadlines, nowMin), nil
}

func maxDeadline(d []int) int {
	m := 0
	for _, v := range d {
		m = max(m, v)
	}
	return m
}

// buildTasks turns each dish into its tasks: a make-ahead chain (the steps
// up to the last one the dish can be held after) and a day-of chain (a
// reheat, if the hold calls for one, then the remaining steps). deadlines
// are each dish's serving time.
func (p *planner) buildTasks(serve time.Time) (all []*ptask, dayOf, ahead [][]*ptask, deadlines []int) {
	n := len(p.in.meal.Items)
	dayOf, ahead, deadlines = make([][]*ptask, n), make([][]*ptask, n), make([]int, n)
	for i, it := range p.in.meal.Items {
		r := p.in.recipes[i]
		deadlines[i] = p.minutes(serve) + it.ServeOffsetMinutes
		if r.DeletedAt != nil {
			p.warnItem("in_trash", r.Title+" is in the trash; it's still planned, but restore it to keep it.", it.ID)
		}
		if len(r.Steps) == 0 {
			p.warnItem("no_steps", r.Title+" has no steps, so there's nothing to plan for it.", it.ID)
			continue
		}
		texts := make([]string, len(r.Steps))
		for j, st := range r.Steps {
			texts[j] = st.Text
		}
		guesses := SuggestTimings(texts)
		var steps []*ptask
		guessed := 0
		hold := -1
		for j, st := range r.Steps {
			t, est := p.stepTask(it, i, j, st, guesses[j])
			if t == nil {
				continue
			}
			if est {
				guessed++
			}
			if t.holdMax > 0 {
				hold = len(steps)
			}
			steps = append(steps, t)
		}
		if guessed > 0 {
			p.warnItem("estimated", fmt.Sprintf("%s: %s no timing set, so %s guessed from the words. Set timing on the recipe for a better plan.",
				r.Title, plural(guessed, "step has", "steps have"), iffs(guessed == 1, "its time is", "their times are")), it.ID)
		}
		for j := 1; j < len(steps); j++ {
			steps[j].prev = steps[j-1]
		}
		if hold >= 0 {
			ahead[i] = steps[:hold+1]
			for _, t := range ahead[i] {
				t.makeAhead = true
			}
			rest := steps[hold+1:]
			h := steps[hold]
			if tm := r.Steps[h.step].Timing; tm != nil && tm.ReheatMinutes != nil && *tm.ReheatMinutes > 0 {
				rh := &ptask{key: it.ID + ":reheat", kind: KindReheat, item: i, step: -1, dur: *tm.ReheatMinutes, space: 1, oven: -1}
				if tm.ReheatTemp != nil {
					rh.equip, rh.temp = EquipOven, *tm.ReheatTemp
					rh.text = fmt.Sprintf("Reheat %s in the oven at %d°F", r.Title, *tm.ReheatTemp)
				} else {
					rh.text = "Bring " + r.Title + " back to temperature"
				}
				rh.prev = h
				rh.prog = p.in.progress[rh.key]
				if len(rest) > 0 {
					rest[0].prev = rh
				}
				rest = append([]*ptask{rh}, rest...)
				all = append(all, rh)
			}
			dayOf[i] = rest
		} else {
			dayOf[i] = steps
		}
		all = append(all, steps...)
	}
	return all, dayOf, ahead, deadlines
}

// stepTask builds the task for one step, from its timing with the gaps
// filled by the guess. A step the guess reads as preheating is left out:
// the planner preheats ovens itself. est reports a guessed duration.
func (p *planner) stepTask(it MealItem, i, j int, st Step, g StepTiming) (*ptask, bool) {
	tm := StepTiming{}
	if st.Timing != nil {
		tm = *st.Timing
	}
	est := false
	if tm.Minutes == nil {
		if g.Minutes != nil && *g.Minutes == 0 && preheat.MatchString(st.Text) {
			return nil, false
		}
		tm.Minutes, est = g.Minutes, true
	}
	if st.Timing == nil {
		tm.HandsOn, tm.Equipment, tm.OvenTemp = g.HandsOn, g.Equipment, g.OvenTemp
		tm.HoldMinutes, tm.HoldIn = g.HoldMinutes, g.HoldIn
	}
	t := &ptask{
		key: it.ID + ":" + strconv.Itoa(j), kind: KindStep, item: i, step: j, text: st.Text,
		dur: defaultStep, handsOn: true, space: 1, estimated: est, oven: -1,
	}
	if tm.Minutes != nil {
		t.dur = *tm.Minutes
	}
	if tm.HandsOn != nil {
		t.handsOn = *tm.HandsOn
	}
	if tm.Equipment != nil {
		t.equip = *tm.Equipment
	}
	if tm.OvenTemp != nil && t.equip == EquipOven {
		t.temp = *tm.OvenTemp
	}
	if tm.Space != nil {
		t.space = *tm.Space
	}
	if tm.HoldMinutes != nil {
		t.holdMax = *tm.HoldMinutes
		t.holdIn = HoldCounter
		if tm.HoldIn != nil {
			t.holdIn = *tm.HoldIn
		}
	}
	t.prog = p.in.progress[t.key]
	return t, est
}

// fix places a task whose time is already decided: done, started or pinned.
func (p *planner) fix(t *ptask, nowMin int) {
	pr := t.prog
	switch {
	case pr.done != nil:
		t.end = p.minutes(*pr.done)
		t.start = t.end - t.dur
		if pr.started != nil {
			t.start = p.minutes(*pr.started)
		}
	case pr.started != nil:
		t.start = p.minutes(*pr.started)
		// A task running long ends now, and the dishes behind it wait.
		t.end = max(t.start+t.dur, nowMin)
	case pr.pinned != nil:
		t.start = p.minutes(*pr.pinned)
		t.end = t.start + t.dur
		t.pinned = true
	default:
		return
	}
	t.fixed, t.placed = true, true
	t.oven, _ = p.fits(t, t.start, false)
	if t.oven < 0 && t.equip == EquipOven && len(p.in.kitchen.Ovens) > 0 {
		t.oven = 0
	}
	p.placed = append(p.placed, t)
}

func (p *planner) warnItem(code, msg, itemID string) {
	p.warnings = append(p.warnings, PlanWarning{Code: code, Message: msg, ItemID: ref(itemID)})
}

func (p *planner) title(t *ptask) string {
	if t.item < 0 {
		return "The oven"
	}
	return p.in.recipes[t.item].Title
}

// placeBack puts t as late as it fits, finishing by lf.
func (p *planner) placeBack(t *ptask, lfMin int) {
	s0 := lfMin - t.dur
	s0 -= mod(s0, gridMin)
	low := p.floor // nothing starts before now once the plan is live
	for _, day := range p.dayPasses(t) {
		for s := s0; s >= low; s -= gridMin {
			if oven, ok := p.fits(t, s, day); ok {
				p.put(t, s, oven)
				p.checkWait(t, lfMin)
				return
			}
		}
	}
	p.put(t, s0, p.firstOven(t))
	if t.item >= 0 {
		p.stuck[t.item] = true
	}
	p.warn("conflict", fmt.Sprintf("%s: nothing in the kitchen is free for “%s”; it's planned anyway, so check it.", p.title(t), short(t.text)), t, "")
}

// placeFwd puts t as early as it fits from ef, finishing by lf; if it can't
// finish in time it goes as late as it fits instead.
func (p *planner) placeFwd(t *ptask, ef, lfMin int) {
	s0 := ef + mod(-ef, gridMin)
	for _, day := range p.dayPasses(t) {
		for s := s0; s+t.dur <= lfMin; s += gridMin {
			if oven, ok := p.fits(t, s, day); ok {
				p.put(t, s, oven)
				return
			}
		}
	}
	p.placeBack(t, lfMin)
}

// dayPasses: hands-on work tries the kitchen's day first, then any hour.
func (p *planner) dayPasses(t *ptask) []bool {
	if t.handsOn && t.dur > 0 && t.dur <= p.dayEnd-p.dayStart {
		return []bool{true, false}
	}
	return []bool{false}
}

func (p *planner) put(t *ptask, s, oven int) {
	t.start, t.end, t.oven, t.placed = s, s+t.dur, oven, true
	p.placed = append(p.placed, t)
}

func (p *planner) unplace(t *ptask) {
	p.placed = slices.DeleteFunc(p.placed, func(u *ptask) bool { return u == t })
	t.placed = false
}

func (p *planner) firstOven(t *ptask) int {
	if t.equip == EquipOven && len(p.in.kitchen.Ovens) > 0 {
		return 0
	}
	return -1
}

// checkWait flags a cooked step that would sit a while before the next.
func (p *planner) checkWait(t *ptask, lfMin int) {
	if t.equip == "" || t.holdMax > 0 || t.fixed {
		return
	}
	if gap := lfMin - t.end; gap > waitWarnMin {
		p.warn("waits", fmt.Sprintf("%s would wait %s after “%s”, since the kitchen is busy just before it's needed.",
			p.title(t), humanMinutes(gap), short(t.text)), t, "")
	}
}

// fits reports whether t can start at s given everything placed, and which
// oven it would use. Kitchen limits it can't meet at all (no oven, an
// appliance the kitchen doesn't list) are warned about and ignored.
func (p *planner) fits(t *ptask, s int, day bool) (int, bool) {
	e := s + t.dur
	if t.dur <= 0 {
		return p.firstOven(t), true
	}
	if day && t.handsOn && !p.inDay(s, e) {
		return -1, false
	}
	k := p.in.kitchen
	if t.handsOn && load(p.placed, s, e, func(u *ptask) int { return b2i(u.handsOn) }) >= p.in.meal.Cooks {
		return -1, false
	}
	switch t.equip {
	case "":
		return -1, true
	case EquipStovetop:
		if k.Burners < 1 {
			p.warn("no_burners", "The kitchen has no burners listed, so stovetop steps are planned as if there were room.", nil, "burners")
			return -1, true
		}
		need := min(t.space, k.Burners)
		return -1, load(p.placed, s, e, func(u *ptask) int { return iff(u.equip == EquipStovetop, u.space) })+need <= k.Burners
	case EquipOven:
		if len(k.Ovens) == 0 {
			p.warn("no_oven", "The kitchen has no oven listed, so oven steps are planned as if there were room.", nil, "ovens")
			return -1, true
		}
		for o, oven := range k.Ovens {
			if p.ovenFree(t, o, oven, s, e) {
				return o, true
			}
		}
		return -1, false
	default:
		capacity, known := p.appCap[t.equip]
		if !known {
			p.warn("unknown_appliance", fmt.Sprintf("Steps use a %s, which isn't in the kitchen; add it there to plan around it.", t.equip), nil, "appl:"+t.equip)
			return -1, true
		}
		need := min(t.space, capacity)
		return -1, load(p.placed, s, e, func(u *ptask) int { return iff(u.equip == t.equip, u.space) })+need <= capacity
	}
}

// ovenFree: the oven has racks to spare over [s, e), and nothing in it then
// (or while it heats beforehand) is at a different temperature.
func (p *planner) ovenFree(t *ptask, o int, oven Oven, s, e int) bool {
	heat := func(u *ptask) int { return iff(u.temp > 0, preheatMin) }
	var same []*ptask
	for _, u := range p.placed {
		if u.oven != o || u.equip != EquipOven || u == t {
			continue
		}
		if u.start-heat(u) < e && s-heat(t) < u.end && u.temp > 0 && t.temp > 0 && abs(u.temp-t.temp) > tempTolerance {
			return false
		}
		same = append(same, u)
	}
	need := min(t.space, oven.Racks)
	return load(same, s, e, func(u *ptask) int { return u.space })+need <= oven.Racks
}

// inDay reports whether [s, e) lies inside one of the kitchen's days.
func (p *planner) inDay(s, e int) bool {
	start := p.at(s).In(p.loc)
	mid := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, p.loc)
	from := mid.Add(time.Duration(p.dayStart) * time.Minute)
	to := mid.Add(time.Duration(p.dayEnd) * time.Minute)
	return !start.Before(from) && !p.at(e).After(to)
}

// pass places every task that isn't fixed. Dishes marked in trouble are
// placed forwards from floor, the most urgent task first (the one whose
// latest possible start comes soonest), with no regard to the kitchen's
// day; the rest are placed as in a plan made in advance, no earlier than
// floor. floor is -1 when the clock doesn't matter.
func (p *planner) pass(tasks []*ptask, dayOf, ahead [][]*ptask, deadlines []int, prepMin int, trouble []bool, floor int, base []*ptask, baseWarnings int) {
	p.placed = slices.Clone(base)
	p.warnings = p.warnings[:baseWarnings]
	p.floor = max(floor, 0)
	p.stuck = map[int]bool{}
	clear(p.lf)
	for _, t := range tasks {
		if !t.fixed {
			t.placed = false
		}
	}

	// Dishes in trouble: one chain each, make-ahead then day-of.
	type link struct {
		t      *ptask
		latest int // the latest it can start for its dish to be on time
	}
	var queue [][]link
	for i := range dayOf {
		if !trouble[i] {
			continue
		}
		chain := append(slices.Clone(ahead[i]), dayOf[i]...)
		links := make([]link, len(chain))
		rest := 0
		for j := len(chain) - 1; j >= 0; j-- {
			rest += chain[j].dur
			links[j] = link{chain[j], deadlines[i] - rest}
		}
		queue = append(queue, links)
	}
	for {
		best := -1
		for q, links := range queue {
			if len(links) == 0 {
				continue
			}
			if best < 0 || links[0].latest < queue[best][0].latest {
				best = q
			}
		}
		if best < 0 {
			break
		}
		t := queue[best][0].t
		queue[best] = queue[best][1:]
		if t.placed {
			continue
		}
		ef := floor
		if t.prev != nil && t.prev.placed {
			ef = max(ef, t.prev.end)
		}
		p.placeFrom(t, ef)
	}

	// Day-of chains of the others, backwards from each dish's serving time.
	var ready []*ptask
	for i, chain := range dayOf {
		if len(chain) > 0 && !trouble[i] {
			last := chain[len(chain)-1]
			ready = append(ready, last)
			p.lf[last] = deadlines[i]
		}
	}
	for len(ready) > 0 {
		// The task that must finish latest goes next; on a tie, the longer.
		slices.SortStableFunc(ready, func(a, b *ptask) int {
			if c := cmp.Compare(p.lf[b], p.lf[a]); c != 0 {
				return c
			}
			return cmp.Compare(b.dur, a.dur)
		})
		t := ready[0]
		ready = ready[1:]
		if !t.placed {
			p.placeBack(t, p.lf[t])
		}
		if t.prev != nil && !t.prev.makeAhead {
			p.lf[t.prev] = t.start
			ready = append(ready, t.prev)
		}
	}

	// Make-ahead chains of the others, forwards from the prep window, each
	// finished in time for its day-of part and no earlier than its hold
	// allows.
	type aheadChain struct {
		tasks []*ptask
		ef    int
		due   int
	}
	var chains []aheadChain
	for i, chain := range ahead {
		if len(chain) == 0 || trouble[i] {
			continue
		}
		due := deadlines[i]
		if d := dayOf[i]; len(d) > 0 {
			due = d[0].start
		}
		total := 0
		for _, t := range chain {
			total += t.dur
		}
		hold := chain[len(chain)-1].holdMax
		chains = append(chains, aheadChain{chain, max(prepMin, due-hold-total, floor), due})
	}
	slices.SortStableFunc(chains, func(a, b aheadChain) int { return cmp.Compare(a.ef, b.ef) })
	for _, ch := range chains {
		ef := ch.ef
		for j, t := range ch.tasks {
			after := 0
			for _, u := range ch.tasks[j+1:] {
				after += u.dur
			}
			if !t.placed {
				p.placeFwd(t, ef, ch.due-after)
			}
			ef = t.end
		}
		hold := ch.tasks[len(ch.tasks)-1]
		if hold.end > ch.due {
			p.warn("conflict", fmt.Sprintf("%s can't be made ahead in time; its make-ahead part runs into the day's.",
				p.title(hold)), hold, "")
		}
	}
}

// placeFrom puts t at the earliest time from ef that it fits, at any hour.
func (p *planner) placeFrom(t *ptask, ef int) {
	s0 := ef + mod(-ef, gridMin)
	for s := s0; s <= s0+3*24*60; s += gridMin {
		if oven, ok := p.fits(t, s, false); ok {
			p.put(t, s, oven)
			return
		}
	}
	p.put(t, s0, p.firstOven(t)) // a backstop: limits it can't meet are ignored, so something fits
}

// result turns the placed tasks into the wire Plan.
func (p *planner) result(serve, prepFrom time.Time, tasks []*ptask, dayOf, ahead [][]*ptask, deadlines []int, nowMin int) Plan {
	m := p.in.meal
	k := p.in.kitchen
	out := Plan{
		MealID: m.ID, MealVersion: m.Version, ServeAt: serve.UTC().Format(timeFormat),
		Now: p.in.now.UTC().Format(timeFormat), PrepFrom: prepFrom.UTC().Format(timeFormat), OnTime: true,
		Tasks: []PlanTask{}, Ovens: []OvenUse{}, Serving: []ServingDish{}, LateDishes: []LateDish{},
	}
	prepMin := p.minutes(prepFrom)

	// Behind: the latest a dish would land after its serving time.
	for i := range m.Items {
		chain := dayOf[i]
		if len(chain) == 0 {
			chain = ahead[i]
		}
		if len(chain) == 0 {
			continue
		}
		last := chain[len(chain)-1]
		late := last.end - deadlines[i]
		if late <= 0 {
			continue
		}
		out.BehindMinutes = max(out.BehindMinutes, late)
		ld := LateDish{ItemID: m.Items[i].ID, RecipeTitle: p.in.recipes[i].Title, ServeAt: p.stamp(deadlines[i]),
			ReadyAt: p.stamp(last.end), Minutes: late, Cause: "busy", Task: last.key}
		// The most telling reason: missed make-ahead, then work not started.
		for _, t := range append(slices.Clone(ahead[i]), dayOf[i]...) {
			if t.missed {
				ld.Cause, ld.Task = "missed", t.key
				break
			}
			if t.late && ld.Cause == "busy" {
				ld.Cause, ld.Task = "late_start", t.key
			}
		}
		out.LateDishes = append(out.LateDishes, ld)
	}
	slices.SortStableFunc(out.LateDishes, func(a, b LateDish) int { return cmp.Compare(b.Minutes, a.Minutes) })
	if out.BehindMinutes > 0 {
		out.OnTime = false
		worst := out.LateDishes[0]
		why := map[string]string{
			"missed":     "its make-ahead work was planned for earlier",
			"late_start": "work that should have started hasn't",
			"busy":       "the kitchen has no room for it sooner",
		}[worst.Cause]
		w := PlanWarning{Code: "behind", ItemID: ref(worst.ItemID), Task: ref(worst.Task),
			Message: fmt.Sprintf("%s will be about %s late: %s.", worst.RecipeTitle, humanMinutes(worst.Minutes), why)}
		if len(out.LateDishes) > 1 {
			w.Message += " " + plural(len(out.LateDishes)-1, "more dish is", "more dishes are") + " running late too."
		}
		p.warnings = append(p.warnings, w)
	}

	// Holds: the last make-ahead step waits until its day-of part starts.
	holds := map[*ptask]TaskHold{}
	for i := range m.Items {
		if len(ahead[i]) == 0 {
			continue
		}
		h := ahead[i][len(ahead[i])-1]
		until := deadlines[i]
		if len(dayOf[i]) > 0 {
			until = dayOf[i][0].start
		}
		holds[h] = TaskHold{In: h.holdIn, Until: p.stamp(until)}
	}

	tasks = append(tasks, p.preheats(tasks)...)
	slices.SortStableFunc(tasks, func(a, b *ptask) int {
		if c := cmp.Compare(a.start, b.start); c != 0 {
			return c
		}
		return cmp.Compare(a.end, b.end)
	})
	early := false
	for _, t := range tasks {
		pt := PlanTask{
			Key: t.key, Kind: t.kind, Text: t.text, Start: p.stamp(t.start), End: p.stamp(t.end),
			Minutes: t.dur, HandsOn: t.handsOn, MakeAhead: t.makeAhead, Estimated: t.estimated,
			Pinned: t.pinned, Status: StatusPending, DoneBy: t.prog.doneBy,
		}
		if t.item >= 0 {
			it := m.Items[t.item]
			pt.ItemID, pt.RecipeID, pt.RecipeTitle = ref(it.ID), ref(it.RecipeID), ref(p.in.recipes[t.item].Title)
		}
		if t.step >= 0 && t.kind == KindStep {
			pt.Step = ref(t.step + 1)
		}
		if t.equip != "" {
			pt.Equipment = ref(t.equip)
		}
		if t.oven >= 0 && t.oven < len(k.Ovens) {
			pt.Oven = ref(k.Ovens[t.oven].Name)
		}
		if t.temp > 0 {
			pt.OvenTemp = ref(t.temp)
		}
		if h, ok := holds[t]; ok {
			pt.Hold = &h
		}
		if t.prog.started != nil {
			pt.StartedAt = ref(t.prog.started.UTC().Format(timeFormat))
			pt.Status = StatusActive
		}
		if t.prog.done != nil {
			pt.DoneAt = ref(t.prog.done.UTC().Format(timeFormat))
			pt.Status = StatusDone
		} else if t.missed && t.prog.started == nil {
			pt.Status = StatusMissed
		} else if t.late && t.prog.started == nil {
			pt.Status = StatusLate
		}
		if t.kind == KindPreheat && t.end <= nowMin {
			pt.Status = StatusDone
		}
		if t.start < prepMin && !t.fixed && t.kind != KindPreheat {
			early = true
		}
		out.Tasks = append(out.Tasks, pt)
	}
	if len(out.Tasks) > 0 {
		out.Start = ref(out.Tasks[0].Start)
	}
	if early {
		p.warn("early", "Some work has to start before the prep window to be ready in time; start prepping earlier or move the meal.", nil, "early")
	}

	// Each oven's day, as spans of one temperature.
	for o, oven := range k.Ovens {
		use := OvenUse{Name: oven.Name, Racks: oven.Racks, Uses: []OvenSpan{}}
		var mine []*ptask
		for _, t := range tasks {
			if t.oven == o && t.equip == EquipOven && t.kind != KindPreheat && t.dur > 0 {
				mine = append(mine, t)
			}
		}
		slices.SortStableFunc(mine, func(a, b *ptask) int { return cmp.Compare(a.start, b.start) })
		var cur *OvenSpan
		curEnd := 0
		for _, t := range mine {
			if cur != nil && t.start <= curEnd && (cur.Temp == nil || t.temp == 0 || abs(*cur.Temp-t.temp) <= tempTolerance) {
				curEnd = max(curEnd, t.end)
				cur.End = p.stamp(curEnd)
				cur.Tasks = append(cur.Tasks, t.key)
				// Dishes within the tolerance share the oven; say the range.
				if cur.Temp != nil && t.temp > 0 {
					lo, hi := min(*cur.Temp, t.temp), max(*cur.Temp, t.temp)
					if cur.TempMax != nil {
						hi = max(hi, *cur.TempMax)
					}
					cur.Temp = ref(lo)
					if hi != lo {
						cur.TempMax = ref(hi)
					}
				}
				continue
			}
			use.Uses = append(use.Uses, OvenSpan{Start: p.stamp(t.start), End: p.stamp(t.end), Tasks: []string{t.key}})
			cur = &use.Uses[len(use.Uses)-1]
			if t.temp > 0 {
				cur.Temp = ref(t.temp)
			}
			curEnd = t.end
		}
		out.Ovens = append(out.Ovens, use)
	}

	// Serving dishes: each dish named, known to the kitchen, and not used
	// more times than the kitchen has.
	have := map[string]int{}
	for _, d := range k.Dishes {
		have[strings.ToLower(d.Name)] += d.Count
	}
	used := map[string][]string{}
	for i, it := range m.Items {
		title := p.in.recipes[i].Title
		out.Serving = append(out.Serving, ServingDish{ItemID: it.ID, RecipeTitle: title, Dish: it.Dish, ServeAt: p.stamp(deadlines[i])})
		if it.Dish == nil {
			if len(k.Dishes) > 0 {
				p.warnings = append(p.warnings, PlanWarning{Code: "no_dish", Message: title + " has no serving dish yet.", ItemID: ref(it.ID)})
			}
			continue
		}
		name := strings.ToLower(*it.Dish)
		if _, ok := have[name]; !ok && len(k.Dishes) > 0 {
			p.warnings = append(p.warnings, PlanWarning{Code: "dish_unknown",
				Message: fmt.Sprintf("%s goes in “%s”, which isn't in the kitchen's dishes.", title, *it.Dish), ItemID: ref(it.ID)})
		}
		used[name] = append(used[name], title)
	}
	for _, d := range k.Dishes {
		titles := used[strings.ToLower(d.Name)]
		if len(titles) > d.Count {
			p.warnings = append(p.warnings, PlanWarning{Code: "dish_short",
				Message: fmt.Sprintf("“%s” is planned for %s, but the kitchen has %d.", d.Name, joinAnd(titles), d.Count)})
		}
	}
	// Most consequential first: lateness, then what won't fit, then the rest.
	rank := map[string]int{"behind": 0, "conflict": 1, "early": 2, "in_trash": 3, "no_steps": 3, "waits": 4,
		"dish_short": 5, "dish_unknown": 5, "no_dish": 6, "no_oven": 7, "no_burners": 7, "unknown_appliance": 7, "estimated": 8}
	slices.SortStableFunc(p.warnings, func(a, b PlanWarning) int { return cmp.Compare(rank[a.Code], rank[b.Code]) })
	slices.SortStableFunc(out.Serving, func(a, b ServingDish) int { return cmp.Compare(a.ServeAt, b.ServeAt) })
	out.Warnings = p.warnings
	if out.Warnings == nil {
		out.Warnings = []PlanWarning{}
	}
	return out
}

// preheats adds a preheat before each oven task that finds its oven cold
// or at another temperature.
func (p *planner) preheats(tasks []*ptask) []*ptask {
	k := p.in.kitchen
	var out []*ptask
	for o := range k.Ovens {
		var mine []*ptask
		for _, t := range tasks {
			if t.oven == o && t.equip == EquipOven && t.temp > 0 && t.dur > 0 {
				mine = append(mine, t)
			}
		}
		slices.SortStableFunc(mine, func(a, b *ptask) int { return cmp.Compare(a.start, b.start) })
		for i, t := range mine {
			hot := false
			for _, u := range mine[:i] {
				if u.end >= t.start && abs(u.temp-t.temp) <= tempTolerance {
					hot = true
					break
				}
			}
			if hot {
				continue
			}
			ph := &ptask{
				key: "preheat:" + strconv.Itoa(o) + ":" + t.key, kind: KindPreheat, item: -1, step: -1,
				text: fmt.Sprintf("Preheat the %s to %d°F", strings.ToLower(k.Ovens[o].Name), t.temp),
				dur:  preheatMin, equip: EquipOven, temp: t.temp, space: 0, oven: o,
				start: t.start - preheatMin, end: t.start,
			}
			out = append(out, ph)
		}
	}
	return out
}

// load is the most weight in use at any moment of [s, e) among tasks.
func load(tasks []*ptask, s, e int, weight func(*ptask) int) int {
	type ev struct{ at, d int }
	var evs []ev
	for _, u := range tasks {
		w := weight(u)
		if w == 0 || u.start >= e || u.end <= s || u.dur <= 0 {
			continue
		}
		evs = append(evs, ev{max(u.start, s), w}, ev{min(u.end, e), -w})
	}
	slices.SortFunc(evs, func(a, b ev) int {
		if c := cmp.Compare(a.at, b.at); c != 0 {
			return c
		}
		return cmp.Compare(a.d, b.d) // ends before starts at the same minute
	})
	cur, peak := 0, 0
	for _, x := range evs {
		cur += x.d
		peak = max(peak, cur)
	}
	return peak
}

// humanMinutes says minutes as the UI does: "45 min", "2 hr", "1 hr 40".
func humanMinutes(m int) string {
	switch {
	case m < 60:
		return fmt.Sprintf("%d min", m)
	case m%60 == 0:
		return fmt.Sprintf("%d hr", m/60)
	}
	return fmt.Sprintf("%d hr %d", m/60, m%60)
}

// short is a step's opening words, for a warning.
func short(s string) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= 48 {
		return strings.TrimRight(s, ".")
	}
	cut := string(r[:48])
	if i := strings.LastIndex(cut, " "); i >= 20 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,;.:") + "…"
}

func joinAnd(xs []string) string {
	if len(xs) <= 1 {
		return strings.Join(xs, "")
	}
	return strings.Join(xs[:len(xs)-1], ", ") + " and " + xs[len(xs)-1]
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

func iffs(c bool, a, b string) string {
	if c {
		return a
	}
	return b
}

func mod(a, b int) int { return ((a % b) + b) % b }

func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func iff(c bool, v int) int {
	if c {
		return v
	}
	return 0
}
