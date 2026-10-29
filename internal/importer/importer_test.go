package importer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/recipes"
	"github.com/sroberts/sous/internal/safefetch"
	"github.com/sroberts/sous/internal/storage"
)

var bg = context.Background()

// site is a fake recipe site at http://recipes.test:<port>, reached through
// safefetch with test hooks that resolve recipes.test to the httptest
// server and allow only that server.
type site struct {
	t     *testing.T
	srv   *httptest.Server
	base  string
	mu    sync.Mutex
	pages map[string]func(w http.ResponseWriter, r *http.Request)
	dials atomic.Int64
	refs  sync.Map // path -> Referer seen
}

func newSite(t *testing.T) *site {
	s := &site{t: t, pages: map[string]func(http.ResponseWriter, *http.Request){}}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.refs.Store(r.URL.Path, r.Header.Get("Referer"))
		s.mu.Lock()
		h := s.pages[r.URL.Path]
		s.mu.Unlock()
		if h == nil {
			http.NotFound(w, r)
			return
		}
		h(w, r)
	}))
	t.Cleanup(s.srv.Close)
	_, port, _ := net.SplitHostPort(s.srv.Listener.Addr().String())
	s.base = "http://recipes.test:" + port
	return s
}

func (s *site) handle(path string, h func(w http.ResponseWriter, r *http.Request)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pages[path] = h
}

func (s *site) serve(path, contentType string, body []byte) {
	s.handle(path, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Write(body)
	})
}

func (s *site) client(timeout time.Duration) *safefetch.Client {
	ap := netip.MustParseAddrPort(s.srv.Listener.Addr().String())
	return safefetch.New(safefetch.Options{
		UserAgent: "Sous/test",
		Timeout:   timeout,
		Testing: &safefetch.TestHooks{
			AllowAddrPorts: []netip.AddrPort{ap},
			LookupNetIP: func(_ context.Context, host string) ([]netip.Addr, error) {
				if host == "recipes.test" {
					return []netip.Addr{ap.Addr()}, nil
				}
				return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
			},
			OnDial: func(netip.AddrPort) { s.dials.Add(1) },
		},
	})
}

type env struct {
	*site
	svc   *recipes.Service
	im    *Importer
	alice identity.Caller
	logs  *bytes.Buffer
}

func newEnv(t *testing.T) *env {
	return newEnvTimeout(t, 5*time.Second)
}

