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

// Issue #53: an admin manages quote types, including renaming a type's series, and
// quotes keep their folios across it. Since issue #56 they're listed under their
// familias at /familias, where every save lands.
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

	for _, path := range []string{"/tipos-de-cotizacion", "/tipos-de-cotizacion/nuevo", "/tipos-de-cotizacion/1"} {
		if rec := muxDo(t, a, mux, vendedor, "GET", path, nil); rec.Code != http.StatusForbidden {
			t.Errorf("vendedor GET %s = %d, want 403", path, rec.Code)
		}
	}
	// The old list's bookmarks land on the familias page.
	if rec := muxDo(t, a, mux, admin, "GET", "/tipos-de-cotizacion", nil); rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "/familias" {
		t.Errorf("GET /tipos-de-cotizacion = %d %q, want 301 to /familias", rec.Code, rec.Header().Get("Location"))
	}
	body := muxDo(t, a, mux, admin, "GET", "/familias", nil).Body.String()
	for _, want := range []string{"QA", "Cable CCA", "QA0001", "ConduCCAsa", "solo líneas libres", "Nuevo tipo de cotización para esta familia"} {
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
	if rec := muxDo(t, a, mux, admin, "POST", "/tipos-de-cotizacion", form); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/familias?guardado=tipo-creado" {
		t.Fatalf("Create = %d %q: %s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	if body := muxDo(t, a, mux, admin, "GET", "/familias?guardado=tipo-creado", nil).Body.String(); !strings.Contains(body, "Tipo de cotización creado.") {
		t.Error("the familias page doesn't confirm the new type")
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
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), want) || !strings.Contains(rec.Body.String(), `value="Escrito"`) ||
			!strings.Contains(rec.Body.String(), "<h1>Nuevo tipo de cotización</h1>") {
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
	if rec := muxDo(t, a, mux, admin, "POST", editPath, rename); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/familias?guardado=tipo-editado" {
		t.Fatalf("rename = %d %q: %s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
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

// Issue #56: /familias lists each familia with its quote types under it. QL is a
// familia without products holding the QL type, a familia without types offers to
// create one with it already chosen, and a type without a familia gets its own group.
func TestFamiliasPageGroupsTipos(t *testing.T) {
	a := newTestAuth(t)
	mux := newTestMux(t, a)
	ctx := context.Background()
	admin := createNamedUser(t, a, "ana", "Ana", "admin")
	famID, err := a.store.CreateFamily(ctx, "Exceline")
	if err != nil {
		t.Fatal(err)
	}
	fam := strconv.FormatInt(famID, 10)

	// section is the group whose heading has id.
	section := func(body, id string) string {
		t.Helper()
		start := strings.Index(body, `<section aria-labelledby="`+id+`"`)
		if start < 0 {
			t.Fatalf("no section %s", id)
		}
		return body[start : start+strings.Index(body[start:], "</section>")]
	}
	families, _ := a.store.ListAllFamilies(ctx)
	groupID := map[string]string{}
	for _, f := range families {
		groupID[f.Name] = "familia-" + strconv.FormatInt(f.ID, 10)
	}

	body := muxDo(t, a, mux, admin, "GET", "/familias", nil).Body.String()
	ql := section(body, groupID["QL"])
	for _, want := range []string{">QL</h2>", "Solo líneas libres", "No aplica", "<td>QL</td>", "Tipos de cotización de QL"} {
		if !strings.Contains(ql, want) {
			t.Errorf("QL's group lacks %q", want)
		}
	}
	if cca := section(body, groupID["CCA"]); !strings.Contains(cca, "<td>QA</td>") || strings.Contains(cca, "<td>QL</td>") {
		t.Errorf("CCA's group doesn't hold just its own types")
	}
	ex := section(body, groupID["Exceline"])
	for _, want := range []string{"Todavía no tiene tipo de cotización.", `href="/tipos-de-cotizacion/nuevo?familia=` + fam + `"`,
		`href="/familias/` + fam + `"`, "Sin clave", "Sin asignar"} {
		if !strings.Contains(ex, want) {
			t.Errorf("Exceline's group lacks %q", want)
		}
	}
	if strings.Contains(body, "Tipos sin familia") {
		t.Error("the page shows a group for types without a familia, and every seeded type has one")
	}
	if !strings.Contains(body, "<h2>Nueva familia</h2>") {
		t.Error("the page lost the new-familia form")
	}

	// The link opens the new-type form with the familia chosen; a bad one chooses none.
	rec := muxDo(t, a, mux, admin, "GET", "/tipos-de-cotizacion/nuevo?familia="+fam, nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `<option value="`+fam+`" selected>Exceline</option>`) {
		t.Errorf("new-type form = %d, want Exceline chosen", rec.Code)
	}
	rec = muxDo(t, a, mux, admin, "GET", "/tipos-de-cotizacion/nuevo?familia=x", nil)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `" selected>Exceline`) {
		t.Errorf("new-type form with a bad familia = %d, want none chosen", rec.Code)
	}

	// A type created without a familia lands in the last group.
	if rec := muxDo(t, a, mux, admin, "POST", "/tipos-de-cotizacion", url.Values{"prefix": {"QZ"}, "letterhead": {"cladex"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("create QZ = %d: %s", rec.Code, rec.Body.String())
	}
	body = muxDo(t, a, mux, admin, "GET", "/familias", nil).Body.String()
	if orphans := section(body, "tipos-sin-familia"); !strings.Contains(orphans, "<td>QZ</td>") {
		t.Error("QZ isn't under Tipos sin familia")
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
