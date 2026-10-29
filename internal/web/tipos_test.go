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

// Issue #53: an admin manages quote types at /tipos-de-cotizacion, including renaming a
// type's series, and quotes keep their folios across it.
func TestTiposCreateEditRename(t *testing.T) {
	a := newTestAuth(t)
	mux := newTestMux(t, a)
	ctx := context.Background()
	admin := createNamedUser(t, a, "ana", "Ana", "admin")
	vendedor := createNamedUser(t, a, "emilio", "Emilio", "vendedor")
	customerID, err := a.store.CreateCustomer(ctx, store.Customer{Name: "Cliente"})
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"/tipos-de-cotizacion", "/tipos-de-cotizacion/1"} {
		if rec := muxDo(t, a, mux, vendedor, "GET", path, nil); rec.Code != http.StatusForbidden {
			t.Errorf("vendedor GET %s = %d, want 403", path, rec.Code)
		}
	}
	body := muxDo(t, a, mux, admin, "GET", "/tipos-de-cotizacion", nil).Body.String()
	for _, want := range []string{"QA", "Cable CCA", "QA0001", "ConduCCAsa", "solo líneas libres", "Nuevo tipo de cotización"} {
		if !strings.Contains(body, want) {
			t.Errorf("list is missing %q", want)
		}
	}

	// Create: a five-letter series, tidied terms, a familia and some behavior.
	families, _ := a.store.ListAllFamilies(ctx)
	form := url.Values{
		"prefix": {" qccsa "}, "name": {"Cable CCS especial"}, "family_id": {strconv.FormatInt(families[1].ID, 10)},
		"terms": {"Uno\r\n\r\n  Dos  "}, "letterhead": {"copperclad"}, "copper_price_margin": {"1"},
	}
	if rec := muxDo(t, a, mux, admin, "POST", "/tipos-de-cotizacion", form); rec.Code != http.StatusSeeOther {
		t.Fatalf("Create = %d: %s", rec.Code, rec.Body.String())
	}
	id, err := a.store.QuoteTypeIDByPrefix(ctx, "QCCSA")
	if err != nil {
		t.Fatal(err)
	}
	qt, _ := a.store.QuoteTypeByID(ctx, id)
	if qt.Terms != "Uno\nDos" || qt.FamilyName != families[1].Name || !qt.CopperPriceMargin || qt.KgPriced || qt.Letterhead != "copperclad" {
		t.Errorf("new type = %+v", qt)
	}

	// Refusals keep what was typed and say why.
	for prefix, want := range map[string]string{
		"QA": "Esa serie ya la tiene o la usó otro tipo", "Q1": "La serie es una Q seguida de 1 a 5 letras",
		"QABCDEF": "La serie es una Q seguida de 1 a 5 letras",
	} {
		rec := muxDo(t, a, mux, admin, "POST", "/tipos-de-cotizacion", url.Values{"prefix": {prefix}, "name": {"Escrito"}})
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), want) || !strings.Contains(rec.Body.String(), `value="Escrito"`) {
			t.Errorf("prefix %s: %d, want 422 with %q and the typed name kept", prefix, rec.Code, want)
		}
	}

	// Rename QA to QCCA: the issued quote keeps its folio, the next one continues.
	qaID, _ := a.store.QuoteTypeIDByPrefix(ctx, "QA")
	old, err := a.store.CreateDraftQuoteOfType(ctx, customerID, admin, qaID)
	if err != nil {
		t.Fatal(err)
	}
	editPath := "/tipos-de-cotizacion/" + strconv.FormatInt(qaID, 10)
	if body := muxDo(t, a, mux, admin, "GET", editPath, nil).Body.String(); !strings.Contains(body, "QA0002") {
		t.Errorf("edit page doesn't say the next folio is QA0002")
	}
	qa, _ := a.store.QuoteTypeByID(ctx, qaID)
	rename := url.Values{"prefix": {"QCCA"}, "name": {qa.Name}, "terms": {qa.Terms}, "letterhead": {qa.Letterhead},
		"family_id": {strconv.FormatInt(*qa.FamilyID, 10)}, "free_lines_only": {"1"}}
	if rec := muxDo(t, a, mux, admin, "POST", editPath, rename); rec.Code != http.StatusSeeOther {
		t.Fatalf("rename = %d: %s", rec.Code, rec.Body.String())
	}
	if q, _ := a.store.QuoteByID(ctx, old.ID); q.Folio != old.Folio || q.SeriesFamily != "CCA" {
		t.Errorf("old quote after the rename = %+v", q)
	}
	if qa, _ := a.store.QuoteTypeByID(ctx, qaID); qa.Prefix != "QCCA" || qa.NextFolioText() != "QCCA0002" || qa.FreeLinesOnly {
		t.Errorf("renamed type = %+v; want QCCA, numbering on, still not free-lines", qa)
	}

	// The new-quote form offers it by id and creates it there.
	if body := muxDo(t, a, mux, vendedor, "GET", "/cotizaciones/nueva", nil).Body.String(); !strings.Contains(body, "QCCA — Cable CCA") ||
		!strings.Contains(body, `name="tipo" value="`+strconv.FormatInt(qaID, 10)+`"`) {
		t.Errorf("new-quote form doesn't offer QCCA by its id")
	}
	rec := muxDo(t, a, mux, vendedor, "POST", "/cotizaciones/nueva", url.Values{"customer_id": {strconv.FormatInt(customerID, 10)}, "tipo": {strconv.FormatInt(qaID, 10)}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/cotizaciones/QCCA0002" {
		t.Errorf("new quote = %d %q, want /cotizaciones/QCCA0002", rec.Code, rec.Header().Get("Location"))
	}
}

// typeIDOf is the id of the quote type whose prefix is prefix, as the new-quote form
// posts it.
func typeIDOf(t *testing.T, a *Auth, prefix string) string {
	t.Helper()
	id, err := a.store.QuoteTypeIDByPrefix(context.Background(), prefix)
	if err != nil {
		t.Fatalf("quote type %s: %v", prefix, err)
	}
	return strconv.FormatInt(id, 10)
}
