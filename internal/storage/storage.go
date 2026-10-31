// Package storage owns everything under OUTPOST_STORAGE_DIR (spec.md §3):
// the SQLite database, its migrations, and the content-addressed images.
package storage

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"modernc.org/sqlite" // also registers the "sqlite" driver
	sqlite3 "modernc.org/sqlite/lib"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

const dbName = "sous.db"

// Store is the open storage directory. W is the single writer connection;
// R is a small pool of query_only readers.
type Store struct {
	Dir string
	W   *sql.DB // MaxOpenConns 1, _txlock=immediate
	R   *sql.DB // 4 connections, PRAGMA query_only

	log     *slog.Logger
	version int
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

type migration struct {
	version int
	name    string
	sql     string
}

var migrations = mustLoadMigrations()

func mustLoadMigrations() []migration {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		panic(err)
	}
	var ms []migration
	for _, e := range entries {
		num, _, ok := strings.Cut(e.Name(), "_")
		n, err := strconv.Atoi(num)
		if !ok || err != nil || n <= 0 {
			panic("storage: bad migration name " + e.Name())
		}
		b, err := migrationFS.ReadFile(path.Join("migrations", e.Name()))
		if err != nil {
			panic(err)
		}
		ms = append(ms, migration{version: n, name: e.Name(), sql: string(b)})
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].version < ms[j].version })
	for i, m := range ms {
		if m.version != i+1 {
			panic(fmt.Sprintf("storage: migration %s out of sequence", m.name))
		}
	}
	return ms
}

// LatestVersion is the highest embedded migration number.
func LatestVersion() int {
	return len(migrations)
}

func dsn(file string, params ...string) string {
	u := "file:" + (&url.URL{Path: file}).EscapedPath()
	if len(params) > 0 {
		u += "?" + strings.Join(params, "&")
	}
	return u
}

var commonPragmas = []string{
	"_pragma=foreign_keys(1)",
	"_pragma=busy_timeout(5000)",
	"_pragma=synchronous(FULL)",
	"_pragma=temp_store(MEMORY)",
}

// Open prepares dir for use: creates images/ and tmp/, empties tmp/, checks
// user_version read-only (returning *TooNewError for a newer database)
// before applying any pragma, opens W and R with the DSN pragmas
// foreign_keys(1), busy_timeout(5000), journal_mode(WAL), synchronous(FULL),
// temp_store(MEMORY), and runs pending migrations, each in its own
// transaction, logging "migrated" with from and to versions.
func Open(ctx context.Context, dir string, log *slog.Logger) (*Store, error) {
	return open(ctx, dir, log, migrations)
}

// open is Open with the migrations to apply, so tests can stop at an older
// schema or add a migration that fails.
func open(ctx context.Context, dir string, log *slog.Logger, ms []migration) (*Store, error) {
	if log == nil {
		log = slog.Default()
	}
	for _, d := range []string{"images", "tmp"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o700); err != nil {
			return nil, err
		}
	}
	if err := clearDir(filepath.Join(dir, "tmp")); err != nil {
		return nil, fmt.Errorf("clear tmp: %w", err)
	}

	file := filepath.Join(dir, dbName)
	have, err := peekVersion(ctx, file)
	if err != nil {
		return nil, err
	}
	if have > len(ms) {
		return nil, &TooNewError{Have: have, Want: len(ms)}
	}

	s := &Store{Dir: dir, log: log}
	wparams := append([]string{"_pragma=journal_mode(WAL)", "_txlock=immediate"}, commonPragmas...)
	s.W, err = sql.Open("sqlite", dsn(file, wparams...))
	if err != nil {
		return nil, err
	}
	s.W.SetMaxOpenConns(1)
	s.W.SetMaxIdleConns(1)
	s.W.SetConnMaxLifetime(0)
	s.W.SetConnMaxIdleTime(0)
	if err := s.W.PingContext(ctx); err != nil {
		s.W.Close()
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := s.migrate(ctx, ms); err != nil {
		s.W.Close()
		return nil, err
	}

	rparams := append([]string{"_pragma=query_only(1)"}, commonPragmas...)
	s.R, err = sql.Open("sqlite", dsn(file, rparams...))
	if err != nil {
		s.W.Close()
		return nil, err
	}
	s.R.SetMaxOpenConns(4)
	s.R.SetMaxIdleConns(4)
	if err := s.R.PingContext(ctx); err != nil {
		s.R.Close()
		s.W.Close()
		return nil, fmt.Errorf("open database: %w", err)
	}
	return s, nil
}

