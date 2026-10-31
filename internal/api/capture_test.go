package api

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/importer"
	"github.com/sroberts/sous/internal/recipes"
	"github.com/sroberts/sous/internal/safefetch"
)

const capturePath = "/v1/recipes/import"

// captureSite is a fake recipe site at http://recipes.test:<port> and a
// fetcher that can reach only it.
func captureSite(t *testing.T) (*safefetch.Client, string) {
	t.Helper()
	page := func(body string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(body))
		}
	}
	mux := http.NewServeMux()
	mux.Handle("/noodles", page(`<title>Garlic Noodles</title><script type="application/ld+json">{"@type":"Recipe","name":"Garlic Noodles",
		"recipeIngredient":["8 oz noodles","4 cloves garlic"],"recipeInstructions":"Boil. Toss.","image":"/noodles.png"}</script>`))
	mux.HandleFunc("/noodles.png", func(w http.ResponseWriter, r *http.Request) { w.Write(pngBytes(t)) })
	// The print view names /noodles as its canonical page.
	mux.Handle("/print/noodles", page(`<title>Garlic Noodles</title><link rel="canonical" href="/noodles">
		<script type="application/ld+json">{"@type":"Recipe","name":"Garlic Noodles","recipeIngredient":["8 oz noodles"]}</script>`))
	mux.Handle("/bare", page(`<script type="application/ld+json">{"@type":"Recipe","name":"Bare Noodles",
		"recipeIngredient":["8 oz noodles"]}</script>`))
	mux.Handle("/untitled", page(`<script type="application/ld+json">{"@type":"Recipe",
		"recipeIngredient":["8 oz noodles"],"recipeInstructions":"Boil."}</script>`))
	mux.HandleFunc("/blocked", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`<title>Access denied</title>`))
	})
	mux.HandleFunc("/gone", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`<title>Lost noodles</title>`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	ap := netip.MustParseAddrPort(srv.Listener.Addr().String())
	fetch := safefetch.New(safefetch.Options{Testing: &safefetch.TestHooks{
		AllowAddrPorts: []netip.AddrPort{ap},
		LookupNetIP: func(_ context.Context, host string) ([]netip.Addr, error) {
			if host == "recipes.test" {
				return []netip.Addr{ap.Addr()}, nil
			}
			return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
		},
	}})
	return fetch, "http://recipes.test:" + strconv.Itoa(int(ap.Port()))
}

// captureHarness is a harness on the direct route whose API can capture
// from the fake site.
func captureHarness(t *testing.T) (*harness, string) {
	t.Helper()
	h := newHarness(t)
	fetch, site := captureSite(t)
	log := slog.New(slog.DiscardHandler)
	svc := recipes.New(h.st, log)
	h.h = New(Deps{Service: svc, Resolver: h.res, Log: log, Version: "test", Auth: "none",
		Importer: importer.New(svc, fetch, log)})
	return h, site
}

func (h *harness) count(path string) int {
	h.t.Helper()
	r := h.do("GET", path, nil)
	h.want(r, 200, "")
	return int(r.json(h.t)["total"].(float64))
}

func summaryOf(t *testing.T, r *result) string {
	t.Helper()
	s, _ := r.json(t)["summary"].(string)
	return s
}

func TestCaptureSaves(t *testing.T) {
	h, site := captureHarness(t)
	cook := h.person("Cook")

	r := h.do("POST", capturePath, map[string]any{"url": site + "/noodles"}, cook...)
	h.want(r, 200, "")
	m := r.json(t)
	id, _ := m["id"].(string)
	if m["summary"] != "Saved Garlic Noodles." || m["title"] != "Garlic Noodles" || m["version"] != 1.0 ||
		m["path"] != "/r/"+id || m["url"] != nil || m["duplicate"] != false || m["restored"] != false {
		t.Errorf("capture = %s", r.body)
	}
	if ms := m["missing"].([]any); len(ms) == 0 || len(m["warnings"].([]any)) != 0 {
		t.Errorf("missing = %v, warnings = %v; want times and yield missing, no warnings", ms, m["warnings"])
	}
	r = h.do("GET", "/v1/recipes/"+id, nil)
	h.want(r, 200, "")
	rec := r.json(t)
	if rec["created_by"] != cook[1] || len(rec["images"].([]any)) != 1 || rec["source_url"] != site+"/noodles" {
		t.Errorf("saved = %s", r.body)
	}
	if n := h.count("/v1/recipes"); n != 1 {
		t.Errorf("%d recipes; want 1", n)
	}
}

