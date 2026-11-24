package recipes

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// StepTiming is how a step fits into a meal plan: how long it takes, whether
// it needs the cook the whole time, what it occupies, and whether the dish
// can stop after it and wait (make-ahead). Every field is optional; the
// planner fills gaps with SuggestTimings' guesses from the step's words.
// Unset fields are left out of the JSON, so a client sends only what it
// knows.
type StepTiming struct {
	Minutes       *int    `json:"minutes,omitempty" jsonschema:"how long the step takes, start to finish"`
	HandsOn       *bool   `json:"hands_on,omitempty" jsonschema:"true when the cook is busy for the whole step; false when it looks after itself (baking, resting, chilling). Unset counts as true"`
	Equipment     *string `json:"equipment,omitempty" jsonschema:"what the step occupies: oven, stovetop, or the name of an appliance in the kitchen such as slow cooker; null for nothing"`
	OvenTemp      *int    `json:"oven_temp,omitempty" jsonschema:"oven temperature in °F, with equipment oven"`
	Space         *int    `json:"space,omitempty" jsonschema:"oven racks, burners or appliances the step takes at once; unset counts as 1"`
	HoldMinutes   *int    `json:"hold_minutes,omitempty" jsonschema:"make-ahead: the dish can stop after this step and wait up to this many minutes before carrying on"`
	HoldIn        *string `json:"hold_in,omitempty" jsonschema:"with hold_minutes, where it waits: fridge, counter or warm"`
	ReheatMinutes *int    `json:"reheat_minutes,omitempty" jsonschema:"with hold_minutes, how long it takes to bring back before the next step or serving"`
	ReheatTemp    *int    `json:"reheat_temp,omitempty" jsonschema:"with reheat_minutes, the oven temperature in °F to reheat at; null brings it back out of the oven (to room temperature, say)"`
}

// Where a held dish waits.
const (
	HoldFridge  = "fridge"
	HoldCounter = "counter"
	HoldWarm    = "warm"
)

// Equipment every kitchen has; anything else names an appliance.
const (
	EquipOven     = "oven"
	EquipStovetop = "stovetop"
)

// Limits on timing values: a week of minutes, and oven temperatures from a
// warming drawer to a pizza oven.
const (
	maxStepMinutes = 7 * 24 * 60
	minOvenTemp    = 150
	maxOvenTemp    = 900
	maxSpace       = 12
)

// empty reports a timing with nothing set, which is stored as no timing.
func (t *StepTiming) empty() bool {
	return t == nil || *t == StepTiming{}
}

// normalise tidies t in place (equipment and hold place lower-cased and
// trimmed, blanks to null) and checks its ranges. which names the step for
// the error message.
func (t *StepTiming) normalise(which string) error {
	if t == nil {
		return nil
	}
	tidy := func(p **string) {
		if *p == nil {
			return
		}
		v := strings.ToLower(strings.Join(strings.Fields(**p), " "))
		if v == "" {
			*p = nil
			return
		}
		*p = &v
	}
	tidy(&t.Equipment)
	tidy(&t.HoldIn)
	if t.Equipment != nil && len(*t.Equipment) > 40 {
		return Errorf(CodeInvalid, "%s: equipment is at most 40 characters", which)
	}
	for _, f := range []struct {
		name     string
		v        *int
		min, max int
	}{
		{"minutes", t.Minutes, 0, maxStepMinutes},
		{"space", t.Space, 1, maxSpace},
		{"hold_minutes", t.HoldMinutes, 1, 60 * 24 * 14},
		{"reheat_minutes", t.ReheatMinutes, 0, 24 * 60},
		{"oven_temp", t.OvenTemp, minOvenTemp, maxOvenTemp},
		{"reheat_temp", t.ReheatTemp, minOvenTemp, maxOvenTemp},
	} {
		if f.v != nil && (*f.v < f.min || *f.v > f.max) {
			return Errorf(CodeInvalid, "%s: %s must be between %d and %d", which, f.name, f.min, f.max)
		}
	}
	if t.HoldIn != nil {
		switch *t.HoldIn {
		case HoldFridge, HoldCounter, HoldWarm:
		default:
			return Errorf(CodeInvalid, "%s: hold_in must be fridge, counter or warm", which)
		}
		if t.HoldMinutes == nil {
			return Errorf(CodeInvalid, "%s: hold_in needs hold_minutes", which)
		}
	}
	if (t.ReheatMinutes != nil || t.ReheatTemp != nil) && t.HoldMinutes == nil {
		return Errorf(CodeInvalid, "%s: reheating needs hold_minutes", which)
	}
	if t.OvenTemp != nil && (t.Equipment == nil || *t.Equipment != EquipOven) {
		return Errorf(CodeInvalid, "%s: oven_temp needs equipment oven", which)
	}
	return nil
}

