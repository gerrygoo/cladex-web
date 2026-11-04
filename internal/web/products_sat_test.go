package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/gerrygoo/cladex-web/internal/store"
)

// familyID finds a seeded familia by name.
func familyID(t *testing.T, a *Auth, name string) int64 {
	t.Helper()
	families, err := a.store.ListFamilies(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.Name == name {
			return f.ID
		}
	}
	t.Fatalf("no familia %q", name)
	return 0
}

// Issue #62: a vendedor captures a product's SAT codes when creating it, each checked
// against the SAT's catalogs; two products of one familia keep different units.
func TestProductsSATCodesOnCreate(t *testing.T) {
	a := newTestAuth(t)
	p := newTestProducts(t, a)
	vendedor := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	cca := itoa(familyID(t, a, "CCA"))
	create := func(form url.Values) (int, string) {
		rec := doForm(t, a, vendedor, p.Create, "POST", "/productos/nuevo", nil, form, false)
		return rec.Code, rec.Body.String()
	}
	base := func(sku string) url.Values {
		return url.Values{"family_id": {cca}, "sku": {sku}, "description": {"Cable " + sku}, "cost": {"10"}}
	}

	// Unknown codes are refused, and nothing is created.
	f := base("CCA-C14")
	f.Set("sat_product_code", "26121699")
	f.Set("sat_unit_code", "QQQ")
	code, body := create(f)
	if code != http.StatusUnprocessableEntity ||
		!strings.Contains(body, "La clave de producto 26121699 no existe en el catálogo del SAT. Búscala por su descripción.") ||
		!strings.Contains(body, "La clave de unidad QQQ no existe en el catálogo del SAT.") {
		t.Fatalf("unknown codes = %d; body = %s", code, body)
	}
	if ps, _ := a.store.ListProducts(t.Context(), "", "", "", nil); len(ps) != 0 {
		t.Fatalf("%d products after a refused form", len(ps))
	}
	// The form keeps what was typed, the code's field marked.
	if !strings.Contains(body, `value="26121699"`) || !strings.Contains(body, `aria-describedby="error-sat_product_code"`) {
		t.Errorf("the refused form doesn't keep the code or mark the field: %s", body)
	}

	// Known codes are saved; a unit in lower case is stored as the SAT writes it.
	f = base("CCA-C14")
	f.Set("sat_product_code", "26121600")
	f.Set("sat_unit_code", "mtr")
	if code, body := create(f); code != http.StatusSeeOther {
		t.Fatalf("Create = %d; body = %s", code, body)
	}
	f = base("CCA-C12")
	f.Set("sat_product_code", "26121600")
	f.Set("sat_unit_code", "H87")
	if code, body := create(f); code != http.StatusSeeOther {
		t.Fatalf("Create = %d; body = %s", code, body)
	}
	// Both are optional.
	if code, body := create(base("CCA-C10")); code != http.StatusSeeOther {
		t.Fatalf("Create without SAT codes = %d; body = %s", code, body)
	}
	c14, _ := a.store.ProductBySKU(t.Context(), "CCA-C14")
	c12, _ := a.store.ProductBySKU(t.Context(), "CCA-C12")
	c10, _ := a.store.ProductBySKU(t.Context(), "CCA-C10")
	if c14.SATProductCode != "26121600" || c14.SATUnitCode != "MTR" || c12.SATUnitCode != "H87" ||
		c10.SATProductCode != "" || c10.SATUnitCode != "" {
		t.Errorf("saved: %+v / %+v / %+v", c14, c12, c10)
	}

	// The vendedor sees them read-only afterwards, with their official descriptions.
	id := itoa(c14.ID)
	rec := doForm(t, a, vendedor, p.EditPage, "GET", "/productos/"+id, map[string]string{"id": id}, nil, false)
	body = rec.Body.String()
	if !strings.Contains(body, "26121600 · Cables eléctricos y accesorios") || !strings.Contains(body, "MTR · Metro") ||
		strings.Contains(body, `name="sat_product_code"`) {
		t.Errorf("vendedor's view of the product: %s", body)
	}
}

