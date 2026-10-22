package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/store"
	"golang.org/x/crypto/bcrypt"
)

// Issue #41: products are only editable by an admin once created, customers can't be
// deleted by a vendedor, and a customer's page shows its Historial. These also pin what
// docs/guia/ quotes of it: README.md "Quién puede hacer qué", productos.md and
// clientes.md "Historial de un cliente".

// createNamedUser is createTestUser with a full name, which the Historial shows.
func createNamedUser(t *testing.T, a *Auth, username, name, role string) int64 {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("hunter2"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	id, err := a.store.CreateUser(context.Background(), username, name, string(hash), role)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return id
}

// muxDo sends one request through the real router as userID; form, when non-nil, is
// posted as the body.
func muxDo(t *testing.T, a *Auth, mux http.Handler, userID int64, method, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if form != nil {
		req = httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, withAuthCookie(t, a, userID, req))
	return rec
}

func TestProductAndCustomerPermissions(t *testing.T) {
	a := newTestAuth(t)
	mux := newTestMux(t, a)
	ctx := context.Background()
	vendedor := createTestUser(t, a, "rfm", "vendedor", "hunter2")
	admin := createTestUser(t, a, "ana", "admin", "hunter2")
	familyID := seedFamily(t, a, "ABASTILUM")
	unitID, err := a.store.CreateUnit(ctx, "zz", "unidad de prueba")
	if err != nil {
		t.Fatalf("CreateUnit: %v", err)
	}
	materialID, err := a.store.CreateMaterial(ctx, "Cobre", unitID, money.Micros(160_000_000), admin)
	if err != nil {
		t.Fatalf("CreateMaterial: %v", err)
	}

	productForm := func(sku, cost string) url.Values {
		return url.Values{"family_id": {itoa(familyID)}, "sku": {sku}, "description": {"Cable THW 14"}, "cost": {cost}}
	}

	// A vendedor can still create a product.
	rec := muxDo(t, a, mux, vendedor, "POST", "/productos/nuevo", productForm("thw-14", "6.50"))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("vendedor create product: status = %d, want 303; body = %s", rec.Code, rec.Body.String())
	}
	pid := strings.TrimSuffix(strings.TrimPrefix(rec.Header().Get("Location"), "/productos/"), "?guardado=1")

	// Everything that changes it afterwards is admin-only, enforced on the server.
	cust, err := a.store.CreateCustomer(ctx, store.Customer{Name: "Grupo PEME"})
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	locked := []struct {
		path string
		form url.Values
	}{
		{"/productos/" + pid, productForm("thw-14", "1.00")},
		{"/productos/" + pid + "/eliminar", url.Values{}},
		{"/productos/" + pid + "/conversiones", url.Values{"from_unit_id": {itoa(unitID)}, "to_unit_id": {itoa(unitID)}, "rate": {"2"}}},
		{"/productos/" + pid + "/conversiones/1/eliminar", url.Values{}},
		{"/productos/" + pid + "/materiales", url.Values{"material_id": {itoa(materialID)}, "qty": {"0.5"}}},
		{"/productos/" + pid + "/materiales/1/eliminar", url.Values{}},
		{"/clientes/" + itoa(cust) + "/eliminar", url.Values{}},
	}
	for _, l := range locked {
		rec := muxDo(t, a, mux, vendedor, "POST", l.path, l.form)
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "No tienes permiso para hacer esto. Solo un administrador puede hacerlo.") {
			t.Errorf("vendedor POST %s: status = %d, body = %q; want 403 No tienes permiso", l.path, rec.Code, rec.Body.String())
		}
	}
	cost := func() money.Micros {
		p, err := a.store.ProductByID(ctx, mustAtoi(t, pid))
		if err != nil || p == nil || p.CostMicros == nil {
			t.Fatalf("ProductByID: %+v, %v", p, err)
		}
		return *p.CostMicros
	}
	if p, _ := a.store.ProductByID(ctx, mustAtoi(t, pid)); p == nil || cost() != money.Micros(6_500_000) {
		t.Errorf("product changed by a refused vendedor request: %+v", p)
	}
	if c, _ := a.store.CustomerByID(ctx, cust); c == nil {
		t.Error("customer deleted by a refused vendedor request")
	}

	// A vendedor sees the product read-only, and no Eliminar in either list.
	body := muxDo(t, a, mux, vendedor, "GET", "/productos/"+pid, nil).Body.String()
	if !strings.Contains(body, "Solo un administrador puede modificar o eliminar este producto.") ||
		strings.Contains(body, `action="/productos/`+pid+`"`) || strings.Contains(body, "Agregar material") ||
		strings.Contains(body, "Agregar conversión") || strings.Contains(body, "Guardar") {
		t.Errorf("vendedor product page is not read-only: %s", body)
	}
	if body := muxDo(t, a, mux, vendedor, "GET", "/productos", nil).Body.String(); strings.Contains(body, "Eliminar") || !strings.Contains(body, ">Ver<") {
		t.Errorf("vendedor product list offers Eliminar or lacks Ver: %s", body)
	}
	if body := muxDo(t, a, mux, vendedor, "GET", "/clientes", nil).Body.String(); strings.Contains(body, "¿Eliminar este cliente?") {
		t.Errorf("vendedor customer list offers Eliminar: %s", body)
	}

	// An admin keeps every permission.
	if body := muxDo(t, a, mux, admin, "GET", "/productos/"+pid, nil).Body.String(); !strings.Contains(body, `action="/productos/`+pid+`"`) || !strings.Contains(body, "Agregar material") {
		t.Errorf("admin product page lost its forms: %s", body)
	}
	if body := muxDo(t, a, mux, admin, "GET", "/clientes", nil).Body.String(); !strings.Contains(body, "¿Eliminar este cliente?") {
		t.Errorf("admin customer list lost Eliminar: %s", body)
	}
	for _, l := range locked[:6] {
		if strings.HasSuffix(l.path, "/eliminar") && strings.Contains(l.path, "/productos/"+pid+"/eliminar") {
			continue // deleting the product goes last
		}
		if rec := muxDo(t, a, mux, admin, "POST", l.path, l.form); rec.Code == http.StatusForbidden {
			t.Errorf("admin POST %s: 403", l.path)
		}
	}
	if got := cost(); got != money.Micros(1_000_000) {
		t.Errorf("admin edit didn't apply: cost = %v", got)
	}
	for _, path := range []string{"/productos/" + pid + "/eliminar", "/clientes/" + itoa(cust) + "/eliminar"} {
		if rec := muxDo(t, a, mux, admin, "POST", path, url.Values{}); rec.Code != http.StatusSeeOther {
			t.Errorf("admin POST %s: status = %d, want 303", path, rec.Code)
		}
	}
}

