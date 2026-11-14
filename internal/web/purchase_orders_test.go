package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/pdf"
	"github.com/gerrygoo/cladex-web/internal/pricing"
	"github.com/gerrygoo/cladex-web/internal/store"
)

// qa0037 issues the issue's QA0037 example as a QA quote: six partidas of 200 m of
// Conduccasa cable whose costs are $5.54, $7.77 and $12.30, and receives the client's
// O.C., so the proyecto takes supplier O.C.s. It returns the proyecto's folio.
func qa0037(t *testing.T, a *Auth, q *Quotes, userID, customerID int64) string {
	t.Helper()
	ctx := context.Background()
	familyID, err := a.store.UpsertFamily(ctx, "CCA", "")
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"line_keys": {"0,1,2,3,4,5"}}
	for i, p := range []struct{ desc, cost string }{
		{"CABLE THW/THHW CONDUCCASA CALIBRE 14 AWG verde", "5.54"},
		{"CABLE THW/THHW CONDUCCASA CALIBRE 14 AWG blanco", "5.54"},
		{"CABLE THW/THHW CONDUCCASA CALIBRE 12 AWG negro", "7.77"},
		{"CABLE THW/THHW CONDUCCASA CALIBRE 12 AWG blanco", "7.77"},
		{"CABLE THW/THHW CONDUCCASA CALIBRE 10 AWG negro", "12.30"},
		{"CABLE THW/THHW CONDUCCASA CALIBRE 10 AWG blanco", "12.30"},
	} {
		cost, _ := money.ParseMicros(p.cost)
		id, err := a.store.CreateProduct(ctx, store.Product{FamilyID: familyID, SKU: "CCA-" + strconv.Itoa(i),
			Description: p.desc, CostMicros: &cost})
		if err != nil {
			t.Fatal(err)
		}
		k := strconv.Itoa(i)
		form.Set("lines["+k+"][kind]", "product")
		form.Set("lines["+k+"][product_id]", strconv.FormatInt(id, 10))
		form.Set("lines["+k+"][qty]", "200")
	}
	draft, err := a.store.CreateDraftQuote(ctx, customerID, userID, "QA")
	if err != nil {
		t.Fatal(err)
	}
	pv := map[string]string{"folio": draft.Folio}
	if rec := doForm(t, a, userID, q.Emitir, "POST", "/cotizaciones/"+draft.Folio+"/emitir", pv, form, false); rec.Code != http.StatusSeeOther {
		t.Fatalf("Emitir = %d: %s", rec.Code, rec.Body.String())
	}
	return draft.Folio
}

func receiveClientOC(t *testing.T, a *Auth, folio string, userID int64) {
	t.Helper()
	p, err := a.store.ProjectByFolio(context.Background(), folio)
	if err != nil || p == nil {
		t.Fatalf("ProjectByFolio: %v", err)
	}
	if err := a.store.ReceiveOC(context.Background(), p.ID, userID, store.OC{Number: "4411", Date: "2026-10-07"}, nil, ""); err != nil {
		t.Fatalf("ReceiveOC: %v", err)
	}
}

// adminOnly wraps h the way the router does for the admin routes.
func adminOnly(a *Auth, h http.HandlerFunc) http.HandlerFunc {
	return a.RequireAdmin(h).ServeHTTP
}

func poFormValues(supplierID string, partidas []string, extra map[string]string) url.Values {
	v := url.Values{"proveedor": {supplierID}, "moneda": {"MXN"}, "forma_pago": {store.POPayTransfer}, "uso_cfdi": {"G01"}, "partida": partidas}
	for k, x := range extra {
		v.Set(k, x)
	}
	return v
}

func allPartidas(n int) []string {
	var out []string
	for i := 1; i <= n; i++ {
		out = append(out, strconv.Itoa(i))
	}
	return out
}

