package web

import (
	"context"
	"database/sql"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	cladex "github.com/gerrygoo/cladex-web"
	"github.com/gerrygoo/cladex-web/internal/store"
)

// dashboardFixture is a store with its own file, so a test can write history behind the
// store's back (comment times, follow-up days), and the Dashboard handlers on it.
type dashboardFixture struct {
	a   *Auth
	raw *sql.DB
	d   *Dashboard
}

func newDashboardFixture(t *testing.T) *dashboardFixture {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(context.Background(), dsn, cladex.MigrationsFS)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	t.Cleanup(func() { raw.Close() })
	return &dashboardFixture{a: NewAuth(st, false), raw: raw, d: NewDashboard(st, Build{})}
}

func (f *dashboardFixture) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := f.raw.Exec(q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func assertContains(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("missing %q", w)
		}
	}
}

// stamp is t as the database writes timestamps.
func stamp(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

// mxDay is the Mexico City midnight offset days from today.
func mxDay(offset int) time.Time {
	n := store.InMexicoCity(time.Now())
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, store.MexicoCity()).AddDate(0, 0, offset)
}

// prospecto opens a proyecto of userID's for customer, worth subtotal centavos before IVA
// at probability, whose follow-up day is next, with every comment dated a month back
// (so none counts as attending it) plus the given ones. It returns the folio.
func (f *dashboardFixture) prospecto(t *testing.T, userID, customerID int64, next time.Time, probability int, subtotal int64, comments ...time.Time) string {
	t.Helper()
	ctx := context.Background()
	s := f.a.store
	q, err := s.CreateDraftQuote(ctx, customerID, userID, "QA")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.IssueQuote(ctx, q.ID, store.Issue{IssuedAt: stamp(mxDay(-40)), TermsSnapshot: "t",
		CustomerNameSnapshot: "c", VendedorSnapshot: "v", PDFSHA256: "x"}); err != nil {
		t.Fatal(err)
	}
	q, _ = s.QuoteByID(ctx, q.ID)
	f.exec(t, `UPDATE quotes SET subtotal = ?, total = ? WHERE id = ?`, subtotal, subtotal*116/100, q.ID)
	f.exec(t, `UPDATE projects SET next_followup_date = ?, probability = ? WHERE id = ?`, next.Format("2006-01-02"), probability, *q.ProjectID)
	f.exec(t, `UPDATE quote_comments SET created_at = ? WHERE quote_id = ?`, stamp(mxDay(-40)), q.ID)
	for _, c := range comments {
		f.exec(t, `INSERT INTO quote_comments (quote_id, user_id, body, created_at) VALUES (?, ?, 'Llamé', ?)`, q.ID, userID, stamp(c))
	}
	return q.Folio
}

