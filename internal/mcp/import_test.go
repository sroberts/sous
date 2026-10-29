package mcp

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"testing"

	"github.com/sroberts/sous/internal/importer"
	"github.com/sroberts/sous/internal/recipes"
	"github.com/sroberts/sous/internal/safefetch"
)

// importServer serves /mcp with an importer that reaches only a fake site
// at http://recipes.test:<port>, whose base it returns.
func importServer(t *testing.T) (string, string) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/stew", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<title>Stew</title><script type="application/ld+json">{"@type":"Recipe","name":"Beef Stew",
			"recipeIngredient":["1 lb beef","2 carrots"],"recipeInstructions":["Brown the beef.","Simmer."],"recipeYield":"4"}</script>`))
	})
	mux.HandleFunc("/gone", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusGone)
		w.Write([]byte(`<title>Old stew</title>`))
	})
	site := httptest.NewServer(mux)
	t.Cleanup(site.Close)
	ap := netip.MustParseAddrPort(site.Listener.Addr().String())
	fetch := safefetch.New(safefetch.Options{Testing: &safefetch.TestHooks{
		AllowAddrPorts: []netip.AddrPort{ap},
		LookupNetIP: func(_ context.Context, host string) ([]netip.Addr, error) {
			if host == "recipes.test" {
				return []netip.Addr{ap.Addr()}, nil
			}
			return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
		},
	}})
	svc := newService(t)
	log := slog.New(slog.DiscardHandler)
	url := serve(t, standIn(New(svc, log, "test", WithImporter(importer.New(svc, fetch, log)))))
	return url, "http://recipes.test:" + strconv.Itoa(int(ap.Port()))
}

func TestImportRecipeTool(t *testing.T) {
	url, site := importServer(t)
	alice := as(t, url, "alice")

	res, err := alice.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var tool bool
	for _, tl := range res.Tools {
		if tl.Name == "import_recipe" {
			tool = true
			if a := tl.Annotations; a.OpenWorldHint == nil || !*a.OpenWorldHint || a.DestructiveHint == nil || *a.DestructiveHint {
				t.Errorf("import_recipe annotations = %+v", a)
			}
			if tl.OutputSchema == nil {
				t.Error("import_recipe has no output schema")
			}
		}
	}
	if !tool || len(res.Tools) != 10 {
		t.Fatalf("got %d tools, import_recipe present = %v; want 10 with it", len(res.Tools), tool)
	}

	out := ok[recipes.ImportResult](t, alice, "import_recipe", map[string]any{"url": site + "/stew"})
	if out.Draft.Title != "Beef Stew" || len(out.Draft.Ingredients) != 2 || out.Saved != nil {
		t.Errorf("preview = %+v", out)
	}
	list := ok[recipes.Page[recipes.Summary]](t, alice, "search_recipes", map[string]any{})
	if len(list.Items) != 0 {
		t.Errorf("a preview saved something: %+v", list.Items)
	}

	out = ok[recipes.ImportResult](t, alice, "import_recipe", map[string]any{"url": site + "/stew", "save": true})
	if out.Saved == nil || out.Saved.Title != "Beef Stew" || out.Saved.CreatedBy != "alice" {
		t.Fatalf("saved = %+v", out.Saved)
	}
	msg := fails(t, alice, "import_recipe", map[string]any{"url": site + "/stew", "save": true}, recipes.CodeExists)
	if !strings.Contains(msg, out.Saved.ID) {
		t.Errorf("exists message %q doesn't name the copy", msg)
	}

	msg = fails(t, alice, "import_recipe", map[string]any{"url": site + "/gone"}, recipes.CodePageNotFound)
	if !strings.Contains(msg, "Old stew") {
		t.Errorf("message %q lacks the page title", msg)
	}
	fails(t, alice, "import_recipe", map[string]any{"url": "http://127.0.0.1:1/"}, recipes.CodeUnsupportedURL)
	fails(t, as(t, url, ""), "import_recipe", map[string]any{"url": site + "/stew"}, recipes.CodeWhoAreYou)
}

func TestImportRecipeNeedsImporter(t *testing.T) {
	url := serve(t, standIn(New(newService(t), slog.New(slog.DiscardHandler), "test")))
	res, err := as(t, url, "").ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range res.Tools {
		if tl.Name == "import_recipe" {
			t.Error("import_recipe registered without an importer")
		}
	}
}
