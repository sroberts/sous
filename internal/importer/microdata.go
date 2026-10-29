package importer

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// parseFragment parses saved outer HTML and returns its first element.
func parseFragment(s string) *html.Node {
	ctx := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := html.ParseFragment(strings.NewReader(s), ctx)
	if err != nil {
		return nil
	}
	for _, n := range nodes {
		if n.Type == html.ElementNode {
			return n
		}
	}
	return nil
}

// innerHTML renders n's children, for values that are cleaned later.
func innerHTML(n *html.Node) string {
	var b bytes.Buffer
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		_ = html.Render(&b, c)
	}
	return b.String()
}

// microdataTypes are the nested itemscopes read into the recipe; others,
// such as an author Person, are skipped.
var microdataTypes = map[string]bool{
	"howtostep": true, "howtosection": true, "howtodirection": true, "howtotip": true,
	"nutritioninformation": true, "imageobject": true, "itemlist": true,
}

// microdataRecipe reads a Recipe itemscope into the JSON-LD shape.
func microdataRecipe(s string) map[string]any {
	n := parseFragment(s)
	if n == nil {
		return nil
	}
	return microdataItem(n)
}

func microdataItem(n *html.Node) map[string]any {
	item := map[string]any{}
	var tl []any
	for _, t := range strings.Fields(attr(n, "itemtype")) {
		t = strings.TrimSuffix(t, "/")
		if i := strings.LastIndex(t, "/"); i >= 0 {
			t = t[i+1:]
		}
		tl = append(tl, t)
	}
	if len(tl) == 1 {
		item["@type"] = tl[0]
	} else if len(tl) > 1 {
		item["@type"] = tl
	}
	var walk func(*html.Node)
	walk = func(c *html.Node) {
		for ; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode {
				continue
			}
			props := strings.Fields(attr(c, "itemprop"))
			if hasAttr(c, "itemscope") {
				if len(props) > 0 {
					sub := microdataItem(c)
					if hasType(sub, keys(microdataTypes)...) {
						for _, p := range props {
							addProp(item, p, sub)
						}
					}
				}
				continue
			}
			if len(props) > 0 {
				v := microdataValue(c)
				for _, p := range props {
					addProp(item, p, v)
				}
			}
			walk(c.FirstChild)
		}
	}
	walk(n.FirstChild)
	return item
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func addProp(item map[string]any, name string, v any) {
	name = stripSchema(name)
	switch cur := item[name].(type) {
	case nil:
		item[name] = v
	case []any:
		item[name] = append(cur, v)
	default:
		item[name] = []any{cur, v}
	}
}

func microdataValue(n *html.Node) string {
	switch n.DataAtom {
	case atom.Meta:
		return attr(n, "content")
	case atom.Img, atom.Source, atom.Video, atom.Audio, atom.Embed, atom.Iframe:
		return attr(n, "src")
	case atom.A, atom.Link, atom.Area:
		return attr(n, "href")
	case atom.Object:
		return attr(n, "data")
	case atom.Time:
		if hasAttr(n, "datetime") {
			return attr(n, "datetime")
		}
		return text(n)
	case atom.Data, atom.Meter:
		return attr(n, "value")
	}
	if hasAttr(n, "content") {
		return attr(n, "content")
	}
	return innerHTML(n)
}
