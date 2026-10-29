package api

import (
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/recipes"
)

// payload is a request or response body. typ is nil for one with no Go
// type; media nil means application/json.
type payload struct {
	typ    reflect.Type
	schema func() *jsonschema.Schema
	media  []string
}

func jsonOf[T any]() *payload {
	return &payload{typ: reflect.TypeFor[T](), schema: recipes.SchemaFor[T]}
}

var (
	imagePayload   = &payload{media: []string{"image/jpeg", "image/png", "image/webp", "image/heic"}}
	openapiPayload = &payload{schema: func() *jsonschema.Schema { return &jsonschema.Schema{Type: "object"} }}
)

// componentNames names the API's own types in components/schemas.
var componentNames = map[reflect.Type]string{
	reflect.TypeFor[errorBody]():   "Error",
	reflect.TypeFor[errorDetail](): "ErrorDetail",
	reflect.TypeFor[staleBody]():   "StaleError",
}

const docDescription = `Sous is a household recipe manager. Every response is JSON except images.

Errors are always {"error":{"code","message"}}; branch on code, which is stable.

Identity: behind the Outpost gateway, the caller comes from its signed token. Otherwise
name yourself with the X-Sous-Person header (a username from GET /v1/people). Reads need
no person; writes from nobody get 401 who_are_you.

Concurrency: recipes carry a version, returned as the ETag. PATCH and DELETE need
If-Match with that version; a stale one gets 412 with the current recipe.

Lists page by opaque cursor: pass next_cursor back as cursor until it is null.`

type document struct {
	OpenAPI    string                           `json:"openapi"`
	Info       info                             `json:"info"`
	Paths      map[string]map[string]*operation `json:"paths"`
	Components components                       `json:"components"`
}

type info struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

type components struct {
	Schemas    map[string]*jsonschema.Schema `json:"schemas"`
	Parameters map[string]*parameter         `json:"parameters"`
}

type operation struct {
	OperationID string               `json:"operationId"`
	Summary     string               `json:"summary"`
	Parameters  []*parameter         `json:"parameters,omitempty"`
	RequestBody *requestBody         `json:"requestBody,omitempty"`
	Responses   map[string]*response `json:"responses"`
}

type parameter struct {
	Ref         string             `json:"$ref,omitempty"`
	Name        string             `json:"name,omitempty"`
	In          string             `json:"in,omitempty"`
	Description string             `json:"description,omitempty"`
	Required    bool               `json:"required,omitempty"`
	Explode     *bool              `json:"explode,omitempty"`
	Schema      *jsonschema.Schema `json:"schema,omitempty"`
}

type requestBody struct {
	Required bool                  `json:"required"`
	Content  map[string]*mediaType `json:"content"`
}

type response struct {
	Description string                `json:"description"`
	Headers     map[string]*header    `json:"headers,omitempty"`
	Content     map[string]*mediaType `json:"content,omitempty"`
}

type header struct {
	Description string             `json:"description"`
	Schema      *jsonschema.Schema `json:"schema"`
}

type mediaType struct {
	Schema *jsonschema.Schema `json:"schema,omitempty"`
}

var errorDescriptions = map[int]string{
	400: "invalid, invalid_cursor or unknown_image; the message says what is wrong",
	401: "who_are_you (a write from nobody), unknown_person, invalid_identity or gateway_required",
	403: "cross_origin: a browser request from another site",
	404: "not_found",
	409: "exists, or deleted: the recipe is in the trash",
	412: "stale: the recipe changed since the version in If-Match; current holds it as it now stands",
	413: "too_large",
	415: "unsupported_media_type: not JPEG, PNG, WebP or HEIC",
	428: "version_required: send If-Match",
	500: "internal",
	503: "starting, or identity_unavailable: the gateway's keys cannot be fetched",
}

var pathParam = regexp.MustCompile(`\{(\w+)\}`)

var pathParamDocs = map[string]string{
	"id":   "recipe ID",
	"hash": "image hash, the lower-case hex SHA-256 of its bytes",
}

