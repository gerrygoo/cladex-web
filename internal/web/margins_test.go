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

// marginOption finds a seeded margin option by name.
func marginOption(t *testing.T, a *Auth, name string) store.MarginOption {
	t.Helper()
	opts, err := a.store.ListMarginOptions(context.Background())
	if err != nil {
		t.Fatalf("ListMarginOptions: %v", err)
	}
	for _, o := range opts {
		if o.Name == name {
			return o
		}
	}
	t.Fatalf("no margin option %q", name)
	return store.MarginOption{}
}

// costLineForm is the builder form for one line of the $45.00-cost product, with the
// given margin option picked in the dropdown.
func costLineForm(costProductID, marginID int64) url.Values {
	return url.Values{
		"margin_option_id":     {strconv.FormatInt(marginID, 10)},
		"line_keys":            {"0"},
		"lines[0][kind]":       {"product"},
		"lines[0][product_id]": {strconv.FormatInt(costProductID, 10)},
		"lines[0][qty]":        {"1"},
	}
}

func builderBody(t *testing.T, a *Auth, q *Quotes, userID int64, folio string) string {
	t.Helper()
	rec := doForm(t, a, userID, q.Builder, "GET", "/cotizaciones/"+folio, map[string]string{"folio": folio}, nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("Builder(%s) status = %d", folio, rec.Code)
	}
	return rec.Body.String()
}

// Two drafts on different margin options price the same cost product differently, a
// draft follows an edit to its option, and the dropdown change prices before it's saved.
func TestQuoteMarginPricesCostLines(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	customerID, _, costProductID := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, userID) // default option at 30%: $45.00 / 0.70 = $64.29
	ctx := context.Background()
	alto := marginOption(t, a, "Alto")
	estandar := marginOption(t, a, "Estándar")

	draftA, _ := a.store.CreateDraftQuote(ctx, customerID, userID, "QA")
	draftB, _ := a.store.CreateDraftQuote(ctx, customerID, userID, "QA")
	for _, d := range []struct {
		folio    string
		marginID int64
	}{{draftA.Folio, estandar.ID}, {draftB.Folio, alto.ID}} {
		rec := doForm(t, a, userID, q.Guardar, "POST", "/cotizaciones/"+d.folio+"/guardar",
			map[string]string{"folio": d.folio}, costLineForm(costProductID, d.marginID), false)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("Guardar(%s) status = %d; body = %s", d.folio, rec.Code, rec.Body.String())
		}
	}

	if body := builderBody(t, a, q, userID, draftA.Folio); !strings.Contains(body, "64.29") {
		t.Errorf("draft on Estándar (30%%) missing $64.29")
	}
	bodyB := builderBody(t, a, q, userID, draftB.Folio)
	if !strings.Contains(bodyB, "56.45") { // $45.00 / (1 - 0.2028)
		t.Errorf("draft on Alto (20.28%%) missing $56.45")
	}
	if !strings.Contains(bodyB, `selected>Alto (20.28%)</option>`) {
		t.Errorf("builder dropdown doesn't show Alto selected: %s", bodyB)
	}

	// Drafts follow their option: revaluing Alto to 50% reprices draft B only.
	if err := a.store.UpdateMarginOption(ctx, alto.ID, "Alto", 500_000, userID); err != nil {
		t.Fatalf("UpdateMarginOption: %v", err)
	}
	if body := builderBody(t, a, q, userID, draftB.Folio); !strings.Contains(body, "90.00") {
		t.Errorf("draft on Alto didn't follow the option to 50%% ($90.00)")
	}
	if body := builderBody(t, a, q, userID, draftA.Folio); !strings.Contains(body, "64.29") {
		t.Errorf("draft on Estándar moved when Alto changed")
	}

	// Picking a margin in the dropdown reprices the fragment without saving it.
	rec := doForm(t, a, userID, q.Recalcular, "POST", "/cotizaciones/"+draftA.Folio+"/recalcular",
		map[string]string{"folio": draftA.Folio}, costLineForm(costProductID, alto.ID), true)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "90.00") {
		t.Fatalf("Recalcular(Alto) = %d, want $90.00: %s", rec.Code, rec.Body.String())
	}
	if saved, _ := a.store.QuoteByID(ctx, draftA.ID); *saved.MarginOptionID != estandar.ID {
		t.Errorf("Recalcular saved the margin; draft A is now on %d", *saved.MarginOptionID)
	}
}

