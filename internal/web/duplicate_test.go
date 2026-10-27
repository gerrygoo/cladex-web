package web

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/store"
)

// TestDuplicarCatalogQuote: an issued QA is copied for another customer as a new QA
// draft, owned by whoever duplicated it, priced from the current catalog, without the
// original's comments or proyecto, and the original doesn't change (issue #40).
func TestDuplicarCatalogQuote(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	ctx := context.Background()
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	otherID := createTestUser(t, a, "vendedor2", "vendedor", "hunter2")
	customerID, flatProductID, _ := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, userID)
	otherCustomer, err := a.store.CreateCustomer(ctx, store.Customer{Name: "Constructora Norte"})
	if err != nil {
		t.Fatal(err)
	}
	pv := func(folio string) map[string]string { return map[string]string{"folio": folio} }

	quote, _ := a.store.CreateDraftQuote(ctx, customerID, userID, "QA")
	form := url.Values{
		"line_keys":            {"0"},
		"lines[0][kind]":       {"product"},
		"lines[0][product_id]": {strconv.FormatInt(flatProductID, 10)},
		"lines[0][qty]":        {"3"},
	}
	if rec := doForm(t, a, userID, q.Emitir, "POST", "/cotizaciones/"+quote.Folio+"/emitir", pv(quote.Folio), form, false); rec.Code != http.StatusSeeOther {
		t.Fatalf("Emitir = %d; body = %.300s", rec.Code, rec.Body.String())
	}
	if err := a.store.AddQuoteComment(ctx, quote.ID, userID, "Llamar el lunes"); err != nil {
		t.Fatal(err)
	}
	issued, _ := a.store.QuoteByID(ctx, quote.ID)

	// The cost went from $70 to $140 since: the copy is priced at $200, not $100.
	p, _ := a.store.ProductByID(ctx, flatProductID)
	cost := money.Micros(140_000_000)
	p.CostMicros = &cost
	if err := a.store.UpdateProduct(ctx, *p); err != nil {
		t.Fatal(err)
	}

	// The button and its dialog are on the issued quote, the original's customer picked.
	page := doForm(t, a, userID, q.Builder, "GET", "/cotizaciones/"+quote.Folio, pv(quote.Folio), nil, false).Body.String()
	for _, want := range []string{`data-confirm-dialog="duplicar"`, `action="/cotizaciones/` + quote.Folio + `/duplicar"`, `selected data-detail`} {
		if !strings.Contains(page, want) {
			t.Errorf("issued quote page missing %q", want)
		}
	}

	// "Agregar nuevo cliente" is first in the list and leads to the new-customer form
	// and back (issue #50).
	if !strings.Contains(page, "➕ Agregar nuevo cliente") || !strings.Contains(page, `data-new-customer-href="/clientes/nuevo?volver=duplicar&amp;folio=`+quote.Folio+`"`) {
		t.Error("the Duplicar dialog doesn't offer Agregar nuevo cliente")
	}
	rec := doForm(t, a, otherID, q.Duplicar, "POST", "/cotizaciones/"+quote.Folio+"/duplicar", pv(quote.Folio), url.Values{"customer_id": {"nuevo"}}, false)
	if want := "/clientes/nuevo?folio=" + quote.Folio + "&volver=duplicar"; rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != want {
		t.Errorf("Agregar nuevo cliente = %d, Location %q, want %q", rec.Code, rec.Header().Get("Location"), want)
	}
	back := returnTo{Volver: "duplicar", Folio: quote.Folio}
	if got, want := back.quotePath(7), "/cotizaciones/"+quote.Folio+"/duplicar?cliente=7"; got != want {
		t.Errorf("return path = %q, want %q", got, want)
	}

	// No customer: nothing is created.
	rec = doForm(t, a, otherID, q.Duplicar, "POST", "/cotizaciones/"+quote.Folio+"/duplicar", pv(quote.Folio), url.Values{"customer_id": {""}}, false)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Selecciona un cliente.") {
		t.Fatalf("Duplicar without a customer = %d", rec.Code)
	}
	if next, _ := a.store.QuoteByFolio(ctx, "QA0002"); next != nil {
		t.Fatal("a copy was created without a customer")
	}

	rec = doForm(t, a, otherID, q.Duplicar, "POST", "/cotizaciones/"+quote.Folio+"/duplicar", pv(quote.Folio), url.Values{"customer_id": {strconv.FormatInt(otherCustomer, 10)}}, false)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/cotizaciones/QA0002?duplicada="+quote.Folio {
		t.Fatalf("Duplicar = %d, Location %q", rec.Code, rec.Header().Get("Location"))
	}
	dup, _ := a.store.QuoteByFolio(ctx, "QA0002")
	if dup == nil || dup.Status != "borrador" || dup.CustomerID != otherCustomer || dup.UserID != otherID ||
		dup.ProjectID != nil || dup.SupersedesQuoteID != nil || dup.MarginOptionID == nil || *dup.MarginOptionID != *issued.MarginOptionID {
		t.Fatalf("copy = %+v", dup)
	}
	lines, _ := a.store.ListQuoteLines(ctx, dup.ID)
	if len(lines) != 1 || lines[0].QtyMilli != 3000 || lines[0].UnitPriceMicros != 200_000_000 || dup.Subtotal != 60_000 {
		t.Fatalf("copy lines = %+v, subtotal %d; want 3 × $200.00 from the current cost", lines, dup.Subtotal)
	}
	if comments, _ := a.store.ListQuoteComments(ctx, dup.ID); len(comments) != 0 {
		t.Errorf("the copy has the original's comments: %+v", comments)
	}

	page = doForm(t, a, otherID, q.Builder, "GET", "/cotizaciones/QA0002?duplicada="+quote.Folio, pv("QA0002"), nil, false).Body.String()
	if !strings.Contains(page, "Copia de "+quote.Folio+" creada.") {
		t.Error("the copy doesn't say what it was copied from")
	}
	if page := doForm(t, a, otherID, q.Builder, "GET", "/cotizaciones/QA0002?duplicada=hola", pv("QA0002"), nil, false).Body.String(); strings.Contains(page, "Copia de hola") {
		t.Error("the message takes any text, not only a folio")
	}

	after, _ := a.store.QuoteByID(ctx, quote.ID)
	if after.Status != "emitida" || after.Total != issued.Total || *after.PDFSHA256 != *issued.PDFSHA256 || after.SupersededByFolio != "" {
		t.Errorf("the original changed: %+v", after)
	}

	// A draft can be duplicated too, and the page without JS asks for the customer.
	page = doForm(t, a, userID, q.DuplicarPage, "GET", "/cotizaciones/QA0002/duplicar", pv("QA0002"), nil, false).Body.String()
	if !strings.Contains(page, "Duplicar QA0002") || !strings.Contains(page, "lo último guardado") {
		t.Error("the Duplicar page of a draft doesn't explain what is copied")
	}
	rec = doForm(t, a, userID, q.Duplicar, "POST", "/cotizaciones/QA0002/duplicar", pv("QA0002"), url.Values{"customer_id": {strconv.FormatInt(customerID, 10)}}, false)
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/cotizaciones/QA0003?") {
		t.Fatalf("duplicating a draft = %d, %q", rec.Code, rec.Header().Get("Location"))
	}
}