func newEnvTimeout(t *testing.T, timeout time.Duration) *env {
	t.Helper()
	st, err := storage.Open(bg, t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	svc := recipes.New(st, slog.New(slog.DiscardHandler))
	p, err := svc.CreatePerson(bg, identity.Caller{Via: identity.ViaAPI}, "Alice")
	if err != nil {
		t.Fatal(err)
	}
	s := newSite(t)
	logs := &bytes.Buffer{}
	im := New(svc, s.client(timeout), slog.New(slog.NewTextHandler(logs, nil)))
	alice := identity.Caller{Person: &identity.Person{Username: p.Username, Name: p.Name}, Via: identity.ViaAPI}
	return &env{site: s, svc: svc, im: im, alice: alice, logs: logs}
}

func recipePage(images ...string) []byte {
	imgs := make([]string, len(images))
	for i, u := range images {
		imgs[i] = fmt.Sprintf("%q", u)
	}
	return []byte(`<!doctype html><html><head><title>Lemon Bars | Test Kitchen</title>
<link rel="canonical" href="/lemon-bars/">
<script type="application/ld+json">{"@context":"https://schema.org","@type":"Recipe","name":"Lemon Bars Recipe",
"recipeIngredient":["1 cup butter","2 cups flour","4 eggs"],
"recipeInstructions":[{"@type":"HowToStep","text":"Heat the oven."},{"@type":"HowToStep","text":"Bake."}],
"prepTime":"PT20M","cookTime":"PT35M","recipeYield":"16 bars",
"image":[` + strings.Join(imgs, ",") + `]}</script></head><body><p>Lemon bars.</p></body></html>`)
}

func pngBytes(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes()
}

func gifBytes(t *testing.T, transparentPixel bool) []byte {
	t.Helper()
	pal := color.Palette{color.RGBA{200, 50, 50, 255}, color.RGBA{0, 0, 0, 0}}
	img := image.NewPaletted(image.Rect(0, 0, 8, 8), pal)
	if transparentPixel {
		img.SetColorIndex(3, 3, 1)
	}
	var b bytes.Buffer
	if err := gif.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

var avifBytes = append([]byte("\x00\x00\x00\x1cftypavif\x00\x00\x00\x00avifmif1miaf"), make([]byte, 64)...)

func importError(t *testing.T, err error) *recipes.ImportError {
	t.Helper()
	var ie *recipes.ImportError
	if !errors.As(err, &ie) {
		t.Fatalf("err = %v; want an ImportError", err)
	}
	return ie
}

func TestPreviewStoresImage(t *testing.T) {
	e := newEnv(t)
	e.serve("/lemon-bars", "text/html; charset=utf-8", recipePage("/img/bars-1200x800.png"))
	e.serve("/img/bars-1200x800.png", "image/png", pngBytes(12, 8))

	p, err := e.im.Preview(bg, e.alice, recipes.ImportRequest{URL: e.base + "/lemon-bars?utm_source=x"})
	if err != nil {
		t.Fatal(err)
	}
	d := p.Draft
	if d.Title != "Lemon Bars" || len(d.Ingredients) != 3 || len(d.Steps) != 2 {
		t.Errorf("draft = %+v", d)
	}
	if d.SourceURL == nil || *d.SourceURL != e.base+"/lemon-bars/" {
		t.Errorf("source_url = %v; want the canonical link", d.SourceURL)
	}
	if p.Report.Method != MethodJSONLD || p.Report.SourceURL != *d.SourceURL {
		t.Errorf("report = %+v", p.Report)
	}
	if len(p.Ingredients) != 3 || p.Duplicates == nil || len(p.Duplicates) != 0 {
		t.Errorf("ingredients = %d, duplicates = %v", len(p.Ingredients), p.Duplicates)
	}
	if len(p.Images) != 1 || p.Images[0].Hash == nil {
		t.Fatalf("images = %+v; want one stored", p.Images)
	}
	if len(d.Images) != 1 || d.Images[0] != *p.Images[0].Hash {
		t.Errorf("draft.images = %v; want the stored hash", d.Images)
	}
	if ref, _ := e.refs.Load("/img/bars-1200x800.png"); ref != e.base+"/lemon-bars?utm_source=x" {
		t.Errorf("image Referer = %q; want the page", ref)
	}
	if _, err := e.svc.Create(bg, e.alice, d); err != nil {
		t.Errorf("the draft doesn't save: %v", err)
	}

	log := e.logs.String()
	for _, want := range []string{"msg=import", "host=recipes.test", "path=/lemon-bars", "outcome=ok", "method=json-ld", "duration_ms="} {
		if !strings.Contains(log, want) {
			t.Errorf("log %q lacks %q", log, want)
		}
	}
	if strings.Contains(log, "utm_source") {
		t.Errorf("log %q carries the query string", log)
	}
}

func TestPreviewSkipsAVIFThenStoresNext(t *testing.T) {
	e := newEnv(t)
	e.serve("/r", "text/html", recipePage("/img/a-2000x1500.avif", "/img/b-1200x800.png", "/img/c-1000x800.png", "/img/d-900x600.png"))
	e.serve("/img/a-2000x1500.avif", "image/avif", avifBytes)
	e.serve("/img/b-1200x800.png", "image/png", []byte("not an image at all"))
	e.serve("/img/c-1000x800.png", "image/png", pngBytes(4, 4))
	e.serve("/img/d-900x600.png", "image/png", pngBytes(4, 4))

	p, err := e.im.Preview(bg, e.alice, recipes.ImportRequest{URL: e.base + "/r"})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Images) != 4 {
		t.Fatalf("images = %+v", p.Images)
	}
	if im := p.Images[0]; im.Problem == nil || !strings.Contains(*im.Problem, "AVIF") || im.Hash != nil {
		t.Errorf("avif candidate = %+v; want a Problem", im)
	}
	if im := p.Images[1]; im.Problem == nil || im.Hash != nil {
		t.Errorf("garbage candidate = %+v; want a Problem", im)
	}
	if im := p.Images[2]; im.Hash == nil || len(p.Draft.Images) != 1 || p.Draft.Images[0] != *im.Hash {
		t.Errorf("third candidate = %+v, draft.images = %v; want it stored as the cover", im, p.Draft.Images)
	}
	if im := p.Images[3]; im.Hash != nil || im.Problem != nil {
		t.Errorf("fourth candidate = %+v; want it untried", im)
	}
	if _, seen := e.refs.Load("/img/d-900x600.png"); seen {
		t.Error("fetched a fourth photo after one was stored")
	}
}

func TestPreviewTriesAtMostThreeImages(t *testing.T) {
	e := newEnv(t)
	e.serve("/r", "text/html", recipePage("/1.png", "/2.png", "/3.png", "/4.png"))
	e.serve("/4.png", "image/png", pngBytes(2, 2))
	p, err := e.im.Preview(bg, e.alice, recipes.ImportRequest{URL: e.base + "/r"})
	if err != nil {
		t.Fatal(err)
	}
	for i, im := range p.Images[:3] {
		if im.Problem == nil {
			t.Errorf("image %d = %+v; want a Problem (404)", i, im)
		}
	}
	if p.Images[3].Problem != nil || p.Images[3].Hash != nil || len(p.Draft.Images) != 0 {
		t.Errorf("fourth image was tried: %+v", p.Images[3])
	}
}

func TestPreviewConvertsGIF(t *testing.T) {
	for _, tc := range []struct {
		name        string
		transparent bool
		want        string
	}{
		{"opaque becomes jpeg", false, "image/jpeg"},
		{"transparent becomes png", true, "image/png"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.serve("/r", "text/html", recipePage("/img/x.gif"))
			e.serve("/img/x.gif", "image/gif", gifBytes(t, tc.transparent))
			p, err := e.im.Preview(bg, e.alice, recipes.ImportRequest{URL: e.base + "/r"})
			if err != nil {
				t.Fatal(err)
			}
			if p.Images[0].Hash == nil {
				t.Fatalf("gif not stored: %+v", p.Images[0])
			}
			f, meta, err := e.svc.Image(bg, e.alice, *p.Images[0].Hash)
			if err != nil {
				t.Fatal(err)
			}
			f.Close()
			if meta.ContentType != tc.want {
				t.Errorf("stored as %s; want %s", meta.ContentType, tc.want)
			}
		})
	}
}

