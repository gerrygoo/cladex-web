package views

import (
	"fmt"
	"strings"

	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/store"
)

// ShowsKg reports whether a quote's lines show their weight: Kg por metro and Kg, next
// to the quantity in metres (issue #34). The quote types priced by weight do: the
// kg-priced one (QG) and the one whose margin follows the copper price (CCS, QS).
func ShowsKg(quote store.Quote) bool {
	return quote.CopperPriceMargin || quote.KgPriced
}

// KgPerUnitText is a line's kg per metre with 4 decimals, empty without one.
func KgPerUnitText(l QuoteLineView) string {
	if l.KgPerUnit == nil {
		return ""
	}
	return KgPerMetreText(*l.KgPerUnit)
}

// KgPerMetreText is a weight per metre with 4 decimals, e.g. "0.3896".
func KgPerMetreText(kg money.Micros) string {
	return groupedDecimal(money.RoundHalfUp(int64(kg), 100), 4)
}

// KgTotalText is qty × kg per metre with 2 decimals, e.g. "43.25" for 111 m of 0.3896.
func KgTotalText(qty money.Milli, kg money.Micros) string {
	// Milli (1e-3) × Micros (1e-6) is 1e-9; hundredths are 1e-2.
	return groupedDecimal(money.RoundHalfUp(int64(qty)*int64(kg), 10_000_000), 2)
}

// KgText is a line's total weight, quantity × kg per metre, with 2 decimals, empty
// without a weight or a valid quantity. It works from the unrounded kg per metre.
func KgText(l QuoteLineView) string {
	if l.KgPerUnit == nil || l.Error != "" {
		return ""
	}
	return KgTotalText(l.QtyMilli, *l.KgPerUnit)
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
