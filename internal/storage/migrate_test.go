package storage

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func openAt(t *testing.T, dir string, ms []migration) *Store {
	t.Helper()
	s, err := open(context.Background(), dir, slog.New(slog.NewTextHandler(io.Discard, nil)), ms)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return s
}

func exec(t *testing.T, s *Store, q string, args ...any) {
	t.Helper()
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(q, args...)
		return err
	})
	if err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

// dump returns every row of every ordinary table, as text, keyed by table.
func dump(t *testing.T, s *Store) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for _, table := range []string{"people", "recipes", "recipe_ingredients", "recipe_steps", "images",
		"recipe_images", "tags", "recipe_tags", "person_recipe", "changes"} {
		rows, err := s.R.Query("SELECT * FROM " + table)
		if err != nil {
			t.Fatal(err)
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			var b strings.Builder
			for i, v := range vals {
				if i > 0 {
					b.WriteString("|")
				}
				if raw, ok := v.([]byte); ok {
					v = string(raw)
				}
				fmt.Fprintf(&b, "%#v", v)
			}
			out[table] = append(out[table], b.String())
		}
		rows.Close()
		slices.Sort(out[table])
	}
	return out
}

func schemaSQL(t *testing.T, s *Store, name string) string {
	t.Helper()
	var q string
	if err := s.R.QueryRow("SELECT sql FROM sqlite_master WHERE name = ?", name).Scan(&q); err != nil {
		t.Fatalf("schema of %s: %v", name, err)
	}
	return q
}

