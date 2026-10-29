package identity

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The helpers below mirror outpost's internal/auth/keys.go and
// internal/gateway/jwt.go, so these tests verify what the gateway signs.

type signingKey struct {
	ID      string
	Private ed25519.PrivateKey
}

func (k signingKey) Public() ed25519.PublicKey {
	return k.Private.Public().(ed25519.PublicKey)
}

func keyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return base64.RawURLEncoding.EncodeToString(sum[:])[:16]
}

func generateKey(t *testing.T) signingKey {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return signingKey{ID: keyID(pub), Private: priv}
}

type identityClaims struct {
	Iss    string   `json:"iss"`
	Aud    string   `json:"aud"`
	Sub    string   `json:"sub"`
	Name   string   `json:"name,omitempty"`
	Email  string   `json:"email,omitempty"`
	Groups []string `json:"groups"`
	Iat    int64    `json:"iat"`
	Exp    int64    `json:"exp"`
}

func signToken(key signingKey, c identityClaims) (string, error) {
	header, err := json.Marshal(struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
		Kid string `json:"kid"`
	}{"EdDSA", "JWT", key.ID})
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	enc := base64.RawURLEncoding
	signing := enc.EncodeToString(header) + "." + enc.EncodeToString(payload)
	sig := ed25519.Sign(key.Private, []byte(signing))
	return signing + "." + enc.EncodeToString(sig), nil
}

