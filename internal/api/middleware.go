package api

import (
	"net/http"
	"strings"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/recipes"
)

// CallerHeader carries the resolved username from WithIdentity to the
// handler it wraps. Any client-sent value is removed first, so the handler
// can trust it.
const CallerHeader = identity.CallerHeader

// WithIdentity resolves the caller with res and stores it on the request
// context (identity.WithCaller) before calling next. A resolution failure is
// written as JSON {"error":{"code","message"}} with its status. via is
// recorded as given; /mcp passes identity.ViaMCP.
//
// The username also travels in CallerHeader, because the MCP SDK hands tool
// handlers the request headers but not the request context.
func WithIdentity(res *identity.Resolver, via identity.Via, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ierr := res.Resolve(r, via)
		if ierr != nil {
			writeError(w, ierr.Status, ierr.Code, ierr.Message)
			return
		}
		reportCaller(w, c)
		r = r.WithContext(identity.WithCaller(r.Context(), c))
		r.Header = r.Header.Clone()
		r.Header.Del(CallerHeader)
		if u := c.Username(); u != "" {
			r.Header.Set(CallerHeader, u)
		}
		next.ServeHTTP(w, r)
	})
}

// reportCaller hands the caller to cmd/sous's request log, which wraps the
// ResponseWriter before the API sees the request.
func reportCaller(w http.ResponseWriter, c identity.Caller) {
	if s, ok := w.(interface{ SetCaller(identity.Caller) }); ok {
		s.SetCaller(c)
	}
}

// serve wraps an endpoint's handler with identity, the writer check and the
// body limit, and turns a returned error into a JSON error response.
func (a *api) serve(ep endpoint) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		via := identity.ViaFromHeader(r.Header.Get(identity.HeaderVia))
		c, ierr := a.d.Resolver.Resolve(r, via)
		if ierr != nil {
			writeErrorFor(w, ep.Spoken, ierr.Status, ierr.Code, ierr.Message)
			return
		}
		reportCaller(w, c)
		if ep.Write && c.Person == nil {
			writeErrorFor(w, ep.Spoken, http.StatusUnauthorized, recipes.CodeWhoAreYou, "choose who is cooking before making changes")
			return
		}
		if ep.Limit > 0 {
			r.Body = http.MaxBytesReader(w, r.Body, ep.Limit)
		}
		r = r.WithContext(identity.WithCaller(r.Context(), c))
		if err := ep.Handle(w, r, c); err != nil {
			a.fail(w, r, err, ep.Spoken)
		}
	})
}

func methodNotAllowed(methods []string) http.Handler {
	allow := strings.Join(methods, ", ")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", allow)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", r.Method+" is not allowed here; use "+allow)
	})
}

// crossOrigin refuses unsafe requests a browser made from another site.
// Requests without browser headers (agents, scripts) pass. spoken holds the
// "METHOD /path" of each spoken route, whose refusal carries a summary.
func crossOrigin(next http.Handler, spoken map[string]bool) http.Handler {
	cop := http.NewCrossOriginProtection()
	cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeErrorFor(w, spoken[r.Method+" "+r.URL.Path], http.StatusForbidden, "cross_origin", "cross-origin requests are not allowed")
	}))
	return cop.Handler(next)
}
