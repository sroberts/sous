package api

import (
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/sroberts/sous/internal/recipes"
)

// queryField is one struct field with a query tag. The decoder and the
// OpenAPI generator both read these, so they agree on names and types.
type queryField struct {
	name  string
	index int
	typ   reflect.Type
	doc   string
}

func queryFields(t reflect.Type) []queryField {
	var out []queryField
	for i := range t.NumField() {
		f := t.Field(i)
		name := f.Tag.Get("query")
		if name == "" || !f.IsExported() {
			continue
		}
		out = append(out, queryField{name: name, index: i, typ: f.Type, doc: f.Tag.Get("jsonschema")})
	}
	return out
}

// decodeQuery fills the query-tagged fields of the struct dst points to.
// Unknown parameters are ignored; a malformed value is 400 invalid.
func decodeQuery(q url.Values, dst any) error {
	v := reflect.ValueOf(dst).Elem()
	for _, f := range queryFields(v.Type()) {
		vals := q[f.name]
		if len(vals) == 0 {
			continue
		}
		fv := v.Field(f.index)
		if f.typ.Kind() == reflect.Slice {
			for _, s := range vals {
				if s = strings.TrimSpace(s); s != "" {
					fv.Set(reflect.Append(fv, reflect.ValueOf(s)))
				}
			}
			continue
		}
		if len(vals) > 1 {
			return recipes.Errorf(recipes.CodeInvalid, "%s must be given once", f.name)
		}
		s := strings.TrimSpace(vals[0])
		if s == "" {
			continue
		}
		switch f.typ.Kind() {
		case reflect.String:
			fv.SetString(s)
		case reflect.Bool:
			b, err := strconv.ParseBool(s)
			if err != nil {
				return recipes.Errorf(recipes.CodeInvalid, "%s must be true or false", f.name)
			}
			fv.SetBool(b)
		case reflect.Int:
			n, err := strconv.Atoi(s)
			if err != nil {
				return recipes.Errorf(recipes.CodeInvalid, "%s must be an integer", f.name)
			}
			fv.SetInt(int64(n))
		default:
			panic("api: unsupported query field type " + f.typ.String())
		}
	}
	return nil
}
