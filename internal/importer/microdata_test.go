package importer

import (
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
)

func scanFile(t *testing.T, name, rawURL string) Page {
	t.Helper()
	b, err := os.ReadFile("testdata/html/" + name)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(rawURL)
	return Scan(b, "text/html", u)
}

func TestExtractMicrodata(t *testing.T) {
	p := scanFile(t, "microdata.html", "https://example.com/lemon-bars")
	if p.Microdata == "" || !strings.HasPrefix(p.Microdata, "<div") {
		t.Fatalf("microdata = %q", p.Microdata)
	}
	in, cands, rep := extract(t, p)
	if rep.Method != MethodMicrodata {
		t.Errorf("method = %q", rep.Method)
	}
	if in.Title != "Grandma's Lemon Bars" {
		t.Errorf("title = %q", in.Title)
	}
	if str(in.Description) != "Tart, sweet & easy." {
		t.Errorf("description = %s", str(in.Description))
	}
	if !reflect.DeepEqual(in.Ingredients, []string{"1 cup butter, softened", "2 cups all-purpose flour", "4 eggs"}) {
		t.Errorf("ingredients = %q", in.Ingredients)
	}
	if !reflect.DeepEqual(in.Steps, []string{"Heat the oven to 350°F.", "Press the crust into the pan.", "Pour over the filling and bake."}) {
		t.Errorf("steps = %q", in.Steps)
	}
	if !eqInt(in.PrepMinutes, ip(20)) || !eqInt(in.CookMinutes, ip(35)) || in.TotalMinutes != nil {
		t.Errorf("times = %v %v %v", show(in.PrepMinutes), show(in.CookMinutes), show(in.TotalMinutes))
	}
	if str(in.YieldText) != "16 bars" || str(in.NutritionText) != "Calories: 180 calories · Fat: 9 g" {
		t.Errorf("yield %s nutrition %s", str(in.YieldText), str(in.NutritionText))
	}
	if len(cands) != 1 || cands[0].URL != "https://example.com/img/lemon-bars-1200x800.jpg" {
		t.Errorf("images = %q", urls(cands))
	}
}

func TestExtractMicrodataSteps(t *testing.T) {
	in, cands, rep := extract(t, scanFile(t, "microdata-steps.html", "https://example.com/cornbread"))
	if rep.Method != MethodMicrodata || in.Title != "Skillet Cornbread" {
		t.Errorf("method %q title %q", rep.Method, in.Title)
	}
	if !reflect.DeepEqual(in.Ingredients, []string{"1 cup cornmeal", "1 cup buttermilk"}) {
		t.Errorf("ingredients = %q", in.Ingredients)
	}
	if !reflect.DeepEqual(in.Steps, []string{"Heat a cast-iron skillet in the oven.", "Stir everything together and bake."}) {
		t.Errorf("steps = %q", in.Steps)
	}
	if len(cands) != 1 || cands[0].URL != "https://example.com/cornbread.jpg" {
		t.Errorf("images = %q", urls(cands))
	}
}

func TestExtractHRecipe(t *testing.T) {
	p := scanFile(t, "h-recipe.html", "https://example.com/dal")
	if p.HRecipe == "" || p.Microdata != "" {
		t.Fatalf("h-recipe %q microdata %q", p.HRecipe, p.Microdata)
	}
	in, cands, rep := extract(t, p)
	if rep.Method != MethodHRecipe || in.Title != "Weeknight Dal" {
		t.Errorf("method %q title %q", rep.Method, in.Title)
	}
	if !reflect.DeepEqual(in.Ingredients, []string{"1 cup red lentils", "½ tsp turmeric", "2 cloves garlic"}) {
		t.Errorf("ingredients = %q", in.Ingredients)
	}
	want := []string{"Rinse the lentils.", "Simmer with the turmeric for 20 minutes.", "Fry the garlic and stir it in.", "Season to taste."}
	if !reflect.DeepEqual(in.Steps, want) {
		t.Errorf("steps = %q", in.Steps)
	}
	if str(in.YieldText) != "4" || !eqInt(in.TotalMinutes, ip(40)) || str(in.Description) != "Red lentils, fast." {
		t.Errorf("yield %s total %v description %s", str(in.YieldText), show(in.TotalMinutes), str(in.Description))
	}
	if str(in.NutritionText) != "250 calories · 14 g protein" {
		t.Errorf("nutrition = %s", str(in.NutritionText))
	}
	if len(cands) != 1 || cands[0].URL != "https://example.com/dal.jpg" {
		t.Errorf("images = %q", urls(cands))
	}
}

func TestExtractClassicHRecipe(t *testing.T) {
	in, _, rep := extract(t, scanFile(t, "hrecipe-classic.html", "https://example.com/pancakes"))
	if rep.Method != MethodHRecipe || in.Title != "Old-School Pancakes" {
		t.Errorf("method %q title %q", rep.Method, in.Title)
	}
	if !reflect.DeepEqual(in.Ingredients, []string{"1 cup flour", "1 egg"}) || !reflect.DeepEqual(in.Steps, []string{"Whisk everything.", "Fry in butter."}) {
		t.Errorf("ingredients %q steps %q", in.Ingredients, in.Steps)
	}
	if str(in.YieldText) != "8 pancakes" || !eqInt(in.TotalMinutes, ip(25)) {
		t.Errorf("yield %s total %v", str(in.YieldText), show(in.TotalMinutes))
	}
}

// JSON-LD wins when present; microdata is the fallback when it has no lines.
func TestExtractMethodOrder(t *testing.T) {
	b, _ := os.ReadFile("testdata/html/microdata.html")
	both := page(t, "https://example.com/r", ldScript(`{"@type":"Recipe","name":"From JSON-LD","recipeIngredient":["1 egg"]}`), string(b))
	if in, _, rep := extract(t, both); rep.Method != MethodJSONLD || in.Title != "From JSON-LD" {
		t.Errorf("method %q title %q", rep.Method, in.Title)
	}
	empty := page(t, "https://example.com/r", ldScript(`{"@type":"Recipe","name":"Empty"}`), string(b))
	if in, _, rep := extract(t, empty); rep.Method != MethodMicrodata || in.Title != "Grandma's Lemon Bars" {
		t.Errorf("method %q title %q", rep.Method, in.Title)
	}
}
