package store

import (
	"fmt"
	"strings"

	"github.com/gerrygoo/cladex-web/internal/money"
)

// costChangeNote is the history note a revision's issue leaves when its lines' costs
// differ from the quote it revises (issue #27): costs only change through a revision,
// and the proyecto's history says by how much, line by line. Lines are paired by
// description, in order, so a line moved up or down still pairs; one whose description
// changed reads as removed and added. Only lines with a cost of their own (free lines)
// count: a catalog line's cost is the catalog's. Returns "" when no cost changed.
func costChangeNote(prevFolio string, prev, next []QuoteLine) string {
	used := make([]bool, len(prev))
	var changes []string
	for _, n := range next {
		if n.ProductID != nil {
			continue
		}
		match := -1
		for i, p := range prev {
			if !used[i] && p.ProductID == nil && p.DescriptionSnapshot == n.DescriptionSnapshot {
				match = i
				break
			}
		}
		if match < 0 {
			if n.CostMicros != nil {
				changes = append(changes, fmt.Sprintf("Partida nueva «%s»: costo unitario %s.", n.DescriptionSnapshot, costText(n.CostMicros)))
			}
			continue
		}
		used[match] = true
		if line := costChangeLine(n.DescriptionSnapshot, prev[match].CostMicros, n.CostMicros); line != "" {
			changes = append(changes, line)
		}
	}
	for i, p := range prev {
		if !used[i] && p.ProductID == nil && p.CostMicros != nil {
			changes = append(changes, fmt.Sprintf("Se quitó la partida «%s» (costo unitario %s).", p.DescriptionSnapshot, costText(p.CostMicros)))
		}
	}
	if len(changes) == 0 {
		return ""
	}
	return "Costos cambiados respecto a " + prevFolio + ":\n" + strings.Join(changes, "\n")
}

// costChangeLine says how one line's unit cost moved, or "" if it didn't (to the centavo).
func costChangeLine(desc string, before, after *money.Micros) string {
	switch {
	case before == nil && after == nil:
		return ""
	case before == nil:
		return fmt.Sprintf("«%s»: sin costo → %s.", desc, costText(after))
	case after == nil:
		return fmt.Sprintf("«%s»: %s → sin costo.", desc, costText(before))
	}
	b, a := before.ToCentavosHalfUp(), after.ToCentavosHalfUp()
	if a == b {
		return ""
	}
	verb, diff := "subió", a-b
	if diff < 0 {
		verb, diff = "bajó", -diff
	}
	pct := ""
	if b != 0 {
		pct = fmt.Sprintf(", %.1f%%", float64(diff)*100/float64(b))
	}
	return fmt.Sprintf("«%s»: costo unitario %s → %s (%s %s%s).", desc, b, a, verb, diff, pct)
}

func costText(c *money.Micros) string {
	return c.ToCentavosHalfUp().String()
}