func TestPurchaseOrdersQA0037(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	p := NewProjects(a.store)
	h := NewPurchaseOrders(a.store)
	sp := NewSuppliers(a.store)
	ctx := context.Background()
	ana := createTestUser(t, a, "ana", "vendedor", "hunter2")
	carlos := createTestUser(t, a, "carlos", "vendedor", "hunter2")
	beto := createTestUser(t, a, "beto", "admin", "hunter2")
	customerID, _, _ := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, ana)
	folio := qa0037(t, a, q, ana, customerID)
	pv := map[string]string{"folio": folio}
	base := "/proyectos/" + folio
	page := func(userID int64) string {
		return doForm(t, a, userID, p.Page, "GET", base, pv, nil, false).Body.String()
	}
	generate := func(userID int64) *httptest.ResponseRecorder {
		return doForm(t, a, userID, h.Generar, "POST", base+"/ordenes-de-compra", pv, url.Values{}, false)
	}

	// Not before the client's O.C.
	if strings.Contains(page(ana), "Orden de compra al proveedor") {
		t.Error("a prospecto shows the supplier O.C. section")
	}
	if rec := generate(ana); rec.Code != http.StatusConflict {
		t.Fatalf("Generar on a prospecto = %d", rec.Code)
	}
	receiveClientOC(t, a, folio, ana)
	// The catalog changes after the quote: the O.C. still buys at the quoted cost.
	products, _ := a.store.ListProducts(ctx, "CCA-0", "", "", nil)
	repriced := money.Micros(9_990_000)
	products[0].CostMicros = &repriced
	if err := a.store.UpdateProduct(ctx, products[0]); err != nil {
		t.Fatal(err)
	}

	body := page(ana)
	for _, want := range []string{`id="oc-proveedor"`, "Orden de compra al proveedor", "todavía no tiene orden de compra", "Generar orden de compra"} {
		if !strings.Contains(body, want) {
			t.Errorf("proyecto page lacks %q", want)
		}
	}
	if body := page(carlos); strings.Contains(body, "Generar orden de compra") ||
		!strings.Contains(body, "El vendedor del proyecto o un administrador genera la orden de compra.") {
		t.Error("another vendedor is offered Generar")
	}
	if rec := generate(carlos); rec.Code != http.StatusForbidden {
		t.Fatalf("Generar by another vendedor = %d", rec.Code)
	}
	rec := generate(ana)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Generar = %d: %s", rec.Code, rec.Body.String())
	}
	poPath := rec.Header().Get("Location")
	id, _ := strconv.ParseInt(poPath[strings.LastIndex(poPath, "/")+1:], 10, 64)
	ppv := map[string]string{"folio": folio, "id": strconv.FormatInt(id, 10)}
	if rec := generate(ana); rec.Code != http.StatusConflict {
		t.Fatalf("a second O.C. on QA = %d", rec.Code)
	}
	po, _ := a.store.PurchaseOrderByID(ctx, id)
	if po.SupplierName != "Cable Anti Robo" || po.Total.String() != "$11,883.04" {
		t.Fatalf("generated O.C. = %s, %s", po.SupplierName, po.Total)
	}

	poPage := func(userID int64) string {
		return doForm(t, a, userID, h.Page, "GET", poPath, ppv, nil, false).Body.String()
	}
	body = poPage(ana)
	for _, want := range []string{"Borrador", "CABLE THW/THHW CONDUCCASA CALIBRE 14 AWG verde", "$5.54", "$1,108.00",
		"$10,244.00", "$1,639.04", "$11,883.04", "selected>Cable Anti Robo", "Guardar borrador", "Ver PDF",
		"Un administrador confirma la orden de compra y la descarga para mandarla al proveedor.", "Transferencia electrónica por adelantado",
		"CLADEX S.A. de C.V.", "CLA260326Q19", "601 - General de Ley Personas Morales",
		// The uso de CFDI is chosen per O.C., starting from Configuración's.
		`name="uso_cfdi"`, `<option value="G01" selected>G01 - Adquisición de mercancías`} {
		if !strings.Contains(body, want) {
			t.Errorf("draft page lacks %q", want)
		}
	}
	// Never the sale price: $5.54 at the 30% margin sells for $7.91.
	for _, gone := range []string{"Confirmar y descargar", "$7.91", "$9.99"} {
		if strings.Contains(body, gone) {
			t.Errorf("vendedor's draft page has %q", gone)
		}
	}
	if body := poPage(carlos); strings.Contains(body, "Guardar borrador") || !strings.Contains(body, "Solo el vendedor del proyecto o un administrador edita este borrador.") {
		t.Error("another vendedor can edit the draft")
	}
	if rec := doForm(t, a, carlos, h.Guardar, "POST", poPath, ppv, poFormValues("", nil, nil), false); rec.Code != http.StatusForbidden {
		t.Fatalf("Guardar by another vendedor = %d", rec.Code)
	}

	sup := strconv.FormatInt(*po.SupplierID, 10)
	six := allPartidas(6)
	// The vendedor can save, but not confirm.
	if rec := doForm(t, a, ana, h.Guardar, "POST", poPath, ppv, poFormValues(sup, six, map[string]string{"dias_credito": "treinta"}), false); rec.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rec.Body.String(), "Los días de crédito son un número entero, p. ej. 30.") {
		t.Fatalf("bad días de crédito = %d", rec.Code)
	}
	if rec := doForm(t, a, ana, h.Guardar, "POST", poPath, ppv, poFormValues(sup, six, map[string]string{"forma_pago": store.POPayCredit}), false); rec.Code != http.StatusSeeOther {
		t.Fatalf("Guardar = %d: %s", rec.Code, rec.Body.String())
	}
	if body := doForm(t, a, ana, h.Page, "GET", poPath+"?guardado=1", ppv, nil, false).Body.String(); !strings.Contains(body, "Borrador guardado.") ||
		!strings.Contains(body, `name="dias_credito" id="dias_credito" value=""`) {
		t.Error("Crédito without the supplier's días de crédito isn't left empty")
	}
	confirm := func(userID int64, form url.Values) *httptest.ResponseRecorder {
		return doForm(t, a, userID, adminOnly(a, h.Confirmar), "POST", poPath+"/confirmar", ppv, form, false)
	}
	if rec := confirm(ana, poFormValues(sup, six, nil)); rec.Code != http.StatusForbidden {
		t.Fatalf("a vendedor confirmed: %d", rec.Code)
	}

	// The admin can't confirm without a delivery address, nor on Crédito without days.
	for _, c := range []struct {
		form url.Values
		want string
	}{
		{poFormValues(sup, six, nil), "Escribe el domicilio de entrega."},
		{poFormValues(sup, six, map[string]string{"domicilio": "Bodega", "forma_pago": store.POPayCredit}), "Escribe los días de crédito."},
		{poFormValues(sup, six, map[string]string{"domicilio": "Bodega", "forma_pago": store.POPayOther}), "Escribe la forma de pago."},
		{poFormValues("", six, map[string]string{"domicilio": "Bodega"}), "Elige el proveedor."},
		{poFormValues(sup, nil, map[string]string{"domicilio": "Bodega"}), "Elige al menos una partida."},
	} {
		rec := confirm(beto, c.form)
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), c.want) {
			t.Errorf("confirm without it = %d, want %q", rec.Code, c.want)
		}
	}
	if po, _ := a.store.PurchaseOrderByID(ctx, id); po.Status != store.POBorrador {
		t.Fatal("a refused confirmation confirmed")
	}

	// Without Cladex's billing data it can't be confirmed either.
	if err := a.store.SetSetting(ctx, "oc_domicilio_fiscal", "", beto); err != nil {
		t.Fatal(err)
	}
	if rec := confirm(beto, poFormValues(sup, six, map[string]string{"domicilio": "Bodega"})); rec.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rec.Body.String(), "Faltan datos de facturación de Cladex") {
		t.Fatalf("confirm without billing data = %d", rec.Code)
	}
	if err := a.store.SetSetting(ctx, "oc_domicilio_fiscal", "Calle Circuito Valle del Silencio 143, Col. Lomas de Valle Escondido, C.P. 52930, Atizapán de Zaragoza, Estado de México", beto); err != nil {
		t.Fatal(err)
	}
	if body := poPage(beto); !strings.Contains(body, "Confirmar y descargar") ||
		!strings.Contains(body, "¿Confirmar la orden de compra? Se le asigna su número con la fecha de hoy y ya no se puede editar.") {
		t.Error("the admin isn't offered Confirmar y descargar, or it doesn't ask first")
	}
	pdraft, _ := a.store.PurchaseOrderByID(ctx, id)
	project, _ := a.store.ProjectByFolio(ctx, folio)
	if preview, err := h.previewDocument(ctx, project, pdraft); err != nil || preview.Number != "Se asigna al confirmar" || len(preview.Lines) != 6 {
		t.Fatalf("preview = %+v, %v", preview, err)
	}

	// The preview is a PDF.
	if rec := doForm(t, a, carlos, h.PDF, "GET", poPath+"/pdf", ppv, nil, false); rec.Code != http.StatusOK ||
		rec.Header().Get("Content-Type") != "application/pdf" || !strings.Contains(rec.Header().Get("Content-Disposition"), "inline") {
		t.Fatalf("draft PDF = %d %v", rec.Code, rec.Header())
	}
	if rec := doForm(t, a, ana, h.VerPDF, "POST", poPath+"/ver-pdf", ppv, poFormValues(sup, six, nil), false); rec.Code != http.StatusSeeOther ||
		rec.Header().Get("Location") != poPath+"/pdf" {
		t.Fatalf("Ver PDF = %d → %s", rec.Code, rec.Header().Get("Location"))
	}

	// Without a uso de CFDI it isn't confirmed.
	if rec := confirm(beto, poFormValues(sup, six, map[string]string{"domicilio": "Obra", "uso_cfdi": ""})); rec.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rec.Body.String(), "Elige el uso de CFDI con que el proveedor debe facturar.") {
		t.Errorf("Confirmar without uso de CFDI = %d", rec.Code)
	}
	// Confirmed with "Otra": numbered with today's date in México, its text printed as typed,
	// and this O.C.'s own uso de CFDI (G03), not Configuración's.
	rec = confirm(beto, poFormValues(sup, six, map[string]string{"domicilio": "Av. Industria 10, Querétaro", "uso_cfdi": "G03",
		"referencias": "Andén 3", "moneda": "USD", "forma_pago": store.POPayOther, "forma_pago_otra": "50% anticipo y 50% contra entrega."}))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != poPath+"?confirmada=1" {
		t.Fatalf("Confirmar = %d: %s", rec.Code, rec.Body.String())
	}
	number := "OC-" + folio + "-" + store.InMexicoCity(time.Now()).Format("020106")
	po, _ = a.store.PurchaseOrderByID(ctx, id)
	if po.Status != store.POConfirmada || po.Number != number || po.PDFSHA256 == "" {
		t.Fatalf("confirmed O.C. = %+v", po)
	}
	var doc pdf.PurchaseOrderDocument
	if err := json.Unmarshal([]byte(po.Document), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Number != number || doc.Fecha != store.InMexicoCity(time.Now()).Format("02/01/2006") || doc.Vendedor != "ana" ||
		doc.SupplierName != "CABLE ANTI ROBO" || doc.Issuer != "CLADEX S.A. de C.V. · RFC CLA260326Q19" ||
		doc.DeliveryAddress != "Av. Industria 10, Querétaro" || doc.DeliveryRefs != "Andén 3" || doc.ShowKg ||
		len(doc.Lines) != 6 || doc.Lines[0].UnitPrice != "$5.54" || doc.Lines[0].Qty != "200" || doc.Lines[0].Total != "$1,108.00" ||
		doc.Lines[4].UnitPrice != "$12.30" || doc.Subtotal != "$10,244.00" || doc.IVA != "$1,639.04" || doc.Total != "$11,883.04" ||
		doc.BillingCFDIUse != "G03 - Gastos en general" || po.CFDIUse != "G03" {
		t.Fatalf("frozen document = %+v", doc)
	}
	if strings.Join(doc.Conditions, "|") != "Precios en dólares americanos (USD), más IVA.|50% anticipo y 50% contra entrega.|"+
		"La mercancía se recibe sujeta a revisión de cantidad y calidad.|Favor de emitir el CFDI con los datos de facturación indicados en esta orden." {
		t.Fatalf("conditions = %q", doc.Conditions)
	}
	body = doForm(t, a, beto, h.Page, "GET", poPath+"?confirmada=1", ppv, nil, false).Body.String()
	for _, want := range []string{"Orden de compra confirmada: " + number + ".", "data-auto-download", "descarga el PDF aquí", poPath + "/pdf?descargar=1", "Crear revisión", "Confirmada"} {
		if !strings.Contains(body, want) {
			t.Errorf("confirmed page lacks %q", want)
		}
	}
	rec = doForm(t, a, ana, h.PDF, "GET", poPath+"/pdf?descargar=1", ppv, nil, false)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Disposition") != `attachment; filename=`+number+`.pdf` {
		t.Fatalf("confirmed PDF = %d %q", rec.Code, rec.Header().Get("Content-Disposition"))
	}
	// Confirmed is not editable.
	if rec := doForm(t, a, beto, h.Guardar, "POST", poPath, ppv, poFormValues(sup, six, nil), false); rec.Code != http.StatusConflict {
		t.Fatalf("Guardar on a confirmed O.C. = %d", rec.Code)
	}
	if rec := doForm(t, a, beto, h.Eliminar, "POST", poPath+"/eliminar", ppv, url.Values{}, false); rec.Code != http.StatusConflict {
		t.Fatalf("Eliminar on a confirmed O.C. = %d", rec.Code)
	}
	if body := page(ana); !strings.Contains(body, number) || strings.Contains(body, "Generar orden de compra") {
		t.Error("the proyecto doesn't list the confirmed O.C., or still offers another")
	}

	// A revision: admin only, with a motivo that goes into the history.
	revise := func(userID int64, motivo string) *httptest.ResponseRecorder {
		return doForm(t, a, userID, adminOnly(a, h.Revision), "POST", poPath+"/revision", ppv, url.Values{"motivo": {motivo}}, false)
	}
	if rec := revise(ana, "x"); rec.Code != http.StatusForbidden {
		t.Fatalf("a vendedor revised: %d", rec.Code)
	}
	if rec := revise(beto, " "); rec.Code != http.StatusBadRequest {
		t.Fatalf("a revision without motivo = %d", rec.Code)
	}
	rec = revise(beto, "Se corrigió la dirección de entrega")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Revision = %d", rec.Code)
	}
	revPath := strings.TrimSuffix(rec.Header().Get("Location"), "?revision=1")
	revID, _ := strconv.ParseInt(revPath[strings.LastIndex(revPath, "/")+1:], 10, 64)
	rpv := map[string]string{"folio": folio, "id": strconv.FormatInt(revID, 10)}
	if body := page(ana); !strings.Contains(body, "Revisión de la orden de compra "+number+".\nMotivo: Se corrigió la dirección de entrega") {
		t.Error("the motivo isn't in the history")
	}
	rec = doForm(t, a, beto, adminOnly(a, h.Confirmar), "POST", revPath+"/confirmar", rpv,
		poFormValues(sup, six, map[string]string{"domicilio": "Av. Industria 12, Querétaro"}), false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("confirm revision = %d: %s", rec.Code, rec.Body.String())
	}
	rev, _ := a.store.PurchaseOrderByID(ctx, revID)
	orig, _ := a.store.PurchaseOrderByID(ctx, id)
	if rev.Number != number+"-R1" || orig.Status != store.POReemplazada {
		t.Fatalf("revision %q, original %q", rev.Number, orig.Status)
	}
	body = page(ana)
	for _, want := range []string{number + "-R1", "Reemplazada", "Confirmada"} {
		if !strings.Contains(body, want) {
			t.Errorf("proyecto page lacks %q", want)
		}
	}
	// The replaced one is still there to see and download.
	if rec := doForm(t, a, ana, h.PDF, "GET", poPath+"/pdf", ppv, nil, false); rec.Code != http.StatusOK {
		t.Fatalf("replaced O.C.'s PDF = %d", rec.Code)
	}

	// Its supplier can't be deleted now.
	sppv := map[string]string{"id": sup}
	rec = doForm(t, a, beto, sp.Delete, "POST", "/proveedores/"+sup+"/eliminar", sppv, url.Values{}, false)
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "eliminar=bloqueado") {
		t.Fatalf("deleting a supplier with O.C.s → %s", loc)
	}
	body = doForm(t, a, beto, sp.Page, "GET", "/proveedores/"+sup+"?eliminar=bloqueado", sppv, nil, false).Body.String()
	if !strings.Contains(body, "Tiene órdenes de compra: "+number+", "+number+"-R1. Un proveedor con órdenes de compra no se puede eliminar; márcalo como inactivo.") {
		t.Errorf("supplier page doesn't say why: %s", body)
	}
}

