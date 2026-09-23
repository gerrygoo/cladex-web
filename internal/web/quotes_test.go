package web

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/pricing"
	"github.com/gerrygoo/cladex-web/internal/store"
)

func newTestQuotes(t *testing.T, a *Auth) *Quotes {
	t.Helper()
	return NewQuotes(a.store, nil)
}

func seedPricingSettings(t *testing.T, a *Auth, userID int64) {
	t.Helper()
	set := func(key, val string) {
		m, err := money.ParseMicros(val)
		if err != nil {
			t.Fatalf("ParseMicros(%q): %v", val, err)
		}
		if err := a.store.SetSetting(context.Background(), key, strconv.FormatInt(int64(m), 10), userID); err != nil {
			t.Fatalf("SetSetting(%s): %v", key, err)
		}
	}
	set("fx_rate", "18.50")
	set("copper_price", "145.30")
	set("default_margin", "0.30")
}

// seedQuoteBuilderFixtures creates a customer and two products (flat-priced and
// cost+margin) usable by the quote-builder tests.
func seedQuoteBuilderFixtures(t *testing.T, a *Auth) (customerID, flatProductID, costProductID int64) {
	t.Helper()
	ctx := context.Background()
	customerID, err := a.store.CreateCustomer(ctx, store.Customer{Name: "Grupo PEME"})
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	familyID, err := a.store.UpsertFamily(ctx, "ABASTILUM", "")
	if err != nil {
		t.Fatalf("UpsertFamily: %v", err)
	}
	flatPrice := money.Micros(100_000_000) // $100.00
	flatProductID, err = a.store.CreateProduct(ctx, store.Product{
		FamilyID: familyID, SKU: "abl-foco", Description: "Foco LED", UnitPriceMicros: &flatPrice,
	})
	if err != nil {
		t.Fatalf("CreateProduct(flat): %v", err)
	}
	cost := money.Micros(45_000_000) // $45.00 cost
	costProductID, err = a.store.CreateProduct(ctx, store.Product{
		FamilyID: familyID, SKU: "cca-c14", Description: "Cable THW 14", CostMicros: &cost,
	})
	if err != nil {
		t.Fatalf("CreateProduct(cost): %v", err)
	}
	return customerID, flatProductID, costProductID
}

