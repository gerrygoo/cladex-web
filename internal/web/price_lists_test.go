package web

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/gerrygoo/cladex-web/internal/store"
)

// A price list prices every product of the tipo's familia as a quote of that tipo would,
// takes edited quantities (0 leaves a product off), and, generated for a lead, is kept
// with its PDF and written to the lead's history (#82, slice L3).
func TestPriceListForLead(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	h := NewPriceLists(q)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	_, flatID, costID := seedQuoteBuilderFixtures(t, a)
	setDefaultMarginPercent(t, a, userID, "30") // $70 → $100.00, $45 → $64.29
	ctx := context.Background()
	qaID, _ := a.store.QuoteTypeIDByPrefix(ctx, "QA")
	qa := strconv.FormatInt(qaID, 10)
	leadID, err := a.store.CreateLead(ctx, store.LeadInput{ContactName: "Juan Pérez", Company: "Eléctrica Norte", Phone: "55", UserID: userID}, "", userID)
	if err != nil {
		t.Fatal(err)
	}
	lead := strconv.FormatInt(leadID, 10)

	rec := doForm(t, a, userID, h.NewPage, "GET", "/listas-de-precios/nueva?tipo="+qa+"&lead="+lead, nil, nil, false)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "Foco LED") || !strings.Contains(body, "Cable THW 14") ||
		!strings.Contains(body, "$100.00") || !strings.Contains(body, "$64.29") || !strings.Contains(body, `value="Juan Pérez · Eléctrica Norte"`) {
		t.Fatalf("NewPage = %d:\n%s", rec.Code, body)
	}

	form := func(extra url.Values) url.Values {
		f := url.Values{"tipo": {qa}, "lead": {lead}, "para": {"Juan Pérez"}, "delivery_time": {"2 semanas"},
			"qty_" + strconv.FormatInt(flatID, 10): {"10"}, "qty_" + strconv.FormatInt(costID, 10): {"0"}}
		for k, v := range extra {
			f[k] = v
		}
		return f
	}
	rec = doForm(t, a, userID, h.Build, "POST", "/listas-de-precios/nueva", nil, form(nil), false)
	if body := rec.Body.String(); rec.Code != http.StatusOK || !strings.Contains(body, "$1,000.00") || !strings.Contains(body, "No va en la lista") {
		t.Fatalf("Recalcular = %d", rec.Code)
	}
	rec = doForm(t, a, userID, h.Build, "POST", "/listas-de-precios/nueva", nil, form(url.Values{"generar": {"1"}, "qty_" + strconv.FormatInt(costID, 10): {"x"}}), false)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("Generar with a bad quantity = %d, want 422", rec.Code)
	}
	rec = doForm(t, a, userID, h.Build, "POST", "/listas-de-precios/nueva", nil, form(url.Values{"generar": {"1"}}), false)
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/leads/"+lead+"?lista=") {
		t.Fatalf("Generar = %d → %q: %s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}

	lists, err := a.store.ListPriceLists(ctx, leadID)
	if err != nil || len(lists) != 1 {
		t.Fatalf("lead's lists = %+v, %v", lists, err)
	}
	pl := lists[0]
	if pl.Lines != 1 || pl.MarginMicros != 300_000 || pl.Recipient != "Juan Pérez" || pl.ValidUntil == "" {
		t.Errorf("kept list = %+v", pl)
	}
	comments, _ := a.store.ListLeadComments(ctx, leadID)
	if len(comments) != 1 || !strings.Contains(comments[0].Body, "Lista de precios") || !strings.Contains(comments[0].Body, "1 productos") {
		t.Errorf("lead history = %+v", comments)
	}
	id := strconv.FormatInt(pl.ID, 10)
	rec = doForm(t, a, userID, h.PDF, "GET", "/listas-de-precios/"+id+"/pdf/x.pdf", map[string]string{"id": id, "name": "x.pdf"}, nil, false)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/pdf" || !strings.HasPrefix(rec.Body.String(), "%PDF") {
		t.Errorf("PDF = %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}

	rec = doForm(t, a, userID, h.List, "GET", "/listas-de-precios", nil, nil, false)
	if body := rec.Body.String(); rec.Code != http.StatusOK || !strings.Contains(body, "Juan Pérez") || strings.Contains(body, ">QL") {
		t.Errorf("List = %d, or it offers QL", rec.Code)
	}
	qlID, _ := a.store.QuoteTypeIDByPrefix(ctx, "QL")
	rec = doForm(t, a, userID, h.NewPage, "GET", "/listas-de-precios/nueva?tipo="+strconv.FormatInt(qlID, 10), nil, nil, false)
	if rec.Code != http.StatusNotFound {
		t.Errorf("a QL list = %d, want 404", rec.Code)
	}
}

// A CCS list shows each metre's kilos and prices from the material's price per kilo, at
// the margin picked, Otro margen included.
func TestPriceListCCS(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	h := NewPriceLists(q)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	seedQuoteBuilderFixtures(t, a)
	ccsID, _ := seedCCSProduct(t, a)
	ctx := context.Background()
	qsID, _ := a.store.QuoteTypeIDByPrefix(ctx, "QS")
	qs := strconv.FormatInt(qsID, 10)
	ccsMargin := typeMarginOption(t, a, "QS", "CCS & AC")

	// 0.1723 kg × $160 = $27.568; ÷ (1 − 0.2955) = $39.13; at Otro margen 50%, $55.14.
	f := url.Values{"tipo": {qs}, "margin_option_id": {strconv.FormatInt(ccsMargin.ID, 10)}, "qty_" + strconv.FormatInt(ccsID, 10): {"1"}}
	rec := doForm(t, a, userID, h.Build, "POST", "/listas-de-precios/nueva", nil, f, false)
	if body := rec.Body.String(); rec.Code != http.StatusOK || !strings.Contains(body, "Kg por metro") || !strings.Contains(body, "0.1723") || !strings.Contains(body, "39.13") {
		t.Fatalf("CCS list = %d:\n%s", rec.Code, body)
	}
	f.Set("margin_option_id", "custom")
	f.Set("margin_pct", "50")
	rec = doForm(t, a, userID, h.Build, "POST", "/listas-de-precios/nueva", nil, f, false)
	if body := rec.Body.String(); !strings.Contains(body, "55.14") {
		t.Errorf("Otro margen 50%% didn't price to $55.14")
	}
}
