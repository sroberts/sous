package importer

import (
	"encoding/json"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestScanHead(t *testing.T) {
	p := page(t, "https://www.example.com/a/b",
		`<title>  Garlic &amp; Noodles | Site </title>
		<meta property="og:title" content="Garlic Noodles">
		<meta property="og:site_name" content="The Woks of Life">
		<meta property="og:image" content="/wp/garlic.jpg">
		<meta property="og:image:width" content="1200"><meta property="og:image:height" content="800">
		<meta name="twitter:image" content="https://cdn.example.com/tw.jpg">
		<link rel="canonical" href="/garlic-noodles/">`,
		`<svg><title>icon</title></svg>`)
	want := Page{
		SourceURL: "https://www.example.com/a/b", FetchedURL: "https://www.example.com/a/b",
		Title: "Garlic & Noodles | Site", OGTitle: "Garlic Noodles", OGSiteName: "The Woks of Life",
		OGImage: "https://www.example.com/wp/garlic.jpg", OGImageWidth: 1200, OGImageHeight: 800,
		TwitterImage: "https://cdn.example.com/tw.jpg", Canonical: "https://www.example.com/garlic-noodles/",
	}
	if !reflect.DeepEqual(p, want) {
		t.Errorf("Scan =\n%+v\nwant\n%+v", p, want)
	}
}

func TestScanMarkers(t *testing.T) {
	big := strings.Repeat("<p>filler text</p>", 4000)
	for _, tc := range []struct {
		name, head, body string
		want             []string
	}{
		{"cloudflare", "<title>Just a moment...</title>", `<script src="/cdn-cgi/challenge-platform/h/b/orchestrate/chl_page/v1?ray=1"></script><form id="cf-chl-form"></form>`, []string{"just a moment", "cf-chl", "challenge"}},
		{"reddit bot wall", "<title>Reddit</title>", `<div>Please wait for verification. js_challenge</div>`, []string{"challenge"}},
		{"perimeterx", "", `<div id="px-captcha"></div>`, []string{"captcha", "px-captcha"}},
		{"datadome", "", `<script src="https://ct.captcha-delivery.com/c.js"></script><p>DataDome</p>`, []string{"captcha", "datadome"}},
		{"js", "", `<noscript>Please enable JavaScript to continue.</noscript>`, []string{"enable javascript"}},
		{"challenge on big page ignored", "", big + "challenge", nil},
		{"challenge with recipe ignored", ldScript(`{"@type":"Recipe","name":"Cookie challenge","recipeIngredient":["1 egg"]}`), "", nil},
		{"clean", "<title>Soup</title>", "<p>Soup.</p>", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := page(t, "https://example.com/", tc.head, tc.body)
			if !reflect.DeepEqual(p.Markers, tc.want) {
				t.Errorf("markers = %q; want %q", p.Markers, tc.want)
			}
		})
	}
}

func TestScanCharset(t *testing.T) {
	// "Crème brûlée – “best”" in Windows-1252.
	body := []byte("<html><head><meta charset=\"windows-1252\"><title>Cr\xe8me br\xfbl\xe9e \x96 \x93best\x94</title></head></html>")
	u, _ := url.Parse("https://example.com/")
	if p := Scan(body, "text/html", u); p.Title != "Crème brûlée – “best”" {
		t.Errorf("meta charset title = %q", p.Title)
	}
	if p := Scan([]byte("<title>Caf\xe9</title>"), "text/html; charset=ISO-8859-1", u); p.Title != "Café" {
		t.Errorf("header charset title = %q", p.Title)
	}
	// Declared Latin-1 but really UTF-8: left alone.
	if p := Scan([]byte("<title>Café</title>"), "text/html; charset=iso-8859-1", u); p.Title != "Café" {
		t.Errorf("misdeclared title = %q", p.Title)
	}
	if p := Scan([]byte("\xef\xbb\xbf<title>BOM</title>"), "text/html", u); p.Title != "BOM" {
		t.Errorf("BOM title = %q", p.Title)
	}
}

func TestScanKeepsOnlyRecipeData(t *testing.T) {
	p := page(t, "https://example.com/r",
		`<style>body{}</style>`+
			ldScript(`{"@type":"Organization","name":"Not a recipe"}`)+
			ldScript(`{"@context":"https://schema.org","@graph":[{"@type":"WebPage","@id":"#page","name":"Huge page","breadcrumb":{"@id":"#bc"}},{"@type":"BreadcrumbList","@id":"#bc"},{"@type":"Recipe","name":"Soup","author":{"@id":"#me"},"recipeIngredient":["1 leek"]},{"@type":"Person","@id":"#me","name":"Cook"}]}`),
		`<div itemscope itemtype="https://schema.org/Recipe"><span itemprop="name">Soup</span><script>track()</script><style>.x{}</style><!-- ad --></div>`)
	if len(p.JSONLD) != 1 {
		t.Fatalf("JSONLD = %s", p.JSONLD)
	}
	var v map[string]any
	if err := json.Unmarshal(p.JSONLD[0], &v); err != nil {
		t.Fatal(err)
	}
	g := v["@graph"].([]any)
	if !reflect.DeepEqual(g[0], map[string]any{"@type": "WebPage", "@id": "#page"}) || !reflect.DeepEqual(g[3].(map[string]any)["name"], "Cook") {
		t.Errorf("graph = %v", g)
	}
	if p.Microdata != `<div itemscope="" itemtype="https://schema.org/Recipe"><span itemprop="name">Soup</span></div>` {
		t.Errorf("microdata = %q", p.Microdata)
	}
	b, _ := json.Marshal(p)
	for _, s := range []string{"<html", "<head", "<body", "track()", "Not a recipe", "Huge page"} {
		if strings.Contains(string(b), s) {
			t.Errorf("serialized page holds %q: %s", s, b)
		}
	}
}

func TestScanNotHTML(t *testing.T) {
	u, _ := url.Parse("https://example.com/x.json")
	p := Scan([]byte(`{"a":1}`), "application/json", u)
	if p.Title != "" || len(p.JSONLD) != 0 {
		t.Errorf("page = %+v", p)
	}
}
