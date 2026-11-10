package store

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/gerrygoo/cladex-web/internal/money"
)

// teamHistory seeds a November 2026 written the way production writes it (audit rows,
// comments, notifications), seen on Monday the 16th at 10:00:
//
//   - QA0001, Ana's, Alta 75%, $1,000.00 (profit $250.00): seguimiento Wednesday the 11th,
//     moved on Thursday at 10:00 to Friday the 13th (a period of 1 day, closed by the new
//     date), and nothing since: overdue again, day 1.
//   - QA0002, Ana's, Baja 25%, $500.00: seguimiento Wednesday the 4th, commented that
//     afternoon: on time.
//   - QA0003, Beto's, Alta 75%, $2,000.00 (profit $500.00): seguimiento Monday the 9th,
//     nothing since: 5 days late, escalated Wednesday the 11th.
//   - QA0004, Beto's, Media 50%: seguimiento Monday the 2nd; its O.C. came Thursday the
//     5th at 13:00, 3 days late and escalated on the 4th.
//   - QA0005, ggo's (the development account), overdue: left out.
//
// Beto got avisos on 2 rounds in November (one by email and WhatsApp), one cancelled
// and one in October; Ana none.
type teamSeed struct {
	s                *Store
	ana, beto, carla int64
	now              time.Time
}