func TestNewProductAsksBeforeCreating(t *testing.T) {
	a := newTestAuth(t)
	mux := newTestMux(t, a)
	vendedor := createTestUser(t, a, "rfm", "vendedor", "hunter2")

	body := muxDo(t, a, mux, vendedor, "GET", "/productos/nuevo", nil).Body.String()
	for _, want := range []string{
		`data-confirm-dialog="confirmar-alta"`,
		`<dialog id="confirmar-alta"`,
		"¿Revisaste que todos los datos sean correctos?",
		"Una vez creado, solo un administrador podrá modificarlo.",
		`<button type="submit" class="primary">Sí, crear producto</button>`,
		`<button type="button" data-close-dialog>Regresar a revisar</button>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Nuevo producto missing %q", want)
		}
	}
}

func TestCustomerHistory(t *testing.T) {
	a := newTestAuth(t)
	mux := newTestMux(t, a)
	ctx := context.Background()
	emilio := createNamedUser(t, a, "efs", "Emilio Flores", "vendedor")
	rodolfo := createNamedUser(t, a, "rfm", "Rodolfo Flores", "vendedor")

	// Created from Nueva cotización's "Agregar nuevo cliente": still attributed.
	form := url.Values{"name": {"Constructora Bajío"}, "phone": {"5554090522"}, "volver": {"cotizacion"}}
	rec := muxDo(t, a, mux, emilio, "POST", "/clientes/nuevo", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create: status = %d; body = %s", rec.Code, rec.Body.String())
	}
	customers, err := a.store.ListCustomers(ctx, "Constructora", "", "", nil)
	if err != nil || len(customers) != 1 {
		t.Fatalf("ListCustomers: %v, %v", customers, err)
	}
	id := itoa(customers[0].ID)

	page := muxDo(t, a, mux, rodolfo, "GET", "/clientes/"+id, nil).Body.String()
	if !strings.Contains(page, `<h2 id="historial">Historial</h2>`) || !strings.Contains(page, "Creado por Emilio Flores · ") {
		t.Fatalf("new customer's Historial lacks its creation: %s", page)
	}

	// A vendedor edits the RFC and phone: one entry, with who and old → new.
	form = url.Values{"name": {"Constructora Bajío"}, "rfc": {"cuv130412725"}, "phone": {"5554090523"}}
	if rec := muxDo(t, a, mux, rodolfo, "POST", "/clientes/"+id, form); rec.Code != http.StatusSeeOther {
		t.Fatalf("vendedor edit: status = %d; body = %s", rec.Code, rec.Body.String())
	}
	page = muxDo(t, a, mux, rodolfo, "GET", "/clientes/"+id, nil).Body.String()
	for _, want := range []string{
		"<strong>Rodolfo Flores</strong>",
		"<li>RFC: (vacío) → CUV130412725</li>",
		"<li>Teléfono: 5554090522 → 5554090523</li>",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("Historial missing %q: %s", want, page)
		}
	}
	if strings.Index(page, "Rodolfo Flores</strong>") > strings.Index(page, "Creado por Emilio Flores") {
		t.Error("Historial is not newest first")
	}

	// Saving again without changes records nothing.
	trail := func() int {
		entries, err := a.store.AuditLog(ctx, store.AuditFilter{TableName: "customers", RowKey: id})
		if err != nil {
			t.Fatalf("AuditLog: %v", err)
		}
		return len(entries)
	}
	before := trail()
	if rec := muxDo(t, a, mux, rodolfo, "POST", "/clientes/"+id, form); rec.Code != http.StatusSeeOther {
		t.Fatalf("no-op save: status = %d", rec.Code)
	}
	if after := trail(); after != before {
		t.Errorf("no-op save added %d audit entries", after-before)
	}
	page = muxDo(t, a, mux, rodolfo, "GET", "/clientes/"+id, nil).Body.String()
	if n := strings.Count(page, "<strong>Rodolfo Flores</strong>"); n != 1 {
		t.Errorf("Historial shows %d edits, want 1", n)
	}
}

// A customer written with no user behind it (the admin command line) reads "Sistema".
func TestCustomerHistoryWithoutUser(t *testing.T) {
	a := newTestAuth(t)
	mux := newTestMux(t, a)
	user := createTestUser(t, a, "ana", "admin", "hunter2")
	cliCtx := store.WithActor(context.Background(), store.Actor{Source: store.SourceCLI})
	id, err := a.store.CreateCustomer(cliCtx, store.Customer{Name: "SONEPAR"})
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	page := muxDo(t, a, mux, user, "GET", "/clientes/"+itoa(id), nil).Body.String()
	if !strings.Contains(page, "Creado por Sistema · ") {
		t.Errorf("CLI-created customer's Historial: %s", page)
	}
}
