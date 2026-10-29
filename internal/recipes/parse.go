package recipes

// ParserVersion is stored with each parsed ingredient so lines can be
// re-parsed when the parser improves.
const ParserVersion = 1

// The parser is a faithful port of web/mock/parse.js, checked against a
// golden file generated from it. Use JS \s and trim semantics, a stable
// sort of unit aliases by length, and UTF-16 lengths. Non-finite numbers
// become nil.

// ParseIngredient parses one line (trimmed) into an Ingredient with a nil
// Section. An unparseable line has Item set to the whole line.
func ParseIngredient(raw string) Ingredient {
	panic("TODO")
}

// ParseIngredientLines parses raw lines, skipping blanks and carrying
// "# Heading" lines as the Section of the lines below them.
func ParseIngredientLines(lines []string) []Ingredient {
	panic("TODO")
}

// ParseStepLines turns raw lines into steps: blanks are skipped, "1." style
// numbering is stripped, and short "# Heading" lines set the Section (a bare
// "#" gives a nil section).
func ParseStepLines(lines []string) []Step {
	panic("TODO")
}

// PreviewLines is POST /v1/ingredients/parse: one ParsedLine per input line,
// blanks included. A line starting with # (after trimming) is a heading
// with Section set and Raw the line as given; others are ParseIngredient.
func PreviewLines(lines []string) []ParsedLine {
	panic("TODO")
}

// Servings is the first run of digits in yieldText, or nil.
func Servings(yieldText string) *int {
	panic("TODO")
}

// SourceHost is the hostname of rawURL with a leading "www." removed, or
// nil if rawURL doesn't parse as an absolute URL.
func SourceHost(rawURL string) *string {
	panic("TODO")
}