// QALC (priced by weight): the O.C. adds Kg por metro, Kg and Precio por kg, from the
// issue's Alumoclad 7#8 example.
func TestPurchaseOrderByWeight(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	h := NewPurchaseOrders(a.store)
	ctx := context.Background()
	admin := createTestUser(t, a, "beto", "admin", "hunter2")
	customerID, _, _ := seedQuoteBuilderFixtures(t, a)
	familyID, err := a.store.CreateFamily(ctx, "Alumoclad")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.CreateQuoteType(ctx, store.QuoteType{Prefix: "QALC", Name: "Alumoclad", FamilyID: &familyID,
		QuoteTypeBehavior: store.QuoteTypeBehavior{KgPriced: true, Letterhead: "alumoclad"}}); err != nil {
		t.Fatal(err)
	}
	cost, kg := money.Micros(95_000_000), money.Micros(389_600)
	alc, err := a.store.CreateProduct(ctx, store.Product{FamilyID: familyID, SKU: "ALC-C7#8",
		Description: "Alumoclad 7#8", CostMicros: &cost, KgPerMMicros: &kg})
	if err != nil {
		t.Fatal(err)
	}
	draft, err := a.store.CreateDraftQuote(ctx, customerID, admin, "QALC")
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"line_keys": {"0"}, "lines[0][kind]": {"product"},
		"lines[0][product_id]": {strconv.FormatInt(alc, 10)}, "lines[0][qty]": {"111"}}
	if rec := doForm(t, a, admin, q.Emitir, "POST", "/cotizaciones/"+draft.Folio+"/emitir", map[string]string{"folio": draft.Folio}, form, false); rec.Code != http.StatusSeeOther {
		t.Fatalf("Emitir = %d: %s", rec.Code, rec.Body.String())
	}
	receiveClientOC(t, a, draft.Folio, admin)
	pv := map[string]string{"folio": draft.Folio}
	rec := doForm(t, a, admin, h.Generar, "POST", "/proyectos/"+draft.Folio+"/ordenes-de-compra", pv, url.Values{}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Generar = %d", rec.Code)
	}
	poPath := rec.Header().Get("Location")
	id := poPath[strings.LastIndex(poPath, "/")+1:]
	ppv := map[string]string{"folio": draft.Folio, "id": id}
	body := doForm(t, a, admin, h.Page, "GET", poPath, ppv, nil, false).Body.String()
	for _, want := range []string{"Cantidad (m)", "Kg por metro", "Precio por kg", ">0.3896<", ">43.25<", ">$95.00<", ">$37.01<", ">$4,108.11<", `<option value="">Elige…</option>`} {
		if !strings.Contains(body, want) {
			t.Errorf("QALC draft lacks %q", want)
		}
	}
	// Alumoclad has no default supplier here: pick one, then confirm.
	sup, _ := a.store.SupplierByName(ctx, "Cable Anti Robo")
	rec = doForm(t, a, admin, h.Confirmar, "POST", poPath+"/confirmar", ppv,
		poFormValues(strconv.FormatInt(*sup, 10), []string{"1"}, map[string]string{"domicilio": "Obra"}), false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Confirmar = %d: %s", rec.Code, rec.Body.String())
	}
	n, _ := strconv.ParseInt(id, 10, 64)
	po, _ := a.store.PurchaseOrderByID(ctx, n)
	var doc pdf.PurchaseOrderDocument
	json.Unmarshal([]byte(po.Document), &doc)
	if !doc.ShowKg || len(doc.Lines) != 1 || doc.Lines[0] != (pdf.PurchaseOrderLineDoc{Description: "Alumoclad 7#8", Qty: "111",
		KgPerUnit: "0.3896", Kg: "43.25", PricePerKg: "$95.00", UnitPrice: "$37.01", Total: "$4,108.11"}) {
		t.Fatalf("QALC document = %+v", doc)
	}
}

