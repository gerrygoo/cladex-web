package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/gerrygoo/cladex-web/internal/money"
)

// MarginOption is a margin_options row: one named margin a quote can be priced with.
// ValueMicros is a fraction of the sale price (123_400 == 12.34%). RetiredAt is set once
// an admin retires it; retired options stay in the table because issued quotes and
// audit_log name them. See migrations/0007_margin_options.sql.
type MarginOption struct {
	ID          int64
	Name        string
	ValueMicros money.Micros
	IsDefault   bool
	RetiredAt   *string
}

// Active reports whether the option can still be picked for a quote.
func (o MarginOption) Active() bool {
	return o.RetiredAt == nil
}

var (
	// ErrDuplicateMarginName is returned when another option already has the name.
	ErrDuplicateMarginName = errors.New("store: a margin option with this name already exists")
	// ErrMarginOptionIsDefault is returned by RetireMarginOption for the default option:
	// new quotes start on it, so another option has to become the default first.
	ErrMarginOptionIsDefault = errors.New("store: the default margin option cannot be retired")
	// ErrMarginOptionRetired is returned by SetDefaultMarginOption for a retired option.
	ErrMarginOptionRetired = errors.New("store: margin option is retired")
	// ErrMarginOptionNotFound is returned when the id matches no option.
	ErrMarginOptionNotFound = errors.New("store: margin option not found")
)

const marginOptionCols = `id, name, value_micros, is_default, retired_at`

func scanMarginOption(row interface{ Scan(...any) error }) (*MarginOption, error) {
	var o MarginOption
	var retiredAt sql.NullString
	if err := row.Scan(&o.ID, &o.Name, &o.ValueMicros, &o.IsDefault, &retiredAt); err != nil {
		return nil, err
	}
	if retiredAt.Valid {
		o.RetiredAt = &retiredAt.String
	}
	return &o, nil
}

// ListMarginOptions returns every option, active ones first, each group ordered by
// value, the order the quote builder's dropdown and /ajustes show them in.
func (s *Store) ListMarginOptions(ctx context.Context) ([]MarginOption, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+marginOptionCols+` FROM margin_options
		ORDER BY retired_at IS NOT NULL, value_micros, id`)
	if err != nil {
		return nil, fmt.Errorf("store: list margin options: %w", err)
	}
	defer rows.Close()

	var opts []MarginOption
	for rows.Next() {
		o, err := scanMarginOption(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list margin options: %w", err)
		}
		opts = append(opts, *o)
	}
	return opts, rows.Err()
}

// MarginOptionByID returns the option with the given id, retired or not, or nil if
// none exists.
func (s *Store) MarginOptionByID(ctx context.Context, id int64) (*MarginOption, error) {
	o, err := scanMarginOption(s.db.QueryRowContext(ctx,
		`SELECT `+marginOptionCols+` FROM margin_options WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: margin option %d: %w", id, err)
	}
	return o, nil
}

// CreateMarginOption inserts a new, active, non-default option and returns its id.
func (s *Store) CreateMarginOption(ctx context.Context, name string, value money.Micros, updatedBy int64) (int64, error) {
	res, err := s.exec(ctx, `
		INSERT INTO margin_options (name, value_micros, updated_by) VALUES (?, ?, ?)`,
		name, int64(value), nullIfZero(updatedBy))
	if isUniqueViolation(err, "margin_options.name") {
		return 0, ErrDuplicateMarginName
	}
	if err != nil {
		return 0, fmt.Errorf("store: create margin option %q: %w", name, err)
	}
	return res.LastInsertId()
}

// UpdateMarginOption renames and/or revalues an option. Drafts on it follow the new
// value the next time they're priced; issued quotes keep their snapshot.
func (s *Store) UpdateMarginOption(ctx context.Context, id int64, name string, value money.Micros, updatedBy int64) error {
	res, err := s.exec(ctx, `
		UPDATE margin_options SET
			name = ?, value_micros = ?, updated_by = ?,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?`,
		name, int64(value), nullIfZero(updatedBy), id)
	if isUniqueViolation(err, "margin_options.name") {
		return ErrDuplicateMarginName
	}
	if err != nil {
		return fmt.Errorf("store: update margin option %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrMarginOptionNotFound
	}
	return nil
}

// SetDefaultMarginOption makes id the option new quotes start on, clearing the flag
// from whichever option had it, in one transaction.
func (s *Store) SetDefaultMarginOption(ctx context.Context, id int64, updatedBy int64) error {
	opt, err := s.MarginOptionByID(ctx, id)
	if err != nil {
		return err
	}
	if opt == nil {
		return ErrMarginOptionNotFound
	}
	if !opt.Active() {
		return ErrMarginOptionRetired
	}
	if opt.IsDefault {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: set default margin option %d: %w", id, err)
	}
	defer tx.Rollback()
	if err := stampActor(ctx, tx); err != nil {
		return fmt.Errorf("store: set default margin option %d: %w", id, err)
	}
	for _, stmt := range []string{
		`UPDATE margin_options SET is_default = 0, updated_by = ?,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		 WHERE is_default = 1 AND id != ?`,
		`UPDATE margin_options SET is_default = 1, updated_by = ?,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		 WHERE id = ? AND retired_at IS NULL`,
	} {
		if _, err := tx.ExecContext(ctx, stmt, nullIfZero(updatedBy), id); err != nil {
			return fmt.Errorf("store: set default margin option %d: %w", id, err)
		}
	}
	return tx.Commit()
}

// RetireMarginOption hides an option from the quote builder's dropdown. Drafts already
// on it keep pointing at it and can't be issued until another option is picked.
func (s *Store) RetireMarginOption(ctx context.Context, id int64, updatedBy int64) error {
	opt, err := s.MarginOptionByID(ctx, id)
	if err != nil {
		return err
	}
	if opt == nil {
		return ErrMarginOptionNotFound
	}
	if opt.IsDefault {
		return ErrMarginOptionIsDefault
	}
	_, err = s.exec(ctx, `
		UPDATE margin_options SET
			retired_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), updated_by = ?,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ? AND retired_at IS NULL AND is_default = 0`,
		nullIfZero(updatedBy), id)
	if err != nil {
		return fmt.Errorf("store: retire margin option %d: %w", id, err)
	}
	return nil
}

// RestoreMarginOption makes a retired option pickable again.
func (s *Store) RestoreMarginOption(ctx context.Context, id int64, updatedBy int64) error {
	res, err := s.exec(ctx, `
		UPDATE margin_options SET
			retired_at = NULL, updated_by = ?,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?`,
		nullIfZero(updatedBy), id)
	if err != nil {
		return fmt.Errorf("store: restore margin option %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrMarginOptionNotFound
	}
	return nil
}
