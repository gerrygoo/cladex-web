// Package money is the single owner of Cladex's fixed-point monetary types and
// arithmetic. No float64 crosses this boundary: the catalog holds unit prices with six
// decimal digits of precision (e.g. 6.319872233629933 MXN/metre), and centavos alone
// are too coarse for that — so unit prices live in micro-pesos and everything that gets
// charged lives in centavos, rounded exactly once on the way down.
package money

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Micros is a monetary amount in micro-pesos (1e-6 MXN). Used for unit prices.
type Micros int64

// MicrosFromFloat converts a float64 amount (e.g. a value read from a spreadsheet cell)
// to Micros, rounding half away from zero. This is the float→fixed-point boundary: call
// it once, at the point external float data enters the system, and never round again.
func MicrosFromFloat(f float64) Micros {
	return Micros(math.Round(f * 1_000_000))
}

// ParseMicros parses a plain decimal string (e.g. "6.319872", "1234.56", "-3") into
// Micros. Unlike MicrosFromFloat, it never passes through float64 — form input for a
// catalog value with up to six decimal digits must round-trip exactly, and float64
// can't guarantee that.
func ParseMicros(s string) (Micros, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("money: empty value")
	}
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	intPart, fracPart, hasFrac := strings.Cut(s, ".")
	if strings.Contains(fracPart, ".") || (intPart == "" && !hasFrac) {
		return 0, fmt.Errorf("money: invalid number %q", s)
	}
	if len(fracPart) > 6 {
		return 0, fmt.Errorf("money: too many decimal digits in %q", s)
	}
	if intPart == "" {
		intPart = "0"
	}
	fracPart += strings.Repeat("0", 6-len(fracPart))

	intVal, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("money: invalid number %q: %w", s, err)
	}
	fracVal, err := strconv.ParseInt(fracPart, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("money: invalid number %q: %w", s, err)
	}

	v := intVal*1_000_000 + fracVal
	if neg {
		v = -v
	}
	return Micros(v), nil
}

// String formats Micros as a plain decimal peso string with trailing zeros trimmed
// (e.g. Micros(6_319_872).String() == "6.319872"), for round-tripping through an HTML
// form field. Unlike Centavos.String, this is not currency-formatted — no thousands
// separator or "$" sign, since it feeds a value attribute, not a display label.
func (m Micros) String() string {
	v := int64(m)
	neg := v < 0
	if neg {
		v = -v
	}
	sign := ""
	if neg {
		sign = "-"
	}
	s := fmt.Sprintf("%s%d.%06d", sign, v/1_000_000, v%1_000_000)
	s = strings.TrimRight(s, "0")
	return strings.TrimRight(s, ".")
}

// Centavos is a monetary amount in centavos (1e-2 MXN). Used for anything actually
// charged: line totals, subtotal, IVA, total.
type Centavos int64

const microsPerCentavo = 10_000

// ToCentavosHalfUp converts a micro-peso amount to centavos, rounding half away from
// zero. This is the one place rounding happens; callers must not round upstream of it.
func (m Micros) ToCentavosHalfUp() Centavos {
	return Centavos(RoundHalfUp(int64(m), microsPerCentavo))
}

// RoundHalfUp divides numerator by denominator and rounds half away from zero. The one
// rounding rule for every fixed-point conversion in this package, and reused by
// internal/pricing for margin and line-total math that doesn't reduce to a plain
// Micros->Centavos conversion — so the rule lives in exactly one place.
func RoundHalfUp(numerator, denominator int64) int64 {
	neg := numerator < 0
	if neg {
		numerator = -numerator
	}
	q, r := numerator/denominator, numerator%denominator
	if r*2 >= denominator {
		q++
	}
	if neg {
		q = -q
	}
	return q
}

// String formats centavos as es-MX currency, e.g. "$1,234.56".
func (c Centavos) String() string {
	v := int64(c)
	neg := v < 0
	if neg {
		v = -v
	}
	sign := ""
	if neg {
		sign = "-"
	}
	return fmt.Sprintf("%s$%s.%02d", sign, groupThousands(v/100), v%100)
}

func groupThousands(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var groups []string
	for len(s) > 3 {
		groups = append([]string{s[len(s)-3:]}, groups...)
		s = s[:len(s)-3]
	}
	groups = append([]string{s}, groups...)
	return strings.Join(groups, ",")
}

// Milli is a quantity in thousandths of a unit (1e-3) — used for quote line quantities
// (qty_milli) and price-break thresholds (min_qty_milli). Kept separate from Micros
// because it measures a physical quantity (metres, pieces), not money.
type Milli int64

// MilliFromFloat converts a float64 quantity to Milli, rounding half away from zero.
func MilliFromFloat(f float64) Milli {
	return Milli(math.Round(f * 1_000))
}

// ParseMilli parses a plain decimal string into Milli, the Milli counterpart of
// ParseMicros — never through float64, so a quantity typed into a form round-trips
// exactly.
func ParseMilli(s string) (Milli, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("money: empty value")
	}
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	intPart, fracPart, hasFrac := strings.Cut(s, ".")
	if strings.Contains(fracPart, ".") || (intPart == "" && !hasFrac) {
		return 0, fmt.Errorf("money: invalid number %q", s)
	}
	if len(fracPart) > 3 {
		return 0, fmt.Errorf("money: too many decimal digits in %q", s)
	}
	if intPart == "" {
		intPart = "0"
	}
	fracPart += strings.Repeat("0", 3-len(fracPart))

	intVal, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("money: invalid number %q: %w", s, err)
	}
	fracVal, err := strconv.ParseInt(fracPart, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("money: invalid number %q: %w", s, err)
	}

	v := intVal*1_000 + fracVal
	if neg {
		v = -v
	}
	return Milli(v), nil
}

// String formats Milli as a plain decimal string with trailing zeros trimmed, the
// Milli counterpart of Micros.String.
func (m Milli) String() string {
	v := int64(m)
	neg := v < 0
	if neg {
		v = -v
	}
	sign := ""
	if neg {
		sign = "-"
	}
	s := fmt.Sprintf("%s%d.%03d", sign, v/1_000, v%1_000)
	s = strings.TrimRight(s, "0")
	return strings.TrimRight(s, ".")
}

// LineTotalCentavos computes qty*unitPrice and rounds directly to centavos, half away
// from zero, in one step — the "round exactly once, at line total" rule applies here,
// not to any intermediate micros-scale value.
func LineTotalCentavos(unitPrice Micros, qty Milli) Centavos {
	return Centavos(RoundHalfUp(int64(unitPrice)*int64(qty), 10_000_000))
}

// ApplyRate multiplies a centavos amount by a fraction expressed in Micros (e.g.
// 160_000 for 16% IVA), rounding half away from zero. General-purpose fixed-point rate
// application, not specific to any one tax or fee.
func ApplyRate(amount Centavos, rateMicros Micros) Centavos {
	return Centavos(RoundHalfUp(int64(amount)*int64(rateMicros), 1_000_000))
}
