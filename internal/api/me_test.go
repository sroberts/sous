package api

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/recipes"
	"github.com/sroberts/sous/internal/storage"
)

// TestMeSource checks /v1/me says whether Outpost signed the caller in, so
// the UI can drop its who's-cooking picker for a gateway login.
func TestMeSource(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding
	jwks, _ := json.Marshal(map[string]any{"keys": []map[string]string{{
		"kty": "OKP", "crv": "Ed25519", "x": enc.EncodeToString(pub), "kid": "k1", "alg": "EdDSA", "use": "sig",
	}}})
	keys := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(jwks)
	}))
	t.Cleanup(keys.Close)

	st, err := storage.Open(context.Background(), t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	log := slog.New(slog.DiscardHandler)
	svc := recipes.New(st, log)
	res := identity.NewResolver(identity.ResolverConfig{
		Gateway: true, Issuer: "https://outpost.test", Directory: svc,
		JWKS: identity.NewJWKS(keys.URL, keys.Client()),
	})
	h := New(Deps{Service: svc, Resolver: res, Log: log, Version: "test", Auth: "gateway", Portal: "https://outpost.test"})

	header, _ := json.Marshal(map[string]string{"alg": "EdDSA", "typ": "JWT", "kid": "k1"})
	now := time.Now().Unix()
	payload, _ := json.Marshal(map[string]any{
		"iss": "https://outpost.test", "aud": "sous.test", "sub": "scott", "name": "Scott",
		"groups": []string{}, "iat": now, "exp": now + 300,
	})
	signing := enc.EncodeToString(header) + "." + enc.EncodeToString(payload)
	token := signing + "." + enc.EncodeToString(ed25519.Sign(priv, []byte(signing)))

	me := func(headers ...string) map[string]any {
		t.Helper()
		req := httptest.NewRequest("GET", "http://sous.test/v1/me", nil)
		for i := 0; i < len(headers); i += 2 {
			req.Header.Set(headers[i], headers[i+1])
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /v1/me %v = %d: %s", headers, rec.Code, rec.Body)
		}
		var m map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}

	if m := me("X-Outpost-Identity", token); m["username"] != "scott" || m["name"] != "Scott" || m["source"] != "outpost" || m["portal"] != "https://outpost.test" {
		t.Errorf("through the gateway: me = %v; want scott, Scott, outpost, the portal", m)
	}
	// The gateway created scott, so the direct route can name him; that is
	// a choice made on the device, not an Outpost login.
	if m := me("X-Sous-Person", "scott"); m["source"] != "device" || m["portal"] != nil {
		t.Errorf("direct: me = %v; want device, no portal", m)
	}
	// A picked name never outranks the token.
	if m := me("X-Outpost-Identity", token, "X-Sous-Person", "someone"); m["username"] != "scott" || m["source"] != "outpost" {
		t.Errorf("token with X-Sous-Person: me = %v; want scott via outpost", m)
	}
}
