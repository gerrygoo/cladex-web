package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/gerrygoo/cladex-web/internal/money"
)

// PriceList is a price_lists row without its PDF (migrations/0049_price_lists.sql).
type PriceList struct {
	ID           int64
	QuoteTypeID  int64
	TypePrefix   string
	FamilyName   string
	LeadID       *int64
	LeadName     string
	Recipient    string
	UserID       int64
	UserName     string
	MarginName   string
	MarginMicros money.Micros
	ValidUntil   string
	Lines        int
	CreatedAt    string
}

// NewPriceList is a generated price list to keep.
type NewPriceList struct {
	QuoteTypeID  int64
	FamilyName   string
	LeadID       *int64
	Recipient    string
	UserID       int64
	MarginName   string
	MarginMicros money.Micros
	ValidUntil   string
	Lines        int
	PDF          []byte
	// Note is the entry the lead's history gets, when LeadID is set.
	Note string
}

// CreatePriceList keeps a generated price list and, when it's for a lead, writes Note to
// the lead's history. It returns the list's id.
func (s *Store) CreatePriceList(ctx context.Context, pl NewPriceList) (int64, error) {
	var id int64
	err := s.inTx(ctx, "create price list", func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO price_lists (quote_type_id, family_name, lead_id, recipient, user_id,
				margin_name, margin_micros, valid_until, lines, pdf)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			pl.QuoteTypeID, pl.FamilyName, pl.LeadID, pl.Recipient, pl.UserID,
			pl.MarginName, int64(pl.MarginMicros), pl.ValidUntil, pl.Lines, pl.PDF)
		if err != nil {
			return err
		}
		if id, err = res.LastInsertId(); err != nil {
			return err
		}
		if pl.LeadID != nil && pl.Note != "" {
			return addLeadComment(ctx, tx, *pl.LeadID, pl.UserID, pl.Note)
		}
		return nil
	})
	return id, err
}

const priceListSelect = `
	SELECT pl.id, pl.quote_type_id, t.prefix, pl.family_name, pl.lead_id, COALESCE(l.contact_name, ''),
		pl.recipient, pl.user_id, u.name, pl.margin_name, pl.margin_micros, pl.valid_until,
		pl.lines, pl.created_at
	FROM price_lists pl
	JOIN quote_types t ON t.id = pl.quote_type_id
	JOIN users u ON u.id = pl.user_id
	LEFT JOIN leads l ON l.id = pl.lead_id`

func scanPriceList(row interface{ Scan(...any) error }) (*PriceList, error) {
	var p PriceList
	var leadID sql.NullInt64
	if err := row.Scan(&p.ID, &p.QuoteTypeID, &p.TypePrefix, &p.FamilyName, &leadID, &p.LeadName,
		&p.Recipient, &p.UserID, &p.UserName, &p.MarginName, &p.MarginMicros, &p.ValidUntil,
		&p.Lines, &p.CreatedAt); err != nil {
		return nil, err
	}
	if leadID.Valid {
		p.LeadID = &leadID.Int64
	}
	return &p, nil
}

// ListPriceLists returns the price lists made for lead leadID (0: every one), newest
// first.
func (s *Store) ListPriceLists(ctx context.Context, leadID int64) ([]PriceList, error) {
	rows, err := s.db.QueryContext(ctx, priceListSelect+`
		WHERE ? = 0 OR pl.lead_id = ? ORDER BY pl.created_at DESC, pl.id DESC`, leadID, leadID)
	if err != nil {
		return nil, fmt.Errorf("store: list price lists: %w", err)
	}
	defer rows.Close()
	var out []PriceList
	for rows.Next() {
		p, err := scanPriceList(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list price lists: %w", err)
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// PriceListByID returns the price list, or nil.
func (s *Store) PriceListByID(ctx context.Context, id int64) (*PriceList, error) {
	p, err := scanPriceList(s.db.QueryRowContext(ctx, priceListSelect+` WHERE pl.id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: price list %d: %w", id, err)
	}
	return p, nil
}

// PriceListPDF returns the price list's PDF, nil when there's no such list.
func (s *Store) PriceListPDF(ctx context.Context, id int64) ([]byte, error) {
	var data []byte
	err := s.db.QueryRowContext(ctx, `SELECT pdf FROM price_lists WHERE id = ?`, id).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: price list %d pdf: %w", id, err)
	}
	return data, nil
}
