package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// issueTestQuote creates a QA draft with one catalog line and issues it through the
// handler, which opens its proyecto. It returns the quote's folio, also the proyecto's.
func issueTestQuote(t *testing.T, a *Auth, q *Quotes, userID, customerID, productID int64) string {
	t.Helper()
	quote, err := a.store.CreateDraftQuote(context.Background(), customerID, userID, "QA")
	if err != nil {
		t.Fatalf("CreateDraftQuote: %v", err)
	}
	form := url.Values{
		"line_keys":            {"0"},
		"lines[0][kind]":       {"product"},
		"lines[0][product_id]": {strconv.FormatInt(productID, 10)},
		"lines[0][qty]":        {"1"},
	}
	pv := map[string]string{"folio": quote.Folio}
	if rec := doForm(t, a, userID, q.Emitir, "POST", "/cotizaciones/"+quote.Folio+"/emitir", pv, form, false); rec.Code != http.StatusSeeOther {
		t.Fatalf("Emitir status = %d; body = %s", rec.Code, rec.Body.String())
	}
	return quote.Folio
}

func TestProjectsFollowUp(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	p := NewProjects(a.store)
	ana := createTestUser(t, a, "ana", "vendedor", "hunter2")
	beto := createTestUser(t, a, "beto", "admin", "hunter2")
	customerID, _, costProductID := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, ana)

	// A draft has no proyecto yet.
	draft, err := a.store.CreateDraftQuote(context.Background(), customerID, ana, "QA")
	if err != nil {
		t.Fatalf("CreateDraftQuote: %v", err)
	}
	if rec := doForm(t, a, ana, p.Page, "GET", "/proyectos/"+draft.Folio, map[string]string{"folio": draft.Folio}, nil, false); rec.Code != http.StatusNotFound {
		t.Fatalf("Page(draft's folio) status = %d, want 404", rec.Code)
	}

	folio := issueTestQuote(t, a, q, ana, customerID, costProductID)
	pv := map[string]string{"folio": folio}
	base := "/proyectos/" + folio
	move := func(userID int64, to, note string) *httptest.ResponseRecorder {
		return doForm(t, a, userID, p.Etapa, "POST", base+"/etapa", pv, url.Values{"to": {to}, "note": {note}}, false)
	}
	prob := func(userID int64, value, note string) *httptest.ResponseRecorder {
		return doForm(t, a, userID, p.Probabilidad, "POST", base+"/probabilidad", pv,
			url.Values{"probabilidad": {value}, "note": {note}}, false)
	}
	page := func(userID int64) string {
		return doForm(t, a, userID, p.Page, "GET", base, pv, nil, false).Body.String()
	}
	quotePage := func() string {
		return doForm(t, a, ana, q.Builder, "GET", "/cotizaciones/"+folio, pv, nil, false).Body.String()
	}

	// Issuing opened the proyecto as a prospecto at the lowest probability.
	body := page(ana)
	for _, want := range []string{"Proyecto " + folio, "Prospecto", "Inicial (10%)", "Guardar probabilidad",
		"Pasar a O.C. recibida", "Cotización vigente", `href="/cotizaciones/` + folio + `"`} {
		if !strings.Contains(body, want) {
			t.Errorf("prospecto page missing %q", want)
		}
	}
	if strings.Contains(body, "Regresar a") {
		t.Error("a vendedor is offered going back a stage")
	}
	if strings.Contains(body, "Relevante para pronóstico") {
		t.Error("a prospecto at Inicial is shown as relevante para pronóstico")
	}
	// The quote page points at the proyecto and carries none of its controls.
	body = quotePage()
	for _, want := range []string{`href="/proyectos/` + folio + `"`, ">Revisar<"} {
		if !strings.Contains(body, want) {
			t.Errorf("quote page missing %q", want)
		}
	}
	for _, gone := range []string{"Pasar a", "Guardar probabilidad"} {
		if strings.Contains(body, gone) {
			t.Errorf("quote page still shows %q", gone)
		}
	}

	// The probability takes only the fixed steps, and from Alta the proyecto counts for
	// the forecast.
	for _, bad := range []string{"", "40", "100", "alta"} {
		if rec := prob(ana, bad, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("Probabilidad(%q) status = %d, want 400", bad, rec.Code)
		}
	}
	if rec := prob(ana, "75", strings.Repeat("x", maxCommentLen+1)); rec.Code != http.StatusBadRequest {
		t.Errorf("Probabilidad(note too long) status = %d, want 400", rec.Code)
	}
	if rec := prob(ana, "75", "Le interesa"); rec.Code != http.StatusSeeOther {
		t.Fatalf("Probabilidad(75) status = %d; body = %s", rec.Code, rec.Body.String())
	}
	body = page(ana)
	for _, want := range []string{"Alta (75%)", "Relevante para pronóstico", "Probabilidad: Inicial → Alta.", "Le interesa"} {
		if !strings.Contains(body, want) {
			t.Errorf("prospecto page at Alta missing %q", want)
		}
	}

	if rec := move(ana, "en_entrega", ""); rec.Code != http.StatusConflict {
		t.Errorf("Etapa(skip a stage) status = %d, want 409", rec.Code)
	}
	if rec := move(ana, "oc_recibida", strings.Repeat("x", maxCommentLen+1)); rec.Code != http.StatusBadRequest {
		t.Errorf("Etapa(note too long) status = %d, want 400", rec.Code)
	}
	if rec := move(ana, "oc_recibida", "OC 4411"); rec.Code != http.StatusSeeOther {
		t.Fatalf("Etapa(→ oc_recibida) status = %d; body = %s", rec.Code, rec.Body.String())
	}

	// With the purchase order in: no probability, and the quote can't be revised.
	body = page(ana)
	for _, want := range []string{"O.C. recibida", "Pasar a En entrega", "Pasó a O.C. recibida.", "OC 4411"} {
		if !strings.Contains(body, want) {
			t.Errorf("oc_recibida page missing %q", want)
		}
	}
	for _, gone := range []string{"Guardar probabilidad", "Relevante para pronóstico"} {
		if strings.Contains(body, gone) {
			t.Errorf("oc_recibida page still shows %q", gone)
		}
	}
	if rec := prob(ana, "90", ""); rec.Code != http.StatusConflict {
		t.Errorf("Probabilidad(oc_recibida) status = %d, want 409", rec.Code)
	}
	if body := quotePage(); strings.Contains(body, ">Revisar<") || !strings.Contains(body, "ya no se puede revisar") {
		t.Error("the quote of a proyecto with its O.C. in still offers Revisar, or doesn't say why not")
	}
	if rec := doForm(t, a, ana, q.Revisar, "POST", "/cotizaciones/"+folio+"/revisar", pv, nil, false); rec.Code != http.StatusConflict {
		t.Errorf("Revisar(oc_recibida) status = %d, want 409", rec.Code)
	}

	// Going back is admin-only.
	if rec := move(ana, "prospecto", ""); rec.Code != http.StatusForbidden {
		t.Errorf("Etapa(back as vendedor) status = %d, want 403", rec.Code)
	}
	if !strings.Contains(page(beto), "Regresar a Prospecto") {
		t.Error("admin is not offered going back")
	}
	if rec := move(beto, "prospecto", "Se cayó la junta"); rec.Code != http.StatusSeeOther {
		t.Fatalf("Etapa(back as admin) status = %d", rec.Code)
	}
	// Back as a prospecto it has the probability it left with.
	if !strings.Contains(page(ana), "Alta (75%)") {
		t.Error("a proyecto sent back to prospecto lost its probability")
	}

	// A comment from the proyecto page lands in its history and on the current quote.
	comment := func(text string) *httptest.ResponseRecorder {
		return doForm(t, a, ana, p.Comentar, "POST", base+"/comentarios", pv, url.Values{"comment": {text}}, false)
	}
	if rec := comment("  "); rec.Code != http.StatusBadRequest {
		t.Errorf("Comentar(empty) status = %d, want 400", rec.Code)
	}
	if rec := comment("Junta el jueves"); rec.Code != http.StatusSeeOther {
		t.Fatalf("Comentar status = %d", rec.Code)
	}
	if !strings.Contains(page(ana), "Junta el jueves") || !strings.Contains(quotePage(), "Junta el jueves") {
		t.Error("a comment made on the proyecto is missing from its history or from the quote")
	}

	// Run it to the end: the last stage has no forward button.
	for _, to := range []string{"oc_recibida", "en_entrega", "cerrado"} {
		if rec := move(ana, to, ""); rec.Code != http.StatusSeeOther {
			t.Fatalf("Etapa(→ %s) status = %d; body = %s", to, rec.Code, rec.Body.String())
		}
	}
	body = page(ana)
	if !strings.Contains(body, "Este proyecto está cerrado") {
		t.Error("closed proyecto page doesn't say so")
	}
	if strings.Contains(body, "Pasar a") {
		t.Error("closed proyecto still offers a next stage")
	}
}

