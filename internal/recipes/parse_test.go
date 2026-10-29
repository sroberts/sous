package recipes

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// goldenFile is the output of scripts/parser-golden/gen.mjs.
type goldenFile struct {
	Ingredients []struct {
		Lines []string
		Out   []Ingredient
	}
	Steps []struct {
		Lines []string
		Out   []Step
	}
	Preview []struct {
		Lines []string
		Out   []ParsedLine
	}
	Servings []struct {
		Text string
		Out  *int
	}
	Hosts []struct {
		URL string
		Out *string
	}
}

func loadGolden(t *testing.T, path string) (goldenFile, bool) {
	t.Helper()
	var g goldenFile
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return g, false
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	return g, true
}

// mismatches runs every golden case and returns how many differ, logging
// the first few.
func mismatches(t *testing.T, g goldenFile) int {
	t.Helper()
	bad := 0
	fail := func(kind string, in, want, got any) {
		bad++
		if bad <= 20 {
			t.Errorf("%s mismatch\n  in:   %q\n  want: %s\n  got:  %s", kind, in, dump(want), dump(got))
		}
	}
	for _, c := range g.Ingredients {
		got := ParseIngredientLines(c.Lines)
		if !reflect.DeepEqual(got, orEmpty(c.Out)) {
			fail("ingredients", c.Lines, c.Out, got)
		}
	}
	for _, c := range g.Steps {
		want := orEmpty(c.Out)
		// Deviation: JS yields "" for a bare "#" heading where Go has nil.
		for i := range want {
			if want[i].Section != nil && *want[i].Section == "" {
				want[i].Section = nil
			}
		}
		got := ParseStepLines(c.Lines)
		if !reflect.DeepEqual(got, want) {
			fail("steps", c.Lines, want, got)
		}
	}
	for _, c := range g.Preview {
		got := PreviewLines(c.Lines)
		if !reflect.DeepEqual(got, orEmpty(c.Out)) {
			fail("preview", c.Lines, c.Out, got)
		}
	}
	for _, c := range g.Servings {
		if got := Servings(c.Text); !reflect.DeepEqual(got, c.Out) {
			fail("servings", c.Text, c.Out, got)
		}
	}
	for _, c := range g.Hosts {
		want := c.Out
		if want != nil && *want == "" {
			want = nil
		}
		if got := SourceHost(c.URL); !reflect.DeepEqual(got, want) {
			fail("host", c.URL, want, got)
		}
	}
	return bad
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func dump(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestParseGolden(t *testing.T) {
	g, ok := loadGolden(t, "testdata/parse_golden.json")
	if !ok {
		t.Fatal("testdata/parse_golden.json missing; see scripts/parser-golden/gen.mjs")
	}
	if n := mismatches(t, g); n > 0 {
		t.Errorf("%d mismatches", n)
	}
}

// TestParseGoldenLocal runs the personal corpus when it has been generated
// (it is gitignored).
func TestParseGoldenLocal(t *testing.T) {
	g, ok := loadGolden(t, "testdata/parse_golden.local.json")
	if !ok {
		t.Skip("testdata/parse_golden.local.json not generated")
	}
	n := mismatches(t, g)
	t.Logf("%d recipes, %d mismatches", len(g.Ingredients), n)
	if n > 0 {
		t.Errorf("%d mismatches", n)
	}
}

func TestParseSections(t *testing.T) {
	steps := ParseStepLines([]string{"# Prep", "a", "#", "b"})
	if steps[0].Section == nil || *steps[0].Section != "Prep" || steps[1].Section != nil {
		t.Errorf("sections: %s", dump(steps))
	}
}
