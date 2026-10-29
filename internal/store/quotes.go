package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

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
// Quote statuses, as stored in quotes.status: a draft is issued exactly once, and an
// issued quote is superseded by its revision.
const (
	QuoteBorrador = "borrador"
	QuoteEmitida  = "emitida"
	QuoteRevisada = "revisada"
)

type Quote struct {
	ID    int64
	Folio string
	// Prefix is the prefix folio was printed with, a snapshot never joined on; the
	// quote's type is QuoteTypeID. FolioNumber is folio's number, shared by revisions.
	Prefix               string
	QuoteTypeID          int64
	FolioNumber          int64
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
	// CustomMarginMicros, when set, prices the draft instead of MarginOptionID's value.
	CustomMarginMicros *money.Micros
	// CustomCopperPriceMicros is the sale price per kg the custom margin was typed as,
	// when it was: the CCS 30% price on QS (issue #38), the products' own price per kg on
	// QG (issue #43). That price, not the margin, then prices the draft.
	CustomCopperPriceMicros *money.Micros
	// SeriesFamily is the product familia the quote's series belongs to, "" when the
	// series has no familia or the familia is free-lines-only: the builder's product
	// search defaults to it. SeriesTerms is that series' current terms block, one term
	// per line; an issued quote prints TermsSnapshot instead.
	SeriesFamily string
	SeriesTerms  string
	// DeliveryTime is what the salesperson typed for the series' {tiempo_de_entrega}
	// term; empty when the series doesn't ask for one or none is set yet.
	DeliveryTime string
	// Currency is MXN or USD once chosen, for a series whose terms hold {moneda}; a
	// quote is priced in it and the PDF names it. SeriesFreeLines marks a series whose
	// quotes hold only free lines: no catalog picker, and a validity date the salesperson
	// types (held in ValidUntil while the quote is a draft).
	Currency        string
	SeriesFreeLines bool
	// The quote type's behavior flags (migrations/0029_quote_types.sql). KgPriced: its
	// products are costed per kg and quoted per metre (issue #37), priced like a CCS
	// material, and its factura is in metros. CopperPriceMargin: "Otro margen" can be
	// typed as the CCS 30% price per kg (issue #38). HideAddress and Letterhead shape
	// the PDF's letterhead (issues #16, #47, #48, #51).
	KgPriced          bool
	CopperPriceMargin bool
	HideAddress       bool
	Letterhead        string
	// ExtraTerms are a QL draft's own terms, one per line (issue #27), which RenderTerms
	// adds after the series' payment term; issuing folds them into TermsSnapshot.
	ExtraTerms string
	// ProjectID is the proyecto the quote belongs to: nil on a first draft, set when it
	// is issued, and shared by its revisions. ProjectFolio, Stage and Probability are
	// that proyecto's folio, stage and probabilidad de cierre (joined, so always
	// current); they are zero without one.
	ProjectID    *int64
	ProjectFolio string
	Stage        string
	Probability  int
}

// Revisable reports whether the quote can be revised: it is the issued, current quote
// and its proyecto is still a prospecto. Once the client's purchase order is in, the
// quote it answers is locked.
func (q Quote) Revisable() bool {
	return q.Status == QuoteEmitida && (q.Stage == "" || q.Stage == "prospecto")
}

// QuoteLine is a quote_lines row. ProductID is nil for a free-text ("Cotizador libre")
// line. PricingInputsJSON is nil for free-text lines, which have no formula behind
// their price, except on QL, where it holds the line's own margin. CostMicros is a free
// line's MXN cost per unit, nil when none was given (always nil on catalog lines); it is
// never printed. See migrations/0001_init.sql and 0021_quote_line_cost.sql.
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
	CostMicros          *money.Micros
}

// costArg is a line's cost as a nullable column value.
func costArg(c *money.Micros) any {
	if c == nil {
		return nil
	}
	return int64(*c)
}

