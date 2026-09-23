package web

import (
	"net/http"
	"net/http/httptest"
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

// Ajustes holds the margin options and the materials; the FX rate and copper price it
// used to hold are gone (docs/PLAN.md, M3).
func TestAjustesPage(t *testing.T) {
	a := newTestAuth(t)
	s := NewSettings(a.store)
	adminID := createTestUser(t, a, "ana", "admin", "hunter2")

	req := withAuthCookie(t, a, adminID, httptest.NewRequest("GET", "/ajustes", nil))
	rec := httptest.NewRecorder()
	a.RequireAuth(a.RequireAdmin(http.HandlerFunc(s.Page))).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Page status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Márgenes", "Estándar", "Materiales", "CCS 30%"} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q", want)
		}
	}
	for _, gone := range []string{"Tipo de cambio", "Precio del cobre", `name="fx_rate"`} {
		if strings.Contains(body, gone) {
			t.Errorf("page still shows %q", gone)
		}
	}
}