// TestMigrateV1ToV2 fills a version 1 library with a row in every table,
// then opens it with every migration and checks that nothing was lost or
// renumbered, that tables pointing at people and changes still point at
// them (SQLite rewrites references on RENAME, which the rebuild must not
// trip), and that the triggers and foreign keys still work.
func TestMigrateV1ToV2(t *testing.T) {
	dir := t.TempDir()
	s := openAt(t, dir, migrations[:1])
	if s.SchemaVersion() != 1 {
		t.Fatalf("SchemaVersion = %d, want 1", s.SchemaVersion())
	}
	const now = "2026-01-01T00:00:00.000Z"
	hash := strings.Repeat("a", 64)
	exec(t, s, `INSERT INTO people VALUES ('ann','Ann','ann@example.com','gateway',?,?), ('bob','Bob',NULL,'local',?,?)`, now, now, now, now)
	exec(t, s, `INSERT INTO recipes (id, title, description, created_at, created_by, updated_at, updated_by)
		VALUES ('r1', 'Pad Thai', 'Crème brûlée style', ?, 'ann', ?, 'bob'), ('r2', 'Gone', NULL, ?, 'bob', ?, 'bob')`, now, now, now, now)
	exec(t, s, `UPDATE recipes SET deleted_at = ? WHERE id = 'r2'`, now)
	exec(t, s, `INSERT INTO recipe_ingredients (recipe_id, position, raw, quantity, item, parser_version)
		VALUES ('r1', 0, '2 cups basil', 2, 'basil', 1)`)
	exec(t, s, `INSERT INTO recipe_steps (recipe_id, position, text) VALUES ('r1', 0, 'Cook.')`)
	exec(t, s, `INSERT INTO images VALUES (?, 'image/png', 1, 1, 70, ?, 'ann')`, hash, now)
	exec(t, s, `INSERT INTO recipe_images VALUES ('r1', 0, ?)`, hash)
	exec(t, s, `INSERT INTO tags (id, name, created_at, created_by) VALUES (7, 'Dinner', ?, 'ann')`, now)
	exec(t, s, `INSERT INTO recipe_tags VALUES ('r1', 7, 0)`)
	exec(t, s, `INSERT INTO person_recipe VALUES ('bob', 'r1', 1, 0, ?)`, now)
	// Gaps in seq must survive: the change log's cursors are seqs.
	exec(t, s, `INSERT INTO changes (seq, id, at, person_id, via, entity, entity_id, action, summary) VALUES
		(3, 'c3', ?, 'ann', 'ui', 'recipe', 'r1', 'create', 'Added Pad Thai'),
		(9, 'c9', ?, NULL, 'mcp', 'person', 'bob', 'create', 'Added person Bob'),
		(12, 'c12', ?, 'bob', 'api', 'recipe', 'r2', 'delete', 'Deleted Gone')`, now, now, now)
	before := dump(t, s)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s = openTest(t, dir)
	defer s.Close()
	if s.SchemaVersion() != 2 || LatestVersion() != 2 {
		t.Fatalf("SchemaVersion = %d, LatestVersion = %d, want 2", s.SchemaVersion(), LatestVersion())
	}
	after := dump(t, s)
	for table, rows := range before {
		if !slices.Equal(rows, after[table]) {
			t.Errorf("%s:\n before %q\n after  %q", table, rows, after[table])
		}
	}

	// References into the rebuilt tables are unchanged.
	for _, table := range []string{"recipes", "images", "tags", "person_recipe", "changes"} {
		q := schemaSQL(t, s, table)
		if !strings.Contains(q, "REFERENCES people(username)") || strings.Contains(q, "people_new") {
			t.Errorf("%s no longer references people: %s", table, q)
		}
	}
	for _, name := range []string{"people", "changes"} {
		if q := schemaSQL(t, s, name); !strings.Contains(q, "'system'") || strings.Contains(q, "_new") {
			t.Errorf("%s schema: %s", name, q)
		}
	}
	var leftovers int
	if err := s.R.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name LIKE '%_new'`).Scan(&leftovers); err != nil || leftovers != 0 {
		t.Errorf("leftover _new objects: %d, %v", leftovers, err)
	}
	var indexes, triggers []string
	rows, err := s.R.Query(`SELECT type, name FROM sqlite_master WHERE tbl_name = 'changes' AND type IN ('index', 'trigger') AND sql IS NOT NULL ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var typ, name string
		rows.Scan(&typ, &name)
		if typ == "index" {
			indexes = append(indexes, name)
		} else {
			triggers = append(triggers, name)
		}
	}
	rows.Close()
	if want := []string{"changes_by_entity", "changes_by_person", "changes_by_via"}; !slices.Equal(indexes, want) {
		t.Errorf("changes indexes = %v, want %v", indexes, want)
	}
	if want := []string{"changes_no_delete", "changes_no_update"}; !slices.Equal(triggers, want) {
		t.Errorf("changes triggers = %v, want %v", triggers, want)
	}

	// The writer is back to enforcing foreign keys.
	var fk int
	if err := s.W.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		t.Fatalf("writer foreign_keys = %d, %v", fk, err)
	}
	ctx := context.Background()
	write := func(q string, args ...any) error {
		return s.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec(q, args...)
			return err
		})
	}
	if err := write(`INSERT INTO recipes (id, title, created_at, created_by, updated_at, updated_by)
		VALUES ('r3', 'X', ?, 'nobody', ?, 'nobody')`, now, now); err == nil {
		t.Error("recipe by an unknown person was accepted")
	}
	if err := write(`DELETE FROM people WHERE username = 'ann'`); err == nil {
		t.Error("deleting a referenced person was accepted")
	}
	if err := write(`INSERT INTO changes (id, at, person_id, via, entity, entity_id, action, summary)
		VALUES ('cx', ?, 'nobody', 'api', 'recipe', 'r1', 'update', 'x')`, now); err == nil {
		t.Error("change by an unknown person was accepted")
	}
	if err := write(`UPDATE changes SET summary = 'tampered'`); err == nil {
		t.Error("UPDATE on changes succeeded")
	}
	if err := write(`DELETE FROM changes`); err == nil {
		t.Error("DELETE on changes succeeded")
	}

	// The new values are allowed, and new changes continue after the last seq.
	if err := write(`INSERT INTO people VALUES ('_sous', 'Sous', NULL, 'system', ?, ?)`, now, now); err != nil {
		t.Fatalf("system person: %v", err)
	}
	if err := write(`INSERT INTO changes (id, at, person_id, via, entity, entity_id, action, summary)
		VALUES ('c13', ?, '_sous', 'system', 'recipe', 'r1', 'create', 'Added X')`, now); err != nil {
		t.Fatalf("system change: %v", err)
	}
	var seq int
	if err := s.R.QueryRow(`SELECT seq FROM changes WHERE id = 'c13'`).Scan(&seq); err != nil || seq != 13 {
		t.Errorf("new change seq = %d, %v, want 13", seq, err)
	}
	if err := write(`INSERT INTO people VALUES ('x', 'X', NULL, 'robot', ?, ?)`, now, now); err == nil {
		t.Error("people.source accepted 'robot'")
	}

	// Search still finds the recipe and still follows edits.
	match := func(q string) int {
		var n int
		if err := s.R.QueryRow(`SELECT count(*) FROM recipe_search WHERE recipe_search MATCH ?`, q).Scan(&n); err != nil {
			t.Fatalf("match %q: %v", q, err)
		}
		return n
	}
	if match("creme") != 1 || match("ingredients:basil") != 1 || match("tags:dinner") != 1 {
		t.Error("search lost the migrated recipe")
	}
	if err := write(`UPDATE recipes SET title = 'Green Curry' WHERE id = 'r1'`); err != nil {
		t.Fatal(err)
	}
	if match("green") != 1 || match("pad") != 0 {
		t.Error("search triggers stopped following edits")
	}
}

