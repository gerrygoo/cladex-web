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

func TestNormalizeWhatsApp(t *testing.T) {
	for in, want := range map[string]string{
		"33 1234 5678":      "+523312345678",
		"(33) 1234-5678":    "+523312345678",
		"+52 33 1234 5678":  "+523312345678",
		"+1 (415) 555-0100": "+14155550100",
		"12345":             "",
		"+0 123 456 789":    "",
		"331234567":         "",
	} {
		got, ok := normalizeWhatsApp(in)
		if ok != (want != "") || (ok && got != want) {
			t.Errorf("normalizeWhatsApp(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
}

// serve runs one request through mux, as userID, with form as its body when set.
func serve(t *testing.T, a *Auth, mux http.Handler, userID int64, method, path string, form url.Values) *httptest.ResponseRecorder {
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

// TestAvisos covers Mi cuenta → Avisos and an admin's edit of another user's. Its strings
// are the ones docs/guia/acceso.md, "Recibir avisos", quotes.
func TestAvisos(t *testing.T) {
	a := newTestAuth(t)
	ctx := context.Background()
	ana := createTestUser(t, a, "ana", "vendedor", "hunter2")
	beto := createTestUser(t, a, "beto", "admin", "hunter2")
	users := NewUsers(a.store)
	mux := http.NewServeMux()
	mux.Handle("GET /mi-cuenta", a.RequireAuth(http.HandlerFunc(a.MiCuentaPage)))
	mux.Handle("POST /mi-cuenta/avisos", a.RequireAuth(http.HandlerFunc(a.MiCuentaAvisos)))
	mux.Handle("GET /usuarios/{id}/avisos", a.RequireAuth(a.RequireAdmin(http.HandlerFunc(users.AvisosPage))))
	mux.Handle("POST /usuarios/{id}/avisos", a.RequireAuth(a.RequireAdmin(http.HandlerFunc(users.Avisos))))

	// A vendedor sees which avisos they get but can't turn them on or off.
	body := serve(t, a, mux, ana, "GET", "/mi-cuenta", nil).Body.String()
	for _, want := range []string{"Avisos", "Correo", "WhatsApp", "No recibes avisos.", "Solo un administrador puede cambiar qué avisos recibes.", "Guardar avisos"} {
		if !strings.Contains(body, want) {
			t.Errorf("Mi cuenta missing %q", want)
		}
	}
	if strings.Contains(body, `name="notify_email"`) {
		t.Error("a vendedor's Mi cuenta offers the avisos toggles")
	}

	rec := serve(t, a, mux, ana, "POST", "/mi-cuenta/avisos", url.Values{
		"email": {" ana@cladex.com.mx "}, "whatsapp": {"(33) 1234-5678"}, "notify_email": {"1"},
	})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/mi-cuenta?avisos=1#avisos" {
		t.Fatalf("save: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	c, _ := a.store.UserContactByID(ctx, ana)
	if c != (store.UserContact{Email: "ana@cladex.com.mx", WhatsApp: "+523312345678"}) {
		t.Errorf("saved %+v; a vendedor's notify_email must be ignored", c)
	}
	body = serve(t, a, mux, ana, "GET", "/mi-cuenta?avisos=1", nil).Body.String()
	if !strings.Contains(body, "Avisos guardados.") || !strings.Contains(body, `value="+523312345678"`) {
		t.Error("the saved Avisos aren't shown")
	}

	for _, tc := range []struct {
		form url.Values
		want string
	}{
		{url.Values{"email": {"Ana <ana@cladex.com.mx>"}}, "Correo inválido"},
		{url.Values{"whatsapp": {"12345"}}, "Número de WhatsApp inválido"},
	} {
		rec := serve(t, a, mux, ana, "POST", "/mi-cuenta/avisos", tc.form)
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("%v: %d, want 422 with %q", tc.form, rec.Code, tc.want)
		}
	}
	if c2, _ := a.store.UserContactByID(ctx, ana); c2 != c {
		t.Errorf("a refused form changed the saved Avisos: %+v", c2)
	}

	// An admin turns Ana's email avisos on; Ana's own saves keep them, and her clearing the
	// address turns them off.
	if rec := serve(t, a, mux, beto, "GET", "/usuarios/1/avisos", nil); rec.Code != 200 || !strings.Contains(rec.Body.String(), "Avisos de ana") {
		t.Errorf("admin page: %d", rec.Code)
	}
	for _, tc := range []struct {
		form url.Values
		want string
	}{
		{url.Values{"notify_email": {"1"}}, "Escribe el correo para recibir avisos por correo."},
		{url.Values{"notify_whatsapp": {"1"}}, "Escribe el número para recibir avisos por WhatsApp."},
	} {
		rec := serve(t, a, mux, beto, "POST", "/usuarios/1/avisos", tc.form)
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("admin %v: %d, want 422 with %q", tc.form, rec.Code, tc.want)
		}
	}
	rec = serve(t, a, mux, beto, "POST", "/usuarios/1/avisos", url.Values{"email": {"ana@example.com"}, "notify_email": {"1"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("admin save: %d %s", rec.Code, rec.Body.String())
	}
	if c, _ := a.store.UserContactByID(ctx, ana); c != (store.UserContact{Email: "ana@example.com", NotifyEmail: true}) {
		t.Errorf("after the admin's edit: %+v", c)
	}
	if body := serve(t, a, mux, ana, "GET", "/mi-cuenta", nil).Body.String(); !strings.Contains(body, "Recibes avisos por correo.") {
		t.Error("Ana's Mi cuenta doesn't say she gets email avisos")
	}
	serve(t, a, mux, ana, "POST", "/mi-cuenta/avisos", url.Values{"email": {"ana@cladex.com.mx"}})
	if c, _ := a.store.UserContactByID(ctx, ana); c != (store.UserContact{Email: "ana@cladex.com.mx", NotifyEmail: true}) {
		t.Errorf("Ana's save without the toggle turned it off: %+v", c)
	}
	serve(t, a, mux, ana, "POST", "/mi-cuenta/avisos", url.Values{"email": {""}})
	if c, _ := a.store.UserContactByID(ctx, ana); c != (store.UserContact{}) {
		t.Errorf("Ana cleared her email but its avisos stay on: %+v", c)
	}

	// An admin's own Mi cuenta has the toggles.
	if body := serve(t, a, mux, beto, "GET", "/mi-cuenta", nil).Body.String(); !strings.Contains(body, `name="notify_email"`) {
		t.Error("an admin's Mi cuenta lacks the avisos toggles")
	}
	if rec := serve(t, a, mux, ana, "POST", "/usuarios/2/avisos", url.Values{"email": {"x@example.com"}}); rec.Code != http.StatusForbidden {
		t.Errorf("vendedor editing another user: %d", rec.Code)
	}
	if rec := serve(t, a, mux, beto, "GET", "/usuarios/99/avisos", nil); rec.Code != http.StatusNotFound {
		t.Errorf("missing user: %d", rec.Code)
	}
}
