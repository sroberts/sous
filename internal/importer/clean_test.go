package importer

import (
	"reflect"
	"testing"
)

func TestCleanText(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Albóndigas in Red Chile Sauce", "Albóndigas in Red Chile Sauce"},
		{"Alb&oacute;ndigas &amp; Rice", "Albóndigas & Rice"},
		{"Mac &amp;amp; Cheese", "Mac & Cheese"},                         // double-encoded
		{"&lt;p&gt;Stir &lt;b&gt;well&lt;/b&gt;&lt;/p&gt;", "Stir well"}, // encoded HTML
		{"Cook&#8217;s &#x2019;notes&#8221;", "Cook’s ’notes”"},
		{"  lots\t of\n\n  space  ", "lots of space"},
		{"non\u00a0breaking\u202fspaces", "non breaking spaces"},
		{"zero\u200bwidth\u200d joiner\ufeff", "zerowidth joiner"},
		{"<p>One</p><p>Two</p>", "One Two"},
		{"Heat <script>alert(1)</script>oil", "Heat oil"},
		{"Cook < 5 minutes", "Cook < 5 minutes"},
		{"", ""},
	} {
		if got := cleanText(tc.in); got != tc.want {
			t.Errorf("cleanText(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

func TestCleanLines(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"Heat oil.<br>Add onions.<br/>Stir.", []string{"Heat oil.", "Add onions.", "Stir."}},
		{"<p>Heat oil.</p><p>Add onions.</p>", []string{"Heat oil.", "Add onions."}},
		{"<ol><li>One</li><li>Two</li></ol>", []string{"One", "Two"}},
		{"Line one\nLine two\n\n", []string{"Line one", "Line two"}},
		{"&lt;br&gt;", nil},
	} {
		if got := cleanLines(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("cleanLines(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeTitle(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Basil Chicken Curry Zucchini Noodle Bowls.", "Basil Chicken Curry Zucchini Noodle Bowls"},
		{"Lexington-Style Red Coleslaw Recipe", "Lexington-Style Red Coleslaw"},
		{"Mustard Slaw Recipe.", "Mustard Slaw"},
		{"Grandma&#39;s Lemon Bars", "Grandma's Lemon Bars"},
		{"Wait for it...", "Wait for it..."},
		{"Recipe for Disaster", "Recipe for Disaster"},
		{"  Garlic   Noodles  ", "Garlic Noodles"},
	} {
		if got := NormalizeTitle(tc.in); got != tc.want {
			t.Errorf("NormalizeTitle(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

func TestUnnumber(t *testing.T) {
	for _, tc := range []struct {
		name     string
		in, want []string
	}{
		{"numbered", []string{"1 Heat the oven.", "2 Mix.", "3 Bake."}, []string{"Heat the oven.", "Mix.", "Bake."}},
		{"headings skipped", []string{"# Sauce", "1 Simmer.", "# Pasta", "2 Boil."}, []string{"# Sauce", "Simmer.", "# Pasta", "Boil."}},
		{"quantity, not numbering", []string{"Whisk the flour.", "2 eggs go in next.", "Bake."}, []string{"Whisk the flour.", "2 eggs go in next.", "Bake."}},
		{"out of order", []string{"1 Heat.", "3 Mix."}, []string{"1 Heat.", "3 Mix."}},
		{"starts at 2", []string{"2 eggs go in.", "3 cups flour."}, []string{"2 eggs go in.", "3 cups flour."}},
		{"single step", []string{"1 Do it all."}, []string{"1 Do it all."}},
		{"number alone", []string{"1 ", "2 Mix."}, []string{"1 ", "2 Mix."}},
		{"empty", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Unnumber(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Unnumber(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestStripStepPrefixes(t *testing.T) {
	for _, tc := range []struct{ in, want []string }{
		{[]string{"Step 1: Heat.", "Step 2: Mix."}, []string{"Heat.", "Mix."}},
		{[]string{"STEP 1 Heat.", "# Finish", "Step 2. Mix."}, []string{"Heat.", "# Finish", "Mix."}},
		{[]string{"Step 1: Heat.", "Mix."}, []string{"Step 1: Heat.", "Mix."}},
		{[]string{"Step 2: Heat.", "Step 1: Mix."}, []string{"Step 2: Heat.", "Step 1: Mix."}},
	} {
		if got := stripStepPrefixes(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("stripStepPrefixes(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}