func newTeamSeed(t *testing.T) teamSeed {
	t.Helper()
	s := newTestStore(t)
	ctx := context.Background()
	ana, _ := s.CreateUser(ctx, "ana", "Ana", "hash", "vendedor")
	beto, _ := s.CreateUser(ctx, "beto", "Beto", "hash", "vendedor")
	carla, _ := s.CreateUser(ctx, "carla", "Carla", "hash", "admin") // nothing to show
	ggo, _ := s.CreateUser(ctx, "ggo", "Gerardo G.", "hash", "admin")
	emsa, _ := s.CreateCustomer(ctx, Customer{Name: "EMSA"})
	ts := func(at string) string { return mx(t, at).UTC().Format("2006-01-02T15:04:05.000Z") }

	type proj struct {
		folio       string
		user        int64
		probability int
		subtotal    int64 // centavos
		costMicros  int64
	}
	ids := map[string]int64{}
	quotes := map[string]int64{}
	for _, p := range []proj{
		{"QA0001", ana, 75, 1_000_00, 750_000_000},
		{"QA0002", ana, 25, 500_00, 0},
		{"QA0003", beto, 75, 2_000_00, 1_500_000_000},
		{"QA0004", beto, 50, 3_000_00, 0},
		{"QA0005", ggo, 75, 9_000_00, 0},
	} {
		q := insertProject(t, s, p.folio, emsa, p.user, "prospecto", p.probability, p.subtotal)
		quotes[p.folio] = q
		var id int64
		s.db.QueryRow(`SELECT id FROM projects WHERE folio = ?`, p.folio).Scan(&id)
		ids[p.folio] = id
		if p.costMicros > 0 {
			if _, err := s.db.Exec(`UPDATE quotes SET margin_snapshot_micros = 0 WHERE id = ?`, q); err != nil {
				t.Fatal(err)
			}
			if _, err := s.db.Exec(`INSERT INTO quote_lines (quote_id, line_no, product_id, description_snapshot, qty_milli, unit_price_micros, line_total, cost_micros)
				VALUES (?, 1, NULL, 'Servicio', 1000, ?, ?, ?)`, q, p.subtotal*10_000, p.subtotal, p.costMicros); err != nil {
				t.Fatal(err)
			}
		}
	}
	state := func(folio string, user int64, probability int, status, next string) string {
		nx := "null"
		if next != "" {
			nx = `"` + next + `"`
		}
		return `{"folio":"` + folio + `","customer_id":` + strconv.FormatInt(emsa, 10) + `,"user_id":` + strconv.FormatInt(user, 10) +
			`,"status":"` + status + `","probability":` + strconv.Itoa(probability) + `,"next_followup_date":` + nx + `}`
	}
	audit := func(folio, at, old, new string) {
		t.Helper()
		if _, err := s.db.Exec(`INSERT INTO audit_log (table_name, row_key, op, old_values, new_values, at) VALUES ('projects', ?, 'update', ?, ?, ?)`,
			ids[folio], old, new, ts(at)); err != nil {
			t.Fatal(err)
		}
	}
	comment := func(folio string, user int64, at string) {
		t.Helper()
		if _, err := s.db.Exec(`INSERT INTO quote_comments (quote_id, user_id, body, created_at) VALUES (?, ?, 'Llamé', ?)`, quotes[folio], user, ts(at)); err != nil {
			t.Fatal(err)
		}
	}
	audit("QA0001", "2026-11-10 11:00", state("QA0001", ana, 75, "prospecto", ""), state("QA0001", ana, 75, "prospecto", "2026-11-11"))
	audit("QA0001", "2026-11-12 10:00", state("QA0001", ana, 75, "prospecto", "2026-11-11"), state("QA0001", ana, 75, "prospecto", "2026-11-13"))
	comment("QA0001", ana, "2026-11-12 10:00")
	audit("QA0002", "2026-11-02 11:00", state("QA0002", ana, 25, "prospecto", ""), state("QA0002", ana, 25, "prospecto", "2026-11-04"))
	comment("QA0002", ana, "2026-11-04 16:00")
	audit("QA0003", "2026-11-05 11:00", state("QA0003", beto, 75, "prospecto", ""), state("QA0003", beto, 75, "prospecto", "2026-11-09"))
	comment("QA0003", beto, "2026-11-05 11:00")
	audit("QA0004", "2026-10-30 11:00", state("QA0004", beto, 50, "prospecto", ""), state("QA0004", beto, 50, "prospecto", "2026-11-02"))
	audit("QA0004", "2026-11-05 13:00", state("QA0004", beto, 50, "prospecto", "2026-11-02"), state("QA0004", beto, 50, "oc_recibida", "2026-11-02"))
	if _, err := s.db.Exec(`UPDATE projects SET status = 'oc_recibida' WHERE folio = 'QA0004'`); err != nil {
		t.Fatal(err)
	}
	audit("QA0005", "2026-11-05 11:00", state("QA0005", ggo, 75, "prospecto", ""), state("QA0005", ggo, 75, "prospecto", "2026-11-09"))

	aviso := func(user int64, folio, channel, status, at string) {
		t.Helper()
		if _, err := s.db.Exec(`INSERT INTO notifications (kind, channel, user_id, address, project_id, status, created_at) VALUES ('seguimiento_vencido', ?, ?, 'x', ?, ?, ?)`,
			channel, user, ids[folio], status, ts(at)); err != nil {
			t.Fatal(err)
		}
	}
	aviso(beto, "QA0003", "email", "aceptada", "2026-11-10 18:00")
	aviso(beto, "QA0003", "whatsapp", "aceptada", "2026-11-10 18:00") // the same aviso
	aviso(beto, "QA0004", "email", "aceptada", "2026-11-04 18:00")
	aviso(beto, "QA0003", "email", "cancelada", "2026-11-11 18:00")
	aviso(beto, "QA0003", "email", "aceptada", "2026-10-20 18:00") // before the range
	return teamSeed{s: s, ana: ana, beto: beto, carla: carla, now: mx(t, "2026-11-16 10:00")}
}

