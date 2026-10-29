package importer

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/sroberts/sous/internal/recipes"
)

// Extraction methods, as ImportReport.Method gives them.
const (
	MethodJSONLD    = "json-ld"
	MethodMicrodata = "microdata"
	MethodHRecipe   = "h-recipe"
	MethodText      = "text"
)

// WarnNotFree is added when a page marked for subscribers still had a
// usable recipe.
const WarnNotFree = "The site marks this for subscribers; check it's complete."

// Candidate is a photo found on the page. Rank 0 is the best.
type Candidate struct {
	URL    string `json:"url"`
	Width  *int   `json:"width"`
	Height *int   `json:"height"`
	Rank   int    `json:"rank"`
}

// Report says how the recipe was read and what is missing.
type Report struct {
	Method   string   `json:"method"`
	Found    []string `json:"found"`
	Missing  []string `json:"missing"`
	Warnings []string `json:"warnings"`
}

// Extract reads the recipe from a scanned page: JSON-LD first, then
// microdata, then h-recipe. With no usable recipe (no ingredients and no
// steps) it returns a *recipes.ImportError with code no_recipe_found and
// the page's title and image as a fallback; Classify may refine the code.
func Extract(p Page) (recipes.RecipeInput, []Candidate, Report, error) {
	ts, orig, archived := WaybackParts(p.FetchedURL)
	ctx := &extractCtx{page: p, archived: archived, ts: ts, orig: orig}
	var blocks []any
	for _, raw := range p.JSONLD {
		if v := parseJSONLD(string(raw)); v != nil {
			blocks = append(blocks, v)
		}
	}
	g := newGraph(blocks)
	var warnings []string
	found := map[string]any(nil)
	method := ""
	if rs := g.recipes(); len(rs) > 0 {
		pick := -1
		for i, r := range rs {
			if len(listOf(g.resolve(firstOf(r, "recipeIngredient", "ingredients")))) > 0 {
				pick = i
				break
			}
		}
		if pick < 0 {
			pick = 0
		}
		if len(rs) > 1 {
			warnings = append(warnings, fmt.Sprintf("The page has %d recipes; this is the first with ingredients.", len(rs)))
		}
		found, method = rs[pick], MethodJSONLD
		ctx.g = g
	}
	in, cands, rep := ctx.mapRecipe(found, method)
	if !usable(in) && p.Microdata != "" {
		ctx.g = newGraph(nil)
		in, cands, rep = ctx.mapRecipe(microdataRecipe(p.Microdata), MethodMicrodata)
		warnings = nil
	}
	if !usable(in) && p.HRecipe != "" {
		ctx.g = newGraph(nil)
		in, cands, rep = ctx.mapRecipe(hrecipeRecipe(p.HRecipe), MethodHRecipe)
		warnings = nil
	}
	if !usable(in) {
		return recipes.RecipeInput{}, nil, Report{}, &recipes.ImportError{
			Code:    recipes.CodeNoRecipe,
			Message: Message(recipes.CodeNoRecipe),
			Page:    FallbackPage(p),
		}
	}
	rep.Warnings = append(warnings, rep.Warnings...)
	if p.has(MarkerNotFree) {
		rep.Warnings = append(rep.Warnings, WarnNotFree)
	}
	if rep.Warnings == nil {
		rep.Warnings = []string{}
	}
	return in, cands, rep, nil
}

func usable(in recipes.RecipeInput) bool {
	return len(in.Ingredients) > 0 || len(in.Steps) > 0
}

type extractCtx struct {
	page     Page
	g        *graph
	archived bool
	ts, orig string
}

