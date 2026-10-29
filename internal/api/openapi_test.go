package api

import (
	"bytes"
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

var update = flag.Bool("update", false, "rewrite testdata/openapi.golden.json")

func testDocument() *document {
	return buildDocument((&api{}).endpoints(), "test")
}

func TestOpenAPIGolden(t *testing.T) {
	got, err := json.MarshalIndent(testDocument(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	const golden = "testdata/openapi.golden.json"
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/api -run OpenAPIGolden -update)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("the OpenAPI document changed; if that's intended, rerun with -update")
	}
}

func TestOpenAPIOperations(t *testing.T) {
	seen := map[string]string{}
	for _, ep := range (&api{}).endpoints() {
		key := ep.Method + " " + ep.Path
		if ep.OpID == "" || ep.Summary == "" {
			t.Errorf("%s: missing OpID or Summary", key)
		}
		if other, dup := seen[ep.OpID]; dup {
			t.Errorf("OpID %q used by %s and %s", ep.OpID, other, key)
		}
		seen[ep.OpID] = key
		if ep.Body != nil && ep.Limit == 0 {
			t.Errorf("%s: has a body but no limit", key)
		}
	}

	doc := testDocument()
	for name, s := range doc.Components.Schemas {
		if _, err := resolveWith(s, doc.Components.Schemas); err != nil {
			t.Errorf("component %s: %v", name, err)
		}
	}
	for _, want := range []string{"Error", "StaleError", "Recipe", "Ingredient", "Summary", "SummaryPage", "ParsedLine"} {
		if doc.Components.Schemas[want] == nil {
			t.Errorf("no component %s", want)
		}
	}
}

// schemaChecker validates JSON responses against the operation that served
// them. Routes the table doesn't know (404, 405) must match Error.
type schemaChecker struct {
	doc  *document
	mux  *http.ServeMux
	ops  map[string]*operation
	mu   sync.Mutex
	seen map[*jsonschema.Schema]*jsonschema.Resolved
}

func newSchemaChecker(t *testing.T) *schemaChecker {
	doc := testDocument()
	c := &schemaChecker{doc: doc, mux: http.NewServeMux(), ops: map[string]*operation{},
		seen: map[*jsonschema.Schema]*jsonschema.Resolved{}}
	for _, ep := range (&api{}).endpoints() {
		pattern := ep.Method + " " + ep.Path
		c.mux.Handle(pattern, http.NotFoundHandler())
		c.ops[pattern] = doc.Paths[ep.Path][strings.ToLower(ep.Method)]
	}
	return c
}

func (c *schemaChecker) check(t *testing.T, req *http.Request, r *result) {
	t.Helper()
	if !strings.HasPrefix(r.header.Get("Content-Type"), "application/json") {
		return
	}
	schema := ref("Error")
	_, pattern := c.mux.Handler(req)
	if op := c.ops[pattern]; op != nil {
		resp := op.Responses[strconv.Itoa(r.status)]
		if resp == nil || resp.Content["application/json"] == nil {
			t.Errorf("%s answered %d, which its operation doesn't document", pattern, r.status)
			return
		}
		schema = resp.Content["application/json"].Schema
	}
	rs, err := c.resolve(schema)
	if err != nil {
		t.Fatalf("resolve schema for %s: %v", pattern, err)
	}
	var instance any
	if err := json.Unmarshal(r.body, &instance); err != nil {
		t.Errorf("%s %s: response is not JSON: %v", req.Method, req.URL, err)
		return
	}
	if err := rs.Validate(instance); err != nil {
		t.Errorf("%s %s → %d doesn't match its schema: %v\n%s", req.Method, req.URL, r.status, err, r.body)
	}
}

func (c *schemaChecker) resolve(s *jsonschema.Schema) (*jsonschema.Resolved, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if rs, ok := c.seen[s]; ok {
		return rs, nil
	}
	rs, err := resolveWith(s, c.doc.Components.Schemas)
	if err == nil {
		c.seen[s] = rs
	}
	return rs, err
}

// resolveWith makes s standalone: components become $defs, and refs to
// #/components/schemas point there instead.
func resolveWith(s *jsonschema.Schema, comps map[string]*jsonschema.Schema) (*jsonschema.Resolved, error) {
	rewrite := func(v any, into any) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		b = bytes.ReplaceAll(b, []byte(`"#/components/schemas/`), []byte(`"#/$defs/`))
		return json.Unmarshal(b, into)
	}
	var root jsonschema.Schema
	if err := rewrite(s, &root); err != nil {
		return nil, err
	}
	if err := rewrite(comps, &root.Defs); err != nil {
		return nil, err
	}
	return root.Resolve(nil)
}
