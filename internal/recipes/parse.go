package recipes

import (
	"math"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ParserVersion is stored with each parsed ingredient so lines can be
// re-parsed when the parser improves.
const ParserVersion = 1

// The parser is a faithful port of web/mock/parse.js, checked against a
// golden file generated from it. Use JS \s and trim semantics, a stable
// sort of unit aliases by length, and UTF-16 lengths. Non-finite numbers
// become nil.

// jsSpace is the body of a character class matching JS \s; RE2's \s is
// ASCII-only.
const jsSpace = `\t\n\v\f\r \x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}`

func isJSSpace(r rune) bool {
	switch {
	case r >= 0x09 && r <= 0x0d, r == ' ', r == 0xa0, r == 0x1680,
		r >= 0x2000 && r <= 0x200a, r == 0x2028, r == 0x2029, r == 0x202f,
		r == 0x205f, r == 0x3000, r == 0xfeff:
		return true
	}
	return false
}

// jsTrim is String.prototype.trim: unlike strings.TrimSpace it trims U+FEFF
// and leaves U+0085.
func jsTrim(s string) string {
	return strings.TrimFunc(s, isJSSpace)
}

var fractions = map[string]float64{
	"¼": 0.25, "½": 0.5, "¾": 0.75, "⅓": 1.0 / 3, "⅔": 2.0 / 3,
	"⅛": 0.125, "⅜": 0.375, "⅝": 0.625, "⅞": 0.875,
}

type unitAliases struct {
	unit    string
	aliases []string
}

var unitTable = []unitAliases{
	{"tsp", []string{"teaspoons", "teaspoon", "tsps", "tsp", "t"}},
	{"tbsp", []string{"tablespoons", "tablespoon", "tbsps", "tbsp", "tbs", "tbl", "T"}},
	{"cup", []string{"cups", "cup", "c", "C"}},
	{"fl oz", []string{"fluid ounces", "fluid ounce", "fl oz", "fl. oz."}},
	{"oz", []string{"ounces", "ounce", "oz"}},
	{"lb", []string{"pounds", "pound", "lbs", "lb"}},
	{"g", []string{"grams", "gram", "g"}},
	{"kg", []string{"kilograms", "kilogram", "kg"}},
	{"ml", []string{"milliliters", "milliliter", "millilitres", "millilitre", "ml"}},
	{"l", []string{"liters", "liter", "litres", "litre", "l"}},
	{"qt", []string{"quarts", "quart", "qt"}},
	{"pt", []string{"pints", "pint", "pt"}},
	{"pinch", []string{"pinches", "pinch"}},
	{"dash", []string{"dashes", "dash"}},
	{"clove", []string{"cloves", "clove"}},
	{"can", []string{"cans", "can"}},
	{"stick", []string{"sticks", "stick"}},
	{"sprig", []string{"sprigs", "sprig"}},
	{"bunch", []string{"bunches", "bunch"}},
	{"slice", []string{"slices", "slice"}},
	{"package", []string{"packages", "package", "pkg"}},
	{"head", []string{"heads", "head"}},
}

type unitAlias struct{ alias, unit string }

// units is longest alias first, so "fluid ounces" wins over "ounces". The
// sort must be stable to keep table order among equal lengths.
var units = func() []unitAlias {
	var out []unitAlias
	for _, u := range unitTable {
		for _, a := range u.aliases {
			out = append(out, unitAlias{a, u.unit})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return len(out[i].alias) > len(out[j].alias) })
	return out
}()

const num = `(?:\d+[` + jsSpace + `]+\d+/\d+|\d+[` + jsSpace + `]*[¼½¾⅓⅔⅛⅜⅝⅞]|\d+/\d+|\d*\.\d+|\d+|[¼½¾⅓⅔⅛⅜⅝⅞])`

var (
	qtyRE = regexp.MustCompile(`^(` + num + `)(?:[` + jsSpace + `]*(?:-|–|—|[Tt][Oo]|[Oo][Rr])[` + jsSpace + `]*(` + num + `))?[` + jsSpace + `]*`)

	mixedRE    = regexp.MustCompile(`^(\d+)[` + jsSpace + `]+(\d+)/(\d+)$`)
	mixedFrRE  = regexp.MustCompile(`^(\d+)[` + jsSpace + `]*([¼½¾⅓⅔⅛⅜⅝⅞])$`)
	simpleFrRE = regexp.MustCompile(`^(\d+)/(\d+)$`)

	sizeNoteRE  = regexp.MustCompile(`^\(([^)]*)\)[` + jsSpace + `]*`)
	unitEndRE   = regexp.MustCompile(`^(?:[` + jsSpace + `.,)]|$)`)
	unitDotRE   = regexp.MustCompile(`^\.?[` + jsSpace + `]*`)
	ofRE        = regexp.MustCompile(`^[Oo][Ff][` + jsSpace + `]+`)
	parenRE     = regexp.MustCompile(`[` + jsSpace + `]*\(([^)]*)\)[` + jsSpace + `]*`)
	commaRE     = regexp.MustCompile(`,[` + jsSpace + `]*`)
	trailingRE  = regexp.MustCompile(`[` + jsSpace + `,;]+$`)
	headingRE   = regexp.MustCompile(`^#+[` + jsSpace + `]*`)
	stepNumRE   = regexp.MustCompile(`^(?:[Ss][Tt][Ee][Pp][` + jsSpace + `]*)?\d+[.)][` + jsSpace + `]+`)
	trailColon  = regexp.MustCompile(`:$`)
	sentenceEnd = regexp.MustCompile(`[.!?]$`)
	digitsRE    = regexp.MustCompile(`\d+`)

	prepRE = regexp.MustCompile(`(?i)^(?:finely|thinly|roughly|coarsely|freshly|lightly|very|well|about|plus|divided|to taste|as needed|optional|for\b|at room|room temperature|cut|chopped|diced|sliced|minced|grated|peeled|trimmed|halved|quartered|crushed|smashed|softened|melted|cubed|torn|rinsed|drained|beaten|shredded|julienned|seeded|stemmed|cored|zested|juiced|toasted|warmed|cooled|chilled|packed|sifted|separated|whisked|patted|ground|cooked|uncooked|thawed|picked|pitted|deveined|scrubbed|in\b|into\b|such as|or more|preferably|if\b|leaves|white and|torn|broken|removed|reserved|squeezed|scraped|split|sliced)`)
)

// isPrep reports whether s starts with a prep word. Go's (?i) also folds
// U+017F and U+212A onto s and k; JS does not, so a non-ASCII match is
// rejected.
func isPrep(s string) bool {
	loc := prepRE.FindStringIndex(s)
	if loc == nil {
		return false
	}
	for i := 0; i < loc[1]; i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func atof(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func toNumber(s string) float64 {
	s = jsTrim(s)
	if m := mixedRE.FindStringSubmatch(s); m != nil {
		return atof(m[1]) + atof(m[2])/atof(m[3])
	}
	if m := mixedFrRE.FindStringSubmatch(s); m != nil {
		return atof(m[1]) + fractions[m[2]]
	}
	if m := simpleFrRE.FindStringSubmatch(s); m != nil {
		return atof(m[1]) / atof(m[2])
	}
	if f, ok := fractions[s]; ok {
		return f
	}
	return jsParseFloat(s)
}

// jsParseFloat handles the shapes NUM allows: digits, optionally with a
// decimal point.
func jsParseFloat(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil && !math.IsInf(f, 0) {
		return math.NaN()
	}
	return f
}

func finite(f float64) *float64 {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil
	}
	return &f
}

// replaceAllNotes is ReplaceAllStringFunc that also hands the first
// capture group to fn.
func replaceAllNotes(re *regexp.Regexp, s, repl string, fn func(inner string)) string {
	var b strings.Builder
	last := 0
	for _, m := range re.FindAllStringSubmatchIndex(s, -1) {
		b.WriteString(s[last:m[0]])
		b.WriteString(repl)
		fn(s[m[2]:m[3]])
		last = m[1]
	}
	b.WriteString(s[last:])
	return b.String()
}

// matchUnit reports the unit at the start of rest and the remainder after
// the alias. Single-letter aliases are case-sensitive ("T" is tablespoon,
// "t" teaspoon); longer ones are not.
func matchUnit(rest string) (unit, after string, ok bool) {
	for _, u := range units {
		n := len(u.alias)
		var tail string
		if n == 1 {
			if !strings.HasPrefix(rest, u.alias) {
				continue
			}
			tail = rest[1:]
		} else {
			// Rune-wise so the cut matches JS code-unit slicing even for
			// characters like U+212A that lower-case to ASCII.
			i, k := 0, 0
			for k < n && i < len(rest) {
				_, w := utf8.DecodeRuneInString(rest[i:])
				i += w
				k++
			}
			if k < n || strings.ToLower(rest[:i]) != u.alias {
				continue
			}
			tail = rest[i:]
		}
		if unitEndRE.MatchString(tail) {
			return u.unit, tail, true
		}
	}
	return "", "", false
}

// ParseIngredient parses one line (trimmed) into an Ingredient with a nil
// Section. An unparseable line has Item set to the whole line.
func ParseIngredient(raw string) Ingredient {
	raw = jsTrim(raw)
	ing := Ingredient{Raw: raw, Item: raw, ParserVersion: ParserVersion}
	rest := raw
	var notes []string
	if q := qtyRE.FindStringSubmatchIndex(rest); q != nil {
		ing.Quantity = finite(toNumber(rest[q[2]:q[3]]))
		if q[4] >= 0 {
			ing.QuantityMax = finite(toNumber(rest[q[4]:q[5]]))
		}
		rest = rest[q[1]:]
		// "1 (14-ounce) can": the size note sits between quantity and unit.
		if m := sizeNoteRE.FindStringSubmatchIndex(rest); m != nil {
			notes = append(notes, jsTrim(rest[m[2]:m[3]]))
			rest = rest[m[1]:]
		}
		if unit, tail, ok := matchUnit(rest); ok {
			ing.Unit = &unit
			tail = unitDotRE.ReplaceAllString(tail, "")
			rest = ofRE.ReplaceAllString(tail, "")
		}
	}
	rest = jsTrim(replaceAllNotes(parenRE, rest, " ", func(inner string) {
		notes = append(notes, jsTrim(inner))
	}))
	// "8 bone-in, skin-on chicken thighs" keeps its comma; "garlic, minced" splits.
	for _, m := range commaRE.FindAllStringIndex(rest, -1) {
		if isPrep(rest[m[1]:]) {
			notes = append(notes, jsTrim(rest[m[1]:]))
			rest = jsTrim(rest[:m[0]])
			break
		}
	}
	ing.Item = trailingRE.ReplaceAllString(rest, "")
	if ing.Item == "" {
		ing.Item = raw
	}
	var kept []string
	for _, n := range notes {
		if n != "" {
			kept = append(kept, n)
		}
	}
	if len(kept) > 0 {
		note := strings.Join(kept, "; ")
		ing.Note = &note
	}
	return ing
}

func ptr(s string) *string { return &s }

// ParseIngredientLines parses raw lines, skipping blanks and carrying
// "# Heading" lines as the Section of the lines below them.
func ParseIngredientLines(lines []string) []Ingredient {
	var section *string
	out := []Ingredient{}
	for _, line := range lines {
		text := jsTrim(line)
		if text == "" {
			continue
		}
		if strings.HasPrefix(text, "#") {
			s := jsTrim(trailColon.ReplaceAllString(headingRE.ReplaceAllString(text, ""), ""))
			section = nil
			if s != "" {
				section = &s
			}
			continue
		}
		ing := ParseIngredient(text)
		ing.Section = section
		out = append(out, ing)
	}
	return out
}

// utf16Len is the JS string length.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r >= 0x10000 {
			n++
		}
	}
	return n
}