// peekVersion reads user_version without applying any pragma, so a newer
// database is never converted (for example to WAL) before being refused.
func peekVersion(ctx context.Context, file string) (int, error) {
	if _, err := os.Stat(file); errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	db, err := sql.Open("sqlite", dsn(file, "mode=ro"))
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var v int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return v, nil
}

// migrate applies ms[user_version:]. Rebuilding a table, the only way
// SQLite can change a CHECK constraint, needs foreign keys off, and that
// pragma is ignored inside a transaction. So the migrations run on the
// writer's one connection, pinned, with foreign keys off around them; each
// still runs in its own transaction and must leave no foreign key broken.
func (s *Store) migrate(ctx context.Context, ms []migration) (err error) {
	if err := s.W.QueryRowContext(ctx, "PRAGMA user_version").Scan(&s.version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	// Re-checked under the writer in case the file changed since the peek.
	if s.version > len(ms) {
		return &TooNewError{Have: s.version, Want: len(ms)}
	}
	if s.version == len(ms) {
		return nil
	}
	// W allows one connection, so nothing else can use it while it's pinned.
	conn, err := s.W.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return err
	}
	defer func() {
		// The connection goes back to the pool, so it must not keep foreign
		// keys off, even when ctx is done.
		if _, ferr := conn.ExecContext(context.WithoutCancel(ctx), "PRAGMA foreign_keys = ON"); ferr != nil {
			err = errors.Join(err, fmt.Errorf("restore foreign keys: %w", ferr))
		}
	}()
	for _, m := range ms[s.version:] {
		from := s.version
		if err := applyMigration(ctx, conn, m); err != nil {
			return fmt.Errorf("migration %s: %w", m.name, err)
		}
		s.version = m.version
		s.log.Info("migrated", "from", from, "to", m.version, "migration", m.name)
	}
	return nil
}

// applyMigration runs m and sets user_version in one immediate transaction,
// rolling back if m leaves a row whose foreign key points nowhere.
func applyMigration(ctx context.Context, conn *sql.Conn, m migration) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		return err
	}
	if err := foreignKeyCheck(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", m.version)); err != nil {
		return err
	}
	return tx.Commit()
}

func foreignKeyCheck(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		var table, parent string
		var rowid sql.NullInt64
		var fkid int
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			return err
		}
		return fmt.Errorf("foreign key check: a row in %s refers to a missing %s", table, parent)
	}
	return rows.Err()
}

func clearDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// Close closes R, checkpoints the WAL into sous.db with
// wal_checkpoint(TRUNCATE) on W (SQLITE_BUSY is logged as a warning, not
// returned), then closes W.
func (s *Store) Close() error {
	var errs []error
	if s.R != nil {
		errs = append(errs, s.R.Close())
	}
	var busy, logFrames, checkpointed int
	err := s.W.QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logFrames, &checkpointed)
	switch {
	case isBusy(err):
		s.log.Warn("wal checkpoint busy", "err", err)
	case err != nil:
		errs = append(errs, fmt.Errorf("wal checkpoint: %w", err))
	case busy != 0:
		s.log.Warn("wal checkpoint busy", "log_frames", logFrames, "checkpointed", checkpointed)
	}
	errs = append(errs, s.W.Close())
	return errors.Join(errs...)
}

func isBusy(err error) bool {
	var se *sqlite.Error
	return errors.As(err, &se) && se.Code()&0xff == sqlite3.SQLITE_BUSY
}

// Read runs fn in a read-only transaction on R.
func (s *Store) Read(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.R.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// Write runs fn in an immediate transaction on W, committing if fn returns
// nil and rolling back otherwise.
func (s *Store) Write(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.W.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// SchemaVersion is the database's user_version after Open's migrations.
func (s *Store) SchemaVersion() int {
	return s.version
}

// SizeBytes is the total size of the files under Dir: the database, its WAL
// and shared memory, and the images.
func (s *Store) SizeBytes() (int64, error) {
	var total int64
	err := filepath.WalkDir(s.Dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			// Files can vanish mid-walk (tmp uploads, -shm); skip them.
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		} else if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	return total, err
}
