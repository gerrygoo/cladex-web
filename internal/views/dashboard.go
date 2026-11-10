package views

import (
	"sort"
	"strconv"
	"time"

	"github.com/gerrygoo/cladex-web/internal/store"
)

// MisPendientes is the home page (issue #72): the user's open seguimientos and
// pendientes libres in one list, what they completed in the last 30 days, and the
// Vendedores table below.
type MisPendientes struct {
	Items []PendienteItem // in the order shown
	Done  []DoneItem      // newest first
	// Dir is the priority order: "desc" (muy alta first, the default) or "asc".
	Dir string
	// Adding shows the new pendiente's row (?agregar=1, or after a failed save); Body,
	// Priority and Errors refill it.
	Adding   bool
	Body     string
	Priority string
	Errors   map[string]string
	// Announce is read out after an htmx action ("Pendiente completado.").
	Announce string
	Today    string // YYYY-MM-DD, Mexico City
	Sales    *store.Dashboard
	Built    time.Time
	Now      time.Time
	BuildSHA string
}

// PendienteItem is one row of Mis pendientes: a seguimiento (Period) or a pendiente
// libre (Pendiente).
type PendienteItem struct {
	Period    *store.SeguimientoPeriod
	Pendiente *store.Pendiente
}

// Priority is the row's priority key.
func (it PendienteItem) Priority() string {
	if it.Period != nil {
		return it.Period.Priority()
	}
	return it.Pendiente.Priority
}

// since is when the row started, for "oldest first" within a priority: a seguimiento's
// day, a pendiente's creation.
func (it PendienteItem) since() string {
	if it.Period != nil {
		return it.Period.OpenedAt.UTC().Format(time.RFC3339)
	}
	if t, ok := parseStoredTime(it.Pendiente.CreatedAt); ok {
		return t.UTC().Format(time.RFC3339)
	}
	return it.Pendiente.CreatedAt
}

// SortPendientes orders items by priority, highest first for "desc" and lowest first
// otherwise, and oldest first within a priority.
func SortPendientes(items []PendienteItem, dir string) {
	sort.SliceStable(items, func(i, j int) bool {
		ri, rj := store.PriorityRank(items[i].Priority()), store.PriorityRank(items[j].Priority())
		if ri != rj {
			if dir == "asc" {
				return ri < rj
			}
			return ri > rj
		}
		return items[i].since() < items[j].since()
	})
}

// DoneItem is one row of Completados: a closed seguimiento or a completed pendiente.
type DoneItem struct {
	Period    *store.SeguimientoPeriod
	Pendiente *store.Pendiente
	At        time.Time
}

// SortDone orders Completados newest first.
func SortDone(items []DoneItem) {
	sort.SliceStable(items, func(i, j int) bool { return items[i].At.After(items[j].At) })
}

// priorityText is a priority as the list shows it ("Muy alta").
func priorityText(p string) string { return store.PriorityLabel(p) }

// priorityClass is the CSS class that colours a priority's text.
func priorityClass(p string) string { return "prioridad prioridad-" + p }

// closedReasonText says why a seguimiento left the list.
func closedReasonText(reason string) string {
	switch reason {
	case store.ClosedComentario:
		return "con un comentario"
	case store.ClosedNuevaFecha:
		return "con una nueva fecha"
	case store.ClosedCambioEtapa:
		return "por cambio de etapa"
	}
	return ""
}

// daysLateText is n weekdays late in words.
func daysLateText(n int) string {
	if n == 1 {
		return "1 día hábil"
	}
	return strconv.Itoa(n) + " días hábiles"
}

// mxDayText is a stored timestamp's Mexico City day, as 10/11/2026.
func mxDayText(iso string) string {
	if t, ok := parseStoredTime(iso); ok {
		return t.In(store.MexicoCity()).Format("02/01/2006")
	}
	return iso
}

// mxTimeText is t in Mexico City, as 10/11/2026 - 16:42.
func mxTimeText(t time.Time) string { return t.In(store.MexicoCity()).Format(timestampLayout) }

// pendienteIDField is a pendiente's form field id, stable across htmx swaps so focus
// comes back to the control the user used.
func pendienteIDField(kind string, id int64) string {
	return kind + "-" + strconv.FormatInt(id, 10)
}

func pendientePath(id int64, action string) string {
	return "/pendientes/" + strconv.FormatInt(id, 10) + "/" + action
}
