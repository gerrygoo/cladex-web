package web

import (
	"context"
	"database/sql"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gerrygoo/cladex-web"
	"github.com/gerrygoo/cladex-web/internal/store"
)

// Issue #52: an admin can delete proyectos and first drafts (to clear out test data)
// and hand them to another vendedor. Pins docs/guia README.md "Quién puede hacer qué".
func TestAdminDeleteAndReassign(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "admin.db")
	st, err := store.Open(context.Background(), dsn, cladex.MigrationsFS)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	a := NewAuth(st, false)
	mux := newTestMux(t, a)
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer raw.Close()
	ctx := context.Background()
	vendedor := createNamedUser(t, a, "emilio", "Emilio Flores", "vendedor")
	gerardo := createNamedUser(t, a, "gerardo", "Gerardo", "vendedor")
	admin := createNamedUser(t, a, "ana", "Ana", "admin")
	customerID, err := a.store.CreateCustomer(ctx, store.Customer{Name: "Cliente"})
	if err != nil {
		t.Fatal(err)
	}
	issue := func() *store.Quote {
		q, err := a.store.CreateDraftQuote(ctx, customerID, vendedor, "QA")
		if err != nil {
			t.Fatal(err)
		}
		if err := a.store.IssueQuote(ctx, q.ID, store.Issue{IssuedAt: "2026-08-02T15:04:05.000Z", TermsSnapshot: "t",
			CustomerNameSnapshot: "Cliente", VendedorSnapshot: "Emilio Flores", PDFSHA256: "sha" + q.Folio}); err != nil {
			t.Fatal(err)
		}
		q, _ = a.store.QuoteByID(ctx, q.ID)
		return q
	}
	project := issue()
	draft, err := a.store.CreateDraftQuote(ctx, customerID, vendedor, "QA")
	if err != nil {
		t.Fatal(err)
	}
	reassign := url.Values{"user_id": {strconv.FormatInt(gerardo, 10)}}

	// A vendedor can do none of it, and sees no Administración section.
	for _, path := range []string{
		"/proyectos/" + project.Folio + "/reasignar", "/proyectos/" + project.Folio + "/eliminar",
		"/cotizaciones/" + draft.Folio + "/reasignar", "/cotizaciones/" + draft.Folio + "/eliminar",
	} {
		if rec := muxDo(t, a, mux, vendedor, "POST", path, reassign); rec.Code != http.StatusForbidden {
			t.Errorf("vendedor POST %s = %d, want 403", path, rec.Code)
		}
	}
	for _, path := range []string{"/proyectos/" + project.Folio, "/cotizaciones/" + draft.Folio} {
		if body := muxDo(t, a, mux, vendedor, "GET", path, nil).Body.String(); strings.Contains(body, "Administración") {
			t.Errorf("vendedor sees the Administración section on %s", path)
		}
		if body := muxDo(t, a, mux, admin, "GET", path, nil).Body.String(); !strings.Contains(body, "Administración") || !strings.Contains(body, "Gerardo") {
			t.Errorf("admin doesn't see the Administración section with the vendedores on %s", path)
		}
	}

	// An admin reassigns both, and the new owner is the one on the proyecto.
	if rec := muxDo(t, a, mux, admin, "POST", "/proyectos/"+project.Folio+"/reasignar", reassign); rec.Code != http.StatusSeeOther {
		t.Fatalf("reasignar proyecto = %d", rec.Code)
	}
	if rec := muxDo(t, a, mux, admin, "POST", "/cotizaciones/"+draft.Folio+"/reasignar", reassign); rec.Code != http.StatusSeeOther {
		t.Fatalf("reasignar borrador = %d", rec.Code)
	}
	if p, _ := a.store.ProjectByFolio(ctx, project.Folio); p == nil || p.UserID != gerardo {
		t.Errorf("proyecto owner = %+v, want Gerardo", p)
	}
	if d, _ := a.store.QuoteByFolio(ctx, draft.Folio); d == nil || d.UserID != gerardo {
		t.Errorf("draft owner = %+v, want Gerardo", d)
	}
	// The issued quote keeps who issued it, and its page and PDF name the new vendedor (issue #59).
	reassigned, _ := a.store.QuoteByID(ctx, project.ID)
	body := muxDo(t, a, mux, admin, "GET", "/cotizaciones/"+project.Folio, nil).Body.String()
	for _, want := range []string{"<dt>Emitió</dt><dd>Emilio Flores</dd>", "<dt>Vendedor</dt><dd>Gerardo</dd>"} {
		if !strings.Contains(body, want) {
			t.Errorf("reassigned quote page missing %q", want)
		}
	}
	if doc, err := issuedQuoteDocument(*reassigned, nil); err != nil || doc.Vendedor != "Gerardo" {
		t.Errorf("reassigned quote PDF vendedor = %q (%v), want Gerardo", doc.Vendedor, err)
	}
	if rec := muxDo(t, a, mux, admin, "POST", "/proyectos/"+project.Folio+"/reasignar", url.Values{"user_id": {"999"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("reasignar to an unknown user = %d, want 400", rec.Code)
	}

	// A quote that belongs to a proyecto is deleted with it, not on its own.
	if rec := muxDo(t, a, mux, admin, "POST", "/cotizaciones/"+project.Folio+"/eliminar", url.Values{}); rec.Code != http.StatusConflict {
		t.Errorf("eliminar an issued quote = %d, want 409", rec.Code)
	}

	// A stamped invoice blocks deleting the proyecto, and the page says why.
	if _, err := raw.Exec(`INSERT INTO comprobantes (tipo, project_id, quote_id, status, created_by) VALUES ('factura', ?, ?, 'timbrada', ?)`,
		*project.ProjectID, project.ID, admin); err != nil {
		t.Fatal(err)
	}
	if rec := muxDo(t, a, mux, admin, "POST", "/proyectos/"+project.Folio+"/eliminar", url.Values{}); rec.Code != http.StatusConflict {
		t.Errorf("eliminar with a stamped invoice = %d, want 409", rec.Code)
	}
	if body := muxDo(t, a, mux, admin, "GET", "/proyectos/"+project.Folio, nil).Body.String(); !strings.Contains(body, "factura timbrada") || strings.Contains(body, "Eliminar proyecto") {
		t.Errorf("a proyecto with a stamped invoice should explain why it can't be deleted instead of offering the button")
	}
	if _, err := raw.Exec(`DELETE FROM comprobantes`); err != nil {
		t.Fatal(err)
	}

	// Without one, both are deleted.
	if rec := muxDo(t, a, mux, admin, "POST", "/proyectos/"+project.Folio+"/eliminar", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Errorf("eliminar proyecto = %d, want 303", rec.Code)
	}
	if rec := muxDo(t, a, mux, admin, "POST", "/cotizaciones/"+draft.Folio+"/eliminar", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Errorf("eliminar borrador = %d, want 303", rec.Code)
	}
	if p, _ := a.store.ProjectByFolio(ctx, project.Folio); p != nil {
		t.Errorf("proyecto still exists")
	}
	if d, _ := a.store.QuoteByFolio(ctx, draft.Folio); d != nil {
		t.Errorf("draft still exists")
	}
}
