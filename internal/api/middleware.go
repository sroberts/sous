package api

import (
	"net/http"

	"github.com/sroberts/sous/internal/identity"
)

// WithIdentity resolves the caller with res and stores it on the request
// context (identity.WithCaller) before calling next. A resolution failure is
// written as JSON {"error":{"code","message"}} with its status. via is
// recorded as given; /mcp passes identity.ViaMCP.
func WithIdentity(res *identity.Resolver, via identity.Via, next http.Handler) http.Handler {
	panic("TODO")
}
