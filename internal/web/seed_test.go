package web

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/store"
)

// SeedStaging adds the test data to a database shaped like production's (the CCA, CCS &
// AC and ABASTILUM familias with products to copy), through the same code as the
// screens, and does nothing the second time.
func TestSeedStaging(t *testing.T) {
	a := newTestAuth(t)
	ctx := context.Background()
	admin := createTestUser(t, a, "gerry", "admin", "hunter2")
	_, _, ccaID := seedQuoteBuilderFixtures(t, a)
	cca, _ := a.store.ProductByID(ctx, ccaID)
	kg, ccaCost := money.Micros(318_270), money.Micros(45_000_000)
	if _, err := a.store.CreateProduct(ctx, store.Product{FamilyID: cca.FamilyID, SKU: "CCA-C12", Description: "Cable CCA 12",
		CostMicros: &ccaCost, KgPerMMicros: &kg}); err != nil {
		t.Fatal(err)
	}
	seedCCSProduct(t, a)
	seedPricingSettings(t, a, admin)
	abl, err := a.store.UpsertFamily(ctx, "ABASTILUM", "")
	if err != nil {
		t.Fatal(err)
	}
	cost := money.Micros(1_200_000_000)
	if _, err := a.store.CreateProduct(ctx, store.Product{FamilyID: abl, SKU: "ABL-FL-50w", Description: "Luminaria 50 W", CostMicros: &cost}); err != nil {
		t.Fatal(err)
	}

	// A seed that failed halfway left a user and a customer behind: the next run reuses them.
	if _, err := a.store.CreateUser(ctx, "prueba-admin", "Prueba Administrador", "x", store.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.CreateCustomer(ctx, seedCustomers[0]); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	var out bytes.Buffer
	if err := SeedStaging(ctx, a.store, now, &out); err != nil {
		t.Fatalf("SeedStaging: %v\n%s", err, out.String())
	}
	for _, want := range []string{"prueba-admin (admin)", "prueba-vend3 (vendedor)", "contraseña para todos: ", "PRUEBA-CCS-KGM",
		"prospecto con seguimiento vencido", "lista para facturar", "falta complemento", "perdido", "cotización en borrador"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
	customers, err := a.store.ListCustomers(ctx, SeedPrefix, "", "", nil)
	if err != nil || len(customers) != 5 {
		t.Errorf("seeded customers = %d, want 5", len(customers))
	}

	byState := map[string]int{}
	var qs *store.Project
	projects, _ := a.store.ListProjects(ctx, "", "", "", store.Filters{"etapa": {store.StageProspecto, store.StageEnCurso, store.StagePerdido}})
	for i := range projects {
		p := projects[i]
		key := p.Status
		if p.Status == store.StageEnCurso {
			key += "/" + p.States.Facturacion + "/" + p.States.Entrega + "/" + p.States.Cobro
			if p.States.Entrega == store.EntregaEnProceso {
				qs = &p
			}
		}
		byState[key]++
	}
	for _, want := range []string{store.StageProspecto, store.StagePerdido,
		"en_curso/pendiente/en_proceso/pendiente", "en_curso/pendiente_complemento/sin_oc/parcial"} {
		if byState[want] == 0 {
			t.Errorf("no seeded proyecto in %s: %v", want, byState)
		}
	}
	if byState[store.StageProspecto] != 2 {
		t.Errorf("prospectos = %d, want 2: %v", byState[store.StageProspecto], byState)
	}

	// The proyecto ready to invoice gets its KGM line in kilos.
	if qs == nil {
		t.Fatal("no proyecto with a supplier O.C.")
	}
	fid, err := a.store.CreateFacturaDraft(ctx, qs.ID, admin)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := a.store.ComprobanteByID(ctx, fid)
	if len(c.Conceptos) != 2 || c.Conceptos[0].SATUnitCode != "KGM" || c.Conceptos[0].QtyMilli != 172_300 ||
		c.Conceptos[1].SATUnitCode != "MTR" || c.Conceptos[1].QtyMilli != 250_000 {
		t.Errorf("seeded QS factura conceptos = %+v", c.Conceptos)
	}

	out.Reset()
	if err := SeedStaging(ctx, a.store, now, &out); err != nil || !strings.Contains(out.String(), "ya tiene los datos de prueba") {
		t.Errorf("second SeedStaging = %v, %q", err, out.String())
	}
}
