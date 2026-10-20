package store

import (
	"testing"

	"github.com/gerrygoo/cladex-web/internal/money"
)

// TestCostChangeNote pins issue #27's history note: lines pair by description even when
// moved, each changed cost says by how much it went up or down, and lines added or
// removed with a cost are listed; catalog lines and unchanged costs say nothing.
func TestCostChangeNote(t *testing.T) {
	cost := func(pesos int64) *money.Micros { c := money.Micros(pesos * 1_000_000); return &c }
	product := int64(7)
	prev := []QuoteLine{
		{DescriptionSnapshot: "Cable", CostMicros: cost(1000)},
		{DescriptionSnapshot: "Flete", CostMicros: cost(400)},
		{DescriptionSnapshot: "Viejo", CostMicros: cost(50)},
		{DescriptionSnapshot: "Sin cambio", CostMicros: cost(10)},
		{DescriptionSnapshot: "Catálogo", ProductID: &product},
	}
	next := []QuoteLine{
		{DescriptionSnapshot: "Flete", CostMicros: cost(300)},
		{DescriptionSnapshot: "Cable", CostMicros: cost(1250)},
		{DescriptionSnapshot: "Sin cambio", CostMicros: cost(10)},
		{DescriptionSnapshot: "Nuevo", CostMicros: cost(20)},
		{DescriptionSnapshot: "Catálogo", ProductID: &product},
	}
	want := "Costos cambiados respecto a QL0001:\n" +
		"Partida 1 «Flete»: costo unitario $400.00 → $300.00 (bajó $100.00, 25.0%).\n" +
		"Partida 2 «Cable»: costo unitario $1,000.00 → $1,250.00 (subió $250.00, 25.0%).\n" +
		"Partida 4 (nueva) «Nuevo»: costo unitario $20.00.\n" +
		"Se quitó la partida 3 de QL0001 «Viejo» (costo unitario $50.00)."
	if got := costChangeNote("QL0001", prev, next); got != want {
		t.Errorf("costChangeNote =\n%s\nwant\n%s", got, want)
	}
	if got := costChangeNote("QL0001", prev, prev); got != "" {
		t.Errorf("no change gave a note: %q", got)
	}
}
