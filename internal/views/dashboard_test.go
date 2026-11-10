package views

import (
	"strings"
	"testing"
	"time"

	"github.com/gerrygoo/cladex-web/internal/store"
)

func TestSortPendientes(t *testing.T) {
	at := func(s string) time.Time { tt, _ := time.Parse(time.RFC3339, s); return tt }
	seg := func(day string, late int) PendienteItem {
		return PendienteItem{Period: &store.SeguimientoPeriod{Folio: day, DaysLate: late, OpenedAt: at(day + "T06:00:00Z")}}
	}
	lib := func(body, priority, created string) PendienteItem {
		return PendienteItem{Pendiente: &store.Pendiente{Body: body, Priority: priority, CreatedAt: created}}
	}
	items := []PendienteItem{
		lib("baja", "baja", "2026-11-01T10:00:00.000Z"),
		seg("2026-11-11", 1), // media, opened the 12th
		lib("media vieja", "media", "2026-11-02T10:00:00.000Z"),
		seg("2026-11-04", 4), // muy alta
		lib("muy alta", "muy_alta", "2026-11-10T10:00:00.000Z"),
		seg("2026-11-09", 2), // alta
	}
	name := func(it PendienteItem) string {
		if it.Period != nil {
			return it.Period.Folio
		}
		return it.Pendiente.Body
	}
	names := func() string {
		var out []string
		for _, it := range items {
			out = append(out, name(it))
		}
		return strings.Join(out, ", ")
	}
	SortPendientes(items, "desc")
	if got, want := names(), "2026-11-04, muy alta, 2026-11-09, media vieja, 2026-11-11, baja"; got != want {
		t.Errorf("desc = %s, want %s", got, want)
	}
	SortPendientes(items, "asc")
	if got, want := names(), "baja, media vieja, 2026-11-11, 2026-11-09, 2026-11-04, muy alta"; got != want {
		t.Errorf("asc = %s, want %s", got, want)
	}
}

// The words docs/guia/dashboard.md quotes for how a seguimiento was completed and for
// the priorities.
func TestGuiaDashboardWords(t *testing.T) {
	for reason, want := range map[string]string{
		store.ClosedComentario: "con un comentario", store.ClosedNuevaFecha: "con una nueva fecha", store.ClosedCambioEtapa: "por cambio de etapa",
	} {
		if got := closedReasonText(reason); got != want {
			t.Errorf("closedReasonText(%s) = %q, want %q", reason, got, want)
		}
	}
	for late, want := range map[int]string{1: "Media", 2: "Alta", 3: "Muy alta", 7: "Muy alta"} {
		if got := priorityText(store.SeguimientoPeriod{DaysLate: late}.Priority()); got != want {
			t.Errorf("day %d: %q, want %q", late, got, want)
		}
	}
	if got := daysLateText(1) + " / " + daysLateText(3); got != "1 día hábil / 3 días hábiles" {
		t.Errorf("daysLateText = %q", got)
	}
}
