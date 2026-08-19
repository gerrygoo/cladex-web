package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/pricing"
)

// Quote is a quotes row. CustomerName is populated by the CRUD read paths (a join),
// left zero elsewhere. See migrations/0001_init.sql.
type Quote struct {
	ID           int64
	Folio        string
	Prefix       string
	CustomerID   int64
	CustomerName string
	UserID       int64
	Status       string // borrador | emitida | revisada
	Currency     string
	Subtotal     money.Centavos
	IVA          money.Centavos
	Total        money.Centavos
	CreatedAt    string
}

// QuoteLine is a quote_lines row. ProductID is nil for a free-text ("Cotizador libre")
// line. PricingInputsJSON is nil for free-text lines, which have no formula behind
// their price. See migrations/0001_init.sql.
type QuoteLine struct {
	ID                  int64
	QuoteID             int64
	LineNo              int
	ProductID           *int64
	DescriptionSnapshot string
	QtyMilli            money.Milli
	UnitPriceMicros     money.Micros
	LineTotal           money.Centavos
	PricingInputsJSON   *string
	Source              string // manual | rfp_extraction
}

// NextFolio hands out the next folio number for prefix (QA/QS/QI) as
// "<prefix><4-digit zero-padded number>" (e.g. "QA0001"), via folio_sequences
// (migrations/0004_add_folio_sequences.sql). The upsert-and-RETURNING is one atomic
// statement, so concurrent callers can never receive the same folio.
func (s *Store) NextFolio(ctx context.Context, prefix string) (string, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO folio_sequences (prefix, next_number) VALUES (?, 2)
		ON CONFLICT (prefix) DO UPDATE SET next_number = next_number + 1
		RETURNING next_number - 1`, prefix,
	).Scan(&n)
	if err != nil {
		return "", fmt.Errorf("store: next folio for %q: %w", prefix, err)
	}
	return fmt.Sprintf("%s%04d", prefix, n), nil
}

// CreateDraftQuote assigns a folio and inserts a new draft quote (status 'borrador',
// no lines yet). Folio assignment and the quote insert are two statements, not one
// transaction: per docs/PLAN.md, a gap in the folio sequence (from a failed insert
// after a folio was already handed out) is acceptable; a duplicate folio is not, and
// NextFolio alone already rules that out.
func (s *Store) CreateDraftQuote(ctx context.Context, customerID, userID int64, prefix string) (*Quote, error) {
	folio, err := s.NextFolio(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("store: create draft quote: %w", err)
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO quotes (folio, prefix, customer_id, user_id, status, currency)
		VALUES (?, ?, ?, ?, 'borrador', 'MXN')`,
		folio, prefix, customerID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: create draft quote: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("store: create draft quote: %w", err)
	}
	return s.QuoteByID(ctx, id)
}

const quoteSelectCols = `
	q.id, q.folio, q.prefix, q.customer_id, c.name, q.user_id, q.status, q.currency,
	q.subtotal, q.iva, q.total, q.created_at`

const quoteFrom = `
	FROM quotes q
	JOIN customers c ON c.id = q.customer_id`

func scanQuote(row interface{ Scan(...any) error }) (*Quote, error) {
	var q Quote
	err := row.Scan(&q.ID, &q.Folio, &q.Prefix, &q.CustomerID, &q.CustomerName, &q.UserID,
		&q.Status, &q.Currency, &q.Subtotal, &q.IVA, &q.Total, &q.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &q, nil
}

// QuoteByID returns the quote with the given id, or nil if none exists.
func (s *Store) QuoteByID(ctx context.Context, id int64) (*Quote, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+quoteSelectCols+quoteFrom+` WHERE q.id = ?`, id)
	q, err := scanQuote(row)
	if err != nil {
		return nil, fmt.Errorf("store: quote by id %d: %w", id, err)
	}
	return q, nil
}

// QuoteByFolio returns the quote with the given folio, or nil if none exists.
func (s *Store) QuoteByFolio(ctx context.Context, folio string) (*Quote, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+quoteSelectCols+quoteFrom+` WHERE q.folio = ?`, folio)
	q, err := scanQuote(row)
	if err != nil {
		return nil, fmt.Errorf("store: quote by folio %q: %w", folio, err)
	}
	return q, nil
}

