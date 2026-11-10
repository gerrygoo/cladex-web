package web

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gerrygoo/cladex-web/internal/store"
)

// confirmPO generates and confirms a supplier O.C. for the proyecto's quote lines
// partidas (1-based), numbered number with seq, through the store.
func confirmPO(t *testing.T, a *Auth, folio string, userID int64, partidas []int, number string, seq int64) int64 {
	t.Helper()
	ctx := context.Background()
	p, err := a.store.ProjectByFolio(ctx, folio)
	if err != nil || p == nil {
		t.Fatalf("ProjectByFolio: %v", err)
	}
	quoteLines, err := a.store.ListQuoteLines(ctx, p.CurrentQuoteID)
	if err != nil {
		t.Fatal(err)
	}
	var lines []store.PurchaseOrderLine
	for _, n := range partidas {
		l, ok := store.LineFromQuote(quoteLines[n-1], false)
		if !ok {
			t.Fatalf("partida %d has no cost", n)
		}
		lines = append(lines, l)
	}
	id, err := a.store.CreatePurchaseOrder(ctx, p.ID, p.CurrentQuoteID, userID, true, store.PODraft{Lines: lines})
	if err != nil {
		t.Fatalf("CreatePurchaseOrder: %v", err)
	}
	if err := a.store.ConfirmPurchaseOrder(ctx, id, store.PODraft{Lines: lines}, store.POConfirmation{
		Number: number, Seq: seq, ConfirmedAt: "2026-10-30T18:00:00Z", Document: "{}", UserID: userID}); err != nil {
		t.Fatalf("ConfirmPurchaseOrder: %v", err)
	}
	return id
}

