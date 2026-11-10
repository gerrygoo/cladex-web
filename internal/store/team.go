package store

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/gerrygoo/cladex-web/internal/money"
)

// The Equipo view (issue #70; docs/planning/dashboard.md): how up to date each person is
// with their seguimientos, for admins. It is built on the same periods as Mis pendientes
// (seguimientoHistory). The period filter applies to the history (avisos, escalations,
// lateness, ranking); what is overdue and its money are always as of now.

// TeamFilter narrows the Equipo view. From and To are days (YYYY-MM-DD, Mexico City),
// both included. Users, when not empty, keeps only those people. MinProbability keeps
// proyectos (by their probabilidad now) and periods (by the one they opened with) at or
// above it; 0 keeps everything.
type TeamFilter struct {
	From, To       string
	Users          []int64
	MinProbability int
}

// TeamMember is one person's line on Equipo.
type TeamMember struct {
	UserID   int64
	Username string
	Name     string
	// Active is how many prospectos they own now; Overdue are the ones overdue now.
	Active        int
	Overdue       int
	OverdueValue  money.Centavos // subtotals before IVA
	OverdueProfit money.Centavos
	// Avisos is the seguimiento_vencido avisos they were sent in the range (one per
	// proyecto and round, whatever the channels; cancelled ones don't count). Escalated is
	// their periods that reached the team in the range.
	Avisos    int
	Escalated int
	// Resolved and LateDays are the periods that closed in the range and their days late
	// added up: LateDays / Resolved is the average.
	Resolved int
	LateDays int
	// DueDays are their follow-up days in the range that have come due, and OnTime the ones
	// among them that never opened a period.
	DueDays int
	OnTime  int
	// LastComment is their newest comment on any proyecto (UTC timestamp), "" if none.
	LastComment string
}

// AverageLate is the mean weekdays late of the periods resolved in the range, and false
// when none was.
func (m TeamMember) AverageLate() (float64, bool) {
	if m.Resolved == 0 {
		return 0, false
	}
	return float64(m.LateDays) / float64(m.Resolved), true
}

// OnTimeShare is OnTime over DueDays, and false when no follow-up day came due.
func (m TeamMember) OnTimeShare() (float64, bool) {
	if m.DueDays == 0 {
		return 0, false
	}
	return float64(m.OnTime) / float64(m.DueDays), true
}

// TeamOverdue is one proyecto overdue now.
type TeamOverdue struct {
	Folio, Customer string
	UserID          int64 // whose period it is
	UserName        string
	Probability     int    // the proyecto's now
	Day             string // its follow-up day
	DaysLate        int
	Value, Profit   money.Centavos // its current quote's subtotal and profit, before IVA
}

// Team is the Equipo view.
type Team struct {
	// Members are ranked: most seguimientos a tiempo first, ties to the lower average
	// lateness; people with no follow-up day due in the range go last.
	Members []TeamMember
	// Totals adds up the members (its Name is empty).
	Totals TeamMember
	// Overdue is every proyecto overdue now, most days late first.
	Overdue []TeamOverdue
	// People are everyone the view can show, by name, for the Persona filter.
	People []TeamMember
}

// devUsername is the development account, left out of the team as in DashboardSellers.
const devUsername = "ggo"

