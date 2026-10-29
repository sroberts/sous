package importer

import (
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/sroberts/sous/internal/recipes"
)

// page wraps head and body markup in a minimal document and scans it.
func page(t *testing.T, rawURL, head, body string) Page {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	doc := "<!doctype html><html><head>" + head + "</head><body>" + body + "</body></html>"
	return Scan([]byte(doc), "text/html; charset=utf-8", u)
}

func ldScript(js string) string {
	return `<script type="application/ld+json">` + js + `</script>`
}

func extract(t *testing.T, p Page) (recipes.RecipeInput, []Candidate, Report) {
	t.Helper()
	in, cands, rep, err := Extract(p)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	return in, cands, rep
}

func str(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func urls(cs []Candidate) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.URL
	}
	return out
}

// Half Baked Harvest: an @graph, a yield list, four sized image strings,
// HowToStep name == text, and a title ending in a period.
const hbhJSON = `{"@context":"https://schema.org","@graph":[
 {"@type":"Article","@id":"https://www.halfbakedharvest.com/basil-chicken-curry-zucchini-noodle-bowl/#article","headline":"Basil Chicken Curry Zucchini Noodle Bowls.","author":{"@id":"https://www.halfbakedharvest.com/#/schema/person/1"}},
 {"@type":"WebPage","@id":"https://www.halfbakedharvest.com/basil-chicken-curry-zucchini-noodle-bowl/","name":"Basil Chicken Curry Zucchini Noodle Bowls. - Half Baked Harvest","breadcrumb":{"@id":"#breadcrumb"}},
 {"@type":"Person","@id":"https://www.halfbakedharvest.com/#/schema/person/1","name":"Tieghan"},
 {"@type":"BreadcrumbList","@id":"#breadcrumb","itemListElement":[{"@type":"ListItem","position":1,"name":"Home"}]},
 {"@type":"Recipe","@id":"https://www.halfbakedharvest.com/basil-chicken-curry-zucchini-noodle-bowl/#recipe",
  "name":"Basil Chicken Curry Zucchini Noodle Bowls.",
  "author":{"@id":"https://www.halfbakedharvest.com/#/schema/person/1"},
  "description":"Basil Chicken Curry Zucchini Noodle Bowls &#8211; quick &amp; healthy.",
  "image":["https://www.halfbakedharvest.com/wp-content/uploads/2024/05/Basil-Chicken-Curry-1-500x500.jpg","https://www.halfbakedharvest.com/wp-content/uploads/2024/05/Basil-Chicken-Curry-1-500x375.jpg","https://www.halfbakedharvest.com/wp-content/uploads/2024/05/Basil-Chicken-Curry-1-480x270.jpg","https://www.halfbakedharvest.com/wp-content/uploads/2024/05/Basil-Chicken-Curry-1.jpg"],
  "recipeYield":["4","4 Servings"],
  "prepTime":"PT15M","cookTime":"PT20M","totalTime":"PT35M",
  "recipeIngredient":["2 tablespoons coconut oil","1 pound boneless chicken, chopped","&#9634; 1/4 cup thai basil"],
  "recipeInstructions":[
   {"@type":"HowToStep","name":"1. Heat the oil in a large skillet.","text":"1. Heat the oil in a large skillet.","url":"https://www.halfbakedharvest.com/x/#step1"},
   {"@type":"HowToStep","name":"2. Add the chicken and cook until golden.","text":"2. Add the chicken and cook until golden.","url":"https://www.halfbakedharvest.com/x/#step2"}],
  "nutrition":{"@type":"NutritionInformation","proteinContent":"30 g","calories":"420 kcal","fatContent":"18 g"},
  "isAccessibleForFree":true}
]}`

