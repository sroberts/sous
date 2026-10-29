package importer

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var isoDurationRE = regexp.MustCompile(`(?i)^P(?:(\d+(?:\.\d+)?)Y)?(?:(\d+(?:\.\d+)?)M)?(?:(\d+(?:\.\d+)?)W)?(?:(\d+(?:\.\d+)?)D)?(?:T(?:(\d+(?:\.\d+)?)H)?(?:(\d+(?:\.\d+)?)M)?(?:(\d+(?:\.\d+)?)S)?)?$`)

// ParseISODuration reads an ISO 8601 duration such as "PT1H30M" as
// minutes, rounded half to even. ok is false when s isn't one; years and
// months are tolerated only when zero (P0Y0M0DT0H35M0.000S).
func ParseISODuration(s string) (minutes int, ok bool) {
	m := isoDurationRE.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, false
	}
	f := func(i int) float64 {
		if m[i] == "" {
			return 0
		}
		v, _ := strconv.ParseFloat(m[i], 64)
		return v
	}
	if f(1) != 0 || f(2) != 0 {
		return 0, false
	}
	total := f(3)*7*1440 + f(4)*1440 + f(5)*60 + f(6) + f(7)/60
	return int(math.RoundToEven(total)), true
}

// A port of _duration, minutes and time_text in
// scripts/mela-import/mela_import.py.

var (
	durUnit = map[string]float64{
		"h": 60, "hr": 60, "hrs": 60, "hour": 60, "hours": 60,
		"m": 1, "min": 1, "mins": 1, "minute": 1, "minutes": 1,
	}
	durNum    = `\d+(?:\.\d+)?(?:\s+\d+/\d+)?|\d+/\d+|\d*[½¼¾⅓⅔]`
	durPartRE = regexp.MustCompile(`(` + durNum + `)\s*(minutes|minute|hours|hour|mins|hrs|min|hr|h|m)(\.?)`)
	durJoiner = regexp.MustCompile(`^[\s,]*(?:and\s+)?$`)
	durLabel  = regexp.MustCompile(`^[a-z/ ]*time:\s*`)
	durAbout  = regexp.MustCompile(`^about\s+`)
	durDigits = regexp.MustCompile(`^\d+$`)
	durClock  = regexp.MustCompile(`^(\d+):([0-5]\d)$`)
	vulgar    = map[rune]float64{'½': 0.5, '¼': 0.25, '¾': 0.75, '⅓': 1.0 / 3, '⅔': 2.0 / 3}
)

func durNumber(s string) float64 {
	total := 0.0
	for _, piece := range strings.Fields(s) {
		last, size := utf8.DecodeLastRuneInString(piece)
		if v, ok := vulgar[last]; ok {
			whole, _ := strconv.ParseFloat(piece[:len(piece)-size], 64)
			total += v + whole
		} else if a, b, ok := strings.Cut(piece, "/"); ok {
			x, _ := strconv.ParseFloat(a, 64)
			y, _ := strconv.ParseFloat(b, 64)
			total += x / y
		} else {
			v, _ := strconv.ParseFloat(piece, 64)
			total += v
		}
	}
	return total
}

func isLower(b byte) bool { return b >= 'a' && b <= 'z' }

// melaDuration is _duration: minutes for plain wording, or false.
func melaDuration(text string) (float64, bool) {
	t := strings.ToLower(strings.TrimSpace(text))
	t = durLabel.ReplaceAllString(t, "")
	t = durAbout.ReplaceAllString(t, "")
	if durDigits.MatchString(t) {
		v, _ := strconv.ParseFloat(t, 64)
		return v, true
	}
	if m := durClock.FindStringSubmatch(t); m != nil {
		h, _ := strconv.Atoi(m[1])
		mm, _ := strconv.Atoi(m[2])
		return float64(h*60 + mm), true
	}
	total, pos, from := 0.0, 0, 0
	for from <= len(t) {
		loc := durPartRE.FindStringSubmatchIndex(t[from:])
		if loc == nil {
			break
		}
		for i := range loc {
			loc[i] += from
		}
		end := loc[1]
		// Python's (?![a-z]) lookahead: a dot followed by a letter backs off
		// to the unit alone; a letter right after the unit is no match here.
		if loc[6] != loc[7] && end < len(t) && isLower(t[end]) {
			end = loc[6]
		} else if end < len(t) && isLower(t[end]) {
			from = loc[0] + 1
			continue
		}
		if !durJoiner.MatchString(t[pos:loc[0]]) {
			return 0, false
		}
		total += durNumber(t[loc[2]:loc[3]]) * durUnit[t[loc[4]:loc[5]]]
		pos = end
		from = end
	}
	if pos == 0 || strings.Trim(t[pos:], " .") != "" {
		return 0, false
	}
	return total, true
}

// Minutes is minutes() from the Mela import: whole minutes for a plain
// duration ("1 hour 15 minutes", "1h", "45", "1:10", "about 20 min"), nil
// for zero and for anything with more meaning than a number.
func Minutes(text string) *int {
	d, ok := melaDuration(text)
	if !ok {
		return nil
	}
	n := int(math.RoundToEven(d))
	if n == 0 {
		return nil
	}
	return &n
}

// parseTime reads an ISO duration, then Mela-style wording. ok is false
// when neither understands it; minutes is nil for a zero duration.
func parseTime(s string) (minutes *int, ok bool) {
	if s = strings.TrimSpace(s); s == "" {
		return nil, true
	}
	if n, ok := ParseISODuration(s); ok {
		if n == 0 {
			return nil, true
		}
		return &n, true
	}
	if _, ok := melaDuration(s); ok {
		return Minutes(s), true
	}
	return nil, false
}

// TimeText is time_text() from the Mela import: the times that didn't
// parse, as written. A lone total reads fine bare; otherwise each is
// labelled, "Prep: … · Cook: …". ISO durations count as parsed.
func TimeText(prep, cook, total string) *string {
	type loose struct{ label, v string }
	var out []loose
	for _, t := range []loose{{"Prep", prep}, {"Cook", cook}, {"Total", total}} {
		v := strings.TrimSpace(t.v)
		if v == "" {
			continue
		}
		if _, ok := parseTime(v); !ok {
			out = append(out, loose{t.label, v})
		}
	}
	if len(out) == 0 {
		return nil
	}
	if len(out) == 1 && out[0].label == "Total" {
		return &out[0].v
	}
	parts := make([]string, len(out))
	for i, l := range out {
		parts[i] = l.label + ": " + l.v
	}
	s := strings.Join(parts, " · ")
	return &s
}

// Times maps a recipe's three time fields to minutes plus a time note for
// any that didn't parse. A total is never invented from prep and cook.
func Times(prep, cook, total string) (p, c, t *int, text *string) {
	p, _ = parseTime(prep)
	c, _ = parseTime(cook)
	t, _ = parseTime(total)
	return p, c, t, TimeText(prep, cook, total)
}
