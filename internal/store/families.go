package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Family is a product_families row: a catalog category. QuoteTypes are the prefixes of
// the quote types that quote it (quote_types.family_id), ", "-separated, for the
// familias list. FreeLinesOnly marks the one familia that holds no catalog products
// (QL), which the product form leaves out.
type Family struct {
	ID            int64
	Name          string
	FreeLinesOnly bool
	// SATProductCode is the SAT's c_ClaveProdServ a factura's lines from this familia's
	// quote types start with; "" when not set.
	SATProductCode string
	QuoteTypes     string
}

// ListAllFamilies returns every familia, including free-lines-only ones, in creation
// order.
func (s *Store) ListAllFamilies(ctx context.Context) ([]Family, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT f.id, f.name, f.free_lines_only, COALESCE(f.sat_product_code, ''),
			COALESCE((SELECT group_concat(prefix, ', ') FROM
				(SELECT prefix FROM quote_types t WHERE t.family_id = f.id ORDER BY t.id)), '')
		FROM product_families f ORDER BY f.id`)
	if err != nil {
		return nil, fmt.Errorf("store: list all families: %w", err)
	}
	defer rows.Close()

	var families []Family
	for rows.Next() {
		var f Family
		if err := rows.Scan(&f.ID, &f.Name, &f.FreeLinesOnly, &f.SATProductCode, &f.QuoteTypes); err != nil {
			return nil, fmt.Errorf("store: list all families: %w", err)
		}
		families = append(families, f)
	}
	return families, rows.Err()
}

// ErrDuplicateFamilyName is returned by CreateFamily when the name is already taken.
var ErrDuplicateFamilyName = errors.New("store: a familia with this name already exists")

// CreateFamily inserts a familia named name and returns its id. Names are unique,
// ignoring case. Its quote types are created separately (CreateQuoteType).
func (s *Store) CreateFamily(ctx context.Context, name string) (int64, error) {
	var taken int
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM product_families WHERE name = ? COLLATE NOCASE`, name).Scan(&taken); err != nil {
		return 0, fmt.Errorf("store: check familia %q: %w", name, err)
	}
	if taken > 0 {
		return 0, ErrDuplicateFamilyName
	}
	res, err := s.exec(ctx, `INSERT INTO product_families (name) VALUES (?)`, name)
	if err != nil {
		return 0, fmt.Errorf("store: create familia %q: %w", name, err)
	}
	return res.LastInsertId()
}

// SetFamilySATCode sets a familia's default c_ClaveProdServ; "" clears it. It returns
// sql.ErrNoRows when there is no such familia.
func (s *Store) SetFamilySATCode(ctx context.Context, id int64, code string) error {
	res, err := s.exec(ctx, `UPDATE product_families SET sat_product_code = NULLIF(?, '') WHERE id = ?`, code, id)
	if err != nil {
		return fmt.Errorf("store: familia %d sat code: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
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
