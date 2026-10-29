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

// A CCS quote shows each line's weight, Kg por metro from the kg it was priced with and
// Kg from the quantity, on the builder, the issued page and the PDF (issue #34). Other
// series don't.
func TestQuoteCCSShowsKg(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	customerID, flatProductID, _ := seedQuoteBuilderFixtures(t, a)
	ccsProductID, _ := seedCCSProduct(t, a) // 0.1723 kg of CCS 30% per m
	ctx := context.Background()
	ccsMargin := marginOption(t, a, "CCS & AC")

	draft, _ := a.store.CreateDraftQuote(ctx, customerID, userID, "QS")
	form := costLineForm(ccsProductID, ccsMargin.ID)
	form.Set("lines[0][qty]", "1250")
	rec := doForm(t, a, userID, q.Recalcular, "POST", "/cotizaciones/"+draft.Folio+"/recalcular",
		map[string]string{"folio": draft.Folio}, form, true)
	body := rec.Body.String()
	// 1,250 m × 0.1723 kg/m = 215.375 kg, rounded to 215.38.
	// The weight table is centered (issue #34); app.css keys it on kg-lines.
	for _, want := range []string{`<table class="kg-lines">`, "Cantidad (m)", "Kg por metro", "<td class=\"num\">0.1723</td>", "<td class=\"num\">215.38</td>"} {
		if !strings.Contains(body, want) {
			t.Errorf("Recalcular missing %q: %s", want, body)
		}
	}

	rec = doForm(t, a, userID, q.Emitir, "POST", "/cotizaciones/"+draft.Folio+"/emitir",
		map[string]string{"folio": draft.Folio}, form, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Emitir status = %d; body = %s", rec.Code, rec.Body.String())
	}
	if body := builderBody(t, a, q, userID, draft.Folio); !strings.Contains(body, "Kg por metro") || !strings.Contains(body, "215.38") {
		t.Errorf("issued page lost the weights: %s", body)
	}
	issued, err := a.store.QuoteByFolio(ctx, draft.Folio)
	if err != nil || issued == nil {
		t.Fatalf("QuoteByFolio: %v", err)
	}
	persisted, err := a.store.ListQuoteLines(ctx, issued.ID)
	if err != nil {
		t.Fatalf("ListQuoteLines: %v", err)
	}
	doc, err := issuedQuoteDocument(*issued, persisted)
	if err != nil {
		t.Fatalf("issuedQuoteDocument: %v", err)
	}
	if !doc.ShowKg || len(doc.Lines) != 1 || doc.Lines[0].KgPerUnit != "0.1723" || doc.Lines[0].Kg != "215.38" {
		t.Errorf("PDF document = %+v, want the weights", doc)
	}

	other, _ := a.store.CreateDraftQuote(ctx, customerID, userID, "QA")
	rec = doForm(t, a, userID, q.Recalcular, "POST", "/cotizaciones/"+other.Folio+"/recalcular",
		map[string]string{"folio": other.Folio}, costLineForm(flatProductID, ccsMargin.ID), true)
	if strings.Contains(rec.Body.String(), "Kg por metro") || strings.Contains(rec.Body.String(), "kg-lines") {
		t.Error("a QA quote shows weights")
	}
}

