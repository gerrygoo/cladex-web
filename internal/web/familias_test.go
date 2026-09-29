package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func adminFamilias(t *testing.T, a *Auth) (*Familias, int64) {
	t.Helper()
	return NewFamilias(a.store), createTestUser(t, a, "ana", "admin", "hunter2")
}

func TestFamiliasListShowsSeededFamilias(t *testing.T) {
	a := newTestAuth(t)
	f, adminID := adminFamilias(t, a)
	rec := doForm(t, a, adminID, f.List, "GET", "/familias", nil, nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"CCA", "CCS &amp; AC", "ABASTILUM", "QL", "Solo líneas libres", "De catálogo"} {
		if !strings.Contains(body, want) {
			t.Errorf("list is missing %q", want)
		}
	}
}

func TestFamiliasCreateStartsASeries(t *testing.T) {
	a := newTestAuth(t)
	f, adminID := adminFamilias(t, a)

	rec := doForm(t, a, adminID, f.Create, "POST", "/familias", nil, url.Values{
		"name": {"  Fire Blanket "}, "series": {"qf"}, "series_label": {"Cobijas"},
		"terms": {"Precios en MXN\r\n\r\n  Pago por adelantado  "},
	}, false)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/familias?guardado=creada" {
		t.Fatalf("Create = %d %q; body = %s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	series, _ := a.store.ListSeries(t.Context())
	last := series[len(series)-1]
	if last.Prefix != "QF" || last.PickerLabel() != "QF — Cobijas" || last.FamilyName != "Fire Blanket" {
		t.Fatalf("new series = %+v", last)
	}
	all, _ := a.store.ListAllFamilies(t.Context())
	if got := all[len(all)-1].Terms; got != "Precios en MXN\nPago por adelantado" {
		t.Fatalf("terms = %q; want trimmed lines without blanks", got)
	}
}

func TestFamiliasCreateRejectsBadInput(t *testing.T) {
	a := newTestAuth(t)
	f, adminID := adminFamilias(t, a)

	cases := []struct {
		name string
		form url.Values
		want string
	}{
		{"empty name", url.Values{"name": {" "}, "series": {"QF"}}, "El nombre es obligatorio."},
		{"series without Q", url.Values{"name": {"X"}, "series": {"AB"}}, "La serie son dos letras y empieza con Q"},
		{"series too long", url.Values{"name": {"X"}, "series": {"QFF"}}, "La serie son dos letras y empieza con Q"},
		{"taken series", url.Values{"name": {"X"}, "series": {"ql"}}, "Ya existe una familia con esta serie."},
		{"taken name", url.Values{"name": {"cca"}, "series": {"QF"}}, "Ya existe una familia con este nombre."},
	}
	for _, c := range cases {
		rec := doForm(t, a, adminID, f.Create, "POST", "/familias", nil, c.form, false)
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), c.want) {
			t.Errorf("%s: status %d, want 422 with %q", c.name, rec.Code, c.want)
		}
	}
	// A refused form keeps what was typed.
	rec := doForm(t, a, adminID, f.Create, "POST", "/familias", nil, url.Values{"name": {"Mi familia"}, "series": {"AB"}}, false)
	if !strings.Contains(rec.Body.String(), `value="Mi familia"`) {
		t.Error("refused form lost the typed name")
	}
	if all, _ := a.store.ListAllFamilies(t.Context()); len(all) != 4 {
		t.Errorf("%d familias after refused forms; want the 4 seeded", len(all))
	}
}

func TestFamiliasEditTerms(t *testing.T) {
	a := newTestAuth(t)
	f, adminID := adminFamilias(t, a)
	all, _ := a.store.ListAllFamilies(t.Context())
	ql := all[len(all)-1]
	path := "/familias/" + strconv.FormatInt(ql.ID, 10)
	pv := map[string]string{"id": strconv.FormatInt(ql.ID, 10)}

	rec := doForm(t, a, adminID, f.EditPage, "GET", path, pv, nil, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Precios sujetos a cambios sin previo aviso") {
		t.Fatalf("EditPage = %d; want the current terms in the textarea", rec.Code)
	}

	rec = doForm(t, a, adminID, f.Update, "POST", path, pv, url.Values{
		"series_label": {"Líneas libres"}, "terms": {"Definitivo uno\nDefinitivo dos"},
	}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Update = %d", rec.Code)
	}
	all, _ = a.store.ListAllFamilies(t.Context())
	if got := all[len(all)-1]; got.Terms != "Definitivo uno\nDefinitivo dos" || got.Series != "QL" {
		t.Fatalf("after update: %+v", got)
	}

	if rec := doForm(t, a, adminID, f.EditPage, "GET", "/familias/999", map[string]string{"id": "999"}, nil, false); rec.Code != http.StatusNotFound {
		t.Errorf("EditPage(999) = %d, want 404", rec.Code)
	}
}