func TestExtractHalfBakedHarvest(t *testing.T) {
	p := page(t, "https://www.halfbakedharvest.com/basil-chicken-curry-zucchini-noodle-bowl/?utm_source=feed",
		`<title>Basil Chicken Curry Zucchini Noodle Bowls. - Half Baked Harvest</title>
		<link rel="canonical" href="https://www.halfbakedharvest.com/basil-chicken-curry-zucchini-noodle-bowl/">
		<meta property="og:image" content="https://www.halfbakedharvest.com/wp-content/uploads/2024/05/Basil-Chicken-Curry-1.jpg">
		<meta property="og:image:width" content="1200"><meta property="og:image:height" content="1800">`+ldScript(hbhJSON), "")
	in, cands, rep := extract(t, p)

	if in.Title != "Basil Chicken Curry Zucchini Noodle Bowls" {
		t.Errorf("title = %q", in.Title)
	}
	if str(in.SourceURL) != "https://www.halfbakedharvest.com/basil-chicken-curry-zucchini-noodle-bowl/" {
		t.Errorf("source = %s", str(in.SourceURL))
	}
	if str(in.Description) != "Basil Chicken Curry Zucchini Noodle Bowls – quick & healthy." {
		t.Errorf("description = %s", str(in.Description))
	}
	if str(in.YieldText) != "4 Servings" {
		t.Errorf("yield = %s", str(in.YieldText))
	}
	if !eqInt(in.PrepMinutes, ip(15)) || !eqInt(in.CookMinutes, ip(20)) || !eqInt(in.TotalMinutes, ip(35)) || in.TimeText != nil {
		t.Errorf("times = %v %v %v %v", show(in.PrepMinutes), show(in.CookMinutes), show(in.TotalMinutes), showS(in.TimeText))
	}
	wantIng := []string{"2 tablespoons coconut oil", "1 pound boneless chicken, chopped", "1/4 cup thai basil"}
	if !reflect.DeepEqual(in.Ingredients, wantIng) {
		t.Errorf("ingredients = %q", in.Ingredients)
	}
	// "1." numbering is left for ParseStepLines; name == text is not doubled.
	wantSteps := []string{"1. Heat the oil in a large skillet.", "2. Add the chicken and cook until golden."}
	if !reflect.DeepEqual(in.Steps, wantSteps) {
		t.Errorf("steps = %q", in.Steps)
	}
	if str(in.NutritionText) != "Calories: 420 kcal · Fat: 18 g · Protein: 30 g" {
		t.Errorf("nutrition = %s", str(in.NutritionText))
	}
	if in.Tags != nil {
		t.Errorf("tags = %q; none are imported", in.Tags)
	}
	if cands[0].URL != "https://www.halfbakedharvest.com/wp-content/uploads/2024/05/Basil-Chicken-Curry-1.jpg" ||
		cands[0].Width == nil || *cands[0].Width != 1200 {
		t.Errorf("best image = %+v", cands[0])
	}
	if len(cands) != 4 || !strings.HasSuffix(cands[1].URL, "-500x500.jpg") || !strings.HasSuffix(cands[3].URL, "-480x270.jpg") {
		t.Errorf("images = %q", urls(cands))
	}
	if rep.Method != MethodJSONLD || len(rep.Missing) != 0 || len(rep.Warnings) != 0 {
		t.Errorf("report = %+v", rep)
	}
}

// Serious Eats: a top-level array, a type list, ImageObjects with sizes,
// steps that carry images, and a title ending in " Recipe".
const seriousEatsJSON = `[{"@context":"http://schema.org","@type":["Recipe"],
 "name":"Lexington-Style Red Coleslaw Recipe",
 "image":[{"@type":"ImageObject","url":"https://www.seriouseats.com/thmb/a=/1500x0/slaw-hero.jpg","height":1125,"width":1500},
          {"@type":"ImageObject","url":"https://www.seriouseats.com/thmb/b=/750x0/slaw-hero.jpg","height":"563","width":"750"}],
 "recipeYield":["10"],
 "prepTime":"PT10M","totalTime":"PT70M",
 "recipeIngredient":["1 medium head green cabbage","1/2 cup ketchup"],
 "recipeInstructions":[
  {"@type":"HowToStep","text":"In a large bowl, toss cabbage with salt.","image":[{"@type":"ImageObject","url":"https://www.seriouseats.com/thmb/step1.jpg"}]},
  {"@type":"HowToStep","text":"Stir in ketchup &amp; vinegar."}]}]`

