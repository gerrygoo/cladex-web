package views

import (
	"fmt"
	"strings"

	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/store"
)

// KgPricedSeries is the quote series whose products are costed per kg and quoted per
// metre (issue #37, Alumoclad hilo de guarda): a line's cost per metre is the product's
// cost × its Peso (kg/m), priced like a CCS material.
const KgPricedSeries = "QG"

// ShowsKg reports whether a quote's lines show their weight: Kg por metro and Kg, next
// to the quantity in metres (issue #34). CCS and QG quotes do, since both are sold by
// weight; the series never changes familia, so an issued quote keeps its columns.
func ShowsKg(quote store.Quote) bool {
	return quote.SeriesFamily == CCSFamilyName || quote.Prefix == KgPricedSeries
}

// KgPerUnitText is a line's kg per metre with 4 decimals, empty without one.
func KgPerUnitText(l QuoteLineView) string {
	if l.KgPerUnit == nil {
		return ""
	}
	return groupedDecimal(money.RoundHalfUp(int64(*l.KgPerUnit), 100), 4)
}

// KgText is a line's total weight, quantity × kg per metre, with 2 decimals, empty
// without a weight or a valid quantity. It works from the unrounded kg per metre.
func KgText(l QuoteLineView) string {
	if l.KgPerUnit == nil || l.Error != "" {
		return ""
	}
	// Milli (1e-3) × Micros (1e-6) is 1e-9; hundredths are 1e-2.
	return groupedDecimal(money.RoundHalfUp(int64(l.QtyMilli)*int64(*l.KgPerUnit), 10_000_000), 2)
}

// groupedDecimal formats v, a count of 10^-places units, with thousands separators,
// e.g. groupedDecimal(123456, 2) == "1,234.56".
func groupedDecimal(v int64, places int) string {
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	scale := int64(1)
	for range places {
		scale *= 10
	}
	whole := fmt.Sprintf("%d", v/scale)
	var b strings.Builder
	for i, r := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return fmt.Sprintf("%s%s.%0*d", sign, b.String(), places, v%scale)
}
