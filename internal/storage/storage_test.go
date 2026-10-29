package storage

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func openTest(t *testing.T, dir string) *Store {
	t.Helper()
	s, err := Open(context.Background(), dir, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s
}

func TestOpenFresh(t *testing.T) {
	dir := t.TempDir()
	s := openTest(t, dir)
	defer s.Close()

	if got := s.SchemaVersion(); got != LatestVersion() || got < 1 {
		t.Fatalf("SchemaVersion = %d, want %d", got, LatestVersion())
	}
	for _, d := range []string{"images", "tmp"} {
		info, err := os.Stat(filepath.Join(dir, d))
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o700 {
			t.Errorf("%s mode = %o, want 700", d, perm)
		}
	}
	var mode string
	if err := s.W.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		t.Errorf("journal_mode = %q, %v", mode, err)
	}
	var fk int
	if err := s.R.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		t.Errorf("reader foreign_keys = %d, %v", fk, err)
	}
	if size, err := s.SizeBytes(); err != nil || size == 0 {
		t.Errorf("SizeBytes = %d, %v", size, err)
	}
}

func TestOpenClearsTmp(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "tmp", "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tmp", "stale"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := openTest(t, dir)
	defer s.Close()
	entries, err := os.ReadDir(filepath.Join(dir, "tmp"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("tmp has %d entries, %v", len(entries), err)
	}
}

func TestReopen(t *testing.T) {
	dir := t.TempDir()
	s := openTest(t, dir)
	insertPerson(t, s, "ann")
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s = openTest(t, dir)
	defer s.Close()
	if s.SchemaVersion() != LatestVersion() {
		t.Fatalf("SchemaVersion = %d", s.SchemaVersion())
	}
	var n int
	err := s.Read(context.Background(), func(tx *sql.Tx) error {
		return tx.QueryRow("SELECT count(*) FROM people").Scan(&n)
	})
	if err != nil || n != 1 {
		t.Fatalf("people = %d, %v", n, err)
	}
}

func TestRefuseNewerDatabase(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, dbName)
	db, err := sql.Open("sqlite", file)
	if err != nil {
		t.Fatal(err)
	}
	newer := LatestVersion() + 1
	if _, err := db.Exec("CREATE TABLE future (x); PRAGMA user_version = " + strconv.Itoa(newer)); err != nil {
		t.Fatal(err)
	}
	db.Close()
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}

	_, err = Open(context.Background(), dir, slog.New(slog.NewTextHandler(io.Discard, nil)))
	var tooNew *TooNewError
	if !errors.As(err, &tooNew) {
		t.Fatalf("Open err = %v, want *TooNewError", err)
	}
	if tooNew.Have != newer || tooNew.Want != LatestVersion() {
		t.Errorf("TooNewError = %+v", tooNew)
	}

	after, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("database file changed")
	}
	if _, err := os.Stat(file + "-wal"); err == nil {
		t.Error("a -wal file was created")
	}
}

func TestFTS5Compiled(t *testing.T) {
	s := openTest(t, t.TempDir())
	defer s.Close()
	var used int
	if err := s.R.QueryRow("SELECT sqlite_compileoption_used('ENABLE_FTS5')").Scan(&used); err != nil || used != 1 {
		t.Fatalf("ENABLE_FTS5 = %d, %v", used, err)
	}
}

func TestCloseLeavesNoWAL(t *testing.T) {
	dir := t.TempDir()
	s := openTest(t, dir)
	insertPerson(t, s, "ann")
	if _, err := os.Stat(filepath.Join(dir, dbName+"-wal")); err != nil {
		t.Fatalf("expected a -wal while open: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, dbName+"-wal")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("-wal after Close: %v", err)
	}
}

func TestReaderIsQueryOnly(t *testing.T) {
	s := openTest(t, t.TempDir())
	defer s.Close()
	if _, err := s.R.Exec(`INSERT INTO people VALUES ('x','X',NULL,'local','t','t')`); err == nil {
		t.Fatal("write through R succeeded")
	}
}

func TestChangesAppendOnly(t *testing.T) {
	s := openTest(t, t.TempDir())
	defer s.Close()
	ctx := context.Background()
	insertPerson(t, s, "ann")
	err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO changes (id, at, person_id, via, entity, entity_id, action, summary)
			VALUES ('c1', '2026-01-01T00:00:00.000Z', 'ann', 'api', 'recipe', 'r1', 'create', 'Created X')`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	err = s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE changes SET summary = 'tampered'`)
		return err
	})
	if err == nil {
		t.Fatal("UPDATE on changes succeeded")
	}
	err = s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`DELETE FROM changes`)
		return err
	})
	if err == nil {
		t.Fatal("DELETE on changes succeeded")
	}
}

