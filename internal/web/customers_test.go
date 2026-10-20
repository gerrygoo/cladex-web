package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gerrygoo/cladex-web/internal/store"
)

func newTestCustomers(t *testing.T, a *Auth) *Customers {
	t.Helper()
	return NewCustomers(a.store)
}

func storeCustomer(name string) store.Customer {
	return store.Customer{Name: name}
}

func TestCustomersCreateListEditDelete(t *testing.T) {
	a := newTestAuth(t)
	cs := newTestCustomers(t, a)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")

	// Create — plain POST + redirect, no htmx involved (JS-disabled path).
	form := url.Values{
		"name":         {"Grupo PEME"},
		"rfc":          {"PEM010101ABC"},
		"contact_name": {"Juan Pérez"},
		"email":        {"juan@grupopeme.mx"},
		"postal_code":  {"76000"},
		"tax_regime":   {"601 - General de Ley Personas Morales"},
	}
	req := httptest.NewRequest("POST", "/clientes/nuevo", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
	rec := httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(cs.Create)).ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Create status = %d, want 303; body = %s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/clientes/") || !strings.Contains(loc, "guardado=1") {
		t.Fatalf("Location = %q, want /clientes/{id}?guardado=1", loc)
	}
	id := strings.TrimSuffix(strings.TrimPrefix(loc, "/clientes/"), "?guardado=1")

	// List — the new customer shows up.
	req = httptest.NewRequest("GET", "/clientes", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
	rec = httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(cs.List)).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("List status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Grupo PEME") {
		t.Fatalf("List body missing new customer: %s", rec.Body.String())
	}

	// Edit page loads with the persisted values pre-filled.
	req = httptest.NewRequest("GET", "/clientes/"+id, nil)
	req.SetPathValue("id", id)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
	rec = httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(cs.EditPage)).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("EditPage status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "PEM010101ABC") {
		t.Fatalf("EditPage body missing prefilled RFC: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "76000") || !strings.Contains(rec.Body.String(), "General de Ley Personas Morales") {
		t.Fatalf("EditPage body missing prefilled postal code / tax regime: %s", rec.Body.String())
	}

	// Update — change the name.
	form = url.Values{
		"name":         {"Grupo PEME SA de CV"},
		"rfc":          {"PEM010101ABC"},
		"contact_name": {"Juan Pérez"},
		"email":        {"juan@grupopeme.mx"},
	}
	req = httptest.NewRequest("POST", "/clientes/"+id, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", id)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
	rec = httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(cs.Update)).ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Update status = %d, want 303; body = %s", rec.Code, rec.Body.String())
	}

	cust, err := a.store.CustomerByID(context.Background(), mustAtoi(t, id))
	if err != nil || cust == nil {
		t.Fatalf("CustomerByID after update: %v, %v", cust, err)
	}
	if cust.Name != "Grupo PEME SA de CV" {
		t.Fatalf("Name = %q, want updated value", cust.Name)
	}

	// Soft-delete — disappears from the list and edit page 404s.
	req = httptest.NewRequest("POST", "/clientes/"+id+"/eliminar", nil)
	req.SetPathValue("id", id)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
	rec = httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(cs.Delete)).ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Delete status = %d, want 303", rec.Code)
	}

	req = httptest.NewRequest("GET", "/clientes", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
	rec = httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(cs.List)).ServeHTTP(rec, req)
	if strings.Contains(rec.Body.String(), "Grupo PEME") {
		t.Fatalf("List still shows soft-deleted customer: %s", rec.Body.String())
	}

	req = httptest.NewRequest("GET", "/clientes/"+id, nil)
	req.SetPathValue("id", id)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
	rec = httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(cs.EditPage)).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("EditPage after delete status = %d, want 404", rec.Code)
	}
}