// applyTimings sets steps[i].Timing from timings, which follow the steps in
// order. More timings than steps is CodeInvalid; an empty timing is none.
func applyTimings(steps []Step, timings []*StepTiming) error {
	if len(timings) > len(steps) {
		return Errorf(CodeInvalid, "step_timing has %d entries but there are %d steps", len(timings), len(steps))
	}
	for i := range steps {
		steps[i].Timing = nil
		if i >= len(timings) || timings[i].empty() {
			continue
		}
		t := *timings[i]
		if err := t.normalise("step_timing[" + strconv.Itoa(i) + "]"); err != nil {
			return err
		}
		steps[i].Timing = &t
	}
	return nil
}

// carryTimings gives each new step the timing of an old step with the same
// text, each old timing used once, so editing the words of one step (or
// adding one) keeps the timing on the rest.
func carryTimings(old, steps []Step) {
	used := make([]bool, len(old))
	for i := range steps {
		for j, o := range old {
			if !used[j] && o.Timing != nil && o.Text == steps[i].Text {
				t := *o.Timing
				steps[i].Timing = &t
				used[j] = true
				break
			}
		}
	}
}

// ---- Guessing timing from a step's words ----

var (
	numWord = map[string]float64{
		"a": 1, "an": 1, "one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6,
		"seven": 7, "eight": 8, "nine": 9, "ten": 10, "eleven": 11, "twelve": 12,
		"fifteen": 15, "twenty": 20, "thirty": 30, "forty": 40, "forty-five": 45, "sixty": 60,
		"several": 3, "a few": 3, "a couple of": 2, "couple of": 2,
	}
	numPat   = `(\d+(?:[.,]\d+)?(?:\s*[½¼¾⅓⅔]|\s+\d/\d)?|[½¼¾⅓⅔]|\d/\d|a few|a couple of|couple of|forty-five|an?|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|fifteen|twenty|thirty|forty|sixty|several)`
	unitPat  = `(hours?|hrs?|h|minutes?|mins?|m|days?)`
	durRE    = regexp.MustCompile(`(?i)\b` + numPat + `(?:\s*(?:to|-|–|or)\s*` + numPat + `)?\s*` + unitPat + `\b`)
	halfRE   = regexp.MustCompile(`(?i)\bhalf an? hour\b`)
	nightRE  = regexp.MustCompile(`(?i)\bovernight\b`)
	tempFRE  = regexp.MustCompile(`(?i)\b(\d{3})\s*(?:°|º|degrees?)?\s*(?:F\b|fahrenheit)`)
	tempCRE  = regexp.MustCompile(`(?i)\b(\d{2,3})\s*(?:°|º|degrees?)\s*(?:C\b|celsius)`)
	tempRE   = regexp.MustCompile(`(?i)\b(\d{3})\s*(?:°|º|degrees)`)
	preheat  = regexp.MustCompile(`(?i)\b(pre-?heat|heat the oven)\b`)
	sentence = regexp.MustCompile(`[.;!?]\s+`)
	orBefore = regexp.MustCompile(`(?i)\bor\s+(?:(?:in|on|use|with)\s+)?(?:(?:the|a|an)\s+)?$`)
	ovenRE   = regexp.MustCompile(`(?i)\b(bake|baking|roast|roasting|broil|oven)\b`)
	notOven  = regexp.MustCompile(`(?i)\bdutch oven\b.*\b(over|stovetop|stove|burner)\b|\bover (low|medium|high|medium-high|medium-low)\b`)
	stoveRE  = regexp.MustCompile(`(?i)\b(simmer|boil|boiling|saut[eé]|fry|frying|sear|skillet|saucepan|frying pan|wok|stovetop|stove|burner|over (low|medium|high|medium-high|medium-low)( heat)?|bring to a)\b`)
	passive  = regexp.MustCompile(`(?i)\b(bake|roast|simmer|braise|rest|chill|refrigerate|fridge|marinate|cool|rise|prove|proof|stand|set aside|soak|brine|freeze|steep|slow[- ]cook\w*|crock[- ]?pot|pressure cook\w*|smoke|leave)\b`)
	aheadRE  = regexp.MustCompile(`(?i)\b(?:up to|as much as|at most|can be (?:made|prepared|done)|will keep|keeps)\s+(?:for\s+)?` + numPat + `\s*(days?|hours?)\b(?:\s+(?:ahead|in advance|before))?`)
	aheadDay = regexp.MustCompile(`(?i)\b(?:a|one) day (?:ahead|in advance|before)\b|\bthe day before\b|\bmake[- ]ahead\b`)
	fridgeRE = regexp.MustCompile(`(?i)\b(refrigerat\w*|fridge|chill\w*|cover and keep cold)\b`)
	appls    = []struct {
		re   *regexp.Regexp
		name string
	}{
		{regexp.MustCompile(`(?i)\b(slow[- ]cooker|crock[- ]?pot)\b`), "slow cooker"},
		{regexp.MustCompile(`(?i)\b(instant pot|pressure cooker|multicooker)\b`), "pressure cooker"},
		{regexp.MustCompile(`(?i)\b(smoker)\b`), "smoker"},
		{regexp.MustCompile(`(?i)\b(grill|barbecue)\b`), "grill"},
		{regexp.MustCompile(`(?i)\b(air[- ]fryer)\b`), "air fryer"},
		{regexp.MustCompile(`(?i)\b(microwave)\b`), "microwave"},
		{regexp.MustCompile(`(?i)\b(stand mixer)\b`), "stand mixer"},
	}
)