// QL: the supplier of the "Cotización del proveedor" comes preselected, an admin can add
// a new supplier from the O.C., and a second O.C. with the other partidas gets -2.
func TestPurchaseOrdersQL(t *testing.T) {
	a := newTestAuth(t)
	h := NewPurchaseOrders(a.store)
	ctx := context.Background()
	ana := createTestUser(t, a, "ana", "vendedor", "hunter2")
	beto := createTestUser(t, a, "beto", "admin", "hunter2")
	customerID, _, _ := seedQuoteBuilderFixtures(t, a)
	draft, err := a.store.CreateDraftQuote(ctx, customerID, ana, "QL")
	if err != nil {
		t.Fatal(err)
	}
	c1, c2 := money.Micros(1_000_000_000), money.Micros(250_500_000)
	lines := []store.QuoteLine{
		{DescriptionSnapshot: "Poste metálico", QtyMilli: 2_000, UnitPriceMicros: 1_200_000_000, LineTotal: 240_000, CostMicros: &c1, Source: "manual"},
		{DescriptionSnapshot: "Flete", QtyMilli: 1_000, UnitPriceMicros: 300_000_000, LineTotal: 30_000, CostMicros: &c2, Source: "manual"},
		{DescriptionSnapshot: "Sin costo", QtyMilli: 1_000, UnitPriceMicros: 100_000_000, LineTotal: 10_000, Source: "manual"},
	}
	if err := a.store.ReplaceQuoteLines(ctx, draft.ID, nil, nil, lines, pricing.Totals{Subtotal: 280_000}); err != nil {
		t.Fatal(err)
	}
	if err := a.store.SetSupplierQuoteInfo(ctx, draft.ID, "cable anti robo", "COT-9", ana); err != nil {
		t.Fatal(err)
	}
	if err := a.store.IssueQuote(ctx, draft.ID, store.Issue{IssuedAt: "2026-10-20T15:00:00.000Z", TermsSnapshot: "t",
		CustomerNameSnapshot: "Grupo PEME", VendedorSnapshot: "ana", PDFSHA256: "x", IssuedBy: ana}); err != nil {
		t.Fatal(err)
	}
	receiveClientOC(t, a, draft.Folio, ana)
	folio := draft.Folio
	pv := map[string]string{"folio": folio}
	rec := doForm(t, a, ana, h.Generar, "POST", "/proyectos/"+folio+"/ordenes-de-compra", pv, url.Values{}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Generar = %d", rec.Code)
	}
	first := rec.Header().Get("Location")
	fpv := map[string]string{"folio": folio, "id": first[strings.LastIndex(first, "/")+1:]}
	body := doForm(t, a, ana, h.Page, "GET", first, fpv, nil, false).Body.String()
	for _, want := range []string{"selected>Cable Anti Robo", "Marca las que van en esta orden de compra.",
		"Sin costo en la cotización: no se puede comprar desde aquí.",
		"Si el proveedor no está en la lista, pide a un administrador que lo dé de alta."} {
		if !strings.Contains(body, want) {
			t.Errorf("QL draft lacks %q", want)
		}
	}
	if strings.Contains(body, "Dar de alta un proveedor nuevo") {
		t.Error("a vendedor is offered to add a supplier")
	}
	// Only the poste in this one.
	if rec := doForm(t, a, ana, h.Guardar, "POST", first, fpv, poFormValues("99999", []string{"1"}, nil), false); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("an unknown supplier id saved: %d", rec.Code)
	}
	sup, _ := a.store.SupplierByName(ctx, "Cable Anti Robo")
	if rec := doForm(t, a, ana, h.Guardar, "POST", first, fpv, poFormValues(strconv.FormatInt(*sup, 10), []string{"1"}, nil), false); rec.Code != http.StatusSeeOther {
		t.Fatalf("Guardar = %d", rec.Code)
	}

	// A second one takes what is left, for a new supplier added from the O.C. itself.
	rec = doForm(t, a, ana, h.Generar, "POST", "/proyectos/"+folio+"/ordenes-de-compra", pv, url.Values{}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("second Generar = %d", rec.Code)
	}
	second := rec.Header().Get("Location")
	spv := map[string]string{"folio": folio, "id": second[strings.LastIndex(second, "/")+1:]}
	sid, _ := strconv.ParseInt(spv["id"], 10, 64)
	secondLines, _ := a.store.PurchaseOrderLines(ctx, sid)
	if len(secondLines) != 1 || secondLines[0].QuoteLineNo != 2 {
		t.Fatalf("second O.C.'s lines = %+v", secondLines)
	}
	body = doForm(t, a, beto, h.Page, "GET", second, spv, nil, false).Body.String()
	if !strings.Contains(body, "Ya está en un borrador.") || !strings.Contains(body, "Dar de alta un proveedor nuevo") {
		t.Error("the second QL draft doesn't mark the poste as taken or doesn't offer a new supplier")
	}
	newSupplier := poFormValues("", []string{"2"}, map[string]string{"domicilio": "Obra", "nuevo_name": "Fletes Rápidos"})
	rec = doForm(t, a, beto, adminOnly(a, h.ProveedorNuevo), "POST", second+"/proveedor-nuevo", spv, newSupplier, false)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "El RFC es obligatorio.") ||
		!strings.Contains(rec.Body.String(), `value="Fletes Rápidos"`) {
		t.Fatalf("incomplete new supplier = %d", rec.Code)
	}
	for k, v := range map[string]string{"nuevo_rfc": "FRA010101AB1", "nuevo_legal_name": "Fletes Rápidos SA de CV",
		"nuevo_street": "Av. Uno", "nuevo_ext_number": "5", "nuevo_neighborhood": "Centro",
		"nuevo_municipality": "Querétaro", "nuevo_state": "Querétaro", "nuevo_postal_code": "76000", "nuevo_credit_days": "15"} {
		newSupplier.Set(k, v)
	}
	if rec := doForm(t, a, ana, adminOnly(a, h.ProveedorNuevo), "POST", second+"/proveedor-nuevo", spv, newSupplier, false); rec.Code != http.StatusForbidden {
		t.Fatalf("a vendedor added a supplier: %d", rec.Code)
	}
	// A name like an existing supplier's asks first.
	newSupplier.Set("nuevo_name", "Cable Anti Robo")
	rec = doForm(t, a, beto, adminOnly(a, h.ProveedorNuevo), "POST", second+"/proveedor-nuevo", spv, newSupplier, false)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Es un proveedor distinto: darlo de alta de todos modos") {
		t.Fatalf("a possible duplicate = %d", rec.Code)
	}
	newSupplier.Set("nuevo_name", "Fletes Rápidos")
	rec = doForm(t, a, beto, adminOnly(a, h.ProveedorNuevo), "POST", second+"/proveedor-nuevo", spv, newSupplier, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("ProveedorNuevo = %d: %s", rec.Code, rec.Body.String())
	}
	if body := doForm(t, a, beto, h.Page, "GET", second+"?proveedor=1", spv, nil, false).Body.String(); !strings.Contains(body, "Proveedor dado de alta y elegido.") {
		t.Error("no message after adding the supplier")
	}
	if body := doForm(t, a, ana, NewProjects(a.store).Page, "GET", "/proyectos/"+folio, pv, nil, false).Body.String(); !strings.Contains(body, "Generar otra orden de compra") {
		t.Error("a QL proyecto with O.C.s isn't offered another")
	}
	po2, _ := a.store.PurchaseOrderByID(ctx, sid)
	if po2.SupplierName != "Fletes Rápidos" || po2.CreditDays == nil || *po2.CreditDays != 15 || po2.DeliveryAddress != "Obra" {
		t.Fatalf("second O.C. after the new supplier = %+v", po2)
	}

	// A third borrador, discarded.
	rec = doForm(t, a, ana, h.Generar, "POST", "/proyectos/"+folio+"/ordenes-de-compra", pv, url.Values{}, false)
	third := rec.Header().Get("Location")
	tpv := map[string]string{"folio": folio, "id": third[strings.LastIndex(third, "/")+1:]}
	if rec := doForm(t, a, ana, h.Eliminar, "POST", third+"/eliminar", tpv, url.Values{}, false); rec.Code != http.StatusSeeOther ||
		rec.Header().Get("Location") != "/proyectos/"+folio+"#oc-proveedor" {
		t.Fatalf("Eliminar borrador = %d", rec.Code)
	}
	if rec := doForm(t, a, ana, h.Page, "GET", third, tpv, nil, false); rec.Code != http.StatusNotFound {
		t.Fatalf("a discarded borrador = %d", rec.Code)
	}

	day := store.InMexicoCity(time.Now()).Format("020106")
	for i, path := range []string{first, second} {
		ppv := map[string]string{"folio": folio, "id": path[strings.LastIndex(path, "/")+1:]}
		supplier := strconv.FormatInt(*sup, 10)
		if i == 1 {
			supplier = strconv.FormatInt(*po2.SupplierID, 10)
		}
		rec := doForm(t, a, beto, h.Confirmar, "POST", path+"/confirmar", ppv,
			poFormValues(supplier, []string{strconv.Itoa(i + 1)}, map[string]string{"domicilio": "Obra"}), false)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("confirm %d = %d: %s", i+1, rec.Code, rec.Body.String())
		}
	}
	po2, _ = a.store.PurchaseOrderByID(ctx, sid)
	if po2.Number != "OC-"+folio+"-"+day+"-2" || po2.Total.String() != "$290.58" {
		t.Fatalf("second QL O.C. = %q, %s", po2.Number, po2.Total)
	}
}

