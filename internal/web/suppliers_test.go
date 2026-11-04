package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/gerrygoo/cladex-web/internal/store"
)

// Issue #64: Proveedores. These also pin what docs/guia/proveedores.md quotes.

// supplierForm is a complete, valid supplier form; extra overrides or adds fields.
func supplierForm(extra url.Values) url.Values {
	f := url.Values{
		"name": {"Postes del Norte"}, "rfc": {"pno010101ab1"}, "legal_name": {"POSTES DEL NORTE"},
		"street": {"Av. Uno"}, "ext_number": {"10"}, "neighborhood": {"Centro"},
		"municipality": {"Monterrey"}, "state": {"Nuevo León"}, "postal_code": {"64000"},
		"contactos":         {"3"},
		"contacto-0-nombre": {"Ana"}, "contacto-0-email": {"ana@pn.mx"}, "contacto-0-oc": {"1"},
		"contacto-1-nombre": {"Beto"}, "contacto-1-telefono": {"81 1234 5678"}, "contacto-1-oc": {"1"},
	}
	for k, v := range extra {
		f[k] = v
	}
	return f
}

func supplierByRFC(t *testing.T, s *store.Store, rfc string) *store.Supplier {
	t.Helper()
	rows, err := s.ListSuppliers(context.Background(), "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.RFC == rfc {
			sup, err := s.SupplierByID(context.Background(), r.ID)
			if err != nil {
				t.Fatal(err)
			}
			return sup
		}
	}
	return nil
}

func familiaID(t *testing.T, s *store.Store, name string) string {
	t.Helper()
	all, err := s.ListAllFamilies(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range all {
		if f.Name == name {
			return itoa(f.ID)
		}
	}
	t.Fatalf("no familia %q", name)
	return ""
}

func TestSuppliersPermissions(t *testing.T) {
	a := newTestAuth(t)
	mux := newTestMux(t, a)
	ctx := context.Background()
	vendedor := createTestUser(t, a, "rfm", "vendedor", "hunter2")
	admin := createNamedUser(t, a, "ana", "Ana López", "admin")
	car := supplierByRFC(t, a.store, "CAR191126QX6")
	id := itoa(car.ID)

	// An admin adds bank data.
	car.Bank, car.CLABE, car.Beneficiary = "BANAMEX", "002010077777777771", "CABLE ANTI ROBO"
	if err := a.store.UpdateSupplier(store.WithActor(ctx, store.Actor{UserID: admin, Source: store.SourceWeb}), *car); err != nil {
		t.Fatal(err)
	}

	// A vendedor sees the list, without Nuevo proveedor or Eliminar.
	list := muxDo(t, a, mux, vendedor, "GET", "/proveedores", nil).Body.String()
	if !strings.Contains(list, "Cable Anti Robo") || !strings.Contains(list, "CCA, CCS &amp; AC") ||
		strings.Contains(list, "Nuevo proveedor") || strings.Contains(list, "Eliminar") || !strings.Contains(list, ">Ver<") {
		t.Errorf("vendedor list: %s", list)
	}
	// The supplier read-only, with its contacts but no bank data, here or in the Historial.
	page := muxDo(t, a, mux, vendedor, "GET", "/proveedores/"+id, nil).Body.String()
	for _, want := range []string{"Solo un administrador puede modificar o eliminar este proveedor.", "Camino Real de Carretas 356, Col. Milenio III, Querétaro, Querétaro, C.P. 76060",
		"Luis Manuel Enríquez (Sr.)", "cotizaciones@cableantirobo.com", "CCA — Cable CCA (QA); CCS &amp; AC — Cable CCS &amp; AC (QS)", "Creado por Sistema · "} {
		if !strings.Contains(page, want) {
			t.Errorf("vendedor page lacks %q", want)
		}
	}
	for _, bank := range []string{"BANAMEX", "002010077777777771", "Datos bancarios", "CLABE", "<form method=\"post\""} {
		if strings.Contains(page, bank) {
			t.Errorf("vendedor page shows %q: %s", bank, page)
		}
	}
	// Everything that changes a supplier is the admin's.
	for _, path := range []string{"/proveedores/nuevo", "/proveedores/" + id, "/proveedores/" + id + "/eliminar", "/proveedores/" + id + "/inactivar"} {
		rec := muxDo(t, a, mux, vendedor, "POST", path, supplierForm(nil))
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "No tienes permiso para hacer esto. Solo un administrador puede hacerlo.") {
			t.Errorf("vendedor POST %s: %d", path, rec.Code)
		}
	}
	if rec := muxDo(t, a, mux, vendedor, "GET", "/proveedores/nuevo", nil); rec.Code != http.StatusForbidden {
		t.Errorf("vendedor GET /proveedores/nuevo: %d", rec.Code)
	}

	// The admin sees the form with the bank data, and the bank change in the Historial.
	page = muxDo(t, a, mux, admin, "GET", "/proveedores/"+id, nil).Body.String()
	for _, want := range []string{`value="002010077777777771"`, `value="BANAMEX"`, "<strong>Ana López</strong>",
		"<li>CLABE: (vacío) → 002010077777777771</li>", `action="/proveedores/` + id + `"`,
		"Quitar este contacto", "Marcar como inactivo", "Eliminar proveedor", "Datos bancarios (opcionales; los vendedores no los ven)"} {
		if !strings.Contains(page, want) {
			t.Errorf("admin page lacks %q", want)
		}
	}
	if list := muxDo(t, a, mux, admin, "GET", "/proveedores", nil).Body.String(); !strings.Contains(list, "Nuevo proveedor") ||
		!strings.Contains(list, "¿Seguro que quieres eliminar a Cable Anti Robo? No se puede deshacer.") {
		t.Errorf("admin list lacks Nuevo proveedor or the delete question")
	}
}