// TeamDashboard builds the Equipo view as of now.
func (s *Store) TeamDashboard(ctx context.Context, f TeamFilter, now time.Time) (*Team, error) {
	periods, days, err := s.seguimientoHistory(ctx, now)
	if err != nil {
		return nil, err
	}
	users, err := s.ActiveUsers(ctx)
	if err != nil {
		return nil, err
	}
	byID := map[int64]*TeamMember{}
	var order []int64
	for _, u := range users {
		if u.Username == devUsername {
			continue
		}
		byID[u.ID] = &TeamMember{UserID: u.ID, Username: u.Username, Name: u.Name}
		order = append(order, u.ID)
	}
	keepProb := func(p int) bool { return p >= f.MinProbability }
	involved := map[int64]bool{}

	// Proyectos now: who owns a prospecto, and each one's money.
	type prospect struct {
		user          int64
		probability   int
		value, profit money.Centavos
	}
	prospects := map[int64]prospect{}
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.user_id, p.probability, q.subtotal, `+quoteProfitSQL+`
		FROM projects p JOIN quotes q ON q.project_id = p.id AND q.status != 'revisada'
		WHERE p.status = 'prospecto'`)
	if err != nil {
		return nil, fmt.Errorf("store: team dashboard: %w", err)
	}
	for rows.Next() {
		var id int64
		var pr prospect
		if err := rows.Scan(&id, &pr.user, &pr.probability, &pr.value, &pr.profit); err != nil {
			rows.Close()
			return nil, fmt.Errorf("store: team dashboard: %w", err)
		}
		prospects[id] = pr
		involved[pr.user] = true
		if m := byID[pr.user]; m != nil && keepProb(pr.probability) {
			m.Active++
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: team dashboard: %w", err)
	}

	var overdue []TeamOverdue
	for _, p := range periods {
		m := byID[p.UserID]
		if m == nil {
			continue
		}
		openedDay := p.OpenedAt.In(mexicoCity).Format("2006-01-02")
		if p.Open() || openedDay <= f.To && inDays(p.ClosedAt, f.From, "") {
			involved[p.UserID] = true
		}
		if p.Open() {
			pr, ok := prospects[p.ProjectID]
			if !ok || !keepProb(pr.probability) {
				continue
			}
			m.Overdue++
			m.OverdueValue += pr.value
			m.OverdueProfit += pr.profit
			overdue = append(overdue, TeamOverdue{Folio: p.Folio, Customer: p.CustomerName, UserID: m.UserID, UserName: m.Name,
				Probability: pr.probability, Day: p.Day, DaysLate: p.DaysLate, Value: pr.value, Profit: pr.profit})
		}
		if !keepProb(p.Probability) {
			continue
		}
		if p.Escalated() && inDays(p.EscalatedAt, f.From, f.To) {
			m.Escalated++
		}
		if !p.Open() && inDays(p.ClosedAt, f.From, f.To) {
			m.Resolved++
			m.LateDays += p.DaysLate
		}
	}
	for _, d := range days {
		m := byID[d.UserID]
		if m == nil || d.Day < f.From || d.Day > f.To {
			continue
		}
		involved[d.UserID] = true
		if !keepProb(d.Probability) {
			continue
		}
		m.DueDays++
		if !d.Opened {
			m.OnTime++
		}
	}

	if err := s.teamAvisos(ctx, f, byID); err != nil {
		return nil, err
	}
	if err := s.teamLastComments(ctx, byID); err != nil {
		return nil, err
	}

	team := &Team{}
	wanted := map[int64]bool{}
	for _, id := range f.Users {
		wanted[id] = true
	}
	for _, id := range order {
		if !involved[id] {
			continue
		}
		m := *byID[id]
		team.People = append(team.People, m)
		if len(wanted) > 0 && !wanted[id] {
			continue
		}
		team.Members = append(team.Members, m)
		t := &team.Totals
		t.Active += m.Active
		t.Overdue += m.Overdue
		t.OverdueValue += m.OverdueValue
		t.OverdueProfit += m.OverdueProfit
		t.Avisos += m.Avisos
		t.Escalated += m.Escalated
		t.Resolved += m.Resolved
		t.LateDays += m.LateDays
		t.DueDays += m.DueDays
		t.OnTime += m.OnTime
		if m.LastComment > t.LastComment {
			t.LastComment = m.LastComment
		}
	}
	sort.SliceStable(team.Members, func(i, j int) bool { return rankedBefore(team.Members[i], team.Members[j]) })
	for _, o := range overdue {
		if len(wanted) == 0 || wanted[o.UserID] {
			team.Overdue = append(team.Overdue, o)
		}
	}
	sort.SliceStable(team.Overdue, func(i, j int) bool {
		if team.Overdue[i].DaysLate != team.Overdue[j].DaysLate {
			return team.Overdue[i].DaysLate > team.Overdue[j].DaysLate
		}
		return team.Overdue[i].Folio < team.Overdue[j].Folio
	})
	return team, nil
}

// rankedBefore orders the ranking: a higher share of seguimientos a tiempo first, then the
// lower average lateness, where someone with no period resolved in the range (no average
// to compare) goes after someone with one; people with no follow-up day due go last.
// Otherwise by name, which is the order they come in.
func rankedBefore(a, b TeamMember) bool {
	if (a.DueDays > 0) != (b.DueDays > 0) {
		return a.DueDays > 0
	}
	// The shares and averages are compared exactly, by cross-multiplying.
	if x, y := a.OnTime*b.DueDays, b.OnTime*a.DueDays; x != y {
		return x > y
	}
	if (a.Resolved > 0) != (b.Resolved > 0) {
		return a.Resolved > 0
	}
	return a.LateDays*b.Resolved < b.LateDays*a.Resolved
}

// teamAvisos counts each member's seguimiento_vencido avisos sent in the range: one per
// proyecto and round (its email and its WhatsApp are one aviso), cancelled ones left out.
// With a minimum probability, only avisos about proyectos at or above it now count.
func (s *Store) teamAvisos(ctx context.Context, f TeamFilter, byID map[int64]*TeamMember) error {
	from, err := time.ParseInLocation("2006-01-02", f.From, mexicoCity)
	if err != nil {
		return fmt.Errorf("store: team avisos: from %q: %w", f.From, err)
	}
	to, err := time.ParseInLocation("2006-01-02", f.To, mexicoCity)
	if err != nil {
		return fmt.Errorf("store: team avisos: to %q: %w", f.To, err)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT n.user_id, COUNT(DISTINCT COALESCE(n.project_id, '') || '|' || n.created_at)
		FROM notifications n LEFT JOIN projects p ON p.id = n.project_id
		WHERE n.kind = ? AND n.status != 'cancelada' AND n.user_id IS NOT NULL
			AND n.created_at >= ? AND n.created_at < ?
			AND (? = 0 OR p.probability >= ?)
		GROUP BY n.user_id`,
		AvisoSeguimientoVencido, jobTime(from), jobTime(to.AddDate(0, 0, 1)), f.MinProbability, f.MinProbability)
	if err != nil {
		return fmt.Errorf("store: team avisos: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return fmt.Errorf("store: team avisos: %w", err)
		}
		if m := byID[id]; m != nil {
			m.Avisos = n
		}
	}
	return rows.Err()
}

// teamLastComments sets each member's newest comment on a proyecto.
func (s *Store) teamLastComments(ctx context.Context, byID map[int64]*TeamMember) error {
	rows, err := s.db.QueryContext(ctx, `SELECT user_id, MAX(created_at) FROM quote_comments GROUP BY user_id`)
	if err != nil {
		return fmt.Errorf("store: team last comments: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var at string
		if err := rows.Scan(&id, &at); err != nil {
			return fmt.Errorf("store: team last comments: %w", err)
		}
		if m := byID[id]; m != nil {
			m.LastComment = at
		}
	}
	return rows.Err()
}