func TestExtractSeriousEats(t *testing.T) {
	p := page(t, "https://www.seriouseats.com/lexington-style-red-coleslaw-recipe", ldScript(seriousEatsJSON), "")
	in, cands, rep := extract(t, p)
	if in.Title != "Lexington-Style Red Coleslaw" {
		t.Errorf("title = %q", in.Title)
	}
	if !reflect.DeepEqual(in.Steps, []string{"In a large bowl, toss cabbage with salt.", "Stir in ketchup & vinegar."}) {
		t.Errorf("steps = %q", in.Steps)
	}
	if str(in.YieldText) != "10" || !eqInt(in.TotalMinutes, ip(70)) || in.CookMinutes != nil {
		t.Errorf("yield %s total %v cook %v", str(in.YieldText), show(in.TotalMinutes), show(in.CookMinutes))
	}
	if len(cands) != 2 || *cands[0].Width != 1500 || *cands[1].Height != 563 {
		t.Errorf("images = %+v", cands)
	}
	if rep.Method != MethodJSONLD {
		t.Errorf("method = %q", rep.Method)
	}
}

// Bon Appétit: HowToSections, one of them "ingredient info:" notes, a
// double-spaced yield and no times.
const bonAppetitJSON = `{"@context":"http://schema.org","@type":"Recipe",
 "name":"Albóndigas in Red Chile Sauce",
 "recipeYield":"8  Servings",
 "recipeIngredient":["4 dried ancho chiles","1½ lb. ground pork"],
 "recipeInstructions":[
  {"@type":"HowToSection","name":"Sauce","itemListElement":[{"@type":"HowToStep","text":"<p>Toast chiles in a dry skillet.</p>"}]},
  {"@type":"HowToSection","name":"Meatballs","itemListElement":[{"@type":"HowToStep","text":"Mix pork and rice."},{"@type":"HowToStep","text":"Roll into balls."}]},
  {"@type":"HowToSection","name":"ingredient info:","itemListElement":[{"@type":"HowToStep","text":"Ancho chiles can be found at Latin markets."}]}]}`

func TestExtractBonAppetit(t *testing.T) {
	p := page(t, "https://www.bonappetit.com/recipe/albondigas-in-red-chile-sauce", ldScript(bonAppetitJSON), "")
	in, _, rep := extract(t, p)
	if in.Title != "Albóndigas in Red Chile Sauce" {
		t.Errorf("title = %q", in.Title)
	}
	want := []string{"# Sauce", "Toast chiles in a dry skillet.", "# Meatballs", "Mix pork and rice.", "Roll into balls."}
	if !reflect.DeepEqual(in.Steps, want) {
		t.Errorf("steps = %q", in.Steps)
	}
	if str(in.Notes) != "Ancho chiles can be found at Latin markets." {
		t.Errorf("notes = %s", str(in.Notes))
	}
	if str(in.YieldText) != "8 Servings" {
		t.Errorf("yield = %q", str(in.YieldText))
	}
	if in.PrepMinutes != nil || in.CookMinutes != nil || in.TotalMinutes != nil || in.TimeText != nil {
		t.Error("times invented")
	}
	if !reflect.DeepEqual(rep.Missing, []string{"times", "image"}) {
		t.Errorf("missing = %q", rep.Missing)
	}
}