// A draft whose option was retired shows why, can't price its cost lines, and can't be
// issued until another option is picked.
func TestQuoteRetiredMarginBlocksEmitir(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	customerID, _, costProductID := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, userID)
	ctx := context.Background()
	medio := marginOption(t, a, "Medio")

	draft, _ := a.store.CreateDraftQuote(ctx, customerID, userID, "QA")
	rec := doForm(t, a, userID, q.Guardar, "POST", "/cotizaciones/"+draft.Folio+"/guardar",
		map[string]string{"folio": draft.Folio}, costLineForm(costProductID, medio.ID), false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Guardar status = %d", rec.Code)
	}
	if err := a.store.RetireMarginOption(ctx, medio.ID, userID); err != nil {
		t.Fatalf("RetireMarginOption: %v", err)
	}

	body := builderBody(t, a, q, userID, draft.Folio)
	for _, want := range []string{
		"El margen elegido ya no está disponible; elige otro.",
		"Elige un margen disponible para esta cotización.",
		"Medio (16.56%) — retirado",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("builder missing %q", want)
		}
	}

	rec = doForm(t, a, userID, q.Emitir, "POST", "/cotizaciones/"+draft.Folio+"/emitir",
		map[string]string{"folio": draft.Folio}, costLineForm(costProductID, medio.ID), false)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("Emitir(retired margin) status = %d, want 422", rec.Code)
	}
	if after, _ := a.store.QuoteByID(ctx, draft.ID); after.Status != "borrador" {
		t.Fatalf("status after refused Emitir = %q, want borrador", after.Status)
	}
}

// Issuing freezes the margin: the page shows it by name and value, pricing_inputs
// records it, and neither changes when the option is later edited.
func TestQuoteEmitirFreezesMargin(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	customerID, _, costProductID := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, userID)
	ctx := context.Background()
	alto := marginOption(t, a, "Alto")

	draft, _ := a.store.CreateDraftQuote(ctx, customerID, userID, "QA")
	rec := doForm(t, a, userID, q.Emitir, "POST", "/cotizaciones/"+draft.Folio+"/emitir",
		map[string]string{"folio": draft.Folio}, costLineForm(costProductID, alto.ID), false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Emitir status = %d; body = %s", rec.Code, rec.Body.String())
	}
	if err := a.store.UpdateMarginOption(ctx, alto.ID, "Alto renombrado", 500_000, userID); err != nil {
		t.Fatalf("UpdateMarginOption: %v", err)
	}

	body := builderBody(t, a, q, userID, draft.Folio)
	if !strings.Contains(body, "Margen: Alto (20.28%)") || !strings.Contains(body, "56.45") {
		t.Errorf("issued page lost its frozen margin: %s", body)
	}
	lines, err := a.store.ListQuoteLines(ctx, draft.ID)
	if err != nil || len(lines) != 1 || lines[0].PricingInputsJSON == nil {
		t.Fatalf("ListQuoteLines = %+v, %v", lines, err)
	}
	if got := *lines[0].PricingInputsJSON; !strings.Contains(got, `"margin":"0.2028"`) || !strings.Contains(got, `"margin_option":"Alto"`) {
		t.Errorf("pricing_inputs = %s, want margin 0.2028 and option Alto", got)
	}
}

