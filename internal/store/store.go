// Package store owns database/sql access. It is plain, hand-written SQL — the schema is
// small enough that this stays tractable without a query builder.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

// connParams are added to the DSN so the driver applies them to every connection it
// opens. PRAGMAs are per connection, so running them once through the sql.DB pool would
// only configure whichever connection happened to run them. busy_timeout makes a writer
// wait for the lock instead of failing with SQLITE_BUSY, and _txlock=immediate takes the
// write lock at BEGIN, so a transaction that reads before it writes can't fail to upgrade.
const connParams = "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_txlock=immediate"

// Open opens (creating if necessary) the SQLite database at dsn, applies any
// pending migrations from migrationsFS, and returns a ready Store.
func Open(ctx context.Context, dsn string, migrationsFS fs.FS) (*Store, error) {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	db, err := sql.Open("sqlite", dsn+sep+connParams)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: open: %w", err)
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

	names, err := migrationNames(migrationsFS)
	if err != nil {
		return err
	}

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

// migrationNames lists migrationsFS's migration files in the order they apply.
func migrationNames(migrationsFS fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && path.Ext(e.Name()) == ".sql" {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// Markers are a migration's leading lines that start with "-- cladex:", one per line,
// before anything else in the file.
const (
	// foreignKeysOffMarker makes the runner apply the migration with foreign key
	// enforcement off. SQLite can't alter a constraint in place, so a migration that drops
	// one rebuilds the table, and dropping a table other tables reference is refused while
	// enforcement is on. PRAGMA foreign_keys is a no-op inside a transaction, so the runner
	// has to flip it on the connection around the transaction.
	foreignKeysOffMarker = "-- cladex:foreign-keys-off"

	// exclusiveMarker says the previous release's code can't run against the schema this
	// migration leaves (a dropped or renamed table or column, a rebuild, changed values).
	// Production deploys blue/green, the old build serving while the new one migrates;
	// for a pending exclusive migration deploy.sh stops the old build first and shows a
	// maintenance page until the new one is up (docs/NAS_OPERATIONS.md#deploy-pipeline).
	// TestBreakingMigrationsAreExclusive requires it on the changes it can recognise.
	exclusiveMarker = "-- cladex:exclusive"
)

// hasMarker reports whether marker is among script's leading marker lines.
func hasMarker(script, marker string) bool {
	for _, line := range strings.Split(script, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "-- cladex:") {
			return false
		}
		if line == marker {
			return true
		}
	}
	return false
}

// PendingExclusive lists the migrations in migrationsFS that the database at dsn hasn't
// applied yet and that are marked exclusive. It opens the database read-only and never
// migrates it, so deploy.sh can ask a new image before starting it. A database that
// doesn't exist yet has nothing serving from it, so it reports none.
func PendingExclusive(ctx context.Context, dsn string, migrationsFS fs.FS) ([]string, error) {
	if _, err := os.Stat(dsn); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	db, err := sql.Open("sqlite", "file:"+dsn+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	defer db.Close()

	applied := map[string]bool{}
	rows, err := db.QueryContext(ctx, `SELECT filename FROM schema_migrations`)
	if err != nil && !strings.Contains(err.Error(), "no such table") {
		return nil, fmt.Errorf("store: read schema_migrations: %w", err)
	}
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return nil, fmt.Errorf("store: read schema_migrations: %w", err)
			}
			applied[name] = true
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("store: read schema_migrations: %w", err)
		}
	}

	names, err := migrationNames(migrationsFS)
	if err != nil {
		return nil, err
	}
	var exclusive []string
	for _, name := range names {
		if applied[name] {
			continue
		}
		script, err := fs.ReadFile(migrationsFS, path.Join("migrations", name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		if hasMarker(string(script), exclusiveMarker) {
			exclusive = append(exclusive, name)
		}
	}
	return exclusive, nil
}

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

	fkOff := hasMarker(script, foreignKeysOffMarker)
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
