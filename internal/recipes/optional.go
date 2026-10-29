package recipes

import (
	"bytes"
	"encoding/json"
)

// Optional is a PATCH field that tells apart "absent", "null" and a value.
//
// encoding/json calls UnmarshalJSON only for keys that are present, so:
//
//	absent      → Set false
//	null        → Set true, Null true, Value zero
//	a value     → Set true, Null false, Value decoded
//
// With omitzero it marshals back the same way. Its schema (SchemaFor) is
// "T or null".
type Optional[T any] struct {
	Set   bool
	Null  bool
	Value T
}

// Some returns a set, non-null Optional holding v.
func Some[T any](v T) Optional[T] { return Optional[T]{Set: true, Value: v} }

// Null returns a set Optional holding null.
func Null[T any]() Optional[T] { return Optional[T]{Set: true, Null: true} }

// Ptr returns nil for null (or absent), else a pointer to a copy of Value.
func (o Optional[T]) Ptr() *T {
	if !o.Set || o.Null {
		return nil
	}
	v := o.Value
	return &v
}

func (o *Optional[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	var zero T
	o.Value = zero
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		o.Null = true
		return nil
	}
	o.Null = false
	return json.Unmarshal(b, &o.Value)
}

func (o Optional[T]) MarshalJSON() ([]byte, error) {
	if !o.Set || o.Null {
		return []byte("null"), nil
	}
	return json.Marshal(o.Value)
}

// IsZero reports absence, so omitzero drops unset fields.
func (o Optional[T]) IsZero() bool { return !o.Set }