// NextFolio hands out the next folio of a quote type: the type's prefix as it is now and
// the folio's number, which FolioText joins into the folio. The counter is the type's own (migrations/0029_quote_types.sql), so it carries on
// across a prefix rename. The UPDATE-and-RETURNING is one atomic statement, so
// concurrent callers can never receive the same number.
func (s *Store) NextFolio(ctx context.Context, quoteTypeID int64) (prefix string, number int64, err error) {
	err = s.db.QueryRowContext(ctx, `
		UPDATE quote_types SET next_number = next_number + 1 WHERE id = ?
		RETURNING prefix, next_number - 1`, quoteTypeID,
	).Scan(&prefix, &number)
	if err != nil {
		return "", 0, fmt.Errorf("store: next folio for quote type %d: %w", quoteTypeID, err)
	}
	return prefix, number, nil
}

// FolioText is a folio as printed: the prefix and the number zero-padded to four
// digits ("QA0001").
func FolioText(prefix string, number int64) string {
	return fmt.Sprintf("%s%04d", prefix, number)
}

// ErrUnknownQuoteType means no quote type has the prefix a new quote asked for.
var ErrUnknownQuoteType = errors.New("store: no quote type has that prefix")

// QuoteTypeIDByPrefix returns the id of the quote type whose current prefix is prefix,
// or ErrUnknownQuoteType.
func (s *Store) QuoteTypeIDByPrefix(ctx context.Context, prefix string) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM quote_types WHERE prefix = ?`, prefix).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrUnknownQuoteType
	}
	if err != nil {
		return 0, fmt.Errorf("store: quote type %q: %w", prefix, err)
	}
	return id, nil
}

// CreateDraftQuote assigns a folio and inserts a new draft quote (status 'borrador',
// no lines yet), on the default margin option. Folio assignment and the quote insert are two statements, not one
// transaction: per docs/PLAN.md, a gap in the folio sequence (from a failed insert
// after a folio was already handed out) is acceptable; a duplicate folio is not, and
// NextFolio alone already rules that out.
func (s *Store) CreateDraftQuote(ctx context.Context, customerID, userID int64, prefix string) (*Quote, error) {
	typeID, err := s.QuoteTypeIDByPrefix(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("store: create draft quote: %w", err)
	}
	prefix, number, err := s.NextFolio(ctx, typeID)
	if err != nil {
		return nil, fmt.Errorf("store: create draft quote: %w", err)
	}
	folio := FolioText(prefix, number)
	res, err := s.exec(ctx, `
		INSERT INTO quotes (folio, prefix, quote_type_id, folio_number, customer_id, user_id, status, margin_option_id)
		VALUES (?, ?, ?, ?, ?, ?, 'borrador',
			(SELECT id FROM margin_options WHERE is_default = 1 AND retired_at IS NULL))`,
		folio, prefix, typeID, number, customerID, userID,
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
	q.id, q.folio, q.prefix, q.quote_type_id, q.folio_number, q.customer_id, c.name, q.user_id, u.name, q.status,
	q.subtotal, q.iva, q.total, q.terms_snapshot, q.created_at,
	q.issued_at, q.valid_until, q.supersedes_quote_id, q.customer_name_snapshot,
	q.vendedor_snapshot, q.pdf_sha256, q.margin_option_id, q.margin_name_snapshot,
	q.margin_snapshot_micros, q.custom_margin_micros, q.custom_copper_price_micros,
	(SELECT o.folio FROM quotes o WHERE o.id = q.supersedes_quote_id),
	(SELECT r.folio FROM quotes r WHERE r.supersedes_quote_id = q.id),
	CASE WHEN t.free_lines_only = 0 THEN f.name END, t.terms, COALESCE(q.delivery_time, ''),
	COALESCE(q.currency, ''), t.free_lines_only,
	t.kg_priced, t.copper_price_margin, t.hide_address, t.letterhead,
	q.project_id, COALESCE(p.folio, ''), COALESCE(p.status, ''), COALESCE(p.probability, 0),
	COALESCE(q.extra_terms, '')`