func TestAjustesMargenes(t *testing.T) {
	a := newTestAuth(t)
	s := NewSettings(a.store)
	adminID := createTestUser(t, a, "ana", "admin", "hunter2")
	vendedorID := createTestUser(t, a, "rodolfo", "vendedor", "hunter2")

	post := func(userID int64, h http.HandlerFunc, path string, pathValues map[string]string, form url.Values) (int, string) {
		t.Helper()
		rec := doForm(t, a, userID, a.RequireAdmin(h).ServeHTTP, "POST", path, pathValues, form, false)
		return rec.Code, rec.Body.String()
	}

	if code, _ := post(vendedorID, s.CreateMargen, "/ajustes/margenes", nil, url.Values{"name": {"X"}, "value": {"5"}}); code != http.StatusForbidden {
		t.Fatalf("vendedor CreateMargen status = %d, want 403", code)
	}

	if code, body := post(adminID, s.CreateMargen, "/ajustes/margenes", nil, url.Values{"name": {"Distribuidor"}, "value": {"8.5%"}}); code != http.StatusSeeOther {
		t.Fatalf("CreateMargen status = %d; body = %s", code, body)
	}
	dist := marginOption(t, a, "Distribuidor")
	if dist.ValueMicros != 85_000 || dist.IsDefault || !dist.Active() {
		t.Fatalf("created option = %+v, want 8.5%%, not default, active", dist)
	}

	for _, c := range []struct {
		name, value, want string
	}{
		{"Otro", "100", invalidMarginMsg},
		{"Otro", "12.345678", invalidMarginMsg},
		{"Otro", "doce", invalidMarginMsg},
		{"", "12", "El nombre del margen es obligatorio."},
		{"Distribuidor", "9", "Ya existe un margen con ese nombre."},
	} {
		code, body := post(adminID, s.CreateMargen, "/ajustes/margenes", nil, url.Values{"name": {c.name}, "value": {c.value}})
		if code != http.StatusUnprocessableEntity || !strings.Contains(body, c.want) {
			t.Errorf("CreateMargen(%q, %q) = %d, want 422 with %q", c.name, c.value, code, c.want)
		}
	}

	idPath := map[string]string{"id": strconv.FormatInt(dist.ID, 10)}
	if code, _ := post(adminID, s.UpdateMargen, "/ajustes/margenes/"+idPath["id"], idPath, url.Values{"name": {"Mayorista"}, "value": {"9.25"}}); code != http.StatusSeeOther {
		t.Fatalf("UpdateMargen status = %d", code)
	}
	if o := marginOption(t, a, "Mayorista"); o.ValueMicros != 92_500 {
		t.Fatalf("updated option = %+v, want 9.25%%", o)
	}

	estandar := marginOption(t, a, "Estándar")
	defPath := map[string]string{"id": strconv.FormatInt(estandar.ID, 10)}
	if code, body := post(adminID, s.RetirarMargen, "/ajustes/margenes/"+defPath["id"]+"/retirar", defPath, nil); code != http.StatusUnprocessableEntity ||
		!strings.Contains(body, "No puedes retirar el margen predeterminado") {
		t.Fatalf("RetirarMargen(default) = %d, want 422 with the reason", code)
	}
	if code, _ := post(adminID, s.PredeterminarMargen, "/ajustes/margenes/"+idPath["id"]+"/predeterminado", idPath, nil); code != http.StatusSeeOther {
		t.Fatalf("PredeterminarMargen status = %d", code)
	}
	if code, _ := post(adminID, s.RetirarMargen, "/ajustes/margenes/"+defPath["id"]+"/retirar", defPath, nil); code != http.StatusSeeOther {
		t.Fatalf("RetirarMargen(former default) status = %d", code)
	}
	if o := marginOption(t, a, "Estándar"); o.Active() {
		t.Fatal("Estándar still active after Retirar")
	}
	if code, _ := post(adminID, s.RestaurarMargen, "/ajustes/margenes/"+defPath["id"]+"/restaurar", defPath, nil); code != http.StatusSeeOther {
		t.Fatalf("RestaurarMargen status = %d", code)
	}
	if o := marginOption(t, a, "Estándar"); !o.Active() {
		t.Fatal("Estándar still retired after Restaurar")
	}
}

