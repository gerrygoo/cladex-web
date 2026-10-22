package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// SupplierQuote is the cotización del proveedor of a quote's revision chain (issue #39,
// migrations/0025_supplier_quotes.sql): who sent it, its number, and its files, oldest
// first.
type SupplierQuote struct {
	RootQuoteID int64
	Supplier    string
	Number      string
	Files       []SupplierQuoteFile
}

// SupplierQuoteFile is a supplier_quote_files row without its bytes. Removable reports
// whether it can still be removed: the chain's current quote is a borrador and the file
// came after the chain's last emission.
type SupplierQuoteFile struct {
	ID             int64
	Filename       string
	ContentType    string
	Size           int64
	UploadedByName string
	UploadedAt     string
	Removable      bool
}

// ErrSupplierFileLocked is returned by RemoveSupplierQuoteFile for a file an issued quote
// was based on, or while the chain's current quote isn't a borrador.
var ErrSupplierFileLocked = errors.New("store: supplier quote file can't be removed")

// QuoteChainRoot returns the id of the first quote of quoteID's revision chain, the one
// that revises nothing, or 0 when there is no such quote. A revision always shares its
// original's proyecto, and only a first quote opens one, so the root is the proyecto's
// one quote that revises nothing; a quote without a proyecto has no revisions yet and is
// its own root.
func (s *Store) QuoteChainRoot(ctx context.Context, quoteID int64) (int64, error) {
	var root int64
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(
			(SELECT r.id FROM quotes r WHERE r.project_id = q.project_id AND r.supersedes_quote_id IS NULL),
			q.id)
		FROM quotes q WHERE q.id = ?`, quoteID).Scan(&root)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("store: chain root of quote %d: %w", quoteID, err)
	}
	return root, nil
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// chainState returns the status of the chain's current quote (the one that isn't
// revisada) and when the chain was last issued, "" if never.
func chainState(ctx context.Context, q queryer, root int64) (current, lastIssued string, err error) {
	err = q.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(CASE WHEN status != 'revisada' THEN status END), ''), COALESCE(MAX(issued_at), '')
		FROM quotes WHERE id = ?1 OR project_id = (SELECT project_id FROM quotes WHERE id = ?1)`, root).
		Scan(&current, &lastIssued)
	return current, lastIssued, err
}

// supplierFileRemovable is the removal rule. Timestamps are compared to the second:
// issued_at is kept in whole seconds, so a file uploaded within the second of an
// emission counts as part of it.
func supplierFileRemovable(current, lastIssued, uploadedAt string) bool {
	return current == "borrador" && wholeSecond(uploadedAt) > wholeSecond(lastIssued)
}

func wholeSecond(ts string) string {
	if len(ts) > 19 {
		return ts[:19]
	}
	return ts
}

// SupplierQuoteOf returns the cotización del proveedor of quoteID's chain, empty (no
// supplier, no files) when nothing was added yet, or nil when there is no such quote.
func (s *Store) SupplierQuoteOf(ctx context.Context, quoteID int64) (*SupplierQuote, error) {
	root, err := s.QuoteChainRoot(ctx, quoteID)
	if err != nil || root == 0 {
		return nil, err
	}
	sq := &SupplierQuote{RootQuoteID: root}
	current, lastIssued, err := chainState(ctx, s.db, root)
	if err != nil {
		return nil, fmt.Errorf("store: supplier quote of %d: %w", quoteID, err)
	}
	err = s.db.QueryRowContext(ctx, `SELECT supplier, number FROM supplier_quotes WHERE root_quote_id = ?`, root).
		Scan(&sq.Supplier, &sq.Number)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("store: supplier quote of %d: %w", quoteID, err)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT f.id, f.filename, f.content_type, f.size, u.name, f.uploaded_at
		FROM supplier_quote_files f JOIN users u ON u.id = f.uploaded_by
		WHERE f.root_quote_id = ? AND f.deleted_at IS NULL ORDER BY f.id`, root)
	if err != nil {
		return nil, fmt.Errorf("store: supplier quote files of %d: %w", quoteID, err)
	}
	defer rows.Close()
	for rows.Next() {
		var f SupplierQuoteFile
		if err := rows.Scan(&f.ID, &f.Filename, &f.ContentType, &f.Size, &f.UploadedByName, &f.UploadedAt); err != nil {
			return nil, fmt.Errorf("store: supplier quote files of %d: %w", quoteID, err)
		}
		f.Removable = supplierFileRemovable(current, lastIssued, f.UploadedAt)
		sq.Files = append(sq.Files, f)
	}
	return sq, rows.Err()
}

// SetSupplierQuoteInfo saves the supplier and the supplier's quote number for quoteID's
// chain, trimmed; blank clears them. Only while the chain's current quote is a borrador
// (ErrQuoteNotDraft otherwise).
func (s *Store) SetSupplierQuoteInfo(ctx context.Context, quoteID int64, supplier, number string, userID int64) error {
	root, err := s.QuoteChainRoot(ctx, quoteID)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: set supplier quote of %d: %w", quoteID, err)
	}
	defer tx.Rollback()
	current, _, err := chainState(ctx, tx, root)
	if err != nil {
		return fmt.Errorf("store: set supplier quote of %d: %w", quoteID, err)
	}
	if current != "borrador" {
		return ErrQuoteNotDraft
	}
	if err := stampActor(ctx, tx); err != nil {
		return fmt.Errorf("store: set supplier quote of %d: %w", quoteID, err)
	}
	supplier, number = strings.TrimSpace(supplier), strings.TrimSpace(number)
	// Nothing typed on a chain that has no row yet leaves it without one.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO supplier_quotes (root_quote_id, supplier, number, updated_by)
		SELECT ?1, ?2, ?3, ?4 WHERE ?2 != '' OR ?3 != ''
		ON CONFLICT (root_quote_id) DO UPDATE SET
			supplier = excluded.supplier, number = excluded.number,
			updated_by = excluded.updated_by, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE supplier != excluded.supplier OR number != excluded.number`,
		root, supplier, number, userID); err != nil {
		return fmt.Errorf("store: set supplier quote of %d: %w", quoteID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: set supplier quote of %d: %w", quoteID, err)
	}
	return nil
}

