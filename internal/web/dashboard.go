package web

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

// Dashboard serves the Dashboard tab (docs/planning/dashboard.md): Mis pendientes, the
// home page (issue #72). Everything on Mis pendientes is the signed-in user's own: each
// store call takes their id, and a pendiente id of someone else's is a 404, the same as
// one that doesn't exist.
type Dashboard struct {
	store *store.Store
	build Build
	now   func() time.Time
}

// NewDashboard returns the Dashboard handlers; build dates the home page's footer.
func NewDashboard(s *store.Store, build Build) *Dashboard {
	return &Dashboard{store: s, build: build, now: time.Now}
}

// completedWindow is how far back Completados reaches.
const completedWindow = 30 * 24 * time.Hour

// announcements are what the screen reader hears after each action on Mis pendientes,
// keyed by the "hecho" param the action redirects with.
var announcements = map[string]string{
	"agregado":   "Pendiente agregado.",
	"prioridad":  "Prioridad guardada.",
	"completado": "Pendiente completado.",
	"reabierto":  "Pendiente reabierto.",
}

// orden is the list's priority order: "asc" or the default "desc".
func orden(v string) string {
	if v == "asc" {
		return "asc"
	}
	return "desc"
}

// MisPendientes handles GET /: the user's pendientes, sorted by priority (?dir=asc for
// lowest first), Completados, and the Vendedores table.
func (d *Dashboard) MisPendientes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	m := views.MisPendientes{
		Dir:      orden(q.Get("dir")),
		Adding:   q.Get("agregar") == "1",
		Announce: announcements[q.Get("hecho")],
	}
	d.render(w, r, m, http.StatusOK)
}

func (d *Dashboard) render(w http.ResponseWriter, r *http.Request, m views.MisPendientes, status int) {
	ctx := r.Context()
	user, _ := UserFromContext(ctx)
	now := d.now()
	since := now.Add(-completedWindow)
	m.Today = store.InMexicoCity(now).Format("2006-01-02")

	periods, err := d.store.SeguimientoPeriods(ctx, store.InMexicoCity(since).Format("2006-01-02"), "", now)
	if err != nil {
		serverError(w, r, err)
		return
	}
	for i := range periods {
		p := &periods[i]
		switch {
		case p.UserID != user.ID:
		case p.Open():
			m.Items = append(m.Items, views.PendienteItem{Period: p})
		case !p.ClosedAt.Before(since):
			m.Done = append(m.Done, views.DoneItem{Period: p, At: p.ClosedAt})
		}
	}
	open, err := d.store.OpenPendientes(ctx, user.ID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	for i := range open {
		m.Items = append(m.Items, views.PendienteItem{Pendiente: &open[i]})
	}
	done, err := d.store.CompletedPendientes(ctx, user.ID, since)
	if err != nil {
		serverError(w, r, err)
		return
	}
	for i := range done {
		at, _ := time.Parse(time.RFC3339Nano, done[i].CompletedAt)
		m.Done = append(m.Done, views.DoneItem{Pendiente: &done[i], At: at})
	}
	views.SortPendientes(m.Items, m.Dir)
	views.SortDone(m.Done)

	if m.Sales, err = d.store.SalesDashboard(ctx, today(r)); err != nil {
		serverError(w, r, err)
		return
	}
	m.Built, m.Now, m.BuildSHA = d.build.Time, now, d.build.SHA
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	views.MisPendientesPage(m, navUserView(user)).Render(ctx, w)
}

// back redirects to Mis pendientes in the order the form was in, saying what was done.
func back(w http.ResponseWriter, r *http.Request, hecho string) {
	v := url.Values{"hecho": {hecho}}
	if orden(r.FormValue("orden")) == "asc" {
		v.Set("sort", "prioridad")
		v.Set("dir", "asc")
	}
	http.Redirect(w, r, "/?"+v.Encode(), http.StatusSeeOther)
}

// AgregarPendiente handles POST /pendientes: a new pendiente libre with its
// "descripcion" and "prioridad" (media when missing), dated today.
func (d *Dashboard) AgregarPendiente(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, _ := UserFromContext(ctx)
	body := strings.TrimSpace(r.FormValue("descripcion"))
	priority := r.FormValue("prioridad")
	if priority == "" {
		priority = store.PriorityMedia
	}
	if store.PriorityRank(priority) == 0 {
		http.Error(w, "prioridad desconocida", http.StatusBadRequest)
		return
	}
	var msg string
	switch {
	case body == "":
		msg = "Escribe qué tienes pendiente."
	case utf8.RuneCountInString(body) > store.MaxPendienteLen:
		msg = "La descripción no puede pasar de " + strconv.Itoa(store.MaxPendienteLen) + " caracteres."
	}
	if msg != "" {
		status := http.StatusUnprocessableEntity
		if r.Header.Get("HX-Request") == "true" {
			status = http.StatusOK // htmx only swaps a 2xx
		}
		d.render(w, r, views.MisPendientes{
			Dir: orden(r.FormValue("orden")), Adding: true, Body: body, Priority: priority,
			Errors: map[string]string{"descripcion": msg}, Announce: msg,
		}, status)
		return
	}
	if _, err := d.store.AddPendiente(ctx, user.ID, body, priority); err != nil {
		serverError(w, r, err)
		return
	}
	back(w, r, "agregado")
}

// pendienteID reads the {id} of a pendiente route, answering 404 for one that isn't a number.
func pendienteID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return 0, false
	}
	return id, true
}

