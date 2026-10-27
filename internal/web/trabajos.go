package web

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gerrygoo/cladex-web/internal/jobs"
	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

// recentJobs is how many finished jobs the Trabajos page lists.
const recentJobs = 50

// Trabajos is the admin-only /ajustes/trabajos page: the background job queue
// (internal/jobs), with its errors and a way to retry.
type Trabajos struct {
	store  *store.Store
	runner *jobs.Runner
}

func NewTrabajos(s *store.Store, runner *jobs.Runner) *Trabajos {
	return &Trabajos{store: s, runner: runner}
}

func (t *Trabajos) Page(w http.ResponseWriter, r *http.Request) {
	msg := ""
	switch {
	case r.URL.Query().Get("reintentado") == "1":
		msg = "El trabajo se volvió a poner en la cola."
	case r.URL.Query().Get("prueba") == "1":
		msg = "Trabajo de prueba en la cola. Recarga la página en unos segundos para ver si se ejecutó."
	}
	t.render(w, r, msg, "")
}

func (t *Trabajos) render(w http.ResponseWriter, r *http.Request, successMsg, errorMsg string) {
	ctx := r.Context()
	overview, err := t.store.ListJobs(ctx, recentJobs)
	if err != nil {
		serverError(w, r, err)
		return
	}
	if errorMsg != "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	user, _ := UserFromContext(ctx)
	views.Trabajos(overview, successMsg, errorMsg, navUserView(user)).Render(ctx, w)
}

// Reintentar handles POST /ajustes/trabajos/{id}/reintentar: a failed job goes back in
// the queue, and a pending one runs now instead of waiting out its backoff.
func (t *Trabajos) Reintentar(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := t.store.RetryJobNow(r.Context(), id, time.Now()); err != nil {
		if errors.Is(err, store.ErrJobState) {
			t.render(w, r, "", "Ese trabajo ya no se puede reintentar: está en curso o ya terminó.")
			return
		}
		serverError(w, r, err)
		return
	}
	t.runner.Wake()
	http.Redirect(w, r, "/ajustes/trabajos?reintentado=1", http.StatusSeeOther)
}

// Prueba handles POST /ajustes/trabajos/prueba: queues a job that does nothing, to see
// the runner pick it up.
func (t *Trabajos) Prueba(w http.ResponseWriter, r *http.Request) {
	if _, err := t.runner.Enqueue(r.Context(), store.NewJob{Kind: jobs.KindPrueba}); err != nil {
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/ajustes/trabajos?prueba=1", http.StatusSeeOther)
}