// AddSupplierQuoteFiles attaches files to quoteID's chain, all or none. Adding is
// allowed whatever state the chain is in.
func (s *Store) AddSupplierQuoteFiles(ctx context.Context, quoteID int64, files []NewFile, userID int64) error {
	root, err := s.QuoteChainRoot(ctx, quoteID)
	if err != nil {
		return err
	}
	if root == 0 {
		return fmt.Errorf("store: add supplier quote files: no quote %d", quoteID)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: add supplier quote files to %d: %w", quoteID, err)
	}
	defer tx.Rollback()
	for _, f := range files {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO supplier_quote_files (root_quote_id, filename, content_type, size, data, uploaded_by)
			VALUES (?, ?, ?, ?, ?, ?)`,
			root, strings.TrimSpace(f.Filename), f.ContentType, len(f.Data), f.Data, userID); err != nil {
			return fmt.Errorf("store: add supplier quote files to %d: %w", quoteID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: add supplier quote files to %d: %w", quoteID, err)
	}
	return nil
}

// SupplierQuoteFileData returns one of the files of quoteID's chain with its bytes, or
// nil when the chain has no such file or it was removed.
func (s *Store) SupplierQuoteFileData(ctx context.Context, quoteID, fileID int64) (*SupplierQuoteFile, []byte, error) {
	root, err := s.QuoteChainRoot(ctx, quoteID)
	if err != nil || root == 0 {
		return nil, nil, err
	}
	var f SupplierQuoteFile
	var data []byte
	err = s.db.QueryRowContext(ctx, `
		SELECT id, filename, content_type, size, uploaded_at, data FROM supplier_quote_files
		WHERE id = ? AND root_quote_id = ? AND deleted_at IS NULL`, fileID, root).
		Scan(&f.ID, &f.Filename, &f.ContentType, &f.Size, &f.UploadedAt, &data)
	if err == sql.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("store: supplier quote file %d: %w", fileID, err)
	}
	return &f, data, nil
}

// RemoveSupplierQuoteFile takes a file off quoteID's chain, recording who did it; its
// bytes stay in the database. It reports whether the chain had such a file, and returns
// ErrSupplierFileLocked when the file can no longer be removed.
func (s *Store) RemoveSupplierQuoteFile(ctx context.Context, quoteID, fileID, userID int64) (bool, error) {
	root, err := s.QuoteChainRoot(ctx, quoteID)
	if err != nil || root == 0 {
		return false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("store: remove supplier quote file %d: %w", fileID, err)
	}
	defer tx.Rollback()
	var uploadedAt string
	err = tx.QueryRowContext(ctx, `
		SELECT uploaded_at FROM supplier_quote_files
		WHERE id = ? AND root_quote_id = ? AND deleted_at IS NULL`, fileID, root).Scan(&uploadedAt)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: remove supplier quote file %d: %w", fileID, err)
	}
	current, lastIssued, err := chainState(ctx, tx, root)
	if err != nil {
		return false, fmt.Errorf("store: remove supplier quote file %d: %w", fileID, err)
	}
	if !supplierFileRemovable(current, lastIssued, uploadedAt) {
		return true, ErrSupplierFileLocked
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE supplier_quote_files
		SET deleted_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), deleted_by = ?
		WHERE id = ?`, userID, fileID); err != nil {
		return false, fmt.Errorf("store: remove supplier quote file %d: %w", fileID, err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("store: remove supplier quote file %d: %w", fileID, err)
	}
	return true, nil
}
