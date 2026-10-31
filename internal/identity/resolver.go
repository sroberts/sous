package identity

import (
	"context"
	"crypto/ed25519"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// codeInternal matches the API's code for an error that isn't the caller's.
const codeInternal = "internal"

// Directory is the people store the resolver consults. recipes.Service
// implements it.
type Directory interface {
	// Lookup returns the person with username, or (nil, nil) if there is
	// none. SystemUsername is never found: nobody can act as Sous.
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

	mu sync.Mutex
	// upserted holds the name and email last written for each gateway
	// username, so a person is written once per process unless they change.
	upserted map[string]nameEmail
}

type nameEmail struct{ name, email string }

// NewResolver returns a Resolver for cfg.
func NewResolver(cfg ResolverConfig) *Resolver {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Resolver{cfg: cfg, upserted: map[string]nameEmail{}}
}

// Resolve establishes who is calling, in this order:
//
//  1. Gateway mode with X-Outpost-Identity present: verify it (EdDSA, iss,
//     aud equal to the normalised r.Host, exp/iat within 30s skew). Failure
//     is 401 invalid_identity, never a downgrade; an unavailable JWKS is 503
//     identity_unavailable. A token for SystemUsername is invalid_identity.
//     On success, upsert the person when the cached name or email differs.
//     Route is RouteGateway.
//  2. Otherwise, with RequireGateway: 401 gateway_required.
//  3. X-Sous-Person: an unknown username is 401 unknown_person.
//  4. The sous_person cookie (URL-encoded): an unknown username is nobody.
//  5. Nobody: Person nil, RouteNone.
//
// via is recorded on the returned Caller as given.
func (r *Resolver) Resolve(req *http.Request, via Via) (Caller, *Error) {
	ctx := req.Context()
	if r.cfg.Gateway {
		if toks := req.Header.Values(HeaderIdentity); len(toks) > 0 {
			p, err := r.gateway(ctx, toks, hostname(req.Host))
			if err != nil {
				return Caller{}, err
			}
			return Caller{Person: p, Route: RouteGateway, Via: via}, nil
		}
	}
	if r.cfg.RequireGateway {
		return Caller{}, &Error{http.StatusUnauthorized, CodeGatewayRequired, "this service only accepts requests through the Outpost gateway"}
	}
	if name := req.Header.Get(HeaderPerson); name != "" {
		p, err := r.cfg.Directory.Lookup(ctx, name)
		if err != nil {
			return Caller{}, internal()
		}
		if p == nil {
			return Caller{}, &Error{http.StatusUnauthorized, CodeUnknownPerson, "no person named " + name}
		}
		return Caller{Person: p, Route: RouteDirect, Via: via}, nil
	}
	if ck, err := req.Cookie(CookiePerson); err == nil {
		// A cookie that doesn't decode or names no one is stale, not an
		// error: the UI just asks who's cooking again.
		if name, err := url.PathUnescape(ck.Value); err == nil && name != "" {
			p, err := r.cfg.Directory.Lookup(ctx, name)
			if err != nil {
				return Caller{}, internal()
			}
			if p != nil {
				return Caller{Person: p, Route: RouteDirect, Via: via}, nil
			}
		}
	}
	return Caller{Route: RouteNone, Via: via}, nil
}

func (r *Resolver) gateway(ctx context.Context, toks []string, host string) (*Person, *Error) {
	if len(toks) != 1 {
		return nil, &Error{http.StatusUnauthorized, CodeInvalidIdentity, "more than one identity token"}
	}
	key := func(kid string) (ed25519.PublicKey, error) {
		if r.cfg.JWKS == nil {
			return nil, ErrJWKSUnavailable
		}
		return r.cfg.JWKS.Key(ctx, kid)
	}
	c, err := verify(toks[0], key, r.cfg.Issuer, host, r.cfg.Now())
	if errors.Is(err, ErrJWKSUnavailable) {
		return nil, &Error{http.StatusServiceUnavailable, CodeIdentityUnavailable, "cannot fetch the gateway's signing keys"}
	}
	if err != nil {
		return nil, &Error{http.StatusUnauthorized, CodeInvalidIdentity, err.Error()}
	}
	if c.Sub == SystemUsername {
		return nil, &Error{http.StatusUnauthorized, CodeInvalidIdentity, ErrReservedUsername.Error()}
	}
	groups := c.Groups
	if groups == nil {
		groups = []string{}
	}
	p := &Person{Username: c.Sub, Name: c.Name, Email: c.Email, Groups: groups}
	if err := r.upsert(ctx, *p); err != nil {
		return nil, internal()
	}
	return p, nil
}

func (r *Resolver) upsert(ctx context.Context, p Person) error {
	want := nameEmail{p.Name, p.Email}
	r.mu.Lock()
	have, ok := r.upserted[p.Username]
	r.mu.Unlock()
	if ok && have == want {
		return nil
	}
	if err := r.cfg.Directory.UpsertGateway(ctx, p); err != nil {
		return err
	}
	r.mu.Lock()
	r.upserted[p.Username] = want
	r.mu.Unlock()
	return nil
}

// internal hides a store failure's detail from the client.
func internal() *Error {
	return &Error{http.StatusInternalServerError, codeInternal, "could not look up the caller"}
}

// hostname normalises a Host header the way Outpost does before it signs
// aud: lower case, without port, brackets or a trailing dot.
func hostname(raw string) string {
	h := raw
	if host, _, err := net.SplitHostPort(raw); err == nil {
		h = host
	}
	h = strings.TrimPrefix(strings.TrimSuffix(h, "]"), "[")
	return strings.TrimSuffix(strings.ToLower(h), ".")
}
