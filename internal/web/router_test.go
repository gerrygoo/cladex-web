package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	cladex "github.com/gerrygoo/cladex-web"
	"github.com/gerrygoo/cladex-web/internal/guia"
)

// route mirrors one mux.Handle line in router.go. The point of the table is that it is
// maintained by hand: adding a route to NewMux without adding it here, or gating it
// differently than it is gated there, is what these tests are for. The handler tests
// elsewhere in this package wrap RequireAuth/RequireAdmin themselves, so they prove the
// middleware works — only these tests prove the router actually applies it.
type route struct {
	method    string
	path      string
	adminOnly bool
}

var appRoutes = []route{
	{"GET", "/", false},
	{"GET", "/mi-cuenta", false},
	{"POST", "/mi-cuenta", false},

	{"GET", "/productos", false},
	{"GET", "/productos/nuevo", false},
	{"POST", "/productos/nuevo", false},
	{"GET", "/productos/1", false},
	{"POST", "/productos/1", false},
	{"POST", "/productos/1/eliminar", false},
	{"POST", "/productos/1/conversiones", false},
	{"POST", "/productos/1/conversiones/1/eliminar", false},
	{"POST", "/productos/1/materiales", false},
	{"POST", "/productos/1/materiales/1/eliminar", false},

	{"GET", "/clientes", false},
	{"GET", "/clientes/nuevo", false},
	{"POST", "/clientes/nuevo", false},
	{"GET", "/clientes/1", false},
	{"POST", "/clientes/1", false},
	{"POST", "/clientes/1/eliminar", false},

	{"GET", "/cotizaciones", false},
	{"GET", "/cotizaciones/nueva", false},
	{"POST", "/cotizaciones/nueva", false},
	{"GET", "/cotizaciones/QA0001", false},
	{"POST", "/cotizaciones/QA0001/recalcular", false},
	{"GET", "/cotizaciones/QA0001/productos", false},
	{"POST", "/cotizaciones/QA0001/guardar", false},
	{"POST", "/cotizaciones/QA0001/emitir", false},
	{"POST", "/cotizaciones/QA0001/revisar", false},
	{"GET", "/proyectos", false},
	{"GET", "/proyectos/QA0001", false},
	{"POST", "/proyectos/QA0001/etapa", false},
	{"POST", "/proyectos/QA0001/seguimiento", false},
	{"GET", "/pronostico", false},
	{"POST", "/proyectos/QA0001/comentarios", false},
	{"POST", "/proyectos/QA0001/oc", false},
	{"GET", "/proyectos/QA0001/archivos/1", false},
	{"POST", "/proyectos/QA0001/perder", false},
	{"POST", "/proyectos/QA0001/reabrir", true},
	{"POST", "/cotizaciones/QA0001/comentarios", false},
	{"GET", "/cotizaciones/QA0001/pdf", false},

	{"GET", "/usuarios", true},
	{"POST", "/usuarios/1/rol", true},
	{"POST", "/usuarios/1/deshabilitar", true},
	{"POST", "/usuarios/1/habilitar", true},

	{"GET", "/ajustes", true},
	{"POST", "/ajustes/margenes", true},
	{"POST", "/ajustes/margenes/1", true},
	{"POST", "/ajustes/margenes/1/predeterminado", true},
	{"POST", "/ajustes/margenes/1/retirar", true},
	{"POST", "/ajustes/margenes/1/restaurar", true},
	{"POST", "/ajustes/materiales", true},
	{"POST", "/ajustes/materiales/1", true},

	{"GET", "/unidades", true},
	{"POST", "/unidades", true},

	{"GET", "/familias", true},
	{"POST", "/familias", true},
	{"GET", "/familias/1", true},
	{"POST", "/familias/1", true},

	{"GET", "/ayuda", false},
	{"GET", "/ayuda/cotizaciones", false},
}

func newTestMux(t *testing.T, a *Auth) http.Handler {
	t.Helper()
	staticFS, err := fs.Sub(cladex.StaticFS, "static")
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}
	guide, err := guia.New(os.DirFS("../../docs/guia"))
	if err != nil {
		t.Fatalf("guia.New: %v", err)
	}
	return NewMux(Build{SHA: "testsha"}, staticFS, guide, a.store, false, nil)
}

// TestRoutesRequireAuth walks every application route signed out. Every one of them must
// bounce to /login — a route accidentally registered without RequireAuth would answer
// instead, and that is the failure this catches.
func TestRoutesRequireAuth(t *testing.T) {
	a := newTestAuth(t)
	mux := newTestMux(t, a)

	for _, rt := range appRoutes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(rt.method, rt.path, nil))

			if rec.Code != http.StatusFound {
				t.Fatalf("status = %d, want 302 for a signed-out request", rec.Code)
			}
			if loc := rec.Header().Get("Location"); loc != "/login" {
				t.Fatalf("Location = %q, want /login", loc)
			}
		})
	}
}

// TestRoutesEnforceAdminGate walks every application route as a vendedor. The admin-only
// routes must 403; the rest must not — the table is asserted in both directions so that
// dropping RequireAdmin from a route and wrapping an ordinary route in it are both
// failures.
func TestRoutesEnforceAdminGate(t *testing.T) {
	a := newTestAuth(t)
	mux := newTestMux(t, a)
	vendedorID := createTestUser(t, a, "rodolfo", "vendedor", "hunter2")

	for _, rt := range appRoutes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			req := withAuthCookie(t, a, vendedorID, httptest.NewRequest(rt.method, rt.path, nil))
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			switch {
			case rt.adminOnly && rec.Code != http.StatusForbidden:
				t.Fatalf("status = %d, want 403: an admin-only route answered a vendedor", rec.Code)
			case !rt.adminOnly && rec.Code == http.StatusForbidden:
				t.Fatalf("status = 403: a vendedor route is gated behind RequireAdmin")
			}
			// A signed-in vendedor must never be bounced back to the login page either —
			// that would mean the session did not survive the middleware chain.
			if loc := rec.Header().Get("Location"); loc == "/login" {
				t.Fatalf("redirected to /login with a valid vendedor session")
			}
		})
	}
}

// TestRoutesAdminReachesAdminRoutes is the positive half: an admin gets past the gate.
// Statuses beyond "not 403" are the individual handler tests' business.
func TestRoutesAdminReachesAdminRoutes(t *testing.T) {
	a := newTestAuth(t)
	mux := newTestMux(t, a)
	adminID := createTestUser(t, a, "ana", "admin", "hunter2")

	for _, rt := range appRoutes {
		if !rt.adminOnly {
			continue
		}
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			req := withAuthCookie(t, a, adminID, httptest.NewRequest(rt.method, rt.path, nil))
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code == http.StatusForbidden {
				t.Fatalf("status = 403 for an admin")
			}
		})
	}
}

// TestPublicRoutes pins the routes that are deliberately outside RequireAuth. /healthz in
// particular is hit by the NAS deploy check, which has no session.
func TestPublicRoutes(t *testing.T) {
	a := newTestAuth(t)
	mux := newTestMux(t, a)

	for _, path := range []string{"/healthz", "/login", "/saludo"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 without a session", rec.Code)
			}
		})
	}
}
