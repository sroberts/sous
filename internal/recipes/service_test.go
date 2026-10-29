package recipes

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"log/slog"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/storage"
)

var ctx = context.Background()

// newTestService opens a fresh store whose clock ticks one second per read,
// so every write gets a distinct, ordered time.
func newTestService(t *testing.T) *Service {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := storage.Open(ctx, t.TempDir(), log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	s := New(st, log)
	var mu sync.Mutex
	clock := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		clock = clock.Add(time.Second)
		return clock
	}
	return s
}

// person creates a person and returns them as an API caller.
func person(t *testing.T, s *Service, name string) identity.Caller {
	t.Helper()
	p, err := s.CreatePerson(ctx, identity.Caller{Via: identity.ViaAPI}, name)
	if err != nil {
		t.Fatal(err)
	}
	return identity.Caller{Person: &identity.Person{Username: p.Username, Name: p.Name}, Route: identity.RouteDirect, Via: identity.ViaAPI}
}

func create(t *testing.T, s *Service, c identity.Caller, in RecipeInput) Recipe {
	t.Helper()
	r, err := s.Create(ctx, c, in)
	if err != nil {
		t.Fatalf("Create %q: %v", in.Title, err)
	}
	return r
}

func code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	if err != nil {
		return "internal: " + err.Error()
	}
	return ""
}

func sp(s string) *string { return &s }
func ip(n int) *int       { return &n }