// quoteSortColumns is the sortable-column whitelist for ListQuotes; the first entry
// (folio) is the default when sort doesn't match a known column.
var quoteSortColumns = []sortColumn{
	{"folio", "q.folio"},
	{"cliente", "c.name"},
	{"estado", "q.status"},
	{"total", "q.total"},
	{"fecha", "q.created_at"},
}

// ListQuotes returns quotes, optionally filtered by a case-insensitive substring match
// on folio or customer name, sorted per sort/dir (see quoteSortColumns).
func (s *Store) ListQuotes(ctx context.Context, query, sort, dir string) ([]Quote, error) {
	like := "%" + escapeLike(query) + "%"
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+quoteSelectCols+`
		`+quoteFrom+`
		WHERE (? = '' OR q.folio LIKE ? ESCAPE '\' COLLATE NOCASE OR c.name LIKE ? ESCAPE '\' COLLATE NOCASE)
		`+orderByClause(quoteSortColumns, sort, dir), query, like, like,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list quotes: %w", err)
	}
	defer rows.Close()

	var quotes []Quote
	for rows.Next() {
		q, err := scanQuote(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list quotes: %w", err)
		}
		quotes = append(quotes, *q)
	}
	return quotes, rows.Err()
}

const quoteLineSelectCols = `
	id, quote_id, line_no, product_id, description_snapshot, qty_milli,
	unit_price_micros, line_total, pricing_inputs, source`

func scanQuoteLine(row interface{ Scan(...any) error }) (*QuoteLine, error) {
	var l QuoteLine
	var productID sql.NullInt64
	var pricingInputs sql.NullString
	err := row.Scan(&l.ID, &l.QuoteID, &l.LineNo, &productID, &l.DescriptionSnapshot,
		&l.QtyMilli, &l.UnitPriceMicros, &l.LineTotal, &pricingInputs, &l.Source)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if productID.Valid {
		id := productID.Int64
		l.ProductID = &id
	}
	if pricingInputs.Valid {
		v := pricingInputs.String
		l.PricingInputsJSON = &v
	}
	return &l, nil
}

// ListQuoteLines returns a quote's lines ordered by line_no.
func (s *Store) ListQuoteLines(ctx context.Context, quoteID int64) ([]QuoteLine, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+quoteLineSelectCols+`
		FROM quote_lines
		WHERE quote_id = ?
		ORDER BY line_no`, quoteID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list quote lines for quote %d: %w", quoteID, err)
	}
	defer rows.Close()

	var lines []QuoteLine
	for rows.Next() {
		l, err := scanQuoteLine(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list quote lines for quote %d: %w", quoteID, err)
		}
		lines = append(lines, *l)
	}
	return lines, rows.Err()
}

// ReplaceQuoteLines atomically replaces every line of a draft quote with lines, and
// updates the quote's stored totals to match — the one and only write quote_lines gets
// per "Guardar borrador" click, not per edit (see docs/PLAN.md's quote persistence
// design and the M2.2 slice notes). line_no is assigned from the slice order (1-based),
// not from any LineNo already set on the input.
func (s *Store) ReplaceQuoteLines(ctx context.Context, quoteID int64, lines []QuoteLine, totals pricing.Totals) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: replace quote lines for quote %d: %w", quoteID, err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM quote_lines WHERE quote_id = ?`, quoteID); err != nil {
		return fmt.Errorf("store: replace quote lines for quote %d: delete: %w", quoteID, err)
	}

	for i, l := range lines {
		var productID any
		if l.ProductID != nil {
			productID = *l.ProductID
		}
		var pricingInputs any
		if l.PricingInputsJSON != nil {
			pricingInputs = *l.PricingInputsJSON
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO quote_lines (
				quote_id, line_no, product_id, description_snapshot, qty_milli,
				unit_price_micros, line_total, pricing_inputs, source
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			quoteID, i+1, productID, l.DescriptionSnapshot, int64(l.QtyMilli),
			int64(l.UnitPriceMicros), int64(l.LineTotal), pricingInputs, l.Source,
		); err != nil {
			return fmt.Errorf("store: replace quote lines for quote %d: insert line %d: %w", quoteID, i+1, err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE quotes SET subtotal = ?, iva = ?, total = ? WHERE id = ?`,
		int64(totals.Subtotal), int64(totals.IVA), int64(totals.Total), quoteID,
	); err != nil {
		return fmt.Errorf("store: replace quote lines for quote %d: update totals: %w", quoteID, err)
	}

	return tx.Commit()
}
