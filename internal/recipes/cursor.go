package recipes

import (
	"encoding/base64"
	"encoding/json"
)

// cursor is the keyset position after the last item of a page. Kind names
// the list and sort it came from, so a cursor can't be replayed against a
// different ordering.
type cursor struct {
	Kind string `json:"k"`
	Key  string `json:"v"`
	ID   string `json:"i,omitempty"`
}

func encodeCursor(c cursor) *string {
	b, _ := json.Marshal(c)
	s := base64.RawURLEncoding.EncodeToString(b)
	return &s
}

func decodeCursor(s, kind string) (cursor, error) {
	var c cursor
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err == nil {
		err = json.Unmarshal(b, &c)
	}
	if err != nil || c.Kind != kind {
		return cursor{}, Errorf(CodeInvalidCursor, "cursor is not valid for this list")
	}
	return c, nil
}
