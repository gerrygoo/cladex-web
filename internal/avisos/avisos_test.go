package avisos

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cladex "github.com/gerrygoo/cladex-web"
	"github.com/gerrygoo/cladex-web/internal/store"
)

type fakeJobs struct{ keys []string }

func (f *fakeJobs) Enqueue(_ context.Context, j store.NewJob) (int64, error) {
	f.keys = append(f.keys, j.DedupeKey)
	return int64(len(f.keys)), nil
}

func TestNextSlot(t *testing.T) {
	loc := MexicoCity()
	for in, want := range map[string]string{
		"2026-11-12T08:00": "2026-11-12T09:00", // Thursday
		"2026-11-12T09:00": "2026-11-12T12:00",
		"2026-11-12T12:30": "2026-11-12T18:00",
		"2026-11-13T18:30": "2026-11-16T09:00", // Friday evening → Monday
		"2026-11-14T10:00": "2026-11-16T09:00", // Saturday
	} {
		at, _ := time.ParseInLocation(slotLayout, in, loc)
		if got := nextSlot(at, loc).Format(slotLayout); got != want {
			t.Errorf("nextSlot(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestWeekdaysAfter(t *testing.T) {
	loc := MexicoCity()
	d := func(s string) time.Time { t, _ := time.ParseInLocation("2006-01-02", s, loc); return t }
	for _, tc := range []struct {
		from, to string
		want     int
	}{
		{"2026-11-11", "2026-11-11", 0},
		{"2026-11-11", "2026-11-12", 1},
		{"2026-11-13", "2026-11-16", 1}, // Friday's seguimiento: Monday is day 1
		{"2026-11-14", "2026-11-16", 1}, // a Saturday's too
		{"2026-11-11", "2026-11-18", 5},
	} {
		if got := weekdaysAfter(d(tc.from), d(tc.to)); got != tc.want {
			t.Errorf("weekdaysAfter(%s, %s) = %d, want %d", tc.from, tc.to, got, tc.want)
		}
	}
}

// fixture is a store with a prospecto QA0001 of Ana's whose seguimiento is Wednesday
// 2026-11-11, last commented the day before. Ana has seguimiento_vencido on, Gerry and
// Rod seguimiento_escalado; Rod has email off.
type fixture struct {
	s       *store.Store
	raw     *sql.DB
	svc     *Service
	jobs    *fakeJobs
	project int64
	ana     int64
	gerry   int64
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
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

	user := func(username, name, role, kind string, email bool) int64 {
		id, err := s.CreateUser(ctx, username, name, "hash", role)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.SetUserContact(ctx, id, store.UserContact{Email: username + "@cladex.com.mx", NotifyEmail: email}); err != nil {
			t.Fatal(err)
		}
		if err := s.SetUserAvisos(ctx, id, role, []string{kind}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	f := &fixture{s: s, raw: raw, jobs: &fakeJobs{}}
	f.ana = user("ana", "Ana", "vendedor", store.AvisoSeguimientoVencido, true)
	f.gerry = user("gerry", "Gerardo", "admin", store.AvisoSeguimientoEscalado, true)
	user("rod", "Rodolfo", "admin", store.AvisoSeguimientoEscalado, false)

	customerID, err := s.CreateCustomer(ctx, store.Customer{Name: "EMSA", ContactName: "Daniel Muñoz"})
	if err != nil {
		t.Fatal(err)
	}
	q, err := s.CreateDraftQuote(ctx, customerID, f.ana, "QA")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.IssueQuote(ctx, q.ID, store.Issue{IssuedAt: "2026-11-02T15:00:00.000Z", TermsSnapshot: "t",
		CustomerNameSnapshot: "EMSA", VendedorSnapshot: "Ana", PDFSHA256: "x"}); err != nil {
		t.Fatal(err)
	}
	q, _ = s.QuoteByID(ctx, q.ID)
	f.project = *q.ProjectID
	next := "2026-11-11"
	if err := s.FollowUpProject(ctx, f.project, f.ana, store.FollowUp{NextFollowUp: &next, Note: "Le ofrecí cables Copperclad."}); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE quote_comments SET created_at = '2026-11-10T17:00:00.000Z'`)
	f.svc = New(s, f.jobs, "https://cotizador.example/", nil)
	return f
}

func (f *fixture) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := f.raw.Exec(q, args...); err != nil {
		t.Fatal(err)
	}
}

// run runs the round at slot (Mexico City), as if at slot+late, and returns its result
// and the notifications it queued per user.
func (f *fixture) run(t *testing.T, slot string, late time.Duration) (string, map[int64]int) {
	t.Helper()
	at, err := time.ParseInLocation(slotLayout, slot, f.svc.loc)
	if err != nil {
		t.Fatal(err)
	}
	f.svc.now = func() time.Time { return at.Add(late) }
	var before int
	f.raw.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM notifications`).Scan(&before)
	result, err := f.svc.Handle(context.Background(), store.Job{Payload: `{"slot":"` + slot + `"}`})
	if err != nil {
		t.Fatalf("round %s: %v", slot, err)
	}
	rows, err := f.raw.Query(`SELECT user_id FROM notifications WHERE id > ?`, before)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[int64]int{}
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		got[id]++
	}
	return result, got
}

func TestSeguimientoRounds(t *testing.T) {
	f := newFixture(t)
	for _, tc := range []struct {
		slot       string
		ana, gerry int
	}{
		{"2026-11-11T18:00", 0, 0}, // the seguimiento day itself
		{"2026-11-12T09:00", 1, 0}, // day 1: the owner at 9, 12 and 18
		{"2026-11-12T12:00", 1, 0},
		{"2026-11-12T18:00", 1, 0},
		{"2026-11-13T09:00", 0, 0}, // day 2: the team at 12, the owner at 18
		{"2026-11-13T12:00", 0, 1},
		{"2026-11-13T18:00", 1, 0},
		{"2026-11-16T12:00", 0, 0}, // day 3 (Monday)
		{"2026-11-16T18:00", 1, 0},
		{"2026-11-18T12:00", 0, 1}, // day 5: the team again
	} {
		_, got := f.run(t, tc.slot, 0)
		if got[f.ana] != tc.ana || got[f.gerry] != tc.gerry || len(got) > 2 {
			t.Errorf("%s: queued %v, want Ana %d, Gerardo %d (and none for Rodolfo, who has email off)", tc.slot, got, tc.ana, tc.gerry)
		}
	}
	if last := f.jobs.keys[len(f.jobs.keys)-1]; last != "2026-11-18T18:00" {
		t.Errorf("after Wednesday 12:00 the next round queued is %s", last)
	}

	// A round runs once, and one too late sends nothing.
	if result, got := f.run(t, "2026-11-18T12:00", 0); len(got) != 0 || !strings.Contains(result, "ya se había enviado") {
		t.Errorf("rerun: %q, %v", result, got)
	}
	if result, got := f.run(t, "2026-11-18T18:00", 3*time.Hour); len(got) != 0 || !strings.Contains(result, "tarde") {
		t.Errorf("late round: %q, %v", result, got)
	}

	// What the owner's aviso says.
	var subject, body string
	f.raw.QueryRow(`SELECT subject, body_text FROM notifications WHERE user_id = ? ORDER BY id LIMIT 1`, f.ana).Scan(&subject, &body)
	if subject != "Seguimiento vencido: QA0001 · EMSA" {
		t.Errorf("subject %q", subject)
	}
	for _, want := range []string{"Hola, Ana.", "era el 11/11/2026", "Daniel Muñoz", "margen",
		"Último comentario de seguimiento, de Ana el 10/11/2026", "Le ofrecí cables Copperclad.",
		"https://cotizador.example/proyectos/QA0001", "no lo respondas"} {
		if !strings.Contains(body, want) {
			t.Errorf("owner's aviso lacks %q:\n%s", want, body)
		}
	}
	f.raw.QueryRow(`SELECT subject, body_text FROM notifications WHERE user_id = ? ORDER BY id LIMIT 1`, f.gerry).Scan(&subject, &body)
	if subject != "Ana no ha dado seguimiento a QA0001 · EMSA" || !strings.Contains(body, "van 2 días hábiles") {
		t.Errorf("team's aviso: %q\n%s", subject, body)
	}

	// A proyecto with a name (issue #73) is called by it next to its folio.
	if _, err := f.raw.Exec(`UPDATE projects SET name = 'Capuchinas'`); err != nil {
		t.Fatal(err)
	}
	if _, got := f.run(t, "2026-11-19T18:00", 0); got[f.ana] != 1 {
		t.Fatalf("Thursday 18:00 queued %v, want one for Ana", got)
	}
	f.raw.QueryRow(`SELECT subject, body_text FROM notifications WHERE user_id = ? ORDER BY id DESC LIMIT 1`, f.ana).Scan(&subject, &body)
	if subject != "Seguimiento vencido: QA0001 · Capuchinas · EMSA" || !strings.Contains(body, "Proyecto QA0001 · Capuchinas · cotización") {
		t.Errorf("named proyecto's aviso: %q\n%s", subject, body)
	}
}

func TestSeguimientoStops(t *testing.T) {
	f := newFixture(t)
	// A comment on the morning of day 1 stops the rest.
	f.exec(t, `INSERT INTO quote_comments (quote_id, user_id, body, created_at)
		SELECT id, ?, 'Hablé con Daniel.', '2026-11-12T16:00:00.000Z' FROM quotes`, f.ana)
	if _, got := f.run(t, "2026-11-12T12:00", 0); len(got) != 0 {
		t.Errorf("after a comment: %v", got)
	}

	// So does Ana's turning her aviso off, or the proyecto leaving prospecto.
	g := newFixture(t)
	g.s.SetUserAvisos(context.Background(), g.ana, store.RoleVendedor, nil)
	if _, got := g.run(t, "2026-11-12T09:00", 0); len(got) != 0 {
		t.Errorf("with the aviso off: %v", got)
	}
	g.exec(t, `UPDATE projects SET status = 'perdido'`)
	if _, got := g.run(t, "2026-11-13T12:00", 0); len(got) != 0 {
		t.Errorf("a perdido proyecto: %v", got)
	}
}
