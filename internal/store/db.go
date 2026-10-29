// Package store is Gokku's SQLite storage: connection setup, migrations and repositories.
// Dokku stays the source of truth; only Gokku's own state lives here.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"

	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"gokku/migrations"
)

// DB is the database with a single-connection writer and a read pool (DESIGN.md, 3.9).
type DB struct {
	W *sqlx.DB
	R *sqlx.DB
}

// Open creates the data directory if needed, opens gokku.db and applies pending migrations
// while holding a file lock, so concurrent gokku processes never migrate at the same time.
func Open(ctx context.Context, dataDir string, log *slog.Logger) (*DB, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	path := filepath.Join(dataDir, "gokku.db")

	w, err := open(path, "", 1)
	if err != nil {
		return nil, err
	}
	if err := migrate(ctx, w.DB, dataDir, log); err != nil {
		_ = w.Close()
		return nil, err
	}
	r, err := open(path, "&_pragma=query_only(1)", 8)
	if err != nil {
		_ = w.Close()
		return nil, err
	}
	return &DB{W: w, R: r}, nil
}

// OpenMemory opens a private in-memory database with all migrations applied, for tests.
// Reads and writes share one connection, since each in-memory connection is its own database.
func OpenMemory(ctx context.Context) (*DB, error) {
	db, err := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := runMigrations(ctx, db, nil); err != nil {
		_ = db.Close()
		return nil, err
	}
	x := sqlx.NewDb(db, "sqlite3")
	return &DB{W: x, R: x}, nil
}

// Close closes both pools.
func (db *DB) Close() error {
	if db.R == db.W {
		return db.W.Close()
	}
	return errors.Join(db.R.Close(), db.W.Close())
}

func open(path, extra string, maxConns int) (*sqlx.DB, error) {
	// Foreign keys are per connection, so every pragma must be in the DSN.
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)" + extra
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(maxConns)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("restrict database permissions: %w", err)
	}
	// "sqlite3" selects the ? bind style in sqlx; the driver itself is modernc's "sqlite".
	return sqlx.NewDb(db, "sqlite3"), nil
}

func migrate(ctx context.Context, db *sql.DB, dataDir string, log *slog.Logger) error {
	lock, err := os.OpenFile(filepath.Join(dataDir, "gokku.lock"), os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // the configured data directory
	if err != nil {
		return fmt.Errorf("open migration lock: %w", err)
	}
	defer func() { _ = lock.Close() }()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("take migration lock: %w", err)
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()
	return runMigrations(ctx, db, log)
}

func runMigrations(ctx context.Context, db *sql.DB, log *slog.Logger) error {
	opts := []goose.ProviderOption{goose.WithDisableGlobalRegistry(true)}
	if log != nil {
		opts = append(opts, goose.WithSlog(log))
	}
	p, err := goose.NewProvider(goose.DialectSQLite3, db, migrations.FS, opts...)
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