func TestSuppliersValidation(t *testing.T) {
	a := newTestAuth(t)
	mux := newTestMux(t, a)
	admin := createTestUser(t, a, "ana", "admin", "hunter2")
	for _, tc := range []struct {
		field, value, msg string
	}{
		{"name", "", "El nombre comercial es obligatorio."},
		{"rfc", "", "El RFC es obligatorio."},
		{"rfc", "PNO010101AB", "El RFC debe tener 12 caracteres (persona moral) o 13 (persona física), p. ej. CAR191126QX6."},
		{"rfc", "PNOX010101AB12", "El RFC debe tener 12 caracteres (persona moral) o 13 (persona física), p. ej. CAR191126QX6."},
		{"legal_name", "", "La razón social es obligatoria."},
		{"street", "", "La calle es obligatoria."},
		{"ext_number", "", "El número es obligatorio."},
		{"neighborhood", "", "La colonia es obligatoria."},
		{"municipality", "", "El municipio o alcaldía es obligatorio."},
		{"state", "", "El estado es obligatorio."},
		{"postal_code", "", "El código postal es obligatorio."},
		{"postal_code", "7606", "El código postal son 5 dígitos, p. ej. 76060."},
		{"clabe", "00201007777777777", "La CLABE son 18 dígitos y no coincide con su dígito verificador; revísala."},
		{"clabe", "002010077777777772", "La CLABE son 18 dígitos y no coincide con su dígito verificador; revísala."},
		{"credit_days", "treinta", "Los días de crédito son un número entero, p. ej. 30."},
		{"contacto-0-nombre", "", "Escribe el nombre del contacto."},
		{"contacto-1-email", "beto", "Email inválido."},
	} {
		rec := muxDo(t, a, mux, admin, "POST", "/proveedores/nuevo", supplierForm(url.Values{tc.field: {tc.value}}))
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), tc.msg) {
			t.Errorf("%s=%q: status %d, want 422 with %q", tc.field, tc.value, rec.Code, tc.msg)
		}
	}
	if supplierByRFC(t, a.store, "PNO010101AB1") != nil {
		t.Error("an invalid supplier was saved")
	}

	// A 13-character RFC (persona física) and a valid CLABE are fine; empty contact slots
	// are ignored.
	rec := muxDo(t, a, mux, admin, "POST", "/proveedores/nuevo", supplierForm(url.Values{
		"rfc": {"PENA800101AB1"}, "clabe": {"002010077777777771"}, "credit_days": {"30"}, "currency": {"USD"},
	}))
	if rec.Code != http.StatusSeeOther || !strings.HasSuffix(rec.Header().Get("Location"), "?guardado=1") {
		t.Fatalf("valid create: %d %s", rec.Code, rec.Body.String())
	}
	sup := supplierByRFC(t, a.store, "PENA800101AB1")
	if sup == nil || len(sup.Contacts) != 2 || !sup.Contacts[0].ReceivesPO || !sup.Contacts[1].ReceivesPO ||
		sup.CLABE != "002010077777777771" || sup.Currency != "USD" || *sup.CreditDays != 30 {
		t.Errorf("created supplier = %+v", sup)
	}
	page := muxDo(t, a, mux, admin, "GET", rec.Header().Get("Location"), nil).Body.String()
	if !strings.Contains(page, "Proveedor guardado.") || !strings.Contains(page, "Creado por ana · ") {
		t.Errorf("page after create: %s", page)
	}
}