func doForm(t *testing.T, a *Auth, userID int64, handler http.HandlerFunc, method, path string, pathValues map[string]string, form url.Values, hx bool) *httptest.ResponseRecorder {
	t.Helper()
	var body strings.Reader
	if form != nil {
		body = *strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, path, &body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for k, v := range pathValues {
		req.SetPathValue(k, v)
	}
	if hx {
		req.Header.Set("HX-Request", "true")
	}
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
	rec := httptest.NewRecorder()
	a.RequireAuth(handler).ServeHTTP(rec, req)
	return rec
}

func TestQuotesCreateBuildAndSave(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	customerID, flatProductID, costProductID := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, userID)

	// Create — plain POST + redirect, no htmx (JS-disabled path).
	rec := doForm(t, a, userID, q.Create, "POST", "/cotizaciones/nueva", nil, url.Values{
		"customer_id": {strconv.FormatInt(customerID, 10)},
		"prefix":      {"QA"},
	}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Create status = %d, want 303; body = %s", rec.Code, rec.Body.String())
	}
	folio := strings.TrimPrefix(rec.Header().Get("Location"), "/cotizaciones/")
	if folio != "QA0001" {
		t.Fatalf("folio = %q, want QA0001", folio)
	}

	quote, err := a.store.QuoteByFolio(context.Background(), folio)
	if err != nil || quote == nil {
		t.Fatalf("QuoteByFolio: %+v, %v", quote, err)
	}
	if quote.CustomerID != customerID || quote.Status != "borrador" {
		t.Fatalf("new draft = %+v", quote)
	}

	// Recalcular: add the flat product — no other lines yet.
	rec = doForm(t, a, userID, q.Recalcular, "POST", "/cotizaciones/"+folio+"/recalcular",
		map[string]string{"folio": folio},
		url.Values{"line_keys": {""}, "add_product_id": {strconv.FormatInt(flatProductID, 10)}}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("Recalcular(add flat) status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "abl-foco") {
		t.Fatalf("Recalcular body missing added product: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "<html") {
		t.Fatalf("htmx Recalcular response should be a fragment, not a full page: %s", rec.Body.String())
	}

	// Recalcular again: qty=2 on the flat line, plus a free-text line.
	form := url.Values{
		"line_keys":            {"0"},
		"lines[0][kind]":       {"product"},
		"lines[0][product_id]": {strconv.FormatInt(flatProductID, 10)},
		"lines[0][qty]":        {"2"},
		"add_free":             {"1"},
	}
	rec = doForm(t, a, userID, q.Recalcular, "POST", "/cotizaciones/"+folio+"/recalcular",
		map[string]string{"folio": folio}, form, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("Recalcular(qty+free) status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `name="lines[1][description]"`) {
		t.Fatalf("Recalcular body missing the newly-added free line: %s", rec.Body.String())
	}

	// Fill in the free line and the cost+margin product; compute the expected totals
	// independently via internal/pricing to check the handler wired everything
	// correctly (settings load, product lookup, price-break lookup).
	settings := pricing.Settings{FXRate: money.Micros(18_500_000), CopperPrice: money.Micros(145_300_000), DefaultMargin: money.Micros(300_000)}
	flatUnitPrice, err := pricing.UnitPrice(pricing.Product{UnitPriceMicros: &[]money.Micros{100_000_000}[0], Currency: "MXN"}, money.Milli(2_000), nil, settings)
	if err != nil {
		t.Fatalf("pricing.UnitPrice(flat): %v", err)
	}
	freePrice := money.Micros(50_000_000) // $50.00
	freeQty := money.Milli(3_000)
	wantSubtotal := money.LineTotalCentavos(flatUnitPrice, money.Milli(2_000)) + money.LineTotalCentavos(freePrice, freeQty)
	wantIVA := money.ApplyRate(wantSubtotal, pricing.IVARate)
	wantTotal := wantSubtotal + wantIVA

	form = url.Values{
		"line_keys":             {"0,1"},
		"lines[0][kind]":        {"product"},
		"lines[0][product_id]":  {strconv.FormatInt(flatProductID, 10)},
		"lines[0][qty]":         {"2"},
		"lines[1][kind]":        {"free"},
		"lines[1][description]": {"Instalación"},
		"lines[1][qty]":         {"3"},
		"lines[1][unit_price]":  {"50.00"},
	}
	rec = doForm(t, a, userID, q.Guardar, "POST", "/cotizaciones/"+folio+"/guardar",
		map[string]string{"folio": folio}, form, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Guardar status = %d, want 303; body = %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/cotizaciones/"+folio+"?guardado=1" {
		t.Fatalf("Guardar Location = %q", loc)
	}

	lines, err := a.store.ListQuoteLines(context.Background(), quote.ID)
	if err != nil {
		t.Fatalf("ListQuoteLines: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("ListQuoteLines = %d lines, want 2: %+v", len(lines), lines)
	}
	if lines[0].ProductID == nil || *lines[0].ProductID != flatProductID || lines[0].QtyMilli != money.Milli(2_000) {
		t.Fatalf("line 0 = %+v", lines[0])
	}
	if lines[1].ProductID != nil || lines[1].DescriptionSnapshot != "Instalación" || lines[1].UnitPriceMicros != freePrice {
		t.Fatalf("line 1 = %+v", lines[1])
	}

	reloaded, err := a.store.QuoteByID(context.Background(), quote.ID)
	if err != nil {
		t.Fatalf("QuoteByID: %v", err)
	}
	if reloaded.Subtotal != wantSubtotal || reloaded.IVA != wantIVA || reloaded.Total != wantTotal {
		t.Fatalf("totals = %+v, want subtotal=%v iva=%v total=%v", reloaded, wantSubtotal, wantIVA, wantTotal)
	}

	// Builder page reload (fresh request, not the POST response) reflects the saved
	// state — proving persistence, not just an in-memory render.
	rec = doForm(t, a, userID, q.Builder, "GET", "/cotizaciones/"+folio, map[string]string{"folio": folio}, nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("Builder reload status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Instalación") || !strings.Contains(rec.Body.String(), "abl-foco") {
		t.Fatalf("Builder reload missing persisted lines: %s", rec.Body.String())
	}

	// Removing the flat line and re-saving leaves exactly the free line — no stale row.
	form = url.Values{
		"line_keys":             {"0,1"},
		"lines[0][kind]":        {"product"},
		"lines[0][product_id]":  {strconv.FormatInt(flatProductID, 10)},
		"lines[0][qty]":         {"2"},
		"lines[1][kind]":        {"free"},
		"lines[1][description]": {"Instalación"},
		"lines[1][qty]":         {"3"},
		"lines[1][unit_price]":  {"50.00"},
		"remove_key":            {"0"},
	}
	rec = doForm(t, a, userID, q.Guardar, "POST", "/cotizaciones/"+folio+"/guardar",
		map[string]string{"folio": folio}, form, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Guardar(after remove) status = %d, want 303; body = %s", rec.Code, rec.Body.String())
	}
	lines, err = a.store.ListQuoteLines(context.Background(), quote.ID)
	if err != nil || len(lines) != 1 {
		t.Fatalf("ListQuoteLines after remove = %d, %v; want 1", len(lines), err)
	}

	// Reference the second-product fixture so the cost+margin product isn't unused —
	// used indirectly via seedQuoteBuilderFixtures's return, kept here so a future test
	// extension has it in scope.
	_ = costProductID
}

func TestQuotesGuardarRejectsInvalidLine(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	customerID, _, _ := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, userID)

	quote, err := a.store.CreateDraftQuote(context.Background(), customerID, userID, "QS")
	if err != nil {
		t.Fatalf("CreateDraftQuote: %v", err)
	}

	form := url.Values{
		"line_keys":             {"0"},
		"lines[0][kind]":        {"free"},
		"lines[0][description]": {"Servicio"},
		"lines[0][qty]":         {"no-es-un-numero"},
		"lines[0][unit_price]":  {"10.00"},
	}
	rec := doForm(t, a, userID, q.Guardar, "POST", "/cotizaciones/"+quote.Folio+"/guardar",
		map[string]string{"folio": quote.Folio}, form, false)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("Guardar(invalid qty) status = %d, want 422; body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Cantidad inválida") {
		t.Fatalf("body missing row error: %s", rec.Body.String())
	}

	lines, err := a.store.ListQuoteLines(context.Background(), quote.ID)
	if err != nil || len(lines) != 0 {
		t.Fatalf("ListQuoteLines after rejected save = %d, %v; want 0 (nothing persisted)", len(lines), err)
	}
}

// TestQuotesPDF requires the `typst` CLI (part of the documented local dev toolchain,
// see docs/PLAN.md 0.0a).
func TestQuotesPDF(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	customerID, flatProductID, _ := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, userID)

	quote, err := a.store.CreateDraftQuote(context.Background(), customerID, userID, "QA")
	if err != nil {
		t.Fatalf("CreateDraftQuote: %v", err)
	}
	totals := pricing.ComputeTotals([]pricing.Line{{UnitPriceMicros: money.Micros(100_000_000), QtyMilli: money.Milli(1_000)}})
	err = a.store.ReplaceQuoteLines(context.Background(), quote.ID, []store.QuoteLine{
		{ProductID: &flatProductID, DescriptionSnapshot: "Foco LED", QtyMilli: money.Milli(1_000), UnitPriceMicros: money.Micros(100_000_000), LineTotal: money.LineTotalCentavos(money.Micros(100_000_000), money.Milli(1_000)), Source: "manual"},
	}, totals)
	if err != nil {
		t.Fatalf("ReplaceQuoteLines: %v", err)
	}

	rec := doForm(t, a, userID, q.PDF, "GET", "/cotizaciones/"+quote.Folio+"/pdf",
		map[string]string{"folio": quote.Folio}, nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("PDF status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("Content-Type = %q, want application/pdf", ct)
	}
	if !strings.HasPrefix(rec.Body.String(), "%PDF") {
		t.Fatalf("body doesn't look like a PDF")
	}
}

// TestQuotesEmitir requires the `typst` CLI (part of the documented local dev
// toolchain, see docs/PLAN.md 0.0a).
func TestQuotesEmitir(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	customerID, flatProductID, _ := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, userID)

	quote, err := a.store.CreateDraftQuote(context.Background(), customerID, userID, "QA")
	if err != nil {
		t.Fatalf("CreateDraftQuote: %v", err)
	}

	form := url.Values{
		"line_keys":            {"0"},
		"lines[0][kind]":       {"product"},
		"lines[0][product_id]": {strconv.FormatInt(flatProductID, 10)},
		"lines[0][qty]":        {"2"},
	}
	rec := doForm(t, a, userID, q.Emitir, "POST", "/cotizaciones/"+quote.Folio+"/emitir",
		map[string]string{"folio": quote.Folio}, form, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Emitir status = %d, want 303; body = %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/cotizaciones/"+quote.Folio+"?emitida=1" {
		t.Fatalf("Emitir Location = %q", loc)
	}

	issued, err := a.store.QuoteByID(context.Background(), quote.ID)
	if err != nil {
		t.Fatalf("QuoteByID: %v", err)
	}
	if issued.Status != "emitida" {
		t.Fatalf("Status = %q, want emitida", issued.Status)
	}
	if issued.IssuedAt == nil || issued.FxRateUsedMicros == nil || issued.TermsSnapshot == nil || issued.ValidUntil == nil {
		t.Fatalf("issue didn't freeze all expected fields: %+v", issued)
	}
	if issued.CustomerNameSnapshot == nil || issued.VendedorSnapshot == nil || issued.PDFSHA256 == nil {
		t.Fatalf("issue didn't record name snapshots/hash: %+v", issued)
	}

	// Nothing is stored; the PDF is regenerated, and the first reprint already hashes to
	// what Emitir recorded.
	reprint := func() string {
		t.Helper()
		rec := doForm(t, a, userID, q.PDF, "GET", "/cotizaciones/"+quote.Folio+"/pdf",
			map[string]string{"folio": quote.Folio}, nil, false)
		if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Body.String(), "%PDF") {
			t.Fatalf("reprint status = %d, body doesn't look like a PDF", rec.Code)
		}
		return rec.Body.String()
	}
	first := reprint()
	sum := sha256.Sum256([]byte(first))
	if hex.EncodeToString(sum[:]) != *issued.PDFSHA256 {
		t.Fatal("regenerated PDF's SHA-256 doesn't match the hash recorded at issue")
	}

	// Once issued, Guardar refuses to touch it (immutability).
	rec = doForm(t, a, userID, q.Guardar, "POST", "/cotizaciones/"+quote.Folio+"/guardar",
		map[string]string{"folio": quote.Folio}, form, false)
	if rec.Code != http.StatusConflict {
		t.Fatalf("Guardar(issued quote) status = %d, want 409", rec.Code)
	}

	// Issuing again is refused too — a quote is issued exactly once.
	rec = doForm(t, a, userID, q.Emitir, "POST", "/cotizaciones/"+quote.Folio+"/emitir",
		map[string]string{"folio": quote.Folio}, form, false)
	if rec.Code != http.StatusConflict {
		t.Fatalf("Emitir(already issued) status = %d, want 409", rec.Code)
	}

	// Reprint stays byte-identical after everything live that fed the draft changes:
	// settings, the quoted product's price, the customer's name.
	if err := a.store.SetSetting(context.Background(), "fx_rate", "99000000", userID); err != nil {
		t.Fatalf("SetSetting(fx_rate): %v", err)
	}
	if err := a.store.SetSetting(context.Background(), "copper_price", "999000000", userID); err != nil {
		t.Fatalf("SetSetting(copper_price): %v", err)
	}
	product, err := a.store.ProductByID(context.Background(), flatProductID)
	if err != nil || product == nil {
		t.Fatalf("ProductByID: %+v, %v", product, err)
	}
	newPrice := money.Micros(999_000_000)
	product.UnitPriceMicros = &newPrice
	if err := a.store.UpdateProduct(context.Background(), *product); err != nil {
		t.Fatalf("UpdateProduct: %v", err)
	}
	customer, err := a.store.CustomerByID(context.Background(), customerID)
	if err != nil || customer == nil {
		t.Fatalf("CustomerByID: %+v, %v", customer, err)
	}
	customer.Name = "Nombre Nuevo S.A."
	if err := a.store.UpdateCustomer(context.Background(), *customer); err != nil {
		t.Fatalf("UpdateCustomer: %v", err)
	}
	if reprint() != first {
		t.Fatal("reprint after changing settings, product price and customer name is not byte-identical")
	}
}

// TestQuotesIssuedPageShowsFrozenLines guards the on-screen half of issue-time
// immutability: an issued quote's page must show the prices and descriptions the
// customer received, not a live recompute — neither a settings change nor deleting a
// product it quoted may alter it.
func TestQuotesIssuedPageShowsFrozenLines(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	customerID, _, costProductID := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, userID) // margin 0.30: $45.00 / 0.70 = $64.29

	quote, err := a.store.CreateDraftQuote(context.Background(), customerID, userID, "QA")
	if err != nil {
		t.Fatalf("CreateDraftQuote: %v", err)
	}
	form := url.Values{
		"line_keys":            {"0"},
		"lines[0][kind]":       {"product"},
		"lines[0][product_id]": {strconv.FormatInt(costProductID, 10)},
		"lines[0][qty]":        {"1"},
	}
	rec := doForm(t, a, userID, q.Emitir, "POST", "/cotizaciones/"+quote.Folio+"/emitir",
		map[string]string{"folio": quote.Folio}, form, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Emitir status = %d; body = %s", rec.Code, rec.Body.String())
	}

	// Margin 0.50 would reprice the line to $90.00; deleting the product would make a
	// live recompute drop it altogether.
	if err := a.store.SetSetting(context.Background(), "default_margin", "500000", userID); err != nil {
		t.Fatalf("SetSetting(default_margin): %v", err)
	}
	if err := a.store.SoftDeleteProduct(context.Background(), costProductID); err != nil {
		t.Fatalf("SoftDeleteProduct: %v", err)
	}

	rec = doForm(t, a, userID, q.Builder, "GET", "/cotizaciones/"+quote.Folio,
		map[string]string{"folio": quote.Folio}, nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("Builder status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Cable THW 14", "64.29", "74.58"} { // description, unit price, total incl. IVA
		if !strings.Contains(body, want) {
			t.Errorf("issued quote page missing frozen %q", want)
		}
	}
	if strings.Contains(body, "90.00") {
		t.Error("issued quote page shows a live-repriced line ($90.00)")
	}

	// Recalcular never writes, but it must not render an edited view of an issued quote
	// either.
	rec = doForm(t, a, userID, q.Recalcular, "POST", "/cotizaciones/"+quote.Folio+"/recalcular",
		map[string]string{"folio": quote.Folio}, form, true)
	if rec.Code != http.StatusConflict {
		t.Fatalf("Recalcular(issued quote) status = %d, want 409", rec.Code)
	}
}

func TestQuotesEmitirRejectsInvalidLine(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	customerID, _, _ := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, userID)

	quote, err := a.store.CreateDraftQuote(context.Background(), customerID, userID, "QA")
	if err != nil {
		t.Fatalf("CreateDraftQuote: %v", err)
	}
	form := url.Values{
		"line_keys":             {"0"},
		"lines[0][kind]":        {"free"},
		"lines[0][description]": {"Servicio"},
		"lines[0][qty]":         {"no-es-un-numero"},
		"lines[0][unit_price]":  {"10.00"},
	}
	rec := doForm(t, a, userID, q.Emitir, "POST", "/cotizaciones/"+quote.Folio+"/emitir",
		map[string]string{"folio": quote.Folio}, form, false)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("Emitir(invalid line) status = %d, want 422", rec.Code)
	}
	reloaded, err := a.store.QuoteByID(context.Background(), quote.ID)
	if err != nil || reloaded.Status != "borrador" {
		t.Fatalf("quote should remain a draft after a rejected Emitir: %+v, %v", reloaded, err)
	}
}

