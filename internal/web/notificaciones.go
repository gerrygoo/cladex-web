package web

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gerrygoo/cladex-web/internal/jobs"
	"github.com/gerrygoo/cladex-web/internal/notify"
	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

// Notificaciones is the admin-only /ajustes/notificaciones page: every notification the
// app has sent or tried to, their history, and a test send (internal/notify).
type Notificaciones struct {
	store  *store.Store
	runner *jobs.Runner
	status notify.Status
}

func NewNotificaciones(s *store.Store, runner *jobs.Runner, status notify.Status) *Notificaciones {
	return &Notificaciones{store: s, runner: runner, status: status}
}

func (n *Notificaciones) List(w http.ResponseWriter, r *http.Request) {
	n.renderList(w, r, "")
}

func (n *Notificaciones) renderList(w http.ResponseWriter, r *http.Request, errorMsg string) {
	ctx := r.Context()
	q := r.URL.Query()
	f := store.NotificationFilter{Status: q.Get("estado"), Channel: q.Get("canal"), Kind: q.Get("tipo")}
	all, err := n.store.ListNotifications(ctx, f)
	if err != nil {
		serverError(w, r, err)
		return
	}
	kinds, err := n.store.NotificationKinds(ctx)
	if err != nil {
		serverError(w, r, err)
		return
	}
	rows, pager := paginate(r, all)
	keep := url.Values{}
	for k, v := range map[string]string{"estado": f.Status, "canal": f.Channel, "tipo": f.Kind} {
		if v != "" {
			keep.Set(k, v)
		}
	}
	lv := views.ListView{Base: "/ajustes/notificaciones", Pager: pager, Keep: keep}

	msg := ""
	switch {
	case q.Get("prueba") == "1":
		msg = "Prueba en la cola. Recarga la página en unos segundos para ver si se envió."
	case q.Get("reintentada") == "1":
		msg = "La notificación se volvió a poner en la cola."
	case q.Get("cancelada") == "1":
		msg = "Notificación cancelada."
	}
	if errorMsg != "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	user, _ := UserFromContext(ctx)
	views.Notificaciones(rows, f, kinds, lv, n.status, msg, errorMsg, navUserView(user)).Render(ctx, w)
}

func notificationID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id
}

// Page handles GET /ajustes/notificaciones/{id}: one notification, its content and history.
func (n *Notificaciones) Page(w http.ResponseWriter, r *http.Request) {
	n.renderPage(w, r, "")
}

func (n *Notificaciones) renderPage(w http.ResponseWriter, r *http.Request, errorMsg string) {
	ctx := r.Context()
	notif, err := n.store.NotificationByID(ctx, notificationID(r))
	if err != nil {
		serverError(w, r, err)
		return
	}
	if notif == nil {
		http.NotFound(w, r)
		return
	}
	msg := ""
	switch {
	case r.URL.Query().Get("reintentada") == "1":
		msg = "La notificación se volvió a poner en la cola."
	case r.URL.Query().Get("cancelada") == "1":
		msg = "Notificación cancelada."
	}
	if errorMsg != "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	user, _ := UserFromContext(ctx)
	views.Notificacion(*notif, msg, errorMsg, navUserView(user)).Render(ctx, w)
}

// Reintentar handles POST /ajustes/notificaciones/{id}/reintentar: a fallida
// notification goes back in the queue with a new job.
func (n *Notificaciones) Reintentar(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	id := notificationID(r)
	if err := n.store.RetryNotification(r.Context(), id, user.ID, time.Now()); err != nil {
		if errors.Is(err, store.ErrNotificationState) {
			n.renderPage(w, r, "Solo se puede reintentar una notificación fallida.")
			return
		}
		serverError(w, r, err)
		return
	}
	n.runner.Wake()
	http.Redirect(w, r, "/ajustes/notificaciones/"+strconv.FormatInt(id, 10)+"?reintentada=1", http.StatusSeeOther)
}

// Cancelar handles POST /ajustes/notificaciones/{id}/cancelar, for one still por enviar.
func (n *Notificaciones) Cancelar(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	id := notificationID(r)
	if err := n.store.CancelNotification(r.Context(), id, user.ID, "Cancelada por "+user.Name+".", time.Now()); err != nil {
		if errors.Is(err, store.ErrNotificationState) {
			n.renderPage(w, r, "Solo se puede cancelar una notificación por enviar.")
			return
		}
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/ajustes/notificaciones/"+strconv.FormatInt(id, 10)+"?cancelada=1", http.StatusSeeOther)
}

// Prueba handles POST /ajustes/notificaciones/prueba: a test message to the admin
// themselves, on each channel that is on and that they have an address for.
func (n *Notificaciones) Prueba(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, _ := UserFromContext(ctx)
	contact, err := n.store.UserContactByID(ctx, user.ID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	if !n.status.Channel(store.ChannelEmail).Enabled {
		n.renderList(w, r, "El correo está apagado: "+n.status.Channel(store.ChannelEmail).Problem+".")
		return
	}
	if contact.Email == "" {
		n.renderList(w, r, "Escribe tu correo en Mi cuenta → Avisos para enviarte una prueba.")
		return
	}
	subject, text, html := notify.Prueba(user.Name)
	if _, err := n.store.QueueNotification(ctx, store.NewNotification{
		Kind: notify.PruebaKind, Channel: store.ChannelEmail, UserID: user.ID, Address: contact.Email,
		Subject: subject, BodyText: text, BodyHTML: html, CreatedBy: user.ID,
	}, time.Now()); err != nil {
		serverError(w, r, err)
		return
	}
	n.runner.Wake()
	http.Redirect(w, r, "/ajustes/notificaciones?prueba=1", http.StatusSeeOther)
}
