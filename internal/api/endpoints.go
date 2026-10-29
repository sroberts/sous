// Package api is the REST surface under /v1 (spec.md §8) and its OpenAPI
// document, both generated from one route table.
package api

import (
	"log/slog"
	"net/http"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/recipes"
)

// Deps is what the REST handlers need.
type Deps struct {
	Service  *recipes.Service
	Resolver *identity.Resolver
	// JWKS is nil unless OUTPOST_AUTH=gateway; it fills Status.JWKS.
	JWKS    *identity.JWKS
	Log     *slog.Logger
	Version string // the binary's version, for /v1/status
	Auth    string // "none" or "gateway", for /v1/status
}

// New returns the handler for every /v1 route, registered from the route
// table. It resolves identity (via from the x-sous-via header), enforces
// who_are_you on writes, body limits, cross-origin protection on unsafe
// methods, and answers unknown /v1 paths and methods with JSON 404 and 405.
func New(d Deps) http.Handler {
	panic("TODO")
}