// TestQuotesRevisar requires the `typst` CLI (see TestQuotesEmitir).
func TestQuotesRevisar(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	customerID, flatProductID, _ := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, userID)

	quote, err := a.store.CreateDraftQuote(context.Background(), customerID, userID, "QA")
	if err != nil {
		t.Fatalf("CreateDraftQuote: %v", err)
	}

	// Revising a draft (never issued) is refused.
	rec := doForm(t, a, userID, q.Revisar, "POST", "/cotizaciones/"+quote.Folio+"/revisar",
		map[string]string{"folio": quote.Folio}, url.Values{}, false)
	if rec.Code != http.StatusConflict {
		t.Fatalf("Revisar(draft) status = %d, want 409", rec.Code)
	}

	form := url.Values{
		"line_keys":            {"0"},
		"lines[0][kind]":       {"product"},
		"lines[0][product_id]": {strconv.FormatInt(flatProductID, 10)},
		"lines[0][qty]":        {"1"},
	}
	rec = doForm(t, a, userID, q.Emitir, "POST", "/cotizaciones/"+quote.Folio+"/emitir",
		map[string]string{"folio": quote.Folio}, form, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Emitir status = %d, want 303; body = %s", rec.Code, rec.Body.String())
	}
	beforeRevision, err := a.store.QuoteByID(context.Background(), quote.ID)
	if err != nil {
		t.Fatalf("QuoteByID: %v", err)
	}

	rec = doForm(t, a, userID, q.Revisar, "POST", "/cotizaciones/"+quote.Folio+"/revisar",
		map[string]string{"folio": quote.Folio}, url.Values{}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Revisar status = %d, want 303; body = %s", rec.Code, rec.Body.String())
	}
	wantFolio := quote.Folio + "-R1"
	if loc := rec.Header().Get("Location"); loc != "/cotizaciones/"+wantFolio {
		t.Fatalf("Revisar Location = %q, want /cotizaciones/%s", loc, wantFolio)
	}

	rev, err := a.store.QuoteByFolio(context.Background(), wantFolio)
	if err != nil || rev == nil {
		t.Fatalf("QuoteByFolio(%s): %+v, %v", wantFolio, rev, err)
	}
	if rev.Status != "borrador" || rev.SupersedesQuoteID == nil || *rev.SupersedesQuoteID != quote.ID {
		t.Fatalf("revision = %+v", rev)
	}
	revLines, err := a.store.ListQuoteLines(context.Background(), rev.ID)
	if err != nil || len(revLines) != 1 {
		t.Fatalf("revision lines = %+v, %v", revLines, err)
	}

	// The original is untouched (same PDF/hash/totals as right after issuing) except
	// for its status flip to 'revisada'.
	original, err := a.store.QuoteByID(context.Background(), quote.ID)
	if err != nil {
		t.Fatalf("QuoteByID(original): %v", err)
	}
	if original.Status != "revisada" {
		t.Fatalf("original.Status = %q, want revisada", original.Status)
	}
	if original.SupersededByFolio != wantFolio {
		t.Fatalf("original.SupersededByFolio = %q, want %q", original.SupersededByFolio, wantFolio)
	}
	if *original.PDFSHA256 != *beforeRevision.PDFSHA256 || original.Total != beforeRevision.Total {
		t.Fatalf("original quote's own data changed after revising: before=%+v after=%+v", beforeRevision, original)
	}

	// Revising the (now-superseded) original again is refused — only the active
	// revision can be revised further.
	rec = doForm(t, a, userID, q.Revisar, "POST", "/cotizaciones/"+quote.Folio+"/revisar",
		map[string]string{"folio": quote.Folio}, url.Values{}, false)
	if rec.Code != http.StatusConflict {
		t.Fatalf("Revisar(revisada) status = %d, want 409", rec.Code)
	}
}

