package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

// Projects holds the /proyectos handlers: the list, a proyecto's page, and the actions
// that follow it up (stage, probabilidad de cierre, comments).
type Projects struct {
	store *store.Store
}

func NewProjects(s *store.Store) *Projects {
	return &Projects{store: s}
}

// List handles GET /proyectos: every proyecto, searchable, sortable and filterable by
// column like the other lists.
func (p *Projects) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	sortCol, dir := sortParams(r)
	filters := filterParams(r)
	projects, err := p.store.ListProjects(ctx, query, sortCol, dir, filters)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	projects, pager := paginate(r, projects)
	lv := views.ListView{Base: "/proyectos", Query: query, Sort: sortCol, Dir: dir, Filters: filters, Pager: pager}
	if r.Header.Get("HX-Request") == "true" {
		views.ProjectsTableBody(projects).Render(ctx, w)
		views.ListPagerOOB(lv).Render(ctx, w)
		return
	}
	customers, owners, err := p.store.ProjectFilterChoices(ctx)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	user, _ := UserFromContext(ctx)
	views.ProjectsList(projects, lv, customers, owners, navUserView(user)).Render(ctx, w)
}

func (p *Projects) loadProjectOrNotFound(w http.ResponseWriter, r *http.Request) *store.Project {
	project, err := p.store.ProjectByFolio(r.Context(), r.PathValue("folio"))
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return nil
	}
	if project == nil {
		http.NotFound(w, r)
		return nil
	}
	return project
}

// Page handles GET /proyectos/{folio}: the proyecto, its quotes and its history.
func (p *Projects) Page(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := p.loadProjectOrNotFound(w, r)
	if project == nil {
		return
	}
	quotes, err := p.store.ListProjectQuotes(ctx, project.ID)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	comments, err := p.store.ListProjectComments(ctx, project.ID)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	comments, pager := paginate(r, comments)
	commentsLV := views.ListView{Base: "/proyectos/" + project.Folio, Pager: pager, Anchor: "historial"}
	user, _ := UserFromContext(ctx)
	views.ProjectPage(*project, today(r), quotes, comments, commentsLV, navUserView(user)).Render(ctx, w)
}

