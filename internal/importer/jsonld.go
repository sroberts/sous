package importer

import (
	"bytes"
	"encoding/json"
	"strings"
)

// sanitizeJSON repairs what sites commonly get wrong in ld+json blocks:
// HTML comment and CDATA wrappers, and raw control characters inside
// strings.
func sanitizeJSON(s string) []byte {
	s = strings.TrimSpace(s)
	for _, w := range [][2]string{{"<!--", "-->"}, {"<![CDATA[", "]]>"}, {"/*<![CDATA[*/", "/*]]>*/"}, {"//<![CDATA[", "//]]>"}} {
		if strings.HasPrefix(s, w[0]) {
			s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(s, w[0])), w[1]))
		}
	}
	var b bytes.Buffer
	in, esc := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case !in:
			if c == '"' {
				in = true
			}
		case esc:
			esc = false
		case c == '\\':
			esc = true
		case c == '"':
			in = false
		case c == '\n':
			b.WriteString(`\n`)
			continue
		case c == '\t':
			b.WriteString(`\t`)
			continue
		case c < 0x20:
			b.WriteByte(' ')
			continue
		}
		b.WriteByte(c)
	}
	return b.Bytes()
}

// parseJSONLD decodes one block, or returns nil.
func parseJSONLD(s string) any {
	var v any
	if err := json.Unmarshal(sanitizeJSON(s), &v); err != nil {
		return nil
	}
	return normalizeKeys(v)
}

// normalizeKeys strips "schema:" and schema.org URL prefixes from keys.
func normalizeKeys(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[stripSchema(k)] = normalizeKeys(val)
		}
		return out
	case []any:
		for i := range t {
			t[i] = normalizeKeys(t[i])
		}
	}
	return v
}

func stripSchema(s string) string {
	for _, p := range []string{"schema:", "http://schema.org/", "https://schema.org/", "http://www.schema.org/", "https://www.schema.org/"} {
		if len(s) > len(p) && strings.EqualFold(s[:len(p)], p) {
			return s[len(p):]
		}
	}
	return s
}

// types is a node's @type values, lower-cased without schema prefixes.
func types(v any) []string {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	var out []string
	add := func(x any) {
		if s, ok := x.(string); ok {
			out = append(out, strings.ToLower(stripSchema(strings.TrimSpace(s))))
		}
	}
	switch t := m["@type"].(type) {
	case []any:
		for _, x := range t {
			add(x)
		}
	default:
		add(t)
	}
	return out
}

func hasType(v any, names ...string) bool {
	for _, t := range types(v) {
		for _, n := range names {
			if t == n {
				return true
			}
		}
	}
	return false
}

func isRecipe(v any) bool { return hasType(v, "recipe") }

// graph is the ld+json blocks of a page, walked: every node in document
// order, and the nodes with an @id.
type graph struct {
	nodes []map[string]any
	ids   map[string]map[string]any
}

func newGraph(blocks []any) *graph {
	g := &graph{ids: map[string]map[string]any{}}
	for _, b := range blocks {
		g.walk(b)
	}
	return g
}

func (g *graph) walk(v any) {
	switch t := v.(type) {
	case []any:
		for _, x := range t {
			g.walk(x)
		}
	case map[string]any:
		if len(types(t)) > 0 {
			g.nodes = append(g.nodes, t)
		}
		if id, ok := t["@id"].(string); ok && len(t) > 1 {
			if _, seen := g.ids[id]; !seen {
				g.ids[id] = t
			}
		}
		for _, k := range []string{"@graph", "mainEntity", "mainEntityOfPage", "hasPart", "itemListElement"} {
			if x, ok := t[k]; ok {
				if k == "itemListElement" && !hasType(t, "itemlist") {
					continue
				}
				g.walk(x)
			}
		}
		if hasType(t, "listitem") {
			g.walk(t["item"])
		}
	}
}

// resolve replaces an {"@id": …}-only reference with the node it names.
func (g *graph) resolve(v any) any {
	if m, ok := v.(map[string]any); ok && len(m) == 1 {
		if id, ok := m["@id"].(string); ok {
			if n, ok := g.ids[id]; ok {
				return n
			}
		}
	}
	return v
}

// recipes is every Recipe node, once each, in document order.
func (g *graph) recipes() []map[string]any {
	var out []map[string]any
	for _, n := range g.nodes {
		if !isRecipe(n) {
			continue
		}
		dup := false
		for _, o := range out {
			if sameNode(o, n) {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, n)
		}
	}
	return out
}

func sameNode(a, b map[string]any) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	ia, _ := a["@id"].(string)
	ib, _ := b["@id"].(string)
	if ia != "" && ia == ib {
		return true
	}
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return bytes.Equal(ja, jb)
}

// containsRecipe reports whether v is or holds a Recipe node.
func containsRecipe(v any) bool {
	return len(newGraph([]any{v}).recipes()) > 0
}

// notFree reports whether any node says isAccessibleForFree is false.
func notFree(v any) bool {
	switch t := v.(type) {
	case []any:
		for _, x := range t {
			if notFree(x) {
				return true
			}
		}
	case map[string]any:
		switch f := t["isAccessibleForFree"].(type) {
		case bool:
			if !f {
				return true
			}
		case string:
			if strings.EqualFold(strings.TrimSpace(f), "false") {
				return true
			}
		}
		for _, x := range t {
			if notFree(x) {
				return true
			}
		}
	}
	return false
}

// pruneJSONLD keeps what a fixture needs from a block that holds a
// Recipe: @graph or array members that are not, don't hold, and aren't
// referenced by a recipe are stubbed to their @type and @id.
func pruneJSONLD(v any) any {
	members := func(v any) []any {
		switch t := v.(type) {
		case []any:
			return t
		case map[string]any:
			if gr, ok := t["@graph"].([]any); ok {
				return gr
			}
		}
		return nil
	}
	list := members(v)
	if list == nil {
		return v
	}
	keep := map[int]bool{}
	refs := map[string]bool{}
	for i, x := range list {
		if containsRecipe(x) {
			keep[i] = true
			collectRefs(x, refs)
		}
	}
	for changed := true; changed; {
		changed = false
		for i, x := range list {
			if keep[i] {
				continue
			}
			if m, ok := x.(map[string]any); ok {
				if id, _ := m["@id"].(string); id != "" && refs[id] {
					keep[i] = true
					collectRefs(x, refs)
					changed = true
				}
			}
		}
	}
	out := make([]any, len(list))
	for i, x := range list {
		out[i] = x
		if keep[i] {
			continue
		}
		stub := map[string]any{}
		if m, ok := x.(map[string]any); ok {
			for _, k := range []string{"@type", "@id"} {
				if val, ok := m[k]; ok {
					stub[k] = val
				}
			}
		}
		out[i] = stub
	}
	if m, ok := v.(map[string]any); ok {
		c := make(map[string]any, len(m))
		for k, val := range m {
			c[k] = val
		}
		c["@graph"] = out
		return c
	}
	return out
}

func collectRefs(v any, refs map[string]bool) {
	switch t := v.(type) {
	case []any:
		for _, x := range t {
			collectRefs(x, refs)
		}
	case map[string]any:
		if id, ok := t["@id"].(string); ok && len(t) == 1 {
			refs[id] = true
		}
		for _, x := range t {
			collectRefs(x, refs)
		}
	}
}