func TestQuotesBuscarProductosHTMXFragment(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	customerID, flatProductID, _ := seedQuoteBuilderFixtures(t, a)
	_ = flatProductID

	quote, err := a.store.CreateDraftQuote(context.Background(), customerID, userID, "QI")
	if err != nil {
		t.Fatalf("CreateDraftQuote: %v", err)
	}

	rec := doForm(t, a, userID, q.BuscarProductos, "GET", "/cotizaciones/"+quote.Folio+"/productos?q=foco",
		map[string]string{"folio": quote.Folio}, nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("BuscarProductos status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "<html") {
		t.Fatalf("BuscarProductos response should be a fragment: %s", body)
	}
	if !strings.Contains(body, "abl-foco") {
		t.Fatalf("BuscarProductos missing matching product: %s", body)
	}
}

// TestQuotesBuscarProductosFamilyFilter covers the picker's "solo productos de la
// familia" toggle: on by default (a QA quote only sees CCA products), off when the
// form sends only the hidden solo_familia=0.
func TestQuotesBuscarProductosFamilyFilter(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	customerID, _, _ := seedQuoteBuilderFixtures(t, a)
	ctx := context.Background()
	ccaFamilyID, err := a.store.UpsertFamily(ctx, "CCA", "CCA")
	if err != nil {
		t.Fatalf("UpsertFamily(CCA): %v", err)
	}
	cost := money.Micros(45_000_000)
	if _, err := a.store.CreateProduct(ctx, store.Product{
		FamilyID: ccaFamilyID, SKU: "cca-c12", Description: "Cable CCA 12", CostMicros: &cost,
	}); err != nil {
		t.Fatalf("CreateProduct(cca): %v", err)
	}
	quote, err := a.store.CreateDraftQuote(ctx, customerID, userID, "QA")
	if err != nil {
		t.Fatalf("CreateDraftQuote: %v", err)
	}

	search := func(query string) string {
		t.Helper()
		rec := doForm(t, a, userID, q.BuscarProductos, "GET", "/cotizaciones/"+quote.Folio+"/productos?"+query,
			map[string]string{"folio": quote.Folio}, nil, true)
		if rec.Code != http.StatusOK {
			t.Fatalf("BuscarProductos(%s) status = %d, want 200", query, rec.Code)
		}
		return rec.Body.String()
	}

	for _, query := range []string{"q=cable", "q=cable&solo_familia=0&solo_familia=1"} {
		body := search(query)
		if !strings.Contains(body, "cca-c12") || strings.Contains(body, "cca-c14") {
			t.Fatalf("BuscarProductos(%s) should show only CCA-family products: %s", query, body)
		}
	}
	body := search("q=cable&solo_familia=0")
	if !strings.Contains(body, "cca-c12") || !strings.Contains(body, "cca-c14") {
		t.Fatalf("BuscarProductos with the toggle off should show every family: %s", body)
	}
}

// TestQuotesProductPickerBrowsesFamily covers the picker's browse mode: with nothing
// typed, the builder and BuscarProductos list the series family's products (never
// other families) 10 per page, and ?pagina= pages through them, clamped to range.
func TestQuotesProductPickerBrowsesFamily(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	customerID, _, _ := seedQuoteBuilderFixtures(t, a)
	ctx := context.Background()
	ccaFamilyID, err := a.store.UpsertFamily(ctx, "CCA", "CCA")
	if err != nil {
		t.Fatalf("UpsertFamily(CCA): %v", err)
	}
	cost := money.Micros(45_000_000)
	for i := 1; i <= 12; i++ {
		if _, err := a.store.CreateProduct(ctx, store.Product{
			FamilyID: ccaFamilyID, SKU: fmt.Sprintf("cca-b%02d", i), Description: fmt.Sprintf("Cable CCA %02d", i), CostMicros: &cost,
		}); err != nil {
			t.Fatalf("CreateProduct(cca %d): %v", i, err)
		}
	}
	quote, err := a.store.CreateDraftQuote(ctx, customerID, userID, "QA")
	if err != nil {
		t.Fatalf("CreateDraftQuote: %v", err)
	}

	get := func(handler http.HandlerFunc, path string, hx bool) string {
		t.Helper()
		rec := doForm(t, a, userID, handler, "GET", path, map[string]string{"folio": quote.Folio}, nil, hx)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200; body = %s", path, rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}

	// The builder page opens on page 1 of the family without any search.
	body := get(q.Builder, "/cotizaciones/"+quote.Folio, false)
	if !strings.Contains(body, "cca-b01") || !strings.Contains(body, "cca-b10") || strings.Contains(body, "cca-b11") {
		t.Fatalf("builder should list the first 10 CCA products: %s", body)
	}
	if strings.Contains(body, "abl-foco") || strings.Contains(body, "cca-c14") {
		t.Fatalf("builder should not list other families while browsing: %s", body)
	}
	if !strings.Contains(body, "Página 1 de 2") || !strings.Contains(body, "Siguiente") || strings.Contains(body, "Anterior") {
		t.Fatalf("builder should show page 1 of 2 with only a next button: %s", body)
	}

	for _, path := range []string{"/productos?pagina=2", "/productos?pagina=99"} {
		body = get(q.BuscarProductos, "/cotizaciones/"+quote.Folio+path, true)
		if !strings.Contains(body, "cca-b11") || !strings.Contains(body, "cca-b12") || strings.Contains(body, "cca-b10") {
			t.Fatalf("BuscarProductos(%s) should list the last 2 CCA products: %s", path, body)
		}
		if !strings.Contains(body, "Página 2 de 2") || !strings.Contains(body, "Anterior") || strings.Contains(body, "Siguiente") {
			t.Fatalf("BuscarProductos(%s) should show page 2 of 2 with only a previous button: %s", path, body)
		}
	}

	// A search that fits on one page has no pager.
	body = get(q.BuscarProductos, "/cotizaciones/"+quote.Folio+"/productos?q=b12", true)
	if !strings.Contains(body, "cca-b12") || strings.Contains(body, "Página") {
		t.Fatalf("single-page search should list its match without a pager: %s", body)
	}
}

func TestQuotesRequireAuth(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)

	req := httptest.NewRequest("GET", "/cotizaciones", nil)
	rec := httptest.NewRecorder()
	a.RequireAuth(http.HandlerFunc(q.List)).ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302 redirect to /login", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Fatalf("Location = %q, want /login", loc)
	}
}

