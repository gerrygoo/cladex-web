package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gerrygoo/cladex-web/internal/jobs"
	"github.com/gerrygoo/cladex-web/internal/store"
)

// TestTrabajos walks the /ajustes/trabajos page. Its strings are the ones
// docs/guia/administracion.md, "Trabajos en segundo plano", quotes.
func TestTrabajos(t *testing.T) {
	a := newTestAuth(t)
	adminID := createTestUser(t, a, "ana", "admin", "hunter2")
	// A runner nothing runs: jobs stay where the test puts them.
	tr := NewTrabajos(a.store, jobs.New(a.store, nil))
	page := a.RequireAuth(a.RequireAdmin(http.HandlerFunc(tr.Page)))
	ctx := context.Background()

	do := func(h http.HandlerFunc, method, path string, pattern string) *httptest.ResponseRecorder {
		t.Helper()
		mux := http.NewServeMux()
		mux.Handle(pattern, a.RequireAuth(a.RequireAdmin(h)))
		req := withAuthCookie(t, a, adminID, httptest.NewRequest(method, path, nil))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	get := func(path string) string {
		t.Helper()
		req := withAuthCookie(t, a, adminID, httptest.NewRequest("GET", path, nil))
		rec := httptest.NewRecorder()
		page.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d", path, rec.Code)
		}
		return rec.Body.String()
	}
	mustContain := func(body string, wants ...string) {
		t.Helper()
		for _, w := range wants {
			if !strings.Contains(body, w) {
				t.Errorf("page missing %q", w)
			}
		}
	}

	mustContain(get("/ajustes/trabajos"), "Trabajos en segundo plano", "En cola y fallidos",
		"Terminados recientemente", "No hay trabajos pendientes ni fallidos.", "Encolar trabajo de prueba")

	// Encolar trabajo de prueba queues one.
	rec := do(tr.Prueba, "POST", "/ajustes/trabajos/prueba", "POST /ajustes/trabajos/prueba")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/ajustes/trabajos?prueba=1" {
		t.Fatalf("prueba: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	mustContain(get("/ajustes/trabajos?prueba=1"), "Trabajo de prueba en la cola.", "Prueba", "Pendiente", "Reintentar ahora")

	// Once it runs, it moves to Terminados recientemente with its result.
	now := time.Now()
	j, err := a.store.ClaimJob(ctx, []string{jobs.KindPrueba}, now, time.Minute)
	if err != nil || j == nil {
		t.Fatalf("claim: %v %v", j, err)
	}
	result, _ := jobs.Prueba(ctx, *j)
	if err := a.store.CompleteJob(ctx, j.ID, result, now); err != nil {
		t.Fatal(err)
	}
	mustContain(get("/ajustes/trabajos"), "No hay trabajos pendientes ni fallidos.", "Hecho", "El trabajo de prueba se ejecutó.")

	// A finished job can't be retried.
	done := strconv.FormatInt(j.ID, 10)
	rec = do(tr.Reintentar, "POST", "/ajustes/trabajos/"+done+"/reintentar", "POST /ajustes/trabajos/{id}/reintentar")
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Ese trabajo ya no se puede reintentar") {
		t.Errorf("retry a finished job: %d", rec.Code)
	}

	// A failed one shows its error and goes back in the queue.
	id, err := a.store.EnqueueJob(ctx, store.NewJob{Kind: "aviso"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.ClaimJob(ctx, []string{"aviso"}, now, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := a.store.FailJob(ctx, id, "dirección inválida", true, now, now); err != nil {
		t.Fatal(err)
	}
	mustContain(get("/ajustes/trabajos"), "Fallido", "dirección inválida", "1 de 5")
	failed := strconv.FormatInt(id, 10)
	rec = do(tr.Reintentar, "POST", "/ajustes/trabajos/"+failed+"/reintentar", "POST /ajustes/trabajos/{id}/reintentar")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/ajustes/trabajos?reintentado=1" {
		t.Fatalf("retry: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	mustContain(get("/ajustes/trabajos?reintentado=1"), "El trabajo se volvió a poner en la cola.", "0 de 5")
}
