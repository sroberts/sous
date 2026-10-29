package importer

import (
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// blockTags end a line when HTML is flattened to text.
var blockTags = map[string]bool{
	"br": true, "p": true, "li": true, "div": true, "ol": true, "ul": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"tr": true, "dt": true, "dd": true, "blockquote": true, "section": true,
	"article": true, "header": true, "footer": true, "table": true, "hr": true,
}

var (
	entityRE = regexp.MustCompile(`&(?:[a-zA-Z][a-zA-Z0-9]*|#[0-9]+|#[xX][0-9a-fA-F]+);`)
	tagRE    = regexp.MustCompile(`<(?:[a-zA-Z]|/[a-zA-Z])`)
	spaceRE  = regexp.MustCompile(`\s+`)
	zeroWide = strings.NewReplacer("\u200b", "", "\u200c", "", "\u200d", "", "\u2060", "", "\ufeff", "", "\u00ad", "")
	nbsp     = strings.NewReplacer("\u00a0", " ", "\u202f", " ", "\u2007", " ")
)

// flatten strips tags, decoding entities once; block elements and line
// breaks become newlines. Script and style contents are dropped.
func flatten(s string) string {
	if !strings.ContainsAny(s, "<&") {
		return s
	}
	var b strings.Builder
	z := html.NewTokenizer(strings.NewReader(s))
	skip := 0
	for {
		switch z.Next() {
		case html.ErrorToken:
			return b.String()
		case html.TextToken:
			if skip == 0 {
				b.Write(z.Text())
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			switch n := string(name); {
			case n == "script" || n == "style":
				skip++
			case blockTags[n]:
				b.WriteByte('\n')
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			switch n := string(name); {
			case n == "script" || n == "style":
				if skip > 0 {
					skip--
				}
			case blockTags[n]:
				b.WriteByte('\n')
			}
		}
	}
}

// cleanLines decodes entities (a second time if the text was encoded
// twice), strips tags, and returns the non-empty lines, each with its
// whitespace collapsed.
func cleanLines(s string) []string {
	s = flatten(s)
	if entityRE.MatchString(s) || tagRE.MatchString(s) {
		s = flatten(s)
	}
	s = nbsp.Replace(zeroWide.Replace(s))
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(spaceRE.ReplaceAllString(line, " ")); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// cleanText is cleanLines joined into one line.
func cleanText(s string) string {
	return strings.Join(cleanLines(s), " ")
}

var (
	trailingRecipeRE = regexp.MustCompile(`(?i)\s+recipe$`)
	bulletRE         = regexp.MustCompile(`^(?:[▢•●◦▪‣∙·*]|-(?:\s|$))\s*`)
)

// NormalizeTitle cleans a title and drops a trailing period and " Recipe".
func NormalizeTitle(s string) string {
	s = cleanText(s)
	for range 2 {
		if strings.HasSuffix(s, ".") && !strings.HasSuffix(s, "..") {
			s = strings.TrimSpace(strings.TrimSuffix(s, "."))
		}
		s = trailingRecipeRE.ReplaceAllString(s, "")
	}
	return s
}

func stripBullet(s string) string {
	return strings.TrimSpace(bulletRE.ReplaceAllString(s, ""))
}

var (
	leadingDotRE = regexp.MustCompile(`^\.(\d)`)
	capsHeadRE   = regexp.MustCompile(`^[\p{Lu}][\p{Lu}\s&'’/,-]{2,48}:?$`)
)

// ingredientHeading turns an all-capitals ingredient line ("CHILE-LIME SLAW"),
// which some sites use for a component, into a "# Chile-Lime Slaw" section.
func ingredientHeading(s string) (string, bool) {
	if !capsHeadRE.MatchString(s) || strings.ContainsAny(s, "0123456789") {
		return s, false
	}
	words := strings.Fields(strings.TrimSuffix(strings.ToLower(s), ":"))
	for i, w := range words {
		parts := strings.Split(w, "-")
		for j, p := range parts {
			if r := []rune(p); len(r) > 0 {
				parts[j] = strings.ToUpper(string(r[0])) + string(r[1:])
			}
		}
		words[i] = strings.Join(parts, "-")
	}
	return "# " + strings.Join(words, " "), true
}

var doubleParenRE = regexp.MustCompile(`\(\(([^()]*)\)\)`)

// singleParens undoes the doubled parentheses WP Recipe Maker writes around
// ingredient notes: "spaghetti ((340g))" becomes "spaghetti (340g)".
func singleParens(s string) string {
	return doubleParenRE.ReplaceAllString(s, "($1)")
}

var (
	unnumberMatch = regexp.MustCompile(`^(\d+)\s+\S`)
	unnumberStrip = regexp.MustCompile(`^\d+\s+`)
	stepPrefixRE  = regexp.MustCompile(`(?i)^step\s*(\d+)\s*[:.)\-–—]?\s*`)
)

// Unnumber is unnumber() from the Mela import: strip "1 Heat the oven…"
// numbering, but only when every non-heading step is numbered 1, 2, 3… in
// order, so a step that merely starts with a quantity ("2 eggs go in next")
// is safe.
func Unnumber(steps []string) []string {
	return stripSequential(steps, unnumberMatch, unnumberStrip)
}

var inlineNumberRE = regexp.MustCompile(`(?:^|\s)(\d{1,2})\.\s+`)

// splitInlineNumbered splits a step that holds a whole numbered method
// ("1. Preheat… 2. In a skillet…"), as some recipe plugins publish it, into
// one step per number. It only splits when the numbers run 1, 2, 3… in order,
// so "Bake for 2. Then" style text is left alone.
func splitInlineNumbered(steps []string) []string {
	var out []string
	for _, s := range steps {
		locs := inlineNumberRE.FindAllStringSubmatchIndex(s, -1)
		var cuts [][2]int // where each number starts, and where its text starts
		next := 1
		for _, l := range locs {
			if n, _ := strconv.Atoi(s[l[2]:l[3]]); n == next {
				cuts = append(cuts, [2]int{l[0], l[1]})
				next++
			}
		}
		if len(cuts) < 2 || strings.TrimSpace(s[:cuts[0][0]]) != "" {
			out = append(out, s)
			continue
		}
		for i, c := range cuts {
			end := len(s)
			if i+1 < len(cuts) {
				end = cuts[i+1][0]
			}
			out = append(out, strings.TrimSpace(s[c[1]:end]))
		}
	}
	return out
}

// stripStepPrefixes drops "Step 1:" labels under the same rule.
func stripStepPrefixes(steps []string) []string {
	return stripSequential(steps, stepPrefixRE, stepPrefixRE)
}

func stripSequential(steps []string, match, strip *regexp.Regexp) []string {
	n := 0
	for _, s := range steps {
		if strings.HasPrefix(s, "#") {
			continue
		}
		n++
		m := match.FindStringSubmatch(s)
		if m == nil {
			return steps
		}
		if v, err := strconv.Atoi(m[1]); err != nil || v != n {
			return steps
		}
	}
	if n < 2 {
		return steps
	}
	out := make([]string, len(steps))
	for i, s := range steps {
		out[i] = s
		if !strings.HasPrefix(s, "#") {
			out[i] = strip.ReplaceAllString(s, "")
		}
	}
	return out
}