func ref(name string) *jsonschema.Schema {
	return &jsonschema.Schema{Ref: "#/components/schemas/" + name}
}

// buildDocument describes eps as OpenAPI 3.1.
func buildDocument(eps []endpoint, version string) *document {
	b := &docBuilder{schemas: map[string]*jsonschema.Schema{}, types: map[string]reflect.Type{}}
	doc := &document{
		OpenAPI: "3.1.0",
		Info:    info{Title: "Sous", Version: version, Description: docDescription},
		Paths:   map[string]map[string]*operation{},
		Components: components{
			Schemas: b.schemas,
			Parameters: map[string]*parameter{
				"Person": {Name: identity.HeaderPerson, In: "header",
					Description: "username of the person making the request, when not behind the Outpost gateway",
					Schema:      &jsonschema.Schema{Type: "string"}},
				"Via": {Name: identity.HeaderVia, In: "header",
					Description: "ui for the web UI; anything else is recorded as api",
					Schema:      &jsonschema.Schema{Type: "string"}},
			},
		},
	}
	errRef := b.component(jsonOf[errorBody]())
	staleRef := b.component(jsonOf[staleBody]())
	for _, ep := range eps {
		op := &operation{
			OperationID: ep.OpID,
			Summary:     ep.Summary,
			Parameters:  []*parameter{{Ref: "#/components/parameters/Person"}, {Ref: "#/components/parameters/Via"}},
			Responses:   map[string]*response{},
		}
		for _, m := range pathParam.FindAllStringSubmatch(ep.Path, -1) {
			op.Parameters = append(op.Parameters, &parameter{Name: m[1], In: "path", Required: true,
				Description: pathParamDocs[m[1]], Schema: &jsonschema.Schema{Type: "string"}})
		}
		if ep.Method == "PATCH" || ep.Method == "DELETE" {
			op.Parameters = append(op.Parameters, &parameter{Name: "If-Match", In: "header", Required: true,
				Description: `the recipe version being changed, e.g. "3"`, Schema: &jsonschema.Schema{Type: "string"}})
		}
		if ep.Query != nil {
			for _, f := range queryFields(ep.Query) {
				op.Parameters = append(op.Parameters, queryParam(f))
			}
		}
		if ep.Body != nil {
			op.RequestBody = &requestBody{Required: true, Content: b.content(ep.Body)}
		}

		status := ep.Status
		if status == 0 {
			status = http.StatusOK
		}
		ok := &response{Description: http.StatusText(status)}
		if ep.Resp != nil {
			ok.Content = b.content(ep.Resp)
			if ep.Resp.typ == reflect.TypeFor[recipes.Recipe]() {
				ok.Headers = map[string]*header{"ETag": {
					Description: "the recipe's version, quoted; send it back as If-Match",
					Schema:      &jsonschema.Schema{Type: "string"}}}
			}
		}
		op.Responses[strconv.Itoa(status)] = ok

		errs := slices.Clone(ep.Errors)
		errs = append(errs, 401, 500, 503)
		if ep.Query != nil || ep.Body != nil {
			errs = append(errs, 400)
		}
		if ep.Body != nil {
			errs = append(errs, 413)
		}
		if ep.Method != "GET" {
			errs = append(errs, 403)
		}
		for _, code := range errs {
			s := errRef
			if code == http.StatusPreconditionFailed {
				s = staleRef
			}
			op.Responses[strconv.Itoa(code)] = &response{
				Description: errorDescriptions[code],
				Content:     map[string]*mediaType{"application/json": {Schema: s}},
			}
		}

		if doc.Paths[ep.Path] == nil {
			doc.Paths[ep.Path] = map[string]*operation{}
		}
		doc.Paths[ep.Path][strings.ToLower(ep.Method)] = op
	}
	return doc
}