func parseNum(s string) float64 {
	s = strings.ToLower(strings.TrimSpace(s))
	if v, ok := numWord[s]; ok {
		return v
	}
	frac := map[rune]float64{'½': .5, '¼': .25, '¾': .75, '⅓': 1.0 / 3, '⅔': 2.0 / 3}
	total := 0.0
	for _, part := range strings.Fields(strings.ReplaceAll(s, ",", ".")) {
		r := []rune(part)
		if f, ok := frac[r[len(r)-1]]; ok {
			total += f
			part = string(r[:len(r)-1])
		}
		if n, d, ok := strings.Cut(part, "/"); ok {
			a, _ := strconv.ParseFloat(n, 64)
			b, _ := strconv.ParseFloat(d, 64)
			if b != 0 {
				total += a / b
			}
			continue
		}
		v, _ := strconv.ParseFloat(part, 64)
		total += v
	}
	return total
}

func unitMinutes(u string) float64 {
	switch u = strings.ToLower(u); {
	case strings.HasPrefix(u, "d"):
		return 24 * 60
	case strings.HasPrefix(u, "h"):
		return 60
	}
	return 1
}

// stepMinutes adds up the durations a step names ("Simmer 10 minutes, then
// 5 minutes more" is 15), taking the top of a range, since finishing early
// is easier to absorb than finishing late. "Overnight" is 8 hours. A make-
// ahead allowance ("up to 2 days ahead") is not a duration.
func stepMinutes(text string) (int, bool) {
	text = aheadRE.ReplaceAllString(text, " ")
	total := 0.0
	found := false
	// "Half an hour" first, so its "an hour" isn't counted again.
	if halfRE.MatchString(text) {
		total += 30 * float64(len(halfRE.FindAllString(text, -1)))
		found = true
		text = halfRE.ReplaceAllString(text, " ")
	}
	for _, m := range durRE.FindAllStringSubmatch(text, -1) {
		v := parseNum(m[1])
		if m[2] != "" {
			v = math.Max(v, parseNum(m[2]))
		}
		// "m" alone is too often something else ("2 m long"); only after a figure.
		if strings.EqualFold(m[3], "m") && !strings.ContainsAny(m[1], "0123456789") {
			continue
		}
		total += v * unitMinutes(m[3])
		found = true
	}
	if nightRE.MatchString(text) {
		total += 8 * 60
		found = true
	}
	return int(math.Round(total)), found
}

