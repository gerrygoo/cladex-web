// Package money is the single owner of Cladex's fixed-point monetary types and
// arithmetic. No float64 crosses this boundary: the catalog holds unit prices with six
// decimal digits of precision (e.g. 6.319872233629933 MXN/metre), and centavos alone
// are too coarse for that — so unit prices live in micro-pesos and everything that gets
// charged lives in centavos, rounded exactly once on the way down.
package money

import (
	"fmt"
	"math"
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

// Centavos is a monetary amount in centavos (1e-2 MXN). Used for anything actually
// charged: line totals, subtotal, IVA, total.
type Centavos int64

const microsPerCentavo = 10_000

// ToCentavosHalfUp converts a micro-peso amount to centavos, rounding half away from
// zero. This is the one place rounding happens; callers must not round upstream of it.
func (m Micros) ToCentavosHalfUp() Centavos {
	return Centavos(roundHalfUp(int64(m)))
}

func roundHalfUp(numerator int64) int64 {
	neg := numerator < 0
	if neg {
		numerator = -numerator
	}
	q, r := numerator/microsPerCentavo, numerator%microsPerCentavo
	if r*2 >= microsPerCentavo {
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
