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

	"github.com/gerrygoo/cladex-web"
	"github.com/gerrygoo/cladex-web/internal/store"
)

// testOCForm is a complete purchase order form, without a file.
func testOCForm(note string) url.Values {
	return url.Values{"oc_numero": {"4411"}, "oc_fecha": {"2026-10-07"}, "forma_pago": {"PUE"}, "note": {note}}
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
	for _, want := range []string{"Proyecto " + folio, "Prospecto", "Inicial (10%)", "Guardar seguimiento",
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
	for _, gone := range []string{"Pasar a", "Guardar seguimiento"} {
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
	if rec := move(ana, "en_entrega", strings.Repeat("x", maxCommentLen+1)); rec.Code != http.StatusBadRequest {
		t.Errorf("Etapa(note too long) status = %d, want 400", rec.Code)
	}
	// The stage button doesn't take a prospecto to O.C. recibida: the purchase order does.
	if rec := move(ana, "oc_recibida", ""); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "primero captura la orden de compra") {
		t.Errorf("Etapa(→ oc_recibida) = %d %q, want 409 asking for the purchase order", rec.Code, rec.Body.String())
	}
	receive := func(note string) *httptest.ResponseRecorder {
		return doForm(t, a, ana, p.OC, "POST", base+"/oc", pv, testOCForm(note), false)
	}
	if rec := receive("OC 4411"); rec.Code != http.StatusSeeOther {
		t.Fatalf("OC status = %d; body = %s", rec.Code, rec.Body.String())
	}

	// With the purchase order in: no probability, and the quote can't be revised.
	body = page(ana)
	for _, want := range []string{"O.C. recibida", "Registrar pago y pasar a Facturado", "Pasó a O.C. recibida.", "OC 4411"} {
		if !strings.Contains(body, want) {
			t.Errorf("oc_recibida page missing %q", want)
		}
	}
	for _, gone := range []string{"Guardar seguimiento", "Relevante para pronóstico"} {
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

	body := doForm(t, a, ana, p.Page, "GET", "/proyectos/"+folio, pv, nil, false).Body.String()
	for _, want := range []string{rev.Folio, "revisión en borrador", "primero emite la revisión", "Revisada", "Guardar seguimiento"} {
		if !strings.Contains(body, want) {
			t.Errorf("proyecto page missing %q", want)
		}
	}
	if strings.Contains(body, "Pasar a O.C. recibida") {
		t.Error("a proyecto whose current quote is a draft offers O.C. recibida")
	}
	rec := doForm(t, a, ana, p.OC, "POST", "/proyectos/"+folio+"/oc", pv, testOCForm(""), false)
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
	beto := createTestUser(t, a, "beto", "admin", "hunter2")
	customerID, _, costProductID := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, ana)

	folio := issueTestQuote(t, a, q, ana, customerID, costProductID)
	pv := map[string]string{"folio": folio}
	base := "/proyectos/" + folio
	page := func(userID int64) string {
		return doForm(t, a, userID, p.Page, "GET", base, pv, nil, false).Body.String()
	}
	lose := func(reason string) *httptest.ResponseRecorder {
		return doForm(t, a, ana, p.Perder, "POST", base+"/perder", pv, url.Values{"reason": {reason}}, false)
	}

	if body := page(ana); !strings.Contains(body, "Marcar como perdido") || !strings.Contains(body, "Perder el proyecto") {
		t.Error("a prospecto doesn't offer marking it lost")
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
		"pídele a un administrador que lo reabra", "Se perdió."} {
		if !strings.Contains(body, want) {
			t.Errorf("lost proyecto page missing %q", want)
		}
	}
	for _, gone := range []string{"Marcar como perdido", "Guardar seguimiento", "Pasar a", "Reabrir como"} {
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

	// An admin reopens it, back to where it was. (The router keeps vendedores out.)
	if !strings.Contains(page(beto), "Reabrir como Prospecto") {
		t.Fatal("admin is not offered reopening")
	}
	rec := doForm(t, a, beto, p.Reabrir, "POST", base+"/reabrir", pv, url.Values{"note": {"Volvieron a llamar"}}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Reabrir status = %d; body = %s", rec.Code, rec.Body.String())
	}
	body = page(ana)
	for _, want := range []string{"Prospecto", "Marcar como perdido", "Se reabrió como Prospecto.", "Volvieron a llamar"} {
		if !strings.Contains(body, want) {
			t.Errorf("reopened proyecto page missing %q", want)
		}
	}
	if rec := doForm(t, a, beto, p.Reabrir, "POST", base+"/reabrir", pv, nil, false); rec.Code != http.StatusConflict {
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
		"Guardar seguimiento", "Sin fecha", "Vence la vigencia · ", "Sin comentarios", "Total ponderado"} {
		if !strings.Contains(body, want) {
			t.Errorf("all-prospects page missing %q", want)
		}
	}

	// Saved from the full list, it returns there, at the same proyecto.
	rec := save(first, url.Values{"probabilidad": {"75"}, "oc_esperada": {"2030-10-15"}, "proximo_seguimiento": {"2020-01-09"},
		"note": {"Compras pidió ajustar entrega"}, "volver": {"pronostico-todos"}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/pronostico?todos=1#p-"+first {
		t.Fatalf("Seguimiento status = %d, Location = %q", rec.Code, rec.Header().Get("Location"))
	}
	if rec := save(second, url.Values{"probabilidad": {"90"}, "volver": {"pronostico"}}); rec.Header().Get("Location") != "/pronostico#p-"+second {
		t.Fatalf("Seguimiento Location = %q", rec.Header().Get("Location"))
	}
	if rec := save(second, url.Values{"probabilidad": {"90"}}); rec.Header().Get("Location") != "/proyectos/"+second+"#historial" {
		t.Fatalf("Seguimiento Location = %q", rec.Header().Get("Location"))
	}

	body = forecast("")
	for _, want := range []string{`id="p-` + first + `"`, `id="p-` + second + `"`, "Ver todos los prospectos",
		"Alta (75%)", "Inminente (90%)", "Relevante para pronóstico", "15/10/2030", "Vence la vigencia · ",
		"Vencido: Seguimiento · 09/01/2020", "Compras pidió ajustar entrega", "Sin contacto capturado", "Total ponderado"} {
		if !strings.Contains(body, want) {
			t.Errorf("forecast page missing %q", want)
		}
	}
	// The dated proyecto comes before the undated one.
	if strings.Index(body, `id="p-`+first+`"`) > strings.Index(body, `id="p-`+second+`"`) {
		t.Error("the forecast doesn't put the soonest expected O.C. first")
	}
	// Two proyectos of $74.58 each (a $64.29 line plus IVA): 75% + 90% of it is $123.06.
	for _, want := range []string{"$149.16", "$123.06"} {
		if !strings.Contains(body, want) {
			t.Errorf("forecast totals missing %s", want)
		}
	}

	// The proyecto page shows the dates and that the follow-up is due.
	page := doForm(t, a, ana, p.Page, "GET", "/proyectos/"+first, map[string]string{"folio": first}, nil, false).Body.String()
	for _, want := range []string{"15/10/2030", "09/01/2020", "toca darle seguimiento", `value="2030-10-15"`,
		"O.C. esperada: 15/10/2030.", "Próximo seguimiento: 09/01/2020."} {
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
		return doForm(t, a, ana, p.Archivo, "GET", "/proyectos/"+folio+"/archivos/"+id, map[string]string{"folio": folio, "id": id}, nil, false)
	}
	rec = download(folio, id)
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), pdf) || rec.Header().Get("Content-Type") != "application/pdf" ||
		!strings.HasPrefix(rec.Header().Get("Content-Disposition"), "attachment") || !strings.Contains(rec.Header().Get("Content-Disposition"), "OC 4411.pdf") ||
		rec.Header().Get("X-Content-Type-Options") != "nosniff" {
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
	if rec := doForm(t, a, ana, p.OC, "POST", "/proyectos/"+folio+"/oc", pv, testOCForm(""), false); rec.Code != http.StatusSeeOther {
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
	oc := url.Values{"oc_numero": {"4411"}, "oc_fecha": {"2026-10-07"}, "forma_pago": {"PPD"}}
	if rec := post(p.OC, "/oc", oc); rec.Code != http.StatusSeeOther {
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
	if rec := doForm(t, a, ana, p.OC, "POST", base+"/oc", pv, testOCForm(""), false); rec.Code != http.StatusSeeOther {
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