// Issue #62: an admin edits the codes later; a unit outside the most used ones shows
// under "Otras", and clearing a code is allowed.
func TestProductsSATCodesAdminEdit(t *testing.T) {
	a := newTestAuth(t)
	p := newTestProducts(t, a)
	admin := createTestUser(t, a, "admin1", "admin", "hunter2")
	cca := itoa(familyID(t, a, "CCA"))
	id, err := a.store.CreateProduct(t.Context(), store.Product{FamilyID: familyID(t, a, "CCA"), SKU: "CCA-C8", Description: "Cable 8"})
	if err != nil {
		t.Fatal(err)
	}
	pid := itoa(id)
	pv := map[string]string{"id": pid}
	form := url.Values{"family_id": {cca}, "sku": {"CCA-C8"}, "description": {"Cable 8"},
		"sat_product_code": {"26121609"}, "sat_unit_code": {"Ohm"}}
	if rec := doForm(t, a, admin, p.Update, "POST", "/productos/"+pid, pv, form, false); rec.Code != http.StatusSeeOther {
		t.Fatalf("Update = %d; body = %s", rec.Code, rec.Body.String())
	}
	rec := doForm(t, a, admin, p.EditPage, "GET", "/productos/"+pid, pv, nil, false)
	body := rec.Body.String()
	for _, want := range []string{
		"Datos para facturar (SAT)",
		`name="sat_product_code"`, `value="26121609"`, "Cable de redes",
		`<optgroup label="Más usadas">`, `<option value="MTR">MTR · Metro</option>`,
		`<optgroup label="Otras" data-sat-other><option value="Ohm" selected>Ohm · Ohm</option>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("edit form missing %q", want)
		}
	}

	form.Set("sat_product_code", "")
	form.Set("sat_unit_code", "")
	if rec := doForm(t, a, admin, p.Update, "POST", "/productos/"+pid, pv, form, false); rec.Code != http.StatusSeeOther {
		t.Fatalf("clearing Update = %d", rec.Code)
	}
	if got, _ := a.store.ProductByID(t.Context(), id); got.SATProductCode != "" || got.SATUnitCode != "" {
		t.Errorf("after clearing: %+v", got)
	}
}

// Issue #62: a product quoted per metre by weight (CCS & AC's QS, a kg-priced type's
// familia) and invoiced in KGM needs its Peso; elsewhere, or in another unit, it doesn't.
func TestProductsKGMNeedsWeightWhenQuotedByWeight(t *testing.T) {
	a := newTestAuth(t)
	p := newTestProducts(t, a)
	vendedor := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	ccs := itoa(familyID(t, a, "CCS & AC"))
	cca := itoa(familyID(t, a, "CCA"))
	create := func(fam, sku, unit, peso string) (int, string) {
		form := url.Values{"family_id": {fam}, "sku": {sku}, "description": {"Cable " + sku},
			"sat_product_code": {"26121600"}, "sat_unit_code": {unit}, "kg_per_m": {peso}}
		rec := doForm(t, a, vendedor, p.Create, "POST", "/productos/nuevo", nil, form, false)
		return rec.Code, rec.Body.String()
	}
	const msg = "El peso es obligatorio para facturar en kilos (KGM) un producto que se cotiza por metro: convierte los metros a kilos. Escríbelo en kg/m, p. ej. 0.3896."
	if code, body := create(ccs, "CCS-1", "KGM", ""); code != http.StatusUnprocessableEntity || !strings.Contains(body, msg) {
		t.Errorf("CCS & AC in KGM without Peso = %d; want 422 with %q", code, msg)
	}
	if code, body := create(ccs, "CCS-1", "kgm", "0"); code != http.StatusUnprocessableEntity || !strings.Contains(body, msg) {
		t.Errorf("CCS & AC in kgm with Peso 0 = %d; want 422", code)
	}
	for _, c := range []struct{ fam, sku, unit, peso string }{
		{ccs, "CCS-1", "KGM", "0.1723"},
		{ccs, "CCS-2", "MTR", ""},
		{cca, "CCA-1", "KGM", ""},
	} {
		if code, body := create(c.fam, c.sku, c.unit, c.peso); code != http.StatusSeeOther {
			t.Errorf("%s in %s with Peso %q = %d; body = %s", c.sku, c.unit, c.peso, code, body)
		}
	}
}

// Issue #62: the list's Datos SAT column and its "Sin datos para facturar" filter.
func TestProductsListSinDatosParaFacturar(t *testing.T) {
	a := newTestAuth(t)
	p := newTestProducts(t, a)
	vendedor := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	fam := familyID(t, a, "CCA")
	for _, pr := range []store.Product{
		{FamilyID: fam, SKU: "LISTO", Description: "Completo", SATProductCode: "26121600", SATUnitCode: "MTR"},
		{FamilyID: fam, SKU: "SINUNIDAD", Description: "Sin unidad", SATProductCode: "26121600"},
		{FamilyID: fam, SKU: "NADA", Description: "Sin nada"},
	} {
		if _, err := a.store.CreateProduct(t.Context(), pr); err != nil {
			t.Fatal(err)
		}
	}
	rec := doForm(t, a, vendedor, p.List, "GET", "/productos", nil, nil, false)
	body := rec.Body.String()
	for _, want := range []string{
		`href="/productos?f.sat=faltan"`, ">Sin datos para facturar</a>", "Datos SAT",
		"26121600 · MTR", "Sin datos para facturar (falta la clave de unidad)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("list missing %q", want)
		}
	}
	rec = doForm(t, a, vendedor, p.List, "GET", "/productos?f.sat=faltan", nil, nil, false)
	body = rec.Body.String()
	if strings.Contains(body, "LISTO") || !strings.Contains(body, "SINUNIDAD") || !strings.Contains(body, "NADA") {
		t.Errorf("Sin datos para facturar filter: %s", body)
	}
}

// A unit's SAT code, the default for products without their own, must exist in the
// SAT's c_ClaveUnidad catalog too (issue #62).
func TestUnidadesSATCodeMustExist(t *testing.T) {
	a := newTestAuth(t)
	u := NewUnits(a.store)
	adminID := createTestUser(t, a, "ana", "admin", "hunter2")
	units, _ := a.store.ListUnits(t.Context())
	id := itoa(units[0].ID)
	pv := map[string]string{"id": id}
	rec := doForm(t, a, adminID, u.SetSAT, "POST", "/unidades/"+id+"/sat", pv, url.Values{"sat_code": {"QQQ"}}, false)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "La clave de unidad SAT QQQ no existe en el catálogo del SAT.") {
		t.Errorf("QQQ = %d; want 422 saying why", rec.Code)
	}
	if rec := doForm(t, a, adminID, u.SetSAT, "POST", "/unidades/"+id+"/sat", pv, url.Values{"sat_code": {"xrl"}}, false); rec.Code != http.StatusSeeOther {
		t.Errorf("xrl = %d; want saved", rec.Code)
	}
}

// Issue #62: the form's lookups — the description of a typed code, and the searches.
func TestProductsSATLookups(t *testing.T) {
	a := newTestAuth(t)
	p := newTestProducts(t, a)
	vendedor := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	get := func(h http.HandlerFunc, path string) string {
		rec := doForm(t, a, vendedor, h, "GET", path, nil, nil, true)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d", path, rec.Code)
		}
		return rec.Body.String()
	}
	for q, want := range map[string]string{
		"26121600": "Cables eléctricos y accesorios",
		"26121699": "La clave 26121699 no existe en el catálogo del SAT.",
		"2612":     "La clave son 8 dígitos.",
		"":         "Sin clave.",
	} {
		if body := get(p.SATClave, "/productos/sat/clave?sat_product_code="+q); !strings.Contains(body, want) || !strings.Contains(body, `id="sat-product-desc"`) {
			t.Errorf("SATClave(%q) = %s; want %q", q, body, want)
		}
	}
	body := get(p.SATClaves, "/productos/sat/claves?sat_q="+url.QueryEscape("cables electricos"))
	if !strings.Contains(body, `data-sat-pick="26121600"`) || !strings.Contains(body, `data-sat-target="sat_product_code"`) {
		t.Errorf("SATClaves(cables electricos) = %s", body)
	}
	if body := get(p.SATClaves, "/productos/sat/claves?sat_q=cable"); !strings.Contains(body, "Se muestran las primeras 20") {
		t.Errorf("SATClaves(cable) should say it shows only the first 20: %s", body)
	}
	if body := get(p.SATClaves, "/productos/sat/claves?sat_q=zzqqxx"); !strings.Contains(body, "Ninguna de las claves del SAT coincide con «zzqqxx».") {
		t.Errorf("SATClaves(zzqqxx) = %s", body)
	}
	body = get(p.SATUnidades, "/productos/sat/unidades?sat_unit_q=carrete")
	if !strings.Contains(body, `data-sat-pick="XRL"`) || !strings.Contains(body, `data-sat-label="XRL · Carrete"`) ||
		!strings.Contains(body, `data-sat-target="sat_unit_code"`) {
		t.Errorf("SATUnidades(carrete) = %s", body)
	}
}