func TestExtractInstructionShapes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		instr string
		steps []string
		notes string
	}{
		{"html string", `"<p>Heat oil.</p><p>Add onions.<br>Stir.</p>"`, []string{"Heat oil.", "Add onions.", "Stir."}, ""},
		{"newline string", `"Heat oil.\nAdd onions."`, []string{"Heat oil.", "Add onions."}, ""},
		{"string list", `["Heat oil.","Add onions."]`, []string{"Heat oil.", "Add onions."}, ""},
		{"single step object", `{"@type":"HowToStep","text":"Do it all."}`, []string{"Do it all."}, ""},
		{"text over name", `[{"@type":"HowToStep","name":"Heat","text":"Heat the oil."}]`, []string{"Heat the oil."}, ""},
		{"name when text empty", `[{"@type":"HowToStep","name":"Heat the oil.","text":""}]`, []string{"Heat the oil."}, ""},
		{"how to direction", `[{"@type":"HowToDirection","text":"Whisk."}]`, []string{"Whisk."}, ""},
		{"step holding directions", `[{"@type":"HowToStep","itemListElement":[{"@type":"HowToDirection","text":"Whisk."},{"@type":"HowToTip","text":"Don't overmix."}]}]`, []string{"Whisk."}, "Don't overmix."},
		{"item list", `{"@type":"ItemList","itemListElement":[{"@type":"ListItem","position":1,"item":{"@type":"HowToStep","text":"One."}},{"@type":"ListItem","position":2,"text":"Two."}]}`, []string{"One.", "Two."}, ""},
		{"nested sections", `[{"@type":"HowToSection","name":"Dough:","itemListElement":[{"@type":"HowToSection","name":"Rest","itemListElement":["Cover."]}]}]`, []string{"# Dough", "# Rest", "Cover."}, ""},
		{"tip to notes", `[{"@type":"HowToStep","text":"Bake."},{"@type":"HowToTip","text":"Use a dark pan."}]`, []string{"Bake."}, "Use a dark pan."},
		{"cook's notes section", `[{"@type":"HowToSection","name":"Cook’s Notes","itemListElement":["Keeps 3 days."]},"Serve."]`, []string{"Serve."}, "Keeps 3 days."},
		{"tips section", `[{"@type":"HowToSection","name":"Tips","itemListElement":[{"@type":"HowToStep","text":"Chill first."}]},"Serve."]`, []string{"Serve."}, "Chill first."},
		{"step prefixes", `["Step 1: Heat.","Step 2: Mix."]`, []string{"Heat.", "Mix."}, ""},
		{"mela numbering", `["1 Heat the oven.","2 Mix.","3 Bake."]`, []string{"Heat the oven.", "Mix.", "Bake."}, ""},
		{"quantity step", `["Whisk flour.","2 eggs go in next."]`, []string{"Whisk flour.", "2 eggs go in next."}, ""},
		{"schema prefixed types", `[{"@type":"schema:HowToStep","schema:text":"Heat."},{"@type":"http://schema.org/HowToStep","text":"Mix."}]`, []string{"Heat.", "Mix."}, ""},
		{"entities", `["Mac &amp;amp; cheese &#8211; bake."]`, []string{"Mac & cheese – bake."}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			js := `{"@type":"Recipe","name":"T","recipeIngredient":["1 egg"],"recipeInstructions":` + tc.instr + `}`
			in, _, _ := extract(t, page(t, "https://example.com/r", ldScript(js), ""))
			if !reflect.DeepEqual(in.Steps, tc.steps) {
				t.Errorf("steps = %q; want %q", in.Steps, tc.steps)
			}
			if got := strings.TrimPrefix(str(in.Notes), "<nil>"); got != tc.notes {
				t.Errorf("notes = %q; want %q", got, tc.notes)
			}
		})
	}
}

func TestExtractYield(t *testing.T) {
	for _, tc := range []struct{ field, want string }{
		{`"recipeYield":"8  Servings"`, "8 Servings"},
		{`"recipeYield":["4","4 Servings"]`, "4 Servings"},
		{`"recipeYield":["6"]`, "6"},
		{`"recipeYield":4`, "4"},
		{`"recipeYield":2.5`, "2.5"},
		{`"recipeYield":"Makes about 2&nbsp;cups"`, "Makes about 2 cups"},
		{`"recipeServings":"6"`, "6"},
		{`"yield":"1 loaf"`, "1 loaf"},
		{`"recipeYield":"","recipeServings":"3"`, "3"},
		{`"recipeYield":[]`, "<nil>"},
	} {
		js := `{"@type":"Recipe","name":"T","recipeIngredient":["1 egg"],` + tc.field + `}`
		in, _, _ := extract(t, page(t, "https://example.com/r", ldScript(js), ""))
		if got := str(in.YieldText); got != tc.want {
			t.Errorf("%s: yield = %q; want %q", tc.field, got, tc.want)
		}
	}
}

