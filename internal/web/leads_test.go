package web

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// A vendedor adds a lead, sees everyone's, follows it up, and converts it into a quote;
// the proyecto it becomes links back to it (#82, slice L2).
func TestLeadsFlow(t *testing.T) {
	a := newTestAuth(t)
	h := NewLeads(a.store)
	vendedorID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	otherID := createTestUser(t, a, "rodolfo", "vendedor", "hunter2")
	ctx := context.Background()

	// Errors keep what was typed.
	rec := doForm(t, a, vendedorID, h.Create, "POST", "/leads", nil, url.Values{
		"contact_name": {"Juan Pérez"}, "company": {"Eléctrica Norte"}, "user_id": {strconv.FormatInt(vendedorID, 10)}}, false)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Escribe un teléfono o un correo") ||
		!strings.Contains(rec.Body.String(), `value="Eléctrica Norte"`) {
		t.Fatalf("lead without phone or email = %d", rec.Code)
	}

	rec = doForm(t, a, vendedorID, h.Create, "POST", "/leads", nil, url.Values{
		"contact_name": {"Juan Pérez"}, "company": {"Eléctrica Norte"}, "phone": {"55 1234 5678"},
		"user_id": {strconv.FormatInt(otherID, 10)}, "next_followup": {"2026-12-01"}, "note": {"Pidió precios de CCA."}}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Create = %d: %s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	id := strings.TrimSuffix(strings.TrimPrefix(loc, "/leads/"), "?guardado=1")
	idPath := map[string]string{"id": id}

	// Everyone sees every lead.
	rec = doForm(t, a, vendedorID, h.List, "GET", "/leads", nil, nil, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Juan Pérez") {
		t.Fatalf("List = %d", rec.Code)
	}
	rec = doForm(t, a, vendedorID, h.List, "GET", "/leads?mios=1", nil, nil, false)
	if strings.Contains(rec.Body.String(), "Juan Pérez") {
		t.Error("Ver solo los míos lists Rodolfo's lead")
	}
	rec = doForm(t, a, vendedorID, h.Page, "GET", "/leads/"+id, idPath, nil, false)
	if body := rec.Body.String(); rec.Code != http.StatusOK || !strings.Contains(body, "Pidió precios de CCA.") || !strings.Contains(body, "Convertir en proyecto") {
		t.Fatalf("Page = %d", rec.Code)
	}

	rec = doForm(t, a, vendedorID, h.Seguimiento, "POST", "/leads/"+id+"/seguimiento", idPath, url.Values{"proximo_seguimiento": {"2026-12-15"}}, false)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("follow-up without a comment = %d", rec.Code)
	}
	rec = doForm(t, a, vendedorID, h.Seguimiento, "POST", "/leads/"+id+"/seguimiento", idPath, url.Values{"proximo_seguimiento": {"2099-12-15"}, "note": {"Le mandé la lista."}}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Seguimiento = %d: %s", rec.Code, rec.Body.String())
	}
	rec = doForm(t, a, vendedorID, h.Perdido, "POST", "/leads/"+id+"/perdido", idPath, url.Values{}, false)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("Perdido without a reason = %d", rec.Code)
	}

	qaID, _ := a.store.QuoteTypeIDByPrefix(ctx, "QA")
	rec = doForm(t, a, vendedorID, h.Convertir, "POST", "/leads/"+id+"/convertir", idPath, url.Values{"tipo": {strconv.FormatInt(qaID, 10)}}, false)
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/cotizaciones/QA") {
		t.Fatalf("Convertir = %d → %q", rec.Code, rec.Header().Get("Location"))
	}
	rec = doForm(t, a, vendedorID, h.Convertir, "POST", "/leads/"+id+"/convertir", idPath, url.Values{"tipo": {strconv.FormatInt(qaID, 10)}}, false)
	if rec.Code != http.StatusConflict {
		t.Errorf("converting twice = %d, want 409", rec.Code)
	}
	rec = doForm(t, a, vendedorID, h.Page, "GET", "/leads/"+id, idPath, nil, false)
	if body := rec.Body.String(); !strings.Contains(body, "Convertido en proyecto") || !strings.Contains(body, "el proyecto se abre al emitirla") {
		t.Errorf("converted lead page doesn't say so")
	}
}