// TestEntrega is issue #66 on QA0037's example: the Estado de entrega in the summary and
// the Entrega section, marking an O.C. delivered with and without a comprobante, adding it
// later, an admin's undo, cancelling the O.C., two O.C.s ("1 de 2 entregadas") and
// Cerrado · Ganado.
func TestEntrega(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	p := NewProjects(a.store)
	h := NewPurchaseOrders(a.store)
	ana := createTestUser(t, a, "ana", "vendedor", "hunter2")
	carlos := createTestUser(t, a, "carlos", "vendedor", "hunter2")
	beto := createTestUser(t, a, "beto", "admin", "hunter2")
	customerID, _, _ := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, ana)
	ctx := context.Background()
	folio := qa0037(t, a, q, ana, customerID)
	pv := map[string]string{"folio": folio}
	base := "/proyectos/" + folio
	page := func(userID int64) string {
		return doForm(t, a, userID, p.Page, "GET", base, pv, nil, false).Body.String()
	}
	estado := func(userID int64) string {
		t.Helper()
		body := page(userID)
		i := strings.Index(body, "<dt>Estado de entrega</dt><dd>")
		if i < 0 {
			return ""
		}
		body = body[i+len("<dt>Estado de entrega</dt><dd>"):]
		return body[:strings.Index(body, "</dd>")]
	}
	post := func(userID int64, handler http.HandlerFunc, poID int64, action string, fields map[string]string, file string, data []byte) *httptest.ResponseRecorder {
		t.Helper()
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		for k, v := range fields {
			mw.WriteField(k, v)
		}
		if file != "" {
			fw, _ := mw.CreateFormFile("archivo", file)
			fw.Write(data)
		}
		mw.Close()
		id := strconv.FormatInt(poID, 10)
		req := httptest.NewRequest("POST", base+"/ordenes-de-compra/"+id+"/"+action, &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.SetPathValue("folio", folio)
		req.SetPathValue("id", id)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
		rec := httptest.NewRecorder()
		a.RequireAuth(handler).ServeHTTP(rec, req)
		return rec
	}
	admin := func(userID int64, handler http.HandlerFunc, poID int64, action string, form url.Values) *httptest.ResponseRecorder {
		id := strconv.FormatInt(poID, 10)
		return doForm(t, a, userID, adminOnly(a, handler), "POST", base+"/ordenes-de-compra/"+id+"/"+action,
			map[string]string{"folio": folio, "id": id}, form, false)
	}
	pdf := []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n")
	today := time.Now().UTC().Format("2006-01-02")

	// En curso without a confirmed O.C.: Sin emitir O.C., and no Entrega section.
	receiveClientOC(t, a, folio, ana)
	if got := estado(ana); got != "Sin emitir O.C." {
		t.Errorf("Estado de entrega = %q", got)
	}
	if strings.Contains(page(ana), `id="entrega"`) {
		t.Error("the Entrega section shows without a confirmed O.C.")
	}

	// Confirmed: En proceso de entrega with the O.C.'s number and link, and the section.
	oc := confirmPO(t, a, folio, beto, []int{1, 2, 3, 4, 5, 6}, "OC-"+folio+"-301026", 1)
	ocPath := base + "/ordenes-de-compra/" + strconv.FormatInt(oc, 10)
	if got := estado(ana); got != `En proceso de entrega · <a href="`+ocPath+`">OC-`+folio+`-301026</a>` {
		t.Errorf("Estado de entrega = %q", got)
	}
	body := page(ana)
	for _, want := range []string{`<section id="entrega"><h2>Entrega</h2>`, "Marcar como entregado", "Fecha de entrega",
		`name="fecha" value="` + today[:4], "¿Quieres subir un comprobante de entrega?", `name="comprobante" value="no" checked`,
		"<label data-comprobante-file hidden>", "Comprobante de entrega (PDF, JPG o PNG de hasta 10 MB)"} {
		if !strings.Contains(body, want) {
			t.Errorf("en proceso page missing %q", want)
		}
	}
	if i, j, k := strings.Index(body, `id="oc-proveedor"`), strings.Index(body, `id="entrega"`), strings.Index(body, `id="cobro"`); !(i < j && j < k) {
		t.Error("Entrega isn't between the supplier O.C.s and Cobro")
	}
	// Another vendedor sees it but can't mark it.
	if body := page(carlos); strings.Contains(body, "Marcar como entregado") || !strings.Contains(body, "El vendedor del proyecto o un administrador marca la entrega.") {
		t.Error("another vendedor is offered Marcar como entregado")
	}
	if rec := post(carlos, h.Entregar, oc, "entregar", map[string]string{"fecha": "2026-11-05"}, "", nil); rec.Code != http.StatusForbidden {
		t.Errorf("Entregar(another vendedor) = %d", rec.Code)
	}
	for name, c := range map[string]struct {
		fields map[string]string
		file   string
		data   []byte
		want   string
	}{
		"no date":          {map[string]string{"comprobante": "no"}, "", nil, "escribe la fecha de entrega"},
		"future":           {map[string]string{"fecha": "2099-01-01"}, "", nil, "no puede ser futura"},
		"sí without file":  {map[string]string{"fecha": "2026-11-05", "comprobante": "si"}, "", nil, "adjunta el comprobante de entrega"},
		"sí with a .txt":   {map[string]string{"fecha": "2026-11-05", "comprobante": "si"}, "e.txt", []byte("hola"), "debe ser PDF, JPG o PNG"},
		"unknown comprob.": {map[string]string{"fecha": "2026-11-05", "comprobante": "quizá"}, "", nil, "elige si quieres subir"},
	} {
		if rec := post(ana, h.Entregar, oc, "entregar", c.fields, c.file, c.data); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), c.want) {
			t.Errorf("Entregar(%s) = %d %q, want 400 %q", name, rec.Code, rec.Body.String(), c.want)
		}
	}

	// Marked with No: Entregado with its day, no comprobante, in the history.
	rec := post(ana, h.Entregar, oc, "entregar", map[string]string{"fecha": "2026-11-05", "comprobante": "no"}, "", nil)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != base+"#entrega-"+strconv.FormatInt(oc, 10) {
		t.Fatalf("Entregar = %d %q: %s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	if got := estado(ana); got != "Entregado · 05/11/2026" {
		t.Errorf("Estado de entrega = %q", got)
	}
	body = page(ana)
	for _, want := range []string{"<dd>Entregado · 05/11/2026</dd>", "<dt>Marcó</dt><dd>ana · ", "Sin comprobante",
		"Marcado como entregado el 05/11/2026 por ana: orden de compra OC-" + folio + "-301026.", "<summary>Subir comprobante de entrega"} {
		if !strings.Contains(body, want) {
			t.Errorf("delivered page missing %q", want)
		}
	}
	if strings.Contains(body, "Marcar como entregado<") || strings.Contains(body, "Regresar a En proceso de entrega") {
		t.Error("the vendedor is offered marking again or the admin's undo")
	}
	if rec := post(ana, h.Entregar, oc, "entregar", map[string]string{"fecha": "2026-11-05"}, "", nil); rec.Code != http.StatusConflict {
		t.Errorf("Entregar(twice) = %d", rec.Code)
	}

	// The comprobante, added later: required, then "Ver comprobante" in the summary.
	if rec := post(ana, h.ComprobanteEntrega, oc, "comprobante-entrega", nil, "", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("ComprobanteEntrega(no file) = %d", rec.Code)
	}
	if rec := post(carlos, h.ComprobanteEntrega, oc, "comprobante-entrega", nil, "remision.pdf", pdf); rec.Code != http.StatusForbidden {
		t.Errorf("ComprobanteEntrega(another vendedor) = %d", rec.Code)
	}
	if rec := post(ana, h.ComprobanteEntrega, oc, "comprobante-entrega", nil, "remision.pdf", pdf); rec.Code != http.StatusSeeOther {
		t.Fatalf("ComprobanteEntrega = %d: %s", rec.Code, rec.Body.String())
	}
	ds, _ := a.store.ListDeliveries(ctx, mustProjectID(t, a, folio))
	fileLink := `<a href="` + base + `/archivos/` + strconv.FormatInt(ds[0].FileID, 10) + `">`
	if got := estado(ana); got != "Entregado · 05/11/2026 · "+fileLink+"Ver comprobante</a>" {
		t.Errorf("Estado de entrega = %q", got)
	}
	if body := page(ana); !strings.Contains(body, fileLink+"remision.pdf</a>") || !strings.Contains(body, "Comprobante de entrega agregado por ana: orden de compra OC-"+folio+"-301026.") ||
		strings.Contains(body, "Subir comprobante de entrega") {
		t.Error("the comprobante added later isn't shown, or is still asked for")
	}
	if rec := post(ana, h.ComprobanteEntrega, oc, "comprobante-entrega", nil, "otra.pdf", pdf); rec.Code != http.StatusConflict {
		t.Errorf("ComprobanteEntrega(twice) = %d", rec.Code)
	}

	// Delivered, an admin can't cancel it; the O.C.'s page says so.
	poPage := func(userID int64) string {
		return doForm(t, a, userID, h.Page, "GET", ocPath, map[string]string{"folio": folio, "id": strconv.FormatInt(oc, 10)}, nil, false).Body.String()
	}
	if body := poPage(beto); !strings.Contains(body, "No se puede cancelar: ya está entregada.") || strings.Contains(body, "Cancelar orden de compra</button>") ||
		!strings.Contains(body, "<dt>Estado de entrega</dt><dd>Entregado · 05/11/2026") {
		t.Error("the O.C.'s page offers cancelling a delivered O.C.")
	}
	if rec := admin(beto, h.Cancelar, oc, "cancelar", url.Values{"motivo": {"Otro proveedor"}}); rec.Code != http.StatusConflict {
		t.Errorf("Cancelar(delivered) = %d", rec.Code)
	}

	// An admin takes it back to En proceso, with a motivo.
	if !strings.Contains(page(beto), "<summary>Regresar a En proceso de entrega") {
		t.Error("an admin isn't offered the undo")
	}
	if rec := admin(beto, h.DeshacerEntrega, oc, "deshacer-entrega", url.Values{"reason": {" "}}); rec.Code != http.StatusBadRequest {
		t.Errorf("DeshacerEntrega(no motivo) = %d", rec.Code)
	}
	if rec := admin(beto, h.DeshacerEntrega, oc, "deshacer-entrega", url.Values{"reason": {"Se marcó antes de tiempo"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("DeshacerEntrega = %d: %s", rec.Code, rec.Body.String())
	}
	body = page(ana)
	if !strings.Contains(body, "Se regresó la orden de compra OC-"+folio+"-301026 a En proceso de entrega.\nMotivo: Se marcó antes de tiempo") ||
		!strings.Contains(estado(ana), "En proceso de entrega") {
		t.Error("the undo isn't in the history or the state")
	}

	// Marked with Sí and its file: Ver comprobante straight away.
	if rec := post(beto, h.Entregar, oc, "entregar", map[string]string{"fecha": today, "comprobante": "si"}, "acuse.png", []byte("\x89PNG\r\n\x1a\n0000")); rec.Code != http.StatusSeeOther {
		t.Fatalf("Entregar(sí) = %d: %s", rec.Code, rec.Body.String())
	}
	todayText := today[8:10] + "/" + today[5:7] + "/" + today[:4]
	if got := estado(ana); !strings.HasPrefix(got, "Entregado · "+todayText) || !strings.Contains(got, "Ver comprobante") {
		t.Errorf("Estado de entrega = %q", got)
	}
	if !strings.Contains(page(ana), "Marcado como entregado el "+todayText+" por beto con comprobante: orden de compra OC-"+folio+"-301026.") {
		t.Error("the history doesn't say it came with a comprobante")
	}

	// Undone and cancelled by an admin: Cancelada with its motivo, and back to Sin emitir O.C.
	admin(beto, h.DeshacerEntrega, oc, "deshacer-entrega", url.Values{"reason": {"Cambio de proveedor"}})
	if body := poPage(ana); strings.Contains(body, "Cancelar orden de compra") {
		t.Error("a vendedor is offered cancelling the O.C.")
	}
	if rec := admin(beto, h.Cancelar, oc, "cancelar", url.Values{"motivo": {""}}); rec.Code != http.StatusBadRequest {
		t.Errorf("Cancelar(no motivo) = %d", rec.Code)
	}
	rec = admin(beto, h.Cancelar, oc, "cancelar", url.Values{"motivo": {"El proveedor no tiene existencia"}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != ocPath+"?cancelada=1" {
		t.Fatalf("Cancelar = %d %q: %s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	body = doForm(t, a, beto, h.Page, "GET", ocPath+"?cancelada=1", map[string]string{"folio": folio, "id": strconv.FormatInt(oc, 10)}, nil, false).Body.String()
	for _, want := range []string{"Orden de compra cancelada.", "Cancelada", "<dt>Motivo de la cancelación</dt><dd class=\"comment-body\">El proveedor no tiene existencia</dd>"} {
		if !strings.Contains(body, want) {
			t.Errorf("cancelled O.C. page missing %q", want)
		}
	}
	if got := estado(ana); got != "Sin emitir O.C." {
		t.Errorf("Estado de entrega after cancelling = %q", got)
	}
	body = page(ana)
	if strings.Contains(body, `id="entrega"`) || !strings.Contains(body, "Se canceló la orden de compra OC-"+folio+"-301026.\nMotivo: El proveedor no tiene existencia") ||
		!strings.Contains(body, "Generar orden de compra") {
		t.Error("after cancelling: the section stays, the history lacks it, or no new O.C. can be generated")
	}

	// Two O.C.s: 0 de 2, 1 de 2, then Entregado with the last day and no single comprobante.
	a1 := confirmPO(t, a, folio, beto, []int{1, 2, 3}, "OC-"+folio+"-301026-2", 2)
	a2 := confirmPO(t, a, folio, beto, []int{4, 5, 6}, "OC-"+folio+"-301026-3", 3)
	if got := estado(ana); got != "En proceso de entrega · 0 de 2 entregadas" {
		t.Errorf("two O.C.s = %q", got)
	}
	post(ana, h.Entregar, a1, "entregar", map[string]string{"fecha": "2026-11-06", "comprobante": "si"}, "a1.pdf", pdf)
	if got := estado(ana); got != "En proceso de entrega · 1 de 2 entregadas" {
		t.Errorf("one of two = %q", got)
	}
	post(ana, h.Entregar, a2, "entregar", map[string]string{"fecha": "2026-11-04"}, "", nil)
	if got := estado(ana); got != "Entregado · 06/11/2026" {
		t.Errorf("both = %q", got)
	}

	// Facturada, entregado and pagado: Cerrado · Ganado, hidden from the list unless its
	// Etapa filter asks for it.
	project, _ := a.store.ProjectByFolio(ctx, folio)
	if err := a.store.RecordInvoice(ctx, project.ID, ana, store.Document{Ref: "F-1", Date: "2026-11-06"}, "PUE", ""); err != nil {
		t.Fatal(err)
	}
	if err := a.store.RecordPayment(ctx, project.ID, ana, project.Total, "2026-11-07", nil); err != nil {
		t.Fatal(err)
	}
	if body := page(ana); !strings.Contains(body, `<span class="badge badge-cerrado">Cerrado · Ganado</span>`) {
		t.Error("the proyecto isn't Cerrado · Ganado")
	}
	list := func(query string) string {
		return doForm(t, a, ana, p.List, "GET", "/proyectos?"+query, nil, nil, false).Body.String()
	}
	if strings.Contains(list(""), `href="/proyectos/`+folio+`"`) {
		t.Error("the default list shows a Cerrado · Ganado proyecto")
	}
	if !strings.Contains(list("f.etapa=cerrado"), `href="/proyectos/`+folio+`"`) {
		t.Error("the Etapa filter doesn't find the Cerrado · Ganado proyecto")
	}
}
