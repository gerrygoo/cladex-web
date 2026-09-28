package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/gerrygoo/cladex-web/internal/store"
)

func newTestProducts(t *testing.T, a *Auth) *Products {
	t.Helper()
	return NewProducts(a.store)
}

func seedFamily(t *testing.T, a *Auth, name string) int64 {
	t.Helper()
	id, err := a.store.UpsertFamily(context.Background(), name, name)
	if err != nil {
		t.Fatalf("UpsertFamily: %v", err)
	}
	return id
}

func storeProduct(familyID int64, sku, description string) store.Product {
	return store.Product{FamilyID: familyID, SKU: sku, Description: description}
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

func mustAtoi(t *testing.T, s string) int64 {
	t.Helper()
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		t.Fatalf("ParseInt(%q): %v", s, err)
	}
	return n
}

func TestProductsCreateListEditDelete(t *testing.T) {
	a := newTestAuth(t)
	p := newTestProducts(t, a)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	familyID := seedFamily(t, a, "ABASTILUM")

	// Create — plain POST + redirect, no htmx involved (JS-disabled path).
	form := url.Values{
		"family_id":   {itoa(familyID)},
		"sku":         {"abl-cable-thw-14"},
		"description": {"Cable THW Cal. 14"},
		"cost":        {"6.319872"},
	}
	req := httptest.NewRequest("POST", "/productos/nuevo", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
	rec := httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(p.Create)).ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Create status = %d, want 303; body = %s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/productos/") || !strings.Contains(loc, "guardado=1") {
		t.Fatalf("Location = %q, want /productos/{id}?guardado=1", loc)
	}
	id := strings.TrimSuffix(strings.TrimPrefix(loc, "/productos/"), "?guardado=1")

	// List — the new product shows up.
	req = httptest.NewRequest("GET", "/productos", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
	rec = httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(p.List)).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("List status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "abl-cable-thw-14") {
		t.Fatalf("List body missing new product: %s", rec.Body.String())
	}

	// Edit page loads with the persisted values pre-filled.
	req = httptest.NewRequest("GET", "/productos/"+id, nil)
	req.SetPathValue("id", id)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
	rec = httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(p.EditPage)).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("EditPage status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "6.319872") {
		t.Fatalf("EditPage body missing prefilled price: %s", rec.Body.String())
	}

	// Update — change the description.
	form = url.Values{
		"family_id":   {itoa(familyID)},
		"sku":         {"abl-cable-thw-14"},
		"description": {"Cable THW Cal. 14 AWG"},
		"cost":        {"6.319872"},
	}
	req = httptest.NewRequest("POST", "/productos/"+id, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", id)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
	rec = httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(p.Update)).ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Update status = %d, want 303; body = %s", rec.Code, rec.Body.String())
	}

	prod, err := a.store.ProductByID(context.Background(), mustAtoi(t, id))
	if err != nil || prod == nil {
		t.Fatalf("ProductByID after update: %v, %v", prod, err)
	}
	if prod.Description != "Cable THW Cal. 14 AWG" {
		t.Fatalf("Description = %q, want updated value", prod.Description)
	}

	// Soft-delete — disappears from the list and edit page 404s.
	req = httptest.NewRequest("POST", "/productos/"+id+"/eliminar", nil)
	req.SetPathValue("id", id)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
	rec = httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(p.Delete)).ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Delete status = %d, want 303", rec.Code)
	}

	req = httptest.NewRequest("GET", "/productos", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
	rec = httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(p.List)).ServeHTTP(rec, req)
	if strings.Contains(rec.Body.String(), "abl-cable-thw-14") {
		t.Fatalf("List still shows soft-deleted product: %s", rec.Body.String())
	}

	req = httptest.NewRequest("GET", "/productos/"+id, nil)
	req.SetPathValue("id", id)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
	rec = httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(p.EditPage)).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("EditPage after delete status = %d, want 404", rec.Code)
	}
}

func TestProductsCreateValidationErrors(t *testing.T) {
	a := newTestAuth(t)
	p := newTestProducts(t, a)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")

	form := url.Values{
		"family_id":   {""},
		"sku":         {""},
		"description": {""},
		"cost":        {"no-es-un-numero"},
	}
	req := httptest.NewRequest("POST", "/productos/nuevo", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
	rec := httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(p.Create)).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"El SKU es obligatorio", "La descripción es obligatoria", "Selecciona una familia", "Costo inválido"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q: %s", want, body)
		}
	}

	products, err := a.store.ListProducts(context.Background(), "", "", "", nil)
	if err != nil {
		t.Fatalf("ListProducts: %v", err)
	}
	if len(products) != 0 {
		t.Fatalf("ListProducts = %d, want 0 (nothing should have been created)", len(products))
	}
}

func TestProductsCreateDuplicateSKU(t *testing.T) {
	a := newTestAuth(t)
	p := newTestProducts(t, a)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	familyID := seedFamily(t, a, "ABASTILUM")

	form := url.Values{
		"family_id":   {itoa(familyID)},
		"sku":         {"abl-dup"},
		"description": {"Primero"},
	}
	req := httptest.NewRequest("POST", "/productos/nuevo", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
	rec := httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(p.Create)).ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("first Create status = %d, want 303", rec.Code)
	}

	form.Set("description", "Segundo")
	req = httptest.NewRequest("POST", "/productos/nuevo", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
	rec = httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(p.Create)).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("second Create status = %d, want 422; body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Ya existe un producto con este SKU") {
		t.Fatalf("body missing duplicate SKU error: %s", rec.Body.String())
	}
}

func TestProductsListHTMXReturnsOnlyTableFragment(t *testing.T) {
	a := newTestAuth(t)
	p := newTestProducts(t, a)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	familyID := seedFamily(t, a, "ABASTILUM")
	if _, err := a.store.CreateProduct(context.Background(), storeProduct(familyID, "abl-x", "Producto X")); err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	req := httptest.NewRequest("GET", "/productos?q=Producto", nil)
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
	rec := httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(p.List)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "<html") || strings.Contains(body, "<!DOCTYPE") {
		t.Fatalf("htmx response should be a fragment, not a full page: %s", body)
	}
	if !strings.Contains(body, "id=\"productos-tbody\"") || !strings.Contains(body, "Producto X") {
		t.Fatalf("fragment missing expected table content: %s", body)
	}
}

func TestProductsRequireAuth(t *testing.T) {
	a := newTestAuth(t)
	p := newTestProducts(t, a)

	req := httptest.NewRequest("GET", "/productos", nil)
	rec := httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(p.List)).ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302 redirect to /login", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Fatalf("Location = %q, want /login", loc)
	}
}
