package samples

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/recipes"
	"github.com/sroberts/sous/internal/storage"
)

const (
	// wantCount is how many samples ship. Change it with the data.
	wantCount = 12
	// maxImageBytes keeps the binary, and every new library, small.
	maxImageBytes = 200 << 10
)

// courses are the course tags a sample may lead with. The UI treats a
// recipe's first tag as its course, so this catches "Historic", or a typo,
// becoming one of the library's courses.
var courses = []string{"Main", "Side", "Salad", "Soup", "Bread", "Dessert", "Breakfast", "Drink", "Snack"}

func TestAll(t *testing.T) {
	all, err := All()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != wantCount {
		t.Fatalf("%d samples, want %d", len(all), wantCount)
	}
	titles := map[string]bool{}
	for _, sm := range all {
		r := sm.Recipe
		if titles[r.Title] {
			t.Errorf("duplicate title %q", r.Title)
		}
		titles[r.Title] = true
		historic := slices.Contains(r.Tags, "Historic")
		if len(r.Tags) < 2 || !slices.Contains(courses, r.Tags[0]) || historic == slices.Contains(r.Tags, "Classic") {
			t.Errorf("%s: tags %v, want a course first and one of Historic or Classic", r.Title, r.Tags)
		}
		// A historic recipe cites its book; a classic one may have no source.
		if (historic && r.SourceURL == nil) || (r.SourceURL != nil && !strings.HasPrefix(*r.SourceURL, "https://")) {
			t.Errorf("%s: source_url %v", r.Title, r.SourceURL)
		}
		if r.Description == nil || r.Notes == nil || r.YieldText == nil {
			t.Errorf("%s: needs a description, notes and yield", r.Title)
		}
		if len(r.Ingredients) == 0 || len(r.Steps) == 0 {
			t.Errorf("%s: %d ingredients, %d steps", r.Title, len(r.Ingredients), len(r.Steps))
		}
		for _, ing := range recipes.ParseIngredientLines(r.Ingredients) {
			if ing.Item == "" {
				t.Errorf("%s: %q parses to no item", r.Title, ing.Raw)
			}
		}
		if sm.Image == nil {
			continue
		}
		if len(sm.Image) > maxImageBytes {
			t.Errorf("%s: image is %d KB, over %d KB", r.Title, len(sm.Image)>>10, maxImageBytes>>10)
		}
		if _, _, err := image.DecodeConfig(bytes.NewReader(sm.Image)); err != nil {
			t.Errorf("%s: image: %v", r.Title, err)
		}
	}
}

func TestLoadRejects(t *testing.T) {
	recipe := func(extra string) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte(`{"title": "Soup"` + extra + `}`)}
	}
	png := &fstest.MapFile{Data: []byte("\x89PNG")}
	for name, files := range map[string]fstest.MapFS{
		"unknown field":   {"d/01-soup.json": recipe(`, "serves": 4`)},
		"images":          {"d/01-soup.json": recipe(`, "images": ["abc"]`)},
		"no title":        {"d/01-soup.json": {Data: []byte(`{"title": " "}`)}},
		"trailing data":   {"d/01-soup.json": {Data: []byte(`{"title": "Soup"} {}`)}},
		"not json":        {"d/01-soup.json": {Data: []byte(`title: Soup`)}},
		"stray image":     {"d/01-soup.json": recipe(""), "d/02-stew.jpg": png},
		"two images":      {"d/01-soup.json": recipe(""), "d/01-soup.jpg": png, "d/01-soup.png": png},
		"unexpected file": {"d/01-soup.json": recipe(""), "d/README.md": {Data: []byte("hi")}},
		"no directory":    {"x/01-soup.json": recipe("")},
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := load(files, "d"); err == nil {
				t.Fatalf("load = %+v, want an error", got)
			}
		})
	}
}

func TestLoadOrderAndImages(t *testing.T) {
	got, err := load(fstest.MapFS{
		"d/02-stew.json": {Data: []byte(`{"title": "Stew"}`)},
		"d/01-soup.json": {Data: []byte(`{"title": "Soup", "tags": ["Soup", "Historic"]}`)},
		"d/01-soup.png":  {Data: []byte("png")},
	}, "d")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Recipe.Title != "Soup" || string(got[0].Image) != "png" ||
		got[1].Recipe.Title != "Stew" || got[1].Image != nil {
		t.Fatalf("load = %+v", got)
	}
}

func TestSeed(t *testing.T) {
	ctx := context.Background()
	st, err := storage.Open(ctx, t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	svc := recipes.New(st, log)

	if err := Seed(ctx, svc, log); err != nil {
		t.Fatal(err)
	}
	var line struct {
		Msg   string `json:"msg"`
		Count int    `json:"count"`
	}
	if err := json.Unmarshal(logs.Bytes(), &line); err != nil || line.Msg != "samples added" || line.Count != wantCount {
		t.Fatalf("log = %s (%v)", logs.Bytes(), err)
	}
	page, err := svc.List(ctx, identity.Caller{}, recipes.ListQuery{})
	if err != nil || page.Total != wantCount {
		t.Fatalf("List = %d, %v", page.Total, err)
	}
	withCover := 0
	for _, s := range page.Items {
		if s.Cover != nil {
			withCover++
		}
	}
	all, _ := All()
	wantCovers := 0
	for _, sm := range all {
		if sm.Image != nil {
			wantCovers++
		}
	}
	if withCover != wantCovers {
		t.Errorf("%d recipes have a cover, want %d", withCover, wantCovers)
	}

	logs.Reset()
	if err := Seed(ctx, svc, log); err != nil {
		t.Fatal(err)
	}
	if logs.Len() != 0 {
		t.Errorf("second Seed logged %s", logs.Bytes())
	}
	if page, _ := svc.List(ctx, identity.Caller{}, recipes.ListQuery{}); page.Total != wantCount {
		t.Errorf("second Seed: %d recipes", page.Total)
	}
}