func (c *extractCtx) mapRecipe(r map[string]any, method string) (recipes.RecipeInput, []Candidate, Report) {
	var in recipes.RecipeInput
	rep := Report{Method: method, Found: []string{}, Missing: []string{}}
	if r == nil {
		return in, nil, rep
	}
	g := c.g
	str := func(keys ...string) string { return stringOf(g.resolve(firstOf(r, keys...))) }

	in.Title = NormalizeTitle(str("name", "headline"))
	if in.Title == "" {
		in.Title = NormalizeTitle(c.page.OGTitle)
	}
	if in.Title == "" {
		in.Title = NormalizeTitle(c.page.Title)
	}
	if s := sourceURL(c.page, c.archived, c.orig); s != "" {
		in.SourceURL = &s
	}
	if d := cleanText(str("description")); d != "" {
		in.Description = &d
	}

	for _, v := range listOf(g.resolve(firstOf(r, "recipeIngredient", "ingredients"))) {
		for _, line := range cleanLines(stringOf(v)) {
			if line = singleParens(stripBullet(line)); line != "" {
				line = leadingDotRE.ReplaceAllString(line, "0.$1") // ".25 cup" reads as 0.25
				line, _ = ingredientHeading(line)
				in.Ingredients = append(in.Ingredients, line)
			}
		}
	}

	var steps, notes []string
	c.instructions(r["recipeInstructions"], &steps, &notes, false)
	in.Steps = Unnumber(stripStepPrefixes(splitInlineNumbered(steps)))
	if len(notes) > 0 {
		n := strings.Join(notes, "\n\n")
		in.Notes = &n
	}

	if y := yield(g, r); y != "" {
		in.YieldText = &y
	}
	in.PrepMinutes, in.CookMinutes, in.TotalMinutes, in.TimeText = Times(str("prepTime"), str("cookTime"), str("totalTime"))
	if n := nutrition(g, g.resolve(r["nutrition"])); n != "" {
		in.NutritionText = &n
	}

	cands := c.images(g.resolve(r["image"]))

	mark := func(name string, ok bool, required bool) {
		if ok {
			rep.Found = append(rep.Found, name)
		} else if required {
			rep.Missing = append(rep.Missing, name)
		}
	}
	mark("title", in.Title != "", true)
	mark("description", in.Description != nil, false)
	mark("ingredients", len(in.Ingredients) > 0, true)
	mark("steps", len(in.Steps) > 0, true)
	mark("times", in.PrepMinutes != nil || in.CookMinutes != nil || in.TotalMinutes != nil || in.TimeText != nil, true)
	mark("yield", in.YieldText != nil, true)
	mark("nutrition", in.NutritionText != nil, false)
	mark("notes", in.Notes != nil, false)
	mark("image", len(cands) > 0, true)
	return in, cands, rep
}

// firstOf is the first present key's value.
func firstOf(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return v
		}
	}
	return nil
}

// stringOf is a scalar as text, or the first scalar of a list.
func stringOf(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	case bool:
		return ""
	case []any:
		for _, x := range t {
			if s := stringOf(x); s != "" {
				return s
			}
		}
	case map[string]any:
		for _, k := range []string{"text", "name", "value", "@value"} {
			if s := stringOf(t[k]); s != "" {
				return s
			}
		}
	}
	return ""
}

func listOf(v any) []any {
	switch t := v.(type) {
	case nil:
		return nil
	case []any:
		return t
	}
	return []any{v}
}

var noteSectionRE = regexp.MustCompile(`(?i)^(?:ingredient info|cook['’]?s? notes?|notes?|tips?|recipe notes?|chef['’]?s? notes?|tips? (?:and|&) notes?)\s*:?$`)

func noteLike(name string) bool {
	name = strings.TrimSpace(name)
	return noteSectionRE.MatchString(name) || strings.HasPrefix(strings.ToLower(name), "ingredient info")
}

// instructions walks recipeInstructions in every shape sites use,
// appending steps, "# Section" headings, and notes.
func (c *extractCtx) instructions(v any, steps, notes *[]string, inNote bool) {
	out := steps
	if inNote {
		out = notes
	}
	v = c.g.resolve(v)
	switch t := v.(type) {
	case string:
		*out = append(*out, cleanLines(t)...)
	case float64:
	case []any:
		for _, x := range t {
			c.instructions(x, steps, notes, inNote)
		}
	case map[string]any:
		switch {
		case hasType(t, "howtosection"):
			name := cleanText(stringOf(t["name"]))
			if noteLike(name) {
				inNote = true
			} else if name != "" && !inNote {
				*steps = append(*steps, "# "+strings.TrimSuffix(name, ":"))
			}
			c.instructions(firstOf(t, "itemListElement", "steps", "recipeInstructions"), steps, notes, inNote)
		case hasType(t, "howtotip"):
			*notes = append(*notes, cleanLines(stringOf(firstOf(t, "text", "name")))...)
		case hasType(t, "itemlist"):
			c.instructions(t["itemListElement"], steps, notes, inNote)
		case hasType(t, "listitem"):
			c.instructions(firstOf(t, "item", "text", "name"), steps, notes, inNote)
		default:
			// HowToStep, HowToDirection, or an untyped step.
			text := stringOf(t["text"])
			if strings.TrimSpace(text) == "" {
				if el, ok := t["itemListElement"]; ok {
					c.instructions(el, steps, notes, inNote)
					return
				}
				text = stringOf(t["name"])
			}
			*out = append(*out, cleanLines(text)...)
		}
	}
}

