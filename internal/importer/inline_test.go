package importer

import (
	"slices"
	"testing"
)

func TestSplitInlineNumbered(t *testing.T) {
	for _, tc := range []struct {
		in, want []string
	}{
		{[]string{"1. Preheat the oven to 425 degrees F. 2. In a large skillet, heat the oil. 3. Bake 20 minutes."},
			[]string{"Preheat the oven to 425 degrees F.", "In a large skillet, heat the oil.", "Bake 20 minutes."}},
		{[]string{"Bake for 2. Then rest."}, []string{"Bake for 2. Then rest."}},
		{[]string{"Heat oil. 1. Add onion. 2. Add garlic."}, []string{"Heat oil. 1. Add onion. 2. Add garlic."}},
		{[]string{"1. Only one numbered part."}, []string{"1. Only one numbered part."}},
	} {
		if got := Unnumber(splitInlineNumbered(tc.in)); !slices.Equal(got, tc.want) {
			t.Errorf("split(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSingleParens(t *testing.T) {
	for in, want := range map[string]string{
		"12 ounces thin spaghetti ((340g; can also use dried noodles))": "12 ounces thin spaghetti (340g; can also use dried noodles)",
		"8 cloves garlic ((peeled and thinly sliced))":                  "8 cloves garlic (peeled and thinly sliced)",
		"1 cup stock (or water)":                                        "1 cup stock (or water)",
	} {
		if got := singleParens(in); got != want {
			t.Errorf("singleParens(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIngredientHeading(t *testing.T) {
	for in, want := range map[string]string{
		"CHILE-LIME SLAW":         "# Chile-Lime Slaw",
		"AVOCADO-TOMATILLO SALSA": "# Avocado-Tomatillo Salsa",
		"FOR THE SAUCE:":          "# For The Sauce",
		"1 CUP SUGAR":             "1 CUP SUGAR",
		"Salt":                    "Salt",
		"OIL":                     "# Oil",
	} {
		if got, _ := ingredientHeading(in); got != want {
			t.Errorf("ingredientHeading(%q) = %q, want %q", in, got, want)
		}
	}
}