func TestCaptureLiveDuplicate(t *testing.T) {
	h, site := captureHarness(t)
	cook := h.person("Cook")
	r := h.do("POST", capturePath, map[string]any{"url": site + "/noodles"}, cook...)
	h.want(r, 200, "")
	id := r.json(t)["id"].(string)

	host := strings.TrimPrefix(site, "http://")
	for _, link := range []string{
		site + "/noodles",
		site + "/noodles?utm_source=shortcut",
		"https://www." + host + "/noodles/", // never fetched: www.recipes.test doesn't resolve
		site + "/print/noodles",             // matches only by the page's canonical link
	} {
		r := h.do("POST", capturePath, map[string]any{"url": link}, cook...)
		h.want(r, 200, "")
		m := r.json(t)
		if m["id"] != id || m["duplicate"] != true || m["restored"] != false || m["summary"] != "Already in Sous: Garlic Noodles." ||
			len(m["missing"].([]any)) != 0 || len(m["warnings"].([]any)) != 0 {
			t.Errorf("%s: capture = %s", link, r.body)
		}
	}
	if n := h.count("/v1/recipes"); n != 1 {
		t.Errorf("%d recipes; want 1", n)
	}
}

func TestCaptureRestoresTrashed(t *testing.T) {
	h, site := captureHarness(t)
	cook, other := h.person("Cook"), h.person("Other")
	r := h.do("POST", capturePath, map[string]any{"url": site + "/noodles"}, cook...)
	h.want(r, 200, "")
	id := r.json(t)["id"].(string)
	h.want(h.do("DELETE", "/v1/recipes/"+id, nil, append(cook, "If-Match", `"1"`)...), 204, "")

	r = h.do("POST", capturePath, map[string]any{"url": site + "/noodles"}, other...)
	h.want(r, 200, "")
	m := r.json(t)
	if m["id"] != id || m["duplicate"] != true || m["restored"] != true || m["version"] != 3.0 ||
		m["summary"] != "Restored Garlic Noodles from the trash." {
		t.Errorf("capture = %s", r.body)
	}
	r = h.do("GET", "/v1/recipes/"+id, nil)
	if rec := r.json(t); rec["deleted_at"] != nil || rec["updated_by"] != other[1] {
		t.Errorf("restored = %s", r.body)
	}
	r = h.do("GET", "/v1/changes", nil)
	h.want(r, 200, "")
	last := r.json(t)["items"].([]any)[0].(map[string]any)
	if last["action"] != "restore" || last["entity_id"] != id || last["person_id"] != other[1] {
		t.Errorf("newest change = %v; want the restore by %s", last, other[1])
	}
	if n := h.count("/v1/recipes"); n != 1 {
		t.Errorf("%d recipes; want 1", n)
	}
}

func TestCaptureLiveBeatsTrashed(t *testing.T) {
	h, site := captureHarness(t)
	cook := h.person("Cook")
	r := h.do("POST", capturePath, map[string]any{"url": site + "/noodles"}, cook...)
	h.want(r, 200, "")
	trashed := r.json(t)["id"].(string)
	h.want(h.do("DELETE", "/v1/recipes/"+trashed, nil, append(cook, "If-Match", `"1"`)...), 204, "")
	live := h.create(cook, map[string]any{"title": "My Noodles", "source_url": site + "/noodles"})["id"]

	r = h.do("POST", capturePath, map[string]any{"url": site + "/noodles"}, cook...)
	h.want(r, 200, "")
	if m := r.json(t); m["id"] != live || m["restored"] != false || m["summary"] != "Already in Sous: My Noodles." {
		t.Errorf("capture = %s; want the live copy %v", r.body, live)
	}
	if n := h.count("/v1/trash"); n != 1 {
		t.Errorf("%d in the trash; want the trashed copy left there", n)
	}
}

