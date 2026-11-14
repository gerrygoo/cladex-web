package web

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gerrygoo/cladex-web/internal/facturacion"
	"github.com/gerrygoo/cladex-web/internal/facturoporti"
	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

// testFacturacion is facturación against an in-process fake Facturoporti, with the SAT's
// test CSD and Cladex's emisor set to that CSD's company.
func testFacturacion(t *testing.T, a *Auth, adminID int64) (*facturacion.Service, *facturoporti.Fake) {
	t.Helper()
	fake := facturoporti.NewFake()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	cer, err1 := os.ReadFile("../facturoporti/testdata/eku9003173c9.cer")
	key, err2 := os.ReadFile("../facturoporti/testdata/eku9003173c9.key")
	if err1 != nil || err2 != nil {
		t.Fatal(err1, err2)
	}
	csd, err := facturoporti.ParseCSD(cer, key, "12345678a")
	if err != nil {
		t.Fatal(err)
	}
	svc := &facturacion.Service{
		Store:  a.store,
		Client: &facturoporti.Client{BaseURL: srv.URL, Sandbox: true, Token: "tok", CSD: csd, HTTP: srv.Client()},
		Status: facturoporti.Status{Enabled: true, Sandbox: true, RFC: csd.RFC},
		Now:    time.Now,
	}
	if err := a.store.SetEmisor(context.Background(), store.Emisor{RFC: "EKU9003173C9", Name: "ESCUELA KEMPER URGATE", TaxRegime: "601",
		PostalCode: "42501", SerieFactura: "F", SeriePago: "P"}, adminID); err != nil {
		t.Fatal(err)
	}
	return svc, fake
}

// facturaFixture is a proyecto in O.C. recibida (P.P.D.), with a fiscally complete
// customer, ready to invoice.
func facturaFixture(t *testing.T, a *Auth, admin int64) (projectFolio string) {
	t.Helper()
	ctx := context.Background()
	q := newTestQuotes(t, a)
	p := NewProjects(a.store)
	customerID, _, costProductID := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, admin)
	cust, _ := a.store.CustomerByID(ctx, customerID)
	cust.RFC, cust.LegalName, cust.TaxRegime, cust.PostalCode, cust.CFDIUse, cust.Email =
		"IIA040805DZ4", "INDISTRIA ILUMINADORA DE ALMACENES", "601", "62661", "G01", "compras@peme.mx"
	if err := a.store.UpdateCustomer(ctx, *cust); err != nil {
		t.Fatal(err)
	}
	folio := issueTestQuote(t, a, q, admin, customerID, costProductID)
	project, _ := a.store.ProjectByFolio(ctx, folio)
	if err := a.store.SetProjectProbability(ctx, project.ID, admin, 90, ""); err != nil {
		t.Fatal(err)
	}
	if rec := postOC(t, a, p, admin, folio, map[string]string{"oc_numero": "4411", "oc_fecha": "2026-10-07"},
		"oc.pdf", []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n")); rec.Code != http.StatusSeeOther {
		t.Fatalf("postOC = %d: %s", rec.Code, rec.Body.String())
	}
	return folio
}

