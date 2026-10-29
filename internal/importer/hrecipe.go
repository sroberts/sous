package importer

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// hrecipeClasses maps h-recipe classes, and the classic hRecipe names, to
// schema.org properties.
var hrecipeClasses = map[string]string{
	"p-name": "name", "fn": "name",
	"p-ingredient": "recipeIngredient", "ingredient": "recipeIngredient",
	"e-instructions": "recipeInstructions", "instructions": "recipeInstructions",
	"dt-duration": "totalTime", "duration": "totalTime",
	"p-yield": "recipeYield", "yield": "recipeYield",
	"u-photo": "image", "photo": "image",
	"p-summary": "description", "summary": "description",
	"p-nutrition": "nutrition", "nutrition": "nutrition",
}

// hrecipeRecipe reads an h-recipe element into the JSON-LD shape.
// Instructions keep their HTML so they split on li, p and br like any
// other instructions string.
func hrecipeRecipe(s string) map[string]any {
	n := parseFragment(s)
	if n == nil {
		return nil
	}
	item := map[string]any{"@type": "Recipe"}
	var walk func(*html.Node)
	walk = func(c *html.Node) {
		for ; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode {
				continue
			}
			classes := strings.Fields(attr(c, "class"))
			if nestedMicroformat(classes) {
				continue
			}
			descend := true
			for _, cl := range classes {
				prop, ok := hrecipeClasses[cl]
				if !ok {
					continue
				}
				if _, set := item[prop]; set && prop != "recipeIngredient" && prop != "nutrition" && prop != "image" {
					continue
				}
				addProp(item, prop, hrecipeValue(c, prop))
				if prop == "recipeInstructions" || prop == "recipeIngredient" {
					descend = false
				}
			}
			if descend {
				walk(c.FirstChild)
			}
		}
	}
	walk(n.FirstChild)
	return item
}

// nestedMicroformat is another h-* root inside the recipe, such as an
// author's h-card.
func nestedMicroformat(classes []string) bool {
	for _, c := range classes {
		if strings.HasPrefix(c, "h-") || c == "vcard" {
			return true
		}
	}
	return false
}

func hrecipeValue(n *html.Node, prop string) string {
	switch prop {
	case "image":
		if v := attr(n, "src"); v != "" {
			return v
		}
		if v := attr(n, "href"); v != "" {
			return v
		}
		return attr(n, "data")
	case "totalTime":
		if n.DataAtom == atom.Time && attr(n, "datetime") != "" {
			return attr(n, "datetime")
		}
		if v := attr(n, "title"); v != "" && n.DataAtom == atom.Abbr {
			return v
		}
		return text(n)
	case "recipeInstructions":
		return innerHTML(n)
	}
	return text(n)
}