func TestSniffImage(t *testing.T) {
	for _, tc := range []struct {
		b    string
		want string
	}{
		{"GIF89a....", "gif"},
		{"II*\x00....", "TIFF"},
		{"MM\x00*....", "TIFF"},
		{"BM......", "BMP"},
		{string(avifBytes), "AVIF"},
		{`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"></svg>`, "SVG"},
		{string(pngBytes(1, 1)), ""},
		{"\xff\xd8\xff\xe0", ""},
	} {
		if got := sniffImage([]byte(tc.b)); got != tc.want {
			t.Errorf("sniffImage(%q) = %q; want %q", tc.b[:min(len(tc.b), 12)], got, tc.want)
		}
	}
}

func TestPreviewDuplicates(t *testing.T) {
	e := newEnv(t)
	e.serve("/lemon-bars", "text/html", recipePage())
	link := func(s string) *string { return &s }
	live, err := e.svc.Create(bg, e.alice, recipes.RecipeInput{Title: "Bars", SourceURL: link(strings.Replace(e.base, "http://", "https://www.", 1) + "/lemon-bars")})
	if err != nil {
		t.Fatal(err)
	}
	gone, err := e.svc.Create(bg, e.alice, recipes.RecipeInput{Title: "Old bars", SourceURL: link(e.base + "/lemon-bars/#top")})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Delete(bg, e.alice, gone.ID, gone.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Create(bg, e.alice, recipes.RecipeInput{Title: "Other", SourceURL: link(e.base + "/other")}); err != nil {
		t.Fatal(err)
	}

	p, err := e.im.Preview(bg, e.alice, recipes.ImportRequest{URL: e.base + "/lemon-bars"})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Duplicates) != 2 {
		t.Fatalf("duplicates = %+v; want the live and the trashed copy", p.Duplicates)
	}
	if d := p.Duplicates[0]; d.ID != live.ID || d.Deleted {
		t.Errorf("first duplicate = %+v; want the live one", d)
	}
	if d := p.Duplicates[1]; d.ID != gone.ID || !d.Deleted {
		t.Errorf("second duplicate = %+v; want the trashed one", d)
	}

	if _, err := e.im.Import(bg, e.alice, e.base+"/lemon-bars", true); recipeCode(err) != recipes.CodeExists {
		t.Errorf("save over a duplicate: err = %v; want exists", err)
	}
}

func recipeCode(err error) string {
	var re *recipes.Error
	if errors.As(err, &re) {
		return re.Code
	}
	return ""
}

