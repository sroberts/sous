package ui

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

func TestHandlerPWAFiles(t *testing.T) {
	dist := fstest.MapFS{
		"index.html":           {Data: []byte("<!doctype html><title>Sous</title>")},
		"sw.js":                {Data: []byte("const BUILD = 'abc';")},
		"manifest.webmanifest": {Data: []byte(`{"name":"Sous"}`)},
		"icons/icon-192.png":   {Data: []byte("png")},
	}
	h := newHandler(dist, slog.New(slog.DiscardHandler))

	tests := []struct {
		name, method, target, body, cache, ctype string
	}{
		{"worker", "GET", "/sw.js", "const BUILD", "no-cache", "text/javascript; charset=utf-8"},
		{"worker head", "HEAD", "/sw.js", "", "no-cache", "text/javascript; charset=utf-8"},
		{"manifest", "GET", "/manifest.webmanifest", `"name":"Sous"`, "no-cache", "application/manifest+json"},
		{"manifest head", "HEAD", "/manifest.webmanifest", "", "no-cache", "application/manifest+json"},
		{"icon", "GET", "/icons/icon-192.png", "png", "", "image/png"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serve(t, h, tt.method, tt.target)
			if rec.Code != 200 {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if tt.body != "" && !strings.Contains(rec.Body.String(), tt.body) {
				t.Errorf("body = %q, want it to contain %q", rec.Body.String(), tt.body)
			}
			if got := rec.Header().Get("Cache-Control"); got != tt.cache {
				t.Errorf("Cache-Control = %q, want %q", got, tt.cache)
			}
			if got := rec.Header().Get("Content-Type"); got != tt.ctype {
				t.Errorf("Content-Type = %q, want %q", got, tt.ctype)
			}
		})
	}
}

// Without a build there is no worker to serve, and a missing worker must not
// turn into index.html, which a browser would reject as a script.
func TestNotBuiltHasNoWorker(t *testing.T) {
	h := newHandler(fstest.MapFS{}, slog.New(slog.DiscardHandler))
	for _, target := range []string{"/sw.js", "/manifest.webmanifest"} {
		if rec := serve(t, h, "GET", target); rec.Code != 404 {
			t.Errorf("GET %s = %d, want 404", target, rec.Code)
		}
	}
}

// TestDistPWA checks the real embedded build. It needs `npm run build` first,
// so CI turns it on with SOUS_REQUIRE_UI=1.
func TestDistPWA(t *testing.T) {
	if os.Getenv("SOUS_REQUIRE_UI") != "1" {
		t.Skip("set SOUS_REQUIRE_UI=1 to check the built UI")
	}
	dist, err := fs.Sub(files, "dist")
	if err != nil {
		t.Fatal(err)
	}
	read := func(name string) string {
		t.Helper()
		b, err := fs.ReadFile(dist, name)
		if err != nil {
			t.Fatalf("dist/%s: %v", name, err)
		}
		return string(b)
	}

	var fields map[string]any
	if err := json.Unmarshal([]byte(read("manifest.webmanifest")), &fields); err != nil {
		t.Fatalf("manifest is not JSON: %v", err)
	}
	for key, want := range map[string]string{
		"id":          "/",
		"name":        "Sous",
		"short_name":  "Sous",
		"start_url":   "/",
		"scope":       "/",
		"display":     "standalone",
		"theme_color": "#fafbfc",
	} {
		if got, _ := fields[key].(string); got != want {
			t.Errorf("manifest %s = %q, want %q", key, got, want)
		}
	}

	// Chrome wants a 192 and a 512 icon, and maskable ones apart from "any".
	have := map[string]bool{}
	icons, _ := fields["icons"].([]any)
	for _, i := range icons {
		m, _ := i.(map[string]any)
		src, _ := m["src"].(string)
		sizes, _ := m["sizes"].(string)
		purpose, _ := m["purpose"].(string)
		if purpose == "" {
			purpose = "any"
		}
		if _, err := fs.Stat(dist, strings.TrimPrefix(src, "/")); err != nil {
			t.Errorf("manifest icon %s is not in dist", src)
		}
		have[sizes+" "+purpose] = true
	}
	for _, want := range []string{"192x192 any", "512x512 any", "192x192 maskable", "512x512 maskable"} {
		if !have[want] {
			t.Errorf("manifest has no %s icon", want)
		}
	}

	index := read("index.html")
	if !regexp.MustCompile(`<link[^>]*rel="manifest"[^>]*>`).MatchString(index) ||
		!regexp.MustCompile(`<link[^>]*rel="manifest"[^>]*crossorigin="use-credentials"`).MatchString(index) {
		t.Error(`index.html needs <link rel="manifest" crossorigin="use-credentials">`)
	}
	if !strings.Contains(index, `rel="apple-touch-icon"`) {
		t.Error("index.html has no apple-touch-icon link")
	}
	if !strings.Contains(index, `name="theme-color" content="#fafbfc"`) {
		t.Error("index.html theme-color is not #fafbfc")
	}

	sw := read("sw.js")
	build := regexp.MustCompile(`const BUILD = '([0-9a-f]{12})'`).FindStringSubmatch(sw)
	if build == nil {
		t.Fatal("sw.js has no 12-hex BUILD")
	}
	if !regexp.MustCompile(`<meta[^>]*name="sous-build"[^>]*content="` + build[1] + `"`).MatchString(index) {
		t.Errorf("index.html sous-build meta does not match sw.js BUILD %s", build[1])
	}
	list := regexp.MustCompile(`(?s)const PRECACHE = (\[.*?\])`).FindStringSubmatch(sw)
	if list == nil {
		t.Fatal("sw.js has no PRECACHE list")
	}
	var precache []string
	if err := json.Unmarshal([]byte(list[1]), &precache); err != nil {
		t.Fatalf("PRECACHE is not a JSON array: %v", err)
	}
	if len(precache) == 0 {
		t.Error("PRECACHE is empty")
	}
	for _, p := range precache {
		if p == "/" {
			continue
		}
		if _, err := fs.Stat(dist, strings.TrimPrefix(p, "/")); err != nil {
			t.Errorf("PRECACHE lists %s, which is not in dist", p)
		}
	}
}