// While a revision is in draft the proyecto stays put: it shows the draft as its
// current quote and can't receive the purchase order until the revision is issued.
func TestProjectsWithARevisionInDraft(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	p := NewProjects(a.store)
	ana := createTestUser(t, a, "ana", "vendedor", "hunter2")
	customerID, _, costProductID := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, ana)
	ctx := context.Background()

	folio := issueTestQuote(t, a, q, ana, customerID, costProductID)
	original, _ := a.store.QuoteByFolio(ctx, folio)
	rev, err := a.store.CreateRevision(ctx, original.ID, ana)
	if err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	pv := map[string]string{"folio": folio}

	body := doForm(t, a, ana, p.Page, "GET", "/proyectos/"+folio, pv, nil, false).Body.String()
	for _, want := range []string{rev.Folio, "revisión en borrador", "primero emite la revisión", "Revisada", "Guardar probabilidad"} {
		if !strings.Contains(body, want) {
			t.Errorf("proyecto page missing %q", want)
		}
	}
	if strings.Contains(body, "Pasar a O.C. recibida") {
		t.Error("a proyecto whose current quote is a draft offers O.C. recibida")
	}
	rec := doForm(t, a, ana, p.Etapa, "POST", "/proyectos/"+folio+"/etapa", pv, url.Values{"to": {"oc_recibida"}}, false)
	if rec.Code != http.StatusConflict {
		t.Errorf("Etapa with a draft revision status = %d, want 409", rec.Code)
	}
	// Both quotes point at the same proyecto.
	for _, f := range []string{folio, rev.Folio} {
		page := doForm(t, a, ana, q.Builder, "GET", "/cotizaciones/"+f, map[string]string{"folio": f}, nil, false).Body.String()
		if !strings.Contains(page, `href="/proyectos/`+folio+`"`) {
			t.Errorf("quote %s doesn't link to its proyecto", f)
		}
	}
}