// Etapa handles POST /proyectos/{folio}/etapa: moves the proyecto one stage along the
// flow ("to" is the stage; the optional "note" is saved as a comment). Anyone signed in
// can move a proyecto forward; going back a stage is for admins, since it undoes what a
// salesperson reported. Anything but the stage right before or after the current one is
// refused, as is a stale click on a proyecto that has since moved.
func (p *Projects) Etapa(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := p.loadProjectOrNotFound(w, r)
	if project == nil {
		return
	}
	to := r.FormValue("to")
	note := strings.TrimSpace(r.FormValue("note"))
	if utf8.RuneCountInString(note) > maxCommentLen {
		http.Error(w, "el comentario no puede pasar de 2000 caracteres", http.StatusBadRequest)
		return
	}
	user, _ := UserFromContext(ctx)
	if to != "" && to == store.PrevStage(project.Status) && user.Role != "admin" {
		http.Error(w, "solo un administrador puede regresar un proyecto de etapa", http.StatusForbidden)
		return
	}
	if err := p.store.MoveProject(ctx, project.ID, user.ID, project.Status, to, note); err != nil {
		if errors.Is(err, store.ErrBadTransition) {
			http.Error(w, "el proyecto no puede pasar a esa etapa", http.StatusConflict)
			return
		}
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/proyectos/"+project.Folio+"#historial", http.StatusSeeOther)
}

// Seguimiento handles POST /proyectos/{folio}/seguimiento: saves a prospecto's
// follow-up — "probabilidad" (the percentage of one of the fixed steps), "oc_esperada"
// and "proximo_seguimiento" (days, either may be blank to clear it) — with an optional
// "note" in the comment that records what changed. Any signed-in user can, while the
// proyecto is a prospecto. "volver" sends the user back to the Pronóstico page it was
// submitted from.
func (p *Projects) Seguimiento(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := p.loadProjectOrNotFound(w, r)
	if project == nil {
		return
	}
	note := strings.TrimSpace(r.FormValue("note"))
	if utf8.RuneCountInString(note) > maxCommentLen {
		http.Error(w, "el comentario no puede pasar de 2000 caracteres", http.StatusBadRequest)
		return
	}
	percent, err := strconv.Atoi(r.FormValue("probabilidad"))
	if err != nil || store.ProbabilityLabel(percent) == "" {
		http.Error(w, "elige una probabilidad de la lista", http.StatusBadRequest)
		return
	}
	expectedOC := strings.TrimSpace(r.FormValue("oc_esperada"))
	nextFollowUp := strings.TrimSpace(r.FormValue("proximo_seguimiento"))
	for _, d := range []string{expectedOC, nextFollowUp} {
		if _, err := time.Parse("2006-01-02", d); d != "" && err != nil {
			http.Error(w, "las fechas deben tener el formato AAAA-MM-DD", http.StatusBadRequest)
			return
		}
	}
	user, _ := UserFromContext(ctx)
	err = p.store.FollowUpProject(ctx, project.ID, user.ID, store.FollowUp{
		Probability: &percent, ExpectedOC: &expectedOC, NextFollowUp: &nextFollowUp, Note: note,
	})
	if err != nil {
		if errors.Is(err, store.ErrBadTransition) {
			http.Error(w, "solo a un prospecto se le anota probabilidad y fechas de seguimiento", http.StatusConflict)
			return
		}
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	switch r.FormValue("volver") {
	case "pronostico":
		http.Redirect(w, r, "/pronostico#p-"+project.Folio, http.StatusSeeOther)
	case "pronostico-todos":
		http.Redirect(w, r, "/pronostico?todos=1#p-"+project.Folio, http.StatusSeeOther)
	default:
		http.Redirect(w, r, "/proyectos/"+project.Folio+"#historial", http.StatusSeeOther)
	}
}

// today is the viewer's current day (YYYY-MM-DD), in the zone their browser reported.
func today(r *http.Request) string {
	return time.Now().In(store.LocationFromContext(r.Context())).Format("2006-01-02")
}

// Pronostico handles GET /pronostico: the prospectos relevante para pronóstico, or every
// prospecto with ?todos=1, each with its follow-up form.
func (p *Projects) Pronostico(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	all := r.URL.Query().Get("todos") == "1"
	projects, err := p.store.ListProspects(ctx, all)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	user, _ := UserFromContext(ctx)
	views.ForecastPage(views.Forecast{Projects: projects, All: all, Today: today(r)}, navUserView(user)).Render(ctx, w)
}

// Comentar handles POST /proyectos/{folio}/comentarios: any signed-in user adds a note
// to the proyecto's history. It is saved on the proyecto's current quote, so it also
// shows on that quote's page.
func (p *Projects) Comentar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := p.loadProjectOrNotFound(w, r)
	if project == nil {
		return
	}
	body := strings.TrimSpace(r.FormValue("comment"))
	if body == "" || utf8.RuneCountInString(body) > maxCommentLen {
		http.Error(w, "el comentario no puede estar vacío ni pasar de 2000 caracteres", http.StatusBadRequest)
		return
	}
	user, _ := UserFromContext(ctx)
	if err := p.store.AddQuoteComment(ctx, project.CurrentQuoteID, user.ID, body); err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/proyectos/"+project.Folio+"#historial", http.StatusSeeOther)
}

// Perder handles POST /proyectos/{folio}/perder: marks the proyecto lost with a required
// "reason". Any signed-in user can, while it is a prospecto or has its purchase order
// in; a proyecto already being delivered or closed can't be lost.
func (p *Projects) Perder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := p.loadProjectOrNotFound(w, r)
	if project == nil {
		return
	}
	reason := strings.TrimSpace(r.FormValue("reason"))
	if reason == "" || utf8.RuneCountInString(reason) > maxCommentLen {
		http.Error(w, "escribe el motivo, de no más de 2000 caracteres", http.StatusBadRequest)
		return
	}
	user, _ := UserFromContext(ctx)
	if err := p.store.LoseProject(ctx, project.ID, user.ID, project.Status, reason); err != nil {
		if errors.Is(err, store.ErrBadTransition) {
			http.Error(w, "este proyecto ya no se puede marcar como perdido", http.StatusConflict)
			return
		}
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/proyectos/"+project.Folio, http.StatusSeeOther)
}

// Reabrir handles POST /proyectos/{folio}/reabrir: brings a lost proyecto back to the
// stage it was lost from (the optional "note" goes into the comment that records it).
// Admin-only, enforced by the router, like going back a stage.
func (p *Projects) Reabrir(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := p.loadProjectOrNotFound(w, r)
	if project == nil {
		return
	}
	note := strings.TrimSpace(r.FormValue("note"))
	if utf8.RuneCountInString(note) > maxCommentLen {
		http.Error(w, "el comentario no puede pasar de 2000 caracteres", http.StatusBadRequest)
		return
	}
	user, _ := UserFromContext(ctx)
	if err := p.store.ReopenProject(ctx, project.ID, user.ID, note); err != nil {
		if errors.Is(err, store.ErrBadTransition) {
			http.Error(w, "este proyecto no está perdido", http.StatusConflict)
			return
		}
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/proyectos/"+project.Folio+"#historial", http.StatusSeeOther)
}
