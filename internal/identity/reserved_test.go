package identity

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// TestSystemUsernameReserved: no route resolves to Sous itself. A gateway
// token for it is refused before anything is written; the direct route
// relies on the Directory never finding it, which recipes tests.
func TestSystemUsernameReserved(t *testing.T) {
	f := newFixture(t, nil)
	tok := f.token(t, f.key, f.claims(SystemUsername))
	_, err := f.res.Resolve(request(testHost, map[string]string{HeaderIdentity: tok}), ViaUI)
	wantErr(t, err, http.StatusUnauthorized, CodeInvalidIdentity)
	if !strings.Contains(err.Message, "reserved") {
		t.Errorf("message = %q", err.Message)
	}
	if len(f.dir.upserts) != 0 {
		t.Errorf("upserted %+v", f.dir.upserts)
	}

	f = newFixture(t, func(c *ResolverConfig) { c.Gateway = false })
	_, err = f.res.Resolve(request(testHost, map[string]string{HeaderPerson: SystemUsername}), ViaAPI)
	wantErr(t, err, http.StatusUnauthorized, CodeUnknownPerson)
	c, err := f.res.Resolve(request(testHost, map[string]string{"Cookie": CookiePerson + "=" + url.PathEscape(SystemUsername)}), ViaUI)
	if err != nil || c.Person != nil || c.Route != RouteNone {
		t.Fatalf("cookie _sous = %+v, %v; want nobody", c, err)
	}
}
