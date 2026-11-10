package store

import (
	"context"
	"strconv"
	"testing"
	"time"
)

// November 2026: Mon 9, Tue 10, Wed 11, Thu 12, Fri 13, Sat 14, Sun 15, Mon 16, Tue 17.

// mx is a Mexico City time, "2006-01-02 15:04".
func mx(t *testing.T, s string) time.Time {
	t.Helper()
	at, err := time.ParseInLocation("2006-01-02 15:04", s, mexicoCity)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

func TestReplaySeguimientos(t *testing.T) {
	const ana, beto = 1, 2
	type change struct {
		at           string
		status, next string
		user         int64
		deleted      bool
		probability  int
	}
	type want struct {
		day       string
		reason    string // "" open
		closed    string // "" open
		daysLate  int
		escalated bool
		owner     int64
		priority  string
	}
	cases := []struct {
		name     string
		changes  []change // the first is the proyecto as it was set up
		comments []string
		now      string
		periods  []want
		days     map[string]bool // follow-up day → opened
	}{
		{
			name:     "overdue since Thursday, day 1",
			changes:  []change{{at: "2026-11-10 11:00", next: "2026-11-11"}},
			comments: []string{"2026-11-10 11:00"},
			now:      "2026-11-12 10:00",
			periods:  []want{{day: "2026-11-11", daysLate: 1, owner: ana, priority: PriorityMedia}},
			days:     map[string]bool{"2026-11-11": true},
		},
		{
			name:    "not due on the day itself",
			changes: []change{{at: "2026-11-10 11:00", next: "2026-11-11"}},
			now:     "2026-11-11 23:59",
		},
		{
			name:    "day 2 before 12:00 is not escalated yet",
			changes: []change{{at: "2026-11-10 11:00", next: "2026-11-11"}},
			now:     "2026-11-13 11:59",
			periods: []want{{day: "2026-11-11", daysLate: 2, owner: ana, priority: PriorityAlta}},
			days:    map[string]bool{"2026-11-11": true},
		},
		{
			name:    "day 2 at 12:00 escalates",
			changes: []change{{at: "2026-11-10 11:00", next: "2026-11-11"}},
			now:     "2026-11-13 12:00",
			periods: []want{{day: "2026-11-11", daysLate: 2, escalated: true, owner: ana, priority: PriorityAlta}},
			days:    map[string]bool{"2026-11-11": true},
		},
		{
			name:    "the weekend doesn't count: day 3 on Monday",
			changes: []change{{at: "2026-11-10 11:00", next: "2026-11-11"}},
			now:     "2026-11-16 08:00",
			periods: []want{{day: "2026-11-11", daysLate: 3, escalated: true, owner: ana, priority: PriorityMuyAlta}},
			days:    map[string]bool{"2026-11-11": true},
		},
		{
			name:     "a comment closes it",
			changes:  []change{{at: "2026-11-10 11:00", next: "2026-11-11"}},
			comments: []string{"2026-11-12 10:30"},
			now:      "2026-11-17 10:00",
			periods:  []want{{day: "2026-11-11", reason: ClosedComentario, closed: "2026-11-12 10:30", daysLate: 1, owner: ana, priority: PriorityMedia}},
			days:     map[string]bool{"2026-11-11": true},
		},
		{
			name:     "a comment on the day itself: it never opens",
			changes:  []change{{at: "2026-11-10 11:00", next: "2026-11-11"}},
			comments: []string{"2026-11-11 18:00"},
			now:      "2026-11-17 10:00",
			days:     map[string]bool{"2026-11-11": false},
		},
		{
			name:     "a comment before the day doesn't count",
			changes:  []change{{at: "2026-11-09 11:00", next: "2026-11-11"}},
			comments: []string{"2026-11-10 23:59"},
			now:      "2026-11-12 00:00",
			periods:  []want{{day: "2026-11-11", daysLate: 1, owner: ana, priority: PriorityMedia}},
			days:     map[string]bool{"2026-11-11": true},
		},
		{
			name:     "Friday's seguimiento commented on Saturday never opens",
			changes:  []change{{at: "2026-11-10 11:00", next: "2026-11-13"}},
			comments: []string{"2026-11-14 10:00"},
			now:      "2026-11-17 10:00",
			days:     map[string]bool{"2026-11-13": false},
		},
		{
			name:    "Friday's seguimiento opens on Monday",
			changes: []change{{at: "2026-11-10 11:00", next: "2026-11-13"}},
			now:     "2026-11-14 10:00",
		},
		{
			name:    "Friday's seguimiento is day 1 on Monday and escalates Tuesday at 12:00",
			changes: []change{{at: "2026-11-10 11:00", next: "2026-11-13"}},
			now:     "2026-11-17 12:00",
			periods: []want{{day: "2026-11-13", daysLate: 2, escalated: true, owner: ana, priority: PriorityAlta}},
			days:    map[string]bool{"2026-11-13": true},
		},
		{
			name: "a new date closes it, even with the comment that comes with it",
			changes: []change{
				{at: "2026-11-10 11:00", next: "2026-11-11"},
				{at: "2026-11-12 09:00", next: "2026-11-20"},
			},
			comments: []string{"2026-11-12 09:00"},
			now:      "2026-11-17 10:00",
			periods:  []want{{day: "2026-11-11", reason: ClosedNuevaFecha, closed: "2026-11-12 09:00", daysLate: 1, owner: ana, priority: PriorityMedia}},
			days:     map[string]bool{"2026-11-11": true},
		},
		{
			name: "clearing the date closes it too",
			changes: []change{
				{at: "2026-11-10 11:00", next: "2026-11-11"},
				{at: "2026-11-12 09:00", next: ""},
			},
			now:     "2026-11-17 10:00",
			periods: []want{{day: "2026-11-11", reason: ClosedNuevaFecha, closed: "2026-11-12 09:00", daysLate: 1, owner: ana, priority: PriorityMedia}},
			days:    map[string]bool{"2026-11-11": true},
		},
		{
			name: "leaving prospecto after 12:00 of day 2: closed, escalated",
			changes: []change{
				{at: "2026-11-10 11:00", next: "2026-11-11"},
				{at: "2026-11-13 13:00", status: "oc_recibida", next: "2026-11-11"},
			},
			now:     "2026-11-17 10:00",
			periods: []want{{day: "2026-11-11", reason: ClosedCambioEtapa, closed: "2026-11-13 13:00", daysLate: 2, escalated: true, owner: ana, priority: PriorityAlta}},
			days:    map[string]bool{"2026-11-11": true},
		},
		{
			name: "closed at 12:00 sharp of day 2 was not open then",
			changes: []change{
				{at: "2026-11-10 11:00", next: "2026-11-11"},
				{at: "2026-11-13 12:00", status: "perdido", next: "2026-11-11"},
			},
			now:     "2026-11-17 10:00",
			periods: []want{{day: "2026-11-11", reason: ClosedCambioEtapa, closed: "2026-11-13 12:00", daysLate: 2, owner: ana, priority: PriorityAlta}},
			days:    map[string]bool{"2026-11-11": true},
		},
		{
			name: "deleting the proyecto closes it",
			changes: []change{
				{at: "2026-11-10 11:00", next: "2026-11-11"},
				{at: "2026-11-12 15:00", deleted: true, next: "2026-11-11"},
			},
			now:     "2026-11-17 10:00",
			periods: []want{{day: "2026-11-11", reason: ClosedCambioEtapa, closed: "2026-11-12 15:00", daysLate: 1, owner: ana, priority: PriorityMedia}},
			days:    map[string]bool{"2026-11-11": true},
		},
		{
			name: "reassigned while open: it stays its first owner's",
			changes: []change{
				{at: "2026-11-10 11:00", next: "2026-11-11"},
				{at: "2026-11-12 09:00", next: "2026-11-11", user: beto},
			},
			now:     "2026-11-12 10:00",
			periods: []want{{day: "2026-11-11", daysLate: 1, owner: ana, priority: PriorityMedia}},
			days:    map[string]bool{"2026-11-11": true},
		},
		{
			name: "reassigned before it opened: the new owner's",
			changes: []change{
				{at: "2026-11-10 11:00", next: "2026-11-11"},
				{at: "2026-11-11 15:00", next: "2026-11-11", user: beto},
			},
			now:     "2026-11-12 10:00",
			periods: []want{{day: "2026-11-11", daysLate: 1, owner: beto, priority: PriorityMedia}},
			days:    map[string]bool{"2026-11-11": true},
		},
		{
			name: "replaced before the day came: not a follow-up day",
			changes: []change{
				{at: "2026-11-09 11:00", next: "2026-11-11"},
				{at: "2026-11-10 11:00", next: "2026-11-16"},
			},
			comments: []string{"2026-11-10 11:00"},
			now:      "2026-11-17 10:00",
			periods:  []want{{day: "2026-11-16", daysLate: 1, owner: ana, priority: PriorityMedia}},
			days:     map[string]bool{"2026-11-16": true},
		},
		{
			name: "history replay: one period per overdue day",
			changes: []change{
				{at: "2026-11-02 11:00", next: "2026-11-04"},
				{at: "2026-11-06 10:00", next: "2026-11-11"},
				{at: "2026-11-12 09:00", next: "2026-11-13"},
			},
			comments: []string{"2026-11-02 11:00", "2026-11-06 10:00", "2026-11-12 09:00", "2026-11-13 17:00"},
			now:      "2026-11-17 10:00",
			periods: []want{
				{day: "2026-11-04", reason: ClosedNuevaFecha, closed: "2026-11-06 10:00", daysLate: 2, owner: ana, priority: PriorityAlta},
				{day: "2026-11-11", reason: ClosedNuevaFecha, closed: "2026-11-12 09:00", daysLate: 1, owner: ana, priority: PriorityMedia},
			},
			days: map[string]bool{"2026-11-04": true, "2026-11-11": true, "2026-11-13": false},
		},
		{
			name: "what happens after now doesn't count yet",
			changes: []change{
				{at: "2026-11-10 11:00", next: "2026-11-11"},
				{at: "2026-11-13 09:00", next: "2026-11-20"},
			},
			comments: []string{"2026-11-13 09:00"},
			now:      "2026-11-12 10:00",
			periods:  []want{{day: "2026-11-11", daysLate: 1, owner: ana, priority: PriorityMedia}},
			days:     map[string]bool{"2026-11-11": true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := projectTimeline{ID: 7, Folio: "QA0007"}
			for _, c := range tc.changes {
				st := projectState{At: mx(t, c.at), Exists: !c.deleted, Status: "prospecto", UserID: ana, Probability: 50, Next: c.next}
				if c.status != "" {
					st.Status = c.status
				}
				if c.user != 0 {
					st.UserID = c.user
				}
				h.States = append(h.States, st)
			}
			for _, c := range tc.comments {
				h.Comments = append(h.Comments, mx(t, c))
			}
			periods, days := replaySeguimientos(h, mx(t, tc.now), mexicoCity)
			if len(periods) != len(tc.periods) {
				t.Fatalf("periods = %+v, want %d", periods, len(tc.periods))
			}
			for i, w := range tc.periods {
				p := periods[i]
				if p.Day != w.day || p.Reason != w.reason || p.DaysLate != w.daysLate || p.Escalated() != w.escalated ||
					p.UserID != w.owner || p.Priority() != w.priority || p.Folio != "QA0007" || p.ProjectID != 7 {
					t.Errorf("period %d = %+v (priority %s), want %+v", i, p, p.Priority(), w)
				}
				if w.closed == "" && !p.Open() || w.closed != "" && !p.ClosedAt.Equal(mx(t, w.closed)) {
					t.Errorf("period %d closed at %v, want %q", i, p.ClosedAt, w.closed)
				}
				if p.Deleted != (tc.changes[len(tc.changes)-1].deleted) {
					t.Errorf("period %d Deleted = %v", i, p.Deleted)
				}
			}
			got := map[string]bool{}
			for _, d := range days {
				got[d.Day] = d.Opened
			}
			if len(got) != len(tc.days) {
				t.Errorf("days = %v, want %v", got, tc.days)
			}
			for d, opened := range tc.days {
				if o, ok := got[d]; !ok || o != opened {
					t.Errorf("day %s: opened %v (listed %v), want %v", d, o, ok, opened)
				}
			}
		})
	}
}

// TestSeguimientoPeriodsFromTheAuditLog reads the history the way production writes it:
// audit rows (an update first, for a proyecto older than the trail, and a delete) and
// comments on the proyecto's quotes.
func TestSeguimientoPeriodsFromTheAuditLog(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ana, _ := s.CreateUser(ctx, "ana", "Ana", "hash", "vendedor")
	beto, _ := s.CreateUser(ctx, "beto", "Beto", "hash", "vendedor")
	emsa, _ := s.CreateCustomer(ctx, Customer{Name: "EMSA"})
	peme, _ := s.CreateCustomer(ctx, Customer{Name: "Grupo PEME"})
	q1 := insertProject(t, s, "QA0001", emsa, ana, "prospecto", 50, 1_000_00)
	insertProject(t, s, "QA0002", peme, beto, "prospecto", 75, 2_000_00)
	insertProject(t, s, "QA0003", peme, beto, "prospecto", 75, 3_000_00)
	var p1, p2, p3 int64
	s.db.QueryRow(`SELECT id FROM projects WHERE folio = 'QA0001'`).Scan(&p1)
	s.db.QueryRow(`SELECT id FROM projects WHERE folio = 'QA0002'`).Scan(&p2)
	s.db.QueryRow(`SELECT id FROM projects WHERE folio = 'QA0003'`).Scan(&p3)

	audit := func(project int64, op, at, old, new string) {
		t.Helper()
		var o, n any
		if old != "" {
			o = old
		}
		if new != "" {
			n = new
		}
		if _, err := s.db.Exec(`INSERT INTO audit_log (table_name, row_key, op, old_values, new_values, at) VALUES ('projects', ?, ?, ?, ?, ?)`,
			project, op, o, n, mx(t, at).UTC().Format("2006-01-02T15:04:05.000Z")); err != nil {
			t.Fatal(err)
		}
	}
	state := func(folio string, customer, user int64, status, next string) string {
		nx := "null"
		if next != "" {
			nx = `"` + next + `"`
		}
		return `{"folio":"` + folio + `","customer_id":` + itoa(customer) + `,"user_id":` + itoa(user) +
			`,"status":"` + status + `","probability":75,"next_followup_date":` + nx + `}`
	}
	// QA0001 (Ana's): seguimiento Wednesday the 11th, commented Thursday at 10:00.
	audit(p1, "update", "2026-11-10 11:00", state("QA0001", emsa, ana, "prospecto", ""), state("QA0001", emsa, ana, "prospecto", "2026-11-11"))
	if _, err := s.db.Exec(`INSERT INTO quote_comments (quote_id, user_id, body, created_at) VALUES (?, ?, 'Llamé', ?)`,
		q1, ana, mx(t, "2026-11-12 10:00").UTC().Format("2006-01-02T15:04:05.000Z")); err != nil {
		t.Fatal(err)
	}
	// QA0002 (Beto's): seguimiento Thursday the 12th, nothing since: open.
	audit(p2, "update", "2026-11-10 11:00", state("QA0002", peme, beto, "prospecto", ""), state("QA0002", peme, beto, "prospecto", "2026-11-12"))
	// QA0003 (Beto's): seguimiento the 11th, deleted Friday the 13th.
	audit(p3, "update", "2026-11-10 11:00", state("QA0003", peme, beto, "prospecto", ""), state("QA0003", peme, beto, "prospecto", "2026-11-11"))
	audit(p3, "delete", "2026-11-13 09:00", state("QA0003", peme, beto, "prospecto", "2026-11-11"), "")

	now := mx(t, "2026-11-16 10:00")
	all, err := s.SeguimientoPeriods(ctx, "", "", now)
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		folio, customer, reason string
		user                    int64
		late                    int
		deleted                 bool
	}
	var got []row
	for _, p := range all {
		got = append(got, row{p.Folio, p.CustomerName, p.Reason, p.UserID, p.DaysLate, p.Deleted})
	}
	want := []row{
		{"QA0001", "EMSA", ClosedComentario, ana, 1, false},
		{"QA0003", "Grupo PEME", ClosedCambioEtapa, beto, 2, true},
		{"QA0002", "Grupo PEME", "", beto, 2, false},
	}
	if len(got) != len(want) {
		t.Fatalf("periods = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("period %d = %+v, want %+v", i, got[i], want[i])
		}
	}

	// The range keeps the open ones and the ones closed on a day within it.
	ranged, err := s.SeguimientoPeriods(ctx, "2026-11-13", "2026-11-16", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(ranged) != 2 || ranged[0].Folio != "QA0003" || ranged[1].Folio != "QA0002" {
		t.Errorf("periods closed from the 13th, and open = %+v", ranged)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