func pngBytes(t *testing.T, w int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewGray(image.Rect(0, 0, w, 1))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestCreateGetRoundTrip(t *testing.T) {
	s := newTestService(t)
	scott := person(t, s, "Scott")
	img, err := s.UploadImage(ctx, scott, bytes.NewReader(pngBytes(t, 3)))
	if err != nil {
		t.Fatal(err)
	}

	in := RecipeInput{
		Title:       "  Pad Thai ",
		SourceURL:   sp("https://www.example.com/pad-thai"),
		YieldText:   sp("Serves 4 to 6"),
		PrepMinutes: ip(15),
		Tags:        []string{"Dinner", "Thai", "dinner", " ", "Noodles"},
		Images:      []string{img.Hash},
		Ingredients: []string{"# Sauce", "2 tbsp fish sauce", "", "# Noodles:", "200 g rice noodles, soaked"},
		Steps:       []string{"1. Soak the noodles.", "# Finish", "Toss everything together."},
	}
	created := create(t, s, scott, in)

	got, err := s.Get(ctx, scott, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, created) {
		t.Errorf("Get differs from Create:\n got %+v\nwant %+v", got, created)
	}
	if got.Title != "Pad Thai" || got.Version != 1 || got.DeletedAt != nil {
		t.Errorf("title, version, deleted_at = %q, %d, %v", got.Title, got.Version, got.DeletedAt)
	}
	if got.Servings == nil || *got.Servings != 4 {
		t.Errorf("servings = %v, want 4", got.Servings)
	}
	if got.SourceHost == nil || *got.SourceHost != "example.com" {
		t.Errorf("source_host = %v", got.SourceHost)
	}
	if want := []string{"Dinner", "Thai", "Noodles"}; !slices.Equal(got.Tags, want) {
		t.Errorf("tags = %q, want %q", got.Tags, want)
	}
	if !slices.Equal(got.Images, []string{img.Hash}) {
		t.Errorf("images = %q", got.Images)
	}
	if len(got.Ingredients) != 2 {
		t.Fatalf("ingredients = %+v", got.Ingredients)
	}
	if i := got.Ingredients[0]; i.Section == nil || *i.Section != "Sauce" || i.Item != "fish sauce" || i.Unit == nil || *i.Unit != "tbsp" {
		t.Errorf("ingredient 0 = %+v", i)
	}
	if i := got.Ingredients[1]; i.Section == nil || *i.Section != "Noodles" || i.Raw != "200 g rice noodles, soaked" {
		t.Errorf("ingredient 1 = %+v", i)
	}
	if len(got.Steps) != 2 || got.Steps[0].Section != nil || got.Steps[0].Text != "Soak the noodles." ||
		got.Steps[1].Section == nil || *got.Steps[1].Section != "Finish" {
		t.Errorf("steps = %+v", got.Steps)
	}
	if got.CreatedBy != "scott" || got.UpdatedBy != "scott" || got.CreatedAt != "2026-01-01T12:00:03.000Z" {
		t.Errorf("audit = %s %s %s", got.CreatedAt, got.CreatedBy, got.UpdatedBy)
	}

	if _, err := s.Get(ctx, scott, "nope"); code(err) != CodeNotFound {
		t.Errorf("Get unknown: %v", err)
	}
}

func TestCreateValidation(t *testing.T) {
	s := newTestService(t)
	scott := person(t, s, "Scott")
	for _, tc := range []struct {
		in   RecipeInput
		want string
	}{
		{RecipeInput{Title: "  "}, CodeInvalid},
		{RecipeInput{Title: "x", CookMinutes: ip(-1)}, CodeInvalid},
		{RecipeInput{Title: "x", Images: []string{"ab"}}, CodeUnknownImage},
	} {
		if _, err := s.Create(ctx, scott, tc.in); code(err) != tc.want {
			t.Errorf("Create(%+v) = %v, want %s", tc.in, err, tc.want)
		}
	}
	if _, err := s.Create(ctx, identity.Caller{}, RecipeInput{Title: "x"}); code(err) != CodeWhoAreYou {
		t.Errorf("Create from nobody = %v", err)
	}
	// The failed creates left nothing behind, including in the change log.
	page, _ := s.List(ctx, scott, ListQuery{})
	changes, _ := s.Changes(ctx, scott, ChangeQuery{Via: "api"})
	if page.Total != 0 || len(changes.Items) != 1 {
		t.Errorf("after failures: %d recipes, %d changes", page.Total, len(changes.Items))
	}
}

func TestUpdate(t *testing.T) {
	s := newTestService(t)
	scott := person(t, s, "Scott")
	r := create(t, s, scott, RecipeInput{
		Title: "Soup", Notes: sp("salty"), YieldText: sp("4 bowls"),
		Ingredients: []string{"1 onion", "2 carrots"}, Steps: []string{"Chop."},
	})

	got, err := s.Update(ctx, scott, r.ID, 1, RecipePatch{
		Ingredients: Some([]string{"# Base", "3 leeks"}),
		Notes:       Null[string](),
		YieldText:   Null[string](),
		Steps:       Some([]string{"Chop."}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 2 || got.Notes != nil || got.YieldText != nil || got.Servings != nil {
		t.Errorf("version %d, notes %v, yield %v, servings %v", got.Version, got.Notes, got.YieldText, got.Servings)
	}
	if len(got.Ingredients) != 1 || got.Ingredients[0].Item != "leeks" || *got.Ingredients[0].Section != "Base" {
		t.Errorf("ingredients = %+v", got.Ingredients)
	}
	if got.Title != "Soup" || len(got.Steps) != 1 {
		t.Errorf("untouched fields changed: %+v", got)
	}

	ch, err := s.Changes(ctx, scott, ChangeQuery{EntityID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(ch.Items) != 2 || ch.Items[0].Summary != "Edited Soup: notes, yield, ingredients" || ch.Items[0].Action != "update" {
		t.Errorf("changes = %+v", ch.Items)
	}

	for _, p := range []RecipePatch{
		{Title: Null[string]()},
		{Title: Some(" ")},
		{PrepMinutes: Some(-5)},
	} {
		if _, err := s.Update(ctx, scott, r.ID, 2, p); code(err) != CodeInvalid {
			t.Errorf("Update(%+v) = %v, want invalid", p, err)
		}
	}
	if _, err := s.Update(ctx, scott, r.ID, 2, RecipePatch{Images: Some([]string{"missing"})}); code(err) != CodeUnknownImage {
		t.Errorf("unknown image: %v", err)
	}
	// The rejected update rolled back its version bump and change entry.
	again, _ := s.Get(ctx, scott, r.ID)
	ch, _ = s.Changes(ctx, scott, ChangeQuery{EntityID: r.ID})
	if again.Version != 2 || len(ch.Items) != 2 {
		t.Errorf("after rollback: version %d, %d changes", again.Version, len(ch.Items))
	}
}

func TestConcurrency(t *testing.T) {
	s := newTestService(t)
	scott := person(t, s, "Scott")
	r := create(t, s, scott, RecipeInput{Title: "Stew"})

	if _, err := s.Update(ctx, scott, r.ID, 1, RecipePatch{Title: Some("Beef Stew")}); err != nil {
		t.Fatal(err)
	}
	_, err := s.Update(ctx, scott, r.ID, 1, RecipePatch{Title: Some("Lamb Stew")})
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeStale || e.Current == nil {
		t.Fatalf("stale update = %v", err)
	}
	if e.Current.Version != 2 || e.Current.Title != "Beef Stew" {
		t.Errorf("current = v%d %q", e.Current.Version, e.Current.Title)
	}
	if err := s.Delete(ctx, scott, r.ID, 1); code(err) != CodeStale {
		t.Errorf("stale delete = %v", err)
	}
	if _, err := s.Update(ctx, scott, "missing", 1, RecipePatch{}); code(err) != CodeNotFound {
		t.Errorf("update missing = %v", err)
	}

	if err := s.Delete(ctx, scott, r.ID, 2); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, scott, r.ID)
	if err != nil || got.DeletedAt == nil || got.Version != 3 {
		t.Fatalf("Get deleted = %+v, %v", got, err)
	}
	if _, err := s.Update(ctx, scott, r.ID, 3, RecipePatch{}); code(err) != CodeDeleted {
		t.Errorf("update deleted = %v", err)
	}
	if err := s.Delete(ctx, scott, r.ID, 3); code(err) != CodeDeleted {
		t.Errorf("delete deleted = %v", err)
	}

	restored, err := s.Restore(ctx, scott, r.ID)
	if err != nil || restored.DeletedAt != nil || restored.Version != 4 {
		t.Fatalf("Restore = %+v, %v", restored, err)
	}
	noop, err := s.Restore(ctx, scott, r.ID)
	if err != nil || noop.Version != 4 {
		t.Errorf("second Restore = v%d, %v", noop.Version, err)
	}
	if _, err := s.Restore(ctx, scott, "missing"); code(err) != CodeNotFound {
		t.Errorf("restore missing = %v", err)
	}

	ch, _ := s.Changes(ctx, scott, ChangeQuery{EntityID: r.ID})
	var actions []string
	for _, c := range ch.Items {
		actions = append(actions, c.Action+": "+c.Summary)
	}
	want := []string{"restore: Restored Beef Stew", "delete: Deleted Beef Stew", "update: Edited Beef Stew: title", "create: Added Stew"}
	if !slices.Equal(actions, want) {
		t.Errorf("changes = %q, want %q", actions, want)
	}
}

// Parallel writers at the same version: exactly one wins, the rest are
// stale, and the version moves by exactly one.
func TestConcurrentUpdates(t *testing.T) {
	s := newTestService(t)
	scott := person(t, s, "Scott")
	r := create(t, s, scott, RecipeInput{Title: "Race"})
	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Go(func() {
			_, errs[i] = s.Update(ctx, scott, r.ID, 1, RecipePatch{Notes: Some(fmt.Sprint(i))})
		})
	}
	wg.Wait()
	wins := 0
	for _, err := range errs {
		switch code(err) {
		case "":
			wins++
		case CodeStale:
		default:
			t.Errorf("unexpected: %v", err)
		}
	}
	got, _ := s.Get(ctx, scott, r.ID)
	if wins != 1 || got.Version != 2 {
		t.Errorf("wins = %d, version = %d", wins, got.Version)
	}
}

func TestChangeLog(t *testing.T) {
	s := newTestService(t)
	scott := person(t, s, "Scott")
	mcp := scott
	mcp.Via = identity.ViaMCP
	r := create(t, s, mcp, RecipeInput{Title: "Pad Thai"})

	ch, err := s.Changes(ctx, scott, ChangeQuery{Via: "mcp"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ch.Items) != 1 {
		t.Fatalf("mcp changes = %+v", ch.Items)
	}
	c := ch.Items[0]
	if c.PersonID == nil || *c.PersonID != "scott" || c.Via != "mcp" || c.Entity != "recipe" ||
		c.EntityID != r.ID || c.Action != "create" || c.Summary != "Added Pad Thai" || c.At != r.CreatedAt {
		t.Errorf("change = %+v", c)
	}

	all, _ := s.Changes(ctx, scott, ChangeQuery{})
	if len(all.Items) != 2 || all.Items[1].Summary != "Added person Scott" || all.Items[1].PersonID != nil {
		t.Errorf("all changes = %+v", all.Items)
	}
	if byPerson, _ := s.Changes(ctx, scott, ChangeQuery{Person: "scott"}); len(byPerson.Items) != 1 {
		t.Errorf("person filter = %+v", byPerson.Items)
	}

	for range 4 {
		create(t, s, scott, RecipeInput{Title: "More"})
	}
	var seen []string
	cur := ""
	for {
		p, err := s.Changes(ctx, scott, ChangeQuery{Cursor: cur, Limit: 2})
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range p.Items {
			seen = append(seen, c.ID)
		}
		if p.NextCursor == nil {
			break
		}
		cur = *p.NextCursor
	}
	if len(seen) != 6 || len(slices.Compact(slices.Clone(seen))) != 6 {
		t.Errorf("paged changes = %d", len(seen))
	}
	if _, err := s.Changes(ctx, scott, ChangeQuery{Via: "fax"}); code(err) != CodeInvalid {
		t.Errorf("bad via = %v", err)
	}
}

func TestSetMine(t *testing.T) {
	s := newTestService(t)
	scott := person(t, s, "Scott")
	r := create(t, s, scott, RecipeInput{Title: "Cake"})
	yes, no := true, false

	m, err := s.SetMine(ctx, scott, r.ID, MinePatch{Favorite: &yes})
	if err != nil || m != (Mine{Favorite: true}) {
		t.Fatalf("SetMine = %+v, %v", m, err)
	}
	m, _ = s.SetMine(ctx, scott, r.ID, MinePatch{WantToCook: &yes})
	if m != (Mine{true, true}) {
		t.Errorf("merge = %+v", m)
	}
	m, _ = s.SetMine(ctx, scott, r.ID, MinePatch{Favorite: &no})
	if m != (Mine{false, true}) {
		t.Errorf("unset = %+v", m)
	}
	got, _ := s.Get(ctx, scott, r.ID)
	if got.Mine != m {
		t.Errorf("Get mine = %+v", got.Mine)
	}
	if nobody, _ := s.Get(ctx, identity.Caller{}, r.ID); nobody.Mine != (Mine{}) {
		t.Errorf("nobody's mine = %+v", nobody.Mine)
	}
	if _, err := s.SetMine(ctx, scott, "missing", MinePatch{}); code(err) != CodeNotFound {
		t.Errorf("missing = %v", err)
	}
}

func TestSummaryMinutes(t *testing.T) {
	for _, tc := range []struct {
		total, prep, cook *int
		want              *int
	}{
		{nil, nil, nil, nil},
		{ip(90), ip(10), ip(20), ip(90)},
		{ip(0), ip(10), nil, ip(0)},
		{nil, ip(10), ip(20), ip(30)},
		{nil, ip(10), nil, ip(10)},
		{nil, nil, ip(25), ip(25)},
		{nil, ip(0), ip(0), nil},
		{nil, ip(0), nil, nil},
	} {
		got := summaryMinutes(tc.total, tc.prep, tc.cook)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("summaryMinutes(%v, %v, %v) = %v, want %v", tc.total, tc.prep, tc.cook, got, tc.want)
		}
	}
}

func TestPeople(t *testing.T) {
	s := newTestService(t)
	p, err := s.CreatePerson(ctx, identity.Caller{}, "  Mary-Jane O'Brien ")
	if err != nil || p != (Person{Username: "mary-jane-o-brien", Name: "Mary-Jane O'Brien"}) {
		t.Fatalf("CreatePerson = %+v, %v", p, err)
	}
	if _, err := s.CreatePerson(ctx, identity.Caller{}, "MARY-JANE o'brien"); code(err) != CodeExists {
		t.Errorf("duplicate = %v", err)
	}
	for _, name := range []string{"", "   ", "!!!", "--"} {
		if _, err := s.CreatePerson(ctx, identity.Caller{}, name); code(err) != CodeInvalid {
			t.Errorf("CreatePerson(%q) = %v", name, err)
		}
	}
	if got := slug("Ánna  Smith!"); got != "nna-smith" {
		t.Errorf("slug = %q", got)
	}

	found, err := s.Lookup(ctx, "mary-jane-o-brien")
	if err != nil || found == nil || found.Name != "Mary-Jane O'Brien" {
		t.Errorf("Lookup = %+v, %v", found, err)
	}
	if missing, err := s.Lookup(ctx, "nobody"); missing != nil || err != nil {
		t.Errorf("Lookup missing = %+v, %v", missing, err)
	}
	s.CreatePerson(ctx, identity.Caller{}, "Alice")
	people, _ := s.ListPeople(ctx, identity.Caller{})
	if len(people) != 2 || people[0].Username != "alice" {
		t.Errorf("ListPeople = %+v", people)
	}
}

func TestUpsertGateway(t *testing.T) {
	s := newTestService(t)
	if err := s.UpsertGateway(ctx, identity.Person{Username: "scott", Name: "Scott", Email: "s@example.com"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertGateway(ctx, identity.Person{Username: "scott", Name: "Scott Roberts", Email: "s@example.com"}); err != nil {
		t.Fatal(err)
	}
	p, err := s.Lookup(ctx, "scott")
	if err != nil || p == nil || p.Name != "Scott Roberts" || p.Email != "s@example.com" {
		t.Fatalf("Lookup = %+v, %v", p, err)
	}
	var source, updated string
	s.st.R.QueryRow(`SELECT source, updated_at FROM people WHERE username = 'scott'`).Scan(&source, &updated)
	if source != "gateway" {
		t.Errorf("source = %q", source)
	}
	// An unchanged identity doesn't rewrite the row.
	s.UpsertGateway(ctx, identity.Person{Username: "scott", Name: "Scott Roberts", Email: "s@example.com"})
	var again string
	s.st.R.QueryRow(`SELECT updated_at FROM people WHERE username = 'scott'`).Scan(&again)
	if again != updated {
		t.Errorf("updated_at moved: %s → %s", updated, again)
	}
	// A local person seen through the gateway becomes a gateway person.
	s.CreatePerson(ctx, identity.Caller{}, "Alice")
	s.UpsertGateway(ctx, identity.Person{Username: "alice", Name: "Alice"})
	s.st.R.QueryRow(`SELECT source FROM people WHERE username = 'alice'`).Scan(&source)
	if source != "gateway" {
		t.Errorf("alice source = %q", source)
	}
}

func TestTags(t *testing.T) {
	s := newTestService(t)
	scott := person(t, s, "Scott")
	create(t, s, scott, RecipeInput{Title: "A", Tags: []string{"Dinner", "Quick"}})
	create(t, s, scott, RecipeInput{Title: "B", Tags: []string{"dinner", "Vegan"}})
	c := create(t, s, scott, RecipeInput{Title: "C", Tags: []string{"Vegan", "Quick", "Baking"}})
	d := create(t, s, scott, RecipeInput{Title: "D", Tags: []string{"Baking"}})
	if err := s.Delete(ctx, scott, c.ID, c.Version); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, scott, d.ID, d.Version); err != nil {
		t.Fatal(err)
	}
	got, err := s.Tags(ctx, scott)
	if err != nil {
		t.Fatal(err)
	}
	want := []TagCount{{"Dinner", 2}, {"Quick", 1}, {"Vegan", 1}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Tags = %+v, want %+v", got, want)
	}
}

func TestImages(t *testing.T) {
	s := newTestService(t)
	scott := person(t, s, "Scott")
	b := pngBytes(t, 5)
	ref, err := s.UploadImage(ctx, scott, bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if again, err := s.UploadImage(ctx, scott, bytes.NewReader(b)); err != nil || again != ref {
		t.Errorf("re-upload = %v, %v", again, err)
	}
	f, meta, err := s.Image(ctx, scott, ref.Hash)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if meta.ContentType != "image/png" || meta.Width == nil || *meta.Width != 5 || meta.Bytes != int64(len(b)) {
		t.Errorf("meta = %+v", meta)
	}
	if _, err := s.UploadImage(ctx, scott, bytes.NewReader([]byte("not an image"))); code(err) != CodeUnsupportedMediaType {
		t.Errorf("text upload = %v", err)
	}
	if _, err := s.UploadImage(ctx, identity.Caller{}, bytes.NewReader(b)); code(err) != CodeWhoAreYou {
		t.Errorf("nobody upload = %v", err)
	}
	if _, _, err := s.Image(ctx, scott, "00"); code(err) != CodeNotFound {
		t.Errorf("missing image = %v", err)
	}
}

func TestStatus(t *testing.T) {
	s := newTestService(t)
	scott := person(t, s, "Scott")
	create(t, s, scott, RecipeInput{Title: "A"})
	b := create(t, s, scott, RecipeInput{Title: "B"})
	s.Delete(ctx, scott, b.ID, 1)
	st, err := s.Status(ctx, scott)
	if err != nil || st.Recipes != 1 || st.SchemaVersion != storage.LatestVersion() || st.StorageBytes <= 0 {
		t.Errorf("Status = %+v, %v", st, err)
	}
}

func TestSchemaFor(t *testing.T) {
	for _, s := range []any{SchemaFor[RecipePatch](), SchemaFor[Recipe](), SchemaFor[Items[ParsedLine]]()} {
		if s == nil {
			t.Fatal("nil schema")
		}
	}
	p := SchemaFor[RecipePatch]()
	if got := p.Properties["notes"].Types; !slices.Equal(got, []string{"null", "string"}) {
		t.Errorf("notes types = %v", got)
	}
	if got := p.Properties["tags"]; got.Items == nil || got.Items.Type != "string" {
		t.Errorf("tags schema = %+v", got)
	}
	if len(p.Required) != 0 {
		t.Errorf("patch required = %v", p.Required)
	}
}