func TestCapturePartial(t *testing.T) {
	h, site := captureHarness(t)
	cook := h.person("Cook")

	r := h.do("POST", capturePath, map[string]any{"url": site + "/bare"}, cook...)
	h.want(r, 200, "")
	m := r.json(t)
	if m["summary"] != "Saved Bare Noodles, but no steps were found." || !strings.Contains(string(r.body), `"steps"`) {
		t.Errorf("capture = %s", r.body)
	}

	r = h.do("POST", capturePath, map[string]any{"url": site + "/untitled"}, cook...)
	h.want(r, 200, "")
	m = r.json(t)
	if m["title"] != "Recipe from recipes.test" || m["summary"] != "Saved Recipe from recipes.test." || len(m["warnings"].([]any)) != 1 {
		t.Errorf("capture = %s", r.body)
	}
	if n := h.count("/v1/recipes"); n != 2 {
		t.Errorf("%d recipes; want 2", n)
	}
}

func TestCaptureFailures(t *testing.T) {
	h, site := captureHarness(t)
	cook := h.person("Cook")
	for _, tc := range []struct{ url, code string }{
		{site + "/blocked", recipes.CodeBlocked},
		{site + "/gone", recipes.CodePageNotFound},
		{"http://127.0.0.1:1/", recipes.CodeUnsupportedURL},
		{"ftp://recipes.test/x", recipes.CodeUnsupportedURL},
	} {
		r := h.do("POST", capturePath, map[string]any{"url": tc.url}, cook...)
		h.want(r, 422, tc.code)
		e := r.json(t)["error"].(map[string]any)
		if e["message"] == "" || summaryOf(t, r) != importer.Spoken(tc.code) {
			t.Errorf("%s: body = %s", tc.url, r.body)
		}
	}
	if n := h.count("/v1/recipes"); n != 0 {
		t.Errorf("%d recipes; want none saved", n)
	}
}

func TestCaptureRequests(t *testing.T) {
	h, site := captureHarness(t)
	link := map[string]any{"url": site + "/noodles"}

	r := h.do("POST", capturePath, link)
	h.want(r, 401, recipes.CodeWhoAreYou)
	if !strings.Contains(summaryOf(t, r), "X-Sous-Person") {
		t.Errorf("who_are_you summary = %q; want it to name the header", summaryOf(t, r))
	}
	// A personal access token means nothing on the direct route.
	h.want(h.do("POST", capturePath, link, "Authorization", "Bearer opat_x"), 401, recipes.CodeWhoAreYou)
	r = h.do("POST", capturePath, link, "X-Sous-Person", "nobody")
	h.want(r, 401, identity.CodeUnknownPerson)
	if summaryOf(t, r) != "no person named nobody" {
		t.Errorf("unknown_person summary = %q", summaryOf(t, r))
	}

	cook := h.person("Cook")
	r = h.do("POST", capturePath, link, append(cook, "Sec-Fetch-Site", "cross-site")...)
	h.want(r, 403, "cross_origin")
	if summaryOf(t, r) == "" {
		t.Errorf("cross_origin has no summary: %s", r.body)
	}

	for _, body := range []any{
		map[string]any{},
		map[string]any{"url": "  "},
		map[string]any{"url": 5},
		map[string]any{"url": site + "/noodles", "save": true},
		"not json",
	} {
		r := h.do("POST", capturePath, body, cook...)
		h.want(r, 400, recipes.CodeInvalid)
		if summaryOf(t, r) == "" {
			t.Errorf("%v: no summary: %s", body, r.body)
		}
	}
	r = h.do("POST", capturePath, []byte(`{"url":"`+strings.Repeat("x", jsonLimit)+`"}`), cook...)
	h.want(r, 413, codeTooLarge)
	if summaryOf(t, r) == "" {
		t.Errorf("too_large has no summary: %s", r.body)
	}

	// Other routes never carry a summary.
	r = h.do("POST", "/v1/recipes/import/preview", map[string]any{"url": site + "/gone"}, cook...)
	h.want(r, 422, recipes.CodePageNotFound)
	if _, ok := r.json(t)["summary"]; ok {
		t.Errorf("preview error has a summary: %s", r.body)
	}
	r = h.do("POST", "/v1/recipes", map[string]any{"title": "x"})
	h.want(r, 401, recipes.CodeWhoAreYou)
	if _, ok := r.json(t)["summary"]; ok {
		t.Errorf("createRecipe error has a summary: %s", r.body)
	}

	// GET isn't getRecipe("import"). The harness's schema check would map it
	// to getRecipe, so this goes straight to the handler.
	rec := httptest.NewRecorder()
	h.h.ServeHTTP(rec, httptest.NewRequest("GET", capturePath, nil))
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "POST" {
		t.Errorf("GET %s = %d, Allow %q: %s", capturePath, rec.Code, rec.Header().Get("Allow"), rec.Body)
	}
}