func TestCustomersCreateValidationErrors(t *testing.T) {
	a := newTestAuth(t)
	cs := newTestCustomers(t, a)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")

	form := url.Values{
		"name":  {""},
		"email": {"not-an-email"},
	}
	req := httptest.NewRequest("POST", "/clientes/nuevo", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
	rec := httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(cs.Create)).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"El nombre es obligatorio", "Email inválido"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q: %s", want, body)
		}
	}

	customers, err := a.store.ListCustomers(context.Background(), "", "", "", nil)
	if err != nil {
		t.Fatalf("ListCustomers: %v", err)
	}
	if len(customers) != 0 {
		t.Fatalf("ListCustomers = %d, want 0 (nothing should have been created)", len(customers))
	}
}

func TestCustomersListHTMXReturnsOnlyTableFragment(t *testing.T) {
	a := newTestAuth(t)
	cs := newTestCustomers(t, a)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	if _, err := a.store.CreateCustomer(context.Background(), storeCustomer("Cliente X")); err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}

	req := httptest.NewRequest("GET", "/clientes?q=Cliente", nil)
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
	rec := httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(cs.List)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "<html") || strings.Contains(body, "<!DOCTYPE") {
		t.Fatalf("htmx response should be a fragment, not a full page: %s", body)
	}
	if !strings.Contains(body, "id=\"clientes-tbody\"") || !strings.Contains(body, "Cliente X") {
		t.Fatalf("fragment missing expected table content: %s", body)
	}
}

func TestCustomersRequireAuth(t *testing.T) {
	a := newTestAuth(t)
	cs := newTestCustomers(t, a)

	req := httptest.NewRequest("GET", "/clientes", nil)
	rec := httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(cs.List)).ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302 redirect to /login", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Fatalf("Location = %q, want /login", loc)
	}
}

func TestCustomersListColumnFilterKeptAcrossSortAndSearch(t *testing.T) {
	a := newTestAuth(t)
	cs := newTestCustomers(t, a)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	ctx := context.Background()
	for _, c := range []store.Customer{
		{Name: "Grupo PEME", RFC: "PEM010101ABC"},
		{Name: "Cables del Norte", RFC: "CNO010101XYZ"},
	} {
		if _, err := a.store.CreateCustomer(ctx, c); err != nil {
			t.Fatalf("CreateCustomer: %v", err)
		}
	}

	req := httptest.NewRequest("GET", "/clientes?f.rfc=PEM&sort=name&dir=desc", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
	rec := httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(cs.List)).ServeHTTP(rec, req)
	body := rec.Body.String()

	if rec.Code != http.StatusOK || !strings.Contains(body, "Grupo PEME") || strings.Contains(body, "Cables del Norte") {
		t.Fatalf("filtered list: status %d, body should hold only Grupo PEME:\n%s", rec.Code, body)
	}
	// The sort links and the search form carry the filter along.
	if !strings.Contains(body, "f.rfc=PEM") || !strings.Contains(body, `name="f.rfc"`) {
		t.Errorf("filter is not preserved in the page's links and forms")
	}
	if !strings.Contains(body, "Quitar todos") {
		t.Errorf("active-filter notice missing")
	}
}

