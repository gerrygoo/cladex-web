package views

import (
	"sort"
	"strconv"
	"strings"

	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/store"
)

// ProjectEvent is a dated thing coming up on a prospecto: a follow-up the salesperson
// set, the purchase order they expect, or the quote's vigencia running out.
type ProjectEvent struct {
	Label string
	Date  string // YYYY-MM-DD
}

// projectEvents lists a prospecto's dated events, soonest first, leaving out the ones
// that have no date.
func projectEvents(p store.Project) []ProjectEvent {
	var events []ProjectEvent
	for _, e := range []ProjectEvent{
		{"Seguimiento", p.NextFollowUpDate},
		{"O.C. esperada", p.ExpectedOCDate},
		{"Vence la vigencia", p.QuoteValidUntil},
	} {
		if e.Date != "" {
			events = append(events, e)
		}
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].Date < events[j].Date })
	return events
}

// NextProjectEvent is the prospecto's soonest event from today on, or nil when it has
// none, and OverdueProjectEvents the ones whose day already passed. today is YYYY-MM-DD.
func NextProjectEvent(p store.Project, today string) *ProjectEvent {
	for _, e := range projectEvents(p) {
		if e.Date >= today {
			return &e
		}
	}
	return nil
}

func OverdueProjectEvents(p store.Project, today string) []ProjectEvent {
	var out []ProjectEvent
	for _, e := range projectEvents(p) {
		if e.Date < today {
			out = append(out, e)
		}
	}
	return out
}

func projectFilePath(folio string, fileID int64) string {
	return projectPath(folio) + "/archivos/" + strconv.FormatInt(fileID, 10)
}

// paymentMethodText is a forma de pago with what its initials stand for.
func paymentMethodText(method string) string {
	switch method {
	case "PUE":
		return "P.U.E. · pago en una sola exhibición"
	case "PPD":
		return "P.P.D. · pago en parcialidades o diferido"
	}
	return method
}

// ocDateValue is the purchase order form's date: the one on record, or today for a
// proyecto that has none yet.
func ocDateValue(p store.Project, today string) string {
	if p.OC.Date != "" {
		return p.OC.Date
	}
	return today
}

// fileSizeText is a file size as people read it: "84 KB", "2.3 MB".
func fileSizeText(size int64) string {
	if size < 1<<20 {
		return strconv.FormatInt(max(1, (size+512)>>10), 10) + " KB"
	}
	return strconv.FormatFloat(float64(size)/(1<<20), 'f', 1, 64) + " MB"
}

// joinNonEmpty joins the parts that have something in them.
func joinNonEmpty(sep string, parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, sep)
}

// Forecast is the Pronóstico page's data: the prospectos shown, whether that is every
// prospecto or only the ones relevante para pronóstico, and the viewer's today.
type Forecast struct {
	Projects []store.Project
	All      bool
	Today    string
}

// Total is what the listed proyectos add up to, and Weighted their expected amount:
// each total times its probability.
func (f Forecast) Total() (total, weighted string) {
	var sum, weight int64
	for _, p := range f.Projects {
		sum += int64(p.Total)
		weight += p.Weight()
	}
	return money.Centavos(sum).String(), store.WeightedTotal(weight).String()
}

// forecastReturn is the "volver" value a follow-up form on the Pronóstico page sends, so
// saving comes back to the same list.
func (f Forecast) forecastReturn() string {
	if f.All {
		return "pronostico-todos"
	}
	return "pronostico"
}