func TestCaptureUnavailable(t *testing.T) {
	h := newHarness(t)
	cook := h.person("Cook")
	r := h.do("POST", capturePath, map[string]any{"url": "https://example.com/"}, cook...)
	h.want(r, 503, "unavailable")
	if summaryOf(t, r) == "" {
		t.Errorf("no summary: %s", r.body)
	}
}

// TestCaptureGateway checks a capture through Outpost is credited to the
// token's subject and answers with a full link on the request's host.
func TestCaptureGateway(t *testing.T) {
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

	h := newHarness(t)
	fetch, site := captureSite(t)
	log := slog.New(slog.DiscardHandler)
	svc := recipes.New(h.st, log)
	h.res = identity.NewResolver(identity.ResolverConfig{
		Gateway: true, Issuer: "https://outpost.test", Directory: svc,
		JWKS: identity.NewJWKS(keys.URL, keys.Client()),
	})
	h.h = New(Deps{Service: svc, Resolver: h.res, Log: log, Version: "test", Auth: "gateway",
		Portal: "https://outpost.test", Importer: importer.New(svc, fetch, log)})

	// h.do sends to example.com, so that is the audience.
	header, _ := json.Marshal(map[string]string{"alg": "EdDSA", "typ": "JWT", "kid": "k1"})
	now := time.Now().Unix()
	payload, _ := json.Marshal(map[string]any{
		"iss": "https://outpost.test", "aud": "example.com", "sub": "scott", "name": "Scott",
		"groups": []string{}, "iat": now, "exp": now + 300,
	})
	signing := enc.EncodeToString(header) + "." + enc.EncodeToString(payload)
	token := signing + "." + enc.EncodeToString(ed25519.Sign(priv, []byte(signing)))

	r := h.do("POST", capturePath, map[string]any{"url": site + "/noodles"}, "X-Outpost-Identity", token)
	h.want(r, 200, "")
	m := r.json(t)
	id := m["id"].(string)
	if m["url"] != "https://example.com/r/"+id {
		t.Errorf("url = %v; want https://example.com/r/%s", m["url"], id)
	}
	r = h.do("GET", "/v1/recipes/"+id, nil)
	if rec := r.json(t); rec["created_by"] != "scott" {
		t.Errorf("created_by = %v; want scott", rec["created_by"])
	}

	// The direct route on the same server has no public address to give.
	r = h.do("POST", capturePath, map[string]any{"url": site + "/noodles"}, "X-Sous-Person", "scott")
	h.want(r, 200, "")
	if m := r.json(t); m["url"] != nil || m["duplicate"] != true {
		t.Errorf("direct capture = %s", r.body)
	}

	r = h.do("POST", capturePath, map[string]any{"url": site + "/noodles"}, "X-Outpost-Identity", "not.a.token")
	h.want(r, 401, identity.CodeInvalidIdentity)
	if summaryOf(t, r) == "" {
		t.Errorf("invalid_identity has no summary: %s", r.body)
	}
}
