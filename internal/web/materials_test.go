package web

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/store"
)

// seedCCSProduct creates a CCS & AC product costed only from its CCS 30% content
// (0.1723 kg/m, as ALAMBRE 4), returning its id and the seeded material.
func seedCCSProduct(t *testing.T, a *Auth) (int64, store.Material) {
	t.Helper()
	ctx := context.Background()
	familyID, err := a.store.UpsertFamily(ctx, "CCS & AC", "")
	if err != nil {
		t.Fatalf("UpsertFamily: %v", err)
	}
	id, err := a.store.CreateProduct(ctx, store.Product{FamilyID: familyID, SKU: "ccs-alambre-4", Description: "Cable CCS 30% ALAMBRE 4"})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	materials, err := a.store.ListMaterials(ctx)
	if err != nil || len(materials) == 0 {
		t.Fatalf("ListMaterials = %+v, %v", materials, err)
	}
	ccs := materials[0]
	if _, err := a.store.AddProductMaterial(ctx, id, ccs.ID, money.Micros(172_300)); err != nil {
		t.Fatalf("AddProductMaterial: %v", err)
	}
	return id, ccs
}

// A CCS & AC line prices from its material content and the quote's margin, follows a
// material price change while a draft, and freezes the material in pricing_inputs.
func TestQuoteMaterialPricing(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	customerID, _, _ := seedQuoteBuilderFixtures(t, a)
	ccsProductID, ccs := seedCCSProduct(t, a)
	ctx := context.Background()
	ccsMargin := marginOption(t, a, "CCS & AC")

	draft, _ := a.store.CreateDraftQuote(ctx, customerID, userID, "QS")
	form := costLineForm(ccsProductID, ccsMargin.ID)
	rec := doForm(t, a, userID, q.Guardar, "POST", "/cotizaciones/"+draft.Folio+"/guardar",
		map[string]string{"folio": draft.Folio}, form, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Guardar status = %d; body = %s", rec.Code, rec.Body.String())
	}

	// 0.1723 kg × $160 = $27.568; ÷ (1 − 0.2955) = $39.13.
	body := builderBody(t, a, q, userID, draft.Folio)
	for _, want := range []string{"39.13", "CCS 30% $160.00/kg (actualizado hoy)"} {
		if !strings.Contains(body, want) {
			t.Errorf("builder missing %q", want)
		}
	}

	// Drafts follow the material: $200/kg → 0.1723 × 200 / 0.7045 = $48.91.
	if err := a.store.UpdateMaterial(ctx, ccs.ID, ccs.Name, 200_000_000, userID); err != nil {
		t.Fatalf("UpdateMaterial: %v", err)
	}
	if body := builderBody(t, a, q, userID, draft.Folio); !strings.Contains(body, "48.91") {
		t.Errorf("draft didn't follow the material price to $48.91")
	}

	rec = doForm(t, a, userID, q.Emitir, "POST", "/cotizaciones/"+draft.Folio+"/emitir",
		map[string]string{"folio": draft.Folio}, form, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Emitir status = %d; body = %s", rec.Code, rec.Body.String())
	}
	lines, err := a.store.ListQuoteLines(ctx, draft.ID)
	if err != nil || len(lines) != 1 || lines[0].PricingInputsJSON == nil {
		t.Fatalf("ListQuoteLines = %+v, %v", lines, err)
	}
	want := `"materials":[{"material":"CCS 30%","qty_per_unit":"0.1723","unit":"kg","price":"200"}]`
	if got := *lines[0].PricingInputsJSON; !strings.Contains(got, want) || !strings.Contains(got, `"margin":"0.2955"`) {
		t.Errorf("pricing_inputs = %s, want %s and margin 0.2955", got, want)
	}

	// Issued: a later material change moves nothing.
	if err := a.store.UpdateMaterial(ctx, ccs.ID, ccs.Name, 999_000_000, userID); err != nil {
		t.Fatalf("UpdateMaterial: %v", err)
	}
	if body := builderBody(t, a, q, userID, draft.Folio); !strings.Contains(body, "48.91") {
		t.Errorf("issued quote moved with the material price")
	}
}

