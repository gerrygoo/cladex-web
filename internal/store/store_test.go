package store

import (
	"context"
	"database/sql"
	"io/fs"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	cladex "github.com/gerrygoo/cladex-web"
)

// Migrations apply in filename order, so a shared number is legal but leaves their order
// to the rest of the name. 0004 predates this check and can't be renamed once applied.
func TestMigrationNumbersAreUnique(t *testing.T) {
	entries, err := fs.ReadDir(cladex.MigrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, e := range entries {
		num, _, _ := strings.Cut(e.Name(), "_")
		if prev, dup := seen[num]; dup && num != "0004" {
			t.Errorf("%s and %s share number %s", prev, e.Name(), num)
		}
		seen[num] = e.Name()
	}
}

// Every pooled connection, not just the first, must enforce foreign keys and wait on a
// locked database rather than failing at once.
func TestOpenConfiguresEveryConnection(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	var conns []*sql.Conn
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()
	for i := 0; i < 10; i++ {
		c, err := s.db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)

		var fk, timeout int
		var mode string
		if err := c.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fk); err != nil {
			t.Fatal(err)
		}
		if err := c.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&timeout); err != nil {
			t.Fatal(err)
		}
		if err := c.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&mode); err != nil {
			t.Fatal(err)
		}
		if fk != 1 || timeout != 5000 || mode != "wal" {
			t.Errorf("conn %d: foreign_keys=%d busy_timeout=%d journal_mode=%s", i, fk, timeout, mode)
		}
	}
}

// breakingSQL matches the schema changes the previous release's code can't survive: a
// dropped table, view or column, or a rename. A table rebuild drops the old table, so it
// matches too. Changes it can't see (say, renaming the values of a status) are the
// author's to mark.
var breakingSQL = regexp.MustCompile(`(?i)\b(drop\s+(table|view|column)|rename)\b`)

var sqlComments = regexp.MustCompile(`(?s)--[^\n]*|/\*.*?\*/`)

// Production deploys blue/green: the old build keeps serving while the new one migrates,
// so a migration that breaks the old build's queries must say so, and deploy.sh then
// pauses production for it instead (docs/NAS_OPERATIONS.md#deploy-pipeline). 0001–0028
// predate blue/green and are applied everywhere.
func TestBreakingMigrationsAreExclusive(t *testing.T) {
	names, err := migrationNames(cladex.MigrationsFS)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if name <= "0028_~" {
			continue
		}
		b, err := fs.ReadFile(cladex.MigrationsFS, path.Join("migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		script := string(b)
		if hasMarker(script, exclusiveMarker) {
			continue
		}
		if m := breakingSQL.FindString(sqlComments.ReplaceAllString(script, "")); m != "" {
			t.Errorf("%s has %q, which the previous release's code can't run against while it migrates. "+
				"Either split it (add now, drop or rename in a later release, once no running code uses it) "+
				"or put %q among its first lines so deploy.sh pauses production for it.", name, m, exclusiveMarker)
		} else if hasMarker(script, foreignKeysOffMarker) {
			t.Errorf("%s rebuilds a table (%s) without %q", name, foreignKeysOffMarker, exclusiveMarker)
		}
	}
}

func TestHasMarker(t *testing.T) {
	both := foreignKeysOffMarker + "\n" + exclusiveMarker + "\n-- Why this migration exists.\nCREATE TABLE t (id INTEGER);\n"
	if !hasMarker(both, foreignKeysOffMarker) || !hasMarker(both, exclusiveMarker) {
		t.Error("both leading markers should be found")
	}
	late := "-- Why.\n" + exclusiveMarker + "\nCREATE TABLE t (id INTEGER);\n"
	if hasMarker(late, exclusiveMarker) {
		t.Error("a marker after other lines isn't a marker")
	}
}

func TestPendingExclusive(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "test.db")
	file := func(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }
	released := fstest.MapFS{
		"migrations/0001_a.sql": file("CREATE TABLE a (id INTEGER PRIMARY KEY, old TEXT);"),
	}
	next := fstest.MapFS{
		"migrations/0001_a.sql": released["migrations/0001_a.sql"],
		"migrations/0002_b.sql": file("ALTER TABLE a ADD COLUMN b TEXT;"),
		"migrations/0003_c.sql": file(exclusiveMarker + "\nALTER TABLE a DROP COLUMN old;"),
	}

	if got, err := PendingExclusive(ctx, dsn, next); err != nil || got != nil {
		t.Fatalf("no database yet: got %v, %v; want none", got, err)
	}

	s, err := Open(ctx, dsn, released)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := PendingExclusive(ctx, dsn, next)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"0003_c.sql"}; !slices.Equal(got, want) {
		t.Errorf("pending exclusive = %v, want %v", got, want)
	}
	// Asking must not migrate: 0002 is still pending.
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil || n != 1 {
		t.Errorf("schema_migrations rows = %d (%v), want 1", n, err)
	}

	s2, err := Open(ctx, dsn, next)
	if err != nil {
		t.Fatal(err)
	}
	s2.Close()
	if got, err := PendingExclusive(ctx, dsn, next); err != nil || got != nil {
		t.Errorf("after migrating: got %v, %v; want none", got, err)
	}
}
