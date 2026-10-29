package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Family is a product_families row with the quote type that quotes its products
// (migrations/0029_quote_types.sql): Series, SeriesLabel and Terms are that type's
// prefix, name and terms. Series is empty for a familia without one, which then can't
// start quotes. Until the familias page is split from the quote types (slice T2 of
// docs/planning/tipos-de-cotizacion.md) a familia has at most one type.
type Family struct {
	ID            int64
	Name          string
	Series        string
	SeriesLabel   string
	Terms         string // the series' terms block, one term per line
	FreeLinesOnly bool   // holds no catalog products: quoted with líneas libres only
	// SATProductCode is the SAT's c_ClaveProdServ a factura's lines from this familia's
	// series start with; "" when not set.
	SATProductCode string
}

// Series is a quote series a new quote can start in: the folio prefix, how the picker
// labels it, and the familia that owns it.
type Series struct {
	Prefix        string
	Label         string
	FamilyName    string
	FreeLinesOnly bool
}

// PickerLabel is the series as the new-quote form lists it: "QA — Cable CCA", or just
// "QA — <familia>" when the series has no label of its own.
func (s Series) PickerLabel() string {
	label := s.Label
	if label == "" {
		label = s.FamilyName
	}
	return s.Prefix + " — " + label
}

const familyCols = `f.id, f.name, COALESCE(t.prefix, ''), COALESCE(t.name, ''), COALESCE(t.terms, ''),
	f.free_lines_only, COALESCE(f.sat_product_code, '')`

const familyFrom = ` FROM product_families f LEFT JOIN quote_types t ON t.family_id = f.id`

func scanFamily(row interface{ Scan(...any) error }) (Family, error) {
	var f Family
	err := row.Scan(&f.ID, &f.Name, &f.Series, &f.SeriesLabel, &f.Terms, &f.FreeLinesOnly, &f.SATProductCode)
	return f, err
}

// ListAllFamilies returns every familia, including free-lines-only ones, in creation
// order (which is also the order of their series in the new-quote form).
func (s *Store) ListAllFamilies(ctx context.Context) ([]Family, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+familyCols+familyFrom+` ORDER BY f.id`)
	if err != nil {
		return nil, fmt.Errorf("store: list all families: %w", err)
	}
	defer rows.Close()

	var families []Family
	for rows.Next() {
		f, err := scanFamily(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list all families: %w", err)
		}
		families = append(families, f)
	}
	return families, rows.Err()
}

// ListSeries returns the quote types a new quote can start in, in creation order.
func (s *Store) ListSeries(ctx context.Context) ([]Series, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.prefix, t.name, COALESCE(f.name, ''), t.free_lines_only
		FROM quote_types t LEFT JOIN product_families f ON f.id = t.family_id ORDER BY t.id`)
	if err != nil {
		return nil, fmt.Errorf("store: list series: %w", err)
	}
	defer rows.Close()

	var series []Series
	for rows.Next() {
		var sr Series
		if err := rows.Scan(&sr.Prefix, &sr.Label, &sr.FamilyName, &sr.FreeLinesOnly); err != nil {
			return nil, fmt.Errorf("store: list series: %w", err)
		}
		series = append(series, sr)
	}
	return series, rows.Err()
}

// SeriesExists reports whether prefix is the current prefix of some quote type.
func (s *Store) SeriesExists(ctx context.Context, prefix string) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM quote_types WHERE prefix = ?`, prefix).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: series %q: %w", prefix, err)
	}
	return true, nil
}

// ErrDuplicateFamilyName and ErrDuplicateSeries are returned by CreateFamily when the
// name or the series prefix is already taken. A prefix is taken if any quote type has it
// or any quote was ever given it: a type reusing a retired prefix would hand out folios
// that already exist.
var (
	ErrDuplicateFamilyName = errors.New("store: a familia with this name already exists")
	ErrDuplicateSeries     = errors.New("store: a familia with this series already exists")
)

// CreateFamily inserts a familia together with its quote type (prefix f.Series) and
// returns the familia's id. Name and prefix are unique; terms is the type's terms block,
// one term per line. The type starts with no special behavior: default letterhead,
// numbering from 1.
func (s *Store) CreateFamily(ctx context.Context, f Family) (int64, error) {
	var taken int
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM product_families WHERE name = ? COLLATE NOCASE`, f.Name).Scan(&taken); err != nil {
		return 0, fmt.Errorf("store: check familia %q: %w", f.Name, err)
	}
	if taken > 0 {
		return 0, ErrDuplicateFamilyName
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT (SELECT count(*) FROM quote_types WHERE prefix = ?1)
			+ (SELECT count(*) FROM quotes WHERE prefix = ?1)`, f.Series).Scan(&taken); err != nil {
		return 0, fmt.Errorf("store: check series %q: %w", f.Series, err)
	}
	if taken > 0 {
		return 0, ErrDuplicateSeries
	}

	var id int64
	err := s.inTx(ctx, "create familia", func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO product_families (name, free_lines_only) VALUES (?, ?)`, f.Name, f.FreeLinesOnly)
		if err != nil {
			return err
		}
		if id, err = res.LastInsertId(); err != nil {
			return err
		}
		if f.Series == "" {
			return nil
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO quote_types (prefix, name, terms, family_id, free_lines_only) VALUES (?, ?, ?, ?, ?)`,
			f.Series, f.SeriesLabel, f.Terms, id, f.FreeLinesOnly)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("store: create familia %q: %w", f.Name, err)
	}
	return id, nil
}

