package views

import (
	"math"
	"strconv"

	"github.com/gerrygoo/cladex-web/internal/store"
)

// Equipo is the admins' view of the team's seguimiento (issue #70).
type Equipo struct {
	Team *store.Team
	// Periodo is "semana", "mes" or "rango"; Desde and Hasta the days it covers.
	Periodo      string
	Desde, Hasta string
	// MinProb is the probabilidad mínima, 0 for all.
	MinProb int
	// Error explains a range that couldn't be used.
	Error string
	// LV carries the Persona column filter and the rest of the page's state.
	LV ListView
}

// equipoPeriods are the Periodo choices.
var equipoPeriods = []struct{ Value, Label string }{
	{"semana", "Esta semana"}, {"mes", "Este mes"}, {"rango", "Rango de fechas"},
}

// minProbabilityLabel is a probabilidad mínima's choice ("Alta 75% o más").
func minProbabilityLabel(p int) string {
	if p == store.ProbabilitySteps[len(store.ProbabilitySteps)-1].Percent {
		return probabilityText(p)
	}
	return store.ProbabilityLabel(p) + " " + strconv.Itoa(p) + "% o más"
}

// personaFilter is the Persona column's choices: everyone the view can show.
func personaFilter(people []store.TeamMember) ColumnFilter {
	opts := make([]FilterOption, len(people))
	for i, p := range people {
		opts[i] = FilterOption{Value: p.Username, Label: p.Name}
	}
	return ColumnFilter{Kind: store.FilterText, Options: opts}
}

// onTimeText is the share of seguimientos a tiempo: "67% (2 de 3)", or "—" when no
// follow-up day came due.
func onTimeText(m store.TeamMember) string {
	share, ok := m.OnTimeShare()
	if !ok {
		return "—"
	}
	return strconv.Itoa(int(share*100+0.5)) + "% (" + strconv.Itoa(m.OnTime) + " de " + strconv.Itoa(m.DueDays) + ")"
}

// averageLateText is the average lateness to one decimal, "1.5 días hábiles", or "—" when nothing was
// resolved in the range.
func averageLateText(m store.TeamMember) string {
	avg, ok := m.AverageLate()
	if !ok {
		return "—"
	}
	avg = math.Round(avg*10) / 10
	if avg == 1 {
		return "1 día hábil"
	}
	return strconv.FormatFloat(avg, 'f', -1, 64) + " días hábiles"
}