func TestSearchTriggers(t *testing.T) {
	s := openTest(t, t.TempDir())
	defer s.Close()
	ctx := context.Background()
	insertPerson(t, s, "ann")

	exec := func(q string, args ...any) {
		t.Helper()
		err := s.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec(q, args...)
			return err
		})
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	match := func(q string) []string {
		t.Helper()
		var ids []string
		err := s.Read(ctx, func(tx *sql.Tx) error {
			rows, err := tx.Query(`SELECT r.id FROM recipe_search JOIN recipes r ON r.seq = recipe_search.rowid
				WHERE recipe_search MATCH ? ORDER BY r.id`, q)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					return err
				}
				ids = append(ids, id)
			}
			return rows.Err()
		})
		if err != nil {
			t.Fatalf("match %q: %v", q, err)
		}
		return ids
	}
	expect := func(q string, want ...string) {
		t.Helper()
		got := match(q)
		if len(got) != len(want) {
			t.Fatalf("match %q = %v, want %v", q, got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("match %q = %v, want %v", q, got, want)
			}
		}
	}

	const now = "2026-01-01T00:00:00.000Z"
	exec(`INSERT INTO recipes (id, title, description, notes, created_at, created_by, updated_at, updated_by)
		VALUES ('r1', 'Pad Thai', 'Crème brûlée style', NULL, ?, 'ann', ?, 'ann')`, now, now)

	// Insert.
	expect("pad", "r1")
	expect("creme", "r1") // remove_diacritics

	// Update of indexed columns.
	exec(`UPDATE recipes SET title = 'Green Curry', notes = 'spicy' WHERE id = 'r1'`)
	expect("pad")
	expect("green", "r1")
	expect("notes:spicy", "r1")

	// Ingredient insert, update, delete.
	exec(`INSERT INTO recipe_ingredients (recipe_id, position, raw, item, parser_version)
		VALUES ('r1', 0, '2 cups basil', 'basil', 1), ('r1', 1, '1 lime', 'lime', 1)`)
	expect("ingredients:basil", "r1")
	expect("ingredients:lime", "r1")
	exec(`UPDATE recipe_ingredients SET item = 'coriander' WHERE recipe_id = 'r1' AND position = 0`)
	expect("ingredients:basil")
	expect("ingredients:coriander", "r1")
	exec(`DELETE FROM recipe_ingredients WHERE recipe_id = 'r1' AND position = 1`)
	expect("ingredients:lime")
	expect("ingredients:coriander", "r1")

	// Tag insert and delete.
	exec(`INSERT INTO tags (id, name, created_at, created_by) VALUES (1, 'Dinner', ?, 'ann'), (2, 'Thai', ?, 'ann')`, now, now)
	exec(`INSERT INTO recipe_tags (recipe_id, tag_id, position) VALUES ('r1', 1, 0), ('r1', 2, 1)`)
	expect("tags:dinner", "r1")
	expect("tags:thai", "r1")
	exec(`DELETE FROM recipe_tags WHERE recipe_id = 'r1' AND tag_id = 1`)
	expect("tags:dinner")
	expect("tags:thai", "r1")

	// Soft delete leaves the row indexed; hard delete removes it.
	exec(`UPDATE recipes SET deleted_at = ? WHERE id = 'r1'`, now)
	expect("green", "r1")
	exec(`DELETE FROM recipes WHERE id = 'r1'`)
	expect("green")
	var n int
	if err := s.R.QueryRow("SELECT count(*) FROM recipe_search").Scan(&n); err != nil || n != 0 {
		t.Fatalf("recipe_search rows = %d, %v", n, err)
	}
}

func TestWriteRollsBack(t *testing.T) {
	s := openTest(t, t.TempDir())
	defer s.Close()
	boom := errors.New("boom")
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO people VALUES ('x','X',NULL,'local','t','t')`); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Write err = %v", err)
	}
	var n int
	if err := s.R.QueryRow("SELECT count(*) FROM people").Scan(&n); err != nil || n != 0 {
		t.Fatalf("people = %d, %v", n, err)
	}
}

func insertPerson(t *testing.T, s *Store, username string) {
	t.Helper()
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO people (username, name, source, created_at, updated_at)
			VALUES (?, ?, 'local', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z')`, username, username)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
