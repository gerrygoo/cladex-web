package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrHasStampedInvoice means a proyecto can't be deleted because one of its comprobantes
// has been, or may have been, stamped by the PAC: a CFDI is a legal document.
var ErrHasStampedInvoice = errors.New("store: project has a stamped comprobante")

// ErrNotDeletable means a quote can't be deleted on its own: it is issued, or it belongs
// to a proyecto (which is deleted as a whole).
var ErrNotDeletable = errors.New("store: quote can't be deleted on its own")

// deletableComprobanteStatuses are the states in which nothing was stamped.
const deletableComprobanteStatuses = `'borrador', 'lista', 'rechazada'`

// DeleteProject removes a proyecto with everything that hangs from it: its comprobantes
// (only unstamped ones; otherwise ErrHasStampedInvoice and nothing is touched), the
// purchase order files and every quote it ever had, with their lines, comments and
// supplier quotes. No foreign key cascades, so the order here is the dependency order.
// Folios are not reused: folio_sequences only counts up.
func (s *Store) DeleteProject(ctx context.Context, projectID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: delete project %d: %w", projectID, err)
	}
	defer tx.Rollback()
	if err := stampActor(ctx, tx); err != nil {
		return fmt.Errorf("store: delete project %d: %w", projectID, err)
	}
	var stamped int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM comprobantes
		WHERE project_id = ? AND status NOT IN (`+deletableComprobanteStatuses+`)`, projectID).Scan(&stamped); err != nil {
		return fmt.Errorf("store: delete project %d: %w", projectID, err)
	}
	if stamped > 0 {
		return ErrHasStampedInvoice
	}
	steps := []struct{ what, query string }{
		{"comprobante conceptos", `DELETE FROM comprobante_conceptos WHERE comprobante_id IN (SELECT id FROM comprobantes WHERE project_id = ?1)`},
		{"comprobante events", `DELETE FROM comprobante_events WHERE comprobante_id IN (SELECT id FROM comprobantes WHERE project_id = ?1)`},
		{"comprobante links", `UPDATE comprobantes SET related_id = NULL WHERE project_id = ?1`},
		{"comprobantes", `DELETE FROM comprobantes WHERE project_id = ?1`},
		{"project files", `DELETE FROM project_files WHERE project_id = ?1`},
		{"quote comments", `DELETE FROM quote_comments WHERE quote_id IN (SELECT id FROM quotes WHERE project_id = ?1)`},
		{"quote lines", `DELETE FROM quote_lines WHERE quote_id IN (SELECT id FROM quotes WHERE project_id = ?1)`},
		{"supplier quote files", `DELETE FROM supplier_quote_files WHERE root_quote_id IN (SELECT id FROM quotes WHERE project_id = ?1)`},
		{"supplier quotes", `DELETE FROM supplier_quotes WHERE root_quote_id IN (SELECT id FROM quotes WHERE project_id = ?1)`},
	}
	for _, st := range steps {
		if _, err := tx.ExecContext(ctx, st.query, projectID); err != nil {
			return fmt.Errorf("store: delete project %d: %s: %w", projectID, st.what, err)
		}
	}
	// Each revision points at the one it revises, so the chain goes newest first: every
	// round deletes the quotes no other quote points at. Unlinking them first instead
	// would turn each revision into a first quote sharing its folio number.
	for {
		res, err := tx.ExecContext(ctx, `
			DELETE FROM quotes WHERE project_id = ?1 AND id NOT IN
				(SELECT supersedes_quote_id FROM quotes WHERE project_id = ?1 AND supersedes_quote_id IS NOT NULL)`, projectID)
		if err != nil {
			return fmt.Errorf("store: delete project %d: quotes: %w", projectID, err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			break
		}
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, projectID)
	if err != nil {
		return fmt.Errorf("store: delete project %d: %w", projectID, err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: delete project %d: %w", projectID, err)
	}
	return nil
}

// DeleteDraftQuote removes a first draft that never became a proyecto, with its lines,
// comments and supplier quotes. Anything issued, or already part of a proyecto, is
// ErrNotDeletable.
func (s *Store) DeleteDraftQuote(ctx context.Context, quoteID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: delete draft quote %d: %w", quoteID, err)
	}
	defer tx.Rollback()
	if err := stampActor(ctx, tx); err != nil {
		return fmt.Errorf("store: delete draft quote %d: %w", quoteID, err)
	}
	var ok int
	err = tx.QueryRowContext(ctx, `
		SELECT 1 FROM quotes WHERE id = ? AND status = 'borrador' AND project_id IS NULL
			AND supersedes_quote_id IS NULL`, quoteID).Scan(&ok)
	if err == sql.ErrNoRows {
		return ErrNotDeletable
	}
	if err != nil {
		return fmt.Errorf("store: delete draft quote %d: %w", quoteID, err)
	}
	var used int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM comprobantes WHERE quote_id = ?`, quoteID).Scan(&used); err != nil {
		return fmt.Errorf("store: delete draft quote %d: %w", quoteID, err)
	}
	if used > 0 {
		return ErrNotDeletable
	}
	for _, q := range []string{
		`DELETE FROM quote_comments WHERE quote_id = ?1`,
		`DELETE FROM quote_lines WHERE quote_id = ?1`,
		`DELETE FROM supplier_quote_files WHERE root_quote_id = ?1`,
		`DELETE FROM supplier_quotes WHERE root_quote_id = ?1`,
		`DELETE FROM quotes WHERE id = ?1`,
	} {
		if _, err := tx.ExecContext(ctx, q, quoteID); err != nil {
			return fmt.Errorf("store: delete draft quote %d: %w", quoteID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: delete draft quote %d: %w", quoteID, err)
	}
	return nil
}

// ReassignProject makes newUserID the owner (vendedor) of a proyecto and of all its
// quotes, and leaves a comment on its current quote saying who did it. Issued quotes
// keep the vendedor_snapshot their PDF was frozen with; drafts have none yet.
// newUserID must be an enabled user.
func (s *Store) ReassignProject(ctx context.Context, projectID, actorID, newUserID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: reassign project %d: %w", projectID, err)
	}
	defer tx.Rollback()
	if err := stampActor(ctx, tx); err != nil {
		return fmt.Errorf("store: reassign project %d: %w", projectID, err)
	}
	oldName, newName, err := ownerNames(ctx, tx, `SELECT user_id FROM projects WHERE id = ?`, projectID, newUserID)
	if err != nil {
		return fmt.Errorf("store: reassign project %d: %w", projectID, err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE projects SET user_id = ? WHERE id = ?`, newUserID, projectID); err != nil {
		return fmt.Errorf("store: reassign project %d: %w", projectID, err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE quotes SET user_id = ? WHERE project_id = ?`, newUserID, projectID); err != nil {
		return fmt.Errorf("store: reassign project %d: quotes: %w", projectID, err)
	}
	quoteID, _, err := currentQuote(ctx, tx, projectID)
	if err != nil {
		return fmt.Errorf("store: reassign project %d: %w", projectID, err)
	}
	if quoteID != 0 {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO quote_comments (quote_id, user_id, body) VALUES (?, ?, ?)`,
			quoteID, actorID, fmt.Sprintf("Se reasignó de %s a %s.", oldName, newName)); err != nil {
			return fmt.Errorf("store: reassign project %d: comment: %w", projectID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: reassign project %d: %w", projectID, err)
	}
	return nil
}

// ReassignDraftQuote is ReassignProject for a first draft that has no proyecto yet.
func (s *Store) ReassignDraftQuote(ctx context.Context, quoteID, actorID, newUserID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: reassign quote %d: %w", quoteID, err)
	}
	defer tx.Rollback()
	if err := stampActor(ctx, tx); err != nil {
		return fmt.Errorf("store: reassign quote %d: %w", quoteID, err)
	}
	var ok int
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM quotes WHERE id = ? AND status = 'borrador' AND project_id IS NULL`, quoteID).Scan(&ok)
	if err == sql.ErrNoRows {
		return ErrNotDeletable
	}
	if err != nil {
		return fmt.Errorf("store: reassign quote %d: %w", quoteID, err)
	}
	oldName, newName, err := ownerNames(ctx, tx, `SELECT user_id FROM quotes WHERE id = ?`, quoteID, newUserID)
	if err != nil {
		return fmt.Errorf("store: reassign quote %d: %w", quoteID, err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE quotes SET user_id = ? WHERE id = ?`, newUserID, quoteID); err != nil {
		return fmt.Errorf("store: reassign quote %d: %w", quoteID, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO quote_comments (quote_id, user_id, body) VALUES (?, ?, ?)`,
		quoteID, actorID, fmt.Sprintf("Se reasignó de %s a %s.", oldName, newName)); err != nil {
		return fmt.Errorf("store: reassign quote %d: comment: %w", quoteID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: reassign quote %d: %w", quoteID, err)
	}
	return nil
}

// ownerNames returns the names of the current owner (found by ownerQuery with id) and
// of newUserID, which must exist and be enabled.
func ownerNames(ctx context.Context, tx *sql.Tx, ownerQuery string, id, newUserID int64) (oldName, newName string, err error) {
	var oldID int64
	if err = tx.QueryRowContext(ctx, ownerQuery, id).Scan(&oldID); err != nil {
		return "", "", err
	}
	if err = tx.QueryRowContext(ctx, `SELECT name FROM users WHERE id = ?`, oldID).Scan(&oldName); err != nil {
		return "", "", err
	}
	err = tx.QueryRowContext(ctx, `SELECT name FROM users WHERE id = ? AND disabled_at IS NULL`, newUserID).Scan(&newName)
	if err == sql.ErrNoRows {
		return "", "", ErrBadOwner
	}
	return oldName, newName, err
}

// ErrBadOwner means the user chosen as new owner doesn't exist or is disabled.
var ErrBadOwner = errors.New("store: new owner is not an enabled user")

// ActiveUsers lists the enabled users by name: who a record can be assigned to.
func (s *Store) ActiveUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, username, name, role FROM users WHERE disabled_at IS NULL ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, fmt.Errorf("store: list active users: %w", err)
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Name, &u.Role); err != nil {
			return nil, fmt.Errorf("store: list active users: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
