package importer

import (
	"net/http"
	"testing"

	"github.com/sroberts/sous/internal/recipes"
)

func TestClassify(t *testing.T) {
	html := http.Header{"Content-Type": {"text/html; charset=utf-8"}}
	with := func(k, v string) http.Header {
		h := html.Clone()
		h.Set(k, v)
		return h
	}
	empty := Page{}
	wall := Page{Markers: []string{"just a moment", "cf-chl"}}
	reddit := Page{Title: "Reddit", Markers: []string{"challenge"}}
	paywall := Page{Markers: []string{MarkerNotFree}}
	for _, tc := range []struct {
		name      string
		status    int
		header    http.Header
		page      Page
		extracted bool
		want      string
	}{
		{"recipe", 200, html, empty, true, ""},
		{"no content type", 200, http.Header{}, empty, true, ""},
		{"xhtml", 200, http.Header{"Content-Type": {"application/xhtml+xml"}}, empty, true, ""},
		{"no recipe", 200, html, empty, false, recipes.CodeNoRecipe},
		{"404", 404, html, empty, false, recipes.CodePageNotFound},
		{"410", 410, html, empty, false, recipes.CodePageNotFound},
		{"401", 401, html, empty, false, recipes.CodeBlocked},
		{"403", 403, html, wall, false, recipes.CodeBlocked},
		{"429", 429, html, empty, false, recipes.CodeBlocked},
		{"451", 451, html, empty, false, recipes.CodeBlocked},
		{"503 cf-mitigated", 503, with("Cf-Mitigated", "challenge"), empty, false, recipes.CodeBlocked},
		{"503 challenge page", 503, html, wall, false, recipes.CodeBlocked},
		{"503 plain", 503, html, empty, false, recipes.CodeSiteError},
		{"500", 500, html, empty, false, recipes.CodeSiteError},
		{"502", 502, html, empty, false, recipes.CodeSiteError},
		{"200 reddit bot wall", 200, html, reddit, false, recipes.CodeBlocked},
		{"200 cloudflare wall", 200, html, wall, false, recipes.CodeBlocked},
		{"200 cf-mitigated header", 200, with("Cf-Mitigated", "challenge"), empty, false, recipes.CodeBlocked},
		{"not free, no recipe", 200, html, paywall, false, recipes.CodePaywalled},
		{"not free with recipe", 200, html, paywall, true, ""},
		{"bot markers but a recipe", 200, html, Page{Markers: []string{"captcha"}}, true, ""},
		{"pdf", 200, http.Header{"Content-Type": {"application/pdf"}}, empty, false, recipes.CodeUnsupportedURL},
		{"image", 200, http.Header{"Content-Type": {"image/jpeg"}}, empty, false, recipes.CodeUnsupportedURL},
		{"400", 400, html, empty, false, recipes.CodeNoRecipe},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify(tc.status, tc.header, tc.page, tc.extracted); got != tc.want {
				t.Errorf("Classify = %q; want %q", got, tc.want)
			}
		})
	}
}

func TestMessages(t *testing.T) {
	for _, c := range []string{recipes.CodeBlocked, recipes.CodePaywalled, recipes.CodePageNotFound, recipes.CodeNoRecipe,
		recipes.CodeTimeout, recipes.CodePageTooLarge, recipes.CodeUnsupportedURL, recipes.CodeSiteError} {
		if Message(c) == "" {
			t.Errorf("no message for %s", c)
		}
	}
}
