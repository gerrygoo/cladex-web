package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gerrygoo/cladex-web/internal/guia"
	"github.com/gerrygoo/cladex-web/internal/views"
)

func TestHelpPageByRole(t *testing.T) {
	a := newTestAuth(t)
	guide, err := guia.New(os.DirFS("../../docs/guia"))
	if err != nil {
		t.Fatal(err)
	}
	h := NewHelp(guide)
	mux := http.NewServeMux()
	mux.Handle("GET /ayuda", a.RequireAuth(http.HandlerFunc(h.Page)))
	mux.Handle("GET /ayuda/{pagina}", a.RequireAuth(http.HandlerFunc(h.Page)))

	vendedor := mustSessionToken(t, a, createTestUser(t, a, "rodolfo", "vendedor", "hunter2"))
	admin := mustSessionToken(t, a, createTestUser(t, a, "ana", "admin", "hunter2"))

	get := func(path, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		if token != "" {
			req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	if rec := get("/ayuda", ""); rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		t.Errorf("anonymous /ayuda = %d, want redirect to login", rec.Code)
	}

	rec := get("/ayuda/cotizaciones", vendedor)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `id="emitir-la-cotización"`) {
		t.Errorf("vendedor /ayuda/cotizaciones = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), `href="/ayuda/administracion"`) {
		t.Error("vendedor nav links to the admin page")
	}
	if rec := get("/ayuda/administracion", vendedor); rec.Code != http.StatusNotFound {
		t.Errorf("vendedor /ayuda/administracion = %d, want 404", rec.Code)
	}
	if rec := get("/ayuda/administracion", admin); rec.Code != http.StatusOK {
		t.Errorf("admin /ayuda/administracion = %d, want 200", rec.Code)
	}
	if rec := get("/ayuda/no-existe", admin); rec.Code != http.StatusNotFound {
		t.Errorf("/ayuda/no-existe = %d, want 404", rec.Code)
	}
}

// TestHelpTopicsResolve checks every screen's "?" link against the rendered guide, as
// the role that sees that screen — admin screens may point at admin-only sections.
func TestHelpTopicsResolve(t *testing.T) {
	guide, err := guia.New(os.DirFS("../../docs/guia"))
	if err != nil {
		t.Fatal(err)
	}
	for topic, adminOnly := range views.HelpTopics {
		if !guide.Resolves(string(topic), adminOnly) {
			t.Errorf("help topic %s does not resolve to a guide page and heading", topic)
		}
	}
}