func TestExtractDocumentShapes(t *testing.T) {
	for _, tc := range []struct {
		name, head, title string
	}{
		{"graph", ldScript(`{"@context":"https://schema.org","@graph":[{"@type":"WebSite","@id":"#site"},{"@type":"Recipe","name":"Graph","recipeIngredient":["1 egg"]}]}`), "Graph"},
		{"top-level array", ldScript(`[{"@type":"Organization","name":"X"},{"@type":"Recipe","name":"Array","recipeIngredient":["1 egg"]}]`), "Array"},
		{"type list", ldScript(`{"@type":["Recipe","NewsArticle"],"name":"Listed","recipeIngredient":["1 egg"]}`), "Listed"},
		{"lower-case type", ldScript(`{"@type":"recipe","name":"Lower","recipeIngredient":["1 egg"]}`), "Lower"},
		{"schema prefix", ldScript(`{"@context":{"schema":"https://schema.org/"},"@type":"schema:Recipe","schema:name":"Prefixed","schema:recipeIngredient":["1 egg"]}`), "Prefixed"},
		{"schema URL type", ldScript(`{"@type":"https://schema.org/Recipe","http://schema.org/name":"URL","recipeIngredient":["1 egg"]}`), "URL"},
		{"main entity", ldScript(`{"@type":"WebPage","mainEntity":{"@type":"Recipe","name":"Main","recipeIngredient":["1 egg"]}}`), "Main"},
		{"second block", ldScript(`{"@type":"Organization","name":"X"}`) + ldScript(`{"@type":"Recipe","name":"Second","recipeIngredient":["1 egg"]}`), "Second"},
		{"html comment wrapper", ldScript(`<!-- {"@type":"Recipe","name":"Commented","recipeIngredient":["1 egg"]} -->`), "Commented"},
		{"cdata wrapper", ldScript(`//<![CDATA[
{"@type":"Recipe","name":"CDATA","recipeIngredient":["1 egg"]}
//]]>`), "CDATA"},
		{"raw newline in string", ldScript("{\"@type\":\"Recipe\",\"name\":\"Raw\",\"recipeIngredient\":[\"1 egg\"],\"recipeInstructions\":\"Line one.\nLine two.\"}"), "Raw"},
		{"no name uses og:title", `<meta property="og:title" content="From OG Recipe">` + ldScript(`{"@type":"Recipe","recipeIngredient":["1 egg"]}`), "From OG"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in, _, rep := extract(t, page(t, "https://example.com/r", tc.head, ""))
			if in.Title != tc.title || rep.Method != MethodJSONLD {
				t.Errorf("title = %q, method %q; want %q", in.Title, rep.Method, tc.title)
			}
		})
	}
}

func TestExtractIDReferences(t *testing.T) {
	js := `{"@context":"https://schema.org","@graph":[
	 {"@type":"Recipe","@id":"#recipe","name":"Refs","image":{"@id":"#primaryimage"},
	  "recipeIngredient":["1 egg"],"recipeInstructions":{"@id":"#steps"},"nutrition":{"@id":"#nut"}},
	 {"@type":"ImageObject","@id":"#primaryimage","url":"https://example.com/hero.jpg","width":1200,"height":800},
	 {"@type":"ItemList","@id":"#steps","itemListElement":[{"@type":"HowToStep","text":"Fry."}]},
	 {"@type":"NutritionInformation","@id":"#nut","calories":"100"},
	 {"@type":"WebSite","@id":"#site","name":"Big site","description":"lots of text"}]}`
	p := page(t, "https://example.com/r", ldScript(js), "")
	in, cands, _ := extract(t, p)
	if len(cands) != 1 || cands[0].URL != "https://example.com/hero.jpg" || *cands[0].Width != 1200 {
		t.Errorf("images = %+v", cands)
	}
	if !reflect.DeepEqual(in.Steps, []string{"Fry."}) || str(in.NutritionText) != "Calories: 100" {
		t.Errorf("steps %q nutrition %s", in.Steps, str(in.NutritionText))
	}
	// The fixture keeps referenced nodes and stubs the rest.
	b, _ := json.Marshal(p.JSONLD)
	if !strings.Contains(string(b), "hero.jpg") || strings.Contains(string(b), "lots of text") {
		t.Errorf("pruned JSON-LD = %s", b)
	}
}

