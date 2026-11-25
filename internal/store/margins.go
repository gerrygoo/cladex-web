package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/gerrygoo/cladex-web/internal/money"
)

// MarginOption is a margin_options row: one named margin a quote can be priced with.
// ValueMicros is a fraction of the sale price (123_400 == 12.34%). RetiredAt is set once
// an admin retires it; retired options stay in the table because issued quotes and
// audit_log name them. QuoteTypeID is the quote type the option belongs to, nil for the
// generales, which every type without options of its own uses (#82; see
// migrations/0007_margin_options.sql and 0047_margins_per_quote_type.sql).
type MarginOption struct {
	ID          int64
	QuoteTypeID *int64
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
	// ErrHasOwnMargins is returned by UseOwnMargins for a type that already has them.
	ErrHasOwnMargins = errors.New("store: quote type already has its own margins")
)

const marginOptionCols = `id, quote_type_id, name, value_micros, is_default, retired_at`

func scanMarginOption(row interface{ Scan(...any) error }) (*MarginOption, error) {
	var o MarginOption
	var retiredAt sql.NullString
	var typeID sql.NullInt64
	if err := row.Scan(&o.ID, &typeID, &o.Name, &o.ValueMicros, &o.IsDefault, &retiredAt); err != nil {
		return nil, err
	}
	if typeID.Valid {
		o.QuoteTypeID = &typeID.Int64
	}
	if retiredAt.Valid {
		o.RetiredAt = &retiredAt.String
	}
	return &o, nil
}

// marginScopeSQL is the quote_type_id (0 for the generales) whose options a quote of the
// type bound to its two ? placeholders uses: its own when it has any, else the generales.
const marginScopeSQL = `(SELECT CASE WHEN EXISTS (SELECT 1 FROM margin_options WHERE quote_type_id = ?) THEN ? ELSE 0 END)`

// draftMarginSQL picks the option a new draft of a type starts on, from the option it
// was copied from (a revision or a duplicate): the active option with the same name
// among the type's margins, else the type's default. Its args are draftMarginArgs.
const draftMarginSQL = `COALESCE(
	(SELECT m.id FROM margin_options m, margin_options o
		WHERE o.id = ? AND m.name = o.name AND m.retired_at IS NULL
		  AND COALESCE(m.quote_type_id, 0) = ` + marginScopeSQL + `),
	(SELECT id FROM margin_options WHERE is_default = 1 AND retired_at IS NULL
		AND COALESCE(quote_type_id, 0) = ` + marginScopeSQL + `))`

func draftMarginArgs(fromOptionID *int64, typeID int64) []any {
	var from any
	if fromOptionID != nil {
		from = *fromOptionID
	}
	return []any{from, typeID, typeID, typeID, typeID}
}

// ListMarginOptions returns the options of one scope, the generales when quoteTypeID is
// nil, active ones first, each group ordered by value: the order /ajustes shows them in.
func (s *Store) ListMarginOptions(ctx context.Context, quoteTypeID *int64) ([]MarginOption, error) {
	var scope int64
	if quoteTypeID != nil {
		scope = *quoteTypeID
	}
	return s.listMarginOptions(ctx, `COALESCE(quote_type_id, 0) = ?`, scope)
}

// QuoteMarginOptions returns the options a quote of type quoteTypeID is priced with,
// in the quote builder's order: the type's own when it has any, else the generales.
func (s *Store) QuoteMarginOptions(ctx context.Context, quoteTypeID int64) ([]MarginOption, error) {
	return s.listMarginOptions(ctx, `COALESCE(quote_type_id, 0) = `+marginScopeSQL, quoteTypeID, quoteTypeID)
}

// MarginTypeIDs lists the quote types that have margins of their own.
func (s *Store) MarginTypeIDs(ctx context.Context) (map[int64]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT quote_type_id FROM margin_options WHERE quote_type_id IS NOT NULL AND deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("store: margin type ids: %w", err)
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: margin type ids: %w", err)
		}
		out[id] = true
	}
	return out, rows.Err()
}

func (s *Store) listMarginOptions(ctx context.Context, where string, args ...any) ([]MarginOption, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+marginOptionCols+` FROM margin_options
		WHERE deleted_at IS NULL AND `+where+`
		ORDER BY retired_at IS NOT NULL, value_micros, id`, args...)
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

