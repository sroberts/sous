package ui

import (
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

// Without Outpost nothing serves /.outpost/theme.css but Sous, and it must be
// a 404 rather than the page, so the stylesheet is dropped and the tokens'
// own values stand.
func TestOutpostThemeAbsent(t *testing.T) {
	dist := fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>Sous</title>")}}
	h := newHandler(dist, slog.New(slog.DiscardHandler))
	for _, target := range []string{"/.outpost/theme.css", "/.outpost/theme.json"} {
		if rec := serve(t, h, http.MethodGet, target); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", target, rec.Code)
		}
	}
}

// TestDistLinksOutpostTheme checks the built page links the theme ahead of the
// build's own CSS, so its --op-* properties are there for the tokens to take.
func TestDistLinksOutpostTheme(t *testing.T) {
	if os.Getenv("SOUS_REQUIRE_UI") != "1" {
		t.Skip("set SOUS_REQUIRE_UI=1 to check the built UI")
	}
	index, err := fs.ReadFile(files, "dist/index.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(index)
	theme := strings.Index(page, `href="/.outpost/theme.css"`)
	own := strings.Index(page, `href="/assets/`)
	if theme < 0 {
		t.Fatal("index.html does not link /.outpost/theme.css")
	}
	if own >= 0 && own < theme {
		t.Error("index.html links the build's CSS before /.outpost/theme.css")
	}
}