// TestMigrationFailureRollsBack: a migration that breaks a foreign key, or
// fails outright, leaves the database at its old version with its rows,
// and the writer still enforcing foreign keys.
func TestMigrationFailureRollsBack(t *testing.T) {
	for name, bad := range map[string]string{
		"foreign key": `DROP TABLE people;
			CREATE TABLE people (username TEXT PRIMARY KEY, name TEXT NOT NULL, email TEXT,
				source TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL) WITHOUT ROWID;`,
		"error": `CREATE TABLE extra (x); SELECT no_such_function();`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			s := openTest(t, dir)
			insertPerson(t, s, "ann")
			exec(t, s, `INSERT INTO recipes (id, title, created_at, created_by, updated_at, updated_by)
				VALUES ('r1', 'Soup', 't', 'ann', 't', 'ann')`)
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}

			ms := append(slices.Clone(migrations), migration{version: len(migrations) + 1, name: "bad.sql", sql: bad})
			_, err := open(context.Background(), dir, slog.New(slog.NewTextHandler(io.Discard, nil)), ms)
			if err == nil || !strings.Contains(err.Error(), "bad.sql") {
				t.Fatalf("open err = %v, want the bad migration named", err)
			}
			if name == "foreign key" && !strings.Contains(err.Error(), "foreign key") {
				t.Errorf("err = %v, want a foreign key failure", err)
			}

			s = openTest(t, dir)
			defer s.Close()
			if s.SchemaVersion() != LatestVersion() {
				t.Fatalf("SchemaVersion = %d, want %d", s.SchemaVersion(), LatestVersion())
			}
			var people, extra int
			s.R.QueryRow(`SELECT count(*) FROM people`).Scan(&people)
			s.R.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'extra'`).Scan(&extra)
			if people != 1 || extra != 0 {
				t.Errorf("people = %d, extra tables = %d; the failed migration was not rolled back", people, extra)
			}
			if q := schemaSQL(t, s, "people"); !strings.Contains(q, "CHECK") {
				t.Errorf("people was replaced: %s", q)
			}
		})
	}
}

// TestMigrateKeepsForeignKeysOnWhenCanceled: the pinned connection gets
// foreign keys back even when the context is already done.
func TestMigrateKeepsForeignKeysOnWhenCanceled(t *testing.T) {
	dir := t.TempDir()
	s := openAt(t, dir, migrations[:1])
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openAt(t, dir, migrations[:1])
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.migrate(ctx, migrations); err == nil {
		t.Fatal("migrate with a canceled context succeeded")
	}
	var fk, v int
	if err := s.W.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		t.Errorf("writer foreign_keys = %d, %v", fk, err)
	}
	if err := s.W.QueryRow("PRAGMA user_version").Scan(&v); err != nil || v != 1 {
		t.Errorf("user_version = %d, %v, want 1", v, err)
	}
}

// TestOpenRefusesNewerThanItsMigrations covers the second, under-the-writer
// version check through open with fewer migrations than the file has.
func TestOpenRefusesNewerThanItsMigrations(t *testing.T) {
	dir := t.TempDir()
	s := openTest(t, dir)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	_, err := open(context.Background(), dir, slog.New(slog.NewTextHandler(io.Discard, nil)), migrations[:1])
	if tooNew, ok := err.(*TooNewError); !ok || tooNew.Have != LatestVersion() || tooNew.Want != 1 {
		t.Fatalf("open err = %v, want TooNewError{%d, 1}", err, LatestVersion())
	}
	if _, err := os.Stat(filepath.Join(dir, dbName)); err != nil {
		t.Fatal(err)
	}
}

// TestMigrateWithDanglingRow: a library that already has a row pointing
// nowhere, say from a restore, still migrates, and the row is kept; a
// migration that adds another such row is still rolled back.
func TestMigrateWithDanglingRow(t *testing.T) {
	setup := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		s := openAt(t, dir, migrations[:1])
		ctx := context.Background()
		conn, err := s.W.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, q := range []string{
			"PRAGMA foreign_keys = OFF",
			`INSERT INTO recipe_steps (recipe_id, position, text) VALUES ('ghost', 0, 'Haunt.')`,
			"PRAGMA foreign_keys = ON",
		} {
			if _, err := conn.ExecContext(ctx, q); err != nil {
				t.Fatalf("%s: %v", q, err)
			}
		}
		conn.Close()
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	steps := func(t *testing.T, s *Store) int {
		t.Helper()
		var n int
		if err := s.R.QueryRow(`SELECT count(*) FROM recipe_steps`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	t.Run("migrates", func(t *testing.T) {
		s := openTest(t, setup(t))
		defer s.Close()
		if s.SchemaVersion() != LatestVersion() {
			t.Fatalf("SchemaVersion = %d, want %d", s.SchemaVersion(), LatestVersion())
		}
		if n := steps(t, s); n != 1 {
			t.Errorf("recipe_steps = %d, want the dangling row kept", n)
		}
	})
	t.Run("a new dangling row still rolls back", func(t *testing.T) {
		dir := setup(t)
		// The same table and parent as the existing one, so only the count
		// tells them apart.
		ms := append(slices.Clone(migrations), migration{version: len(migrations) + 1, name: "bad.sql",
			sql: `INSERT INTO recipe_steps (recipe_id, position, text) VALUES ('ghost2', 0, 'Boo.')`})
		_, err := open(context.Background(), dir, slog.New(slog.NewTextHandler(io.Discard, nil)), ms)
		if err == nil || !strings.Contains(err.Error(), "bad.sql") || !strings.Contains(err.Error(), "foreign key") {
			t.Fatalf("open err = %v, want a foreign key failure in bad.sql", err)
		}
		s := openTest(t, dir)
		defer s.Close()
		if s.SchemaVersion() != LatestVersion() {
			t.Fatalf("SchemaVersion = %d, want %d", s.SchemaVersion(), LatestVersion())
		}
		if n := steps(t, s); n != 1 {
			t.Errorf("recipe_steps = %d, want 1: the bad migration was not rolled back", n)
		}
	})
}