func TestExtractImages(t *testing.T) {
	for _, tc := range []struct {
		name, head, image string
		want              []string
	}{
		{"string", "", `"https://example.com/a.jpg"`, []string{"https://example.com/a.jpg"}},
		{"relative", "", `"/img/a.jpg"`, []string{"https://example.com/img/a.jpg"}},
		{"contentUrl", "", `{"@type":"ImageObject","contentUrl":"https://example.com/c.jpg"}`, []string{"https://example.com/c.jpg"}},
		{"largest known first", "", `[{"@type":"ImageObject","url":"https://example.com/small.jpg","width":400,"height":300},{"@type":"ImageObject","url":"https://example.com/big.jpg","width":"1600","height":"900"}]`,
			[]string{"https://example.com/big.jpg", "https://example.com/small.jpg"}},
		{"url hints", "", `["https://example.com/a-300x300.jpg","https://example.com/a-1200x800.jpg","https://example.com/a.jpg"]`,
			[]string{"https://example.com/a-1200x800.jpg", "https://example.com/a.jpg", "https://example.com/a-300x300.jpg"}},
		{"cloudinary and path hints", "", `["https://res.example.com/image/upload/w_320,h_240/x.jpg","https://img.example.com/1600x900/x.jpg"]`,
			[]string{"https://img.example.com/1600x900/x.jpg", "https://res.example.com/image/upload/w_320,h_240/x.jpg"}},
		{"og and twitter after recipe", `<meta property="og:image" content="/og.jpg"><meta name="twitter:image" content="https://example.com/tw.jpg">`, `"https://example.com/a.jpg"`,
			[]string{"https://example.com/a.jpg", "https://example.com/og.jpg", "https://example.com/tw.jpg"}},
		{"og with size beats unhinted", `<meta property="og:image" content="https://example.com/og.jpg"><meta property="og:image:width" content="1200"><meta property="og:image:height" content="630">`, `"https://example.com/a.jpg"`,
			[]string{"https://example.com/og.jpg", "https://example.com/a.jpg"}},
		{"deduplicated", `<meta property="og:image" content="https://example.com/a.jpg">`, `["https://example.com/a.jpg","https://example.com/a.jpg"]`, []string{"https://example.com/a.jpg"}},
		{"data URI skipped", "", `["data:image/gif;base64,R0lGOD","https://example.com/a.jpg"]`, []string{"https://example.com/a.jpg"}},
		{"none", "", `null`, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			js := `{"@type":"Recipe","name":"T","recipeIngredient":["1 egg"],"image":` + tc.image + `}`
			_, cands, rep := extract(t, page(t, "https://example.com/r", tc.head+ldScript(js), ""))
			if got := urls(cands); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("images = %q; want %q", got, tc.want)
			}
			for i, c := range cands {
				if c.Rank != i {
					t.Errorf("rank %d at %d", c.Rank, i)
				}
			}
			if hasImage := !contains(rep.Missing, "image"); hasImage != (len(tc.want) > 0) {
				t.Errorf("missing = %q", rep.Missing)
			}
		})
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestExtractSeveralRecipes(t *testing.T) {
	js := `[{"@type":"Recipe","name":"Teaser"},{"@type":"Recipe","name":"The Real One","recipeIngredient":["1 egg"]},{"@type":"Recipe","name":"Another","recipeIngredient":["2 eggs"]}]`
	in, _, rep := extract(t, page(t, "https://example.com/r", ldScript(js), ""))
	if in.Title != "The Real One" {
		t.Errorf("title = %q", in.Title)
	}
	if len(rep.Warnings) != 1 || !strings.Contains(rep.Warnings[0], "3 recipes") {
		t.Errorf("warnings = %q", rep.Warnings)
	}
}

func TestExtractNoRecipe(t *testing.T) {
	p := page(t, "https://www.reddit.com/r/tumblr/comments/jy26xm/all_hail_the_gatorade_bread/",
		`<title>Reddit</title><meta property="og:title" content="All hail the Gatorade bread : r/tumblr">
		<meta property="og:image" content="https://preview.redd.it/bread.png">`+ldScript(`{"@type":"DiscussionForumPosting","headline":"x"}`),
		"<p>Gatorade bread is real.</p>")
	_, cands, _, err := Extract(p)
	var ie *recipes.ImportError
	if !errors.As(err, &ie) || ie.Code != recipes.CodeNoRecipe || ie.Message == "" {
		t.Fatalf("err = %v", err)
	}
	if cands != nil || ie.Page == nil || str(ie.Page.Title) != "All hail the Gatorade bread : r/tumblr" ||
		ie.Page.Image == nil || ie.Page.Image.URL != "https://preview.redd.it/bread.png" ||
		ie.Page.SourceURL != "https://www.reddit.com/r/tumblr/comments/jy26xm/all_hail_the_gatorade_bread/" {
		t.Errorf("page = %+v", ie.Page)
	}
	if len(p.JSONLD) != 0 {
		t.Errorf("non-recipe JSON-LD kept: %s", p.JSONLD)
	}
}