// CreateMarginOption inserts a new, active, non-default option of quoteTypeID's margins
// (nil: the generales) and returns its id.
func (s *Store) CreateMarginOption(ctx context.Context, quoteTypeID *int64, name string, value money.Micros, updatedBy int64) (int64, error) {
	res, err := s.exec(ctx, `
		INSERT INTO margin_options (quote_type_id, name, value_micros, updated_by) VALUES (?, ?, ?, ?)`,
		quoteTypeID, name, int64(value), nullIfZero(updatedBy))
	if isMarginNameViolation(err) {
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
		WHERE id = ? AND deleted_at IS NULL`,
		name, int64(value), nullIfZero(updatedBy), id)
	if isMarginNameViolation(err) {
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

// isMarginNameViolation reports whether err is a second option with the same name in one
// scope (idx_margin_options_name, an expression index SQLite reports as "index ...").
func isMarginNameViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") &&
		strings.Contains(err.Error(), "idx_margin_options_name")
}

// UseOwnMargins gives quote type typeID margins of its own: a copy of the active
// generales, same names, values and default. Its drafts move to the copy of the option
// they were on, so nothing they price changes.
func (s *Store) UseOwnMargins(ctx context.Context, typeID, updatedBy int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: own margins for type %d: %w", typeID, err)
	}
	defer tx.Rollback()
	if err := stampActor(ctx, tx); err != nil {
		return fmt.Errorf("store: own margins for type %d: %w", typeID, err)
	}
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM margin_options WHERE quote_type_id = ?`, typeID).Scan(&n); err != nil {
		return fmt.Errorf("store: own margins for type %d: %w", typeID, err)
	}
	if n > 0 {
		return ErrHasOwnMargins
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO margin_options (quote_type_id, name, value_micros, is_default, updated_by)
		SELECT ?, name, value_micros, is_default, ? FROM margin_options
		WHERE quote_type_id IS NULL AND retired_at IS NULL ORDER BY id`,
		typeID, nullIfZero(updatedBy)); err != nil {
		return fmt.Errorf("store: own margins for type %d: %w", typeID, err)
	}
	// A draft on a retired general option has no copy and stays on it, so the builder
	// still asks for another margin.
	if _, err := tx.ExecContext(ctx, `
		UPDATE quotes SET margin_option_id = (
			SELECT c.id FROM margin_options c, margin_options g
			WHERE g.id = quotes.margin_option_id AND c.quote_type_id = quotes.quote_type_id AND c.name = g.name)
		WHERE quote_type_id = ? AND status = 'borrador' AND EXISTS (
			SELECT 1 FROM margin_options c, margin_options g
			WHERE g.id = quotes.margin_option_id AND c.quote_type_id = quotes.quote_type_id AND c.name = g.name)`,
		typeID); err != nil {
		return fmt.Errorf("store: own margins for type %d: move drafts: %w", typeID, err)
	}
	return tx.Commit()
}

// SetDefaultMarginOption makes id the option new quotes start on, clearing the flag
// from whichever option had it in the same scope, in one transaction.
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
		`UPDATE margin_options SET is_default = 0, updated_by = ?1,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		 WHERE is_default = 1 AND id != ?2
		   AND COALESCE(quote_type_id, 0) = (SELECT COALESCE(quote_type_id, 0) FROM margin_options WHERE id = ?2)`,
		`UPDATE margin_options SET is_default = 1, updated_by = ?1,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		 WHERE id = ?2 AND retired_at IS NULL`,
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
		WHERE id = ? AND deleted_at IS NULL`,
		nullIfZero(updatedBy), id)
	if err != nil {
		return fmt.Errorf("store: restore margin option %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrMarginOptionNotFound
	}
	return nil
}

// ErrMarginOptionActive is returned by DeleteMarginOption for an option that isn't
// retired: only a retired one can be deleted.
var ErrMarginOptionActive = errors.New("store: only a retired margin option can be deleted")

// DeleteMarginOption takes a retired option out of Márgenes for good (#86). Quotes
// issued with it keep their frozen name and value; drafts still on it (refused at issue
// while it's retired) move to "Otro margen" with its value, as revisions and duplicates of
// quotes that used it start (see deletedMarginCarry). The row stays, marked deleted, so
// those quotes can still point at it. An active option is ErrMarginOptionActive.
func (s *Store) DeleteMarginOption(ctx context.Context, id int64, updatedBy int64) error {
	return s.inTx(ctx, "delete margin option", func(tx *sql.Tx) error {
		var retired sql.NullString
		var value money.Micros
		err := tx.QueryRowContext(ctx, `SELECT retired_at, value_micros FROM margin_options WHERE id = ? AND deleted_at IS NULL`, id).
			Scan(&retired, &value)
		if err == sql.ErrNoRows {
			return ErrMarginOptionNotFound
		}
		if err != nil {
			return err
		}
		if !retired.Valid {
			return ErrMarginOptionActive
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE quotes SET custom_margin_micros = ?
			WHERE margin_option_id = ? AND status = 'borrador' AND custom_margin_micros IS NULL
				AND custom_copper_price_micros IS NULL`, value, id); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE margin_options SET deleted_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), updated_by = ?,
				updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
			WHERE id = ?`, nullIfZero(updatedBy), id)
		return err
	})
}

// deletedMarginCarry is the custom margin a revision or duplicate of original starts on
// when original was priced with a margin option since deleted (#86) and its type has no
// active option of that name to take its place: the margin it used (frozen at issue, else
// the option's value), as "Otro margen". Nil otherwise, including when original already
// had a custom margin, which is copied as it is.
func (s *Store) deletedMarginCarry(ctx context.Context, original *Quote) (*money.Micros, error) {
	if original.MarginOptionID == nil || original.CustomMarginMicros != nil {
		return nil, nil
	}
	var value money.Micros
	err := s.db.QueryRowContext(ctx, `
		SELECT o.value_micros FROM margin_options o
		WHERE o.id = ?1 AND o.deleted_at IS NOT NULL
			AND NOT EXISTS (SELECT 1 FROM margin_options m
				WHERE m.name = o.name AND m.retired_at IS NULL AND m.deleted_at IS NULL
					AND COALESCE(m.quote_type_id, 0) = (SELECT CASE WHEN EXISTS
						(SELECT 1 FROM margin_options WHERE quote_type_id = ?2) THEN ?2 ELSE 0 END))`,
		*original.MarginOptionID, original.QuoteTypeID).Scan(&value)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: deleted margin of quote %d: %w", original.ID, err)
	}
	if original.MarginSnapshotMicros != nil {
		value = *original.MarginSnapshotMicros
	}
	return &value, nil
}