// TestNewCustomerFromQuoteAndDuplicates covers issue #29: Nueva cotización's list offers
// "Agregar nuevo cliente", which opens the new-customer form and comes back with the
// customer and series selected; Cancelar goes back too. Before creating, a possible
// duplicate (same RFC, or a name equal or contained once normalized) is shown with a way
// to use it, to create anyway, or to edit; without matches it saves directly. A vendedor
// can do all of it.
func TestNewCustomerFromQuoteAndDuplicates(t *testing.T) {
	a := newTestAuth(t)
	cs := newTestCustomers(t, a)
	q := newTestQuotes(t, a)
	ctx := context.Background()
	userID := createTestUser(t, a, "rfm", "vendedor", "hunter2")
	sonepar, _ := a.store.CreateCustomer(ctx, store.Customer{Name: "SONEPAR", ContactName: "Alfonso Felix"})
	peme, _ := a.store.CreateCustomer(ctx, store.Customer{Name: "Grupo PEME", RFC: "PEM-010101-ABC"})

	// The list has the search and the fixed first entry, and comes back preselected.
	page := doForm(t, a, userID, q.NewPage, "GET", "/cotizaciones/nueva?cliente="+itoa(peme)+"&serie=QA", nil, nil, false).Body.String()
	for _, want := range []string{"data-customer-search", `<option value="nuevo" data-keep>➕ Agregar nuevo cliente</option>`,
		`value="` + itoa(peme) + `" data-search="Grupo PEME PEM-010101-ABC " selected`, `value="QA" checked`} {
		if !strings.Contains(page, want) {
			t.Errorf("Nueva cotización missing %q", want)
		}
	}
	// Without JS, "Agregar nuevo cliente" goes to the form through the server.
	rec := doForm(t, a, userID, q.Create, "POST", "/cotizaciones/nueva", nil, url.Values{"customer_id": {"nuevo"}, "prefix": {"QA"}}, false)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/clientes/nuevo?serie=QA&volver=cotizacion" {
		t.Fatalf("Create(nuevo) = %d, Location %q", rec.Code, rec.Header().Get("Location"))
	}
	form := doForm(t, a, userID, cs.NewPage, "GET", "/clientes/nuevo?volver=cotizacion&serie=QA", nil, nil, false).Body.String()
	for _, want := range []string{`name="volver" value="cotizacion"`, `name="serie" value="QA"`, `href="/cotizaciones/nueva?serie=QA">Cancelar`} {
		if !strings.Contains(form, want) {
			t.Errorf("new-customer form from a quote missing %q", want)
		}
	}

	create := func(f url.Values) *httptest.ResponseRecorder {
		return doForm(t, a, userID, cs.Create, "POST", "/clientes/nuevo", nil, f, false)
	}
	count := func() int {
		all, _ := a.store.ListCustomers(ctx, "", "name", "asc", nil)
		return len(all)
	}
	// A name like an existing one: warned, nothing created.
	fromQuote := url.Values{"name": {"Sonepar S.A. de C.V."}, "volver": {"cotizacion"}, "serie": {"QA"}}
	rec = create(fromQuote)
	body := rec.Body.String()
	if rec.Code != http.StatusUnprocessableEntity || count() != 2 {
		t.Fatalf("Create(duplicate) = %d with %d customers, want 422 and none created", rec.Code, count())
	}
	for _, want := range []string{"Este cliente podría estar duplicado", "SONEPAR — RFC: (sin RFC) — Contacto: Alfonso Felix",
		`href="/cotizaciones/nueva?cliente=` + itoa(sonepar) + `&amp;serie=QA"`, `name="confirmar" value="1"`, "Regresar a editar", `value="Sonepar S.A. de C.V."`} {
		if !strings.Contains(body, want) {
			t.Errorf("duplicate warning missing %q", want)
		}
	}
	// "Sí, darlo de alta de todos modos" creates it and goes back to the quote with it.
	fromQuote.Set("confirmar", "1")
	rec = create(fromQuote)
	if rec.Code != http.StatusSeeOther || count() != 3 || !strings.HasPrefix(rec.Header().Get("Location"), "/cotizaciones/nueva?cliente=") ||
		!strings.HasSuffix(rec.Header().Get("Location"), "&serie=QA") {
		t.Fatalf("Create(confirmed) = %d, Location %q, %d customers", rec.Code, rec.Header().Get("Location"), count())
	}

	// From the catalog: the same RFC warns though the name differs, and "Usar este
	// cliente" leads back to the list.
	rec = create(url.Values{"name": {"Otra Razón"}, "rfc": {"pem 010101 abc"}})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Grupo PEME — RFC: PEM-010101-ABC") ||
		!strings.Contains(rec.Body.String(), `href="/clientes" class="button">Usar este cliente`) {
		t.Errorf("same RFC from the catalog = %d, want the warning pointing at the list", rec.Code)
	}
	// Nothing alike: saved right away.
	if rec := create(url.Values{"name": {"Constructora Nueva"}}); rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "guardado=1") {
		t.Errorf("Create(unique) = %d, Location %q", rec.Code, rec.Header().Get("Location"))
	}
}
