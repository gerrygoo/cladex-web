package web

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gerrygoo/cladex-web/internal/jobs"
	"github.com/gerrygoo/cladex-web/internal/notify"
	"github.com/gerrygoo/cladex-web/internal/store"
)

// TestNotificaciones walks /ajustes/notificaciones. Its strings are the ones
// docs/guia/administracion.md, "Notificaciones", quotes.
func TestNotificaciones(t *testing.T) {
	a := newTestAuth(t)
	ctx := context.Background()
	admin := createTestUser(t, a, "ana", "admin", "hunter2")
	on := notify.Status{Production: true, Channels: []notify.ChannelStatus{
		{Channel: store.ChannelEmail, Enabled: true, Provider: "resend", From: "Cladex <avisos@avisos.cladex.com.mx>"},
		{Channel: store.ChannelWhatsApp, Problem: "todavía no está disponible"},
	}}
	// A runner nothing runs: notifications stay where the test puts them.
	n := NewNotificaciones(a.store, jobs.New(a.store, nil), on)
	mux := http.NewServeMux()
	for pattern, h := range map[string]http.HandlerFunc{
		"GET /ajustes/notificaciones":                  n.List,
		"POST /ajustes/notificaciones/prueba":          n.Prueba,
		"GET /ajustes/notificaciones/{id}":             n.Page,
		"POST /ajustes/notificaciones/{id}/reintentar": n.Reintentar,
		"POST /ajustes/notificaciones/{id}/cancelar":   n.Cancelar,
	} {
		mux.Handle(pattern, a.RequireAuth(a.RequireAdmin(h)))
	}
	get := func(path string) string {
		t.Helper()
		rec := serve(t, a, mux, admin, "GET", path, nil)
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

	mustContain(get("/ajustes/notificaciones"), "Notificaciones", "Encendido, por resend, desde Cladex &lt;avisos@avisos.cladex.com.mx&gt;",
		"Apagado: todavía no está disponible", "No hay notificaciones.", "Enviarme una prueba", "Estado", "Canal", "Tipo")

	// No email of their own: nothing to send the test to.
	rec := serve(t, a, mux, admin, "POST", "/ajustes/notificaciones/prueba", nil)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Escribe tu correo en Mi cuenta → Avisos para enviarte una prueba.") {
		t.Errorf("prueba without an email: %d", rec.Code)
	}
	if err := a.store.SetUserContact(ctx, admin, store.UserContact{Email: "ana@cladex.com.mx"}); err != nil {
		t.Fatal(err)
	}
	rec = serve(t, a, mux, admin, "POST", "/ajustes/notificaciones/prueba", nil)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/ajustes/notificaciones?prueba=1" {
		t.Fatalf("prueba: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	mustContain(get("/ajustes/notificaciones?prueba=1"), "Prueba en la cola.", "ana (ana@cladex.com.mx)", "Prueba de avisos de Cladex", "Por enviar")

	// Filters.
	if body := get("/ajustes/notificaciones?estado=fallida"); !strings.Contains(body, "No hay notificaciones.") {
		t.Error("estado=fallida should match nothing yet")
	}
	if body := get("/ajustes/notificaciones?canal=email&tipo=prueba"); strings.Contains(body, "No hay notificaciones.") {
		t.Error("canal=email&tipo=prueba should match the test")
	}

	// Its page: the content, the history, and Cancelar while it's por enviar.
	mustContain(get("/ajustes/notificaciones/1"), "Notificación 1", "Si lo recibiste, los avisos por correo funcionan.", "En la cola", "Cancelar")
	if rec := serve(t, a, mux, admin, "POST", "/ajustes/notificaciones/1/reintentar", nil); rec.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rec.Body.String(), "Solo se puede reintentar una notificación fallida.") {
		t.Errorf("retry por enviar: %d", rec.Code)
	}

	// Failed: it shows the error and is retried with a new job.
	job, _ := a.store.ClaimJob(ctx, []string{store.NotificationJobKind}, time.Now(), time.Minute)
	if job == nil {
		t.Fatal("the test wasn't queued")
	}
	if err := a.store.StartSending(ctx, 1, "resend", "", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := a.store.MarkNotificationError(ctx, 1, "resend 403 validation_error: domain is not verified", true, time.Now()); err != nil {
		t.Fatal(err)
	}
	mustContain(get("/ajustes/notificaciones?estado=fallida"), "Fallida", "domain is not verified")
	rec = serve(t, a, mux, admin, "POST", "/ajustes/notificaciones/1/reintentar", nil)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/ajustes/notificaciones/1?reintentada=1" {
		t.Fatalf("retry: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	mustContain(get("/ajustes/notificaciones/1?reintentada=1"), "La notificación se volvió a poner en la cola.", "Reintentada a mano por ana", "Fallida: resend 403", "Por enviar")
	if notif, _ := a.store.NotificationByID(ctx, 1); notif.JobID == job.ID {
		t.Error("the retry should have its own job")
	}

	// Cancelled by hand.
	rec = serve(t, a, mux, admin, "POST", "/ajustes/notificaciones/1/cancelar", nil)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("cancel: %d", rec.Code)
	}
	mustContain(get("/ajustes/notificaciones/1?cancelada=1"), "Notificación cancelada.", "Cancelada por ana.")
	if rec := serve(t, a, mux, admin, "POST", "/ajustes/notificaciones/1/cancelar", nil); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("cancel twice: %d", rec.Code)
	}
	if rec := serve(t, a, mux, admin, "GET", "/ajustes/notificaciones/99", nil); rec.Code != http.StatusNotFound {
		t.Errorf("missing: %d", rec.Code)
	}

	// With email off, the test says why instead of queueing.
	n.status = notify.Status{Channels: []notify.ChannelStatus{{Channel: store.ChannelEmail, Problem: "falta la llave de Resend"}}}
	rec = serve(t, a, mux, admin, "POST", "/ajustes/notificaciones/prueba", nil)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "El correo está apagado: falta la llave de Resend.") {
		t.Errorf("prueba with email off: %d", rec.Code)
	}
}