func ovenTemp(text string) (int, bool) {
	if m := tempFRE.FindStringSubmatch(text); m != nil {
		v, _ := strconv.Atoi(m[1])
		return v, true
	}
	if m := tempCRE.FindStringSubmatch(text); m != nil {
		c, _ := strconv.Atoi(m[1])
		// To °F, to the nearest 5 as ovens are marked.
		return int(math.Round((float64(c)*9/5+32)/5) * 5), true
	}
	if m := tempRE.FindStringSubmatch(text); m != nil {
		v, _ := strconv.Atoi(m[1])
		if v >= 200 && v <= 550 {
			return v, true
		}
	}
	return 0, false
}

// SuggestTimings guesses each step's timing from its words: the durations it
// names, an oven temperature (carried forward from "preheat the oven to
// 400°F" to the steps that bake), the equipment its verbs need, whether it
// looks after itself, and make-ahead wording ("up to 2 days ahead"). A guess
// leaves a field null when the words say nothing about it.
func SuggestTimings(texts []string) []StepTiming {
	out := make([]StepTiming, len(texts))
	lastTemp := 0
	for i, text := range texts {
		t := &out[i]
		temp, hasTemp := ovenTemp(text)
		if hasTemp {
			lastTemp = temp
		}
		if preheat.MatchString(text) {
			// The planner preheats ovens itself. A step that only preheats
			// sets the temperature and is left out; one that also does other
			// work ("Heat the oven. Pat the beef dry and season it.") is read
			// for that work alone.
			rest := withoutPreheat(text)
			if len(strings.Fields(rest)) < 3 {
				t.Minutes, t.HandsOn = ref(0), ref(false)
				continue
			}
			text = rest
		}
		if m, ok := stepMinutes(text); ok {
			t.Minutes = &m
		}
		for _, a := range appls {
			// "over simmering water, or in the microwave" offers it, not needs it.
			if loc := a.re.FindStringIndex(text); loc != nil && !orBefore.MatchString(text[:loc[0]]) {
				t.Equipment = ref(a.name)
				break
			}
		}
		if t.Equipment == nil {
			switch {
			case ovenRE.MatchString(text) && !notOven.MatchString(text):
				t.Equipment = ref(EquipOven)
				if hasTemp {
					t.OvenTemp = ref(temp)
				} else if lastTemp != 0 {
					t.OvenTemp = ref(lastTemp)
				}
			case stoveRE.MatchString(text):
				t.Equipment = ref(EquipStovetop)
			}
		}
		if passive.MatchString(text) && t.Minutes != nil && *t.Minutes >= 10 {
			t.HandsOn = ref(false)
		}
		if m := aheadRE.FindStringSubmatch(text); m != nil {
			h := int(math.Round(parseNum(m[1]) * unitMinutes(m[2])))
			if h > 0 {
				t.HoldMinutes = &h
			}
		} else if aheadDay.MatchString(text) {
			t.HoldMinutes = ref(24 * 60)
		}
		if t.HoldMinutes != nil {
			where := HoldCounter
			if fridgeRE.MatchString(text) {
				where = HoldFridge
			}
			t.HoldIn = &where
		}
	}
	return out
}

// ref returns a pointer to a copy of v.
func ref[T any](v T) *T { return &v }

// withoutPreheat is a step's words with any sentence about preheating taken out.
func withoutPreheat(text string) string {
	var keep []string
	for _, part := range sentence.Split(text, -1) {
		if part = strings.TrimSpace(part); part != "" && !preheat.MatchString(part) {
			keep = append(keep, part)
		}
	}
	return strings.Join(keep, ". ")
}