func queryParam(f queryField) *parameter {
	p := &parameter{Name: f.name, In: "query", Description: f.doc}
	switch f.typ.Kind() {
	case reflect.Slice:
		explode := true
		p.Explode = &explode
		p.Schema = &jsonschema.Schema{Type: "array", Items: &jsonschema.Schema{Type: "string"}}
	case reflect.Bool:
		p.Schema = &jsonschema.Schema{Type: "boolean"}
	case reflect.Int:
		p.Schema = &jsonschema.Schema{Type: "integer"}
	default:
		p.Schema = &jsonschema.Schema{Type: "string"}
	}
	return p
}

// docBuilder collects named types into components/schemas.
type docBuilder struct {
	schemas map[string]*jsonschema.Schema
	types   map[string]reflect.Type
}

func (b *docBuilder) content(p *payload) map[string]*mediaType {
	if p.media != nil {
		out := map[string]*mediaType{}
		for _, m := range p.media {
			out[m] = &mediaType{}
		}
		return out
	}
	if p.typ == nil {
		return map[string]*mediaType{"application/json": {Schema: p.schema()}}
	}
	return map[string]*mediaType{"application/json": {Schema: b.component(p)}}
}

// component adds p's type to components/schemas and returns a $ref to it.
func (b *docBuilder) component(p *payload) *jsonschema.Schema {
	name := componentName(p.typ)
	s := p.schema()
	b.walk(p.typ, s, true)
	b.add(name, p.typ, s)
	return ref(name)
}

func (b *docBuilder) add(name string, t reflect.Type, s *jsonschema.Schema) {
	if have, ok := b.types[name]; ok {
		if have != t {
			panic(fmt.Sprintf("api: %v and %v both want the component name %s", have, t, name))
		}
		return
	}
	b.types[name] = t
	b.schemas[name] = s
}

// walk follows t and its schema s together and replaces each nested named
// type's schema with a $ref, hoisting the schema into components. It
// returns what should stand in s's place.
func (b *docBuilder) walk(t reflect.Type, s *jsonschema.Schema, root bool) *jsonschema.Schema {
	nullable := false
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
		nullable = true
	}
	switch t.Kind() {
	case reflect.Slice, reflect.Array:
		if s.Items != nil {
			s.Items = b.walk(t.Elem(), s.Items, false)
		}
		return s
	case reflect.Struct:
	default:
		return s
	}
	name, hoist := hoistName(t)
	if root || !hoist {
		b.walkFields(t, s)
		return s
	}
	desc := s.Description
	s.Description = ""
	if nullable {
		s.Types = slices.DeleteFunc(s.Types, func(x string) bool { return x == "null" })
		if len(s.Types) == 1 {
			s.Type, s.Types = s.Types[0], nil
		}
	}
	b.walkFields(t, s)
	b.add(name, t, s)
	r := ref(name)
	if nullable {
		return &jsonschema.Schema{Description: desc, AnyOf: []*jsonschema.Schema{r, {Type: "null"}}}
	}
	r.Description = desc
	return r
}

func (b *docBuilder) walkFields(t reflect.Type, s *jsonschema.Schema) {
	if s.Properties == nil {
		return
	}
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		if p := s.Properties[name]; p != nil {
			s.Properties[name] = b.walk(f.Type, p, false)
		}
	}
}

// hoistName reports whether a nested struct type gets its own component:
// the named wire types do, generic wrappers such as Optional don't.
func hoistName(t reflect.Type) (string, bool) {
	if n, ok := componentNames[t]; ok {
		return n, true
	}
	if t.Name() == "" || strings.Contains(t.Name(), "[") {
		return "", false
	}
	switch t.PkgPath() {
	case reflect.TypeFor[recipes.Recipe]().PkgPath(), reflect.TypeFor[identity.Caller]().PkgPath():
		return t.Name(), true
	}
	return "", false
}

// componentName names a top-level body type; generic wrappers take their
// element's name, as in SummaryPage and PersonList.
func componentName(t reflect.Type) string {
	if n, ok := componentNames[t]; ok {
		return n
	}
	base, arg, ok := strings.Cut(t.Name(), "[")
	if !ok {
		return base
	}
	arg = strings.TrimSuffix(arg, "]")
	arg = arg[strings.LastIndex(arg, ".")+1:]
	if base == "Items" {
		base = "List"
	}
	return arg + base
}