var letterRE = regexp.MustCompile(`\pL`)

// yield prefers recipeYield, then recipeServings, then yield. From a list
// it takes the first entry with words ("4 Servings" over "4").
func yield(g *graph, r map[string]any) string {
	for _, k := range []string{"recipeYield", "recipeServings", "yield"} {
		list := listOf(g.resolve(r[k]))
		var pick string
		for _, x := range list {
			s := cleanText(stringOf(x))
			if s == "" {
				continue
			}
			if pick == "" {
				pick = s
			}
			if letterRE.MatchString(s) {
				pick = s
				break
			}
		}
		if pick != "" {
			return pick
		}
	}
	return ""
}

var nutritionLabels = []struct{ key, label string }{
	{"servingSize", "Serving"},
	{"calories", "Calories"},
	{"fatContent", "Fat"},
	{"saturatedFatContent", "Saturated fat"},
	{"transFatContent", "Trans fat"},
	{"unsaturatedFatContent", "Unsaturated fat"},
	{"cholesterolContent", "Cholesterol"},
	{"sodiumContent", "Sodium"},
	{"carbohydrateContent", "Carbohydrates"},
	{"fiberContent", "Fiber"},
	{"sugarContent", "Sugar"},
	{"proteinContent", "Protein"},
}

// nutrition joins NutritionInformation in a fixed label order; a string
// or list (h-recipe) is cleaned and joined as given.
func nutrition(g *graph, v any) string {
	var parts []string
	switch t := v.(type) {
	case map[string]any:
		for _, l := range nutritionLabels {
			if s := cleanText(stringOf(g.resolve(t[l.key]))); s != "" {
				parts = append(parts, l.label+": "+s)
			}
		}
	default:
		for _, x := range listOf(v) {
			if s := cleanText(stringOf(x)); s != "" {
				parts = append(parts, s)
			}
		}
	}
	return strings.Join(parts, " · ")
}

// images gathers photo candidates from the recipe, then og:image and
// twitter:image, and ranks them.
func (c *extractCtx) images(v any) []Candidate {
	var list []Candidate
	seen := map[string]int{}
	add := func(u string, w, h int) {
		if u = c.imageURL(u); u == "" {
			return
		}
		i, dup := seen[u]
		if !dup {
			i = len(list)
			seen[u] = i
			list = append(list, Candidate{URL: u})
		}
		if w > 0 && h > 0 && list[i].Width == nil {
			list[i].Width, list[i].Height = &w, &h
		}
	}
	var walk func(any)
	walk = func(v any) {
		v = c.g.resolve(v)
		switch t := v.(type) {
		case string:
			add(t, 0, 0)
		case []any:
			for _, x := range t {
				walk(x)
			}
		case map[string]any:
			add(stringOf(firstOf(t, "url", "contentUrl")), dimension(t["width"]), dimension(t["height"]))
		}
	}
	walk(v)
	add(c.page.OGImage, c.page.OGImageWidth, c.page.OGImageHeight)
	add(c.page.TwitterImage, 0, 0)
	return rankImages(list)
}

// imageURL resolves u against the page and points archived pages' images
// at the archive.
func (c *extractCtx) imageURL(u string) string {
	u = strings.TrimSpace(u)
	if u == "" || strings.HasPrefix(u, "data:") {
		return ""
	}
	if c.archived {
		return RewriteArchived(c.ts, c.orig, u)
	}
	base, err := url.Parse(c.page.FetchedURL)
	if err != nil {
		return u
	}
	r, err := base.Parse(u)
	if err != nil || (r.Scheme != "http" && r.Scheme != "https") {
		return ""
	}
	return r.String()
}

func dimension(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case string:
		n, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(t), "px"))
		return n
	case map[string]any:
		return dimension(t["value"])
	}
	return 0
}

var (
	dimSuffixRE = regexp.MustCompile(`-(\d{2,5})x(\d{2,5})\.[a-zA-Z]+(?:$|\?)`)
	dimPathRE   = regexp.MustCompile(`/(\d{2,5})x(\d{2,5})/`)
	dimWidthRE  = regexp.MustCompile(`[,/_]w_(\d{2,5})(?:[,/]|$)`)
	dimHeightRE = regexp.MustCompile(`[,/_]h_(\d{2,5})(?:[,/]|$)`)
)

