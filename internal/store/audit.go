package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
)

// Audit sources, recorded alongside the actor so a write from the admin CLI is
// distinguishable from the same write made through the web UI.
const (
	SourceWeb    = "web"
	SourceCLI    = "cli"
	SourceImport = "import"
)

// Actor identifies who is performing a write, for the audit triggers in
// migrations/0003_audit_log.sql. UserID is 0 when there is no logged-in user (the CLI
// and the catalog importer both run unattended), in which case only Source is recorded.
type Actor struct {
	UserID int64
	Source string
}

type actorCtxKey struct{}

// WithActor returns a context that attributes any subsequent store write to a. The web
// layer attaches the session user; cmd/cladexctl and cmd/import attach their source.
func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorCtxKey{}, a)
}

// ActorFromContext returns the actor attached by WithActor, if any.
func ActorFromContext(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(actorCtxKey{}).(Actor)
	return a, ok
}

// stampActor writes ctx's actor (or the unattributed zero value) into the single-row
// audit_actor table, inside tx, so the AFTER INSERT/UPDATE/DELETE triggers on an
// audited table can attribute whatever tx writes next. Callers that only need one
// statement should use exec instead; stampActor exists for callers that must compose
// an audited write into a larger, already-open transaction (e.g. one that also touches
// an unaudited table) — see store/quotes.go's ReplaceQuoteLines and CreateRevision,
// which each write both quote_lines (unaudited) and quotes (audited) atomically.
//
// The stamp happens on every call, including when the context carries no actor — that
// clears the previous writer's identity, so an unattributed write is recorded as
// unattributed rather than inheriting whoever wrote last. SQLite serialises write
// transactions even in WAL mode, so no concurrent writer can observe or clobber the row
// between the stamp and the write.
func stampActor(ctx context.Context, tx *sql.Tx) error {
	var actor Actor
	if a, ok := ActorFromContext(ctx); ok {
		actor = a
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE audit_actor SET user_id = ?, source = ? WHERE id = 1`,
		nullIfZero(actor.UserID), nullIfEmpty(actor.Source),
	); err != nil {
		return fmt.Errorf("store: stamp audit actor: %w", err)
	}
	return nil
}

// exec runs a single write in its own transaction, via stampActor. Every one-statement
// write in this package goes through here rather than db.ExecContext.
func (s *Store) exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store: begin: %w", err)
	}
	defer tx.Rollback()

	if err := stampActor(ctx, tx); err != nil {
		return nil, err
	}

	res, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	// modernc.org/sqlite materialises LastInsertId/RowsAffected at exec time, so the
	// returned Result stays valid after the transaction closes.
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: commit: %w", err)
	}
	return res, nil
}

func nullIfZero(n int64) any {
	if n == 0 {
		return nil
	}
	return n
}

// AuditEntry is one row of the trail. OldValues and NewValues are the audited columns
// before and after the write; one of them is nil for an insert or a delete.
type AuditEntry struct {
	ID        int64
	TableName string
	RowKey    string
	Op        string // "insert", "update", or "delete"
	ActorID   *int64
	ActorName string // joined from users; empty for CLI/import/unattributed writes
	Source    string
	OldValues map[string]any
	NewValues map[string]any
	At        string
}

// AuditFilter narrows an AuditLog query. Zero-valued fields are not applied.
type AuditFilter struct {
	TableName string
	RowKey    string
	ActorID   int64
	Limit     int
}

const defaultAuditLimit = 200

// AuditLog returns matching audit rows, newest first.
func (s *Store) AuditLog(ctx context.Context, f AuditFilter) ([]AuditEntry, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = defaultAuditLimit
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id, a.table_name, a.row_key, a.op, a.actor_id,
		       COALESCE(u.username, ''), COALESCE(a.source, ''),
		       a.old_values, a.new_values, a.at
		FROM audit_log a
		LEFT JOIN users u ON u.id = a.actor_id
		WHERE (? = '' OR a.table_name = ?)
		  AND (? = '' OR a.row_key = ?)
		  AND (? = 0  OR a.actor_id = ?)
		ORDER BY a.id DESC
		LIMIT ?`,
		f.TableName, f.TableName, f.RowKey, f.RowKey, f.ActorID, f.ActorID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("store: audit log: %w", err)
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var actorID sql.NullInt64
		var oldJSON, newJSON sql.NullString
		if err := rows.Scan(&e.ID, &e.TableName, &e.RowKey, &e.Op, &actorID,
			&e.ActorName, &e.Source, &oldJSON, &newJSON, &e.At); err != nil {
			return nil, fmt.Errorf("store: audit log: %w", err)
		}
		if actorID.Valid {
			id := actorID.Int64
			e.ActorID = &id
		}
		if e.OldValues, err = decodeAuditValues(oldJSON); err != nil {
			return nil, fmt.Errorf("store: audit log entry %d: %w", e.ID, err)
		}
		if e.NewValues, err = decodeAuditValues(newJSON); err != nil {
			return nil, fmt.Errorf("store: audit log entry %d: %w", e.ID, err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func decodeAuditValues(v sql.NullString) (map[string]any, error) {
	if !v.Valid || v.String == "" {
		return nil, nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(v.String), &m); err != nil {
		return nil, err
	}
	return m, nil
}

// passwordChangedKey is a marker the users update trigger sets instead of writing the
// bcrypt hash. It has no counterpart in OldValues, so ChangedFields treats it as a
// change only when it is actually set.
const passwordChangedKey = "password_changed"

// ChangedFields returns the audited columns whose value differs between OldValues and
// NewValues, for rendering "cost_micros: 5000000 → 5250000" instead of the whole row.
// An insert or a delete reports every present field.
func (e AuditEntry) ChangedFields() []string {
	var changed []string
	seen := map[string]bool{}
	for k, newVal := range e.NewValues {
		seen[k] = true
		if k == passwordChangedKey {
			if truthyAuditValue(newVal) {
				changed = append(changed, k)
			}
			continue
		}
		if oldVal, ok := e.OldValues[k]; !ok || !sameAuditValue(oldVal, newVal) {
			changed = append(changed, k)
		}
	}
	for k := range e.OldValues {
		if !seen[k] {
			changed = append(changed, k)
		}
	}
	sort.Strings(changed)
	return changed
}

func truthyAuditValue(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	case string:
		return t != "" && t != "0"
	default:
		return v != nil
	}
}

func sameAuditValue(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}