// TestDuplicarQL: a QL copy keeps each line's cost and margin, the validity date, the
// extra terms and the cotización del proveedor, its files now removable on the copy and
// still there on the original.
func TestDuplicarQL(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	ctx := context.Background()
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	customerID, _, _ := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, userID)
	pv := func(folio string) map[string]string { return map[string]string{"folio": folio} }
	future := time.Now().AddDate(0, 0, 10).Format("2006-01-02")

	ql, _ := a.store.CreateDraftQuote(ctx, customerID, userID, "QL")
	form := url.Values{
		"line_keys":             {"0"},
		"lines[0][kind]":        {"free"},
		"lines[0][description]": {"Transformador"},
		"lines[0][cost]":        {"1500"},
		"lines[0][margin]":      {"10"},
		"lines[0][qty]":         {"2"},
		"delivery_time":         {"5 días"},
		"currency":              {"MXN"},
		"valid_until":           {future},
		"extra_terms":           {"Garantía de 1 año"},
	}
	if err := a.store.SetSupplierQuoteInfo(ctx, ql.ID, "CT Internacional", "Q-1", userID); err != nil {
		t.Fatal(err)
	}
	pdf := store.NewFile{Filename: "cot.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1.7")}
	if err := a.store.AddSupplierQuoteFiles(ctx, ql.ID, []store.NewFile{pdf}, userID); err != nil {
		t.Fatal(err)
	}
	if rec := doForm(t, a, userID, q.Emitir, "POST", "/cotizaciones/"+ql.Folio+"/emitir", pv(ql.Folio), form, false); rec.Code != http.StatusSeeOther {
		t.Fatalf("Emitir = %d; body = %.300s", rec.Code, rec.Body.String())
	}

	rec := doForm(t, a, userID, q.Duplicar, "POST", "/cotizaciones/"+ql.Folio+"/duplicar", pv(ql.Folio), url.Values{"customer_id": {strconv.FormatInt(customerID, 10)}}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Duplicar = %d", rec.Code)
	}
	dup, _ := a.store.QuoteByFolio(ctx, "QL0002")
	if dup == nil || dup.ExtraTerms != "Garantía de 1 año" || dup.Currency != "MXN" || dup.ValidUntil == nil || *dup.ValidUntil != future {
		t.Fatalf("copy = %+v", dup)
	}
	lines, _ := a.store.ListQuoteLines(ctx, dup.ID)
	if len(lines) != 1 || lines[0].CostMicros == nil || *lines[0].CostMicros != 1_500_000_000 || freeLineMargin(lines[0]) == nil || *freeLineMargin(lines[0]) != 100_000 {
		t.Fatalf("copy lines = %+v", lines)
	}
	sq, _ := a.store.SupplierQuoteOf(ctx, dup.ID)
	if sq.Supplier != "CT Internacional" || sq.Number != "Q-1" || len(sq.Files) != 1 || !sq.Files[0].Removable {
		t.Fatalf("copy's supplier quote = %+v", sq)
	}
	// Removing the copy's file leaves the original's.
	if found, err := a.store.RemoveSupplierQuoteFile(ctx, dup.ID, sq.Files[0].ID, userID); !found || err != nil {
		t.Fatalf("removing the copy's file = %v, %v", found, err)
	}
	if orig, _ := a.store.SupplierQuoteOf(ctx, ql.ID); len(orig.Files) != 1 {
		t.Errorf("the original lost its file: %+v", orig)
	}
}
