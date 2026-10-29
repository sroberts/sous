package recipes

import "fmt"

// Error codes. They are stable: clients branch on them. The API layer maps
// each to an HTTP status (noted alongside).
const (
	CodeInvalid              = "invalid"                // 400: bad input; Message names the field
	CodeWhoAreYou            = "who_are_you"            // 401: a write from nobody
	CodeNotFound             = "not_found"              // 404
	CodeExists               = "exists"                 // 409: username already taken
	CodeDeleted              = "deleted"                // 409: PATCH or DELETE on a recipe in the trash
	CodeStale                = "stale"                  // 412: version mismatch; Current is set
	CodeVersionRequired      = "version_required"       // 428: no If-Match
	CodeUnsupportedMediaType = "unsupported_media_type" // 415: not jpeg, png, webp or heic
)

// Error is a domain failure the API and MCP layers report to the client as
// {"error":{"code","message"}}. Any other error is internal.
type Error struct {
	Code    string
	Message string
	// Current is the recipe as it now stands, set only with CodeStale.
	Current *Recipe
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Errorf returns an *Error with code and a formatted message.
func Errorf(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}
