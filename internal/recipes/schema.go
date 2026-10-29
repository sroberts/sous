package recipes

import (
	"fmt"
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"
)

// SchemaFor returns the JSON Schema for T, used for both the OpenAPI
// document and MCP tool input and output schemas.
//
// It calls jsonschema.For with ForOptions.TypeSchemas mapping every
// Optional[X] instantiation used in these types to "X or null" (TypeSchemas
// is keyed by concrete reflect.Type), and ParsedLine to its two shapes. The
// types are static, so a failure is a programming error and panics.
func SchemaFor[T any]() *jsonschema.Schema {
	s, err := jsonschema.For[T](&jsonschema.ForOptions{TypeSchemas: typeSchemas})
	if err != nil {
		panic(fmt.Sprintf("recipes: schema for %v: %v", reflect.TypeFor[T](), err))
	}
	return s
}

var typeSchemas = map[reflect.Type]*jsonschema.Schema{
	reflect.TypeFor[Optional[string]]():   {Types: []string{"null", "string"}},
	reflect.TypeFor[Optional[int]]():      {Types: []string{"null", "integer"}},
	reflect.TypeFor[Optional[[]string]](): {Types: []string{"null", "array"}, Items: &jsonschema.Schema{Type: "string"}},
	reflect.TypeFor[ParsedLine]():         parsedLineSchema(),
}

// parsedLineSchema mirrors ParsedLine.MarshalJSON: a heading or an
// ingredient line, never a mix.
func parsedLineSchema() *jsonschema.Schema {
	heading, err := jsonschema.For[struct {
		Section string `json:"section" jsonschema:"the heading text"`
		Raw     string `json:"raw"`
	}](nil)
	if err != nil {
		panic(err)
	}
	line, err := jsonschema.For[struct {
		Raw           string   `json:"raw"`
		Quantity      *float64 `json:"quantity"`
		QuantityMax   *float64 `json:"quantity_max"`
		Unit          *string  `json:"unit"`
		Item          string   `json:"item"`
		Note          *string  `json:"note"`
		ParserVersion int      `json:"parser_version"`
	}](nil)
	if err != nil {
		panic(err)
	}
	return &jsonschema.Schema{OneOf: []*jsonschema.Schema{heading, line}}
}
