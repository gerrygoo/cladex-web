package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/pricing"
)

// Quote is a quotes row. CustomerName, UserName, and SupersededByFolio are populated by
// the CRUD read paths (joins, so always the current names), left zero elsewhere.
// TermsSnapshot, IssuedAt, ValidUntil, CustomerNameSnapshot,
// VendedorSnapshot, and PDFSHA256 are nil until IssueQuote freezes the row.
// SupersedesQuoteID is set only on a revision (a quote created by CreateRevision).
// MarginOptionID is the margin a draft is priced with, read live (drafts follow edits to
// the option); MarginNameSnapshot and MarginSnapshotMicros freeze it at issue. See
// migrations/0001_init.sql and 0007_margin_options.sql.
type Quote struct {
	ID                   int64
	Folio                string
	Prefix               string
	CustomerID           int64
	CustomerName         string
	UserID               int64
	UserName             string
	Status               string // borrador | emitida | revisada
	Subtotal             money.Centavos
	IVA                  money.Centavos
	Total                money.Centavos
	TermsSnapshot        *string
	CreatedAt            string
	IssuedAt             *string
	ValidUntil           *string
	SupersedesQuoteID    *int64
	SupersedesFolio      string  // folio of the quote this one revises, if this is a revision
	SupersededByFolio    string  // folio of the revision that supersedes this quote, if any
	CustomerNameSnapshot *string // customer name as printed on the issued PDF
	VendedorSnapshot     *string // salesperson name as printed on the issued PDF
	PDFSHA256            *string // hash of the PDF as issued; reprints are compared to it
	MarginOptionID       *int64
	MarginNameSnapshot   *string
	MarginSnapshotMicros *money.Micros
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
// (migrations/0005_add_folio_sequences.sql). The upsert-and-RETURNING is one atomic
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
// no lines yet), on the default margin option. Folio assignment and the quote insert are two statements, not one
// transaction: per docs/PLAN.md, a gap in the folio sequence (from a failed insert
// after a folio was already handed out) is acceptable; a duplicate folio is not, and
// NextFolio alone already rules that out.
func (s *Store) CreateDraftQuote(ctx context.Context, customerID, userID int64, prefix string) (*Quote, error) {
	folio, err := s.NextFolio(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("store: create draft quote: %w", err)
	}
	res, err := s.exec(ctx, `
		INSERT INTO quotes (folio, prefix, customer_id, user_id, status, margin_option_id)
		VALUES (?, ?, ?, ?, 'borrador',
			(SELECT id FROM margin_options WHERE is_default = 1 AND retired_at IS NULL))`,
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
	q.id, q.folio, q.prefix, q.customer_id, c.name, q.user_id, u.name, q.status,
	q.subtotal, q.iva, q.total, q.terms_snapshot, q.created_at,
	q.issued_at, q.valid_until, q.supersedes_quote_id, q.customer_name_snapshot,
	q.vendedor_snapshot, q.pdf_sha256, q.margin_option_id, q.margin_name_snapshot,
	q.margin_snapshot_micros,
	(SELECT o.folio FROM quotes o WHERE o.id = q.supersedes_quote_id),
	(SELECT r.folio FROM quotes r WHERE r.supersedes_quote_id = q.id)`

const quoteFrom = `
	FROM quotes q
	JOIN customers c ON c.id = q.customer_id
	JOIN users u ON u.id = q.user_id`

func scanQuote(row interface{ Scan(...any) error }) (*Quote, error) {
	var q Quote
	var termsSnapshot, issuedAt, validUntil, customerNameSnapshot, vendedorSnapshot, pdfSHA256 sql.NullString
	var supersedesFolio, supersededByFolio sql.NullString
	var supersedesQuoteID, marginOptionID, marginSnapshotMicros sql.NullInt64
	var marginNameSnapshot sql.NullString
	err := row.Scan(&q.ID, &q.Folio, &q.Prefix, &q.CustomerID, &q.CustomerName, &q.UserID, &q.UserName,
		&q.Status, &q.Subtotal, &q.IVA, &q.Total, &termsSnapshot, &q.CreatedAt,
		&issuedAt, &validUntil, &supersedesQuoteID, &customerNameSnapshot, &vendedorSnapshot, &pdfSHA256,
		&marginOptionID, &marginNameSnapshot, &marginSnapshotMicros,
		&supersedesFolio, &supersededByFolio)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if termsSnapshot.Valid {
		q.TermsSnapshot = &termsSnapshot.String
	}
	if issuedAt.Valid {
		q.IssuedAt = &issuedAt.String
	}
	if validUntil.Valid {
		q.ValidUntil = &validUntil.String
	}
	if supersedesQuoteID.Valid {
		q.SupersedesQuoteID = &supersedesQuoteID.Int64
	}
	if customerNameSnapshot.Valid {
		q.CustomerNameSnapshot = &customerNameSnapshot.String
	}
	if vendedorSnapshot.Valid {
		q.VendedorSnapshot = &vendedorSnapshot.String
	}
	if pdfSHA256.Valid {
		q.PDFSHA256 = &pdfSHA256.String
	}
	if marginOptionID.Valid {
		q.MarginOptionID = &marginOptionID.Int64
	}
	if marginNameSnapshot.Valid {
		q.MarginNameSnapshot = &marginNameSnapshot.String
	}
	if marginSnapshotMicros.Valid {
		m := money.Micros(marginSnapshotMicros.Int64)
		q.MarginSnapshotMicros = &m
	}
	q.SupersedesFolio = supersedesFolio.String
	q.SupersededByFolio = supersededByFolio.String
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

// quoteSortColumns is the sortable-column whitelist for ListQuotes. When sort doesn't
// match a known column, ListQuotes uses quoteDefaultOrder instead.
var quoteSortColumns = []sortColumn{
	{"folio", "q.folio"},
	{"cliente", "c.name"},
	{"autor", "u.name COLLATE NOCASE"},
	{"estado", "q.status"},
	{"total", "q.total"},
	{"fecha", "q.created_at"},
}

// quoteDefaultOrder is the list's order when no sort column is chosen: newest first,
// with the QI (alumbrado) family after all the others.
const quoteDefaultOrder = `ORDER BY (q.prefix = 'QI') ASC, q.created_at DESC, q.id DESC`

// ListQuotes returns quotes, optionally filtered by a case-insensitive substring match
// on folio or customer name, sorted per sort/dir (see quoteSortColumns).
func (s *Store) ListQuotes(ctx context.Context, query, sort, dir string) ([]Quote, error) {
	like := "%" + escapeLike(query) + "%"
	order := quoteDefaultOrder
	for _, c := range quoteSortColumns {
		if c.name == sort {
			order = orderByClause(quoteSortColumns, sort, dir)
			break
		}
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+quoteSelectCols+`
		`+quoteFrom+`
		WHERE (? = '' OR q.folio LIKE ? ESCAPE '\' COLLATE NOCASE OR c.name LIKE ? ESCAPE '\' COLLATE NOCASE)
		`+order, query, like, like,
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
// updates the quote's stored totals (and, when marginOptionID is non-nil, its margin
// option) to match — the one and only write quote_lines gets
// per "Guardar borrador" click, not per edit (see docs/PLAN.md's quote persistence
// design and the M2.2 slice notes). line_no is assigned from the slice order (1-based),
// not from any LineNo already set on the input. quote_lines isn't an audited table
// (see migrations/0004_audit_log.sql), but quotes is, so the totals UPDATE below stamps
// the actor first via stampActor — this method can't just call s.exec for it, since
// that write has to share this transaction with the quote_lines delete+reinsert.
func (s *Store) ReplaceQuoteLines(ctx context.Context, quoteID int64, marginOptionID *int64, lines []QuoteLine, totals pricing.Totals) error {
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

	if err := stampActor(ctx, tx); err != nil {
		return fmt.Errorf("store: replace quote lines for quote %d: %w", quoteID, err)
	}
	var marginArg any
	if marginOptionID != nil {
		marginArg = *marginOptionID
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE quotes SET subtotal = ?, iva = ?, total = ?,
			margin_option_id = COALESCE(?, margin_option_id)
		WHERE id = ?`,
		int64(totals.Subtotal), int64(totals.IVA), int64(totals.Total), marginArg, quoteID,
	); err != nil {
		return fmt.Errorf("store: replace quote lines for quote %d: update totals: %w", quoteID, err)
	}

	return tx.Commit()
}

// ErrQuoteNotDraft is returned by IssueQuote when the target quote isn't currently
// 'borrador' — a quote is issued exactly once; see CreateRevision for changing an
// already-issued quote afterward.
var ErrQuoteNotDraft = errors.New("store: quote is not a draft")

// Issue is everything IssueQuote freezes onto a draft. IssuedAt is supplied by the
// caller rather than taken from SQLite's clock because the PDF (whose hash is
// PDFSHA256) is rendered before the row is written, and it prints and embeds
// issued_at; see Quotes.Emitir.
type Issue struct {
	IssuedAt             string // ISO-8601 UTC, same shape as strftime('%Y-%m-%dT%H:%M:%fZ')
	TermsSnapshot        string
	ValidUntil           *string
	CustomerNameSnapshot string
	VendedorSnapshot     string
	MarginName           string       // the quote's margin option, frozen by name
	MarginMicros         money.Micros // and by value
	PDFSHA256            string
}

// IssueQuote freezes a draft quote: locks the margin, terms text, and the
// customer and salesperson names actually used, records the rendered PDF's SHA-256, sets an
// optional expiry, and flips status to 'emitida'. The PDF itself is not stored; it
// is regenerated from this frozen row on every request. Only succeeds against a
// quote currently 'borrador' (checked and enforced in the same statement, so two
// concurrent issue attempts can't both succeed) — the caller is expected to have
// already saved the final line set (e.g. via ReplaceQuoteLines) before calling this.
func (s *Store) IssueQuote(ctx context.Context, quoteID int64, is Issue) error {
	var validUntilArg any
	if is.ValidUntil != nil {
		validUntilArg = *is.ValidUntil
	}
	res, err := s.exec(ctx, `
		UPDATE quotes SET
			status = 'emitida',
			issued_at = ?,
			terms_snapshot = ?,
			valid_until = ?,
			customer_name_snapshot = ?,
			vendedor_snapshot = ?,
			margin_name_snapshot = ?,
			margin_snapshot_micros = ?,
			pdf_sha256 = ?
		WHERE id = ? AND status = 'borrador'`,
		is.IssuedAt, is.TermsSnapshot, validUntilArg,
		is.CustomerNameSnapshot, is.VendedorSnapshot, is.MarginName, int64(is.MarginMicros),
		is.PDFSHA256, quoteID,
	)
	if err != nil {
		return fmt.Errorf("store: issue quote %d: %w", quoteID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: issue quote %d: %w", quoteID, err)
	}
	if n != 1 {
		return ErrQuoteNotDraft
	}
	return nil
}

// ErrQuoteNotIssued is returned by CreateRevision when the target quote isn't
// currently 'emitida' — only the active issued version of a quote lineage can be
// revised; a superseded ('revisada') quote must be revised via its own successor.
var ErrQuoteNotIssued = errors.New("store: quote is not issued")

// revisionSuffixRe matches a folio's "-R<n>" revision suffix, if present. Folios are
// always machine-generated by NextFolio/CreateRevision, never user-typed, so this
// pattern (and the plain LIKE below) never needs to defend against adversarial input.
var revisionSuffixRe = regexp.MustCompile(`^(.+)-R(\d+)$`)

// baseFolio strips a "-R<n>" suffix, if present, returning the root folio a whole
// revision chain shares (e.g. "QA0105-R2" -> "QA0105").
func baseFolio(folio string) string {
	if m := revisionSuffixRe.FindStringSubmatch(folio); m != nil {
		return m[1]
	}
	return folio
}

// nextRevisionNumber finds the highest existing "<base>-R<n>" folio and returns n+1,
// or 1 if base has no revisions yet.
func nextRevisionNumber(ctx context.Context, tx *sql.Tx, base string) (int, error) {
	var lastFolio string
	err := tx.QueryRowContext(ctx, `
		SELECT folio FROM quotes WHERE folio LIKE ? ORDER BY id DESC LIMIT 1`,
		base+"-R%",
	).Scan(&lastFolio)
	if err == sql.ErrNoRows {
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	m := revisionSuffixRe.FindStringSubmatch(lastFolio)
	if m == nil {
		return 1, nil
	}
	n, err := strconv.Atoi(m[2])
	if err != nil {
		return 1, nil
	}
	return n + 1, nil
}

// CreateRevision makes an editable copy of an issued quote as a new draft: a new
// folio (<base>-R<n>, e.g. QA0105-R1, or QA0105-R2 if revising a quote that's already
// a revision), sharing customer/prefix/margin option and starting from the original's
// lines — and marks the original 'revisada'. Nothing about the original's own row is
// changed beyond that one status flip: its lines, totals, and snapshots (and so its PDF)
// stay exactly as issued, per docs/PLAN.md's "original untouched" revision design. Only the
// currently-active issued quote in a lineage can be revised (see ErrQuoteNotIssued) —
// revise the latest revision, not a superseded one. Stamps the actor once, via
// stampActor, before this transaction's first write to quotes (an audited table) —
// one stamp covers all three of this method's quotes writes (the new draft's INSERT,
// its totals UPDATE, and the original's status UPDATE), since nothing else can write
// to audit_actor while this transaction holds the write lock.
func (s *Store) CreateRevision(ctx context.Context, originalID, userID int64) (*Quote, error) {
	original, err := s.QuoteByID(ctx, originalID)
	if err != nil {
		return nil, fmt.Errorf("store: create revision of quote %d: %w", originalID, err)
	}
	if original == nil {
		return nil, nil
	}
	if original.Status != "emitida" {
		return nil, ErrQuoteNotIssued
	}
	lines, err := s.ListQuoteLines(ctx, originalID)
	if err != nil {
		return nil, fmt.Errorf("store: create revision of quote %d: %w", originalID, err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store: create revision of quote %d: %w", originalID, err)
	}
	defer tx.Rollback()

	base := baseFolio(original.Folio)
	n, err := nextRevisionNumber(ctx, tx, base)
	if err != nil {
		return nil, fmt.Errorf("store: create revision of quote %d: %w", originalID, err)
	}
	newFolio := fmt.Sprintf("%s-R%d", base, n)

	if err := stampActor(ctx, tx); err != nil {
		return nil, fmt.Errorf("store: create revision of quote %d: %w", originalID, err)
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO quotes (folio, prefix, customer_id, user_id, status, supersedes_quote_id, margin_option_id)
		VALUES (?, ?, ?, ?, 'borrador', ?, ?)`,
		newFolio, original.Prefix, original.CustomerID, userID, originalID,
		original.MarginOptionID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: create revision of quote %d: insert: %w", originalID, err)
	}
	newID, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("store: create revision of quote %d: %w", originalID, err)
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
			newID, i+1, productID, l.DescriptionSnapshot, int64(l.QtyMilli),
			int64(l.UnitPriceMicros), int64(l.LineTotal), pricingInputs, l.Source,
		); err != nil {
			return nil, fmt.Errorf("store: create revision of quote %d: copy line %d: %w", originalID, i+1, err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE quotes SET subtotal = ?, iva = ?, total = ? WHERE id = ?`,
		int64(original.Subtotal), int64(original.IVA), int64(original.Total), newID,
	); err != nil {
		return nil, fmt.Errorf("store: create revision of quote %d: totals: %w", originalID, err)
	}

	res, err = tx.ExecContext(ctx, `
		UPDATE quotes SET status = 'revisada' WHERE id = ? AND status = 'emitida'`, originalID)
	if err != nil {
		return nil, fmt.Errorf("store: create revision of quote %d: mark superseded: %w", originalID, err)
	}
	if affected, _ := res.RowsAffected(); affected != 1 {
		return nil, ErrQuoteNotIssued
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: create revision of quote %d: %w", originalID, err)
	}
	return s.QuoteByID(ctx, newID)
}
