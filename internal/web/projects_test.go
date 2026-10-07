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
	for _, want := range []string{rev.Folio, "revisión en borrador", "primero emite la revisión", "Revisada", "Guardar seguimiento"} {
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
