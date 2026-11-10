package store

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"
)

// Seguimiento periods (issues #72 and #70; docs/planning/dashboard.md). A period is the
// stretch during which one follow-up day D of a prospecto is overdue: the rule of the
// avisos (internal/avisos, issue #69) applied to the proyecto's whole history. Periods
// are derived, never stored: the audit log has every change to a proyecto's status,
// user_id and next_followup_date with its time, and quote_comments has each comment's,
// so every period, past ones included, is rebuilt from them.

// MexicoCity is the zone Cladex's days are counted in: seguimiento days, the avisos'
// rounds, a stamp's or an O.C.'s day. Mexico has kept UTC-6 all year since 2022.
func MexicoCity() *time.Location { return mexicoCity }

// IsWeekday reports whether t falls Monday to Friday.
func IsWeekday(t time.Time) bool { return t.Weekday() != time.Saturday && t.Weekday() != time.Sunday }

// WeekdaysAfter counts the weekdays after day d up to and including today, both
// midnights in the same zone. It is how late a seguimiento is: a Friday's is 1 on Monday.
func WeekdaysAfter(d, today time.Time) int {
	n := 0
	for t := d.AddDate(0, 0, 1); !t.After(today); t = t.AddDate(0, 0, 1) {
		if IsWeekday(t) {
			n++
		}
	}
	return n
}