// A custom margin typed as a percentage, or as the copper price per kg it implies,
// prices the lines, is saved on the draft with the other field derived, and is frozen
// as "Personalizado" when the quote is issued.
func TestQuoteCustomMargin(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	customerID, _, _ := seedQuoteBuilderFixtures(t, a)
	ccsProductID, _ := seedCCSProduct(t, a) // CCS 30% at $160/kg
	ctx := context.Background()
	estandar := marginOption(t, a, "Estándar")

	draft, _ := a.store.CreateDraftQuote(ctx, customerID, userID, "QS")
	customForm := func(fields url.Values) url.Values {
		f := costLineForm(ccsProductID, estandar.ID)
		f.Set("margin_option_id", "custom")
		for k, v := range fields {
			f[k] = v
		}
		return f
	}
	recalc := func(f url.Values) string {
		t.Helper()
		rec := doForm(t, a, userID, q.Recalcular, "POST", "/cotizaciones/"+draft.Folio+"/recalcular",
			map[string]string{"folio": draft.Folio}, f, true)
		if rec.Code != http.StatusOK {
			t.Fatalf("Recalcular status = %d", rec.Code)
		}
		return rec.Body.String()
	}

	// 0.1723 kg × $160 = $27.568; ÷ (1 − 0.5) = $55.14.
	if body := recalc(customForm(url.Values{"margin_pct": {"50"}})); !strings.Contains(body, "55.14") {
		t.Errorf("50%% custom margin didn't price to $55.14: %s", body)
	}
	// Typing the copper price wins when it was the last field edited: $320 = 160 / (1 − 0.5).
	if body := recalc(customForm(url.Values{"margin_pct": {"10"}, "copper_price": {"320"}, "margin_edited": {"copper"}})); !strings.Contains(body, "55.14") {
		t.Errorf("$320 copper price didn't price to $55.14: %s", body)
	}
	// A price below the material's cost, or a margin outside 0–100%, can't price.
	for name, f := range map[string]url.Values{
		"copper below cost": {"copper_price": {"100"}, "margin_edited": {"copper"}},
		"margin over 100":   {"margin_pct": {"100"}},
		"empty":             {},
	} {
		body := recalc(customForm(f))
		if !strings.Contains(body, `class="error"`) {
			t.Errorf("%s: no error shown: %s", name, body)
		}
	}

	// Saving keeps the custom margin, and the builder shows both fields for it.
	rec := doForm(t, a, userID, q.Guardar, "POST", "/cotizaciones/"+draft.Folio+"/guardar",
		map[string]string{"folio": draft.Folio}, customForm(url.Values{"copper_price": {"320"}, "margin_edited": {"copper"}}), false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Guardar status = %d; body = %s", rec.Code, rec.Body.String())
	}
	saved, _ := a.store.QuoteByID(ctx, draft.ID)
	if saved.CustomMarginMicros == nil || *saved.CustomMarginMicros != 500_000 {
		t.Fatalf("saved custom margin = %v, want 0.5", saved.CustomMarginMicros)
	}
	body := builderBody(t, a, q, userID, draft.Folio)
	for _, want := range []string{`value="50"`, `value="320"`, `value="custom" selected`, "55.14"} {
		if !strings.Contains(body, want) {
			t.Errorf("builder missing %q", want)
		}
	}

	// Picking a menu option again drops the custom margin.
	rec = doForm(t, a, userID, q.Guardar, "POST", "/cotizaciones/"+draft.Folio+"/guardar",
		map[string]string{"folio": draft.Folio}, costLineForm(ccsProductID, estandar.ID), false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Guardar(option) status = %d", rec.Code)
	}
	if saved, _ = a.store.QuoteByID(ctx, draft.ID); saved.CustomMarginMicros != nil {
		t.Errorf("custom margin survived picking an option: %v", *saved.CustomMarginMicros)
	}

	// On a menu option the fields still show its margin and the copper price it implies
	// ($160 / (1 - 0.1234) = $182.52), read-only.
	if body := builderBody(t, a, q, userID, draft.Folio); !strings.Contains(body, `value="182.52"`) || !strings.Contains(body, `value="12.34"`) || !strings.Contains(body, "readonly") {
		t.Errorf("preset margin doesn't show its percentage and copper price read-only: %s", body)
	}

	// Issuing freezes it as Personalizado.
	rec = doForm(t, a, userID, q.Emitir, "POST", "/cotizaciones/"+draft.Folio+"/emitir",
		map[string]string{"folio": draft.Folio}, customForm(url.Values{"margin_pct": {"50"}}), false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Emitir status = %d; body = %s", rec.Code, rec.Body.String())
	}
	if body := builderBody(t, a, q, userID, draft.Folio); !strings.Contains(body, "Margen: Personalizado (50%)") {
		t.Errorf("issued page lost its custom margin: %s", body)
	}
}