const quoteFrom = `
	FROM quotes q
	JOIN customers c ON c.id = q.customer_id
	JOIN users u ON u.id = q.user_id
	JOIN quote_types t ON t.id = q.quote_type_id
	LEFT JOIN product_families f ON f.id = t.family_id
	LEFT JOIN projects p ON p.id = q.project_id`

func scanQuote(row interface{ Scan(...any) error }) (*Quote, error) {
	var q Quote
	var termsSnapshot, issuedAt, validUntil, customerNameSnapshot, vendedorSnapshot, pdfSHA256 sql.NullString
	var supersedesFolio, supersededByFolio, seriesFamily sql.NullString
	var supersedesQuoteID, marginOptionID, marginSnapshotMicros, customMarginMicros, customCopperPrice, projectID sql.NullInt64
	var marginNameSnapshot sql.NullString
	err := row.Scan(&q.ID, &q.Folio, &q.Prefix, &q.QuoteTypeID, &q.FolioNumber, &q.CustomerID, &q.CustomerName, &q.UserID, &q.UserName,
		&q.Status, &q.Subtotal, &q.IVA, &q.Total, &termsSnapshot, &q.CreatedAt,
		&issuedAt, &validUntil, &supersedesQuoteID, &customerNameSnapshot, &vendedorSnapshot, &pdfSHA256,
		&marginOptionID, &marginNameSnapshot, &marginSnapshotMicros, &customMarginMicros, &customCopperPrice,
		&supersedesFolio, &supersededByFolio, &seriesFamily, &q.SeriesTerms, &q.DeliveryTime,
		&q.Currency, &q.SeriesFreeLines,
		&q.KgPriced, &q.CopperPriceMargin, &q.HideAddress, &q.Letterhead,
		&projectID, &q.ProjectFolio, &q.Stage, &q.Probability, &q.ExtraTerms)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	q.SeriesFamily = seriesFamily.String
	if projectID.Valid {
		q.ProjectID = &projectID.Int64
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
	if customMarginMicros.Valid {
		m := money.Micros(customMarginMicros.Int64)
		q.CustomMarginMicros = &m
	}
	if customCopperPrice.Valid {
		p := money.Micros(customCopperPrice.Int64)
		q.CustomCopperPriceMicros = &p
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
// with the types marked sort_last (QI, alumbrado) after all the others.
const quoteDefaultOrder = `ORDER BY t.sort_last ASC, q.created_at DESC, q.id DESC`

// quoteFilterColumns are the per-column filters ListQuotes accepts.
var quoteFilterColumns = []filterColumn{
	{name: "folio", expr: "q.folio", kind: FilterText},
	{name: "cliente", expr: "c.name", kind: FilterText},
	{name: "autor", expr: "u.name", kind: FilterText},
	{name: "estado", expr: "q.status", kind: FilterEnum},
	{name: "total", expr: "q.total", kind: FilterNumber, scale: 100},
	{name: "fecha", expr: "q.created_at", kind: FilterDate},
}

// MaxFilterChoices caps how many values a column filter dropdown lists.
const MaxFilterChoices = 200

// QuoteFilterChoices returns the customer and author names that appear on quotes, for
// the Cliente and Autor filter dropdowns. A column with more than MaxFilterChoices
// distinct names returns nil, and its filter stays a free-text box.
func (s *Store) QuoteFilterChoices(ctx context.Context) (customers, authors []string, err error) {
	if customers, err = s.distinctQuoteNames(ctx, "c.name"); err != nil {
		return nil, nil, err
	}
	if authors, err = s.distinctQuoteNames(ctx, "u.name"); err != nil {
		return nil, nil, err
	}
	return customers, authors, nil
}

func (s *Store) distinctQuoteNames(ctx context.Context, expr string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT `+expr+` `+quoteFrom+`
		ORDER BY `+expr+` COLLATE NOCASE LIMIT ?`, MaxFilterChoices+1)
	if err != nil {
		return nil, fmt.Errorf("store: quote filter choices: %w", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("store: quote filter choices: %w", err)
		}
		names = append(names, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: quote filter choices: %w", err)
	}
	if len(names) > MaxFilterChoices {
		return nil, nil
	}
	return names, nil
}

// ListQuotes returns quotes, optionally filtered by a case-insensitive substring match
// on folio or customer name and by per-column filters (see quoteFilterColumns), sorted
// per sort/dir (see quoteSortColumns).
func (s *Store) ListQuotes(ctx context.Context, query, sort, dir string, filters Filters) ([]Quote, error) {
	like := "%" + escapeLike(query) + "%"
	order := quoteDefaultOrder
	for _, c := range quoteSortColumns {
		if c.name == sort {
			order = orderByClause(quoteSortColumns, sort, dir)
			break
		}
	}
	extra, extraArgs := filterWhere(ctx, quoteFilterColumns, filters)
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+quoteSelectCols+`
		`+quoteFrom+`
		WHERE (? = '' OR q.folio LIKE ? ESCAPE '\' COLLATE NOCASE OR c.name LIKE ? ESCAPE '\' COLLATE NOCASE)`+extra+`
		`+order, append([]any{query, like, like}, extraArgs...)...,
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
	unit_price_micros, line_total, pricing_inputs, source, cost_micros`

func scanQuoteLine(row interface{ Scan(...any) error }) (*QuoteLine, error) {
	var l QuoteLine
	var productID sql.NullInt64
	var pricingInputs sql.NullString
	var cost sql.NullInt64
	err := row.Scan(&l.ID, &l.QuoteID, &l.LineNo, &productID, &l.DescriptionSnapshot,
		&l.QtyMilli, &l.UnitPriceMicros, &l.LineTotal, &pricingInputs, &l.Source, &cost)
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
	if cost.Valid {
		c := money.Micros(cost.Int64)
		l.CostMicros = &c
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
// option; customMargin, nil or not, always replaces the quote's custom margin) to match — the one and only write quote_lines gets
// per "Guardar borrador" click, not per edit (see docs/PLAN.md's quote persistence
// design and the M2.2 slice notes). line_no is assigned from the slice order (1-based),
// not from any LineNo already set on the input. quote_lines isn't an audited table
// (see migrations/0004_audit_log.sql), but quotes is, so the totals UPDATE below stamps
// the actor first via stampActor — this method can't just call s.exec for it, since
// that write has to share this transaction with the quote_lines delete+reinsert.
func (s *Store) ReplaceQuoteLines(ctx context.Context, quoteID int64, marginOptionID *int64, customMargin *CustomMargin, lines []QuoteLine, totals pricing.Totals) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: replace quote lines for quote %d: %w", quoteID, err)
	}
	defer tx.Rollback()
	if err := replaceQuoteLines(ctx, tx, quoteID, marginOptionID, customMargin, lines, totals); err != nil {
		return err
	}
	return tx.Commit()
}

// replaceQuoteLines is ReplaceQuoteLines inside the caller's transaction.
func replaceQuoteLines(ctx context.Context, tx *sql.Tx, quoteID int64, marginOptionID *int64, customMargin *CustomMargin, lines []QuoteLine, totals pricing.Totals) error {
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
				unit_price_micros, line_total, pricing_inputs, source, cost_micros
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			quoteID, i+1, productID, l.DescriptionSnapshot, int64(l.QtyMilli),
			int64(l.UnitPriceMicros), int64(l.LineTotal), pricingInputs, l.Source, costArg(l.CostMicros),
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
	var customArg, copperArg any
	if customMargin != nil {
		customArg = int64(customMargin.Micros)
		if customMargin.CopperPrice != nil {
			copperArg = int64(*customMargin.CopperPrice)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE quotes SET subtotal = ?, iva = ?, total = ?,
			margin_option_id = COALESCE(?, margin_option_id),
			custom_margin_micros = ?, custom_copper_price_micros = ?
		WHERE id = ?`,
		int64(totals.Subtotal), int64(totals.IVA), int64(totals.Total), marginArg, customArg, copperArg, quoteID,
	); err != nil {
		return fmt.Errorf("store: replace quote lines for quote %d: update totals: %w", quoteID, err)
	}
	return nil
}

// CustomMargin is a draft's margin of its own ("Otro margen"): the fraction it prices
// with and, when it was typed as a price per kg (CCS 30% on QS, the products' own on
// QG), that exact price.
type CustomMargin struct {
	Micros      money.Micros
	CopperPrice *money.Micros
}

// SetQuoteTermsInputs saves what a draft's terms and series ask the salesperson for: the
// delivery time, the currency, and the validity date. Empty values clear the column.
func (s *Store) SetQuoteTermsInputs(ctx context.Context, quoteID int64, deliveryTime, currency, validUntil, extraTerms string) error {
	if _, err := s.exec(ctx, setTermsInputsSQL, deliveryTime, currency, validUntil, extraTerms, quoteID); err != nil {
		return fmt.Errorf("store: set terms inputs on quote %d: %w", quoteID, err)
	}
	return nil
}

const setTermsInputsSQL = `UPDATE quotes SET delivery_time = NULLIF(?, ''), currency = NULLIF(?, ''), valid_until = NULLIF(?, ''), extra_terms = NULLIF(?, '') WHERE id = ?`

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
	IssuedBy             int64 // who issues it, the author of a revision's cost-change note
}

// IssueQuote freezes a draft quote: locks the margin, terms text, and the
// customer and salesperson names actually used, records the rendered PDF's SHA-256, sets an
// optional expiry, and flips status to 'emitida'. The PDF itself is not stored; it
// is regenerated from this frozen row on every request. Only succeeds against a
// quote currently 'borrador' (checked and enforced in the same statement, so two
// concurrent issue attempts can't both succeed) — the caller is expected to have
// already saved the final line set (e.g. via ReplaceQuoteLines) before calling this.
//
// Issuing a first quote opens its proyecto, as a prospecto at the lowest probability,
// named after the quote's folio and owned by whoever created the quote. A revision
// already belongs to its original's proyecto and stays in it. Issuing a revision whose
// line costs differ from its original's adds a note saying how to the history (see
// costChangeNote).
func (s *Store) IssueQuote(ctx context.Context, quoteID int64, is Issue) error {
	costNote, err := s.revisionCostNote(ctx, quoteID, nil)
	if err != nil {
		return fmt.Errorf("store: issue quote %d: %w", quoteID, err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: issue quote %d: %w", quoteID, err)
	}
	defer tx.Rollback()
	if err := stampActor(ctx, tx); err != nil {
		return fmt.Errorf("store: issue quote %d: %w", quoteID, err)
	}
	if err := issueQuote(ctx, tx, quoteID, is, costNote); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: issue quote %d: %w", quoteID, err)
	}
	return nil
}

// DraftSave is a draft's final state as the builder submits it: what ReplaceQuoteLines
// and SetQuoteTermsInputs would each write.
type DraftSave struct {
	MarginOptionID *int64
	CustomMargin   *CustomMargin
	Lines          []QuoteLine
	Totals         pricing.Totals
	DeliveryTime   string
	Currency       string
	ValidUntil     string
	ExtraTerms     string
}

// SaveAndIssueQuote saves a draft's lines and terms inputs and issues it, all in one
// transaction, so a failure part-way leaves the draft as it was rather than saved but
// not issued. It returns ErrQuoteNotDraft, writing nothing, if the quote has been issued
// meanwhile.
func (s *Store) SaveAndIssueQuote(ctx context.Context, quoteID int64, save DraftSave, is Issue) error {
	costNote, err := s.revisionCostNote(ctx, quoteID, save.Lines)
	if err != nil {
		return fmt.Errorf("store: issue quote %d: %w", quoteID, err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: issue quote %d: %w", quoteID, err)
	}
	defer tx.Rollback()
	// replaceQuoteLines stamps the actor for the whole transaction.
	if err := replaceQuoteLines(ctx, tx, quoteID, save.MarginOptionID, save.CustomMargin, save.Lines, save.Totals); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, setTermsInputsSQL, save.DeliveryTime, save.Currency, save.ValidUntil, save.ExtraTerms, quoteID); err != nil {
		return fmt.Errorf("store: issue quote %d: terms inputs: %w", quoteID, err)
	}
	if err := issueQuote(ctx, tx, quoteID, is, costNote); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: issue quote %d: %w", quoteID, err)
	}
	return nil
}

// issueQuote is IssueQuote inside the caller's transaction, which has stamped the actor.
func issueQuote(ctx context.Context, tx *sql.Tx, quoteID int64, is Issue, costNote string) error {
	var validUntilArg any
	if is.ValidUntil != nil {
		validUntilArg = *is.ValidUntil
	}
	// The proyecto is opened first so the quote is written once, with it. If the quote
	// turns out not to be a draft, the rollback takes the proyecto with it.
	var newProjectID any
	res, err := tx.ExecContext(ctx, `
		INSERT INTO projects (folio, customer_id, user_id)
		SELECT folio, customer_id, user_id FROM quotes
		WHERE id = ? AND status = 'borrador' AND project_id IS NULL`, quoteID)
	if err != nil {
		return fmt.Errorf("store: issue quote %d: open project: %w", quoteID, err)
	}
	if opened, _ := res.RowsAffected(); opened == 1 {
		id, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("store: issue quote %d: open project: %w", quoteID, err)
		}
		newProjectID = id
	}
	res, err = tx.ExecContext(ctx, `
		UPDATE quotes SET
			status = 'emitida',
			issued_at = ?,
			terms_snapshot = ?,
			valid_until = ?,
			customer_name_snapshot = ?,
			vendedor_snapshot = ?,
			margin_name_snapshot = ?,
			margin_snapshot_micros = ?,
			pdf_sha256 = ?,
			project_id = COALESCE(project_id, ?)
		WHERE id = ? AND status = 'borrador'`,
		is.IssuedAt, is.TermsSnapshot, validUntilArg,
		is.CustomerNameSnapshot, is.VendedorSnapshot, is.MarginName, int64(is.MarginMicros),
		is.PDFSHA256, newProjectID, quoteID,
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
	if costNote != "" {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO quote_comments (quote_id, user_id, body) VALUES (?, ?, ?)`,
			quoteID, is.IssuedBy, costNote); err != nil {
			return fmt.Errorf("store: issue quote %d: cost note: %w", quoteID, err)
		}
	}
	return nil
}

// revisionCostNote is the cost-change note for issuing quoteID, or "" if it isn't a
// revision or no cost changed. next is the line set being issued; nil means the lines
// already saved.
func (s *Store) revisionCostNote(ctx context.Context, quoteID int64, next []QuoteLine) (string, error) {
	q, err := s.QuoteByID(ctx, quoteID)
	if err != nil || q == nil || q.SupersedesQuoteID == nil {
		return "", err
	}
	prev, err := s.ListQuoteLines(ctx, *q.SupersedesQuoteID)
	if err != nil {
		return "", err
	}
	if next == nil {
		if next, err = s.ListQuoteLines(ctx, quoteID); err != nil {
			return "", err
		}
	}
	return costChangeNote(q.SupersedesFolio, prev, next), nil
}

// ErrQuoteNotIssued is returned by CreateRevision when the target quote isn't
// currently 'emitida' — only the active issued version of a quote lineage can be
// revised; a superseded ('revisada') quote must be revised via its own successor.
var ErrQuoteNotIssued = errors.New("store: quote is not issued")

// ErrProjectLocked is returned by CreateRevision when the quote's proyecto has left
// prospecto: once the client's purchase order is in, the quote it answers can't change.
var ErrProjectLocked = errors.New("store: project is past prospecto, its quote can't be revised")

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
// a revision), sharing customer/prefix/margin option and proyecto and starting from the
// original's lines — and marks the original 'revisada'. The draft takes the original's
// place as the proyecto's current quote, which is why the original is marked first: a
// proyecto holds one quote that isn't revisada (see ErrProjectLocked for when it can't). Nothing about the original's own row is
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
	if original.Status != QuoteEmitida {
		return nil, ErrQuoteNotIssued
	}
	if !original.Revisable() {
		return nil, ErrProjectLocked
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
		UPDATE quotes SET status = 'revisada' WHERE id = ? AND status = 'emitida'`, originalID)
	if err != nil {
		return nil, fmt.Errorf("store: create revision of quote %d: mark superseded: %w", originalID, err)
	}
	if affected, _ := res.RowsAffected(); affected != 1 {
		return nil, ErrQuoteNotIssued
	}
	res, err = tx.ExecContext(ctx, `
		INSERT INTO quotes (folio, prefix, quote_type_id, folio_number, customer_id, user_id, status, supersedes_quote_id, margin_option_id, custom_margin_micros, custom_copper_price_micros, delivery_time, currency, project_id, extra_terms)
		VALUES (?, ?, ?, ?, ?, ?, 'borrador', ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, NULLIF(?, ''))`,
		newFolio, original.Prefix, original.QuoteTypeID, original.FolioNumber, original.CustomerID, userID, originalID,
		original.MarginOptionID, original.CustomMarginMicros, original.CustomCopperPriceMicros, original.DeliveryTime, original.Currency,
		original.ProjectID, original.ExtraTerms,
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
				unit_price_micros, line_total, pricing_inputs, source, cost_micros
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			newID, i+1, productID, l.DescriptionSnapshot, int64(l.QtyMilli),
			int64(l.UnitPriceMicros), int64(l.LineTotal), pricingInputs, l.Source, costArg(l.CostMicros),
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

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: create revision of quote %d: %w", originalID, err)
	}
	return s.QuoteByID(ctx, newID)
}

// DuplicateQuote copies any quote, in any state, as an independent new draft for
// customerID, owned by userID (issue #40): a new folio in the same series, not a
// revision, and no proyecto until it is issued. It starts from the original's lines,
// margin (its option, or the default one if that was retired since, and any custom
// margin), delivery time, currency and extra terms; on a free-lines series also its
// validity date and its cotización del proveedor (data and files, copied onto the new
// chain). Status, comments, follow-up and the O.C. stay behind, and the original isn't
// touched. Lines and totals are copied as they are: the caller reprices them, since the
// builder prices a draft from the current catalog anyway. Returns nil if there is no
// such quote.
func (s *Store) DuplicateQuote(ctx context.Context, originalID, customerID, userID int64) (*Quote, error) {
	original, err := s.QuoteByID(ctx, originalID)
	if err != nil {
		return nil, fmt.Errorf("store: duplicate quote %d: %w", originalID, err)
	}
	if original == nil {
		return nil, nil
	}
	lines, err := s.ListQuoteLines(ctx, originalID)
	if err != nil {
		return nil, fmt.Errorf("store: duplicate quote %d: %w", originalID, err)
	}
	root, err := s.QuoteChainRoot(ctx, originalID)
	if err != nil {
		return nil, fmt.Errorf("store: duplicate quote %d: %w", originalID, err)
	}
	// A gap in the folio sequence is fine, as in CreateDraftQuote.
	prefix, number, err := s.NextFolio(ctx, original.QuoteTypeID)
	if err != nil {
		return nil, fmt.Errorf("store: duplicate quote %d: %w", originalID, err)
	}
	folio := FolioText(prefix, number)
	var validUntil any
	if original.SeriesFreeLines && original.ValidUntil != nil {
		validUntil = *original.ValidUntil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store: duplicate quote %d: %w", originalID, err)
	}
	defer tx.Rollback()
	if err := stampActor(ctx, tx); err != nil {
		return nil, fmt.Errorf("store: duplicate quote %d: %w", originalID, err)
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO quotes (folio, prefix, quote_type_id, folio_number, customer_id, user_id, status, margin_option_id, custom_margin_micros, custom_copper_price_micros, delivery_time, currency, extra_terms, valid_until, subtotal, iva, total)
		VALUES (?, ?, ?, ?, ?, ?, 'borrador',
			COALESCE((SELECT id FROM margin_options WHERE id = ? AND retired_at IS NULL),
				(SELECT id FROM margin_options WHERE is_default = 1 AND retired_at IS NULL)),
			?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?)`,
		folio, prefix, original.QuoteTypeID, number, customerID, userID,
		original.MarginOptionID, original.CustomMarginMicros, original.CustomCopperPriceMicros,
		original.DeliveryTime, original.Currency, original.ExtraTerms, validUntil,
		int64(original.Subtotal), int64(original.IVA), int64(original.Total),
	)
	if err != nil {
		return nil, fmt.Errorf("store: duplicate quote %d: insert: %w", originalID, err)
	}
	newID, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("store: duplicate quote %d: %w", originalID, err)
	}
	for i, l := range lines {
		var productID, pricingInputs any
		if l.ProductID != nil {
			productID = *l.ProductID
		}
		if l.PricingInputsJSON != nil {
			pricingInputs = *l.PricingInputsJSON
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO quote_lines (
				quote_id, line_no, product_id, description_snapshot, qty_milli,
				unit_price_micros, line_total, pricing_inputs, source, cost_micros
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			newID, i+1, productID, l.DescriptionSnapshot, int64(l.QtyMilli),
			int64(l.UnitPriceMicros), int64(l.LineTotal), pricingInputs, l.Source, costArg(l.CostMicros),
		); err != nil {
			return nil, fmt.Errorf("store: duplicate quote %d: copy line %d: %w", originalID, i+1, err)
		}
	}
	if original.SeriesFreeLines && root != 0 {
		// The new draft is its own chain's root. The files keep who uploaded them and when.
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO supplier_quotes (root_quote_id, supplier, number, updated_by)
			SELECT ?, supplier, number, ? FROM supplier_quotes WHERE root_quote_id = ?`,
			newID, userID, root); err != nil {
			return nil, fmt.Errorf("store: duplicate quote %d: copy supplier quote: %w", originalID, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO supplier_quote_files (root_quote_id, filename, content_type, size, data, uploaded_by, uploaded_at)
			SELECT ?, filename, content_type, size, data, uploaded_by, uploaded_at
			FROM supplier_quote_files WHERE root_quote_id = ? AND deleted_at IS NULL ORDER BY id`,
			newID, root); err != nil {
			return nil, fmt.Errorf("store: duplicate quote %d: copy supplier files: %w", originalID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: duplicate quote %d: %w", originalID, err)
	}
	return s.QuoteByID(ctx, newID)
}

// QuoteComment is a quote_comments row; UserName is the author's current name (join).
type QuoteComment struct {
	ID        int64
	QuoteID   int64
	UserID    int64
	UserName  string
	Body      string
	CreatedAt string
}

// AddQuoteComment appends a follow-up note to a quote. The body is stored as given
// apart from surrounding whitespace; an empty one is refused by the table's CHECK.
func (s *Store) AddQuoteComment(ctx context.Context, quoteID, userID int64, body string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO quote_comments (quote_id, user_id, body) VALUES (?, ?, ?)`,
		quoteID, userID, strings.TrimSpace(body))
	if err != nil {
		return fmt.Errorf("store: add comment to quote %d: %w", quoteID, err)
	}
	return nil
}

// ListQuoteComments returns a quote's comments, newest first.
func (s *Store) ListQuoteComments(ctx context.Context, quoteID int64) ([]QuoteComment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.quote_id, c.user_id, u.name, c.body, c.created_at
		FROM quote_comments c JOIN users u ON u.id = c.user_id
		WHERE c.quote_id = ?
		ORDER BY c.created_at DESC, c.id DESC`, quoteID)
	if err != nil {
		return nil, fmt.Errorf("store: list comments of quote %d: %w", quoteID, err)
	}
	defer rows.Close()
	var out []QuoteComment
	for rows.Next() {
		var c QuoteComment
		if err := rows.Scan(&c.ID, &c.QuoteID, &c.UserID, &c.UserName, &c.Body, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: list comments of quote %d: %w", quoteID, err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
