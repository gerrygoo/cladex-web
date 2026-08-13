// Package pricing computes quote line unit prices and totals from catalog and settings
// data. Pure Go, no HTTP, no database — callers pass in already-loaded store.Product
// fields and settings.Settings values.
//
// Reverse-engineered from the legacy workbook's own formulas (Cotizador CCA, Cotizador
// CCS, and their CCA/CCS & AC source sheets — see docs/PLAN.md's M2 footnotes), since
// the workbook has no written spec beyond "the numbers the formulas produce".
package pricing

import (
	"errors"
	"fmt"

	"github.com/gerrygoo/cladex-web/internal/money"
)

// IVARate is Mexico's standard value-added tax rate (16%), matching the legacy
// workbook's own hardcoded formula (subtotal*0.16). Not admin-configurable.
const IVARate = money.Micros(160_000)

// Settings are the admin-configurable knobs this package needs from the settings
// table (fx_rate, copper_price, default_margin), already parsed to Micros — this
// package never touches the database.
type Settings struct {
	FXRate        money.Micros // USD -> MXN
	CopperPrice   money.Micros // MXN per kg
	DefaultMargin money.Micros // fraction of the sale price, e.g. 123_400 == 12.34%
}

// Product is the subset of store.Product the pricing engine reads. Exactly one of
// UnitPriceMicros, CostMicros, or KgPerMMicros is expected to be set in practice — see
// migrations/0001_init.sql and 0002_add_product_cost.sql for what each column means
// and which product families populate it.
type Product struct {
	UnitPriceMicros *money.Micros
	CostMicros      *money.Micros
	KgPerMMicros    *money.Micros
	Currency        string // "MXN" or "USD"; "" is treated as MXN
}

// PriceBreak is one price_breaks row: at qty >= MinQty, UnitPriceMicros overrides the
// product's base price outright — already a final MXN price, with no margin or FX
// applied to it. Breaks need not be pre-sorted; UnitPrice picks the highest threshold
// the quantity clears.
type PriceBreak struct {
	MinQty          money.Milli
	UnitPriceMicros money.Micros
}

// UnitPrice computes a product's per-unit price in MXN micros for the given quantity:
// the product's base price (flat, cost+margin, or weight*metal price), converted from
// USD if needed, then overridden by the best price break the quantity clears, if any.
func UnitPrice(p Product, qty money.Milli, breaks []PriceBreak, s Settings) (money.Micros, error) {
	base, err := basePrice(p, s)
	if err != nil {
		return 0, err
	}

	if p.Currency == "USD" {
		if s.FXRate <= 0 {
			return 0, errors.New("pricing: USD product needs a positive FX rate")
		}
		base = money.Micros(money.RoundHalfUp(int64(base)*int64(s.FXRate), 1_000_000))
	}

	if tier, ok := bestPriceBreak(breaks, qty); ok {
		return tier, nil
	}
	return base, nil
}

// basePrice applies the product's pricing rule, in priority order:
//  1. UnitPriceMicros set: a flat catalog price (e.g. ABASTILUM) — used as-is, already
//     final MXN (or USD, converted by the caller above).
//  2. CostMicros set: cost / (1 - margin) — e.g. CCA. This is a margin-on-sale-price
//     convention, matching the workbook's own formula exactly (not cost*(1+margin));
//     using the plan's shorthand convention here would compute different numbers for
//     the same nominal margin fraction.
//  3. KgPerMMicros set: kg/m * copper price — e.g. CCS & AC, where the workbook's own
//     margin math reduces algebraically to exactly this (see PLAN.md footnote).
func basePrice(p Product, s Settings) (money.Micros, error) {
	switch {
	case p.UnitPriceMicros != nil:
		return *p.UnitPriceMicros, nil
	case p.CostMicros != nil:
		if s.DefaultMargin < 0 || s.DefaultMargin >= 1_000_000 {
			return 0, fmt.Errorf("pricing: margin %v out of range [0, 1_000_000)", s.DefaultMargin)
		}
		complement := 1_000_000 - int64(s.DefaultMargin)
		return money.Micros(money.RoundHalfUp(int64(*p.CostMicros)*1_000_000, complement)), nil
	case p.KgPerMMicros != nil:
		if s.CopperPrice <= 0 {
			return 0, errors.New("pricing: weight-priced product needs a positive copper price")
		}
		return money.Micros(money.RoundHalfUp(int64(*p.KgPerMMicros)*int64(s.CopperPrice), 1_000_000)), nil
	default:
		return 0, errors.New("pricing: product has no pricing data (unit price, cost, or weight)")
	}
}

func bestPriceBreak(breaks []PriceBreak, qty money.Milli) (money.Micros, bool) {
	found := false
	var best money.Micros
	bestMinQty := money.Milli(-1)
	for _, b := range breaks {
		if qty >= b.MinQty && b.MinQty > bestMinQty {
			best = b.UnitPriceMicros
			bestMinQty = b.MinQty
			found = true
		}
	}
	return best, found
}

// Line pairs a computed unit price with the quantity it applies to — the minimal
// input ComputeTotals needs per quote line.
type Line struct {
	UnitPriceMicros money.Micros
	QtyMilli        money.Milli
}

// Totals is a quote's subtotal, IVA, and grand total, all in centavos.
type Totals struct {
	Subtotal money.Centavos
	IVA      money.Centavos
	Total    money.Centavos
}

// ComputeTotals sums each line's rounded total (money.LineTotalCentavos — the one
// rounding point per line) and applies IVA once to the summed subtotal.
func ComputeTotals(lines []Line) Totals {
	var subtotal money.Centavos
	for _, l := range lines {
		subtotal += money.LineTotalCentavos(l.UnitPriceMicros, l.QtyMilli)
	}
	iva := money.ApplyRate(subtotal, IVARate)
	return Totals{Subtotal: subtotal, IVA: iva, Total: subtotal + iva}
}