// pendienteDone finishes a write on one pendiente: 404 when it isn't the user's (or
// isn't in a state for it), back to the list otherwise.
func pendienteDone(w http.ResponseWriter, r *http.Request, err error, hecho string) {
	switch {
	case errors.Is(err, store.ErrPendienteNotFound):
		http.Error(w, "pendiente no encontrado", http.StatusNotFound)
	case err != nil:
		serverError(w, r, err)
	default:
		back(w, r, hecho)
	}
}

// Prioridad handles POST /pendientes/{id}/prioridad: changes an open pendiente libre's
// "prioridad".
func (d *Dashboard) Prioridad(w http.ResponseWriter, r *http.Request) {
	id, ok := pendienteID(w, r)
	if !ok {
		return
	}
	priority := r.FormValue("prioridad")
	if store.PriorityRank(priority) == 0 {
		http.Error(w, "prioridad desconocida", http.StatusBadRequest)
		return
	}
	user, _ := UserFromContext(r.Context())
	pendienteDone(w, r, d.store.SetPendientePriority(r.Context(), user.ID, id, priority), "prioridad")
}

// Completar handles POST /pendientes/{id}/completar: checks off an open pendiente libre.
func (d *Dashboard) Completar(w http.ResponseWriter, r *http.Request) {
	id, ok := pendienteID(w, r)
	if !ok {
		return
	}
	user, _ := UserFromContext(r.Context())
	pendienteDone(w, r, d.store.CompletePendiente(r.Context(), user.ID, id, d.now()), "completado")
}

// Reabrir handles POST /pendientes/{id}/reabrir: puts a completed pendiente libre back.
func (d *Dashboard) Reabrir(w http.ResponseWriter, r *http.Request) {
	id, ok := pendienteID(w, r)
	if !ok {
		return
	}
	user, _ := UserFromContext(r.Context())
	pendienteDone(w, r, d.store.ReopenPendiente(r.Context(), user.ID, id), "reabierto")
}

// Equipo handles GET /dashboard/equipo (admin, issue #70): each person's seguimiento in a
// period ("periodo" semana, mes, the default, or rango with "desde" and "hasta"), for some
// people (the Persona column filter) and from a probabilidad mínima ("prob").
func (d *Dashboard) Equipo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	now := d.now()
	e := views.Equipo{Periodo: q.Get("periodo")}
	if e.Periodo != "semana" && e.Periodo != "rango" {
		e.Periodo = "mes"
	}
	e.Desde, e.Hasta, e.Error = equipoRange(e.Periodo, q.Get("desde"), q.Get("hasta"), now)
	if e.Error != "" {
		e.Periodo = "mes"
		e.Desde, e.Hasta, _ = equipoRange("mes", "", "", now)
	}
	if p, err := strconv.Atoi(q.Get("prob")); err == nil && store.ProbabilityLabel(p) != "" {
		e.MinProb = p
	}
	e.LV = views.ListView{Base: "/dashboard/equipo", Filters: store.Filters{}, Keep: url.Values{"periodo": {e.Periodo}}}
	if e.Periodo == "rango" {
		e.LV.Keep.Set("desde", e.Desde)
		e.LV.Keep.Set("hasta", e.Hasta)
	}
	if e.MinProb > 0 {
		e.LV.Keep.Set("prob", strconv.Itoa(e.MinProb))
	}
	f := store.TeamFilter{From: e.Desde, To: e.Hasta, MinProbability: e.MinProb}
	if names := filterParams(r)["persona.is"]; len(names) > 0 {
		e.LV.Filters["persona.is"] = names
		users, err := d.store.ActiveUsers(ctx)
		if err != nil {
			serverError(w, r, err)
			return
		}
		for _, u := range users {
			for _, n := range names {
				if strings.EqualFold(u.Username, n) {
					f.Users = append(f.Users, u.ID)
				}
			}
		}
		if len(f.Users) == 0 {
			f.Users = []int64{-1} // nobody chosen exists any more: show nobody
		}
	}
	team, err := d.store.TeamDashboard(ctx, f, now)
	if err != nil {
		serverError(w, r, err)
		return
	}
	e.Team = team
	user, _ := UserFromContext(ctx)
	views.EquipoPage(e, navUserView(user)).Render(ctx, w)
}

// equipoRange is the days (YYYY-MM-DD, Mexico City) a periodo covers: this week, Monday
// to Sunday; this month; or desde to hasta. A rango without both days, or backwards, is
// an error message.
func equipoRange(periodo, desde, hasta string, now time.Time) (from, to, errMsg string) {
	today := store.InMexicoCity(now)
	day := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	switch periodo {
	case "semana":
		monday := day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
		return monday.Format("2006-01-02"), monday.AddDate(0, 0, 6).Format("2006-01-02"), ""
	case "rango":
		a, errA := time.Parse("2006-01-02", desde)
		b, errB := time.Parse("2006-01-02", hasta)
		if errA != nil || errB != nil || b.Before(a) {
			return "", "", "Para un rango, elige el día Desde y el día Hasta, el primero antes del segundo."
		}
		return desde, hasta, ""
	}
	first := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, day.Location())
	return first.Format("2006-01-02"), first.AddDate(0, 1, -1).Format("2006-01-02"), ""
}
