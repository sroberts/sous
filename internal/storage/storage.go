// Package storage owns everything under OUTPOST_STORAGE_DIR (spec.md §3):
// the SQLite database, its migrations, and the content-addressed images.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// Store is the open storage directory. W is the single writer connection;
// R is a small pool of query_only readers.
type Store struct {
	Dir string
	W   *sql.DB // MaxOpenConns 1, _txlock=immediate
	R   *sql.DB // 4 connections, PRAGMA query_only

	log *slog.Logger
}

// TooNewError is returned by Open when the database's user_version is newer
// than this binary's migrations: a downgrade, which Sous refuses to touch.
type TooNewError struct {
	Have int // the database's user_version
	Want int // LatestVersion()
}

func (e *TooNewError) Error() string {
	return fmt.Sprintf("database schema version %d is newer than this binary supports (%d)", e.Have, e.Want)
}

// LatestVersion is the highest embedded migration number.
func LatestVersion() int {
	return 0
}

// Open prepares dir for use: creates images/ and tmp/, empties tmp/, checks
// user_version read-only (returning *TooNewError for a newer database)
// before applying any pragma, opens W and R with the DSN pragmas
// foreign_keys(1), busy_timeout(5000), journal_mode(WAL), synchronous(FULL),
// temp_store(MEMORY), and runs pending migrations, each in its own
// transaction, logging "migrated" with from and to versions.
func Open(ctx context.Context, dir string, log *slog.Logger) (*Store, error) {
	panic("TODO")
}

// Close closes R, checkpoints the WAL into sous.db with
// wal_checkpoint(TRUNCATE) on W (SQLITE_BUSY is logged as a warning, not
// returned), then closes W.
func (s *Store) Close() error {
	panic("TODO")
}

// Read runs fn in a read-only transaction on R.
func (s *Store) Read(ctx context.Context, fn func(*sql.Tx) error) error {
	panic("TODO")
}

// Write runs fn in an immediate transaction on W, committing if fn returns
// nil and rolling back otherwise.
func (s *Store) Write(ctx context.Context, fn func(*sql.Tx) error) error {
	panic("TODO")
}

// SchemaVersion is the database's user_version after Open's migrations.
func (s *Store) SchemaVersion() int {
	return 0
}

// SizeBytes is the total size of the files under Dir: the database, its WAL
// and shared memory, and the images.
func (s *Store) SizeBytes() (int64, error) {
	return 0, nil
}