// midnight is the start of t's day in loc.
func midnight(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

// nthWeekdayAfter is the midnight of the n-th weekday after day d (a midnight).
func nthWeekdayAfter(d time.Time, n int) time.Time {
	t := d
	for n > 0 {
		t = t.AddDate(0, 0, 1)
		if IsWeekday(t) {
			n--
		}
	}
	return t
}

// Why a seguimiento period closed.
const (
	// ClosedComentario: someone commented on the proyecto (a follow-up writes one too).
	ClosedComentario = "comentario"
	// ClosedNuevaFecha: the proyecto's next follow-up day was changed or cleared.
	ClosedNuevaFecha = "nueva_fecha"
	// ClosedCambioEtapa: the proyecto stopped being a prospecto, or was deleted.
	ClosedCambioEtapa = "cambio_de_etapa"
)

// SeguimientoPeriod is one overdue follow-up day of a proyecto, open or closed.
type SeguimientoPeriod struct {
	ProjectID    int64
	Folio        string
	CustomerName string
	// Deleted is set when the proyecto no longer exists: there is no page to link to.
	Deleted bool
	// UserID is the proyecto's owner when the period opened, and Probability its
	// probabilidad de cierre then.
	UserID      int64
	Probability int
	// LastUserID is the owner when the period closed, or now while it is open: the person
	// who can act on it, so Mis pendientes lists it under them (as the avisos do) while the
	// Equipo stats count it for UserID.
	LastUserID int64
	// Day is the follow-up day D (YYYY-MM-DD).
	Day string
	// OpenedAt is the start of the first weekday after D (or, rarely, the moment D was set,
	// if later). ClosedAt is zero while the period is open; Reason says why it closed.
	OpenedAt time.Time
	ClosedAt time.Time
	Reason   string
	// DaysLate is n, the weekdays after D up to the closing day, or up to today while open.
	DaysLate int
	// EscalatedAt is 12:00 of the second weekday after D when the period was still open
	// then (the team aviso's first step); zero otherwise.
	EscalatedAt time.Time
}

// Open reports whether the follow-up is still overdue.
func (p SeguimientoPeriod) Open() bool { return p.ClosedAt.IsZero() }

// Escalated reports whether the period reached the team: still open at 12:00 of day 2.
func (p SeguimientoPeriod) Escalated() bool { return !p.EscalatedAt.IsZero() }

// Priority is how urgent the period is on Mis pendientes: media on day 1, alta on day 2,
// muy alta from day 3 on. It can't be changed by hand.
func (p SeguimientoPeriod) Priority() string {
	switch {
	case p.DaysLate >= 3:
		return PriorityMuyAlta
	case p.DaysLate == 2:
		return PriorityAlta
	default:
		return PriorityMedia
	}
}

// seguimientoDay is one follow-up day that came due (the start of the first weekday
// after it has passed) and whether it opened a period: the ranking's "a tiempo" counts
// the ones that didn't.
type seguimientoDay struct {
	ProjectID   int64
	UserID      int64
	Probability int
	Day         string
	Opened      bool
}

// projectState is a proyecto's audited columns from one moment on.
type projectState struct {
	At          time.Time // zero for the state before its first audit row
	Exists      bool
	Status      string
	UserID      int64
	Probability int
	Next        string
}

func (st projectState) followingUp() bool {
	return st.Exists && st.Status == "prospecto" && st.Next != ""
}

// projectTimeline is what the replay needs of one proyecto: its states in order and its
// comments' times, sorted.
type projectTimeline struct {
	ID         int64
	Folio      string
	CustomerID int64
	States     []projectState
	Comments   []time.Time
}

// replaySeguimientos rebuilds a proyecto's periods and the follow-up days that came due,
// as of now, counting days in loc.
func replaySeguimientos(h projectTimeline, now time.Time, loc *time.Location) ([]SeguimientoPeriod, []seguimientoDay) {
	var periods []SeguimientoPeriod
	var days []seguimientoDay
	// Only what had happened by now counts.
	var states []projectState
	for _, st := range h.States {
		if !st.At.After(now) {
			states = append(states, st)
		}
	}
	h.States = states
	h.Comments = h.Comments[:sort.Search(len(h.Comments), func(k int) bool { return h.Comments[k].After(now) })]
	deleted := len(h.States) > 0 && !h.States[len(h.States)-1].Exists
	for i := 0; i < len(h.States); i++ {
		st := h.States[i]
		if !st.followingUp() {
			continue
		}
		// The interval [a, b) in which the proyecto stays a prospecto with this same day;
		// a reassignment or a new probability doesn't break it.
		j := i + 1
		for j < len(h.States) && h.States[j].followingUp() && h.States[j].Next == st.Next {
			j++
		}
		interval := h.States[i:j]
		var end *projectState
		if j < len(h.States) {
			end = &h.States[j]
		}
		i = j - 1

		d, err := time.ParseInLocation("2006-01-02", st.Next, loc)
		if err != nil {
			continue
		}
		if end != nil && !end.At.After(d) {
			continue // replaced before the day came: not a follow-up day
		}
		w := nthWeekdayAfter(d, 1)
		if w.After(now) {
			continue // not due yet
		}
		opens := st.At
		if w.After(opens) {
			opens = w
		}
		stateAt := func(t time.Time) projectState {
			s := interval[0]
			for _, x := range interval[1:] {
				if x.At.After(t) {
					break
				}
				s = x
			}
			return s
		}
		owner := stateAt(opens)
		// The first comment from the start of D on.
		k := sort.Search(len(h.Comments), func(k int) bool { return !h.Comments[k].Before(d) })
		var comment time.Time
		if k < len(h.Comments) {
			comment = h.Comments[k]
		}
		attended := !comment.IsZero() && !comment.After(opens)
		closedFirst := end != nil && !opens.Before(end.At)
		opened := !attended && !closedFirst
		days = append(days, seguimientoDay{ProjectID: h.ID, UserID: owner.UserID, Probability: owner.Probability, Day: st.Next, Opened: opened})
		if !opened {
			continue
		}

		p := SeguimientoPeriod{
			ProjectID: h.ID, Folio: h.Folio, Deleted: deleted,
			UserID: owner.UserID, Probability: owner.Probability,
			Day: st.Next, OpenedAt: opens,
		}
		switch {
		case !comment.IsZero() && (end == nil || comment.Before(end.At)):
			p.ClosedAt, p.Reason = comment, ClosedComentario
		case end != nil && (!end.Exists || end.Status != "prospecto"):
			p.ClosedAt, p.Reason = end.At, ClosedCambioEtapa
		case end != nil:
			p.ClosedAt, p.Reason = end.At, ClosedNuevaFecha
		}
		last := now
		if !p.Open() {
			last = p.ClosedAt
		}
		p.LastUserID = stateAt(last).UserID
		p.DaysLate = WeekdaysAfter(d, midnight(last, loc))
		escalation := nthWeekdayAfter(d, 2).Add(12 * time.Hour)
		stillOpen := (p.Open() && !now.Before(escalation)) || p.ClosedAt.After(escalation)
		if !opens.After(escalation) && stillOpen {
			p.EscalatedAt = escalation
		}
		periods = append(periods, p)
	}
	return periods, days
}

// seguimientoHistory replays every proyecto's history as of now.
func (s *Store) seguimientoHistory(ctx context.Context, now time.Time) ([]SeguimientoPeriod, []seguimientoDay, error) {
	histories, err := s.projectTimelines(ctx)
	if err != nil {
		return nil, nil, err
	}
	customers, err := s.customerNames(ctx)
	if err != nil {
		return nil, nil, err
	}
	var periods []SeguimientoPeriod
	var days []seguimientoDay
	for _, h := range histories {
		ps, ds := replaySeguimientos(*h, now, mexicoCity)
		for i := range ps {
			ps[i].CustomerName = customers[h.CustomerID]
		}
		periods = append(periods, ps...)
		days = append(days, ds...)
	}
	sort.SliceStable(periods, func(i, j int) bool {
		if periods[i].Day != periods[j].Day {
			return periods[i].Day < periods[j].Day
		}
		return periods[i].Folio < periods[j].Folio
	})
	return periods, days, nil
}

// SeguimientoPeriods returns the periods open now plus the ones that closed on a day in
// [from, to] (YYYY-MM-DD, Mexico City; "" leaves that end open), oldest follow-up day
// first. Mis pendientes and the Equipo view both build on it.
func (s *Store) SeguimientoPeriods(ctx context.Context, from, to string, now time.Time) ([]SeguimientoPeriod, error) {
	all, _, err := s.seguimientoHistory(ctx, now)
	if err != nil {
		return nil, err
	}
	var out []SeguimientoPeriod
	for _, p := range all {
		if p.Open() || inDays(p.ClosedAt, from, to) {
			out = append(out, p)
		}
	}
	return out, nil
}

// inDays reports whether t's Mexico City day is within [from, to].
func inDays(t time.Time, from, to string) bool {
	day := t.In(mexicoCity).Format("2006-01-02")
	return (from == "" || day >= from) && (to == "" || day <= to)
}

// projectTimelines reads every proyecto's audited states and comment times.
func (s *Store) projectTimelines(ctx context.Context) ([]*projectTimeline, error) {
	const cols = `json_extract(%[1]s, '$.status'), json_extract(%[1]s, '$.user_id'),
		json_extract(%[1]s, '$.probability'), json_extract(%[1]s, '$.next_followup_date'),
		json_extract(%[1]s, '$.folio'), json_extract(%[1]s, '$.customer_id')`
	rows, err := s.db.QueryContext(ctx, `SELECT CAST(row_key AS INTEGER), op, at, `+
		fmt.Sprintf(cols, "old_values")+`, `+fmt.Sprintf(cols, "new_values")+`
		FROM audit_log WHERE table_name = 'projects' ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("store: project histories: %w", err)
	}
	defer rows.Close()
	byID := map[int64]*projectTimeline{}
	var order []*projectTimeline
	type vals struct {
		status, next, folio  sql.NullString
		user, prob, customer sql.NullInt64
	}
	state := func(v vals, at time.Time) projectState {
		return projectState{At: at, Exists: true, Status: v.status.String, UserID: v.user.Int64,
			Probability: int(v.prob.Int64), Next: v.next.String}
	}
	for rows.Next() {
		var id int64
		var op, at string
		var o, n vals
		if err := rows.Scan(&id, &op, &at,
			&o.status, &o.user, &o.prob, &o.next, &o.folio, &o.customer,
			&n.status, &n.user, &n.prob, &n.next, &n.folio, &n.customer); err != nil {
			return nil, fmt.Errorf("store: project histories: %w", err)
		}
		t, err := time.Parse(time.RFC3339Nano, at)
		if err != nil {
			return nil, fmt.Errorf("store: project histories: audit time %q: %w", at, err)
		}
		h := byID[id]
		if h == nil {
			h = &projectTimeline{ID: id}
			byID[id] = h
			order = append(order, h)
			// A proyecto from before the audit trail (0016 moved them over without
			// triggers) starts from what its first audited change replaced.
			if op != "insert" {
				h.States = append(h.States, state(o, time.Time{}))
			}
		}
		latest := n
		if op == "delete" {
			latest = o
			st := state(o, t)
			st.Exists = false
			h.States = append(h.States, st)
		} else {
			h.States = append(h.States, state(n, t))
		}
		if latest.folio.Valid {
			h.Folio = latest.folio.String
		}
		if latest.customer.Valid {
			h.CustomerID = latest.customer.Int64
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: project histories: %w", err)
	}

	crows, err := s.db.QueryContext(ctx, `SELECT q.project_id, c.created_at FROM quote_comments c
		JOIN quotes q ON q.id = c.quote_id WHERE q.project_id IS NOT NULL ORDER BY c.created_at, c.id`)
	if err != nil {
		return nil, fmt.Errorf("store: project comments: %w", err)
	}
	defer crows.Close()
	for crows.Next() {
		var id int64
		var at string
		if err := crows.Scan(&id, &at); err != nil {
			return nil, fmt.Errorf("store: project comments: %w", err)
		}
		t, err := time.Parse(time.RFC3339Nano, at)
		if err != nil {
			return nil, fmt.Errorf("store: project comments: time %q: %w", at, err)
		}
		if h := byID[id]; h != nil {
			h.Comments = append(h.Comments, t)
		}
	}
	if err := crows.Err(); err != nil {
		return nil, fmt.Errorf("store: project comments: %w", err)
	}
	for _, h := range order {
		sort.Slice(h.Comments, func(i, j int) bool { return h.Comments[i].Before(h.Comments[j]) })
	}
	return order, nil
}

// customerNames maps every customer's id to its name, deleted ones included.
func (s *Store) customerNames(ctx context.Context) (map[int64]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name FROM customers`)
	if err != nil {
		return nil, fmt.Errorf("store: customer names: %w", err)
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("store: customer names: %w", err)
		}
		out[id] = name
	}
	return out, rows.Err()
}
