package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// adminUnits wires the /unidades handlers the way router.go does, so each test below
// exercises the same middleware chain a real request goes through.
func adminUnits(t *testing.T, a *Auth) (*Units, http.Handler, http.Handler) {
	t.Helper()
	u := NewUnits(a.store)
	list := a.RequireAuth(a.RequireAdmin(http.HandlerFunc(u.List)))
	create := a.RequireAuth(a.RequireAdmin(http.HandlerFunc(u.Create)))
	return u, list, create
}

func postUnit(t *testing.T, a *Auth, create http.Handler, userID int64, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := withAuthCookie(t, a, userID, httptest.NewRequest("POST", "/unidades", strings.NewReader(form.Encode())))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	create.ServeHTTP(rec, req)
	return rec
}

func TestUnidadesListShowsSeededUnits(t *testing.T) {
	a := newTestAuth(t)
	_, list, _ := adminUnits(t, a)
	adminID := createTestUser(t, a, "ana", "admin", "hunter2")

	req := withAuthCookie(t, a, adminID, httptest.NewRequest("GET", "/unidades", nil))
	rec := httptest.NewRecorder()
	list.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	// Seeded by migrations/0003_add_units_and_conversions.sql.
	for _, want := range []string{"rollo", "Rollo", "kg", "Kilogramo"} {
		if !strings.Contains(body, want) {
			t.Errorf("list body missing seeded unit %q", want)
		}
	}
}

func TestUnidadesCreateAndReload(t *testing.T) {
	a := newTestAuth(t)
	_, list, create := adminUnits(t, a)
	adminID := createTestUser(t, a, "ana", "admin", "hunter2")

	rec := postUnit(t, a, create, adminID, url.Values{"code": {"caja"}, "name": {"Caja"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Create status = %d, want 303; body = %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/unidades?guardado=1" {
		t.Fatalf("Location = %q, want /unidades?guardado=1", loc)
	}

	// Follow the redirect: the new unit round-tripped through SQLite and the success
	// banner is keyed off the query parameter.
	req := withAuthCookie(t, a, adminID, httptest.NewRequest("GET", "/unidades?guardado=1", nil))
	rec = httptest.NewRecorder()
	list.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "caja") || !strings.Contains(body, "Caja") {
		t.Errorf("list body missing the created unit: %s", body)
	}
	if !strings.Contains(body, "Unidad guardada.") {
		t.Errorf("list body missing the success message: %s", body)
	}
}

func TestUnidadesCreateRejectsDuplicateCode(t *testing.T) {
	a := newTestAuth(t)
	_, _, create := adminUnits(t, a)
	adminID := createTestUser(t, a, "ana", "admin", "hunter2")

	// "m" is seeded by the migration.
	rec := postUnit(t, a, create, adminID, url.Values{"code": {"m"}, "name": {"Metro lineal"}})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Ya existe una unidad con este código.") {
		t.Errorf("body missing the duplicate-code message: %s", rec.Body.String())
	}
	// The page re-renders the list alongside the error rather than losing it.
	if !strings.Contains(rec.Body.String(), "Kilogramo") {
		t.Errorf("error page dropped the units list: %s", rec.Body.String())
	}
}

func TestUnidadesCreateRequiresBothFields(t *testing.T) {
	a := newTestAuth(t)
	_, _, create := adminUnits(t, a)
	adminID := createTestUser(t, a, "ana", "admin", "hunter2")

	for name, form := range map[string]url.Values{
		"missing name": {"code": {"caja"}, "name": {""}},
		"missing code": {"code": {""}, "name": {"Caja"}},
		// Whitespace is trimmed before the check, so a space-only field is empty.
		"blank code": {"code": {"   "}, "name": {"Caja"}},
	} {
		t.Run(name, func(t *testing.T) {
			rec := postUnit(t, a, create, adminID, form)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), "El código y el nombre son obligatorios.") {
				t.Errorf("body missing the validation message: %s", rec.Body.String())
			}
		})
	}

	units, err := a.store.ListUnits(t.Context())
	if err != nil {
		t.Fatalf("ListUnits: %v", err)
	}
	if len(units) != 4 {
		t.Fatalf("len(units) = %d, want the 4 seeded ones — a rejected submit wrote a row", len(units))
	}
}
