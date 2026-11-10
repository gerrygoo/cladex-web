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
	return url.Values{"oc_numero": {"4411"}, "oc_fecha": {"2026-10-07"}, "note": {note}}
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

// testInvoiceForm is a complete factura form, P.U.E., so posting it also leaves the
// proyecto pagado.
func testInvoiceForm() url.Values {
	return url.Values{"folio": {"F-1001"}, "fecha": {"2026-10-08"}, "metodo_pago": {"PUE"}}
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
	for _, gone := range []string{"Recibir la orden de compra", "Pasar a En curso", "Perder el proyecto", "Comentario (opcional)"} {
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
	for _, want := range []string{"Inminente (90%)", "Recibir la orden de compra", "Pasar a En curso"} {
		if !strings.Contains(body, want) {
			t.Errorf("prospecto page at Inminente missing %q", want)
		}
	}
	if !strings.Contains(body, "<label data-imminent-only>") {
		t.Error("O.C. esperada is hidden at Inminente")
	}
	// Issue #61: the section sits right below O.C. esperada and Próximo seguimiento and
	// above the follow-up's Comentario and Guardar, then the history, the quotes and (for
	// an admin) Administración. It asks for no comment and no forma de pago.
	adminBody := page(beto)
	order := []string{"Probabilidad de cierre", "O.C. esperada", "Próximo seguimiento", "Recibir la orden de compra",
		`class="followup-note"`, `name="note"`, "Ya casi cierra", "<h2>Cotizaciones</h2>", "Administración"}
	last := -1
	for _, want := range order {
		i := strings.Index(adminBody[last+1:], want)
		if i < 0 {
			t.Errorf("Inminente page: %q missing or out of order", want)
			continue
		}
		last += 1 + i
	}
	section := body[strings.Index(body, "Recibir la orden de compra"):strings.Index(body, `class="followup-note"`)]
	for _, gone := range []string{"Comentario", "Forma de pago", "forma_pago", "P.U.E."} {
		if strings.Contains(section, gone) {
			t.Errorf("the receive-the-purchase-order section still has %q", gone)
		}
	}
	// The comment and Guardar sit outside the follow-up <form> and belong to it.
	if !strings.Contains(body, `<textarea form="seguimiento-`+folio+`" name="note"`) ||
		!strings.Contains(body, `<button type="submit" form="seguimiento-`+folio+`">Guardar`) {
		t.Error("the follow-up's comment and Guardar are not tied to its form")
	}

	receive := func(note string) *httptest.ResponseRecorder {
		return receiveOC(t, a, p, ana, folio, note)
	}
	if rec := receive("OC 4411"); rec.Code != http.StatusSeeOther {
		t.Fatalf("OC status = %d; body = %s", rec.Code, rec.Body.String())
	}

	// With the purchase order in: en curso, its three states in the summary right under
	// Margen (#66, #67), no probability to pick, and the quote can't be revised.
	body = page(ana)
	for _, want := range []string{"En curso", "Registrar factura", "Pasó a En curso.", "OC 4411",
		"Orden colocada", "Orden de compra recibida", "<dt>Registró</dt><dd>ana · ",
		"<dt>Estado de facturación</dt><dd>Pendiente</dd>", "<dt>Estado de entrega</dt><dd>Sin emitir O.C.",
		"<dt>Estado de cobro</dt><dd>Pendiente de pago",
		// Issue #61: the probability stays in view, fixed.
		`name="probabilidad_paso" value="90" disabled`, `name="probabilidad_oc" value="100" checked disabled`,
		// The comment needs no date.
		`name="comment"`} {
		if !strings.Contains(body, want) {
			t.Errorf("en curso page missing %q", want)
		}
	}
	summary := body[strings.Index(body, "<dt>Margen</dt>"):]
	if i, j, k := strings.Index(summary, "Estado de facturación"), strings.Index(summary, "Estado de entrega"), strings.Index(summary, "Estado de cobro"); i < 0 || !(i < j && j < k) {
		t.Error("the three states are not under Margen in order")
	}
	// No stage buttons any more, and a vendedor can't lose a proyecto en curso nor see the
	// admin's changes of stage.
	for _, gone := range []string{"Guardar seguimiento", "Relevante para pronóstico", "<dt>O.C. esperada</dt>", "<dt>Próximo seguimiento</dt>", `name="oc_esperada"`,
		"Recibir la orden de compra", "proximo_seguimiento", "data-followup", "Pasar a", "Regresar a", "Marcar como perdida",
		"Cancelar proyecto", "Deshacer O.C. recibida"} {
		if strings.Contains(body, gone) {
			t.Errorf("en curso page still shows %q", gone)
		}
	}
	if rec := prob(ana, "90", "Otra vez"); rec.Code != http.StatusConflict {
		t.Errorf("Probabilidad(en_curso) status = %d, want 409", rec.Code)
	}
	if rec := doForm(t, a, ana, p.Perder, "POST", base+"/perder", pv, url.Values{"reason": {"Se cayó"}}, false); rec.Code != http.StatusConflict {
		t.Errorf("Perder(en_curso) status = %d, want 409", rec.Code)
	}
	if body := quotePage(); strings.Contains(body, ">Revisar<") || !strings.Contains(body, "ya no se puede revisar") {
		t.Error("the quote of a proyecto with its O.C. in still offers Revisar, or doesn't say why not")
	}
	if rec := doForm(t, a, ana, q.Revisar, "POST", "/cotizaciones/"+folio+"/revisar", pv, nil, false); rec.Code != http.StatusConflict {
		t.Errorf("Revisar(en_curso) status = %d, want 409", rec.Code)
	}

	// An admin undoes the O.C. (the route is admin-only: router_test), with a motivo,
	// back to prospecto at Inminente with the O.C. form open again.
	admin := func(h http.HandlerFunc, action, reason string) *httptest.ResponseRecorder {
		return doForm(t, a, beto, h, "POST", base+"/"+action, pv, url.Values{"reason": {reason}}, false)
	}
	adminBody = page(beto)
	for _, want := range []string{"Cancelar proyecto", "Deshacer O.C. recibida", `action="` + base + `/deshacer-oc"`, `action="` + base + `/cancelar"`} {
		if !strings.Contains(adminBody, want) {
			t.Errorf("admin's en curso page missing %q", want)
		}
	}
	if rec := admin(p.DeshacerOC, "deshacer-oc", "  "); rec.Code != http.StatusBadRequest {
		t.Errorf("DeshacerOC(no motivo) status = %d, want 400", rec.Code)
	}
	if rec := admin(p.DeshacerOC, "deshacer-oc", "Se cayó la junta"); rec.Code != http.StatusSeeOther {
		t.Fatalf("DeshacerOC status = %d; body = %s", rec.Code, rec.Body.String())
	}
	body = page(ana)
	for _, want := range []string{"Inminente (90%)", "Recibir la orden de compra", "Se deshizo la O.C. recibida (O.C. 4411 · 07/10/2026).", "Se cayó la junta"} {
		if !strings.Contains(body, want) {
			t.Errorf("page after undoing the O.C. missing %q", want)
		}
	}
	if rec := admin(p.DeshacerOC, "deshacer-oc", "otra vez"); rec.Code != http.StatusConflict {
		t.Errorf("DeshacerOC(prospecto) status = %d, want 409", rec.Code)
	}

	// A comment from the proyecto page lands in its history and on the current quote.
	if rec := receive(""); rec.Code != http.StatusSeeOther {
		t.Fatalf("OC again status = %d; body = %s", rec.Code, rec.Body.String())
	}
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

	// Cancelled by an admin, it shows why and only an admin reactivates it, back en curso.
	if rec := admin(p.Cancelar, "cancelar", "El cliente canceló el pedido"); rec.Code != http.StatusSeeOther {
		t.Fatalf("Cancelar status = %d; body = %s", rec.Code, rec.Body.String())
	}
	body = page(ana)
	for _, want := range []string{"Cancelado", "Proyecto cancelado", "El cliente canceló el pedido", "estaba en En curso",
		"Se canceló.", "Solo un administrador puede reactivarlo."} {
		if !strings.Contains(body, want) {
			t.Errorf("cancelled page missing %q", want)
		}
	}
	if strings.Contains(body, "Estado de cobro") || strings.Contains(body, "Reactivar como") {
		t.Error("a cancelled proyecto shows its states, or offers a vendedor to reactivate it")
	}
	if !strings.Contains(page(beto), "Reactivar como En curso") {
		t.Error("an admin is not offered reactivating")
	}
	if rec := admin(p.Reactivar, "reactivar", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("Reactivar(no motivo) status = %d, want 400", rec.Code)
	}
	if rec := admin(p.Reactivar, "reactivar", "Siempre sí"); rec.Code != http.StatusSeeOther {
		t.Fatalf("Reactivar status = %d; body = %s", rec.Code, rec.Body.String())
	}
	if body := page(ana); !strings.Contains(body, "Se reactivó como En curso.") || !strings.Contains(body, "<dt>Estado de cobro</dt>") {
		t.Error("the reactivated proyecto isn't en curso again")
	}
	if rec := admin(p.Reactivar, "reactivar", "otra vez"); rec.Code != http.StatusConflict {
		t.Errorf("Reactivar(en curso) status = %d, want 409", rec.Code)
	}

	// With a factura on record the O.C. can't be undone; the page says so instead.
	if rec := doForm(t, a, ana, p.Factura, "POST", base+"/factura", pv, testInvoiceForm(), false); rec.Code != http.StatusSeeOther {
		t.Fatalf("Factura status = %d; body = %s", rec.Code, rec.Body.String())
	}
	if body := page(beto); !strings.Contains(body, "No se puede deshacer: el proyecto ya tiene factura.") ||
		!strings.Contains(body, "<dt>Estado de facturación</dt><dd>Facturada</dd>") {
		t.Error("an invoiced proyecto still offers undoing its O.C., or isn't Facturada")
	}
	if rec := admin(p.DeshacerOC, "deshacer-oc", "x"); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "tiene una factura") {
		t.Errorf("DeshacerOC(facturada) = %d %q, want 409", rec.Code, rec.Body.String())
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
	if strings.Contains(body, "Pasar a En curso") {
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
	third := issueTestQuote(t, a, q, ana, customerID, costProductID)
	fourth := issueTestQuote(t, a, q, ana, customerID, costProductID)
	// Next follow-ups (#71): first's is past, third's ahead, fourth has none, and second's
	// stops showing once its O.C. arrives.
	setFollowUp := func(folio, day string) {
		t.Helper()
		pr, err := a.store.ProjectByFolio(context.Background(), folio)
		if err != nil || pr == nil {
			t.Fatalf("ProjectByFolio(%s): %v, %v", folio, pr, err)
		}
		if err := a.store.FollowUpProject(context.Background(), pr.ID, ana, store.FollowUp{NextFollowUp: &day}); err != nil {
			t.Fatalf("FollowUpProject(%s): %v", folio, err)
		}
	}
	setFollowUp(first, "2020-01-09")
	setFollowUp(second, "2030-05-05")
	setFollowUp(third, "2099-03-04")
	project, _ := a.store.ProjectByFolio(context.Background(), second)
	if err := a.store.SetProjectProbability(context.Background(), project.ID, ana, 90, ""); err != nil {
		t.Fatalf("SetProjectProbability: %v", err)
	}
	if err := a.store.ReceiveOC(context.Background(), project.ID, ana, store.OC{Number: "4411", Date: "2026-10-07"}, nil, ""); err != nil {
		t.Fatalf("ReceiveOC: %v", err)
	}

	link := func(folio string) string { return `href="/proyectos/` + folio + `"` }
	body := list("", false)
	for _, want := range []string{link(first), link(second), "Inicial (10%)", "En curso", "<h1>Proyectos</h1>"} {
		if !strings.Contains(body, want) {
			t.Errorf("list missing %q", want)
		}
	}
	// Past prospecto, the probability isn't shown.
	if strings.Contains(body, "Inminente (90%)") {
		t.Error("the list shows a probability for a proyecto past prospecto")
	}
	// The next follow-up shows for prospectos only, and a past one says it is overdue.
	head := body[strings.Index(body, "<thead"):strings.Index(body, "</thead>")]
	if !strings.Contains(head, "Próximo seguimiento") || strings.Index(head, "Próximo seguimiento") < strings.Index(head, "Probabilidad") {
		t.Error("the list has no Próximo seguimiento column after Probabilidad")
	}
	for _, want := range []string{"09/01/2020", "04/03/2099", "Sin fecha"} {
		if !strings.Contains(body, want) {
			t.Errorf("list missing follow-up %q", want)
		}
	}
	if strings.Contains(body, "05/05/2030") {
		t.Error("the list shows the follow-up date of a proyecto past prospecto")
	}
	if n := strings.Count(body, `<strong class="overdue">Vencido</strong>`); n != 1 {
		t.Errorf("list marks %d follow-ups as overdue, want 1 (the past one)", n)
	}
	// Sorting by it: soonest first, prospectos without a date next, then other stages.
	order := func(body string, folios ...string) bool {
		last := -1
		for _, f := range folios {
			at := strings.Index(body, link(f))
			if at < last {
				return false
			}
			last = at
		}
		return true
	}
	if body := list("sort=seguimiento", false); !order(body, first, third, fourth, second) {
		t.Error("sorting by Próximo seguimiento doesn't put the soonest first and other stages last")
	}
	if body := list("sort=seguimiento&dir=desc", false); !order(body, second, fourth, third, first) {
		t.Error("sorting by Próximo seguimiento descending isn't the reverse")
	}
	if body := list("f.etapa=en_curso", false); strings.Contains(body, link(first)) || !strings.Contains(body, link(second)) {
		t.Error("the stage filter doesn't narrow the list to En curso")
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
	fields := func(number, date string) map[string]string {
		return map[string]string{"oc_numero": number, "oc_fecha": date, "note": "Llegó por correo"}
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
	for _, want := range []string{"Recibir la orden de compra", "No. de O.C.", "Fecha de la O.C.",
		"Archivo de la O.C. (obligatorio; PDF, JPG o PNG de hasta 10 MB)", "Pasar a En curso",
		`enctype="multipart/form-data"`} {
		if !strings.Contains(body, want) {
			t.Errorf("prospecto page missing %q", want)
		}
	}
	// Issue #61: the método de pago is no longer asked when the order is received.
	for _, gone := range []string{"Orden de compra recibida", "Forma de pago", "forma_pago", "P.U.E.", "P.P.D."} {
		if strings.Contains(body, gone) {
			t.Errorf("prospecto page still shows %q", gone)
		}
	}

	// Number and date are both required; the file has to be a real PDF or image,
	// whatever it is called.
	for name, c := range map[string]struct {
		fields map[string]string
		file   string
		data   []byte
		want   int
	}{
		"no number":      {fields("", "2026-10-07"), "", nil, http.StatusBadRequest},
		"no date":        {fields("4411", ""), "", nil, http.StatusBadRequest},
		"no file":        {fields("4411", "2026-10-07"), "", nil, http.StatusBadRequest}, // required to receive it
		"html as pdf":    {fields("4411", "2026-10-07"), "oc.pdf", []byte("<html><script>alert(1)</script></html>"), http.StatusBadRequest},
		"empty file":     {fields("4411", "2026-10-07"), "oc.pdf", nil, http.StatusBadRequest},
		"file too large": {fields("4411", "2026-10-07"), "oc.pdf", append([]byte("%PDF-1.7\n"), make([]byte, maxOCFileBytes)...), http.StatusRequestEntityTooLarge},
	} {
		if rec := postOC(t, a, p, ana, folio, c.fields, c.file, c.data); rec.Code != c.want {
			t.Errorf("OC(%s) status = %d, want %d; body = %s", name, rec.Code, c.want, rec.Body.String())
		}
	}
	if project, _ := a.store.ProjectByFolio(context.Background(), folio); project.Status != "prospecto" || project.HasOC() {
		t.Fatalf("after refused attempts = %+v", project)
	}

	// A forma_pago sent by an old page is ignored.
	received := fields("4411", "2026-10-07")
	received["forma_pago"] = "PPD"
	rec := postOC(t, a, p, ana, folio, received, `C:\Users\ana\OC 4411.pdf`, pdf)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/proyectos/"+folio+"#oc" {
		t.Fatalf("OC status = %d, Location = %q; body = %s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	if project, _ := a.store.ProjectByFolio(context.Background(), folio); project.PaymentMethod != "" {
		t.Errorf("receiving the O.C. stored método de pago %q", project.PaymentMethod)
	}
	body = page()
	for _, want := range []string{"En curso", "Orden de compra recibida", "<dd>4411</dd>", "07/10/2026",
		"OC 4411.pdf", "Descargar", "<dt>Registró</dt><dd>ana · ", "Corregir la orden de compra", "Guardar O.C.", "Registrar la factura",
		"O.C. 4411 · 07/10/2026", "Archivo de la O.C.: OC 4411.pdf", "Llegó por correo"} {
		if !strings.Contains(body, want) {
			t.Errorf("oc_recibida page missing %q", want)
		}
	}
	// The receive section and the follow-up dates are gone; the purchase order's summary
	// comes before the comment box.
	for _, gone := range []string{"Recibir la orden de compra", "O.C. 4411 · 07/10/2026 · P.", `name="forma_pago"`} {
		if strings.Contains(body, gone) {
			t.Errorf("oc_recibida page still shows %q", gone)
		}
	}
	if i, j := strings.Index(body, "Orden de compra recibida"), strings.Index(body, `name="comment"`); i < 0 || j < 0 || i > j {
		t.Error("Orden de compra recibida is not above the comment box")
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
	// Its name opens it; Descargar downloads it.
	if !strings.Contains(body, `href="/proyectos/`+folio+`/archivos/`+id+`">OC 4411.pdf</a>`) ||
		!strings.Contains(body, `href="/proyectos/`+folio+`/archivos/`+id+`?descargar=1">Descargar`) {
		t.Error("the purchase order's file has no link to open it and one to download it")
	}
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
	if rec := postOC(t, a, p, ana, folio, fields("4411-A", "2026-10-08"), "oc-corregida.png", png); rec.Code != http.StatusSeeOther {
		t.Fatalf("OC(correction) status = %d; body = %s", rec.Code, rec.Body.String())
	}
	body = page()
	for _, want := range []string{"4411-A", "08/10/2026", "oc-corregida.png", "OC 4411.pdf", "el vigente",
		"O.C. actualizada: 4411-A · 08/10/2026"} {
		if !strings.Contains(body, want) {
			t.Errorf("corrected page missing %q", want)
		}
	}

	// Once it is invoiced the purchase order is shown but closed.
	if rec := doForm(t, a, ana, p.Factura, "POST", "/proyectos/"+folio+"/factura", pv, testInvoiceForm(), false); rec.Code != http.StatusSeeOther {
		t.Fatalf("Factura status = %d; body = %s", rec.Code, rec.Body.String())
	}
	body = page()
	if !strings.Contains(body, "Orden de compra recibida") || strings.Contains(body, "Guardar O.C.") {
		t.Error("an invoiced proyecto doesn't show its purchase order, or still lets it be edited")
	}
	// En curso the probability is still in view, fixed, with no Perdida.
	if !strings.Contains(body, `<fieldset class="probability" disabled>`) || strings.Contains(body, `name="perdida"`) {
		t.Error("an invoiced proyecto's probabilidad de cierre is not shown fixed")
	}
	if rec := postOC(t, a, p, ana, folio, fields("9999", "2026-10-09"), "", nil); rec.Code != http.StatusConflict {
		t.Errorf("OC(facturado) status = %d, want 409", rec.Code)
	}
}

// A proyecto that got its O.C. in (en curso) before the purchase order's data was asked
// for has to have it captured before it can be invoiced.
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
	if _, err := raw.Exec(`UPDATE projects SET status = 'en_curso' WHERE folio = ?`, folio); err != nil {
		t.Fatalf("seed: %v", err)
	}

	body := doForm(t, a, ana, p.Page, "GET", "/proyectos/"+folio, pv, nil, false).Body.String()
	for _, want := range []string{"antes de que se pidieran los datos de la orden", "Guardar O.C."} {
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
	// A método de pago chosen when the order was received, before issue #61, is kept and
	// comes picked in the factura's form.
	if _, err := raw.Exec(`UPDATE projects SET payment_method = 'PPD' WHERE folio = ?`, folio); err != nil {
		t.Fatalf("seed método: %v", err)
	}
	body = doForm(t, a, ana, p.Page, "GET", "/proyectos/"+folio, pv, nil, false).Body.String()
	if !strings.Contains(body, `name="metodo_pago" value="PPD" checked required`) || strings.Contains(body, `value="PUE" checked`) {
		t.Error("the stored método de pago is not the factura form's starting choice")
	}
	if rec := invoice(); rec.Code != http.StatusSeeOther {
		t.Errorf("Factura with O.C. status = %d", rec.Code)
	}
	// The método sent with the factura is the one that counts.
	if project, _ := a.store.ProjectByFolio(context.Background(), folio); project.PaymentMethod != "PUE" || project.PaymentStatus != store.PaymentPaid {
		t.Errorf("after a P.U.E. factura = %+v", project)
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

// TestProjectsTypedFactura follows the factura typed in by hand (while facturación can't
// stamp) through the screens: it makes the Estado de facturación Facturada and moves no
// stage. Typed P.P.D., the pago completado is typed in later.
func TestProjectsTypedFactura(t *testing.T) {
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
	doc := func(folio, date string) url.Values {
		return url.Values{"folio": {folio}, "fecha": {date}, "note": {"nota"}, "metodo_pago": {"PPD"}}
	}

	// Nothing to invoice or collect before the purchase order.
	if rec := post(p.Factura, "/factura", doc("A-77", "2026-10-08")); rec.Code != http.StatusConflict {
		t.Errorf("Factura(prospecto) status = %d, want 409", rec.Code)
	}
	oc := map[string]string{"oc_numero": "4411", "oc_fecha": "2026-10-07"}
	if rec := postOC(t, a, p, ana, folio, oc, "oc.pdf", []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n")); rec.Code != http.StatusSeeOther {
		t.Fatalf("OC status = %d; body = %s", rec.Code, rec.Body.String())
	}

	body := page()
	for _, want := range []string{"Facturación y pago", "Registrar la factura", "Método de pago",
		`name="metodo_pago" value="PUE" required`, `name="metodo_pago" value="PPD" required`,
		"Folio de la factura", "Fecha de la factura", ">Registrar factura</button>", "El Estado de facturación pasa a Facturada."} {
		if !strings.Contains(body, want) {
			t.Errorf("en curso page missing %q", want)
		}
	}
	noMethod, badMethod := doc("A-77", "2026-10-08"), doc("A-77", "2026-10-08")
	noMethod.Del("metodo_pago")
	badMethod.Set("metodo_pago", "contado")
	for name, bad := range map[string]url.Values{"no folio": doc("", "2026-10-08"), "no date": doc("A-77", ""), "bad date": doc("A-77", "8/10/2026"),
		"no método": noMethod, "unknown método": badMethod} {
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
	for _, want := range []string{"En curso", "<dt>Estado de facturación</dt><dd>Facturada</dd>", "A-77 · 08/10/2026", "Registrar pago completado",
		"<dt>Método de pago</dt><dd>P.P.D. · pago en parcialidades o diferido</dd>", "<dt>Factura de anticipo</dt>",
		"Folio del comprobante de pago", "Factura de anticipo A-77 · 08/10/2026"} {
		if !strings.Contains(body, want) {
			t.Errorf("invoiced page missing %q", want)
		}
	}
	if strings.Contains(body, "Registrar la factura") {
		t.Error("an invoiced proyecto still offers typing in a factura")
	}
	if rec := post(p.Factura, "/factura", doc("A-78", "2026-10-09")); rec.Code != http.StatusConflict {
		t.Errorf("Factura twice status = %d, want 409", rec.Code)
	}
	if rec := post(p.Pago, "/pago", doc("CP-9", "2026-10-20")); rec.Code != http.StatusSeeOther {
		t.Fatalf("Pago status = %d; body = %s", rec.Code, rec.Body.String())
	}
	body = page()
	for _, want := range []string{"Pagado", "CP-9", "20/10/2026", "Pago completado · comprobante de pago CP-9 · 20/10/2026"} {
		if !strings.Contains(body, want) {
			t.Errorf("paid page missing %q", want)
		}
	}
	if rec := post(p.Pago, "/pago", doc("CP-10", "2026-10-21")); rec.Code != http.StatusConflict {
		t.Errorf("Pago twice status = %d, want 409", rec.Code)
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
