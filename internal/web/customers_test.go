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

	customers, err := a.store.ListCustomers(context.Background(), "", "", "")
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