// A QG quote prices like a CCS one, per metre from kg (issue #37): the product's cost is
// per kg, a metre holds its Peso of it, and the quote's margin applies on top. The line
// shows its weight, the PDF has the CCS columns, the proyecto earns the margin, and a
// product without a Peso can't be quoted.
func TestQuoteQGPricesPerMetreFromKg(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	customerID, _, _ := seedQuoteBuilderFixtures(t, a)
	ctx := context.Background()
	familyID := createQGFamily(t, a, "Cable Alumoclad 7#8 AWG hilo de guarda", "Precios en pesos mexicanos (MXN), no incluyen IVA")
	cost, kg := money.Micros(95_000_000), money.Micros(389_600)
	var kgUnit *int64
	units, _ := a.store.ListUnits(ctx)
	for _, u := range units {
		if u.Code == "kg" {
			kgUnit = &u.ID
		}
	}
	// Catalogued in kilos, as production has it; quoted (and invoiced) per metre.
	alcID, err := a.store.CreateProduct(ctx, store.Product{FamilyID: familyID, SKU: "ALC-C7#8",
		Description: "Cable Alumoclad 7#8 AWG", CostMicros: &cost, KgPerMMicros: &kg, UnitID: kgUnit})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	noWeightID, err := a.store.CreateProduct(ctx, store.Product{FamilyID: familyID, SKU: "ALC-SIN-PESO",
		Description: "Alumoclad sin peso", CostMicros: &cost})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	marginID, err := a.store.CreateMarginOption(ctx, "Diez", 100_000, userID)
	if err != nil {
		t.Fatalf("CreateMarginOption: %v", err)
	}

	draft, err := a.store.CreateDraftQuote(ctx, customerID, userID, "QG")
	if err != nil {
		t.Fatalf("CreateDraftQuote: %v", err)
	}
	form := costLineForm(alcID, marginID)
	form.Set("lines[0][qty]", "1000")
	rec := doForm(t, a, userID, q.Recalcular, "POST", "/cotizaciones/"+draft.Folio+"/recalcular",
		map[string]string{"folio": draft.Folio}, form, true)
	body := rec.Body.String()
	// $95/kg × 0.3896 kg/m = $37.012 per metre; ÷ (1 − 0.10) = $41.1244, $41.12 shown;
	// 1,000 m of it is $41,124.44 and 389.60 kg.
	for _, want := range []string{"Cantidad (m)", "<td class=\"num\">0.3896</td>", "<td class=\"num\">389.60</td>",
		"<td class=\"num\">$41.12</td>", "<td class=\"num\">$41,124.44</td>"} {
		if !strings.Contains(body, want) {
			t.Errorf("Recalcular missing %q: %s", want, body)
		}
	}

	rec = doForm(t, a, userID, q.Emitir, "POST", "/cotizaciones/"+draft.Folio+"/emitir",
		map[string]string{"folio": draft.Folio}, form, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Emitir status = %d; body = %s", rec.Code, rec.Body.String())
	}
	issued, err := a.store.QuoteByFolio(ctx, draft.Folio)
	if err != nil || issued == nil {
		t.Fatalf("QuoteByFolio: %v", err)
	}
	persisted, err := a.store.ListQuoteLines(ctx, issued.ID)
	if err != nil || len(persisted) != 1 {
		t.Fatalf("ListQuoteLines = %+v, %v", persisted, err)
	}
	if persisted[0].UnitPriceMicros != 41_124_444 || persisted[0].LineTotal != 41_124_44 {
		t.Errorf("issued line = %+v, want $41.124444/m and $41,124.44", persisted[0])
	}
	if in := *persisted[0].PricingInputsJSON; !strings.Contains(in, `"cost":"95"`) || !strings.Contains(in, `"kg_per_unit":"0.3896"`) {
		t.Errorf("pricing_inputs = %s, want the cost per kg and the weight", in)
	}
	doc, err := issuedQuoteDocument(*issued, persisted)
	if err != nil {
		t.Fatalf("issuedQuoteDocument: %v", err)
	}
	if !doc.ShowKg || doc.Lines[0].KgPerUnit != "0.3896" || doc.Lines[0].Kg != "389.60" || doc.Lines[0].UnitPrice != "$41.12" {
		t.Errorf("PDF document = %+v, want the CCS columns", doc)
	}
	// It earns the margin on the price: 10% of $41,124.44.
	project, _ := a.store.ProjectByFolio(ctx, issued.ProjectFolio)
	if project == nil || project.Profit != 4_112_44 {
		t.Fatalf("proyecto profit = %+v, want $4,112.44", project)
	}
	// Its factura takes the metro unit the line was quoted in, not the catalog's kilo.
	if err := a.store.SetProjectProbability(ctx, project.ID, userID, 90, ""); err != nil {
		t.Fatal(err)
	}
	if rec := postOC(t, a, NewProjects(a.store), userID, project.Folio, map[string]string{"oc_numero": "77", "oc_fecha": "2026-10-07", "forma_pago": "PUE"},
		"oc.pdf", []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n")); rec.Code != http.StatusSeeOther {
		t.Fatalf("postOC = %d: %s", rec.Code, rec.Body.String())
	}
	facturaID, err := a.store.CreateFacturaDraft(ctx, project.ID, userID)
	if err != nil {
		t.Fatalf("CreateFacturaDraft: %v", err)
	}
	if c, _ := a.store.ComprobanteByID(ctx, facturaID); c == nil || c.Conceptos[0].SATUnitCode != "MTR" || c.Conceptos[0].UnitName != "Metro" {
		t.Errorf("QG factura concepto = %+v, want MTR Metro", c)
	}

	other, _ := a.store.CreateDraftQuote(ctx, customerID, userID, "QG")
	form = costLineForm(noWeightID, marginID)
	rec = doForm(t, a, userID, q.Recalcular, "POST", "/cotizaciones/"+other.Folio+"/recalcular",
		map[string]string{"folio": other.Folio}, form, true)
	// Quoted verbatim in docs/guia/cotizaciones.md's Problemas comunes.
	if !strings.Contains(rec.Body.String(), "Este producto no tiene Peso (kg/m) y en esta serie el precio por metro sale de él. Quítalo de la cotización y captura su peso en Productos.") {
		t.Errorf("a product without a weight was priced: %s", rec.Body.String())
	}
	rec = doForm(t, a, userID, q.Guardar, "POST", "/cotizaciones/"+other.Folio+"/guardar",
		map[string]string{"folio": other.Folio}, form, false)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("Guardar with a weightless QG product = %d, want 422", rec.Code)
	}
}

