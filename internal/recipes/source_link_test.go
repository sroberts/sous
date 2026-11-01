package recipes

import (
	"database/sql"
	"testing"
)

func TestSourceMustBeWebLink(t *testing.T) {
	s := newTestService(t)
	scott := person(t, s, "Scott")
	for _, src := range []string{"javascript:alert(1)", " JavaScript:alert(1)", "data:text/html,hi", "vbscript:x", "//example.com/a", "Joy of Cooking, p. 42"} {
		if _, err := s.Create(ctx, scott, RecipeInput{Title: "x", SourceURL: sp(src)}); code(err) != CodeInvalid {
			t.Errorf("Create with source %q = %v, want %s", src, err, CodeInvalid)
		}
	}
	for _, src := range []string{"https://example.com/a", "HTTP://Example.com/a", ""} {
		if _, err := s.Create(ctx, scott, RecipeInput{Title: "x", SourceURL: sp(src)}); err != nil {
			t.Errorf("Create with source %q = %v", src, err)
		}
	}

	r := create(t, s, scott, RecipeInput{Title: "Soup"})
	if _, err := s.Update(ctx, scott, r.ID, r.Version, RecipePatch{SourceURL: Some("javascript:alert(1)")}); code(err) != CodeInvalid {
		t.Errorf("Update to a javascript: source = %v, want %s", err, CodeInvalid)
	}
	if got, _ := s.Get(ctx, scott, r.ID); got.Version != r.Version || got.SourceURL != nil {
		t.Errorf("after the refused update: version %d, source %v", got.Version, got.SourceURL)
	}

	// A source stored before links were checked, such as a book title from
	// Mela, saves back unchanged with the rest of an edit, but can't be
	// changed to another non-link.
	book := "Joy of Cooking, p. 42"
	if err := s.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE recipes SET source_url = ? WHERE id = ?`, book, r.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Update(ctx, scott, r.ID, r.Version, RecipePatch{SourceURL: Some(book), Notes: Some("salty")})
	if err != nil {
		t.Fatalf("Update keeping the stored source = %v", err)
	}
	if _, err := s.Update(ctx, scott, r.ID, got.Version, RecipePatch{SourceURL: Some("Joy of Cooking, p. 43")}); code(err) != CodeInvalid {
		t.Errorf("Update to another non-link = %v, want %s", err, CodeInvalid)
	}
	if _, err := s.Update(ctx, scott, r.ID, got.Version, RecipePatch{SourceURL: Null[string]()}); err != nil {
		t.Errorf("Update clearing the source = %v", err)
	}
}

func TestIsWebLink(t *testing.T) {
	for in, want := range map[string]bool{
		"https://example.com/a": true,
		" http://example.com ":  true,
		"HTTPS://EXAMPLE.COM":   true,
		"https://":              false,
		"javascript:alert(1)":   false,
		"java\tscript:x":        false,
		"ftp://example.com/a":   false,
		"example.com/a":         false,
		"":                      false,
	} {
		if got := IsWebLink(in); got != want {
			t.Errorf("IsWebLink(%q) = %v, want %v", in, got, want)
		}
	}
}
