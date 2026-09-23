package views

import (
	"strconv"
	"time"

	"github.com/gerrygoo/cladex-web/internal/store"
)

// MaterialAge is the "actualizado hace N días" staleness hint for a material price,
// from its updated_at (ISO-8601 UTC). It's only a hint: an old price never blocks a
// quote. Unparseable timestamps render as "".
func MaterialAge(updatedAt string, now time.Time) string {
	var t time.Time
	var err error
	for _, layout := range []string{"2006-01-02T15:04:05.000Z", time.RFC3339Nano} {
		if t, err = time.Parse(layout, updatedAt); err == nil {
			break
		}
	}
	if err != nil {
		return ""
	}
	days := int(now.Sub(t).Hours() / 24)
	switch {
	case days <= 0:
		return "actualizado hoy"
	case days == 1:
		return "actualizado hace 1 día"
	default:
		return "actualizado hace " + strconv.Itoa(days) + " días"
	}
}

// materialsInUse lists, once each, the materials the builder's lines were priced from,
// for the staleness hint under the totals.
func materialsInUse(lines []QuoteLineView) []store.Material {
	var out []store.Material
	seen := map[int64]bool{}
	for _, l := range lines {
		for _, m := range l.Materials {
			if !seen[m.ID] {
				seen[m.ID] = true
				out = append(out, m)
			}
		}
	}
	return out
}

// MaterialPrice renders a material's price per its unit: "$155.00/kg".
func MaterialPrice(m store.Material) string {
	return m.PriceMicros.ToCentavosHalfUp().String() + "/" + m.UnitCode
}
