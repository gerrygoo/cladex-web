// Package pricing computes quote line unit prices and totals. Pure Go, no HTTP, no
// database — callers pass in an already-loaded product cost and the quote's margin.
//
// Every product prices the same way: cost / (1 - margin), the margin-on-sale-price
// convention reverse-engineered from the legacy workbook's own formulas (Cotizador CCA
// and its source sheet — see docs/PLAN.md's M2 footnotes), where cost is a flat cost
// plus the materials one unit contains, and the margin is the quote's chosen option
// (docs/PLAN.md, M3).
package pricing

import (
	"errors"
	"fmt"

	"github.com/gerrygoo/cladex-web/internal/money"
)

// IVARate is Mexico's standard value-added tax rate (16%), matching the legacy
// workbook's own hardcoded formula (subtotal*0.16). Not admin-configurable.
const IVARate = money.Micros(160_000)

// ErrNoMargin is returned when the quote has no margin it can use.
var ErrNoMargin = errors.New("pricing: product needs a margin")

// ErrNoCost is returned for a product with neither a flat cost nor materials.
var ErrNoCost = errors.New("pricing: product has no pricing data (cost or materials)")

// Product is the subset of store.Product the pricing engine reads: a flat cost
// (CostMicros) plus the materials one unit contains. See migrations/0002_add_product_cost.sql
// and 0008_materials.sql.
type Product struct {
	CostMicros *money.Micros
	Materials  []MaterialContent
}

// MaterialContent is how much of one material a unit of a product holds, and that
// material's pure-cost price per its own unit (e.g. 0.1723 kg at $160/kg).
type MaterialContent struct {
	QtyPerUnit money.Micros
	Price      money.Micros
}

// Cost is a product's pre-margin cost per unit: its flat cost plus each material's
// qty x price, each product rounded half up to the micro. ok is false when the product
// has neither.
func (p Product) Cost() (cost money.Micros, ok bool) {
	if p.CostMicros != nil {
		cost, ok = *p.CostMicros, true
	}
	for _, m := range p.Materials {
		cost += money.Micros(money.RoundHalfUp(int64(m.QtyPerUnit)*int64(m.Price), 1_000_000))
		ok = true
	}
	return cost, ok
}

// UnitPrice computes a product's per-unit price in MXN micros: cost / (1 - margin).
// margin is a fraction of the sale price (123_400 == 12.34%), nil when the quote has
// none it can use. This matches the workbook's own formula exactly (not cost*(1+margin);
// using that shorthand would compute different numbers for the same nominal fraction).
func UnitPrice(p Product, margin *money.Micros) (money.Micros, error) {
	cost, ok := p.Cost()
	if !ok {
		return 0, ErrNoCost
	}
	if margin == nil {
		return 0, ErrNoMargin
	}
	if *margin < 0 || *margin >= 1_000_000 {
		return 0, fmt.Errorf("pricing: margin %v out of range [0, 1_000_000)", *margin)
	}
	complement := 1_000_000 - int64(*margin)
	return money.Micros(money.RoundHalfUp(int64(cost)*1_000_000, complement)), nil
}

// ConvertQty converts a quantity from one unit to another, given the rate between
// them (amount of the target unit per 1 of the source unit — see
// migrations/0003_add_units_and_conversions.sql). Used to turn a quote line entered
// in a non-base unit (e.g. "rollos") into the product's own base unit before
// ComputeTotals runs, which assumes qty is already in that base unit.
func ConvertQty(qty money.Milli, rateMicros money.Micros) money.Milli {
	return money.Milli(money.RoundHalfUp(int64(qty)*int64(rateMicros), 1_000_000))
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