// Enter in a line's qty field (iOS's "Ir" key) does an implicit submit through the
// builder form's first submit button. That must be a recalcular, never the "Buscar"
// GET, which reloads the draft from the DB and silently drops unsaved lines.
func TestQuotesBuilderDefaultSubmitRecalculates(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	customerID, _, _ := seedQuoteBuilderFixtures(t, a)
	quote, err := a.store.CreateDraftQuote(context.Background(), customerID, userID, "QA")
	if err != nil {
		t.Fatalf("CreateDraftQuote: %v", err)
	}

	rec := doForm(t, a, userID, q.Builder, "GET", "/cotizaciones/"+quote.Folio,
		map[string]string{"folio": quote.Folio}, nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("Builder status = %d", rec.Code)
	}
	body := rec.Body.String()
	formAt := strings.Index(body, `id="linea-form"`)
	if formAt < 0 {
		t.Fatal("builder page missing #linea-form")
	}
	rest := body[formAt:]
	btnAt := strings.Index(rest, `type="submit"`)
	if btnAt < 0 {
		t.Fatal("#linea-form has no submit button")
	}
	btnEnd := strings.Index(rest[btnAt:], ">")
	firstButton := rest[btnAt : btnAt+btnEnd]
	if !strings.Contains(firstButton, `hx-post="/cotizaciones/`+quote.Folio+`/recalcular"`) ||
		strings.Contains(firstButton, "formaction") {
		t.Errorf("first submit button in #linea-form is not a plain recalcular: %s", firstButton)
	}
}