// The form offers every familia but QL, with its quote types; taking one from another
// supplier asks first, after the duplicate check.
func TestSuppliersDuplicateAndReassign(t *testing.T) {
	a := newTestAuth(t)
	mux := newTestMux(t, a)
	admin := createTestUser(t, a, "ana", "admin", "hunter2")
	cca, abastilum := familiaID(t, a.store, "CCA"), familiaID(t, a.store, "ABASTILUM")

	form := muxDo(t, a, mux, admin, "GET", "/proveedores/nuevo", nil).Body.String()
	for _, want := range []string{"CCA — Cable CCA (QA)", "ABASTILUM — Alumbrado (QI)", "(ahora: Cable Anti Robo)", "Recibe O.C.", "Agregar otro contacto"} {
		if !strings.Contains(form, want) {
			t.Errorf("new form lacks %q", want)
		}
	}
	if strings.Contains(form, "Líneas libres (QL)") || strings.Contains(form, `name="familia" value="`+familiaID(t, a.store, "QL")+`"`) {
		t.Error("QL is offered in Familias que surte")
	}

	// Same name as the seeded one: possible duplicate.
	dup := supplierForm(url.Values{"name": {"Cable Anti Robo S.A. de C.V."}, "familia": {cca, abastilum}})
	rec := muxDo(t, a, mux, admin, "POST", "/proveedores/nuevo", dup)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Este proveedor podría estar duplicado.") ||
		!strings.Contains(rec.Body.String(), "Sí, darlo de alta de todos modos") || !strings.Contains(rec.Body.String(), "Ver este proveedor") {
		t.Fatalf("duplicate: %d %s", rec.Code, rec.Body.String())
	}
	// Confirmed, it asks about CCA, carrying the confirmation along.
	dup.Set("confirmar", "1")
	rec = muxDo(t, a, mux, admin, "POST", "/proveedores/nuevo", dup)
	body := rec.Body.String()
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(body, "La familia CCA ya está asignada a Cable Anti Robo. ¿Quieres reasignarla?") ||
		strings.Contains(body, "ABASTILUM ya está") || !strings.Contains(body, `<input type="hidden" name="confirmar" value="1">`) ||
		strings.Contains(body, "podría estar duplicado") || !strings.Contains(body, "Sí, reasignar y guardar") {
		t.Fatalf("reassign question: %d %s", rec.Code, body)
	}
	dup.Set("reasignar", "1")
	if rec = muxDo(t, a, mux, admin, "POST", "/proveedores/nuevo", dup); rec.Code != http.StatusSeeOther {
		t.Fatalf("reassign: %d %s", rec.Code, rec.Body.String())
	}
	created := supplierByRFC(t, a.store, "PNO010101AB1")
	if created == nil || len(created.FamilyIDs) != 2 {
		t.Fatalf("created = %+v", created)
	}

	// Familias shows each familia's default supplier.
	fam := muxDo(t, a, mux, admin, "GET", "/familias", nil).Body.String()
	for _, want := range []string{
		`<a href="/proveedores/` + itoa(created.ID) + `">Cable Anti Robo S.A. de C.V.</a>`,
		`<a href="/proveedores/` + itoa(supplierByRFC(t, a.store, "CAR191126QX6").ID) + `">Cable Anti Robo</a>`,
		"No aplica", "<th>Proveedor predeterminado</th>",
	} {
		if !strings.Contains(fam, want) {
			t.Errorf("/familias lacks %q", want)
		}
	}
	// A familia nobody supplies.
	if _, err := a.store.CreateFamily(context.Background(), "Exceline"); err != nil {
		t.Fatal(err)
	}
	if fam = muxDo(t, a, mux, admin, "GET", "/familias", nil).Body.String(); !strings.Contains(fam, "Sin asignar") {
		t.Error("/familias lacks Sin asignar")
	}

	// Editing asks too, and not about a familia it already has.
	car := supplierByRFC(t, a.store, "CAR191126QX6")
	edit := supplierForm(url.Values{"name": {"Cable Anti Robo"}, "rfc": {"CAR191126QX6"}, "legal_name": {"CABLE ANTI ROBO"},
		"familia": {familiaID(t, a.store, "CCS & AC"), cca}, "contactos": {"0"}})
	rec = muxDo(t, a, mux, admin, "POST", "/proveedores/"+itoa(car.ID), edit)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "La familia CCA ya está asignada a Cable Anti Robo S.A. de C.V.") ||
		strings.Contains(rec.Body.String(), "La familia CCS") {
		t.Fatalf("edit reassign question: %d %s", rec.Code, rec.Body.String())
	}
}

