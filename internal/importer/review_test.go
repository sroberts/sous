package importer

import (
	"image"
	"image/color"
	"net/http"
	"testing"

	"github.com/sroberts/sous/internal/recipes"
	"github.com/sroberts/sous/internal/safefetch"
)

// A relative og:image on an archived page belongs to the original site.
func TestPreviewWaybackRelativeImage(t *testing.T) {
	e := newEnv(t)
	page := []byte(`<html><head><title>Bars</title><meta property="og:image" content="/wp-content/bars.jpg"></head><body></body></html>`)
	stub := &stubFetcher{page: func(u string) *safefetch.Response {
		r, _ := http.NewRequest("GET", u, nil)
		return &safefetch.Response{URL: r.URL, Status: 200, Header: http.Header{"Content-Type": {"text/html"}}, Body: page}
	}}
	im := New(e.svc, stub, nil)
	const ts = "20190405123456"
	_, err := im.Preview(bg, e.alice, recipes.ImportRequest{URL: "https://web.archive.org/web/" + ts + "/https://dead.example/bars"})
	ie, ok := err.(*recipes.ImportError)
	if !ok || ie.Page == nil || ie.Page.Image == nil {
		t.Fatalf("err = %v; want a no-recipe error with the page photo", err)
	}
	if want := "https://web.archive.org/web/" + ts + "im_/https://dead.example/wp-content/bars.jpg"; ie.Page.Image.URL != want {
		t.Errorf("image = %s, want %s", ie.Page.Image.URL, want)
	}
}

func TestCanonicalHomepageIgnored(t *testing.T) {
	p := Page{FetchedURL: "https://site.example/recipes/bars/", Canonical: "https://site.example/"}
	if got := sourceURL(p, false, ""); got != "https://site.example/recipes/bars/" {
		t.Errorf("source = %s; a homepage canonical must not replace the page", got)
	}
	p.Canonical = "https://site.example/bars"
	if got := sourceURL(p, false, ""); got != "https://site.example/bars" {
		t.Errorf("source = %s; a same-host page canonical should win", got)
	}
}

func TestTransparentPaletted(t *testing.T) {
	opaque := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.RGBA{255, 0, 0, 255}})
	if transparent(opaque) {
		t.Error("opaque palette reported transparent")
	}
	clear := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.RGBA{0, 0, 0, 0}})
	if !transparent(clear) {
		t.Error("transparent palette reported opaque")
	}
}

// Pasted steps are un-numbered once: "1 cup flour" in a numbered step stays.
func TestPastedStepsKeepQuantities(t *testing.T) {
	e := newEnv(t)
	im := New(e.svc, &stubFetcher{}, nil)
	p, err := im.Preview(bg, e.alice, recipes.ImportRequest{Text: "Ingredients\n1 cup flour\n2 eggs\nDirections\n1. 1 cup flour goes in first\n2. 2 eggs next"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"1 cup flour goes in first", "2 eggs next"}; len(p.Draft.Steps) != 2 || p.Draft.Steps[0] != want[0] || p.Draft.Steps[1] != want[1] {
		t.Errorf("steps = %q, want %q", p.Draft.Steps, want)
	}
}
