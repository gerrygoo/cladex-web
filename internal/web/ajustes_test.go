package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAjustesRequiresAdmin(t *testing.T) {
	a := newTestAuth(t)
	s := NewSettings(a.store)
	vendedorID := createTestUser(t, a, "rodolfo", "vendedor", "hunter2")

	req := withAuthCookie(t, a, vendedorID, httptest.NewRequest("GET", "/ajustes", nil))
	rec := httptest.NewRecorder()
	a.RequireAuth(a.RequireAdmin(http.HandlerFunc(s.Page))).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for vendedor", rec.Code)
	}
}

func TestAjustesSaveAndReload(t *testing.T) {
	a := newTestAuth(t)
	s := NewSettings(a.store)
	adminID := createTestUser(t, a, "ana", "admin", "hunter2")

	// First load: nothing set yet, fields blank.
	req := withAuthCookie(t, a, adminID, httptest.NewRequest("GET", "/ajustes", nil))
	rec := httptest.NewRecorder()
	a.RequireAuth(a.RequireAdmin(http.HandlerFunc(s.Page))).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Page status = %d, want 200", rec.Code)
	}

	form := url.Values{
		"fx_rate": {"18.50"},
	}
	req = withAuthCookie(t, a, adminID, httptest.NewRequest("POST", "/ajustes", strings.NewReader(form.Encode())))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	a.RequireAuth(a.RequireAdmin(http.HandlerFunc(s.Submit))).ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Submit status = %d, want 303; body = %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/ajustes?guardado=1" {
		t.Fatalf("Location = %q, want /ajustes?guardado=1", loc)
	}

	// Reload: the saved FX rate is reflected back exactly.
	req = withAuthCookie(t, a, adminID, httptest.NewRequest("GET", "/ajustes", nil))
	rec = httptest.NewRecorder()
	a.RequireAuth(a.RequireAdmin(http.HandlerFunc(s.Page))).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Page after save status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"18.5"} {
		if !strings.Contains(body, want) {
			t.Errorf("reloaded page missing value %q: %s", want, body)
		}
	}
}

func TestAjustesValidationError(t *testing.T) {
	a := newTestAuth(t)
	s := NewSettings(a.store)
	adminID := createTestUser(t, a, "ana", "admin", "hunter2")

	form := url.Values{
		"fx_rate": {"no-es-numero"},
	}
	req := withAuthCookie(t, a, adminID, httptest.NewRequest("POST", "/ajustes", strings.NewReader(form.Encode())))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	a.RequireAuth(a.RequireAdmin(http.HandlerFunc(s.Submit))).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", rec.Code, rec.Body.String())
	}

	// Nothing should have been persisted.
	if v, err := a.store.SettingValue(req.Context(), "fx_rate"); err != nil || v != "" {
		t.Fatalf("fx_rate = %q, %v; want unset after a failed submit", v, err)
	}
}