type jwkOut struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`
}

func renderJWKS(keys []signingKey) []byte {
	set := struct {
		Keys []jwkOut `json:"keys"`
	}{Keys: make([]jwkOut, 0, len(keys))}
	for _, k := range keys {
		set.Keys = append(set.Keys, jwkOut{
			Kty: "OKP", Crv: "Ed25519",
			X:   base64.RawURLEncoding.EncodeToString(k.Public()),
			Kid: k.ID, Alg: "EdDSA", Use: "sig",
		})
	}
	b, _ := json.Marshal(set)
	return b
}

// signRaw signs arbitrary header and payload JSON, for tokens outpost would
// never make.
func signRaw(priv ed25519.PrivateKey, header, payload any) string {
	h, _ := json.Marshal(header)
	p, _ := json.Marshal(payload)
	enc := base64.RawURLEncoding
	signing := enc.EncodeToString(h) + "." + enc.EncodeToString(p)
	return signing + "." + enc.EncodeToString(ed25519.Sign(priv, []byte(signing)))
}

const (
	testIssuer = "https://outpost.example"
	testHost   = "sous.outpost.example"
)

var t0 = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// jwksServer is a fake Outpost /.well-known/jwks.json.
type jwksServer struct {
	*httptest.Server
	hits atomic.Int32

	mu   sync.Mutex
	body []byte
	code int
	gate chan struct{} // if set, each request waits for it to close
}

func newJWKSServer(t *testing.T, keys ...signingKey) *jwksServer {
	s := &jwksServer{body: renderJWKS(keys), code: http.StatusOK}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		s.mu.Lock()
		body, code, gate := s.body, s.code, s.gate
		s.mu.Unlock()
		if gate != nil {
			<-gate
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		w.Write(body)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *jwksServer) set(code int, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.code, s.body = code, body
}

type fakeDir struct {
	mu      sync.Mutex
	people  map[string]*Person
	upserts []Person
	lookups int
}

func newFakeDir(usernames ...string) *fakeDir {
	d := &fakeDir{people: map[string]*Person{}}
	for _, u := range usernames {
		d.people[u] = &Person{Username: u, Name: strings.ToUpper(u)}
	}
	return d
}

func (d *fakeDir) Lookup(_ context.Context, username string) (*Person, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lookups++
	if p, ok := d.people[username]; ok {
		cp := *p
		return &cp, nil
	}
	return nil, nil
}

func (d *fakeDir) UpsertGateway(_ context.Context, p Person) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.upserts = append(d.upserts, p)
	return nil
}

type fixture struct {
	key  signingKey
	srv  *jwksServer
	jwks *JWKS
	clk  *clock
	dir  *fakeDir
	res  *Resolver
}

func newFixture(t *testing.T, mutate func(*ResolverConfig)) *fixture {
	t.Helper()
	f := &fixture{key: generateKey(t), clk: &clock{now: t0}, dir: newFakeDir("ann", "ben b")}
	f.srv = newJWKSServer(t, f.key)
	f.jwks = NewJWKS(f.srv.URL, nil)
	f.jwks.now = f.clk.Now
	cfg := ResolverConfig{
		Gateway:   true,
		Issuer:    testIssuer,
		JWKS:      f.jwks,
		Directory: f.dir,
		Now:       f.clk.Now,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	f.res = NewResolver(cfg)
	return f
}

func (f *fixture) claims(sub string) identityClaims {
	now := f.clk.Now()
	return identityClaims{
		Iss: testIssuer, Aud: testHost, Sub: sub, Name: "Ann Example", Email: "ann@example.com",
		Groups: []string{"family"}, Iat: now.Unix(), Exp: now.Add(60 * time.Second).Unix(),
	}
}

func (f *fixture) token(t *testing.T, key signingKey, c identityClaims) string {
	t.Helper()
	tok, err := signToken(key, c)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func request(host string, headers map[string]string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	r.Host = host
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

func wantErr(t *testing.T, err *Error, status int, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("got no error, want %d %s", status, code)
	}
	if err.Status != status || err.Code != code {
		t.Fatalf("got %d %s (%s), want %d %s", err.Status, err.Code, err.Message, status, code)
	}
}

func TestValidToken(t *testing.T) {
	f := newFixture(t, nil)
	tok := f.token(t, f.key, f.claims("ann"))
	c, err := f.res.Resolve(request(testHost, map[string]string{HeaderIdentity: tok}), ViaMCP)
	if err != nil {
		t.Fatal(err)
	}
	if c.Route != RouteGateway || c.Via != ViaMCP || c.Person == nil {
		t.Fatalf("caller = %+v", c)
	}
	p := *c.Person
	if p.Username != "ann" || p.Name != "Ann Example" || p.Email != "ann@example.com" ||
		len(p.Groups) != 1 || p.Groups[0] != "family" {
		t.Fatalf("person = %+v", p)
	}
	if f.dir.lookups != 0 {
		t.Errorf("gateway route consulted Lookup %d times", f.dir.lookups)
	}

	// Seen again unchanged: no second write. Renamed: written again.
	f.res.Resolve(request(testHost, map[string]string{HeaderIdentity: tok}), ViaUI)
	if len(f.dir.upserts) != 1 {
		t.Fatalf("upserts = %d, want 1", len(f.dir.upserts))
	}
	renamed := f.claims("ann")
	renamed.Name = "Ann Renamed"
	f.res.Resolve(request(testHost, map[string]string{HeaderIdentity: f.token(t, f.key, renamed)}), ViaUI)
	if len(f.dir.upserts) != 2 || f.dir.upserts[1].Name != "Ann Renamed" {
		t.Fatalf("upserts = %+v", f.dir.upserts)
	}
	newEmail := f.claims("ann")
	newEmail.Name = "Ann Renamed"
	newEmail.Email = "ann@new.example"
	f.res.Resolve(request(testHost, map[string]string{HeaderIdentity: f.token(t, f.key, newEmail)}), ViaUI)
	if len(f.dir.upserts) != 3 {
		t.Fatalf("upserts = %d, want 3", len(f.dir.upserts))
	}
	if got := f.srv.hits.Load(); got != 1 {
		t.Errorf("JWKS fetched %d times, want 1", got)
	}
}

func TestAudienceArray(t *testing.T) {
	f := newFixture(t, nil)
	now := f.clk.Now()
	tok := signRaw(f.key.Private,
		map[string]string{"alg": "EdDSA", "typ": "JWT", "kid": f.key.ID},
		map[string]any{"iss": testIssuer, "aud": []string{"other.example", testHost}, "sub": "ann",
			"iat": now.Unix(), "exp": now.Add(time.Minute).Unix()})
	c, err := f.res.Resolve(request(testHost, map[string]string{HeaderIdentity: tok}), ViaAPI)
	if err != nil || c.Username() != "ann" {
		t.Fatalf("got %+v, %v", c, err)
	}
	if c.Person.Groups == nil {
		t.Error("Groups is nil, want empty")
	}
}

func TestInvalidTokens(t *testing.T) {
	f := newFixture(t, nil)
	other := generateKey(t)
	impostor := signingKey{ID: f.key.ID, Private: other.Private}
	now := f.clk.Now()
	header := map[string]string{"alg": "EdDSA", "typ": "JWT", "kid": f.key.ID}
	payload := map[string]any{"iss": testIssuer, "aud": testHost, "sub": "ann",
		"iat": now.Unix(), "exp": now.Add(time.Minute).Unix()}
	with := func(m map[string]any, k string, v any) map[string]any {
		out := map[string]any{}
		for kk, vv := range m {
			out[kk] = vv
		}
		if v == nil {
			delete(out, k)
		} else {
			out[k] = v
		}
		return out
	}
	mod := func(fn func(*identityClaims)) string {
		c := f.claims("ann")
		fn(&c)
		return f.token(t, f.key, c)
	}
	valid := f.token(t, f.key, f.claims("ann"))
	parts := strings.Split(valid, ".")
	enc := base64.RawURLEncoding

	cases := map[string]string{
		"bad signature":     f.token(t, impostor, f.claims("ann")),
		"tampered payload":  parts[0] + "." + enc.EncodeToString([]byte(`{"iss":"`+testIssuer+`","aud":"`+testHost+`","sub":"root","iat":0,"exp":9999999999}`)) + "." + parts[2],
		"wrong iss":         mod(func(c *identityClaims) { c.Iss = "https://evil.example" }),
		"wrong aud":         mod(func(c *identityClaims) { c.Aud = "other.outpost.example" }),
		"aud with port":     mod(func(c *identityClaims) { c.Aud = testHost + ":443" }),
		"empty aud array":   signRaw(f.key.Private, header, with(payload, "aud", []string{})),
		"aud number":        signRaw(f.key.Private, header, with(payload, "aud", 7)),
		"expired":           mod(func(c *identityClaims) { c.Exp = now.Add(-31 * time.Second).Unix() }),
		"expired at skew":   mod(func(c *identityClaims) { c.Exp = now.Add(-30 * time.Second).Unix() }),
		"iat in future":     mod(func(c *identityClaims) { c.Iat = now.Add(31 * time.Second).Unix() }),
		"empty sub":         mod(func(c *identityClaims) { c.Sub = "" }),
		"no exp":            signRaw(f.key.Private, header, with(payload, "exp", nil)),
		"no iat":            signRaw(f.key.Private, header, with(payload, "iat", nil)),
		"alg none":          signRaw(f.key.Private, map[string]string{"alg": "none", "kid": f.key.ID}, payload),
		"alg none unsigned": enc.EncodeToString([]byte(`{"alg":"none","kid":"`+f.key.ID+`"}`)) + "." + parts[1] + ".",
		"alg HS256":         signRaw(f.key.Private, map[string]string{"alg": "HS256", "kid": f.key.ID}, payload),
		"alg lower case":    signRaw(f.key.Private, map[string]string{"alg": "eddsa", "kid": f.key.ID}, payload),
		"no kid":            signRaw(f.key.Private, map[string]string{"alg": "EdDSA"}, payload),
		"crit":              signRaw(f.key.Private, map[string]any{"alg": "EdDSA", "kid": f.key.ID, "crit": []string{"x"}}, payload),
		"two segments":      parts[0] + "." + parts[1],
		"four segments":     valid + ".x",
		"padded signature":  parts[0] + "." + parts[1] + "." + parts[2] + "==",
		"empty":             "",
	}
	for name, tok := range cases {
		t.Run(name, func(t *testing.T) {
			// A cookie and header that would resolve: a bad token must not
			// fall through to them.
			r := request(testHost, map[string]string{HeaderIdentity: tok, HeaderPerson: "ann"})
			r.AddCookie(&http.Cookie{Name: CookiePerson, Value: "ann"})
			c, err := f.res.Resolve(r, ViaAPI)
			wantErr(t, err, http.StatusUnauthorized, CodeInvalidIdentity)
			if c.Person != nil {
				t.Fatalf("person = %+v", c.Person)
			}
		})
	}
	if len(f.dir.upserts) != 0 || f.dir.lookups != 0 {
		t.Errorf("directory touched: %d upserts, %d lookups", len(f.dir.upserts), f.dir.lookups)
	}
}

func TestSkewAllowance(t *testing.T) {
	f := newFixture(t, nil)
	now := f.clk.Now()
	c := f.claims("ann")
	c.Exp = now.Add(-29 * time.Second).Unix()
	c.Iat = now.Add(30 * time.Second).Unix()
	if _, err := f.res.Resolve(request(testHost, map[string]string{HeaderIdentity: f.token(t, f.key, c)}), ViaAPI); err != nil {
		t.Fatalf("within skew: %v", err)
	}
}

func TestTwoIdentityHeaders(t *testing.T) {
	f := newFixture(t, nil)
	tok := f.token(t, f.key, f.claims("ann"))
	r := request(testHost, nil)
	r.Header.Add(HeaderIdentity, tok)
	r.Header.Add(HeaderIdentity, tok)
	_, err := f.res.Resolve(r, ViaAPI)
	wantErr(t, err, http.StatusUnauthorized, CodeInvalidIdentity)
}

func TestUnknownKidRefetch(t *testing.T) {
	f := newFixture(t, nil)
	resolve := func(key signingKey) *Error {
		_, err := f.res.Resolve(request(testHost, map[string]string{HeaderIdentity: f.token(t, key, f.claims("ann"))}), ViaAPI)
		return err
	}
	if err := resolve(f.key); err != nil {
		t.Fatal(err)
	}

	// Outpost rotates: the new key is published, so an unknown kid refetches.
	rotated := generateKey(t)
	f.srv.set(http.StatusOK, renderJWKS([]signingKey{f.key, rotated}))
	f.clk.Advance(2 * time.Minute)
	if err := resolve(rotated); err != nil {
		t.Fatalf("rotated key: %v", err)
	}
	if got := f.srv.hits.Load(); got != 2 {
		t.Fatalf("hits = %d, want 2", got)
	}

	// A kid that isn't published: refetched at most once a minute.
	forged := generateKey(t)
	f.clk.Advance(61 * time.Second)
	wantErr(t, resolve(forged), http.StatusUnauthorized, CodeInvalidIdentity)
	if got := f.srv.hits.Load(); got != 3 {
		t.Fatalf("hits = %d, want 3", got)
	}
	for range 5 {
		f.clk.Advance(10 * time.Second)
		wantErr(t, resolve(forged), http.StatusUnauthorized, CodeInvalidIdentity)
	}
	if got := f.srv.hits.Load(); got != 3 {
		t.Fatalf("hits within the minute = %d, want 3", got)
	}
	if err := resolve(rotated); err != nil {
		t.Fatalf("cached key during the limit: %v", err)
	}
	f.clk.Advance(10 * time.Second)
	wantErr(t, resolve(forged), http.StatusUnauthorized, CodeInvalidIdentity)
	if got := f.srv.hits.Load(); got != 4 {
		t.Fatalf("hits after a minute = %d, want 4", got)
	}

	// A key Outpost drops is no longer accepted once the set is refetched.
	f.srv.set(http.StatusOK, renderJWKS([]signingKey{rotated}))
	f.clk.Advance(time.Minute)
	resolve(forged)
	wantErr(t, resolve(f.key), http.StatusUnauthorized, CodeInvalidIdentity)
}

func TestJWKSDown(t *testing.T) {
	f := newFixture(t, nil)
	f.srv.set(http.StatusInternalServerError, []byte("down"))
	tok := f.token(t, f.key, f.claims("ann"))
	resolve := func() *Error {
		_, err := f.res.Resolve(request(testHost, map[string]string{HeaderIdentity: tok}), ViaAPI)
		return err
	}
	wantErr(t, resolve(), http.StatusServiceUnavailable, CodeIdentityUnavailable)
	st := f.jwks.Status()
	if st.OK || st.FetchedAt != nil || st.Error == nil {
		t.Fatalf("status = %+v", st)
	}

	// Never succeeded: retried at most every 5s.
	f.clk.Advance(4 * time.Second)
	wantErr(t, resolve(), http.StatusServiceUnavailable, CodeIdentityUnavailable)
	if got := f.srv.hits.Load(); got != 1 {
		t.Fatalf("hits = %d, want 1", got)
	}
	f.clk.Advance(time.Second)
	wantErr(t, resolve(), http.StatusServiceUnavailable, CodeIdentityUnavailable)
	if got := f.srv.hits.Load(); got != 2 {
		t.Fatalf("hits = %d, want 2", got)
	}

	f.srv.set(http.StatusOK, renderJWKS([]signingKey{f.key}))
	f.clk.Advance(5 * time.Second)
	if err := resolve(); err != nil {
		t.Fatalf("after recovery: %v", err)
	}
	st = f.jwks.Status()
	if !st.OK || st.FetchedAt == nil || *st.FetchedAt != "2026-10-28T12:00:10.000Z" || st.Error != nil {
		t.Fatalf("status = %+v", st)
	}

	// Down again: cached keys keep working, and the status says so.
	f.srv.set(http.StatusBadGateway, nil)
	f.clk.Advance(2 * time.Minute)
	wantErr(t, func() *Error {
		_, err := f.res.Resolve(request(testHost, map[string]string{HeaderIdentity: f.token(t, generateKey(t), f.claims("ann"))}), ViaAPI)
		return err
	}(), http.StatusServiceUnavailable, CodeIdentityUnavailable)
	tok = f.token(t, f.key, f.claims("ann"))
	if err := resolve(); err != nil {
		t.Fatalf("cached key while down: %v", err)
	}
	if st := f.jwks.Status(); st.OK || st.FetchedAt == nil || st.Error == nil {
		t.Fatalf("status = %+v", st)
	}
}

func TestJWKSUnreachable(t *testing.T) {
	f := newFixture(t, nil)
	f.srv.Close()
	_, err := f.res.Resolve(request(testHost, map[string]string{HeaderIdentity: f.token(t, f.key, f.claims("ann"))}), ViaAPI)
	wantErr(t, err, http.StatusServiceUnavailable, CodeIdentityUnavailable)
}

func TestJWKSKeyFiltering(t *testing.T) {
	good := generateKey(t)
	short := generateKey(t)
	x := base64.RawURLEncoding.EncodeToString(good.Public())
	body, _ := json.Marshal(map[string]any{"keys": []map[string]string{
		{"kty": "RSA", "crv": "Ed25519", "x": x, "kid": "rsa"},
		{"kty": "OKP", "crv": "X25519", "x": x, "kid": "x25519"},
		{"kty": "OKP", "crv": "Ed25519", "x": base64.RawURLEncoding.EncodeToString(short.Public()[:31]), "kid": "short"},
		{"kty": "OKP", "crv": "Ed25519", "x": x + "AA", "kid": "long"},
		{"kty": "OKP", "crv": "Ed25519", "x": x, "kid": "enc", "use": "enc"},
		{"kty": "OKP", "crv": "Ed25519", "x": x, "kid": ""},
		{"kty": "OKP", "crv": "Ed25519", "x": x, "kid": good.ID, "alg": "EdDSA", "use": "sig"},
	}})
	srv := newJWKSServer(t)
	srv.set(http.StatusOK, body)
	j := NewJWKS(srv.URL, nil)
	if k, err := j.Key(t.Context(), good.ID); err != nil || !k.Equal(good.Public()) {
		t.Fatalf("good key: %v", err)
	}
	for _, kid := range []string{"rsa", "x25519", "short", "long", "enc"} {
		if _, err := j.Key(t.Context(), kid); err != ErrUnknownKey {
			t.Errorf("%s: err = %v, want ErrUnknownKey", kid, err)
		}
	}
}

func TestJWKSBadBodies(t *testing.T) {
	big := []byte(`{"keys":[],"pad":"` + strings.Repeat("a", 1<<20) + `"}`)
	for name, body := range map[string][]byte{
		"over 1 MiB":     big,
		"not json":       []byte("nope"),
		"no usable keys": []byte(`{"keys":[{"kty":"RSA","kid":"a"}]}`),
	} {
		t.Run(name, func(t *testing.T) {
			srv := newJWKSServer(t)
			srv.set(http.StatusOK, body)
			_, err := NewJWKS(srv.URL, nil).Key(t.Context(), "a")
			if !errors.Is(err, ErrJWKSUnavailable) {
				t.Fatalf("err = %v, want ErrJWKSUnavailable", err)
			}
		})
	}
}

func TestJWKSSharedFetch(t *testing.T) {
	key := generateKey(t)
	srv := newJWKSServer(t, key)
	gate := make(chan struct{})
	srv.mu.Lock()
	srv.gate = gate
	srv.mu.Unlock()
	j := NewJWKS(srv.URL, nil)

	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for range n {
		wg.Go(func() {
			_, err := j.Key(context.Background(), key.ID)
			errs <- err
		})
	}
	for srv.hits.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	// Give the other callers time to arrive while the fetch is held open.
	time.Sleep(50 * time.Millisecond)
	close(gate)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := srv.hits.Load(); got != 1 {
		t.Fatalf("hits = %d, want 1", got)
	}
}

func TestJWKSCallerCancelled(t *testing.T) {
	key := generateKey(t)
	srv := newJWKSServer(t, key)
	gate := make(chan struct{})
	srv.mu.Lock()
	srv.gate = gate
	srv.mu.Unlock()
	j := NewJWKS(srv.URL, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := j.Key(ctx, key.ID); !errors.Is(err, ErrJWKSUnavailable) {
		t.Fatalf("err = %v", err)
	}
	// The fetch the cancelled caller started still serves the next one.
	close(gate)
	if _, err := j.Key(context.Background(), key.ID); err != nil {
		t.Fatal(err)
	}
	if got := srv.hits.Load(); got != 1 {
		t.Fatalf("hits = %d, want 1", got)
	}
}

func TestTokenWinsOverDirect(t *testing.T) {
	f := newFixture(t, nil)
	r := request(testHost, map[string]string{HeaderIdentity: f.token(t, f.key, f.claims("carol")), HeaderPerson: "ann"})
	r.AddCookie(&http.Cookie{Name: CookiePerson, Value: "ann"})
	c, err := f.res.Resolve(r, ViaUI)
	if err != nil || c.Username() != "carol" || c.Route != RouteGateway {
		t.Fatalf("got %+v, %v", c, err)
	}
}

func TestPlainOutpostHeadersIgnored(t *testing.T) {
	plain := map[string]string{
		"X-Outpost-User": "ann", "X-Outpost-Name": "Ann", "X-Outpost-Email": "ann@example.com", "X-Outpost-Groups": "family",
	}
	for _, gateway := range []bool{true, false} {
		f := newFixture(t, func(c *ResolverConfig) { c.Gateway = gateway })
		c, err := f.res.Resolve(request(testHost, plain), ViaAPI)
		if err != nil || c.Person != nil || c.Route != RouteNone {
			t.Fatalf("gateway=%v: got %+v, %v", gateway, c, err)
		}
	}

	// Without OUTPOST_AUTH even a valid token is ignored, and nothing is fetched.
	f := newFixture(t, func(c *ResolverConfig) { c.Gateway = false })
	h := map[string]string{HeaderIdentity: f.token(t, f.key, f.claims("carol")), HeaderPerson: "ann"}
	c, err := f.res.Resolve(request(testHost, h), ViaAPI)
	if err != nil || c.Username() != "ann" || c.Route != RouteDirect {
		t.Fatalf("got %+v, %v", c, err)
	}
	h[HeaderIdentity] = "garbage"
	delete(h, HeaderPerson)
	if c, err := f.res.Resolve(request(testHost, h), ViaAPI); err != nil || c.Route != RouteNone {
		t.Fatalf("garbage token: got %+v, %v", c, err)
	}
	if got := f.srv.hits.Load(); got != 0 {
		t.Errorf("JWKS fetched %d times", got)
	}
}

func TestRequireGateway(t *testing.T) {
	f := newFixture(t, func(c *ResolverConfig) { c.RequireGateway = true })
	r := request(testHost, map[string]string{HeaderPerson: "ann"})
	r.AddCookie(&http.Cookie{Name: CookiePerson, Value: "ann"})
	_, err := f.res.Resolve(r, ViaAPI)
	wantErr(t, err, http.StatusUnauthorized, CodeGatewayRequired)
	_, err = f.res.Resolve(request(testHost, nil), ViaAPI)
	wantErr(t, err, http.StatusUnauthorized, CodeGatewayRequired)

	c, err := f.res.Resolve(request(testHost, map[string]string{HeaderIdentity: f.token(t, f.key, f.claims("ann"))}), ViaAPI)
	if err != nil || c.Username() != "ann" {
		t.Fatalf("got %+v, %v", c, err)
	}
}

func TestDirectRoute(t *testing.T) {
	f := newFixture(t, func(c *ResolverConfig) { c.Gateway = false })
	resolve := func(header, cookie string) (Caller, *Error) {
		r := request(testHost, nil)
		if header != "" {
			r.Header.Set(HeaderPerson, header)
		}
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: CookiePerson, Value: cookie})
		}
		return f.res.Resolve(r, ViaUI)
	}

	c, err := resolve("ann", "")
	if err != nil || c.Username() != "ann" || c.Route != RouteDirect || c.Via != ViaUI {
		t.Fatalf("header: %+v, %v", c, err)
	}
	_, err = resolve("nobody", "ann")
	wantErr(t, err, http.StatusUnauthorized, CodeUnknownPerson)

	c, err = resolve("", "ben%20b")
	if err != nil || c.Username() != "ben b" || c.Route != RouteDirect {
		t.Fatalf("encoded cookie: %+v, %v", c, err)
	}
	for _, cookie := range []string{"nobody", "%zz"} {
		c, err = resolve("", cookie)
		if err != nil || c.Person != nil || c.Route != RouteNone {
			t.Fatalf("cookie %q: %+v, %v", cookie, c, err)
		}
	}
	c, err = resolve("", "")
	if err != nil || c.Person != nil || c.Route != RouteNone || c.Via != ViaUI {
		t.Fatalf("nobody: %+v, %v", c, err)
	}
}

func TestHostname(t *testing.T) {
	for in, want := range map[string]string{
		"sous.outpost.example":      "sous.outpost.example",
		"Sous.Outpost.Example":      "sous.outpost.example",
		"sous.outpost.example:8443": "sous.outpost.example",
		"sous.outpost.example.":     "sous.outpost.example",
		"SOUS.outpost.example.:443": "sous.outpost.example",
		"[::1]:8080":                "::1",
		"[::1]":                     "::1",
		"[FE80::1]":                 "fe80::1",
		"127.0.0.1:9000":            "127.0.0.1",
		"":                          "",
	} {
		if got := hostname(in); got != want {
			t.Errorf("hostname(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAudienceMatchesNormalisedHost(t *testing.T) {
	f := newFixture(t, nil)
	tok := f.token(t, f.key, f.claims("ann"))
	for _, host := range []string{"SOUS.Outpost.Example", "sous.outpost.example:8443", "sous.outpost.example.", "Sous.outpost.example.:443"} {
		if _, err := f.res.Resolve(request(host, map[string]string{HeaderIdentity: tok}), ViaAPI); err != nil {
			t.Errorf("host %q: %v", host, err)
		}
	}
	// The gateway sets X-Forwarded-Host too, but Host is what counts.
	r := request("evil.example", map[string]string{HeaderIdentity: tok, "X-Forwarded-Host": testHost})
	_, err := f.res.Resolve(r, ViaAPI)
	wantErr(t, err, http.StatusUnauthorized, CodeInvalidIdentity)
}

func TestNoJWKSConfigured(t *testing.T) {
	f := newFixture(t, func(c *ResolverConfig) { c.JWKS = nil })
	_, err := f.res.Resolve(request(testHost, map[string]string{HeaderIdentity: f.token(t, f.key, f.claims("ann"))}), ViaAPI)
	wantErr(t, err, http.StatusServiceUnavailable, CodeIdentityUnavailable)
	_, err = f.res.Resolve(request(testHost, map[string]string{HeaderIdentity: "a.b.c"}), ViaAPI)
	wantErr(t, err, http.StatusUnauthorized, CodeInvalidIdentity)
}
