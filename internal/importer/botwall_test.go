package importer

import (
	"net/http"
	"testing"

	"github.com/sroberts/sous/internal/recipes"
)

// Generic markers ("captcha", "enable javascript", "challenge") turn up in
// ordinary pages: a comment form, a noscript notice. They mean a bot wall
// only on a small page or one that wasn't a plain 200, and never when a
// recipe was read. Strong markers always do.
func TestBotWallMarkers(t *testing.T) {
	html := http.Header{"Content-Type": {"text/html"}}
	const big = 200 << 10
	for _, tc := range []struct {
		name      string
		status    int
		markers   []string
		size      int
		extracted bool
		want      string
	}{
		{"big page, comment captcha", 200, []string{"captcha"}, big, false, recipes.CodeNoRecipe},
		{"big page, noscript notice", 200, []string{"enable javascript"}, big, false, recipes.CodeNoRecipe},
		{"small page, captcha", 200, []string{"captcha"}, 4 << 10, false, recipes.CodeBlocked},
		{"unknown size counts as small", 200, []string{"enable javascript"}, 0, false, recipes.CodeBlocked},
		{"big 503 with captcha", 503, []string{"captcha"}, big, false, recipes.CodeBlocked},
		{"big page, just a moment", 200, []string{"just a moment"}, big, false, recipes.CodeBlocked},
		{"big page, cf-chl", 200, []string{"cf-chl"}, big, false, recipes.CodeBlocked},
		{"big page, px-captcha", 200, []string{"captcha", "px-captcha"}, big, false, recipes.CodeBlocked},
		{"big page, datadome", 200, []string{"datadome"}, big, false, recipes.CodeBlocked},
		{"small page with a recipe", 200, []string{"captcha", "enable javascript"}, 4 << 10, true, ""},
		{"not free only", 200, []string{MarkerNotFree}, 4 << 10, false, recipes.CodePaywalled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := Page{Markers: tc.markers, Size: tc.size}
			if got := Classify(tc.status, html, p, tc.extracted); got != tc.want {
				t.Errorf("Classify = %q; want %q", got, tc.want)
			}
		})
	}
}
