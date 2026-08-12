package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func withAuthCookie(t *testing.T, a *Auth, userID int64, req *http.Request) *http.Request {
	t.Helper()
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
	return req
}

func TestUsuariosRequiresAdmin(t *testing.T) {
	a := newTestAuth(t)
	u := NewUsers(a.store)
	vendedorID := createTestUser(t, a, "rodolfo", "vendedor", "hunter2")

	req := withAuthCookie(t, a, vendedorID, httptest.NewRequest("GET", "/usuarios", nil))
	rec := httptest.NewRecorder()
	a.RequireAuth(a.RequireAdmin(http.HandlerFunc(u.List))).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for vendedor", rec.Code)
	}
}

func TestUsuariosListAndSetRole(t *testing.T) {
	a := newTestAuth(t)
	u := NewUsers(a.store)
	adminID := createTestUser(t, a, "ana", "admin", "hunter2")
	vendedorID := createTestUser(t, a, "rodolfo", "vendedor", "hunter2")

	req := withAuthCookie(t, a, adminID, httptest.NewRequest("GET", "/usuarios", nil))
	rec := httptest.NewRecorder()
	a.RequireAuth(a.RequireAdmin(http.HandlerFunc(u.List))).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("List status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "rodolfo") || !strings.Contains(rec.Body.String(), "ana") {
		t.Fatalf("List body missing users: %s", rec.Body.String())
	}

	// Promote rodolfo to admin.
	form := url.Values{"role": {"admin"}}
	target := "/usuarios/" + itoa(vendedorID) + "/rol"
	req = withAuthCookie(t, a, adminID, httptest.NewRequest("POST", target, strings.NewReader(form.Encode())))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", itoa(vendedorID))
	rec = httptest.NewRecorder()
	a.RequireAuth(a.RequireAdmin(http.HandlerFunc(u.SetRole))).ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("SetRole status = %d, want 303; body = %s", rec.Code, rec.Body.String())
	}

	updated, err := a.store.UserByID(context.Background(), vendedorID)
	if err != nil || updated == nil || updated.Role != "admin" {
		t.Fatalf("UserByID after SetRole = %+v, %v; want role admin", updated, err)
	}
}

func TestUsuariosCannotChangeOwnRole(t *testing.T) {
	a := newTestAuth(t)
	u := NewUsers(a.store)
	adminID := createTestUser(t, a, "ana", "admin", "hunter2")

	form := url.Values{"role": {"vendedor"}}
	target := "/usuarios/" + itoa(adminID) + "/rol"
	req := withAuthCookie(t, a, adminID, httptest.NewRequest("POST", target, strings.NewReader(form.Encode())))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", itoa(adminID))
	rec := httptest.NewRecorder()
	a.RequireAuth(a.RequireAdmin(http.HandlerFunc(u.SetRole))).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", rec.Code, rec.Body.String())
	}
	updated, err := a.store.UserByID(context.Background(), adminID)
	if err != nil || updated == nil || updated.Role != "admin" {
		t.Fatalf("own role changed despite guard: %+v, %v", updated, err)
	}
}

func TestUsuariosDisableAndEnable(t *testing.T) {
	a := newTestAuth(t)
	u := NewUsers(a.store)
	adminID := createTestUser(t, a, "ana", "admin", "hunter2")
	vendedorID := createTestUser(t, a, "rodolfo", "vendedor", "hunter2")

	target := "/usuarios/" + itoa(vendedorID) + "/deshabilitar"
	req := withAuthCookie(t, a, adminID, httptest.NewRequest("POST", target, nil))
	req.SetPathValue("id", itoa(vendedorID))
	rec := httptest.NewRecorder()
	a.RequireAuth(a.RequireAdmin(http.HandlerFunc(u.SetDisabled(true)))).ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("disable status = %d, want 303", rec.Code)
	}
	disabled, err := a.store.UserByID(context.Background(), vendedorID)
	if err != nil || disabled == nil || disabled.DisabledAt == nil {
		t.Fatalf("UserByID after disable = %+v, %v; want disabled", disabled, err)
	}

	target = "/usuarios/" + itoa(vendedorID) + "/habilitar"
	req = withAuthCookie(t, a, adminID, httptest.NewRequest("POST", target, nil))
	req.SetPathValue("id", itoa(vendedorID))
	rec = httptest.NewRecorder()
	a.RequireAuth(a.RequireAdmin(http.HandlerFunc(u.SetDisabled(false)))).ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("enable status = %d, want 303", rec.Code)
	}
	enabled, err := a.store.UserByID(context.Background(), vendedorID)
	if err != nil || enabled == nil || enabled.DisabledAt != nil {
		t.Fatalf("UserByID after re-enable = %+v, %v; want enabled", enabled, err)
	}
}

func TestUsuariosCannotDisableSelf(t *testing.T) {
	a := newTestAuth(t)
	u := NewUsers(a.store)
	adminID := createTestUser(t, a, "ana", "admin", "hunter2")

	target := "/usuarios/" + itoa(adminID) + "/deshabilitar"
	req := withAuthCookie(t, a, adminID, httptest.NewRequest("POST", target, nil))
	req.SetPathValue("id", itoa(adminID))
	rec := httptest.NewRecorder()
	a.RequireAuth(a.RequireAdmin(http.HandlerFunc(u.SetDisabled(true)))).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", rec.Code, rec.Body.String())
	}
	self, err := a.store.UserByID(context.Background(), adminID)
	if err != nil || self == nil || self.DisabledAt != nil {
		t.Fatalf("self got disabled despite guard: %+v, %v", self, err)
	}
}