func TestTeamDashboard(t *testing.T) {
	seed := newTeamSeed(t)
	ctx := context.Background()
	month := TeamFilter{From: "2026-11-01", To: "2026-11-30"}
	team, err := seed.s.TeamDashboard(ctx, month, seed.now)
	if err != nil {
		t.Fatal(err)
	}
	type line struct {
		name                                         string
		active, overdue                              int
		value, profit                                money.Centavos
		avisos, escalated, resolved, late, due, ontm int
	}
	got := func(team *Team) []line {
		var out []line
		for _, m := range team.Members {
			out = append(out, line{m.Name, m.Active, m.Overdue, m.OverdueValue, m.OverdueProfit,
				m.Avisos, m.Escalated, m.Resolved, m.LateDays, m.DueDays, m.OnTime})
		}
		return out
	}
	check := func(name string, team *Team, want []line) {
		t.Helper()
		g := got(team)
		if len(g) != len(want) {
			t.Fatalf("%s: members = %+v, want %+v", name, g, want)
		}
		for i := range want {
			if g[i] != want[i] {
				t.Errorf("%s: member %d = %+v, want %+v", name, i, g[i], want[i])
			}
		}
	}
	// Ana is first: 1 of 3 on time against Beto's 0 of 2. Carla has nothing and ggo is
	// the development account: neither is listed.
	check("month", team, []line{
		{"Ana", 2, 1, 1_000_00, 250_00, 0, 0, 1, 1, 3, 1},
		{"Beto", 1, 1, 2_000_00, 500_00, 2, 2, 1, 3, 2, 0},
	})
	if team.Totals.OverdueValue != 3_000_00 || team.Totals.OverdueProfit != 750_00 || team.Totals.Avisos != 2 || team.Totals.DueDays != 5 {
		t.Errorf("totals = %+v", team.Totals)
	}
	if len(team.Overdue) != 2 || team.Overdue[0].Folio != "QA0003" || team.Overdue[0].DaysLate != 5 || team.Overdue[0].UserName != "Beto" ||
		team.Overdue[1].Folio != "QA0001" || team.Overdue[1].DaysLate != 1 || team.Overdue[1].Day != "2026-11-13" {
		t.Errorf("overdue = %+v", team.Overdue)
	}
	if avg, ok := team.Members[1].AverageLate(); !ok || avg != 3 {
		t.Errorf("Beto's average = %v, %v", avg, ok)
	}
	if team.Members[0].LastComment == "" {
		t.Error("Ana's last comment is missing")
	}

	// The week of the 9th: QA0002's day and QA0004's whole period fall outside it; what is
	// overdue is still today's.
	week, _ := seed.s.TeamDashboard(ctx, TeamFilter{From: "2026-11-09", To: "2026-11-15"}, seed.now)
	check("week", week, []line{
		{"Ana", 2, 1, 1_000_00, 250_00, 0, 0, 1, 1, 2, 0},
		{"Beto", 1, 1, 2_000_00, 500_00, 1, 1, 0, 0, 1, 0},
	})

	// Alta or more: QA0002 (25%) and QA0004 (50%) drop out, its aviso with it.
	alta, _ := seed.s.TeamDashboard(ctx, TeamFilter{From: "2026-11-01", To: "2026-11-30", MinProbability: 75}, seed.now)
	check("alta", alta, []line{
		{"Ana", 1, 1, 1_000_00, 250_00, 0, 0, 1, 1, 2, 0},
		{"Beto", 1, 1, 2_000_00, 500_00, 1, 1, 0, 0, 1, 0},
	})

	// One person: the rest drop from the table and the overdue list, not from the choices.
	beto, _ := seed.s.TeamDashboard(ctx, TeamFilter{From: "2026-11-01", To: "2026-11-30", Users: []int64{seed.beto}}, seed.now)
	if len(beto.Members) != 1 || beto.Members[0].Name != "Beto" || len(beto.Overdue) != 1 || len(beto.People) != 2 ||
		beto.Totals.OverdueValue != 2_000_00 {
		t.Errorf("Beto only = %+v", beto)
	}
}

func TestRankedBefore(t *testing.T) {
	m := func(onTime, due, resolved, late int) TeamMember {
		return TeamMember{OnTime: onTime, DueDays: due, Resolved: resolved, LateDays: late}
	}
	for _, c := range []struct {
		a, b TeamMember
		want bool
	}{
		{m(2, 3, 0, 0), m(1, 3, 0, 0), true},  // more on time
		{m(1, 2, 2, 2), m(2, 4, 1, 3), true},  // same share: lower average late
		{m(2, 4, 1, 3), m(1, 2, 2, 2), false}, //
		{m(0, 1, 1, 9), m(0, 0, 0, 0), true},  // nothing due goes last
		{m(0, 1, 0, 0), m(0, 1, 1, 1), false}, // no average goes after one
	} {
		if got := rankedBefore(c.a, c.b); got != c.want {
			t.Errorf("rankedBefore(%+v, %+v) = %v", c.a, c.b, got)
		}
	}
}