func TestAjustesMateriales(t *testing.T) {
	a := newTestAuth(t)
	s := NewSettings(a.store)
	adminID := createTestUser(t, a, "ana", "admin", "hunter2")
	vendedorID := createTestUser(t, a, "rodolfo", "vendedor", "hunter2")
	ctx := context.Background()

	post := func(userID int64, h http.HandlerFunc, path string, pathValues map[string]string, form url.Values) (int, string) {
		t.Helper()
		rec := doForm(t, a, userID, a.RequireAdmin(h).ServeHTTP, "POST", path, pathValues, form, false)
		return rec.Code, rec.Body.String()
	}
	units, _ := a.store.ListUnits(ctx)
	kg := ""
	for _, u := range units {
		if u.Code == "kg" {
			kg = strconv.FormatInt(u.ID, 10)
		}
	}

	if code, _ := post(vendedorID, s.CreateMaterial, "/ajustes/materiales", nil, url.Values{"name": {"Cobre"}, "unit_id": {kg}, "price": {"180"}}); code != http.StatusForbidden {
		t.Fatalf("vendedor CreateMaterial status = %d, want 403", code)
	}
	if code, body := post(adminID, s.CreateMaterial, "/ajustes/materiales", nil, url.Values{"name": {"Cobre"}, "unit_id": {kg}, "price": {"180.50"}}); code != http.StatusSeeOther {
		t.Fatalf("CreateMaterial status = %d; body = %s", code, body)
	}
	for _, c := range []struct {
		form url.Values
		want string
	}{
		{url.Values{"name": {"Cobre"}, "unit_id": {kg}, "price": {"1"}}, "Ya existe un material con ese nombre."},
		{url.Values{"name": {"Aluminio"}, "unit_id": {kg}, "price": {"-1"}}, "Precio de material inválido"},
		{url.Values{"name": {""}, "unit_id": {kg}, "price": {"1"}}, "El nombre del material es obligatorio."},
		{url.Values{"name": {"Aluminio"}, "price": {"1"}}, "Selecciona la unidad del material."},
	} {
		code, body := post(adminID, s.CreateMaterial, "/ajustes/materiales", nil, c.form)
		if code != http.StatusUnprocessableEntity || !strings.Contains(body, c.want) {
			t.Errorf("CreateMaterial(%v) = %d, want 422 with %q", c.form, code, c.want)
		}
	}

	materials, _ := a.store.ListMaterials(ctx)
	var cobre store.Material
	for _, m := range materials {
		if m.Name == "Cobre" {
			cobre = m
		}
	}
	if cobre.PriceMicros != 180_500_000 || cobre.UnitCode != "kg" {
		t.Fatalf("created material = %+v", cobre)
	}
	idPath := map[string]string{"id": strconv.FormatInt(cobre.ID, 10)}
	if code, _ := post(adminID, s.UpdateMaterial, "/ajustes/materiales/"+idPath["id"], idPath, url.Values{"name": {"Cobre"}, "price": {"190"}}); code != http.StatusSeeOther {
		t.Fatalf("UpdateMaterial status = %d", code)
	}
	materials, _ = a.store.ListMaterials(ctx)
	for _, m := range materials {
		if m.ID == cobre.ID && m.PriceMicros != 190_000_000 {
			t.Fatalf("updated material price = %v, want 190", m.PriceMicros)
		}
	}
}

func TestProductMaterialsSection(t *testing.T) {
	a := newTestAuth(t)
	p := NewProducts(a.store)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	_, _, costProductID := seedQuoteBuilderFixtures(t, a)
	ctx := context.Background()
	materials, _ := a.store.ListMaterials(ctx)
	ccs := materials[0]
	pid := strconv.FormatInt(costProductID, 10)
	path := map[string]string{"id": pid}

	add := func(form url.Values) (int, string) {
		rec := doForm(t, a, userID, p.AddMaterial, "POST", "/productos/"+pid+"/materiales", path, form, false)
		return rec.Code, rec.Body.String()
	}
	if code, body := add(url.Values{"material_id": {strconv.FormatInt(ccs.ID, 10)}, "qty": {"0.25"}}); code != http.StatusSeeOther {
		t.Fatalf("AddMaterial status = %d; body = %s", code, body)
	}
	for _, c := range []struct {
		form url.Values
		want string
	}{
		{url.Values{"material_id": {strconv.FormatInt(ccs.ID, 10)}, "qty": {"1"}}, "Este producto ya tiene ese material"},
		{url.Values{"material_id": {strconv.FormatInt(ccs.ID, 10)}, "qty": {"0"}}, "Cantidad inválida"},
		{url.Values{"qty": {"1"}}, "Selecciona un material."},
	} {
		code, body := add(c.form)
		if code != http.StatusUnprocessableEntity || !strings.Contains(body, c.want) {
			t.Errorf("AddMaterial(%v) = %d, want 422 with %q", c.form, code, c.want)
		}
	}

	rec := doForm(t, a, userID, p.EditPage, "GET", "/productos/"+pid, path, nil, false)
	if !strings.Contains(rec.Body.String(), "0.25 kg") || !strings.Contains(rec.Body.String(), "$160.00/kg") {
		t.Errorf("edit page doesn't list the material: %s", rec.Body.String())
	}

	content, _ := a.store.ListProductMaterials(ctx, costProductID)
	mid := strconv.FormatInt(content[0].ID, 10)
	rec = doForm(t, a, userID, p.DeleteMaterial, "POST", "/productos/"+pid+"/materiales/"+mid+"/eliminar",
		map[string]string{"id": pid, "mid": mid}, url.Values{}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("DeleteMaterial status = %d", rec.Code)
	}
	if content, _ := a.store.ListProductMaterials(ctx, costProductID); len(content) != 0 {
		t.Fatalf("material still listed after delete: %+v", content)
	}
}