func isSection(text string) bool {
	return trailColon.MatchString(text) || (utf16Len(text) <= 40 && !sentenceEnd.MatchString(text))
}

// ParseStepLines turns raw lines into steps: blanks are skipped, "1." style
// numbering is stripped, and short "# Heading" lines set the Section (a bare
// "#" gives a nil section).
func ParseStepLines(lines []string) []Step {
	var section *string
	out := []Step{}
	for _, line := range lines {
		text := jsTrim(line)
		if text == "" {
			continue
		}
		if strings.HasPrefix(text, "#") {
			text = jsTrim(headingRE.ReplaceAllString(text, ""))
			if isSection(text) {
				s := jsTrim(trailColon.ReplaceAllString(text, ""))
				section = nil
				if s != "" {
					section = &s
				}
				continue
			}
		}
		out = append(out, Step{Section: section, Text: stepNumRE.ReplaceAllString(text, "")})
	}
	return out
}

// PreviewLines is POST /v1/ingredients/parse: one ParsedLine per input line,
// blanks included. A line starting with # (after trimming) is a heading
// with Section set and Raw the line as given; others are ParseIngredient.
func PreviewLines(lines []string) []ParsedLine {
	out := make([]ParsedLine, 0, len(lines))
	for _, l := range lines {
		if strings.HasPrefix(jsTrim(l), "#") {
			out = append(out, ParsedLine{Section: ptr(headingRE.ReplaceAllString(l, "")), Raw: l})
			continue
		}
		ing := ParseIngredient(l)
		out = append(out, ParsedLine{
			Raw: ing.Raw, Quantity: ing.Quantity, QuantityMax: ing.QuantityMax,
			Unit: ing.Unit, Item: ing.Item, Note: ing.Note, ParserVersion: ing.ParserVersion,
		})
	}
	return out
}

// Servings is the first run of digits in yieldText, or nil.
func Servings(yieldText string) *int {
	m := digitsRE.FindString(yieldText)
	if m == "" {
		return nil
	}
	n, err := strconv.Atoi(m)
	if err != nil {
		return nil
	}
	return &n
}

// SourceHost is the hostname of rawURL with a leading "www." removed, or
// nil if rawURL doesn't parse as an absolute URL.
func SourceHost(rawURL string) *string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" {
		return nil
	}
	h := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if h == "" {
		return nil
	}
	return &h
}
