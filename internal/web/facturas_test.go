package web

import (
	"context"
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
	"github.com/gerrygoo/cladex-web/internal/store"
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
	if rec := postOC(t, a, p, admin, folio, map[string]string{"oc_numero": "4411", "oc_fecha": "2026-10-07", "forma_pago": "PPD"},
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
	if body := projectPage(); !strings.Contains(body, "Crear factura") || strings.Contains(body, "Registrar factura de anticipo") {
		t.Fatalf("project page should offer Crear factura instead of the typed form")
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
	body := page(admin, id)
	for _, want := range []string{"Factura (borrador)", "Para validarla falta", "falta la clave de producto SAT", "falta la clave de unidad SAT",
		"99 - Por definir", "compras@peme.mx", "IIA040805DZ4", "Pruebas: sin validez fiscal"} {
		if !strings.Contains(body, want) {
			t.Errorf("draft page missing %q", want)
		}
	}
	c, _ := a.store.ComprobanteByID(ctx, mustAtoi(t, id))
	line := strconv.FormatInt(c.Conceptos[0].ID, 10)
	form := url.Values{"uso_cfdi": {"G01"}, "recipients": {"compras@peme.mx"}, "concepto_id": {line},
		"sat_product_" + line: {"26121600"}, "sat_unit_" + line: {"mtr"}, "unit_name_" + line: {"Metro"}, "description_" + line: {"Cable THW 14"}}
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
	if c.Status != store.CompTimbrada || c.UUID == "" || !c.EmailSent || project.Status != "facturado" ||
		project.PaymentStatus != store.PaymentAdvanceInvoiced || project.Invoice.Ref != "F-1" {
		t.Fatalf("after stamp: %s %s; project %s %s %q", c.Status, c.UUID, project.Status, project.PaymentStatus, project.Invoice.Ref)
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

	// The comprobante de pago: stamping it leaves the proyecto pagado.
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
	if project.PaymentStatus != store.PaymentPaid || project.PaymentRef != "P-1" {
		t.Errorf("after pago: %s %q", project.PaymentStatus, project.PaymentRef)
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
	if project.Invoice.Ref != "F-2" {
		t.Errorf("project invoice ref = %q, want F-2", project.Invoice.Ref)
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
	if list := doForm(t, a, vendedor, f.List, "GET", "/facturas", nil, nil, false).Body.String(); !strings.Contains(list, "Factura F-2") ||
		!strings.Contains(list, "Comprobante de pago P-1") || !strings.Contains(list, "Cancelada") {
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
	if err := a.store.UpdateDraft(ctx, id, admin, store.DraftEdit{UsoCFDI: "G01", FormaPago: "99", Conceptos: []store.ConceptoEdit{
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

// With facturación off, the proyecto keeps the typed factura form, and stamping refuses.
func TestFacturacionOff(t *testing.T) {
	a := newTestAuth(t)
	admin := createTestUser(t, a, "gerry", "admin", "hunter2")
	folio := facturaFixture(t, a, admin)
	p := NewProjects(a.store)
	p.fact = facturacion.New(a.store, nil, facturoporti.Status{Sandbox: true, Problem: "no está configurada"})
	body := doForm(t, a, admin, p.Page, "GET", "/proyectos/"+folio, map[string]string{"folio": folio}, nil, false).Body.String()
	if !strings.Contains(body, "Registrar factura de anticipo") || strings.Contains(body, "Crear factura") {
		t.Error("facturación off should keep the typed form")
	}
	if _, err := p.fact.Stamp(context.Background(), 1, admin); err == nil || !strings.Contains(err.Error(), "apagada") {
		t.Errorf("Stamp with facturación off = %v", err)
	}
}
