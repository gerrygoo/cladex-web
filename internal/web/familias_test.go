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
	for _, want := range []string{"CCA", "CCS &amp; AC", "ABASTILUM", "QL", "Solo líneas libres", "De catálogo", "QA", "/tipos-de-cotizacion"} {
		if !strings.Contains(body, want) {
			t.Errorf("list is missing %q", want)
		}
	}
}

// A familia is a catalog category now: creating one asks only for its name (issue #53).
func TestFamiliasCreate(t *testing.T) {
	a := newTestAuth(t)
	f, adminID := adminFamilias(t, a)

	rec := doForm(t, a, adminID, f.Create, "POST", "/familias", nil, url.Values{"name": {"  Fire Blanket "}}, false)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/familias?guardado=creada" {
		t.Fatalf("Create = %d %q; body = %s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	all, _ := a.store.ListAllFamilies(t.Context())
	if last := all[len(all)-1]; last.Name != "Fire Blanket" || last.QuoteTypes != "" || last.FreeLinesOnly {
		t.Fatalf("new familia = %+v", last)
	}

	for name, want := range map[string]string{" ": "El nombre es obligatorio.", "cca": "Ya existe una familia con este nombre."} {
		rec := doForm(t, a, adminID, f.Create, "POST", "/familias", nil, url.Values{"name": {name}}, false)
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("name %q: status %d, want 422 with %q", name, rec.Code, want)
		}
	}
	if all, _ := a.store.ListAllFamilies(t.Context()); len(all) != 5 {
		t.Errorf("%d familias after refused forms; want the 4 seeded and Fire Blanket", len(all))
	}
}

func TestFamiliasEditSATCode(t *testing.T) {
	a := newTestAuth(t)
	f, adminID := adminFamilias(t, a)
	all, _ := a.store.ListAllFamilies(t.Context())
	cca := all[0]
	path := "/familias/" + strconv.FormatInt(cca.ID, 10)
	pv := map[string]string{"id": strconv.FormatInt(cca.ID, 10)}

	if rec := doForm(t, a, adminID, f.EditPage, "GET", path, pv, nil, false); rec.Code != http.StatusOK {
		t.Fatalf("EditPage = %d", rec.Code)
	}
	if rec := doForm(t, a, adminID, f.Update, "POST", path, pv, url.Values{"sat_product_code": {"123"}}, false); rec.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rec.Body.String(), "La clave SAT son 8 dígitos") {
		t.Errorf("a short SAT code = %d, want 422 saying why", rec.Code)
	}
	if rec := doForm(t, a, adminID, f.Update, "POST", path, pv, url.Values{"sat_product_code": {"26121600"}}, false); rec.Code != http.StatusSeeOther {
		t.Fatalf("Update = %d", rec.Code)
	}
	all, _ = a.store.ListAllFamilies(t.Context())
	if all[0].SATProductCode != "26121600" {
		t.Fatalf("after update: %+v", all[0])
	}
	if rec := doForm(t, a, adminID, f.EditPage, "GET", "/familias/999", map[string]string{"id": "999"}, nil, false); rec.Code != http.StatusNotFound {
		t.Errorf("EditPage(999) = %d, want 404", rec.Code)
	}
}
