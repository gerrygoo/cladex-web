package web

import (
	"bytes"
	"context"
	"database/sql"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gerrygoo/cladex-web"
	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

// testOCForm is a complete purchase order form, without a file. Receiving the order on a
// prospecto needs a file too: see receiveOC.
func testOCForm(note string) url.Values {
	return url.Values{"oc_numero": {"4411"}, "oc_fecha": {"2026-10-07"}, "forma_pago": {"PUE"}, "note": {note}}
}

// receiveOC posts testOCForm with a PDF, as a prospecto needs to receive its order.
func receiveOC(t *testing.T, a *Auth, p *Projects, userID int64, folio, note string) *httptest.ResponseRecorder {
	t.Helper()
	fields := map[string]string{}
	for k, v := range testOCForm(note) {
		fields[k] = v[0]
	}
	return postOC(t, a, p, userID, folio, fields, "oc.pdf", []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n"))
}

// testInvoiceForm is a complete factura form. With testOCForm, which is P.U.E., posting
// it also leaves the proyecto pagado.
func testInvoiceForm() url.Values {
	return url.Values{"folio": {"F-1001"}, "fecha": {"2026-10-08"}}
}

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

// futureDay is a day safely after today in any time zone, the earliest kind of next
// follow-up the form takes.
func futureDay() string { return time.Now().UTC().AddDate(0, 0, 3).Format("2006-01-02") }

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
		return doForm(t, a, userID, p.Seguimiento, "POST", base+"/seguimiento", pv,
			url.Values{"probabilidad": {value}, "note": {note}, "proximo_seguimiento": {futureDay()}}, false)
	}
	page := func(userID int64) string {
		return doForm(t, a, userID, p.Page, "GET", base, pv, nil, false).Body.String()
	}
	quotePage := func() string {
		return doForm(t, a, ana, q.Builder, "GET", "/cotizaciones/"+folio, pv, nil, false).Body.String()
	}

	// Issuing opened the proyecto as a prospecto at the lowest probability.
	body := page(ana)
	for _, want := range []string{"Proyecto " + folio, "Prospecto", "Inicial (10%)", "data-followup",
		"Seguimiento y comentarios", "Orden colocada", "Perdida", "Cotización vigente", `href="/cotizaciones/` + folio + `"`} {
		if !strings.Contains(body, want) {
			t.Errorf("prospecto page missing %q", want)
		}
	}
	// One box to write in (issue #22): the follow-up's Comentario, no separate Comentar.
	for _, gone := range []string{"Agregar un comentario", ">Comentar<", `name="comment"`} {
		if strings.Contains(body, gone) {
			t.Errorf("prospecto page still has %q", gone)
		}
	}
	// The Motivo field starts hidden; it shows only once Perdida is picked.
	if !strings.Contains(body, "<label data-lost-only hidden>") {
		t.Error("Motivo de la pérdida is not hidden on load")
	}
	// O.C. esperada only shows at Inminente 90%, but its date is still sent.
	if !strings.Contains(body, "<label data-imminent-only hidden>") || !strings.Contains(body, `name="oc_esperada"`) {
		t.Error("O.C. esperada is not hidden below Inminente")
	}
	if strings.Contains(body, "Regresar a") {
		t.Error("a vendedor is offered going back a stage")
	}
	// The purchase order isn't asked for until Inminente, and "Orden colocada" can't be
	// picked; the old block to lose a proyecto is gone, "Perdida" is a radio now.
	for _, gone := range []string{"Recibir la orden de compra", "Pasar a O.C. recibida", "Perder el proyecto", "Comentario (opcional)"} {
		if strings.Contains(body, gone) {
			t.Errorf("prospecto at Inicial shows %q", gone)
		}
	}
	if !strings.Contains(body, `name="probabilidad_oc" value="100" disabled`) {
		t.Error("Orden colocada 100% is not a disabled radio")
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
	for _, gone := range []string{"Pasar a", "data-followup"} {
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
	// Every save needs a comment and a next follow-up day after today, stage change or not.
	followUp := func(form url.Values) int {
		form.Set("probabilidad", "75")
		return doForm(t, a, ana, p.Seguimiento, "POST", base+"/seguimiento", pv, form, false).Code
	}
	if code := followUp(url.Values{"proximo_seguimiento": {futureDay()}, "note": {"  "}}); code != http.StatusBadRequest {
		t.Errorf("Seguimiento(no comment) status = %d, want 400", code)
	}
	for name, day := range map[string]string{"none": "", "today": time.Now().UTC().Format("2006-01-02"), "past": "2020-01-09", "malformed": "9/1/2030"} {
		if code := followUp(url.Values{"proximo_seguimiento": {day}, "note": {"Le interesa"}}); code != http.StatusBadRequest {
			t.Errorf("Seguimiento(next follow-up %s) status = %d, want 400", name, code)
		}
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

	// At Alta there is still no purchase order section; at Inminente it appears, without a
	// comment field.
	if strings.Contains(body, "Recibir la orden de compra") {
		t.Error("a prospecto at Alta offers receiving the purchase order")
	}
	if rec := prob(ana, "90", "Ya casi cierra"); rec.Code != http.StatusSeeOther {
		t.Fatalf("Probabilidad(90) status = %d; body = %s", rec.Code, rec.Body.String())
	}
	body = page(ana)
	for _, want := range []string{"Inminente (90%)", "Recibir la orden de compra", "Pasar a O.C. recibida"} {
		if !strings.Contains(body, want) {
			t.Errorf("prospecto page at Inminente missing %q", want)
		}
	}
	if !strings.Contains(body, "<label data-imminent-only>") {
		t.Error("O.C. esperada is hidden at Inminente")
	}
	if rest := body[strings.Index(body, "Recibir la orden de compra"):]; strings.Contains(rest, "Comentario") {
		t.Error("the receive-the-purchase-order section still has a comment field")
	}

	if rec := move(ana, "en_entrega", ""); rec.Code != http.StatusConflict {
		t.Errorf("Etapa(skip a stage) status = %d, want 409", rec.Code)
	}
	if rec := move(ana, "en_entrega", strings.Repeat("x", maxCommentLen+1)); rec.Code != http.StatusBadRequest {
		t.Errorf("Etapa(note too long) status = %d, want 400", rec.Code)
	}
	// The stage button doesn't take a prospecto to O.C. recibida: the purchase order does.
	if rec := move(ana, "oc_recibida", ""); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "primero captura la orden de compra") {
		t.Errorf("Etapa(→ oc_recibida) = %d %q, want 409 asking for the purchase order", rec.Code, rec.Body.String())
	}
	receive := func(note string) *httptest.ResponseRecorder {
		return receiveOC(t, a, p, ana, folio, note)
	}
	if rec := receive("OC 4411"); rec.Code != http.StatusSeeOther {
		t.Fatalf("OC status = %d; body = %s", rec.Code, rec.Body.String())
	}

	// With the purchase order in: no probability, and the quote can't be revised.
	body = page(ana)
	for _, want := range []string{"O.C. recibida", "Registrar pago y pasar a Facturado", "Pasó a O.C. recibida.", "OC 4411",
		"Orden colocada", "Marcar como perdida"} {
		if !strings.Contains(body, want) {
			t.Errorf("oc_recibida page missing %q", want)
		}
	}
	for _, gone := range []string{"Guardar seguimiento", "Relevante para pronóstico"} {
		if strings.Contains(body, gone) {
			t.Errorf("oc_recibida page still shows %q", gone)
		}
	}
	if rec := prob(ana, "90", "Otra vez"); rec.Code != http.StatusConflict {
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
	if !strings.Contains(page(ana), "Inminente (90%)") {
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
	if rec := receive(""); rec.Code != http.StatusSeeOther {
		t.Fatalf("OC again status = %d; body = %s", rec.Code, rec.Body.String())
	}
	if rec := doForm(t, a, ana, p.Factura, "POST", base+"/factura", pv, testInvoiceForm(), false); rec.Code != http.StatusSeeOther {
		t.Fatalf("Factura status = %d; body = %s", rec.Code, rec.Body.String())
	}
	for _, to := range []string{"en_entrega", "cerrado"} {
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
	project, err := a.store.ProjectByFolio(ctx, folio)
	if err != nil || project == nil {
		t.Fatalf("ProjectByFolio: %v, %v", project, err)
	}
	// The purchase order is only offered at Inminente.
	if err := a.store.SetProjectProbability(ctx, project.ID, ana, 90, ""); err != nil {
		t.Fatalf("SetProjectProbability: %v", err)
	}

	body := doForm(t, a, ana, p.Page, "GET", "/proyectos/"+folio, pv, nil, false).Body.String()
	for _, want := range []string{rev.Folio, "revisión en borrador", "primero emite la revisión", "Revisada", "data-followup"} {
		if !strings.Contains(body, want) {
			t.Errorf("proyecto page missing %q", want)
		}
	}
	if strings.Contains(body, "Pasar a O.C. recibida") {
		t.Error("a proyecto whose current quote is a draft offers O.C. recibida")
	}
	rec := receiveOC(t, a, p, ana, folio, "")
	if rec.Code != http.StatusConflict {
		t.Errorf("OC with a draft revision status = %d, want 409", rec.Code)
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
	if err := a.store.ReceiveOC(context.Background(), project.ID, ana, store.OC{Number: "4411", Date: "2026-10-07", PaymentMethod: "PUE"}, nil, ""); err != nil {
		t.Fatalf("ReceiveOC: %v", err)
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

func TestProjectsLoseAndReopen(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	p := NewProjects(a.store)
	ana := createTestUser(t, a, "ana", "vendedor", "hunter2")
	customerID, _, costProductID := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, ana)

	folio := issueTestQuote(t, a, q, ana, customerID, costProductID)
	pv := map[string]string{"folio": folio}
	base := "/proyectos/" + folio
	page := func(userID int64) string {
		return doForm(t, a, userID, p.Page, "GET", base, pv, nil, false).Body.String()
	}
	// "Perdida" is one of the follow-up radios; it takes only a Motivo.
	lose := func(reason string) *httptest.ResponseRecorder {
		return doForm(t, a, ana, p.Seguimiento, "POST", base+"/seguimiento", pv,
			url.Values{"probabilidad": {"perdida"}, "reason": {reason}}, false)
	}

	if body := page(ana); !strings.Contains(body, `value="perdida"`) || !strings.Contains(body, "Motivo de la pérdida") || strings.Contains(body, "Perder el proyecto") {
		t.Error("a prospecto doesn't offer Perdida as a probability, or still has the old block")
	}
	for _, bad := range []string{"", "   ", strings.Repeat("x", maxCommentLen+1)} {
		if rec := lose(bad); rec.Code != http.StatusBadRequest {
			t.Errorf("Perder(reason of %d chars) status = %d, want 400", len(bad), rec.Code)
		}
	}
	if rec := lose("Se fueron con otro proveedor"); rec.Code != http.StatusSeeOther {
		t.Fatalf("Perder status = %d; body = %s", rec.Code, rec.Body.String())
	}

	body := page(ana)
	for _, want := range []string{"Perdido", "Proyecto perdido", "Se fueron con otro proveedor", "estaba en Prospecto",
		"Se perdió.", "Reabrir como Prospecto", `name="comment"`} {
		if !strings.Contains(body, want) {
			t.Errorf("lost proyecto page missing %q", want)
		}
	}
	for _, gone := range []string{"Marcar como perdid", "data-followup", "Pasar a", `value="perdida"`} {
		if strings.Contains(body, gone) {
			t.Errorf("lost proyecto page shows %q to a vendedor", gone)
		}
	}
	if rec := lose("otra vez"); rec.Code != http.StatusConflict {
		t.Errorf("Perder(already lost) status = %d, want 409", rec.Code)
	}
	// Its quote says why it can't be revised, and the list finds it under Perdido.
	quotePage := doForm(t, a, ana, q.Builder, "GET", "/cotizaciones/"+folio, pv, nil, false).Body.String()
	if strings.Contains(quotePage, ">Revisar<") || !strings.Contains(quotePage, "su proyecto se marcó como perdido") {
		t.Error("the quote of a lost proyecto offers Revisar, or doesn't say why not")
	}
	list := doForm(t, a, ana, p.List, "GET", "/proyectos?f.etapa=perdido", nil, nil, false).Body.String()
	if !strings.Contains(list, `href="/proyectos/`+folio+`"`) {
		t.Error("the list filtered by Perdido doesn't show the lost proyecto")
	}

	// Any user reopens it, back to where it was (issue #22): here the vendedor.
	if body := page(ana); !strings.Contains(body, "Reabrir como Prospecto") || !strings.Contains(body, "Reabrir con probabilidad de cierre") {
		t.Fatal("vendedor is not offered reopening, with the probability to come back at")
	}
	reopen := func(form url.Values) *httptest.ResponseRecorder {
		return doForm(t, a, ana, p.Reabrir, "POST", base+"/reabrir", pv, form, false)
	}
	// The comment is required, and the probability has to be one of the steps.
	if rec := reopen(url.Values{"note": {"  "}}); rec.Code != http.StatusBadRequest {
		t.Errorf("Reabrir(no comment) status = %d, want 400", rec.Code)
	}
	if rec := reopen(url.Values{"note": {"Volvieron"}, "probabilidad": {"100"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("Reabrir(probability 100) status = %d, want 400", rec.Code)
	}
	rec := reopen(url.Values{"note": {"Volvieron a llamar"}, "probabilidad": {"50"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Reabrir status = %d; body = %s", rec.Code, rec.Body.String())
	}
	body = page(ana)
	for _, want := range []string{"Prospecto", "Media (50%)", "Perdida", "Se reabrió como Prospecto.", "Probabilidad: Inicial → Media.", "Volvieron a llamar"} {
		if !strings.Contains(body, want) {
			t.Errorf("reopened proyecto page missing %q", want)
		}
	}
	if rec := reopen(url.Values{"note": {"otra vez"}}); rec.Code != http.StatusConflict {
		t.Errorf("Reabrir(not lost) status = %d, want 409", rec.Code)
	}
}

func TestProjectsFollowUpDatesAndForecast(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	p := NewProjects(a.store)
	ana := createTestUser(t, a, "ana", "vendedor", "hunter2")
	customerID, _, costProductID := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, ana)

	forecast := func(query string) string {
		return doForm(t, a, ana, p.Pronostico, "GET", "/pronostico?"+query, nil, nil, false).Body.String()
	}
	if body := forecast(""); !strings.Contains(body, "Ningún prospecto es relevante para pronóstico todavía.") {
		t.Error("empty forecast doesn't say so")
	}

	first := issueTestQuote(t, a, q, ana, customerID, costProductID)
	second := issueTestQuote(t, a, q, ana, customerID, costProductID)
	save := func(folio string, form url.Values) *httptest.ResponseRecorder {
		return doForm(t, a, ana, p.Seguimiento, "POST", "/proyectos/"+folio+"/seguimiento", map[string]string{"folio": folio}, form, false)
	}

	if rec := save(first, url.Values{"probabilidad": {"75"}, "oc_esperada": {"15/10/2026"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("Seguimiento(malformed date) status = %d, want 400", rec.Code)
	}
	// A new prospecto is not in the forecast, but is among all prospectos.
	if body := forecast(""); strings.Contains(body, `id="p-`+first+`"`) {
		t.Error("a prospecto at Inicial is in the forecast")
	}
	body := forecast("todos=1")
	for _, want := range []string{`id="p-` + first + `"`, `id="p-` + second + `"`, "Ver solo los relevantes para pronóstico",
		"Sin fecha", "Sin comentarios"} {
		if !strings.Contains(body, want) {
			t.Errorf("all-prospects page missing %q", want)
		}
	}
	// It is a table to read, not a form: nothing is edited here.
	for _, gone := range []string{"Guardar seguimiento", `name="probabilidad"`, `name="note"`} {
		if strings.Contains(body, gone) {
			t.Errorf("forecast page still has %q", gone)
		}
	}

	// The first proyecto's follow-up day is already past, which the form no longer takes, so
	// it is set in the store; the forecast has to show it as overdue.
	firstProject, err := a.store.ProjectByFolio(context.Background(), first)
	if err != nil || firstProject == nil {
		t.Fatalf("ProjectByFolio: %v, %v", firstProject, err)
	}
	seventyFive, expectedOC, overdue := 75, "2020-10-15", "2020-01-09"
	if err := a.store.FollowUpProject(context.Background(), firstProject.ID, ana, store.FollowUp{
		Probability: &seventyFive, ExpectedOC: &expectedOC, NextFollowUp: &overdue, Note: "Compras pidió ajustar entrega"}); err != nil {
		t.Fatalf("FollowUpProject: %v", err)
	}
	// Saving from the proyecto page always returns to it.
	rec := save(second, url.Values{"probabilidad": {"90"}, "proximo_seguimiento": {futureDay()}, "note": {"Pidió fecha"}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/proyectos/"+second+"#historial" {
		t.Fatalf("Seguimiento status = %d, Location = %q", rec.Code, rec.Header().Get("Location"))
	}
	// Commenting without changing anything (issue #22): the form comes with the current
	// probability and dates, and a next follow-up still ahead is enough to save a comment.
	if rec := save(second, url.Values{"probabilidad": {"90"}, "proximo_seguimiento": {futureDay()}, "note": {"Solo un comentario"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("comment-only Seguimiento status = %d; body = %s", rec.Code, rec.Body.String())
	}
	if body := doForm(t, a, ana, p.Page, "GET", "/proyectos/"+second, map[string]string{"folio": second}, nil, false).Body.String(); !strings.Contains(body, "Solo un comentario") ||
		!strings.Contains(body, `name="proximo_seguimiento" value="`+futureDay()+`"`) {
		t.Error("the comment-only follow-up isn't in the history, or the form doesn't come with the current next follow-up")
	}

	body = forecast("")
	for _, want := range []string{`id="p-` + first + `"`, `id="p-` + second + `"`, "Ver todos los prospectos",
		"Alta (75%)", "Inminente (90%)", "15/10/2020", "09/01/2020",
		"Compras pidió ajustar entrega", "Solo un comentario"} {
		if !strings.Contains(body, want) {
			t.Errorf("forecast page missing %q", want)
		}
	}
	// Everything listed is relevant, so no row says so, and there is no weighted total (#23).
	for _, gone := range []string{"Relevante para pronóstico", "Total ponderado"} {
		if strings.Contains(body, gone) {
			t.Errorf("forecast page still has %q", gone)
		}
	}
	// Column order: project number, cliente, vendedor, probabilidad, seguimiento, O.C.,
	// vigencia, días, costo, precio, utilidad, margen, último comentario.
	head := body[strings.Index(body, "<thead"):]
	last := -1
	for _, col := range []string{"No. de proyecto", "Cliente", "Vendedor", "Probabilidad de cierre", "Próximo seguimiento",
		"O.C. esperada", "Vigencia", "Días sin actualizar", "Costo sin IVA", "Precio de venta sin IVA", "Utilidad", "Margen", "Último comentario"} {
		at := strings.Index(head, ">"+col)
		if at < 0 || at < last {
			t.Errorf("column %q is missing or out of order", col)
		}
		last = at
	}
	// The project number leads to the proyecto.
	if !strings.Contains(body, `<th scope="row"><a href="/proyectos/`+first+`">`) {
		t.Error("the project number doesn't link to the proyecto")
	}
	// Warnings come in words: the follow-up day passed and so did the expected O.C.
	if !strings.Contains(body, `<strong class="overdue">Vencido</strong>`) || !strings.Contains(body, `<strong class="overdue">Vencida</strong>`) {
		t.Error("overdue follow-up or expected O.C. is not flagged")
	}
	// The dated proyecto comes before the undated one.
	if strings.Index(body, `id="p-`+first+`"`) > strings.Index(body, `id="p-`+second+`"`) {
		t.Error("the forecast doesn't put the soonest expected O.C. first")
	}
	// Each proyecto is a $64.29 line at 30%: $45.00 of cost and $19.29 of profit. The totals
	// row adds them, with the margin as profit over price.
	for _, want := range []string{"$45.00", "$64.29", "$19.29", "30.0%", "$90.00", "$128.58", "$38.58"} {
		if !strings.Contains(body, want) {
			t.Errorf("forecast missing %s", want)
		}
	}

	// Sorting by a column, either way, and filtering by vendedor.
	inOrder := func(query, firstFolio, secondFolio string) {
		t.Helper()
		b := forecast(query)
		if strings.Index(b, `id="p-`+firstFolio+`"`) > strings.Index(b, `id="p-`+secondFolio+`"`) {
			t.Errorf("forecast?%s doesn't list %s before %s", query, firstFolio, secondFolio)
		}
	}
	inOrder("sort=probabilidad&dir=asc", first, second)
	inOrder("sort=probabilidad&dir=desc", second, first)
	inOrder("sort=oc&dir=desc", second, first) // the one without a date sorts after every date
	for _, col := range []string{"folio", "cliente", "vendedor", "probabilidad", "seguimiento", "oc", "vigencia", "dias",
		"costo", "precio", "utilidad", "margen", "comentario"} {
		if b := forecast("sort=" + col + "&dir=desc"); !strings.Contains(b, `id="p-`+first+`"`) {
			t.Errorf("forecast sorted by %s lost a row", col)
		}
	}
	if b := forecast("f.vendedor.is=ana"); !strings.Contains(b, `id="p-`+first+`"`) {
		t.Error("filtering by the owner hides her proyectos")
	}
	if b := forecast("f.vendedor.is=nadie"); strings.Contains(b, `id="p-`) || !strings.Contains(b, "Ningún prospecto coincide con el filtro.") {
		t.Error("filtering by another vendedor still lists proyectos or doesn't say none match")
	}
	// The all-prospects toggle survives sorting.
	if b := forecast("todos=1&sort=folio"); !strings.Contains(b, "todos=1") || !strings.Contains(b, `href="/pronostico?dir=desc&amp;sort=folio&amp;todos=1"`) {
		t.Error("sort links drop the todos=1 toggle")
	}

	// The proyecto page shows the dates and that the follow-up is due.
	page := doForm(t, a, ana, p.Page, "GET", "/proyectos/"+first, map[string]string{"folio": first}, nil, false).Body.String()
	for _, want := range []string{"15/10/2020", "09/01/2020", "toca darle seguimiento", `value="2020-10-15"`,
		"O.C. esperada: 15/10/2020.", "Próximo seguimiento: 09/01/2020."} {
		if !strings.Contains(page, want) {
			t.Errorf("proyecto page missing %q", want)
		}
	}
}

// postOC sends the purchase order form as the browser does, multipart, with a file when
// filename isn't empty.
func postOC(t *testing.T, a *Auth, p *Projects, userID int64, folio string, fields map[string]string, filename string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatalf("WriteField: %v", err)
		}
	}
	if filename != "" {
		fw, err := mw.CreateFormFile("archivo", filename)
		if err != nil {
			t.Fatalf("CreateFormFile: %v", err)
		}
		fw.Write(data)
	}
	mw.Close()
	req := httptest.NewRequest("POST", "/proyectos/"+folio+"/oc", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.SetPathValue("folio", folio)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
	rec := httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(p.OC)).ServeHTTP(rec, req)
	return rec
}

func TestProjectsReceiveOC(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	p := NewProjects(a.store)
	ana := createTestUser(t, a, "ana", "vendedor", "hunter2")
	customerID, _, costProductID := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, ana)

	folio := issueTestQuote(t, a, q, ana, customerID, costProductID)
	pv := map[string]string{"folio": folio}
	page := func() string {
		return doForm(t, a, ana, p.Page, "GET", "/proyectos/"+folio, pv, nil, false).Body.String()
	}
	fields := func(number, date, method string) map[string]string {
		return map[string]string{"oc_numero": number, "oc_fecha": date, "forma_pago": method, "note": "Llegó por correo"}
	}
	pdf := []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n")

	// The section stays out of sight until the sale is Inminente.
	if body := page(); strings.Contains(body, "Recibir la orden de compra") {
		t.Error("a prospecto at Inicial offers receiving the purchase order")
	}
	project, err := a.store.ProjectByFolio(context.Background(), folio)
	if err != nil || project == nil {
		t.Fatalf("ProjectByFolio: %v, %v", project, err)
	}
	if err := a.store.SetProjectProbability(context.Background(), project.ID, ana, 90, ""); err != nil {
		t.Fatalf("SetProjectProbability: %v", err)
	}

	body := page()
	for _, want := range []string{"Recibir la orden de compra", "No. de O.C.", "Fecha de la O.C.", "Forma de pago",
		"P.U.E. · pago en una sola exhibición", "P.P.D. · pago en parcialidades o diferido", "Pasar a O.C. recibida",
		`enctype="multipart/form-data"`} {
		if !strings.Contains(body, want) {
			t.Errorf("prospecto page missing %q", want)
		}
	}
	if strings.Contains(body, "Orden de compra del cliente") {
		t.Error("a prospecto with no purchase order shows the purchase order section")
	}

	// Number, date and forma de pago are all required; the file has to be a real PDF or
	// image, whatever it is called.
	for name, c := range map[string]struct {
		fields map[string]string
		file   string
		data   []byte
		want   int
	}{
		"no number":      {fields("", "2026-10-07", "PUE"), "", nil, http.StatusBadRequest},
		"no date":        {fields("4411", "", "PUE"), "", nil, http.StatusBadRequest},
		"no forma":       {fields("4411", "2026-10-07", ""), "", nil, http.StatusBadRequest},
		"no file":        {fields("4411", "2026-10-07", "PUE"), "", nil, http.StatusBadRequest}, // required to receive it
		"unknown forma":  {fields("4411", "2026-10-07", "contado"), "", nil, http.StatusBadRequest},
		"html as pdf":    {fields("4411", "2026-10-07", "PUE"), "oc.pdf", []byte("<html><script>alert(1)</script></html>"), http.StatusBadRequest},
		"empty file":     {fields("4411", "2026-10-07", "PUE"), "oc.pdf", nil, http.StatusBadRequest},
		"file too large": {fields("4411", "2026-10-07", "PUE"), "oc.pdf", append([]byte("%PDF-1.7\n"), make([]byte, maxOCFileBytes)...), http.StatusRequestEntityTooLarge},
	} {
		if rec := postOC(t, a, p, ana, folio, c.fields, c.file, c.data); rec.Code != c.want {
			t.Errorf("OC(%s) status = %d, want %d; body = %s", name, rec.Code, c.want, rec.Body.String())
		}
	}
	if project, _ := a.store.ProjectByFolio(context.Background(), folio); project.Status != "prospecto" || project.HasOC() {
		t.Fatalf("after refused attempts = %+v", project)
	}

	rec := postOC(t, a, p, ana, folio, fields("4411", "2026-10-07", "PPD"), `C:\Users\ana\OC 4411.pdf`, pdf)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/proyectos/"+folio+"#oc" {
		t.Fatalf("OC status = %d, Location = %q; body = %s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	body = page()
	for _, want := range []string{"O.C. recibida", "Orden de compra del cliente", "4411", "07/10/2026", "P.P.D. · pago en parcialidades o diferido",
		"OC 4411.pdf", "Corregir la orden de compra", "Guardar O.C.", "Registrar factura de anticipo",
		"O.C. 4411 · 07/10/2026 · P.P.D.", "Archivo de la O.C.: OC 4411.pdf", "Llegó por correo"} {
		if !strings.Contains(body, want) {
			t.Errorf("oc_recibida page missing %q", want)
		}
	}
	if strings.Contains(body, `C:\Users`) {
		t.Error("the uploaded file kept its client-side path")
	}

	// The file downloads as an attachment, with the type its bytes have.
	files, err := a.store.ListProjectFiles(context.Background(), mustProjectID(t, a, folio), "oc")
	if err != nil || len(files) != 1 {
		t.Fatalf("ListProjectFiles = %+v, %v", files, err)
	}
	id := strconv.FormatInt(files[0].ID, 10)
	download := func(folio, id string) *httptest.ResponseRecorder {
		return doForm(t, a, ana, p.Archivo, "GET", "/proyectos/"+folio+"/archivos/"+id+"?descargar=1", map[string]string{"folio": folio, "id": id}, nil, false)
	}
	rec = download(folio, id)
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), pdf) || rec.Header().Get("Content-Type") != "application/pdf" ||
		!strings.HasPrefix(rec.Header().Get("Content-Disposition"), "attachment") || !strings.Contains(rec.Header().Get("Content-Disposition"), "OC 4411.pdf") ||
		rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("Content-Security-Policy") != uploadCSP {
		t.Errorf("download = %d, headers %v", rec.Code, rec.Header())
	}
	other := issueTestQuote(t, a, q, ana, customerID, costProductID)
	for name, r := range map[string]*httptest.ResponseRecorder{
		"another proyecto's url": download(other, id), "unknown id": download(folio, "9999"), "not a number": download(folio, "x"),
	} {
		if r.Code != http.StatusNotFound {
			t.Errorf("download(%s) status = %d, want 404", name, r.Code)
		}
	}

	// Correcting it keeps the first file and marks the new one as the one in force.
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...)
	if rec := postOC(t, a, p, ana, folio, fields("4411-A", "2026-10-08", "PUE"), "oc-corregida.png", png); rec.Code != http.StatusSeeOther {
		t.Fatalf("OC(correction) status = %d; body = %s", rec.Code, rec.Body.String())
	}
	body = page()
	for _, want := range []string{"4411-A", "08/10/2026", "P.U.E. · pago en una sola exhibición", "oc-corregida.png", "OC 4411.pdf", "el vigente",
		"O.C. actualizada: 4411-A · 08/10/2026 · P.U.E."} {
		if !strings.Contains(body, want) {
			t.Errorf("corrected page missing %q", want)
		}
	}

	// Once it is invoiced the purchase order is shown but closed.
	if rec := doForm(t, a, ana, p.Factura, "POST", "/proyectos/"+folio+"/factura", pv, testInvoiceForm(), false); rec.Code != http.StatusSeeOther {
		t.Fatalf("Factura status = %d; body = %s", rec.Code, rec.Body.String())
	}
	body = page()
	if !strings.Contains(body, "Orden de compra del cliente") || strings.Contains(body, "Guardar O.C.") {
		t.Error("an invoiced proyecto doesn't show its purchase order, or still lets it be edited")
	}
	if rec := postOC(t, a, p, ana, folio, fields("9999", "2026-10-09", "PUE"), "", nil); rec.Code != http.StatusConflict {
		t.Errorf("OC(facturado) status = %d, want 409", rec.Code)
	}
}

// A proyecto that was in O.C. recibida before the purchase order was asked for has to
// have it captured before it can go on.
func TestProjectsLegacyOCRecibida(t *testing.T) {
	// Its own database file, so the old state can be written behind the store's back.
	dsn := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(context.Background(), dsn, cladex.MigrationsFS)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	a := NewAuth(st, false)
	q := newTestQuotes(t, a)
	p := NewProjects(a.store)
	ana := createTestUser(t, a, "ana", "vendedor", "hunter2")
	customerID, _, costProductID := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, ana)
	folio := issueTestQuote(t, a, q, ana, customerID, costProductID)
	pv := map[string]string{"folio": folio}
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer raw.Close()
	if _, err := raw.Exec(`UPDATE projects SET status = 'oc_recibida' WHERE folio = ?`, folio); err != nil {
		t.Fatalf("seed: %v", err)
	}

	body := doForm(t, a, ana, p.Page, "GET", "/proyectos/"+folio, pv, nil, false).Body.String()
	for _, want := range []string{"primero captura la orden de compra del cliente", "antes de que se pidieran los datos de la orden", "Guardar O.C."} {
		if !strings.Contains(body, want) {
			t.Errorf("legacy oc_recibida page missing %q", want)
		}
	}
	if strings.Contains(body, "Facturación y pago") {
		t.Error("a proyecto with no purchase order on record offers invoicing")
	}
	invoice := func() *httptest.ResponseRecorder {
		return doForm(t, a, ana, p.Factura, "POST", "/proyectos/"+folio+"/factura", pv, testInvoiceForm(), false)
	}
	if rec := invoice(); rec.Code != http.StatusConflict {
		t.Errorf("Factura without O.C. status = %d, want 409", rec.Code)
	}
	if rec := receiveOC(t, a, p, ana, folio, ""); rec.Code != http.StatusSeeOther {
		t.Fatalf("OC status = %d; body = %s", rec.Code, rec.Body.String())
	}
	if rec := invoice(); rec.Code != http.StatusSeeOther {
		t.Errorf("Factura with O.C. status = %d", rec.Code)
	}
}

func mustProjectID(t *testing.T, a *Auth, folio string) int64 {
	t.Helper()
	p, err := a.store.ProjectByFolio(context.Background(), folio)
	if err != nil || p == nil {
		t.Fatalf("ProjectByFolio(%s): %v, %v", folio, p, err)
	}
	return p.ID
}

// TestProjectsPaymentGates follows a P.P.D. proyecto through the screens: each blocked
// move says what is missing, it is delivered while unpaid, and it closes once paid.
func TestProjectsPaymentGates(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	p := NewProjects(a.store)
	ana := createTestUser(t, a, "ana", "vendedor", "hunter2")
	customerID, _, costProductID := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, ana)

	folio := issueTestQuote(t, a, q, ana, customerID, costProductID)
	pv := map[string]string{"folio": folio}
	base := "/proyectos/" + folio
	page := func() string {
		return doForm(t, a, ana, p.Page, "GET", base, pv, nil, false).Body.String()
	}
	post := func(h http.HandlerFunc, path string, form url.Values) *httptest.ResponseRecorder {
		return doForm(t, a, ana, h, "POST", base+path, pv, form, false)
	}
	move := func(to string) *httptest.ResponseRecorder { return post(p.Etapa, "/etapa", url.Values{"to": {to}}) }
	doc := func(folio, date string) url.Values {
		return url.Values{"folio": {folio}, "fecha": {date}, "note": {"nota"}}
	}

	// Nothing to invoice or collect before the purchase order.
	if rec := post(p.Factura, "/factura", doc("A-77", "2026-10-08")); rec.Code != http.StatusConflict {
		t.Errorf("Factura(prospecto) status = %d, want 409", rec.Code)
	}
	oc := map[string]string{"oc_numero": "4411", "oc_fecha": "2026-10-07", "forma_pago": "PPD"}
	if rec := postOC(t, a, p, ana, folio, oc, "oc.pdf", []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n")); rec.Code != http.StatusSeeOther {
		t.Fatalf("OC status = %d; body = %s", rec.Code, rec.Body.String())
	}

	body := page()
	for _, want := range []string{"Facturación y pago", "Sin tramitar", "Registrar factura de anticipo", "Folio de la factura de anticipo",
		"Registrar factura y pasar a Facturado", "Sin factura no se puede entregar"} {
		if !strings.Contains(body, want) {
			t.Errorf("oc_recibida page missing %q", want)
		}
	}
	if strings.Contains(body, "Pasar a Facturado<") || strings.Contains(body, "Pasar a En entrega") {
		t.Error("oc_recibida offers a stage button that skips the factura")
	}
	for _, to := range []string{"facturado", "en_entrega"} {
		if rec := move(to); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "primero registra la factura") {
			t.Errorf("Etapa(→ %s) without a factura = %d %q", to, rec.Code, rec.Body.String())
		}
	}
	for name, bad := range map[string]url.Values{"no folio": doc("", "2026-10-08"), "no date": doc("A-77", ""), "bad date": doc("A-77", "8/10/2026")} {
		if rec := post(p.Factura, "/factura", bad); rec.Code != http.StatusBadRequest {
			t.Errorf("Factura(%s) status = %d, want 400", name, rec.Code)
		}
	}
	if rec := post(p.Pago, "/pago", doc("CP-9", "2026-10-20")); rec.Code != http.StatusConflict {
		t.Errorf("Pago before the factura status = %d, want 409", rec.Code)
	}

	rec := post(p.Factura, "/factura", doc("A-77", "2026-10-08"))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != base+"#pago" {
		t.Fatalf("Factura status = %d, Location = %q; body = %s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	body = page()
	for _, want := range []string{"Facturado", "Facturado de anticipo", "A-77 · 08/10/2026", "Registrar pago completado",
		"Folio del comprobante de pago", "Pasar a En entrega", "Factura de anticipo A-77 · 08/10/2026"} {
		if !strings.Contains(body, want) {
			t.Errorf("facturado page missing %q", want)
		}
	}

	// Delivered while unpaid, but not closed.
	if rec := move("en_entrega"); rec.Code != http.StatusSeeOther {
		t.Fatalf("Etapa(→ en_entrega) status = %d; body = %s", rec.Code, rec.Body.String())
	}
	body = page()
	if !strings.Contains(body, "Para cerrarlo, primero registra el pago completado") || strings.Contains(body, "Pasar a Cerrado") {
		t.Error("an unpaid proyecto in entrega offers closing, or doesn't say what is missing")
	}
	if rec := move("cerrado"); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "primero registra el pago completado") {
		t.Errorf("Etapa(→ cerrado) while unpaid = %d %q", rec.Code, rec.Body.String())
	}
	if rec := post(p.Pago, "/pago", doc("CP-9", "2026-10-20")); rec.Code != http.StatusSeeOther {
		t.Fatalf("Pago status = %d; body = %s", rec.Code, rec.Body.String())
	}
	body = page()
	for _, want := range []string{"Pagado", "CP-9", "20/10/2026", "Pasar a Cerrado", "Pago completado · comprobante de pago CP-9 · 20/10/2026"} {
		if !strings.Contains(body, want) {
			t.Errorf("paid page missing %q", want)
		}
	}
	if strings.Contains(body, "Registrar pago completado<") {
		t.Error("a paid proyecto still offers recording the payment")
	}
	if rec := post(p.Pago, "/pago", doc("CP-10", "2026-10-21")); rec.Code != http.StatusConflict {
		t.Errorf("Pago twice status = %d, want 409", rec.Code)
	}

	// The list shows and filters by payment state.
	list := func(query string) string {
		return doForm(t, a, ana, p.List, "GET", "/proyectos?"+query, nil, nil, false).Body.String()
	}
	link := `href="/proyectos/` + folio + `"`
	if body := list("f.pago=pagado"); !strings.Contains(body, link) || !strings.Contains(body, "Pagado") {
		t.Error("the list filtered by Pagado doesn't show the paid proyecto")
	}
	if body := list("f.pago=facturado_anticipo"); strings.Contains(body, link) {
		t.Error("the list filtered by Facturado de anticipo shows a paid proyecto")
	}

	if rec := move("cerrado"); rec.Code != http.StatusSeeOther {
		t.Fatalf("Etapa(→ cerrado) once paid status = %d; body = %s", rec.Code, rec.Body.String())
	}
}

// A P.U.E. order is paid by the same step that invoices it.
func TestProjectsPUEIsPaidWhenInvoiced(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	p := NewProjects(a.store)
	ana := createTestUser(t, a, "ana", "vendedor", "hunter2")
	customerID, _, costProductID := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, ana)
	folio := issueTestQuote(t, a, q, ana, customerID, costProductID)
	pv := map[string]string{"folio": folio}
	base := "/proyectos/" + folio
	page := func() string {
		return doForm(t, a, ana, p.Page, "GET", base, pv, nil, false).Body.String()
	}
	if rec := receiveOC(t, a, p, ana, folio, ""); rec.Code != http.StatusSeeOther {
		t.Fatalf("OC status = %d", rec.Code)
	}
	body := page()
	for _, want := range []string{"Registrar pago y factura", "Folio de la factura", "Fecha de pago", "Registrar pago y pasar a Facturado"} {
		if !strings.Contains(body, want) {
			t.Errorf("P.U.E. oc_recibida page missing %q", want)
		}
	}
	if rec := doForm(t, a, ana, p.Factura, "POST", base+"/factura", pv, testInvoiceForm(), false); rec.Code != http.StatusSeeOther {
		t.Fatalf("Factura status = %d", rec.Code)
	}
	body = page()
	for _, want := range []string{"Pagado", "F-1001 · 08/10/2026", "Pagado el", "Pago recibido · factura F-1001 · 08/10/2026", "Pasar a En entrega"} {
		if !strings.Contains(body, want) {
			t.Errorf("P.U.E. facturado page missing %q", want)
		}
	}
	if strings.Contains(body, "Registrar pago completado") {
		t.Error("a P.U.E. proyecto, already paid, offers recording a payment")
	}
}

// Issue #46: the proyecto page shows its current quote's cost, profit and margin before
// IVA, the same figures as the pronóstico, and leaves them out while a revision is in
// draft (no margin frozen yet).
func TestProjectsPageShowsProfit(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	p := NewProjects(a.store)
	ana := createTestUser(t, a, "ana", "vendedor", "hunter2")
	customerID, _, costProductID := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, ana)
	ctx := context.Background()

	folio := issueTestQuote(t, a, q, ana, customerID, costProductID)
	project, err := a.store.ProjectByFolio(ctx, folio)
	if err != nil || project == nil || project.Profit <= 0 {
		t.Fatalf("ProjectByFolio = %+v, %v; want a profit", project, err)
	}
	pv := map[string]string{"folio": folio}
	body := doForm(t, a, ana, p.Page, "GET", "/proyectos/"+folio, pv, nil, false).Body.String()
	for _, want := range []string{
		"<dt>Costo sin IVA</dt><dd>" + project.Cost().String() + "</dd>",
		"<dt>Utilidad</dt><dd>" + project.Profit.String() + "</dd>",
		"<dt>Margen</dt><dd>" + views.ProjectMargin(*project) + "</dd>",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("proyecto page missing %q", want)
		}
	}

	original, _ := a.store.QuoteByFolio(ctx, folio)
	if _, err := a.store.CreateRevision(ctx, original.ID, ana); err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	body = doForm(t, a, ana, p.Page, "GET", "/proyectos/"+folio, pv, nil, false).Body.String()
	if strings.Contains(body, "<dt>Utilidad</dt>") {
		t.Error("a proyecto whose current quote is a draft shows a profit")
	}
}
