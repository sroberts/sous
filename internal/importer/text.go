package importer

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	ingredientsHeadRE = regexp.MustCompile(`(?i)^#*\s*ingredients?\s*:?$`)
	stepsHeadRE       = regexp.MustCompile(`(?i)^#*\s*(?:directions|instructions|method|steps|preparation|how to make it)\s*:?$`)
	quantityLineRE    = regexp.MustCompile(`^(?:\d|[½¼¾⅓⅔⅛⅜⅝⅞]|(?:a|an|one|two|three|four|half|pinch|dash|handful)\s)`)
	terminalRE        = regexp.MustCompile(`[.!?]["')\]]?$`)
	listMarkerRE      = regexp.MustCompile(`^(?:[▢•●◦▪‣∙·*]|-(?:\s|$))`)
)

const shortLine = 60

// SplitText splits pasted recipe text into a title, ingredient lines and
// steps. "Ingredients" and "Directions"-style headings split the sections
// when present; otherwise the leading run of quantity, bullet or short
// unpunctuated lines are the ingredients. The title is the first short
// line that isn't a quantity, when one comes before the ingredients.
func SplitText(text string) (title string, ingredients, steps []string) {
	type line struct {
		s      string
		bullet bool
	}
	var lines []line
	for _, l := range cleanLines(text) {
		b := listMarkerRE.MatchString(l)
		if l = stripBullet(l); l != "" {
			lines = append(lines, line{l, b})
		}
	}
	ing, stp := -1, -1
	for i, l := range lines {
		if ing < 0 && ingredientsHeadRE.MatchString(l.s) {
			ing = i
		} else if stp < 0 && stepsHeadRE.MatchString(l.s) {
			stp = i
		}
	}
	isIngredient := func(l line) bool {
		return l.bullet || quantityLineRE.MatchString(strings.ToLower(l.s)) ||
			(utf8.RuneCountInString(l.s) <= shortLine && !terminalRE.MatchString(l.s))
	}
	plain := func(ls []line) []string {
		out := make([]string, 0, len(ls))
		for _, l := range ls {
			out = append(out, l.s)
		}
		return out
	}
	pickTitle := func(ls []line) string {
		if len(ls) == 0 {
			return ""
		}
		l := ls[0]
		if l.bullet || quantityLineRE.MatchString(strings.ToLower(l.s)) || utf8.RuneCountInString(l.s) > shortLine {
			return ""
		}
		return l.s
	}

	switch {
	case ing >= 0 || stp >= 0:
		head := len(lines)
		for _, i := range []int{ing, stp} {
			if i >= 0 && i < head {
				head = i
			}
		}
		title = pickTitle(lines[:head])
		section := func(start int) []line {
			if start < 0 {
				return nil
			}
			end := len(lines)
			for _, i := range []int{ing, stp} {
				if i > start && i < end {
					end = i
				}
			}
			return lines[start+1 : end]
		}
		ingredients = plain(section(ing))
		steps = plain(section(stp))
		if ing < 0 {
			// Steps headed but ingredients not: the lines before the steps
			// heading (after any title) are the ingredients.
			rest := lines[:head]
			if title != "" {
				rest = rest[1:]
			}
			ingredients = plain(rest)
		}
	default:
		rest := lines
		if title = pickTitle(lines); title != "" && len(lines) > 1 && isIngredient(lines[1]) {
			rest = lines[1:]
		} else {
			title = ""
		}
		n := 0
		for n < len(rest) && isIngredient(rest[n]) {
			n++
		}
		ingredients = plain(rest[:n])
		steps = plain(rest[n:])
	}
	return title, ingredients, Unnumber(steps)
}