// urlDimensions reads a size hint from an image URL: -WxH., /WxH/, w_N.
func urlDimensions(u string) (w, h int) {
	for _, re := range []*regexp.Regexp{dimSuffixRE, dimPathRE} {
		if m := re.FindStringSubmatch(u); m != nil {
			w, _ = strconv.Atoi(m[1])
			h, _ = strconv.Atoi(m[2])
			return w, h
		}
	}
	if m := dimWidthRE.FindStringSubmatch(u); m != nil {
		w, _ = strconv.Atoi(m[1])
		if m := dimHeightRE.FindStringSubmatch(u); m != nil {
			h, _ = strconv.Atoi(m[1])
		} else {
			h = w
		}
	}
	return w, h
}

const thumbMax = 600

// rankImages orders candidates by area, known dimensions first and then
// URL hints. An unhinted image ranks below larger ones but above
// thumbnails of 600px or less. Ties keep page order.
func rankImages(list []Candidate) []Candidate {
	type scored struct {
		c     Candidate
		score int
	}
	s := make([]scored, len(list))
	for i, c := range list {
		w, h := 0, 0
		if c.Width != nil && c.Height != nil {
			w, h = *c.Width, *c.Height
		} else {
			w, h = urlDimensions(c.URL)
		}
		switch {
		case w == 0 || h == 0:
			s[i] = scored{c, (thumbMax+1)*(thumbMax+1) - 1}
		case max(w, h) <= thumbMax:
			s[i] = scored{c, min(w*h, thumbMax*thumbMax)}
		default:
			s[i] = scored{c, max(w*h, (thumbMax+1)*(thumbMax+1))}
		}
	}
	sort.SliceStable(s, func(i, j int) bool { return s[i].score > s[j].score })
	out := make([]Candidate, len(s))
	for i, x := range s {
		out[i] = x.c
		out[i].Rank = i
	}
	return out
}

var trackingParams = regexp.MustCompile(`^(?:utm_.*|fbclid|gclid|mc_.*)$`)

// CleanURL drops the fragment and tracking parameters.
func CleanURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return strings.TrimSpace(raw)
	}
	u.Fragment, u.RawFragment = "", ""
	if u.RawQuery != "" {
		q := u.Query()
		for k := range q {
			if trackingParams.MatchString(strings.ToLower(k)) {
				q.Del(k)
			}
		}
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// sourceURL is the canonical URL when it is on the same host as the page
// (the original page, for an archived copy), else the fetched URL, else
// the link as given.
func sourceURL(p Page, archived bool, orig string) string {
	ref := p.FetchedURL
	if archived {
		ref = orig
	}
	if ref == "" {
		ref = p.SourceURL
	}
	if p.Canonical != "" && sameHost(p.Canonical, ref) {
		return CleanURL(p.Canonical)
	}
	if ref != "" {
		return CleanURL(ref)
	}
	return CleanURL(p.SourceURL)
}

func sameHost(a, b string) bool {
	ua, err1 := url.Parse(a)
	ub, err2 := url.Parse(b)
	if err1 != nil || err2 != nil {
		return false
	}
	h := func(u *url.URL) string { return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.") }
	return h(ua) != "" && h(ua) == h(ub)
}

// FallbackPage is what a page without a usable recipe still offers: its
// title and main image, for a pasted recipe to start from.
func FallbackPage(p Page) *recipes.ImportPage {
	ts, orig, archived := WaybackParts(p.FetchedURL)
	out := &recipes.ImportPage{SourceURL: sourceURL(p, archived, orig)}
	title := p.OGTitle
	if title == "" {
		title = p.Title
	}
	if title = cleanText(title); title != "" {
		out.Title = &title
	}
	ctx := &extractCtx{page: p, g: newGraph(nil), archived: archived, ts: ts, orig: orig}
	img := p.OGImage
	w, h := p.OGImageWidth, p.OGImageHeight
	if img == "" {
		img, w, h = p.TwitterImage, 0, 0
	}
	if u := ctx.imageURL(img); u != "" {
		c := &recipes.ImageCandidate{URL: u}
		if w > 0 && h > 0 {
			c.Width, c.Height = &w, &h
		}
		out.Image = c
	}
	return out
}
