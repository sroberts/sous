package recipes

import "github.com/google/jsonschema-go/jsonschema"

// SchemaFor returns the JSON Schema for T, used for both the OpenAPI
// document and MCP tool input and output schemas.
//
// It calls jsonschema.For with ForOptions.TypeSchemas mapping every
// Optional[X] instantiation used in these types to "X or null" (TypeSchemas
// is keyed by concrete reflect.Type), and ParsedLine to its two shapes. The
// types are static, so a failure is a programming error and panics.
func SchemaFor[T any]() *jsonschema.Schema {
	panic("TODO")
}
