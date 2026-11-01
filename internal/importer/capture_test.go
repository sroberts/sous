package importer

import (
	"errors"
	"net/http"
	"sync"
	"testing"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/recipes"
	"github.com/sroberts/sous/internal/safefetch"
)

func TestCaptureSkipsFetchForKnownLink(t *testing.T) {
	e := newEnv(t)
	stub := &stubFetcher{page: func(u string) *safefetch.Response {
		pu, _ := http.NewRequest("GET", u, nil)
		return &safefetch.Response{URL: pu.URL, Status: 200, Header: http.Header{"Content-Type": {"text/html"}}, Body: recipePage()}
	}}
	im := New(e.svc, stub, nil)
	src := "https://dead.example/bars"
	rec, err := e.svc.Create(bg, e.alice, recipes.RecipeInput{Title: "Lemon Bars", SourceURL: &src})
	if err != nil {
		t.Fatal(err)
	}
	for _, link := range []string{
		"http://www.dead.example/bars/?utm_source=x#top",
		"https://web.archive.org/web/20190405123456/https://dead.example/bars",
	} {
		got, err := im.Capture(bg, e.alice, link)
		if err != nil {
			t.Fatalf("%s: %v", link, err)
		}
		if got.ID != rec.ID || !got.Duplicate || got.Restored || got.Path != "/r/"+rec.ID || got.URL != nil {
			t.Errorf("%s: capture = %+v", link, got)
		}
	}
	if len(stub.got) != 0 {
		t.Errorf("fetched %v; a known link needs no fetch", stub.got)
	}

	// An unknown link is fetched, saved and credited.
	got, err := im.Capture(bg, e.alice, "https://fresh.example/bars")
	if err != nil {
		t.Fatal(err)
	}
	if got.Duplicate || got.Title != "Lemon Bars" || got.Version != 1 || got.Summary != "Saved Lemon Bars." || len(stub.got) == 0 {
		t.Errorf("capture = %+v, fetched %v", got, stub.got)
	}
}

func TestCaptureNeedsPersonAndURL(t *testing.T) {
	e := newEnv(t)
	var re *recipes.Error
	if _, err := e.im.Capture(bg, identity.Caller{Via: identity.ViaAPI}, "https://example.com/"); !errors.As(err, &re) || re.Code != recipes.CodeWhoAreYou {
		t.Errorf("nobody: err = %v; want who_are_you", err)
	}
	if _, err := e.im.Capture(bg, e.alice, "  "); !errors.As(err, &re) || re.Code != recipes.CodeInvalid {
		t.Errorf("blank: err = %v; want invalid", err)
	}
}

func TestSavedSummary(t *testing.T) {
	for _, tc := range []struct {
		missing []string
		want    string
	}{
		{nil, "Saved Pho."},
		{[]string{"times", "yield", "image", "title"}, "Saved Pho."},
		{[]string{"steps", "times"}, "Saved Pho, but no steps were found."},
		{[]string{"ingredients"}, "Saved Pho, but no ingredients were found."},
		{[]string{"steps", "ingredients"}, "Saved Pho, but no ingredients or steps were found."},
	} {
		if got := savedSummary("Pho", tc.missing); got != tc.want {
			t.Errorf("savedSummary(%v) = %q; want %q", tc.missing, got, tc.want)
		}
	}
}

func TestHostOf(t *testing.T) {
	for _, tc := range []struct {
		links []string
		want  string
	}{
		{[]string{"https://WWW.Example.com/x"}, "example.com"},
		{[]string{"", "http://recipes.test:8080/a"}, "recipes.test"},
		{[]string{"not a link"}, "the web"},
	} {
		if got := hostOf(tc.links...); got != tc.want {
			t.Errorf("hostOf(%q) = %q; want %q", tc.links, got, tc.want)
		}
	}
}

func TestEveryCodeIsSpoken(t *testing.T) {
	for code := range messages {
		if Spoken(code) == "" {
			t.Errorf("%s has a message but nothing to say aloud", code)
		}
	}
	for code := range spoken {
		if Message(code) == "" {
			t.Errorf("%s is spoken but has no message", code)
		}
	}
}

func TestCaptureTwiceAtOnceSavesOnce(t *testing.T) {
	e := newEnv(t)
	// Both captures read the page before either saves, as when a shortcut
	// is run twice in a row.
	var arrived sync.WaitGroup
	arrived.Add(2)
	stub := &stubFetcher{page: func(u string) *safefetch.Response {
		arrived.Done()
		arrived.Wait()
		pu, _ := http.NewRequest("GET", u, nil)
		return &safefetch.Response{URL: pu.URL, Status: 200, Header: http.Header{"Content-Type": {"text/html"}}, Body: recipePage()}
	}}
	im := New(e.svc, stub, nil)
	results := make([]recipes.CaptureResult, 2)
	var done sync.WaitGroup
	for i := range results {
		done.Add(1)
		go func() {
			defer done.Done()
			var err error
			if results[i], err = im.Capture(bg, e.alice, "https://twice.example/bars"); err != nil {
				t.Error(err)
			}
		}()
	}
	done.Wait()
	if results[0].ID != results[1].ID || results[0].Duplicate == results[1].Duplicate {
		t.Errorf("captures = %+v; want one saved and one duplicate of it", results)
	}
	rec, err := e.svc.Get(bg, e.alice, results[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	dups, err := e.svc.FindBySource(bg, e.alice, *rec.SourceURL)
	if err != nil {
		t.Fatal(err)
	}
	if len(dups) != 1 {
		t.Errorf("%d recipes from the link; want 1", len(dups))
	}
}