func TestImportSave(t *testing.T) {
	e := newEnv(t)
	e.serve("/r", "text/html", recipePage("/img/p.png"))
	e.serve("/img/p.png", "image/png", pngBytes(3, 3))

	res, err := e.im.Import(bg, e.alice, e.base+"/r", false)
	if err != nil || res.Saved != nil {
		t.Fatalf("preview only: saved = %v, err = %v", res.Saved, err)
	}
	res, err = e.im.Import(bg, e.alice, e.base+"/r", true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Saved == nil || res.Saved.Title != "Lemon Bars" || len(res.Saved.Images) != 1 || len(res.Saved.Ingredients) != 3 {
		t.Fatalf("saved = %+v", res.Saved)
	}
	if _, err := e.im.Import(bg, e.alice, e.base+"/r", true); recipeCode(err) != recipes.CodeExists {
		t.Errorf("second save: err = %v; want exists", err)
	}
}

func TestPreviewText(t *testing.T) {
	e := newEnv(t)
	text := "Pancakes\n\nIngredients\n1 cup flour\n1 egg\n\nMethod\n1 Whisk everything.\n2 Fry in butter."
	p, err := e.im.Preview(bg, e.alice, recipes.ImportRequest{Text: text})
	if err != nil {
		t.Fatal(err)
	}
	if p.Draft.Title != "Pancakes" || len(p.Draft.Ingredients) != 2 || p.Draft.Steps[0] != "Whisk everything." || p.Draft.SourceURL != nil {
		t.Errorf("draft = %+v", p.Draft)
	}
	if p.Report.Method != MethodText || p.Images == nil || p.Duplicates == nil {
		t.Errorf("preview = %+v", p)
	}

	// A link sent with text is credited, not fetched.
	src := e.base + "/pancakes?utm_medium=email"
	link := e.base + "/pancakes"
	if _, err := e.svc.Create(bg, e.alice, recipes.RecipeInput{Title: "Pancakes", SourceURL: &link}); err != nil {
		t.Fatal(err)
	}
	p, err = e.im.Preview(bg, e.alice, recipes.ImportRequest{Text: text, URL: src, Title: "Sunday Pancakes"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Draft.Title != "Sunday Pancakes" || p.Draft.SourceURL == nil || *p.Draft.SourceURL != link || p.Report.SourceURL != link {
		t.Errorf("draft = %+v, report = %+v", p.Draft, p.Report)
	}
	if len(p.Duplicates) != 1 {
		t.Errorf("duplicates = %+v; want the existing copy", p.Duplicates)
	}
	if n := e.dials.Load(); n != 0 {
		t.Errorf("text mode dialed %d times", n)
	}

	_, err = e.im.Preview(bg, e.alice, recipes.ImportRequest{Text: "   ", URL: ""})
	if recipeCode(err) != recipes.CodeInvalid {
		t.Errorf("blank text: err = %v; want invalid", err)
	}
	// Bullets alone split into nothing.
	_, err = e.im.Preview(bg, e.alice, recipes.ImportRequest{Text: "•\n-\n*", URL: src})
	ie := importError(t, err)
	if ie.Code != recipes.CodeNoRecipe || ie.Page == nil || ie.Page.SourceURL != link {
		t.Errorf("empty split: %+v", ie)
	}
}

func TestPreviewNeedsPerson(t *testing.T) {
	e := newEnv(t)
	_, err := e.im.Preview(bg, identity.Caller{Via: identity.ViaAPI}, recipes.ImportRequest{URL: e.base + "/r"})
	if recipeCode(err) != recipes.CodeWhoAreYou {
		t.Errorf("err = %v; want who_are_you", err)
	}
}

func TestPreviewErrors(t *testing.T) {
	e := newEnvTimeout(t, 300*time.Millisecond)
	e.handle("/gone", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(404)
		w.Write([]byte(`<title>Not here</title><meta property="og:image" content="/i.jpg">`))
	})
	e.handle("/wall", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cf-Mitigated", "challenge")
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(403)
		w.Write([]byte(`<title>Just a moment...</title>`))
	})
	e.handle("/broken", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(502) })
	e.handle("/slow", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	})
	e.serve("/pdf", "application/pdf", []byte("%PDF-1.4"))
	e.serve("/huge", "text/html", bytes.Repeat([]byte("a"), safefetch.MaxPageBytes+1))
	e.serve("/blog", "text/html", []byte(`<title>A post about my week</title><p>Nothing to cook.</p>`))
	e.serve("/big-blog", "text/html", append([]byte(`<title>Long post</title><form><div class="captcha"></div></form>`), bytes.Repeat([]byte("<p>words</p>"), 10<<10)...))
	e.serve("/reddit", "text/html", []byte(`<title>Reddit</title><div>js_challenge</div>`))
	e.serve("/paid", "text/html", []byte(`<title>Stew</title><script type="application/ld+json">{"@type":"NewsArticle","isAccessibleForFree":false}</script>`))

	local := httptest.NewServer(http.NotFoundHandler())
	defer local.Close()

	for _, tc := range []struct {
		name, url, code, message string
		page                     bool
	}{
		{"404", e.base + "/gone", recipes.CodePageNotFound, "", true},
		{"bot wall", e.base + "/wall", recipes.CodeBlocked, "", true},
		{"5xx", e.base + "/broken", recipes.CodeSiteError, "", true},
		{"timeout", e.base + "/slow", recipes.CodeTimeout, "", true},
		{"not html", e.base + "/pdf", recipes.CodeUnsupportedURL, "", true},
		{"too large", e.base + "/huge", recipes.CodePageTooLarge, "", true},
		{"no recipe", e.base + "/blog", recipes.CodeNoRecipe, "", true},
		{"big page with a comment captcha", e.base + "/big-blog", recipes.CodeNoRecipe, "", true},
		{"200 challenge shell", e.base + "/reddit", recipes.CodeBlocked, "", true},
		{"paywalled", e.base + "/paid", recipes.CodePaywalled, "", true},
		{"nxdomain", "http://nowhere.test/r", recipes.CodePageNotFound, MessageNoHost, true},
		{"local address", local.URL + "/x", recipes.CodeUnsupportedURL, MessageLocalAddress, true},
		{"ftp", "ftp://recipes.test/r", recipes.CodeUnsupportedURL, Message(recipes.CodeUnsupportedURL), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.im.Preview(bg, e.alice, recipes.ImportRequest{URL: tc.url})
			ie := importError(t, err)
			if ie.Code != tc.code {
				t.Fatalf("code = %s (%s); want %s", ie.Code, ie.Message, tc.code)
			}
			if ie.Message == "" || (tc.message != "" && ie.Message != tc.message) {
				t.Errorf("message = %q; want %q", ie.Message, tc.message)
			}
			if tc.page && (ie.Page == nil || ie.Page.SourceURL == "") {
				t.Errorf("page = %+v; want a fallback", ie.Page)
			}
		})
	}

	_, err := e.im.Preview(bg, e.alice, recipes.ImportRequest{URL: e.base + "/gone"})
	if ie := importError(t, err); ie.Page.Title == nil || *ie.Page.Title != "Not here" || ie.Page.Image == nil {
		t.Errorf("404 fallback page = %+v", ie.Page)
	}
	if !strings.Contains(e.logs.String(), "outcome=not_found") {
		t.Errorf("log lacks the error outcome: %s", e.logs.String())
	}
}

