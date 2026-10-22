package store

import (
	"context"
	"database/sql"
	"testing"
)

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
