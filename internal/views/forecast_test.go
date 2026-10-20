package views

import (
	"strings"
	"testing"
	"time"

	"github.com/gerrygoo/cladex-web/internal/store"
)

func TestForecastWarnings(t *testing.T) {
	const today = "2026-10-18"
	p := func(followUp, oc, valid string) store.Project {
		return store.Project{Status: "prospecto", NextFollowUpDate: followUp, ExpectedOCDate: oc, QuoteValidUntil: valid}
	}

	for _, c := range []struct {
		followUp, want string
	}{
		{"", "Sin fecha"}, {"2026-10-17", "Vencido"}, {"2026-10-18", "Vencido"}, {"2026-10-19", ""},
	} {
		if got := FollowUpWarning(p(c.followUp, "", ""), today); got != c.want {
			t.Errorf("FollowUpWarning(%q) = %q, want %q", c.followUp, got, c.want)
		}
	}
	// An expected O.C. only warns once its day has passed; no date is not a warning.
	for _, c := range []struct{ oc, want string }{{"", ""}, {"2026-10-17", "Vencida"}, {"2026-10-18", ""}, {"2026-10-30", ""}} {
		if got := OCWarning(p("", c.oc, ""), today); got != c.want {
			t.Errorf("OCWarning(%q) = %q, want %q", c.oc, got, c.want)
		}
	}
	// The vigencia warns from five days out, and a draft (no vigencia) never does.
	for _, c := range []struct{ valid, want string }{
		{"", ""}, {"2026-10-24", ""}, {"2026-10-23", "Vence en 5 días"}, {"2026-10-20", "Vence en 2 días"},
		{"2026-10-19", "Vence mañana"}, {"2026-10-18", "Vence hoy"}, {"2026-10-17", "Vencida"},
	} {
		if got := VigenciaWarning(p("", "", c.valid), today); got != c.want {
			t.Errorf("VigenciaWarning(%q) = %q, want %q", c.valid, got, c.want)
		}
	}
}

func TestForecastTotalsAndDays(t *testing.T) {
	f := Forecast{Projects: []store.Project{
		{Subtotal: 10000, Profit: 2500, Total: 11600, Probability: 50},
		{Subtotal: 30000, Profit: 3000, Total: 34800, Probability: 90},
	}}
	got := f.Totals()
	// 25 of 100 and 30 of 300 is 55 of 400 = 13.75%, not the 17.5% average of 25% and 10%.
	if got.Price != 40000 || got.Profit != 5500 || got.Cost != 34500 || got.Margin != "13.8%" {
		t.Errorf("Totals = %+v", got)
	}
	if m := (Forecast{}).Totals().Margin; m != "—" {
		t.Errorf("empty margin = %q, want —", m)
	}
	// Days since the last news, from an ISO timestamp, never negative.
	for _, c := range []struct{ at, want string }{
		{"2026-10-18T09:00:00.000Z", "0"}, {"2026-10-11T23:59:00.000Z", "7"}, {"2026-10-20T00:00:00.000Z", "0"}, {"", ""},
	} {
		if got := DaysWithoutUpdate(store.Project{LastMovementAt: c.at}, "2026-10-18"); got != c.want {
			t.Errorf("DaysWithoutUpdate(%q) = %q, want %q", c.at, got, c.want)
		}
	}
}

func TestHomeSellerTable(t *testing.T) {
	d := &store.Dashboard{Sellers: []store.SellerRow{
		{Name: "Emilio Flores", Projects: 2, Pipeline: 1_500_00, Profit: 270_00, Untracked: []string{"QA0002", "QA0007"}, UntrackedAmount: 500_00},
		{Name: "Rodolfo Flores"},
	}}
	home := render(t, Home(time.Time{}, time.Time{}, "", d, &NavUser{Username: "ana"}))
	assertContains(t, home, "<h2>Vendedores</h2>", "Monto sin seguimiento", "O.C. vencidas",
		"18.0% · $270.00", ">—<", `<a href="/proyectos/QA0007">QA0007</a>`, `<details class="folio-list">`)
	for _, gone := range []string{"Proyectos por etapa", "Perdidos"} {
		if strings.Contains(home, gone) {
			t.Errorf("home still shows %q", gone)
		}
	}
	// A zero is plain text, with nothing to open.
	if strings.Count(home, `<details class="folio-list">`) != 1 {
		t.Error("only a count above zero opens a list of folios")
	}
}