// UpdateFamilyTerms replaces the terms block and name of a familia's quote type. It
// never touches the prefix and never reaches issued quotes, which print the terms
// frozen at issue time.
func (s *Store) UpdateFamilyTerms(ctx context.Context, id int64, seriesLabel, terms string) error {
	res, err := s.exec(ctx,
		`UPDATE quote_types SET name = ?, terms = ? WHERE family_id = ?`, seriesLabel, terms, id)
	if err != nil {
		return fmt.Errorf("store: update familia %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SetFamilySATCode sets a familia's default c_ClaveProdServ; "" clears it.
func (s *Store) SetFamilySATCode(ctx context.Context, id int64, code string) error {
	if _, err := s.exec(ctx, `UPDATE product_families SET sat_product_code = NULLIF(?, '') WHERE id = ?`, code, id); err != nil {
		return fmt.Errorf("store: familia %d sat code: %w", id, err)
	}
	return nil
}

// SplitTerms turns a terms block into its non-empty, trimmed lines.
func SplitTerms(terms string) []string {
	var out []string
	for _, line := range strings.Split(terms, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// DeliveryTimeToken marks where a quote's delivery time goes in a terms block. A series
// whose terms hold it asks for the delivery time on every quote and won't issue one
// without it; RenderTerms puts the value in place.
const DeliveryTimeToken = "{tiempo_de_entrega}"

// CurrencyToken marks where a quote's currency goes in a terms block, the same way: a
// series whose terms hold it asks each quote for MXN or USD and won't issue one without.
const CurrencyToken = "{moneda}"

// Currencies are the currencies a quote can be priced in, in the order the builder
// offers them. Prices are typed in the chosen one; nothing is converted.
var Currencies = []string{"MXN", "USD"}

// currencyNames is how each currency reads inside a terms line.
var currencyNames = map[string]string{
	"MXN": "pesos mexicanos (MXN)",
	"USD": "dólares americanos (USD)",
}

// ValidCurrency reports whether c is one of Currencies.
func ValidCurrency(c string) bool {
	_, ok := currencyNames[c]
	return ok
}

// RequiresDeliveryTime reports whether a terms block asks each quote for a delivery time.
func RequiresDeliveryTime(terms string) bool {
	return strings.Contains(terms, DeliveryTimeToken)
}

// RequiresCurrency reports whether a terms block asks each quote for a currency.
func RequiresCurrency(terms string) bool {
	return strings.Contains(terms, CurrencyToken)
}

// TermsInputs are the per-quote values a terms block can hold tokens for, and the
// quote's own extra terms (QL, issue #27), one per line.
type TermsInputs struct {
	DeliveryTime string
	Currency     string
	ExtraTerms   string
}

// paymentTerm is the series term the extra terms follow (issue #27).
const paymentTerm = "pago por adelantado"

// RenderTerms splits a terms block into lines and fills in the tokens. A value a draft
// hasn't got yet renders as "por definir". The extra terms, blank lines dropped, go
// right after the "Pago por adelantado…" term, or at the end without one.
func RenderTerms(terms string, in TermsInputs) []string {
	deliveryTime := strings.TrimSpace(in.DeliveryTime)
	if deliveryTime == "" {
		deliveryTime = "por definir"
	}
	currency, ok := currencyNames[in.Currency]
	if !ok {
		currency = "moneda por definir"
	}
	lines := SplitTerms(terms)
	at := len(lines)
	for i, l := range lines {
		l = strings.ReplaceAll(l, DeliveryTimeToken, deliveryTime)
		lines[i] = strings.ReplaceAll(l, CurrencyToken, currency)
		if at == len(lines) && strings.Contains(strings.ToLower(l), paymentTerm) {
			at = i + 1
		}
	}
	extra := SplitTerms(in.ExtraTerms)
	if len(extra) == 0 {
		return lines
	}
	return append(lines[:at:at], append(extra, lines[at:]...)...)
}

// QuoteTypeBehavior is what a quote type does differently from a plain one
// (migrations/0029_quote_types.sql). Letterhead is one of quote_types.letterhead's
// values; "" is the plain Cladex one.
type QuoteTypeBehavior struct {
	FreeLinesOnly     bool
	HideAddress       bool
	SortLast          bool
	KgPriced          bool
	CopperPriceMargin bool
	Letterhead        string
}

// SetQuoteTypeBehavior replaces a quote type's behavior flags.
func (s *Store) SetQuoteTypeBehavior(ctx context.Context, quoteTypeID int64, b QuoteTypeBehavior) error {
	if b.Letterhead == "" {
		b.Letterhead = "cladex"
	}
	res, err := s.exec(ctx, `
		UPDATE quote_types SET free_lines_only = ?, hide_address = ?, sort_last = ?, kg_priced = ?,
			copper_price_margin = ?, letterhead = ?
		WHERE id = ?`,
		b.FreeLinesOnly, b.HideAddress, b.SortLast, b.KgPriced, b.CopperPriceMargin, b.Letterhead, quoteTypeID)
	if err != nil {
		return fmt.Errorf("store: quote type %d behavior: %w", quoteTypeID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