func TestExtractRecipeWithoutLinesIsNoRecipe(t *testing.T) {
	_, _, _, err := Extract(page(t, "https://example.com/r", ldScript(`{"@type":"Recipe","name":"Only a name","image":"https://example.com/a.jpg"}`), ""))
	var ie *recipes.ImportError
	if !errors.As(err, &ie) || ie.Code != recipes.CodeNoRecipe {
		t.Fatalf("err = %v", err)
	}
}

// NYT and ATK mark a full recipe isAccessibleForFree: false. It imports,
// with a warning.
func TestExtractNotFreeWithRecipe(t *testing.T) {
	js := `{"@context":"https://schema.org","@type":"Recipe","name":"Creamy, Spicy Tomato Beans and Greens","isAccessibleForFree":false,
	 "hasPart":{"@type":"WebPageElement","isAccessibleForFree":"False","cssSelector":".paywall"},
	 "recipeIngredient":["2 cans beans"],"recipeInstructions":[{"@type":"HowToStep","text":"Simmer."}],"totalTime":"PT30M","recipeYield":"4 servings"}`
	p := page(t, "https://cooking.nytimes.com/recipes/1025325-creamy-spicy-tomato-beans-and-greens", ldScript(js), "")
	if !p.has(MarkerNotFree) {
		t.Fatalf("markers = %q", p.Markers)
	}
	in, _, rep := extract(t, p)
	if in.Title != "Creamy, Spicy Tomato Beans and Greens" || !reflect.DeepEqual(rep.Warnings, []string{WarnNotFree}) {
		t.Errorf("title %q warnings %q", in.Title, rep.Warnings)
	}
	if code := Classify(200, nil, p, true); code != "" {
		t.Errorf("Classify = %q", code)
	}
}

// The Wayback copy of MyRecipes: the source is the original page, and
// image URLs point at the archive's raw copies.
func TestExtractWayback(t *testing.T) {
	const ts = "20231120222809"
	const orig = "https://www.myrecipes.com/recipe/curried-lentil-tomato-soup"
	js := `{"@context":"http://schema.org","@type":"Recipe","name":"Curried Lentil-Tomato Soup",
	 "image":{"@type":"ImageObject","url":"/web/20231120222809/https://imagesvc.meredithcorp.io/v3/mm/image?url=https%3A%2F%2Fstatic.onecms.io%2Fsoup.jpg","width":2000,"height":2000},
	 "recipeIngredient":["1 cup lentils"],"recipeInstructions":"Simmer everything."}`
	p := page(t, WaybackFetchURL(ts, orig),
		`<link rel="canonical" href="https://www.myrecipes.com/recipe/curried-lentil-tomato-soup?utm_medium=x#top">
		<meta property="og:image" content="https://web.archive.org/web/20231120222809im_/https://imagesvc.meredithcorp.io/og.jpg">`+ldScript(js), "")
	in, cands, _ := extract(t, p)
	if str(in.SourceURL) != orig {
		t.Errorf("source = %s", str(in.SourceURL))
	}
	want := []string{
		"https://web.archive.org/web/20231120222809im_/https://imagesvc.meredithcorp.io/v3/mm/image?url=https%3A%2F%2Fstatic.onecms.io%2Fsoup.jpg",
		"https://web.archive.org/web/20231120222809im_/https://imagesvc.meredithcorp.io/og.jpg",
	}
	if !reflect.DeepEqual(urls(cands), want) {
		t.Errorf("images = %q", urls(cands))
	}
}

func TestSourceURL(t *testing.T) {
	for _, tc := range []struct {
		name, fetched, canonical, want string
	}{
		{"canonical same host", "https://www.example.com/r?utm_source=x", "https://example.com/r", "https://example.com/r"},
		{"canonical other host", "https://m.example.com/r", "https://other.com/r", "https://m.example.com/r"},
		{"tracking cleaned", "https://example.com/r?id=3&utm_campaign=a&fbclid=b&gclid=c&mc_eid=d#comments", "", "https://example.com/r?id=3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := Page{SourceURL: "https://example.com/given", FetchedURL: tc.fetched, Canonical: tc.canonical}
			if got := sourceURL(p, false, ""); got != tc.want {
				t.Errorf("sourceURL = %q; want %q", got, tc.want)
			}
		})
	}
}
