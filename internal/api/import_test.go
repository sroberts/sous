package api

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"testing"

	"github.com/sroberts/sous/internal/importer"
	"github.com/sroberts/sous/internal/recipes"
	"github.com/sroberts/sous/internal/safefetch"
)

// importHarness is a harness whose API has an importer that can reach only
// a fake site at http://recipes.test:<port>.
func importHarness(t *testing.T) (*harness, string) {
	t.Helper()
	h := newHarness(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/soup", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<title>Soup</title><script type="application/ld+json">{"@type":"Recipe","name":"Soup",
			"recipeIngredient":["1 onion","2 cups stock"],"recipeInstructions":"Simmer.","image":"/soup.png"}</script>`))
	})
	mux.HandleFunc("/soup.png", func(w http.ResponseWriter, r *http.Request) { w.Write(pngBytes(t)) })
	mux.HandleFunc("/gone", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`<title>Lost soup</title>`))
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
	log := slog.New(slog.DiscardHandler)
	svc := recipes.New(h.st, log)
	h.h = New(Deps{Service: svc, Resolver: h.res, Log: log, Version: "test", Auth: "none",
		Importer: importer.New(svc, fetch, log)})
	return h, "http://recipes.test:" + strconv.Itoa(int(ap.Port()))
}

func TestImportPreview(t *testing.T) {
	h, site := importHarness(t)
	const path = "/v1/recipes/import/preview"
	h.want(h.do("POST", path, map[string]any{"url": site + "/soup"}), 401, "who_are_you")
	cook := h.person("Cook")

	r := h.do("POST", path, map[string]any{"url": site + "/soup"}, cook...)
	h.want(r, 200, "")
	m := r.json(t)
	draft := m["draft"].(map[string]any)
	if draft["title"] != "Soup" || len(draft["ingredients"].([]any)) != 2 || len(draft["images"].([]any)) != 1 {
		t.Errorf("draft = %v", draft)
	}
	if rep := m["report"].(map[string]any); rep["method"] != "json-ld" {
		t.Errorf("report = %v", rep)
	}
	// The draft saves as is.
	h.create(cook, draft)

	r = h.do("POST", path, map[string]any{"url": site + "/soup"}, cook...)
	h.want(r, 200, "")
	if d := r.json(t)["duplicates"].([]any); len(d) != 1 {
		t.Errorf("duplicates = %v; want the saved copy", d)
	}

	r = h.do("POST", path, map[string]any{"url": site + "/gone"}, cook...)
	h.want(r, 422, recipes.CodePageNotFound)
	page, _ := r.json(t)["page"].(map[string]any)
	if page == nil || page["title"] != "Lost soup" {
		t.Errorf("page = %v; want the fallback", r.json(t)["page"])
	}
	if r.header.Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q", r.header.Get("Cache-Control"))
	}

	// Loopback isn't the fake site, and the guard refuses it.
	h.want(h.do("POST", path, map[string]any{"url": "http://127.0.0.1:1/"}, cook...), 422, recipes.CodeUnsupportedURL)
	h.want(h.do("POST", path, map[string]any{"url": "ftp://recipes.test/x"}, cook...), 422, recipes.CodeUnsupportedURL)

	// Text with a link: the link is credited, not fetched.
	r = h.do("POST", path, map[string]any{"text": "Ingredients\n1 onion\nMethod\nSimmer.", "url": site + "/gone", "title": "Onion soup"}, cook...)
	h.want(r, 200, "")
	if d := r.json(t)["draft"].(map[string]any); d["title"] != "Onion soup" || d["source_url"] != site+"/gone" {
		t.Errorf("text draft = %v", d)
	}
	h.want(h.do("POST", path, map[string]any{"text": "•\n-\n*"}, cook...), 422, recipes.CodeNoRecipe)

	h.want(h.do("POST", path, map[string]any{}, cook...), 400, "invalid")
	h.want(h.do("POST", path, map[string]any{"title": "Soup"}, cook...), 400, "invalid")
	h.want(h.do("POST", path, map[string]any{"url": site, "save": true}, cook...), 400, "invalid")
	h.want(h.do("GET", path, nil, cook...), 405, "method_not_allowed")
}

func TestImportUnavailable(t *testing.T) {
	h := newHarness(t)
	cook := h.person("Cook")
	h.want(h.do("POST", "/v1/recipes/import/preview", map[string]any{"url": "https://example.com/"}, cook...), 503, "unavailable")
}

func TestImportImageLimit(t *testing.T) {
	if imageLimit != importer.MaxImageBytes {
		t.Errorf("imageLimit = %d; importer.MaxImageBytes = %d", imageLimit, importer.MaxImageBytes)
	}
}