func TestMisPendientes(t *testing.T) {
	f := newDashboardFixture(t)
	ana := createTestUser(t, f.a, "ana", "vendedor", "hunter2")
	beto := createTestUser(t, f.a, "beto", "admin", "hunter2")
	emsa, _ := f.a.store.CreateCustomer(context.Background(), store.Customer{Name: "EMSA"})
	overdue := f.prospecto(t, ana, emsa, mxDay(-7), 50, 1_000_00)
	f.prospecto(t, beto, emsa, mxDay(-7), 50, 1_000_00) // Beto's: not on Ana's list
	f.prospecto(t, ana, emsa, mxDay(3), 50, 1_000_00)   // not due

	get := func(user int64, query string) string {
		t.Helper()
		rec := doForm(t, f.a, user, f.d.MisPendientes, "GET", "/"+query, nil, nil, false)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /%s = %d", query, rec.Code)
		}
		return rec.Body.String()
	}
	post := func(user int64, path string, pv map[string]string, h http.HandlerFunc, form url.Values) int {
		t.Helper()
		rec := doForm(t, f.a, user, h, "POST", path, pv, form, false)
		return rec.Code
	}

	body := get(ana, "")
	assertContains(t, body, "<h1>Mis pendientes</h1>", "Seguimiento de <a href=\"/proyectos/"+overdue+"\">"+overdue+"</a> · EMSA",
		`<span class="prioridad prioridad-muy_alta">Muy alta</span>`, "Agregar pendiente", "Completados", "Nada todavía.",
		`<tr id="nuevo-pendiente" class="new-row" hidden>`, "<h2 id=\"vendedores-titulo\">Vendedores</h2>")
	if strings.Count(body, "Seguimiento de ") != 1 {
		t.Errorf("Ana should see only her one overdue seguimiento: %s", body)
	}
	if strings.Contains(get(ana, "?agregar=1"), `<tr id="nuevo-pendiente" class="new-row" hidden>`) {
		t.Error("?agregar=1 should show the new row")
	}

	// Adding: an empty description is refused, with the row still open.
	rec := doForm(t, f.a, ana, f.d.AgregarPendiente, "POST", "/pendientes", nil, url.Values{"descripcion": {"  "}, "prioridad": {"alta"}}, false)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Escribe qué tienes pendiente.") ||
		!strings.Contains(rec.Body.String(), `<option value="alta" selected>`) {
		t.Fatalf("empty pendiente = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := doForm(t, f.a, ana, f.d.AgregarPendiente, "POST", "/pendientes", nil, url.Values{"descripcion": {"x"}}, true); rec.Code != http.StatusSeeOther {
		t.Errorf("htmx add = %d", rec.Code)
	}
	if code := post(ana, "/pendientes", nil, f.d.AgregarPendiente, url.Values{"descripcion": {strings.Repeat("a", 501)}}); code != http.StatusUnprocessableEntity {
		t.Errorf("a 501-character pendiente = %d", code)
	}
	rec = doForm(t, f.a, ana, f.d.AgregarPendiente, "POST", "/pendientes", nil, url.Values{"descripcion": {"Mandarle mail a Sonepar"}, "prioridad": {"baja"}, "orden": {"asc"}}, false)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/?dir=asc&hecho=agregado&sort=prioridad" {
		t.Fatalf("add = %d → %q", rec.Code, rec.Header().Get("Location"))
	}
	open, _ := f.a.store.OpenPendientes(context.Background(), ana)
	if len(open) != 2 || open[1].Body != "Mandarle mail a Sonepar" || open[1].Priority != "baja" || open[0].Priority != "media" {
		t.Fatalf("Ana's pendientes = %+v", open)
	}
	mail := strconv.FormatInt(open[1].ID, 10)
	pv := map[string]string{"id": mail}

	// Highest priority first by default, lowest first on asking; the seguimiento is muy alta.
	desc := get(ana, "")
	if strings.Index(desc, "Seguimiento de ") > strings.Index(desc, "Mandarle mail a Sonepar") {
		t.Error("descending: the muy alta seguimiento should come before the baja pendiente")
	}
	asc := get(ana, "?sort=prioridad&dir=asc")
	if strings.Index(asc, "Seguimiento de ") < strings.Index(asc, "Mandarle mail a Sonepar") {
		t.Error("ascending: the baja pendiente should come first")
	}
	assertContains(t, desc, `aria-label="Prioridad de: Mandarle mail a Sonepar"`, `aria-label="Completar: Mandarle mail a Sonepar"`)

	// The priority select saves.
	if code := post(ana, "/pendientes/"+mail+"/prioridad", pv, f.d.Prioridad, url.Values{"prioridad": {"muy_alta"}}); code != http.StatusSeeOther {
		t.Fatalf("priority = %d", code)
	}
	if code := post(ana, "/pendientes/"+mail+"/prioridad", pv, f.d.Prioridad, url.Values{"prioridad": {"urgente"}}); code != http.StatusBadRequest {
		t.Errorf("unknown priority = %d", code)
	}

	// Beto, an admin, can neither see nor touch Ana's pendiente, whatever id he tries.
	if strings.Contains(get(beto, ""), "Sonepar") {
		t.Error("Beto sees Ana's pendiente")
	}
	for _, c := range []struct {
		action string
		h      http.HandlerFunc
		form   url.Values
	}{
		{"prioridad", f.d.Prioridad, url.Values{"prioridad": {"baja"}}},
		{"completar", f.d.Completar, nil},
		{"reabrir", f.d.Reabrir, nil},
	} {
		if code := post(beto, "/pendientes/"+mail+"/"+c.action, pv, c.h, c.form); code != http.StatusNotFound {
			t.Errorf("Beto's %s on Ana's pendiente = %d, want 404", c.action, code)
		}
		if code := post(beto, "/pendientes/x/"+c.action, map[string]string{"id": "x"}, c.h, c.form); code != http.StatusNotFound {
			t.Errorf("%s on a malformed id = %d, want 404", c.action, code)
		}
	}
	if open, _ := f.a.store.OpenPendientes(context.Background(), ana); open[1].Priority != "muy_alta" {
		t.Errorf("after Beto's tries, Ana's pendiente = %+v", open[1])
	}

	// Completing moves it to Completados, in green, where it can be reopened.
	if code := post(ana, "/pendientes/"+mail+"/completar", pv, f.d.Completar, nil); code != http.StatusSeeOther {
		t.Fatalf("complete = %d", code)
	}
	body = get(ana, "?hecho=completado")
	assertContains(t, body, `data-announce="Pendiente completado."`, `<ul class="completados-list">`, `aria-label="Reabrir: Mandarle mail a Sonepar"`)
	if strings.Contains(body, `aria-label="Completar: Mandarle mail a Sonepar"`) {
		t.Error("a completed pendiente is still on the list")
	}
	if strings.Contains(get(beto, ""), "Sonepar") {
		t.Error("Beto sees Ana's completed pendiente")
	}
	if code := post(ana, "/pendientes/"+mail+"/reabrir", pv, f.d.Reabrir, nil); code != http.StatusSeeOther {
		t.Fatalf("reopen = %d", code)
	}
	if body := get(ana, ""); !strings.Contains(body, `aria-label="Completar: Mandarle mail a Sonepar"`) {
		t.Error("a reopened pendiente should be back on the list")
	}
}

// A seguimiento leaves the list when it is attended, and shows in Completados with why.
func TestMisPendientesSeguimientoCompletes(t *testing.T) {
	f := newDashboardFixture(t)
	ana := createTestUser(t, f.a, "ana", "vendedor", "hunter2")
	emsa, _ := f.a.store.CreateCustomer(context.Background(), store.Customer{Name: "EMSA"})
	folio := f.prospecto(t, ana, emsa, mxDay(-7), 50, 1_000_00)
	project, _ := f.a.store.ProjectByFolio(context.Background(), folio)
	if err := f.a.store.AddQuoteComment(context.Background(), project.CurrentQuoteID, ana, "Llamé, quedó de responder."); err != nil {
		t.Fatal(err)
	}
	body := doForm(t, f.a, ana, f.d.MisPendientes, "GET", "/", nil, nil, false).Body.String()
	if strings.Contains(body, "prioridad prioridad-muy_alta") {
		t.Error("an attended seguimiento is still on the list")
	}
	assertContains(t, body, `<a href="/proyectos/`+folio+`">`+folio+`</a>`, "Completado con un comentario el ")
}
