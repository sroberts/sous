package importer

import (
	"reflect"
	"testing"
)

func TestSplitText(t *testing.T) {
	for _, tc := range []struct {
		name               string
		in                 string
		title              string
		ingredients, steps []string
	}{
		{
			name: "headings",
			in: `Gatorade Bread

Ingredients:
• 1 box cake mix
• 1 bottle Gatorade
- 2 eggs

Directions
1 Mix everything.
2 Bake at 350 for 30 minutes.`,
			title:       "Gatorade Bread",
			ingredients: []string{"1 box cake mix", "1 bottle Gatorade", "2 eggs"},
			steps:       []string{"Mix everything.", "Bake at 350 for 30 minutes."},
		},
		{
			name:        "method heading only",
			in:          "Toast\n2 slices bread\nbutter\nMethod\nToast the bread. Butter it.",
			title:       "Toast",
			ingredients: []string{"2 slices bread", "butter"},
			steps:       []string{"Toast the bread. Butter it."},
		},
		{
			name:        "no headings",
			in:          "Quick Pickles\n1 cucumber\n½ cup vinegar\n1 tsp salt\nSlice the cucumber thinly and pack it into a jar.\nPour over the vinegar and salt, then chill overnight.",
			title:       "Quick Pickles",
			ingredients: []string{"1 cucumber", "½ cup vinegar", "1 tsp salt"},
			steps:       []string{"Slice the cucumber thinly and pack it into a jar.", "Pour over the vinegar and salt, then chill overnight."},
		},
		{
			name:        "no title",
			in:          "1 cup rice\n2 cups water\nBring to a boil, cover and simmer for 18 minutes.",
			ingredients: []string{"1 cup rice", "2 cups water"},
			steps:       []string{"Bring to a boil, cover and simmer for 18 minutes."},
		},
		{
			name:        "quantity step kept",
			in:          "Ingredients\n3 eggs\nInstructions\nWhisk the flour and milk.\n2 eggs go in next.",
			ingredients: []string{"3 eggs"},
			steps:       []string{"Whisk the flour and milk.", "2 eggs go in next."},
		},
		{
			name:        "html entities",
			in:          "Ingredients\n1 cup half &amp; half\nSteps\nStir.",
			ingredients: []string{"1 cup half & half"},
			steps:       []string{"Stir."},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			title, ing, steps := SplitText(tc.in)
			if title != tc.title {
				t.Errorf("title = %q; want %q", title, tc.title)
			}
			if !reflect.DeepEqual(ing, tc.ingredients) {
				t.Errorf("ingredients = %q; want %q", ing, tc.ingredients)
			}
			if !reflect.DeepEqual(steps, tc.steps) {
				t.Errorf("steps = %q; want %q", steps, tc.steps)
			}
		})
	}
}