// stubFetcher records what was fetched and serves canned pages.
type stubFetcher struct {
	mu   sync.Mutex
	got  []string
	page func(u string) *safefetch.Response
}

func (s *stubFetcher) Get(_ context.Context, rawURL string, kind safefetch.Kind, _ string) (*safefetch.Response, error) {
	s.mu.Lock()
	s.got = append(s.got, rawURL)
	s.mu.Unlock()
	if kind == safefetch.KindImage {
		return nil, safefetch.ErrTooLarge
	}
	return s.page(rawURL), nil
}

func TestPreviewWayback(t *testing.T) {
	e := newEnv(t)
	stub := &stubFetcher{page: func(u string) *safefetch.Response {
		pu, _ := http.NewRequest("GET", u, nil)
		return &safefetch.Response{URL: pu.URL, Status: 200, Header: http.Header{"Content-Type": {"text/html"}},
			Body: recipePage("https://imagesvc.example/bars.jpg")}
	}}
	im := New(e.svc, stub, nil)
	const ts = "20190405123456"
	p, err := im.Preview(bg, e.alice, recipes.ImportRequest{URL: "https://web.archive.org/web/" + ts + "/https://dead.example/bars?utm_source=x"})
	if err != nil {
		t.Fatal(err)
	}
	if stub.got[0] != "https://web.archive.org/web/"+ts+"id_/https://dead.example/bars?utm_source=x" {
		t.Errorf("fetched %s; want the id_ copy", stub.got[0])
	}
	if !p.Report.Archived || p.Report.FetchedURL == nil || p.Draft.SourceURL == nil || !strings.HasPrefix(*p.Draft.SourceURL, "https://dead.example/") {
		t.Errorf("report = %+v, source = %v", p.Report, p.Draft.SourceURL)
	}
	if len(stub.got) < 2 || stub.got[1] != "https://web.archive.org/web/"+ts+"im_/https://imagesvc.example/bars.jpg" {
		t.Errorf("image fetches = %v; want the im_ copy", stub.got[1:])
	}
}

func TestImageLimitMatchesSafefetch(t *testing.T) {
	if MaxImageBytes != safefetch.MaxImageBytes {
		t.Fatal("MaxImageBytes drifted from safefetch")
	}
}
