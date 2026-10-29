package identity

import (
	"context"
	"net/http"
	"time"
)

// Directory is the people store the resolver consults. recipes.Service
// implements it.
type Directory interface {
	// Lookup returns the person with username, or (nil, nil) if there is none.
	Lookup(ctx context.Context, username string) (*Person, error)
	// UpsertGateway creates or updates a person seen through the gateway
	// (source "gateway"), keyed by Username.
	UpsertGateway(ctx context.Context, p Person) error
}

// ResolverConfig configures a Resolver.
type ResolverConfig struct {
	// Gateway is true when OUTPOST_AUTH=gateway. When false, every
	// X-Outpost-* header is ignored.
	Gateway bool
	// Issuer is OUTPOST_ISSUER; a token's iss must equal it.
	Issuer string
	// RequireGateway closes the direct route (SOUS_REQUIRE_GATEWAY).
	RequireGateway bool
	// JWKS supplies verification keys. Required when Gateway is true.
	JWKS *JWKS
	// Directory looks people up and records gateway users.
	Directory Directory
	// Now is the clock for exp/iat checks; nil means time.Now.
	Now func() time.Time
}

// Resolver resolves the Caller for each request.
type Resolver struct {
	cfg ResolverConfig
}

// NewResolver returns a Resolver for cfg.
func NewResolver(cfg ResolverConfig) *Resolver {
	return &Resolver{cfg: cfg}
}

// Resolve establishes who is calling, in this order:
//
//  1. Gateway mode with X-Outpost-Identity present: verify it (EdDSA, iss,
//     aud equal to the normalised r.Host, exp/iat within 30s skew). Failure
//     is 401 invalid_identity, never a downgrade; an unavailable JWKS is 503
//     identity_unavailable. On success, upsert the person when the cached
//     name or email differs. Route is RouteGateway.
//  2. Otherwise, with RequireGateway: 401 gateway_required.
//  3. X-Sous-Person: an unknown username is 401 unknown_person.
//  4. The sous_person cookie (URL-encoded): an unknown username is nobody.
//  5. Nobody: Person nil, RouteNone.
//
// via is recorded on the returned Caller as given.
func (r *Resolver) Resolve(req *http.Request, via Via) (Caller, *Error) {
	panic("TODO")
}
