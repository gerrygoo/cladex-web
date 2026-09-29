// Package store owns database/sql access. It is plain, hand-written SQL — the schema is
// small enough that this stays tractable without a query builder.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

// Open opens (creating if necessary) the SQLite database at path, applies any
// pending migrations from migrationsFS, and returns a ready Store.
func Open(ctx context.Context, dsn string, migrationsFS fs.FS) (*Store, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: pragma: %w", err)
	}
	if err := migrate(ctx, db, migrationsFS); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: migrate: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// sortColumn maps an external, URL-facing column name to the literal SQL expression
// it sorts by.
type sortColumn struct {
	name string
	expr string
}

// orderByClause builds a safe "ORDER BY <expr> ASC|DESC" clause for a list handler's
// sort/dir query params. allowed is a whitelist — col is looked up against it rather
// than concatenated into SQL directly, since it comes straight from the URL; the first
// entry in allowed is the default when col doesn't match anything in it.
func orderByClause(allowed []sortColumn, col, dir string) string {
	expr := allowed[0].expr
	for _, a := range allowed {
		if a.name == col {
			expr = a.expr
			break
		}
	}
	if dir == "desc" {
		return "ORDER BY " + expr + " DESC"
	}
	return "ORDER BY " + expr + " ASC"
}

func migrate(ctx context.Context, db *sql.DB, migrationsFS fs.FS) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename   TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && path.Ext(e.Name()) == ".sql" {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var applied int
		if err := db.QueryRowContext(ctx,
			`SELECT count(*) FROM schema_migrations WHERE filename = ?`, name,
		).Scan(&applied); err != nil {
			return fmt.Errorf("check %s: %w", name, err)
		}
		if applied > 0 {
			continue
		}

		sqlBytes, err := fs.ReadFile(migrationsFS, path.Join("migrations", name))
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}

		if err := applyMigration(ctx, db, name, string(sqlBytes)); err != nil {
			return err
		}
	}
	return nil
}

// foreignKeysOffMarker, as the first line of a migration, makes the runner apply it with
// foreign key enforcement off. SQLite can't alter a constraint in place, so a migration
// that drops one rebuilds the table, and dropping a table other tables reference is
// refused while enforcement is on. PRAGMA foreign_keys is a no-op inside a transaction,
// so the runner has to flip it on the connection around the transaction.
const foreignKeysOffMarker = "-- cladex:foreign-keys-off"

// applyMigration runs one migration file and records it in schema_migrations, in a
// single transaction. A foreign-keys-off migration is followed by a foreign_key_check
// inside that transaction, so a rebuild that orphans rows is rolled back rather than
// committed, and enforcement is restored on the connection afterwards.
func applyMigration(ctx context.Context, db *sql.DB, name, script string) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("conn for %s: %w", name, err)
	}
	defer conn.Close()

	fkOff := strings.HasPrefix(script, foreignKeysOffMarker)
	if fkOff {
		if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
			return fmt.Errorf("disable foreign keys for %s: %w", name, err)
		}
		defer conn.ExecContext(context.WithoutCancel(ctx), `PRAGMA foreign_keys = ON`)
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin %s: %w", name, err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, script); err != nil {
		return fmt.Errorf("apply %s: %w", name, err)
	}
	if fkOff {
		var table string
		err := tx.QueryRowContext(ctx, `SELECT "table" FROM pragma_foreign_key_check LIMIT 1`).Scan(&table)
		if err == nil {
			return fmt.Errorf("apply %s: foreign key violation in %s", name, table)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("foreign key check for %s: %w", name, err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (filename) VALUES (?)`, name,
	); err != nil {
		return fmt.Errorf("record %s: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit %s: %w", name, err)
	}
	return nil
}