func TestAjustesPOBilling(t *testing.T) {
	a := newTestAuth(t)
	s := NewSettings(a.store)
	admin := createTestUser(t, a, "beto", "admin", "hunter2")
	rec := doForm(t, a, admin, s.POBilling, "POST", "/ajustes/ordenes-de-compra", nil,
		url.Values{"oc_nombre": {""}, "oc_domicilio": {""}, "oc_uso": {"X"}}, false)
	body := rec.Body.String()
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("empty billing = %d", rec.Code)
	}
	for _, want := range []string{"Escribe la razón social con su régimen de capital, p. ej. CLADEX S.A. de C.V.",
		"Escribe el domicilio fiscal.", "Elige el uso de CFDI."} {
		if !strings.Contains(body, want) {
			t.Errorf("refused billing lacks %q", want)
		}
	}
	rec = doForm(t, a, admin, s.POBilling, "POST", "/ajustes/ordenes-de-compra", nil,
		url.Values{"oc_nombre": {"CLADEX S.A. de C.V."}, "oc_domicilio": {"Otra calle 1"}, "oc_uso": {"G03"}}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POBilling = %d", rec.Code)
	}
	body = doForm(t, a, admin, s.Page, "GET", "/ajustes?oc=1", nil, nil, false).Body.String()
	for _, want := range []string{"Datos de facturación de las órdenes de compra guardados.", "Datos de facturación en órdenes de compra",
		`value="Otra calle 1"`, `value="G03" selected`} {
		if !strings.Contains(body, want) {
			t.Errorf("Configuración del sistema lacks %q", want)
		}
	}
}