func TestSuppliersDeleteAndInactive(t *testing.T) {
	a := newTestAuth(t)
	mux := newTestMux(t, a)
	admin := createNamedUser(t, a, "ana", "Ana López", "admin")
	car := supplierByRFC(t, a.store, "CAR191126QX6")
	id := itoa(car.ID)

	// It supplies familias: not deleted; the page says why and offers Inactivo.
	rec := muxDo(t, a, mux, admin, "POST", "/proveedores/"+id+"/eliminar", url.Values{})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/proveedores/"+id+"?eliminar=bloqueado#no-se-puede-eliminar" {
		t.Fatalf("blocked delete: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	page := muxDo(t, a, mux, admin, "GET", "/proveedores/"+id+"?eliminar=bloqueado", nil).Body.String()
	if !strings.Contains(page, "No se puede eliminar a Cable Anti Robo.") || !strings.Contains(page, "Es el proveedor predeterminado de: CCA, CCS &amp; AC.") ||
		!strings.Contains(page, `action="/proveedores/`+id+`/inactivar"`) {
		t.Errorf("blocked delete page: %s", page)
	}
	if supplierByRFC(t, a.store, "CAR191126QX6") == nil {
		t.Fatal("deleted anyway")
	}

	// Inactivo keeps it and its familias.
	if rec = muxDo(t, a, mux, admin, "POST", "/proveedores/"+id+"/inactivar", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("inactivar: %d", rec.Code)
	}
	page = muxDo(t, a, mux, admin, "GET", rec.Header().Get("Location"), nil).Body.String()
	if !strings.Contains(page, "Proveedor marcado como inactivo.") || !strings.Contains(page, `<span class="badge badge-inactivo">Inactivo</span>`) ||
		!strings.Contains(page, "Reactivar") || !strings.Contains(page, "<li>Estado del proveedor: Activo → Inactivo</li>") {
		t.Errorf("inactive page: %s", page)
	}
	if list := muxDo(t, a, mux, admin, "GET", "/proveedores?f.status=inactivo", nil).Body.String(); !strings.Contains(list, "Cable Anti Robo") {
		t.Error("Estado filter doesn't find the inactive supplier")
	}
	if list := muxDo(t, a, mux, admin, "GET", "/proveedores?f.status=activo", nil).Body.String(); strings.Contains(list, "Cable Anti Robo") {
		t.Error("Estado filter lists the inactive supplier as active")
	}
	if sup := supplierByRFC(t, a.store, "CAR191126QX6"); !sup.Inactive || len(sup.FamilyIDs) != 2 {
		t.Errorf("inactive supplier = %+v", sup)
	}
	if rec = muxDo(t, a, mux, admin, "POST", "/proveedores/"+id+"/reactivar", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("reactivar: %d", rec.Code)
	}

	// Without its familias (and its contacts removed), it can be deleted.
	form := supplierForm(url.Values{"name": {"Cable Anti Robo"}, "rfc": {"CAR191126QX6"}, "legal_name": {"CABLE ANTI ROBO"},
		"contactos": {"1"}, "contacto-0-id": {itoa(car.Contacts[0].ID)}, "contacto-0-nombre": {car.Contacts[0].Name}, "contacto-0-quitar": {"1"}})
	if rec = muxDo(t, a, mux, admin, "POST", "/proveedores/"+id, form); rec.Code != http.StatusSeeOther {
		t.Fatalf("remove familias: %d %s", rec.Code, rec.Body.String())
	}
	page = muxDo(t, a, mux, admin, "GET", "/proveedores/"+id, nil).Body.String()
	for _, want := range []string{"<li>Familia que surte quitada: CCA</li>", "<li>Familia que surte quitada: CCS &amp; AC</li>",
		"<li>Contacto quitado: Luis Manuel Enríquez (Sr.)</li>", "<li>Calle: Camino Real de Carretas → Av. Uno</li>"} {
		if !strings.Contains(page, want) {
			t.Errorf("Historial lacks %q", want)
		}
	}
	if rec = muxDo(t, a, mux, admin, "POST", "/proveedores/"+id+"/eliminar", url.Values{}); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/proveedores?eliminado=1" {
		t.Fatalf("delete: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if list := muxDo(t, a, mux, admin, "GET", "/proveedores?eliminado=1", nil).Body.String(); !strings.Contains(list, "Proveedor eliminado.") || strings.Contains(list, "Cable Anti Robo") {
		t.Errorf("list after delete: %s", list)
	}
	if rec = muxDo(t, a, mux, admin, "GET", "/proveedores/"+id, nil); rec.Code != http.StatusNotFound {
		t.Errorf("deleted supplier page: %d", rec.Code)
	}
}

// Editing every kind of data shows in the Historial: fields, contacts, familias.
func TestSupplierHistory(t *testing.T) {
	a := newTestAuth(t)
	mux := newTestMux(t, a)
	admin := createNamedUser(t, a, "ana", "Ana López", "admin")
	car := supplierByRFC(t, a.store, "CAR191126QX6")
	id := itoa(car.ID)

	form := url.Values{
		"name": {"Cable Anti Robo"}, "rfc": {"CAR191126QX6"}, "legal_name": {"CABLE ANTI ROBO"},
		"street": {"Camino Real de Carretas"}, "ext_number": {"356"}, "neighborhood": {"Milenio III"},
		"municipality": {"Querétaro"}, "state": {"Querétaro"}, "postal_code": {"76060"},
		"credit_days": {"30"}, "tax_regime": {"601"},
		"familia":   {familiaID(t, a.store, "CCA"), familiaID(t, a.store, "CCS & AC"), familiaID(t, a.store, "ABASTILUM")},
		"contactos": {"4"},
	}
	for i, c := range car.Contacts {
		p := "contacto-" + itoa(int64(i)) + "-"
		form.Set(p+"id", itoa(c.ID))
		form.Set(p+"nombre", c.Name)
		form.Set(p+"puesto", c.Role)
		form.Set(p+"telefono", c.Phone)
		form.Set(p+"email", c.Email)
		if i != 2 {
			form.Set(p+"oc", "1")
		}
	}
	form.Set("contacto-3-nombre", "Compras")
	if rec := muxDo(t, a, mux, admin, "POST", "/proveedores/"+id, form); rec.Code != http.StatusSeeOther {
		t.Fatalf("edit: %d %s", rec.Code, rec.Body.String())
	}
	page := muxDo(t, a, mux, admin, "GET", "/proveedores/"+id, nil).Body.String()
	for _, want := range []string{
		"<li>Régimen fiscal: (vacío) → 601 - General de Ley Personas Morales</li>",
		"<li>Días de crédito: (vacío) → 30</li>",
		"<li>Contacto Cotizaciones, Recibe O.C.: Sí → No</li>",
		"<li>Contacto agregado: Compras</li>",
		"<li>Familia que surte agregada: ABASTILUM</li>",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("Historial lacks %q", want)
		}
	}
	if n := strings.Count(page, "<strong>Ana López</strong>"); n != 1 {
		t.Errorf("one save makes %d Historial entries, want 1", n)
	}

	// Saving the same again, Compras now with its id, records nothing.
	sup := supplierByRFC(t, a.store, "CAR191126QX6")
	form.Set("contacto-3-id", itoa(sup.Contacts[3].ID))
	trail := func() int {
		entries, err := a.store.SupplierAuditLog(context.Background(), car.ID)
		if err != nil {
			t.Fatal(err)
		}
		return len(entries)
	}
	before := trail()
	if rec := muxDo(t, a, mux, admin, "POST", "/proveedores/"+id, form); rec.Code != http.StatusSeeOther {
		t.Fatalf("no-op edit: %d", rec.Code)
	}
	if n := trail() - before; n != 0 {
		t.Errorf("no-op save wrote %d audit rows", n)
	}
}
