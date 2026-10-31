package recipes

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/storage"
)

func testSamples(t *testing.T) []Sample {
	t.Helper()
	return []Sample{
		{Recipe: RecipeInput{Title: "Pumpkin Pie", Tags: []string{"Dessert", "Historic"},
			Ingredients: []string{"1 quart milk", "3 eggs"}, Steps: []string{"Bake."}}, Image: pngBytes(t, 4)},
		{Recipe: RecipeInput{Title: "Gazpacho", Tags: []string{"Salad", "Historic"}, YieldText: sp("Serves 4"),
			Ingredients: []string{"4 tomatoes"}, Steps: []string{"Layer."}}},
	}
}

func imageFiles(t *testing.T, s *Service) int {
	t.Helper()
	n := 0
	err := filepath.WalkDir(filepath.Join(s.st.Dir, "images"), func(_ string, d os.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			n++
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSeedSamplesFresh(t *testing.T) {
	s := newTestService(t)
	n, err := s.SeedSamples(ctx, testSamples(t))
	if err != nil || n != 2 {
		t.Fatalf("SeedSamples = %d, %v, want 2", n, err)
	}

	page, err := s.List(ctx, identity.Caller{}, ListQuery{})
	if err != nil || page.Total != 2 {
		t.Fatalf("List = %+v, %v", page, err)
	}
	for _, sum := range page.Items {
		r, err := s.Get(ctx, identity.Caller{}, sum.ID)
		if err != nil {
			t.Fatal(err)
		}
		if r.CreatedBy != identity.SystemUsername || r.UpdatedBy != identity.SystemUsername || r.Version != 1 {
			t.Errorf("%s: created_by %q updated_by %q version %d", r.Title, r.CreatedBy, r.UpdatedBy, r.Version)
		}
		switch r.Title {
		case "Pumpkin Pie":
			if len(r.Images) != 1 || len(r.Ingredients) != 2 || r.Tags[1] != "Historic" {
				t.Errorf("pie = %+v", r)
			}
			f, meta, err := s.Image(ctx, identity.Caller{}, r.Images[0])
			if err != nil || meta.ContentType != "image/png" || meta.Width == nil || *meta.Width != 4 {
				t.Errorf("pie image = %+v, %v", meta, err)
			} else {
				f.Close()
			}
		case "Gazpacho":
			if len(r.Images) != 0 || r.Servings == nil || *r.Servings != 4 {
				t.Errorf("gazpacho = %+v", r)
			}
		}
	}

	// Sous is an author, not a household member, and nobody can act as it.
	people, err := s.ListPeople(ctx, identity.Caller{})
	if err != nil || len(people) != 0 {
		t.Errorf("ListPeople = %v, %v, want none", people, err)
	}
	if p, err := s.Lookup(ctx, identity.SystemUsername); p != nil || err != nil {
		t.Errorf("Lookup(_sous) = %+v, %v, want nil", p, err)
	}
	var source string
	if err := s.st.R.QueryRow(`SELECT source FROM people WHERE username = '_sous'`).Scan(&source); err != nil || source != "system" {
		t.Errorf("_sous source = %q, %v", source, err)
	}

	changes, err := s.Changes(ctx, identity.Caller{}, ChangeQuery{Via: "system"})
	if err != nil || len(changes.Items) != 2 {
		t.Fatalf("system changes = %+v, %v", changes, err)
	}
	for _, ch := range changes.Items {
		if ch.PersonID == nil || *ch.PersonID != identity.SystemUsername || ch.Action != "create" ||
			!strings.HasPrefix(ch.Summary, "Added ") {
			t.Errorf("change = %+v", ch)
		}
	}
	if all, _ := s.Changes(ctx, identity.Caller{}, ChangeQuery{}); len(all.Items) != 2 {
		t.Errorf("changes = %d, want 2: no change for adding Sous itself", len(all.Items))
	}
	if _, err := s.Changes(ctx, identity.Caller{}, ChangeQuery{Via: "robot"}); code(err) != CodeInvalid ||
		!strings.Contains(err.Error(), "system") {
		t.Errorf("via=robot err = %v", err)
	}

	// Already seeded: nothing more, even after every sample is deleted.
	scott := person(t, s, "Scott")
	for _, sum := range page.Items {
		if err := s.Delete(ctx, scott, sum.ID, sum.Version); err != nil {
			t.Fatal(err)
		}
	}
	files := imageFiles(t, s)
	if n, err := s.SeedSamples(ctx, testSamples(t)); n != 0 || err != nil {
		t.Fatalf("second SeedSamples = %d, %v, want 0", n, err)
	}
	if got := imageFiles(t, s); got != files {
		t.Errorf("image files %d → %d on an established library", files, got)
	}
	trash, _ := s.Trash(ctx, scott, TrashQuery{})
	if live, _ := s.List(ctx, scott, ListQuery{}); live.Total != 0 || trash.Total != 2 {
		t.Errorf("live %d, trash %d, want 0 and 2", live.Total, trash.Total)
	}
}

func TestSeedSamplesEstablishedLibrary(t *testing.T) {
	t.Run("a person was added", func(t *testing.T) {
		s := newTestService(t)
		person(t, s, "Scott")
		if n, err := s.SeedSamples(ctx, testSamples(t)); n != 0 || err != nil {
			t.Fatalf("SeedSamples = %d, %v, want 0", n, err)
		}
		if got := imageFiles(t, s); got != 0 {
			t.Errorf("%d image files written", got)
		}
	})
	t.Run("a recipe with no change", func(t *testing.T) {
		// A library restored from elsewhere could hold recipes without
		// changes; it is still established.
		s := newTestService(t)
		err := s.st.Write(ctx, func(tx *sql.Tx) error {
			if _, err := tx.Exec(`INSERT INTO people VALUES ('scott', 'Scott', NULL, 'local', 't', 't')`); err != nil {
				return err
			}
			_, err := tx.Exec(`INSERT INTO recipes (id, title, deleted_at, created_at, created_by, updated_at, updated_by)
				VALUES ('r1', 'Soup', 't', 't', 'scott', 't', 'scott')`)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if n, err := s.SeedSamples(ctx, testSamples(t)); n != 0 || err != nil {
			t.Fatalf("SeedSamples = %d, %v, want 0", n, err)
		}
	})
	t.Run("only a gateway visitor", func(t *testing.T) {
		// Signing in writes no change, so the library is still new.
		s := newTestService(t)
		if err := s.UpsertGateway(ctx, identity.Person{Username: "ann", Name: "Ann"}); err != nil {
			t.Fatal(err)
		}
		if n, err := s.SeedSamples(ctx, testSamples(t)); n != 2 || err != nil {
			t.Fatalf("SeedSamples = %d, %v, want 2", n, err)
		}
		if people, _ := s.ListPeople(ctx, identity.Caller{}); len(people) != 1 || people[0].Username != "ann" {
			t.Errorf("ListPeople = %v", people)
		}
	})
}

func TestSeedSamplesRejects(t *testing.T) {
	for name, sm := range map[string]Sample{
		"blank title":  {Recipe: RecipeInput{Title: " "}},
		"bad minutes":  {Recipe: RecipeInput{Title: "X", CookMinutes: ip(-1)}},
		"image hashes": {Recipe: RecipeInput{Title: "X", Images: []string{strings.Repeat("a", 64)}}},
		"not an image": {Recipe: RecipeInput{Title: "X"}, Image: []byte("GIF89a not supported")},
		"empty image":  {Recipe: RecipeInput{Title: "X"}, Image: []byte{}},
	} {
		t.Run(name, func(t *testing.T) {
			s := newTestService(t)
			n, err := s.SeedSamples(ctx, []Sample{sm})
			if err == nil || n != 0 {
				t.Fatalf("SeedSamples = %d, %v, want an error", n, err)
			}
			if name == "not an image" && !errors.Is(err, storage.ErrUnsupportedImage) {
				t.Errorf("err = %v", err)
			}
			if page, _ := s.List(ctx, identity.Caller{}, ListQuery{}); page.Total != 0 {
				t.Errorf("%d recipes added", page.Total)
			}
		})
	}
	t.Run("none", func(t *testing.T) {
		s := newTestService(t)
		if n, err := s.SeedSamples(ctx, nil); n != 0 || err != nil {
			t.Fatalf("SeedSamples(nil) = %d, %v", n, err)
		}
		var people int
		s.st.R.QueryRow(`SELECT count(*) FROM people`).Scan(&people)
		if people != 0 {
			t.Errorf("people = %d, want 0", people)
		}
	})
}

// TestSeedSamplesUsernameTaken: a library from before the system person
// existed could hold someone else as _sous. Seeding refuses rather than
// attributing the samples to them.
func TestSeedSamplesUsernameTaken(t *testing.T) {
	s := newTestService(t)
	err := s.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO people VALUES ('_sous', 'Impostor', NULL, 'gateway', 't', 't')`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	n, err := s.SeedSamples(ctx, testSamples(t))
	if err == nil || n != 0 || !strings.Contains(err.Error(), "gateway person") {
		t.Fatalf("SeedSamples = %d, %v", n, err)
	}
	if page, _ := s.List(ctx, identity.Caller{}, ListQuery{}); page.Total != 0 {
		t.Errorf("%d recipes added", page.Total)
	}
}

func TestReservedUsername(t *testing.T) {
	s := newTestService(t)
	if _, err := s.SeedSamples(ctx, testSamples(t)); err != nil {
		t.Fatal(err)
	}
	err := s.UpsertGateway(ctx, identity.Person{Username: identity.SystemUsername, Name: "Mallory"})
	if !errors.Is(err, identity.ErrReservedUsername) {
		t.Fatalf("UpsertGateway(_sous) = %v", err)
	}
	var name, source string
	s.st.R.QueryRow(`SELECT name, source FROM people WHERE username = '_sous'`).Scan(&name, &source)
	if name != identity.SystemName || source != "system" {
		t.Errorf("_sous is now %q (%s)", name, source)
	}
	// A local person can't take the name either: its slug drops the "_".
	p, err := s.CreatePerson(ctx, identity.Caller{Via: identity.ViaAPI}, "_sous")
	if err != nil || p.Username != "sous" {
		t.Errorf("CreatePerson(_sous) = %+v, %v", p, err)
	}
}
