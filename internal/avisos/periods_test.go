package avisos

import (
	"context"
	"database/sql"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	cladex "github.com/gerrygoo/cladex-web"
	"github.com/gerrygoo/cladex-web/internal/store"
)

// TestOverdueAgreesWithSeguimientoPeriods holds the avisos' own query to the periods Mis
// pendientes and Equipo show (store.SeguimientoPeriods): today, they find the same
// proyectos overdue. It runs on the real clock, since the audit log stamps it.
func TestOverdueAgreesWithSeguimientoPeriods(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(ctx, dsn, cladex.MigrationsFS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { raw.Close() })
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := raw.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	ana, _ := s.CreateUser(ctx, "ana", "Ana", "hash", "vendedor")
	customerID, _ := s.CreateCustomer(ctx, store.Customer{Name: "EMSA"})

	loc := MexicoCity()
	n := time.Now().In(loc)
	today := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, loc)
	day := func(offset int) time.Time { return today.AddDate(0, 0, offset) }
	stamp := func(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }
	prospecto := func(next time.Time, status string, comments ...time.Time) string {
		t.Helper()
		q, err := s.CreateDraftQuote(ctx, customerID, ana, "QA")
		if err != nil {
			t.Fatal(err)
		}
		if err := s.IssueQuote(ctx, q.ID, store.Issue{IssuedAt: stamp(day(-30)), TermsSnapshot: "t",
			CustomerNameSnapshot: "EMSA", VendedorSnapshot: "Ana", PDFSHA256: "x"}); err != nil {
			t.Fatal(err)
		}
		q, _ = s.QuoteByID(ctx, q.ID)
		exec(`UPDATE projects SET next_followup_date = ?, status = ? WHERE id = ?`, next.Format("2006-01-02"), status, *q.ProjectID)
		exec(`UPDATE quote_comments SET created_at = ? WHERE quote_id = ?`, stamp(day(-30)), q.ID)
		for _, c := range comments {
			exec(`INSERT INTO quote_comments (quote_id, user_id, body, created_at) VALUES (?, ?, 'Llamé', ?)`, q.ID, ana, stamp(c))
		}
		return q.Folio
	}
	overdue := prospecto(day(-7), "prospecto")
	prospecto(day(-7), "prospecto", day(-7).Add(10*time.Hour)) // commented on the day
	// A comment from before the day doesn't count.
	commentedBefore := prospecto(day(-14), "prospecto", day(-15).Add(10*time.Hour))
	prospecto(day(0), "prospecto") // due today: not late yet
	prospecto(day(3), "prospecto")
	prospecto(day(-7), "oc_recibida")

	fromAvisos, err := New(s, &fakeJobs{}, "", nil).overdue(ctx, today)
	if err != nil {
		t.Fatal(err)
	}
	periods, err := s.SeguimientoPeriods(ctx, "", "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var a, p []string
	for _, x := range fromAvisos {
		a = append(a, x.Folio)
	}
	for _, x := range periods {
		if x.Open() {
			p = append(p, x.Folio)
		}
	}
	want := []string{overdue, commentedBefore}
	for _, l := range [][]string{a, p, want} {
		sort.Strings(l)
	}
	if strings.Join(a, ",") != strings.Join(want, ",") || strings.Join(p, ",") != strings.Join(want, ",") {
		t.Errorf("overdue today: avisos %v, periods %v, want %v", a, p, want)
	}
}