// QG "Otro margen" typed as a price per kg (issue #43): measured against the products'
// own cost per kg, a metre sells at exactly that price × its Peso; a typed percentage or
// a menu option shows the price it implies. Products with different costs per kg have
// no single price per kg: the field gives way to a hint, and a saved price stops pricing
// (its margin does) until the odd line is gone.
func TestQuoteQGOtroMargenPerKg(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	customerID, _, _ := seedQuoteBuilderFixtures(t, a)
	ctx := context.Background()
	familyID := createQGFamily(t, a, "Cable Alumoclad 7#8 AWG hilo de guarda", "Precios en pesos mexicanos (MXN), no incluyen IVA")
	cost, kg := money.Micros(95_000_000), money.Micros(389_600)
	alcID, err := a.store.CreateProduct(ctx, store.Product{FamilyID: familyID, SKU: "ALC-C7#8",
		Description: "Cable Alumoclad 7#8 AWG", CostMicros: &cost, KgPerMMicros: &kg})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	marginID, err := a.store.CreateMarginOption(ctx, "Diez", 100_000, userID)
	if err != nil {
		t.Fatalf("CreateMarginOption: %v", err)
	}
	draft, _ := a.store.CreateDraftQuote(ctx, customerID, userID, "QG")
	folio := map[string]string{"folio": draft.Folio}
	form := func(fields url.Values) url.Values {
		f := costLineForm(alcID, marginID)
		f.Set("margin_option_id", "custom")
		f.Set("lines[0][qty]", "1000")
		for k, v := range fields {
			f[k] = v
		}
		return f
	}
	recalc := func(f url.Values) string {
		t.Helper()
		rec := doForm(t, a, userID, q.Recalcular, "POST", "/cotizaciones/"+draft.Folio+"/recalcular", folio, f, true)
		if rec.Code != http.StatusOK {
			t.Fatalf("Recalcular status = %d", rec.Code)
		}
		return rec.Body.String()
	}

	// On a menu option the fields show its margin and the price per kg it implies,
	// $95 / (1 − 0.10) = $105.56, read-only, under the family's name.
	body := builderBody(t, a, q, userID, draft.Folio)
	for _, want := range []string{"Precio Alumoclad ($/kg)", `data-copper-cost="95"`, `name="copper_price"`} {
		if !strings.Contains(body, want) {
			t.Errorf("empty QG builder missing %q", want)
		}
	}
	rec := doForm(t, a, userID, q.Guardar, "POST", "/cotizaciones/"+draft.Folio+"/guardar", folio, costLineForm(alcID, marginID), false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Guardar(option) status = %d; body = %s", rec.Code, rec.Body.String())
	}
	if body := builderBody(t, a, q, userID, draft.Folio); !strings.Contains(body, `value="105.56"`) || !strings.Contains(body, `value="10"`) || !strings.Contains(body, "readonly") {
		t.Errorf("menu option doesn't show its margin and price per kg read-only: %s", body)
	}

	// A typed percentage derives the price per kg: 95 / (1 − 0.20) = $118.75; a metre is
	// 95 × 0.3896 ÷ 0.80 = $46.265.
	if body := recalc(form(url.Values{"margin_pct": {"20"}, "margin_edited": {"pct"}})); !strings.Contains(body, "$46,265.00") {
		t.Errorf("20%% didn't price 1,000 m to $46,265.00: %s", body)
	}
	rec = doForm(t, a, userID, q.Guardar, "POST", "/cotizaciones/"+draft.Folio+"/guardar", folio,
		form(url.Values{"margin_pct": {"20"}, "margin_edited": {"pct"}}), false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Guardar(20%%) status = %d; body = %s", rec.Code, rec.Body.String())
	}
	if body := builderBody(t, a, q, userID, draft.Folio); !strings.Contains(body, `value="118.75"`) {
		t.Errorf("20%% doesn't show $118.75/kg: %s", body)
	}

	// A typed $110.55/kg sells a metre at 110.55 × 0.3896 = $43.07028 exactly, and is
	// a 14.0660% margin over the $95 cost, shown rounded.
	typed := url.Values{"margin_pct": {"14.07"}, "copper_price": {"110.55"}, "margin_edited": {"copper"}}
	if body := recalc(form(typed)); !strings.Contains(body, "$43,070.28") {
		t.Errorf("$110.55/kg didn't price 1,000 m to $43,070.28: %s", body)
	}
	rec = doForm(t, a, userID, q.Guardar, "POST", "/cotizaciones/"+draft.Folio+"/guardar", folio, form(typed), false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Guardar($110.55/kg) status = %d; body = %s", rec.Code, rec.Body.String())
	}
	saved, _ := a.store.QuoteByID(ctx, draft.ID)
	if saved.CustomMarginMicros == nil || *saved.CustomMarginMicros != 140_660 ||
		saved.CustomCopperPriceMicros == nil || *saved.CustomCopperPriceMicros != 110_550_000 {
		t.Fatalf("saved margin = %v, price = %v; want 0.14066 and 110.55", saved.CustomMarginMicros, saved.CustomCopperPriceMicros)
	}
	lines, _ := a.store.ListQuoteLines(ctx, draft.ID)
	if len(lines) != 1 || lines[0].UnitPriceMicros != 43_070_280 || lines[0].LineTotal != 43_070_28 {
		t.Fatalf("saved lines = %+v; want $43.07028/m and $43,070.28", lines)
	}
	// Reopened, it keeps the typed price.
	body = builderBody(t, a, q, userID, draft.Folio)
	for _, want := range []string{`<option value="custom" selected>Otro margen (14.07%)</option>`, `value="14.07"`, `value="110.55"`, "$43,070.28"} {
		if !strings.Contains(body, want) {
			t.Errorf("reopened builder missing %q", want)
		}
	}
	// The page sends back what it shows: nothing changes, and the controls aren't swapped.
	unchanged := form(url.Values{"margin_pct": {"14.07"}, "copper_price": {"110.55"}, "margin_kg_ref": {"95"}})
	if body := recalc(unchanged); !strings.Contains(body, "$43,070.28") || strings.Contains(body, "hx-swap-oob") || strings.Contains(body, `class="error"`) {
		t.Errorf("unchanged recalculation changed something: %s", body)
	}
	// A price at or below the cost can't price.
	if body := recalc(form(url.Values{"copper_price": {"95"}, "margin_edited": {"copper"}})); !strings.Contains(body, "El precio de Alumoclad debe ser mayor que su costo (95 por kg)") {
		t.Errorf("price at cost wasn't refused: %s", body)
	}

	// A product with another cost per kg: with both on the quote there's no price per
	// kg. The saved price's margin keeps pricing, and the controls swap in the hint.
	otherCost := money.Micros(100_000_000)
	otherID, err := a.store.CreateProduct(ctx, store.Product{FamilyID: familyID, SKU: "ALC-C7#10",
		Description: "Cable Alumoclad 7#10 AWG", CostMicros: &otherCost, KgPerMMicros: &kg})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	mixed := form(url.Values{"margin_pct": {"14.07"}, "copper_price": {"110.55"}, "margin_kg_ref": {"95"}})
	mixed.Set("line_keys", "0,1")
	mixed.Set("lines[1][kind]", "product")
	mixed.Set("lines[1][product_id]", strconv.FormatInt(otherID, 10))
	mixed.Set("lines[1][qty]", "1000")
	body = recalc(mixed)
	// 95 × 0.3896 ÷ (1 − 0.14066) = $43.070263 a metre, from the margin, not $43.07028.
	for _, want := range []string{"hx-swap-oob", mixedKgCostHint, "110.55 por kg", "$43,070.26"} {
		if !strings.Contains(body, want) {
			t.Errorf("mixed costs recalculation missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, `name="copper_price"`) || strings.Contains(body, `class="error"`) {
		t.Errorf("mixed costs still offer the price per kg, or show an error: %s", body)
	}
	rec = doForm(t, a, userID, q.Guardar, "POST", "/cotizaciones/"+draft.Folio+"/guardar", folio, mixed, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Guardar(mixed) status = %d; body = %s", rec.Code, rec.Body.String())
	}
	if saved, _ := a.store.QuoteByID(ctx, draft.ID); *saved.CustomMarginMicros != 140_660 || saved.CustomCopperPriceMicros == nil || *saved.CustomCopperPriceMicros != 110_550_000 {
		t.Errorf("mixed save kept margin %v, price %v; want 0.14066 and 110.55", *saved.CustomMarginMicros, saved.CustomCopperPriceMicros)
	}
	body = builderBody(t, a, q, userID, draft.Folio)
	if !strings.Contains(body, mixedKgCostHint) || strings.Contains(body, `name="copper_price"`) {
		t.Errorf("reopened mixed builder doesn't show the hint instead of the price: %s", body)
	}

	// Removing the odd line brings the price back, pricing exactly again.
	back := form(url.Values{"margin_pct": {"14.07"}, "margin_kg_ref": {""}})
	body = recalc(back)
	for _, want := range []string{"hx-swap-oob", `value="110.55"`, "$43,070.28"} {
		if !strings.Contains(body, want) {
			t.Errorf("recalculation without the odd line missing %q: %s", want, body)
		}
	}

	// A new QG draft with no lines, over a family with mixed costs: only the percentage.
	empty, _ := a.store.CreateDraftQuote(ctx, customerID, userID, "QG")
	body = builderBody(t, a, q, userID, empty.Folio)
	if strings.Contains(body, `name="copper_price"`) || !strings.Contains(body, mixedKgCostHint) {
		t.Errorf("empty QG draft over mixed costs offers the price per kg: %s", body)
	}
}

// A QG product needs its Peso, which its price per metre comes from (issue #37); other
// familias still take it as optional.
func TestProductQGRequiresWeight(t *testing.T) {
	a := newTestAuth(t)
	p := newTestProducts(t, a)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	qgID := createQGFamily(t, a, "Alumoclad", "Precios en MXN")
	otherID := seedFamily(t, a, "CCA")
	form := func(family int64, sku, kg string) url.Values {
		return url.Values{"family_id": {itoa(family)}, "sku": {sku}, "description": {"Cable"}, "cost": {"95"}, "kg_per_m": {kg}}
	}
	// Quoted verbatim in docs/guia/productos.md's Problemas comunes.
	const msg = "El peso es obligatorio en la serie QG: el precio por metro sale del costo por kg × el peso. Escríbelo en kg/m, p. ej. 0.3896."
	for _, kg := range []string{"", "0"} {
		rec := doForm(t, a, userID, p.Create, "POST", "/productos/nuevo", nil, form(qgID, "ALC-1", kg), false)
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), msg) {
			t.Errorf("QG product with Peso %q: status %d", kg, rec.Code)
		}
	}
	if rec := doForm(t, a, userID, p.Create, "POST", "/productos/nuevo", nil, form(qgID, "ALC-1", "0.3896"), false); rec.Code != http.StatusSeeOther {
		t.Errorf("QG product with a Peso: status %d; body = %s", rec.Code, rec.Body.String())
	}
	if rec := doForm(t, a, userID, p.Create, "POST", "/productos/nuevo", nil, form(otherID, "CCA-1", ""), false); rec.Code != http.StatusSeeOther {
		t.Errorf("CCA product without a Peso: status %d; body = %s", rec.Code, rec.Body.String())
	}
}

// createQGFamily creates the Alumoclad familia with its QG quote type set up as
// production's is (migrations/0029_quote_types.sql): costed per kg, with its own
// letterhead. Behavior comes from those flags, never from the name or the prefix.
func createQGFamily(t *testing.T, a *Auth, label, terms string) int64 {
	t.Helper()
	ctx := context.Background()
	familyID, err := a.store.CreateFamily(ctx, store.Family{Name: "Alumoclad", Series: "QG", SeriesLabel: label, Terms: terms})
	if err != nil {
		t.Fatalf("CreateFamily: %v", err)
	}
	typeID, err := a.store.QuoteTypeIDByPrefix(ctx, "QG")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.SetQuoteTypeBehavior(ctx, typeID, store.QuoteTypeBehavior{KgPriced: true, Letterhead: "alumoclad"}); err != nil {
		t.Fatal(err)
	}
	return familyID
}
