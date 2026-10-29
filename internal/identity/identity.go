// Package identity resolves who is calling and how the request arrived
// (spec.md §5): a verified Outpost gateway token, a named person on a
// direct request, or nobody.
package identity

import "context"

// Person is a household member as identity resolution sees them. Username is
// the stable key across both the gateway and direct routes.
type Person struct {
	Username string
	Name     string
	Email    string
	// Groups comes from the gateway token's claims. It is empty for people
	// resolved on the direct route.
	Groups []string
}

// Route is how the caller's identity was established.
type Route string

const (
	RouteGateway Route = "gateway" // a verified X-Outpost-Identity token
	RouteDirect  Route = "direct"  // X-Sous-Person or the sous_person cookie
	RouteNone    Route = "none"    // nobody
)

// Via is the surface a request came through, recorded on every change.
type Via string

const (
	ViaUI  Via = "ui"
	ViaAPI Via = "api"
	ViaMCP Via = "mcp"
)

// ViaFromHeader maps an x-sous-via header value to a Via. Only "ui" is
// honoured; anything else is an API client.
func ViaFromHeader(v string) Via {
	if v == string(ViaUI) {
		return ViaUI
	}
	return ViaAPI
}

// Caller is the resolved identity of one request.
type Caller struct {
	Person *Person // nil when nobody is calling
	Route  Route
	Via    Via
}

// Username is the caller's username, or "" for nobody.
func (c Caller) Username() string {
	if c.Person == nil {
		return ""
	}
	return c.Person.Username
}

type callerKey struct{}

// WithCaller returns a context carrying c.
func WithCaller(ctx context.Context, c Caller) context.Context {
	return context.WithValue(ctx, callerKey{}, c)
}

// CallerFrom returns the caller stored by WithCaller, and whether there was one.
func CallerFrom(ctx context.Context) (Caller, bool) {
	c, ok := ctx.Value(callerKey{}).(Caller)
	return c, ok
}

// Error is an identity failure, carrying the HTTP status and the stable error
// code the API reports in {"error":{"code","message"}}.
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Error codes returned by Resolve.
const (
	CodeInvalidIdentity     = "invalid_identity"     // 401: token present but fails verification
	CodeIdentityUnavailable = "identity_unavailable" // 503: the JWKS cannot be fetched
	CodeGatewayRequired     = "gateway_required"     // 401: SOUS_REQUIRE_GATEWAY and no token
	CodeUnknownPerson       = "unknown_person"       // 401: X-Sous-Person names no one
)

// Request headers and cookie that carry identity.
const (
	HeaderIdentity = "X-Outpost-Identity"
	HeaderPerson   = "X-Sous-Person"
	HeaderVia      = "X-Sous-Via"
	CookiePerson   = "sous_person"
)

// CallerHeader is the one name for the internal header that carries a
// resolved username from api.WithIdentity to the MCP tools. Both sides refer
// to it here, so the header WithIdentity strips is always the one MCP trusts.
const CallerHeader = "Sous-Internal-Caller"