func TestProjectsList(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	p := NewProjects(a.store)
	ana := createTestUser(t, a, "ana", "vendedor", "hunter2")
	customerID, _, costProductID := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, ana)

	list := func(query string, hx bool) string {
		return doForm(t, a, ana, p.List, "GET", "/proyectos?"+query, nil, nil, hx).Body.String()
	}
	if body := list("", false); !strings.Contains(body, "No se encontraron proyectos") {
		t.Error("empty list doesn't say so")
	}

	first := issueTestQuote(t, a, q, ana, customerID, costProductID)
	second := issueTestQuote(t, a, q, ana, customerID, costProductID)
	project, _ := a.store.ProjectByFolio(context.Background(), second)
	if err := a.store.SetProjectProbability(context.Background(), project.ID, ana, 90, ""); err != nil {
		t.Fatalf("SetProjectProbability: %v", err)
	}
	if err := a.store.MoveProject(context.Background(), project.ID, ana, "prospecto", "oc_recibida", ""); err != nil {
		t.Fatalf("MoveProject: %v", err)
	}

	link := func(folio string) string { return `href="/proyectos/` + folio + `"` }
	body := list("", false)
	for _, want := range []string{link(first), link(second), "Inicial (10%)", "O.C. recibida", "<h1>Proyectos</h1>"} {
		if !strings.Contains(body, want) {
			t.Errorf("list missing %q", want)
		}
	}
	// Past prospecto, the probability isn't shown.
	if strings.Contains(body, "Inminente (90%)") {
		t.Error("the list shows a probability for a proyecto past prospecto")
	}
	if body := list("f.etapa=oc_recibida", false); strings.Contains(body, link(first)) || !strings.Contains(body, link(second)) {
		t.Error("the stage filter doesn't narrow the list to O.C. recibida")
	}
	if body := list("f.probabilidad=10", false); !strings.Contains(body, link(first)) || strings.Contains(body, link(second)) {
		t.Error("the probability filter doesn't narrow the list to Inicial prospectos")
	}
	// The htmx search swaps only the rows.
	if body := list("q="+second, true); strings.Contains(body, "<h1>") || !strings.Contains(body, link(second)) || strings.Contains(body, link(first)) {
		t.Errorf("htmx search body = %s", body)
	}
}