func TestFacturacionEndToEnd(t *testing.T) {
	a := newTestAuth(t)
	ctx := context.Background()
	admin := createTestUser(t, a, "gerry", "admin", "hunter2")
	vendedor := createTestUser(t, a, "emilio", "vendedor", "hunter2")
	svc, fake := testFacturacion(t, a, admin)
	f := NewFacturas(a.store, svc, nil)
	p := NewProjects(a.store)
	p.fact = svc
	folio := facturaFixture(t, a, admin)

	projectPage := func() string {
		return doForm(t, a, admin, p.Page, "GET", "/proyectos/"+folio, map[string]string{"folio": folio}, nil, false).Body.String()
	}
	if body := projectPage(); !strings.Contains(body, "Crear factura") || !strings.Contains(body, "Registrar una factura hecha fuera de la app") {
		t.Fatalf("project page should offer Crear factura and the manual factura")
	}
	post := func(userID int64, h http.HandlerFunc, path string, pv map[string]string, form url.Values) *httptest.ResponseRecorder {
		t.Helper()
		return doForm(t, a, userID, a.RequireAdmin(h).ServeHTTP, "POST", path, pv, form, false)
	}
	rec := post(admin, f.Crear, "/proyectos/"+folio+"/factura/nueva", map[string]string{"folio": folio}, nil)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Crear = %d: %s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	id := strings.TrimPrefix(loc, "/facturas/")
	pv := map[string]string{"id": id}
	page := func(userID int64, id string) string {
		return doForm(t, a, userID, f.Page, "GET", "/facturas/"+id, map[string]string{"id": id}, nil, false).Body.String()
	}
	if rec := post(admin, f.Crear, "/proyectos/"+folio+"/factura/nueva", map[string]string{"folio": folio}, nil); rec.Code != http.StatusConflict {
		t.Errorf("a second factura = %d, want 409", rec.Code)
	}

	// The draft says what's missing: the familia has no SAT code and the product no unit.
	// The proyecto has no método de pago (issue #61: it isn't asked with the O.C.), so the
	// draft asks for it.
	body := page(admin, id)
	for _, want := range []string{"Factura (borrador)", "Para validarla falta", "falta la clave de producto SAT", "falta la clave de unidad SAT",
		"Elige el método de pago: P.U.E. o P.P.D.", "<dd>Sin elegir</dd>", `name="metodo_pago" value="PPD" required`,
		"compras@peme.mx", "IIA040805DZ4", "Pruebas: sin validez fiscal"} {
		if !strings.Contains(body, want) {
			t.Errorf("draft page missing %q", want)
		}
	}
	c, _ := a.store.ComprobanteByID(ctx, mustAtoi(t, id))
	line := strconv.FormatInt(c.Conceptos[0].ID, 10)
	form := url.Values{"uso_cfdi": {"G01"}, "recipients": {"compras@peme.mx"}, "concepto_id": {line}, "metodo_pago": {"PUE"},
		"sat_product_" + line: {"26121600"}, "sat_unit_" + line: {"mtr"}, "unit_name_" + line: {"Metro"}, "description_" + line: {"Cable THW 14"}}
	// P.U.E. asks for the forma de pago; P.P.D. sets it to 99, and back to P.U.E. it is
	// asked again.
	if rec := post(admin, f.Guardar, "/facturas/"+id, pv, form); rec.Code != http.StatusSeeOther {
		t.Fatalf("Guardar(PUE) = %d: %s", rec.Code, rec.Body.String())
	}
	if body := page(admin, id); !strings.Contains(body, "Elige la forma de pago con que el cliente pagó.") || !strings.Contains(body, `value="PUE" checked`) {
		t.Error("a P.U.E. draft without forma de pago doesn't ask for it")
	}
	form.Set("metodo_pago", "PPD")
	if rec := post(admin, f.Guardar, "/facturas/"+id, pv, form); rec.Code != http.StatusSeeOther {
		t.Fatalf("Guardar(PPD) = %d: %s", rec.Code, rec.Body.String())
	}
	if body := page(admin, id); !strings.Contains(body, "99 - Por definir") {
		t.Error("a P.P.D. draft doesn't carry forma de pago 99")
	}
	bad := url.Values{"metodo_pago": {"contado"}}
	for k, v := range form {
		if k != "metodo_pago" {
			bad[k] = v
		}
	}
	if rec := post(admin, f.Guardar, "/facturas/"+id, pv, bad); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("Guardar(método contado) = %d, want 422", rec.Code)
	}
	if rec := post(vendedor, f.Validar, "/facturas/"+id+"/validar", pv, form); rec.Code != http.StatusForbidden {
		t.Fatalf("vendedor Validar = %d, want 403", rec.Code)
	}
	if rec := post(admin, f.Validar, "/facturas/"+id+"/validar", pv, form); rec.Code != http.StatusSeeOther {
		t.Fatalf("Validar = %d: %s", rec.Code, rec.Body.String())
	}
	c, _ = a.store.ComprobanteByID(ctx, c.ID)
	if c.Status != store.CompLista || c.SerieFolio() != "F-1" || !strings.Contains(c.Request, `"CodigoUnidad":"MTR"`) ||
		strings.Contains(c.Request, "LlavePrivada") || !strings.Contains(c.Request, `"FormaPago":"99"`) {
		t.Fatalf("validated = %s %s %s", c.Status, c.SerieFolio(), c.Request)
	}

	// The answer is lost: incierto. Retrying sends the same request and gets the same stamp.
	fake.Drop = true
	if rec := post(admin, f.Timbrar, "/facturas/"+id+"/timbrar", pv, nil); rec.Code != http.StatusSeeOther {
		t.Fatalf("Timbrar = %d", rec.Code)
	}
	if body := page(admin, id); !strings.Contains(body, "Timbrado incierto") || !strings.Contains(body, "Reintentar con la misma solicitud") {
		t.Fatalf("incierto page: %s", body)
	}
	if rec := post(admin, f.Timbrar, "/facturas/"+id+"/timbrar", pv, nil); rec.Header().Get("Location") != "/facturas/"+id+"?ok=timbrada" {
		t.Fatalf("retry = %d %s", rec.Code, rec.Header().Get("Location"))
	}
	if fake.Stamps() != 1 || fake.Requests != 2 {
		t.Errorf("stamps = %d of %d requests, want 1 of 2", fake.Stamps(), fake.Requests)
	}
	c, _ = a.store.ComprobanteByID(ctx, c.ID)
	project, _ := a.store.ProjectByFolio(ctx, folio)
	// Stamping doesn't move the proyecto: its Estado de facturación follows the stamped
	// factura (P.P.D.: waiting for its complemento), and its history says so.
	if c.Status != store.CompTimbrada || c.UUID == "" || !c.EmailSent || project.Status != store.StageEnCurso ||
		project.States.Facturacion != store.FacturacionPendienteComplemento || project.Invoice.Ref != "" ||
		project.LastBody != "Factura F-1 timbrada · Folio fiscal "+c.UUID {
		t.Fatalf("after stamp: %s %s; project %s %+v %q", c.Status, c.UUID, project.Status, project.States, project.LastBody)
	}
	events, _ := a.store.ListComprobanteEvents(ctx, c.ID)
	var names []string
	for _, e := range events {
		names = append(names, e.Event)
	}
	if strings.Join(names, ",") != "created,validated,stamp_requested,stamp_unknown,stamp_requested,stamp_succeeded" {
		t.Errorf("events = %v", names)
	}

	// Anyone signed in downloads it.
	rec = doForm(t, a, vendedor, f.PDF, "GET", "/facturas/"+id+"/pdf", pv, nil, false)
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Body.String(), "%PDF") {
		t.Errorf("PDF = %d", rec.Code)
	}
	rec = doForm(t, a, vendedor, f.XML, "GET", "/facturas/"+id+"/xml", pv, nil, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "TimbreFiscalDigital") ||
		!strings.Contains(rec.Header().Get("Content-Disposition"), "F-1_"+c.UUID+".xml") {
		t.Errorf("XML = %d %s", rec.Code, rec.Header().Get("Content-Disposition"))
	}

	// The comprobante de pago: once stamped, it covers the factura and the proyecto is
	// Facturada.
	rec = post(admin, f.Pago, "/facturas/"+id+"/pago", pv, nil)
	pagoID := strings.TrimPrefix(rec.Header().Get("Location"), "/facturas/")
	ppv := map[string]string{"id": pagoID}
	if rec.Code != http.StatusSeeOther || pagoID == "" {
		t.Fatalf("Pago = %d", rec.Code)
	}
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	if rec := post(admin, f.Validar, "/facturas/"+pagoID+"/validar", ppv, url.Values{"pago_fecha": {yesterday}, "forma_pago": {"03"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("Validar pago = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := post(admin, f.Timbrar, "/facturas/"+pagoID+"/timbrar", ppv, nil); !strings.HasSuffix(rec.Header().Get("Location"), "ok=timbrada") {
		t.Fatalf("Timbrar pago = %d %s", rec.Code, rec.Header().Get("Location"))
	}
	project, _ = a.store.ProjectByFolio(ctx, folio)
	if project.States.Facturacion != store.FacturacionFacturada || !strings.HasPrefix(project.LastBody, "Comprobante de pago P-1 timbrado · Folio fiscal ") {
		t.Errorf("after pago: %+v %q", project.States, project.LastBody)
	}
	if rec := doForm(t, a, admin, f.PDF, "GET", "/facturas/"+pagoID+"/pdf", ppv, nil, false); rec.Code != http.StatusOK {
		t.Errorf("pago PDF = %d", rec.Code)
	}

	// Replace the factura (first refused by the SAT, then fixed), then cancel the original
	// with motivo 01.
	rec = post(admin, f.Sustituir, "/facturas/"+id+"/sustituir", pv, nil)
	repID := strings.TrimPrefix(rec.Header().Get("Location"), "/facturas/")
	rpv := map[string]string{"id": repID}
	rep, _ := a.store.ComprobanteByID(ctx, mustAtoi(t, repID))
	rl := strconv.FormatInt(rep.Conceptos[0].ID, 10)
	repForm := url.Values{"uso_cfdi": {"G03"}, "concepto_id": {rl}, "sat_product_" + rl: {"26121600"}, "sat_unit_" + rl: {"MTR"},
		"unit_name_" + rl: {"Metro"}, "description_" + rl: {"Cable THW 14, corregido"}}
	if rec := post(admin, f.Validar, "/facturas/"+repID+"/validar", rpv, repForm); rec.Code != http.StatusSeeOther {
		t.Fatalf("Validar replacement = %d: %s", rec.Code, rec.Body.String())
	}
	fake.Refuse = "CFDI40147 - El campo Rfc del receptor no existe"
	post(admin, f.Timbrar, "/facturas/"+repID+"/timbrar", rpv, nil)
	if body := page(admin, repID); !strings.Contains(body, "Rechazada") || !strings.Contains(body, "CFDI40147") {
		t.Fatalf("refused page: %s", body)
	}
	if rec := post(admin, f.Editar, "/facturas/"+repID+"/editar", rpv, nil); rec.Code != http.StatusSeeOther {
		t.Fatalf("Editar = %d", rec.Code)
	}
	post(admin, f.Validar, "/facturas/"+repID+"/validar", rpv, repForm)
	post(admin, f.Timbrar, "/facturas/"+repID+"/timbrar", rpv, nil)
	rep, _ = a.store.ComprobanteByID(ctx, rep.ID)
	if rep.Status != store.CompTimbrada || rep.SerieFolio() != "F-2" || !strings.Contains(rep.XML, `TipoRelacion="04"`) {
		t.Fatalf("replacement = %s %s", rep.Status, rep.SerieFolio())
	}
	project, _ = a.store.ProjectByFolio(ctx, folio)
	if !strings.HasPrefix(project.LastBody, "Factura sustituida por F-2 · Folio fiscal ") {
		t.Errorf("project history = %q", project.LastBody)
	}
	if body := page(admin, id); !strings.Contains(body, "Sustituida por") || !strings.Contains(body, "01 - Comprobante emitido con errores con relación") {
		t.Errorf("original page lacks the replacement and motivo 01")
	}
	rec = post(admin, f.Cancelar, "/facturas/"+id+"/cancelar", pv, url.Values{"motivo": {"01"}, "sustitucion": {rep.UUID}})
	if rec.Header().Get("Location") != "/facturas/"+id+"?ok=cancelada" {
		t.Fatalf("Cancelar = %d %s: %s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	c, _ = a.store.ComprobanteByID(ctx, c.ID)
	if c.Status != store.CompCancelada || c.CancelMotivo != "01" || c.CancelSustitucion != rep.UUID {
		t.Errorf("cancelled = %s %s %s", c.Status, c.CancelMotivo, c.CancelSustitucion)
	}
	// The Facturas table shows the proyecto with its valid factura's folio fiscal, the
	// replacement's, linked to it.
	if list := doForm(t, a, vendedor, f.List, "GET", "/facturas", nil, nil, false).Body.String(); !strings.Contains(list, `id="p-`+folio+`"`) ||
		!strings.Contains(list, `href="/facturas/`+repID+`"`) || !strings.Contains(list, rep.UUID) {
		t.Errorf("list page: %s", list)
	}
}

// A pending cancellation waits on the receptor, and checking its state settles it.
func TestFacturacionCancelPending(t *testing.T) {
	a := newTestAuth(t)
	ctx := context.Background()
	admin := createTestUser(t, a, "gerry", "admin", "hunter2")
	svc, fake := testFacturacion(t, a, admin)
	folio := facturaFixture(t, a, admin)
	project, _ := a.store.ProjectByFolio(ctx, folio)
	id, err := a.store.CreateFacturaDraft(ctx, project.ID, admin)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := a.store.ComprobanteByID(ctx, id)
	if err := a.store.UpdateDraft(ctx, id, admin, store.DraftEdit{UsoCFDI: "G01", MetodoPago: "PPD", FormaPago: "99", Conceptos: []store.ConceptoEdit{
		{ID: c.Conceptos[0].ID, SATProductCode: "26121600", SATUnitCode: "MTR", Description: "Cable"}}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Validate(ctx, id, admin); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if o, err := svc.Stamp(ctx, id, admin); err != nil || o.Result != "stamped" {
		t.Fatalf("Stamp: %+v %v", o, err)
	}
	fake.CancelPending = true
	if o, err := svc.Cancel(ctx, id, admin, "02", ""); err != nil || o.Result != "pending" {
		t.Fatalf("Cancel: %+v %v", o, err)
	}
	if o, _ := svc.CheckCancellation(ctx, id, admin); o.Result != "pending" {
		t.Errorf("still pending: %+v", o)
	}
	c, _ = a.store.ComprobanteByID(ctx, id)
	fake.AcceptCancellation(c.UUID)
	if o, err := svc.CheckCancellation(ctx, id, admin); err != nil || o.Result != "cancelled" {
		t.Fatalf("CheckCancellation: %+v %v", o, err)
	}
	c, _ = a.store.ComprobanteByID(ctx, id)
	if c.Status != store.CompCancelada {
		t.Errorf("status = %s", c.Status)
	}
}

// With facturación off, the proyecto offers only the manual factura, and stamping refuses.
func TestFacturacionOff(t *testing.T) {
	a := newTestAuth(t)
	admin := createTestUser(t, a, "gerry", "admin", "hunter2")
	folio := facturaFixture(t, a, admin)
	p := NewProjects(a.store)
	p.fact = facturacion.New(a.store, nil, facturoporti.Status{Sandbox: true, Problem: "no está configurada"})
	body := doForm(t, a, admin, p.Page, "GET", "/proyectos/"+folio, map[string]string{"folio": folio}, nil, false).Body.String()
	if !strings.Contains(body, "Registrar una factura hecha fuera de la app") || strings.Contains(body, "Crear factura") {
		t.Error("facturación off should offer only the manual factura")
	}
	if _, err := p.fact.Stamp(context.Background(), 1, admin); err == nil || !strings.Contains(err.Error(), "apagada") {
		t.Errorf("Stamp with facturación off = %v", err)
	}
}

// TestFacturaConceptoIsOneLine covers issue #57: a quote line's description may hold line
// breaks for the PDF, but the factura's concepto gets it on one line, as it always had.
func TestFacturaConceptoIsOneLine(t *testing.T) {
	a := newTestAuth(t)
	ctx := context.Background()
	admin := createTestUser(t, a, "gerry", "admin", "hunter2")
	q := newTestQuotes(t, a)
	customerID, _, productID := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, admin)
	product, _ := a.store.ProductByID(ctx, productID)
	product.Description = "Cable\nrojo"
	if err := a.store.UpdateProduct(ctx, *product); err != nil {
		t.Fatal(err)
	}
	folio := issueTestQuote(t, a, q, admin, customerID, productID)
	project, _ := a.store.ProjectByFolio(ctx, folio)
	if err := a.store.SetProjectProbability(ctx, project.ID, admin, 90, ""); err != nil {
		t.Fatal(err)
	}
	if rec := postOC(t, a, NewProjects(a.store), admin, folio, map[string]string{"oc_numero": "4411", "oc_fecha": "2026-10-07"},
		"oc.pdf", []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n")); rec.Code != http.StatusSeeOther {
		t.Fatalf("postOC = %d: %s", rec.Code, rec.Body.String())
	}
	id, err := a.store.CreateFacturaDraft(ctx, project.ID, admin)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := a.store.ComprobanteByID(ctx, id)
	if got := c.Conceptos[0].Description; got != "Cable rojo" {
		t.Errorf("concepto description = %q, want %q", got, "Cable rojo")
	}
}

// TestFacturaDraftTakesProductSATCodes covers issue #62: a factura's concepto starts with
// its product's own SAT codes, the unit named as the SAT's catalog has it; a product
// without them falls back to its quote type's familia code and its unit's code.
func TestFacturaDraftTakesProductSATCodes(t *testing.T) {
	a := newTestAuth(t)
	ctx := context.Background()
	admin := createTestUser(t, a, "gerry", "admin", "hunter2")
	q := newTestQuotes(t, a)
	customerID, coded, plain := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, admin)

	product, _ := a.store.ProductByID(ctx, coded)
	product.SATProductCode, product.SATUnitCode = "26121609", "XRL"
	if err := a.store.UpdateProduct(ctx, *product); err != nil {
		t.Fatal(err)
	}
	units, _ := a.store.ListUnits(ctx)
	product, _ = a.store.ProductByID(ctx, plain)
	for _, u := range units {
		if u.Code == "pza" {
			product.UnitID = &u.ID
		}
	}
	if err := a.store.UpdateProduct(ctx, *product); err != nil {
		t.Fatal(err)
	}
	if err := a.store.UpdateFamily(ctx, product.FamilyID, product.FamilyName, "39111500"); err != nil {
		t.Fatal(err)
	}

	quote, err := a.store.CreateDraftQuote(ctx, customerID, admin, "QA")
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"line_keys": {"0,1"},
		"lines[0][kind]": {"product"}, "lines[0][product_id]": {itoa(coded)}, "lines[0][qty]": {"1"},
		"lines[1][kind]": {"product"}, "lines[1][product_id]": {itoa(plain)}, "lines[1][qty]": {"2"}}
	pv := map[string]string{"folio": quote.Folio}
	if rec := doForm(t, a, admin, q.Emitir, "POST", "/cotizaciones/"+quote.Folio+"/emitir", pv, form, false); rec.Code != http.StatusSeeOther {
		t.Fatalf("Emitir = %d: %s", rec.Code, rec.Body.String())
	}
	project, _ := a.store.ProjectByFolio(ctx, quote.Folio)
	if err := a.store.SetProjectProbability(ctx, project.ID, admin, 90, ""); err != nil {
		t.Fatal(err)
	}
	if rec := postOC(t, a, NewProjects(a.store), admin, quote.Folio, map[string]string{"oc_numero": "4411", "oc_fecha": "2026-10-07"},
		"oc.pdf", []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n")); rec.Code != http.StatusSeeOther {
		t.Fatalf("postOC = %d: %s", rec.Code, rec.Body.String())
	}
	id, err := a.store.CreateFacturaDraft(ctx, project.ID, admin)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := a.store.ComprobanteByID(ctx, id)
	if len(c.Conceptos) != 2 {
		t.Fatalf("conceptos = %+v", c.Conceptos)
	}
	if k := c.Conceptos[0]; k.SATProductCode != "26121609" || k.SATUnitCode != "XRL" || k.UnitName != "Carrete" {
		t.Errorf("coded product's concepto = %+v; want 26121609, XRL Carrete", k)
	}
	if k := c.Conceptos[1]; k.SATProductCode != "39111500" || k.SATUnitCode != "H87" || k.UnitName != "Pieza" {
		t.Errorf("uncoded product's concepto = %+v; want the familia's 39111500 and the unit's H87 Pieza", k)
	}

	// Codes typed on the draft are checked against the catalogs before validating.
	svc, _ := testFacturacion(t, a, admin)
	f := NewFacturas(a.store, svc, nil)
	if err := a.store.UpdateDraft(ctx, id, admin, store.DraftEdit{UsoCFDI: "G01", MetodoPago: "PPD", FormaPago: "99", Conceptos: []store.ConceptoEdit{
		{ID: c.Conceptos[0].ID, SATProductCode: "26121699", SATUnitCode: "QQQ", Description: "Foco LED"}}}); err != nil {
		t.Fatal(err)
	}
	sid := itoa(id)
	body := doForm(t, a, admin, f.Page, "GET", "/facturas/"+sid, map[string]string{"id": sid}, nil, false).Body.String()
	for _, want := range []string{"Partida 1: la clave de producto SAT 26121699 no existe en el catálogo del SAT.",
		"Partida 1: la clave de unidad SAT QQQ no existe en el catálogo del SAT."} {
		if !strings.Contains(body, want) {
			t.Errorf("draft page missing %q", want)
		}
	}
}

// The Facturas table (#63) lists the proyectos with the client's O.C. received, Pendiente
// with its días pendiente until a factura is stamped, and filters by Estado de facturación.
func TestFacturasTable(t *testing.T) {
	a := newTestAuth(t)
	ctx := context.Background()
	admin := createTestUser(t, a, "gerry", "admin", "hunter2")
	vendedor := createTestUser(t, a, "emilio", "vendedor", "hunter2")
	svc, _ := testFacturacion(t, a, admin)
	f := NewFacturas(a.store, svc, nil)
	list := func(query string) string {
		return doForm(t, a, vendedor, f.List, "GET", "/facturas?"+query, nil, nil, false).Body.String()
	}
	if body := list(""); !strings.Contains(body, "Todavía no hay proyectos con orden de compra recibida.") {
		t.Errorf("empty table doesn't say so: %s", body)
	}

	folio := facturaFixture(t, a, admin)
	body := list("")
	for _, want := range []string{`id="p-` + folio + `"`, "Pendiente", "4411", "Días pendiente", "Folio fiscal",
		"Costo sin IVA", "Precio de venta sin IVA", "1 proyecto"} {
		if !strings.Contains(body, want) {
			t.Errorf("table missing %q", want)
		}
	}
	project, _ := a.store.ProjectByFolio(ctx, folio)
	if got := views.DaysPendingFactura(*project, "2026-10-17"); got != "10" {
		t.Errorf("DaysPendingFactura = %q, want 10 (O.C. of 2026-10-07)", got)
	}
	if body := list("f.estado=facturada"); strings.Contains(body, `id="p-`+folio+`"`) || !strings.Contains(body, "Ningún proyecto coincide con el filtro.") {
		t.Error("the Facturada filter shows a pending proyecto")
	}
	if body := list("f.estado=pendiente"); !strings.Contains(body, `id="p-`+folio+`"`) {
		t.Error("the Pendiente filter hides a pending proyecto")
	}

	// Stamped as P.U.E., it is Facturada with its folio fiscal and no días pendiente.
	id, err := a.store.CreateFacturaDraft(ctx, project.ID, admin)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := a.store.ComprobanteByID(ctx, id)
	if err := a.store.UpdateDraft(ctx, id, admin, store.DraftEdit{UsoCFDI: "G01", MetodoPago: "PUE", FormaPago: "03", Conceptos: []store.ConceptoEdit{
		{ID: c.Conceptos[0].ID, SATProductCode: "26121600", SATUnitCode: "MTR", Description: "Cable"}}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Validate(ctx, id, admin); err != nil {
		t.Fatal(err)
	}
	if o, err := svc.Stamp(ctx, id, admin); err != nil || o.Result != "stamped" {
		t.Fatalf("Stamp = %+v, %v", o, err)
	}
	c, _ = a.store.ComprobanteByID(ctx, id)
	project, _ = a.store.ProjectByFolio(ctx, folio)
	if project.States.Facturacion != store.FacturacionFacturada || views.DaysPendingFactura(*project, "2026-10-17") != "" {
		t.Fatalf("after stamping: %s", project.States.Facturacion)
	}
	if body := list("f.estado=facturada"); !strings.Contains(body, `id="p-`+folio+`"`) || !strings.Contains(body, c.UUID) {
		t.Error("the Facturada filter misses the stamped proyecto or its folio fiscal")
	}
}

// testCFDI is a minimal stamped CFDI 4.0 factura as the PAC's portal would give it, for
// recording by hand.
func testCFDI(uuid, metodo string, total money.Centavos, rfc string) []byte {
	sub := money.Centavos(int64(total) * 100 / 116)
	amt := func(c money.Centavos) string { return fmt.Sprintf("%d.%02d", int64(c)/100, int64(c)%100) }
	forma := "03"
	if metodo == "PPD" {
		forma = "99"
	}
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<cfdi:Comprobante xmlns:cfdi="http://www.sat.gob.mx/cfd/4" xmlns:tfd="http://www.sat.gob.mx/TimbreFiscalDigital" Version="4.0" Serie="A" Folio="153" Fecha="2026-11-12T10:00:00" FormaPago="` + forma + `" SubTotal="` + amt(sub) + `" Moneda="MXN" Total="` + amt(total) + `" TipoDeComprobante="I" Exportacion="01" MetodoPago="` + metodo + `" LugarExpedicion="64000" Sello="abcdefghijkl" NoCertificado="00001000000500000000">
  <cfdi:Emisor Rfc="CLA010101AAA" Nombre="CLADEX" RegimenFiscal="601"/>
  <cfdi:Receptor Rfc="` + rfc + `" Nombre="INDISTRIA ILUMINADORA DE ALMACENES" DomicilioFiscalReceptor="62661" RegimenFiscalReceptor="601" UsoCFDI="G01"/>
  <cfdi:Conceptos>
    <cfdi:Concepto ClaveProdServ="26121600" Cantidad="100" ClaveUnidad="MTR" Unidad="Metro" Descripcion="Cable THW 14" ValorUnitario="1.00" Importe="` + amt(sub) + `" ObjetoImp="02"/>
  </cfdi:Conceptos>
  <cfdi:Impuestos TotalImpuestosTrasladados="` + amt(total-sub) + `"/>
  <cfdi:Complemento>
    <tfd:TimbreFiscalDigital Version="1.1" UUID="` + uuid + `" FechaTimbrado="2026-11-12T10:01:00" RfcProvCertif="SPR190613I52" SelloCFD="abc" NoCertificadoSAT="00001000000505142236" SelloSAT="def"/>
  </cfdi:Complemento>
</cfdi:Comprobante>`)
}

// recordTestFactura records a manual factura for the proyecto's whole total, to the
// fixtures' customer RFC; P.U.E. ones paid on 2026-11-10.
func recordTestFactura(t *testing.T, a *Auth, userID int64, folio, metodo string) int64 {
	t.Helper()
	ctx := context.Background()
	project, _ := a.store.ProjectByFolio(ctx, folio)
	m := store.ManualFactura{XML: testCFDI("11111111-2222-3333-4444-"+fmt.Sprintf("%012d", project.ID), metodo, project.Total, "IIA040805DZ4")}
	if metodo == "PUE" {
		m.PaidOn = "2026-11-10"
	}
	id, err := a.store.RecordManualFactura(ctx, project.ID, userID, m)
	if err != nil {
		t.Fatalf("RecordManualFactura: %v", err)
	}
	return id
}

// postManual uploads a factura made outside the app to the proyecto.
func postManual(t *testing.T, a *Auth, f *Facturas, userID int64, folio string, fields map[string]string, xmlData, pdfData []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	if xmlData != nil {
		fw, _ := mw.CreateFormFile("xml", "A-153.xml")
		fw.Write(xmlData)
	}
	if pdfData != nil {
		fw, _ := mw.CreateFormFile("pdf", "A-153.pdf")
		fw.Write(pdfData)
	}
	mw.Close()
	req := httptest.NewRequest("POST", "/proyectos/"+folio+"/factura/manual", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.SetPathValue("folio", folio)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
	rec := httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(f.Manual)).ServeHTTP(rec, req)
	return rec
}

// #63's manual mode: a factura issued in the PAC's portal is recorded from its XML. P.U.E.
// needs the payment confirmed and records it in Cobro; P.P.D. leaves the complemento
// pending; what doesn't match the proyecto is warned about; marking it cancelled puts
// Facturación back to Pendiente and keeps the payment.
func TestManualFactura(t *testing.T) {
	a := newTestAuth(t)
	ctx := context.Background()
	admin := createTestUser(t, a, "gerry", "admin", "hunter2")
	f := NewFacturas(a.store, facturacion.New(a.store, nil, facturoporti.Status{Sandbox: true, Problem: "no está configurada"}), nil)
	folio := facturaFixture(t, a, admin)
	project, _ := a.store.ProjectByFolio(ctx, folio)
	pdfData := []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n")
	const uuid = "ABCDEF01-2222-3333-4444-555555555555"
	pue := testCFDI(strings.ToLower(uuid), "PUE", project.Total, "IIA040805DZ4")

	for name, c := range map[string]struct {
		fields map[string]string
		xml    []byte
		want   string
	}{
		"no xml":           {nil, nil, "es obligatorio"},
		"not a cfdi":       {nil, []byte("<?xml version=\"1.0\"?><nota/>"), "no es un CFDI timbrado"},
		"pdf as xml":       {nil, pdfData, "debe ser su XML"},
		"PUE without paid": {nil, pue, "confirma que ya se cobró"},
		"PUE future paid":  {map[string]string{"cobrado": "si", "fecha_pago": "2999-01-01"}, pue, "no puede ser futura"},
	} {
		if rec := postManual(t, a, f, admin, folio, c.fields, c.xml, nil); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), c.want) {
			t.Errorf("Manual(%s) = %d %q, want 400 with %q", name, rec.Code, rec.Body.String(), c.want)
		}
	}

	// P.U.E., paid: Facturada and Pagado, with its PDF.
	rec := postManual(t, a, f, admin, folio, map[string]string{"cobrado": "si", "fecha_pago": "2026-11-10"}, pue, pdfData)
	if rec.Code != http.StatusSeeOther || !strings.HasSuffix(rec.Header().Get("Location"), "?ok=registrada") {
		t.Fatalf("Manual(PUE) = %d: %s", rec.Code, rec.Body.String())
	}
	id := strings.TrimSuffix(strings.TrimPrefix(rec.Header().Get("Location"), "/facturas/"), "?ok=registrada")
	project, _ = a.store.ProjectByFolio(ctx, folio)
	if project.States.Facturacion != store.FacturacionFacturada || project.States.Cobro != store.CobroPagado {
		t.Errorf("after a P.U.E. manual factura: %+v", project.States)
	}
	if !strings.HasPrefix(project.LastBody, "Pagado el 10/11/2026 · Factura A-153 registrada (PUE, emitida el 12/11/2026) · Folio fiscal "+uuid) {
		t.Errorf("history = %q", project.LastBody)
	}
	payments, _ := a.store.ListPayments(ctx, project.ID)
	if len(payments) != 1 || payments[0].Source != store.PaymentSourceFactura || payments[0].Amount != project.Total || payments[0].PaidOn != "2026-11-10" {
		t.Errorf("payments = %+v", payments)
	}
	page := doForm(t, a, admin, f.Page, "GET", "/facturas/"+id+"?ok=registrada", map[string]string{"id": id}, nil, false).Body.String()
	for _, want := range []string{"Factura A-153", uuid, "fuera de la app, registrada a mano", "Factura registrada.", "Cable THW 14",
		"/archivos/", "Ver representación impresa", "Descargar XML", "Marcar como cancelada", "Registrada a mano con su XML"} {
		if !strings.Contains(page, want) {
			t.Errorf("manual factura page missing %q", want)
		}
	}
	if strings.Contains(page, "Revisa la factura") || strings.Contains(page, "Cancelar ante el SAT") || strings.Contains(page, "Crear factura que la sustituye") {
		t.Error("a matching manual factura shows warnings or the PAC's actions")
	}
	if r := doForm(t, a, admin, f.PDF, "GET", "/facturas/"+id+"/pdf", map[string]string{"id": id}, nil, false); r.Code != http.StatusOK || r.Header().Get("Content-Type") != "application/pdf" {
		t.Errorf("printed PDF = %d %s", r.Code, r.Header().Get("Content-Type"))
	}
	if list := doForm(t, a, admin, f.List, "GET", "/facturas", nil, nil, false).Body.String(); !strings.Contains(list, `href="/facturas/`+id+`"`) {
		t.Error("the Facturas table doesn't link the manual factura")
	}
	// A second factura while one is in force is refused.
	if rec := postManual(t, a, f, admin, folio, map[string]string{"cobrado": "si", "fecha_pago": "2026-11-10"}, pue, nil); rec.Code != http.StatusConflict {
		t.Errorf("Manual(twice) = %d, want 409", rec.Code)
	}

	// Marking it cancelled: motivo and why are required; then Pendiente, payment kept.
	pv := map[string]string{"id": id}
	if rec := doForm(t, a, admin, f.CancelarManual, "POST", "/facturas/"+id+"/cancelar-manual", pv, url.Values{"motivo": {"02"}}, false); rec.Code != http.StatusBadRequest {
		t.Errorf("CancelarManual(no nota) = %d, want 400", rec.Code)
	}
	if rec := doForm(t, a, admin, f.CancelarManual, "POST", "/facturas/"+id+"/cancelar-manual", pv, url.Values{"motivo": {"02"}, "nota": {"RFC equivocado"}}, false); rec.Code != http.StatusSeeOther {
		t.Fatalf("CancelarManual = %d: %s", rec.Code, rec.Body.String())
	}
	project, _ = a.store.ProjectByFolio(ctx, folio)
	if project.States.Facturacion != store.FacturacionPendiente || project.States.Cobro != store.CobroPagado ||
		!strings.Contains(project.LastBody, "marcada como cancelada") || !strings.Contains(project.LastBody, "RFC equivocado") {
		t.Errorf("after cancelling: %+v %q", project.States, project.LastBody)
	}

	// Its replacement, P.P.D. to another RFC: Pendiente complemento, warned, no payment.
	ppd := testCFDI("99999999-2222-3333-4444-555555555555", "PPD", project.Total+100, "XAXX010101000")
	rec = postManual(t, a, f, admin, folio, nil, ppd, nil)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Manual(PPD) = %d: %s", rec.Code, rec.Body.String())
	}
	id = strings.TrimSuffix(strings.TrimPrefix(rec.Header().Get("Location"), "/facturas/"), "?ok=registrada")
	project, _ = a.store.ProjectByFolio(ctx, folio)
	if project.States.Facturacion != store.FacturacionPendienteComplemento {
		t.Errorf("after a P.P.D. manual factura: %+v", project.States)
	}
	if payments, _ := a.store.ListPayments(ctx, project.ID); len(payments) != 1 {
		t.Errorf("a P.P.D. factura recorded a payment: %+v", payments)
	}
	page = doForm(t, a, admin, f.Page, "GET", "/facturas/"+id, map[string]string{"id": id}, nil, false).Body.String()
	for _, want := range []string{"Revisa la factura", "no coincide con el total con IVA de la cotización", "El RFC de la factura (XAXX010101000) no coincide"} {
		if !strings.Contains(page, want) {
			t.Errorf("mismatched manual factura page missing %q", want)
		}
	}
	// The same XML can't be recorded twice, even on another proyecto.
	if _, err := a.store.RecordManualFactura(ctx, project.ID, admin, store.ManualFactura{XML: pue, PaidOn: "2026-11-10"}); err == nil {
		t.Error("recorded a factura already on record")
	}
}

// testCFDIPago is a minimal stamped complemento de pago (Pagos 2.0) paying amount of the
// factura facturaUUID on day.
func testCFDIPago(uuid, facturaUUID string, amount money.Centavos, day string) []byte {
	amt := fmt.Sprintf("%d.%02d", int64(amount)/100, int64(amount)%100)
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<cfdi:Comprobante xmlns:cfdi="http://www.sat.gob.mx/cfd/4" xmlns:pago20="http://www.sat.gob.mx/Pagos20" xmlns:tfd="http://www.sat.gob.mx/TimbreFiscalDigital" Version="4.0" Serie="P" Folio="9" Fecha="` + day + `T12:00:00" SubTotal="0" Moneda="XXX" Total="0" TipoDeComprobante="P" Exportacion="01" LugarExpedicion="64000" Sello="abcdefghijkl" NoCertificado="00001000000500000000">
  <cfdi:Emisor Rfc="CLA010101AAA" Nombre="CLADEX" RegimenFiscal="601"/>
  <cfdi:Receptor Rfc="IIA040805DZ4" Nombre="INDISTRIA ILUMINADORA DE ALMACENES" DomicilioFiscalReceptor="62661" RegimenFiscalReceptor="601" UsoCFDI="CP01"/>
  <cfdi:Conceptos><cfdi:Concepto ClaveProdServ="84111506" Cantidad="1" ClaveUnidad="ACT" Descripcion="Pago" ValorUnitario="0" Importe="0" ObjetoImp="01"/></cfdi:Conceptos>
  <cfdi:Complemento>
    <pago20:Pagos Version="2.0">
      <pago20:Totales MontoTotalPagos="` + amt + `"/>
      <pago20:Pago FechaPago="` + day + `T12:00:00" FormaDePagoP="03" MonedaP="MXN" TipoCambioP="1" Monto="` + amt + `">
        <pago20:DoctoRelacionado IdDocumento="` + facturaUUID + `" MonedaDR="MXN" NumParcialidad="1" ImpSaldoAnt="0" ImpPagado="` + amt + `" ImpSaldoInsoluto="0" ObjetoImpDR="01"/>
      </pago20:Pago>
    </pago20:Pagos>
    <tfd:TimbreFiscalDigital Version="1.1" UUID="` + uuid + `" FechaTimbrado="` + day + `T12:01:00" RfcProvCertif="SPR190613I52" SelloCFD="abc" NoCertificadoSAT="00001000000505142236" SelloSAT="def"/>
  </cfdi:Complemento>
</cfdi:Comprobante>`)
}

// postComplemento uploads a complemento de pago made outside the app to a factura.
func postComplemento(t *testing.T, a *Auth, f *Facturas, userID int64, facturaID string, pago string, xmlData []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if pago != "" {
		mw.WriteField("pago", pago)
	}
	if xmlData != nil {
		fw, _ := mw.CreateFormFile("xml", "P-9.xml")
		fw.Write(xmlData)
	}
	mw.Close()
	req := httptest.NewRequest("POST", "/facturas/"+facturaID+"/complemento-manual", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.SetPathValue("id", facturaID)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
	rec := httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(f.ComplementoManual)).ServeHTTP(rec, req)
	return rec
}

// #63's parcialidades: a P.P.D. factura gets several complementos recorded by hand, each
// lowering its saldo, linked to a payment already in Cobro or recording a new one; once
// they cover the total the proyecto is Facturada. The Facturas table warns as the SAT's
// day-5 deadline for a payment without complemento nears.
func TestManualComplementos(t *testing.T) {
	a := newTestAuth(t)
	ctx := context.Background()
	admin := createTestUser(t, a, "gerry", "admin", "hunter2")
	f := NewFacturas(a.store, facturacion.New(a.store, nil, facturoporti.Status{Sandbox: true, Problem: "no está configurada"}), nil)
	folio := facturaFixture(t, a, admin)
	project, _ := a.store.ProjectByFolio(ctx, folio)
	total := project.Total
	half := total / 2

	// The anticipo, recorded in Cobro by hand before the factura.
	if err := a.store.RecordPayment(ctx, project.ID, admin, half, "2026-11-01", nil); err != nil {
		t.Fatal(err)
	}
	fid := recordTestFactura(t, a, admin, folio, "PPD")
	factura, _ := a.store.ComprobanteByID(ctx, fid)
	id := strconv.FormatInt(fid, 10)
	pv := map[string]string{"id": id}

	// The table warns as the anticipo's complemento deadline (05/12/2026) nears.
	rows, _ := a.store.ListBilling(ctx, "", "", nil)
	if len(rows) != 1 || rows[0].UncoveredPaymentSince != "2026-11-01" {
		t.Fatalf("billing rows = %+v", rows)
	}
	for today, want := range map[string]string{"2026-11-15": "", "2026-12-02": "Complemento vence el 05/12/2026",
		"2026-12-06": "Complemento vencido (05/12/2026)"} {
		if got := views.ComplementoWarning(rows[0], today); got != want {
			t.Errorf("ComplementoWarning(%s) = %q, want %q", today, got, want)
		}
	}

	payments, _ := a.store.ListPayments(ctx, project.ID)
	anticipo := strconv.FormatInt(payments[0].ID, 10)
	page := doForm(t, a, admin, f.Page, "GET", "/facturas/"+id, pv, nil, false).Body.String()
	for _, want := range []string{"Complementos de pago", "Todavía no tiene complementos de pago.", "Saldo pendiente",
		"Registrar complemento de pago", `name="pago" value="` + anticipo + `"`, "Sí, el pago de " + half.String() + " del 01/11/2026"} {
		if !strings.Contains(page, want) {
			t.Errorf("PPD factura page missing %q", want)
		}
	}

	first := testCFDIPago("C0000001-2222-3333-4444-555555555555", factura.UUID, half, "2026-11-01")
	for name, c := range map[string]struct {
		pago string
		xml  []byte
		want string
	}{
		"no choice":     {"", first, "elige si el pago ya está registrado"},
		"a factura":     {"nuevo", testCFDI("C0000009-2222-3333-4444-555555555555", "PPD", total, "IIA040805DZ4"), "no es un complemento de pago"},
		"other factura": {"nuevo", testCFDIPago("C0000002-2222-3333-4444-555555555555", "00000000-0000-0000-0000-000000000000", half, "2026-11-01"), "no paga esta factura"},
		"too much":      {"nuevo", testCFDIPago("C0000003-2222-3333-4444-555555555555", factura.UUID, total+1, "2026-11-01"), "más que el saldo de la factura"},
		"wrong amount":  {anticipo, testCFDIPago("C0000004-2222-3333-4444-555555555555", factura.UUID, half-1, "2026-11-01"), "debe ser del mismo monto"},
	} {
		if rec := postComplemento(t, a, f, admin, id, c.pago, c.xml); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), c.want) {
			t.Errorf("ComplementoManual(%s) = %d %q, want 400 with %q", name, rec.Code, rec.Body.String(), c.want)
		}
	}

	// The first complemento covers the anticipo: linked, not a second payment.
	if rec := postComplemento(t, a, f, admin, id, anticipo, first); rec.Code != http.StatusSeeOther {
		t.Fatalf("ComplementoManual(linked) = %d: %s", rec.Code, rec.Body.String())
	}
	project, _ = a.store.ProjectByFolio(ctx, folio)
	payments, _ = a.store.ListPayments(ctx, project.ID)
	if len(payments) != 1 || payments[0].ComprobanteID == 0 || project.States.Facturacion != store.FacturacionPendienteComplemento {
		t.Errorf("after the linked complemento: %+v %+v", payments, project.States)
	}
	if !strings.Contains(project.LastBody, "Complemento de pago P-9 registrado: "+half.String()+" pagado el 01/11/2026") ||
		!strings.Contains(project.LastBody, "Ligado al pago ya registrado") || !strings.Contains(project.LastBody, "Saldo de la factura A-153: "+(total-half).String()) {
		t.Errorf("history = %q", project.LastBody)
	}
	if rows, _ := a.store.ListBilling(ctx, "", "", nil); rows[0].UncoveredPaymentSince != "" {
		t.Error("a payment covered by its complemento still counts as uncovered")
	}

	// A factura with a complemento in force can't be marked cancelled.
	if rec := doForm(t, a, admin, f.CancelarManual, "POST", "/facturas/"+id+"/cancelar-manual", pv, url.Values{"motivo": {"02"}, "nota": {"x"}}, false); rec.Code != http.StatusConflict {
		t.Errorf("CancelarManual(factura with complemento) = %d, want 409", rec.Code)
	}

	// The second covers the rest with a new payment: Facturada and Pagado.
	second := testCFDIPago("C0000005-2222-3333-4444-555555555555", factura.UUID, total-half, "2026-11-20")
	rec := postComplemento(t, a, f, admin, id, "nuevo", second)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("ComplementoManual(new) = %d: %s", rec.Code, rec.Body.String())
	}
	pid := strings.TrimSuffix(strings.TrimPrefix(rec.Header().Get("Location"), "/facturas/"), "?ok=complemento")
	project, _ = a.store.ProjectByFolio(ctx, folio)
	if project.States.Facturacion != store.FacturacionFacturada || project.States.Cobro != store.CobroPagado {
		t.Errorf("after covering the factura: %+v", project.States)
	}
	payments, _ = a.store.ListPayments(ctx, project.ID)
	if len(payments) != 2 || payments[1].Source != store.PaymentSourceComplemento || payments[1].PaidOn != "2026-11-20" {
		t.Errorf("payments = %+v", payments)
	}
	page = doForm(t, a, admin, f.Page, "GET", "/facturas/"+id, pv, nil, false).Body.String()
	if !strings.Contains(page, "Comprobante de pago P-9") || strings.Contains(page, "Registrar complemento de pago") {
		t.Logf("has P-9 %v; has form %v; saldo idx %d", strings.Contains(page, "Complemento de pago P-9"), strings.Contains(page, "Registrar complemento de pago"), strings.Index(page, "Saldo pendiente"))
		t.Log(page[strings.Index(page, "Complementos de pago"):][:1500])
		t.Error("a fully paid factura doesn't list its complementos, or still offers another")
	}
	ppage := doForm(t, a, admin, f.Page, "GET", "/facturas/"+pid, map[string]string{"id": pid}, nil, false).Body.String()
	for _, want := range []string{"Fecha del pago", "20/11/2026", "Monto pagado", "Paga la factura", "Marcar como cancelado"} {
		if !strings.Contains(ppage, want) {
			t.Errorf("complemento page missing %q", want)
		}
	}

	// Marking the second cancelled reopens the saldo; its payment stays.
	if rec := doForm(t, a, admin, f.CancelarManual, "POST", "/facturas/"+pid+"/cancelar-manual", map[string]string{"id": pid},
		url.Values{"motivo": {"02"}, "nota": {"Monto equivocado"}}, false); rec.Code != http.StatusSeeOther {
		t.Fatalf("CancelarManual(complemento) = %d: %s", rec.Code, rec.Body.String())
	}
	project, _ = a.store.ProjectByFolio(ctx, folio)
	if project.States.Facturacion != store.FacturacionPendienteComplemento || project.States.Cobro != store.CobroPagado ||
		!strings.Contains(project.LastBody, "Complemento de pago P-9 marcado como cancelado") {
		t.Errorf("after cancelling the complemento: %+v %q", project.States, project.LastBody)
	}
}
