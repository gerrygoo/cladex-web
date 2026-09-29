package web

import (
	"net/http"
	"strconv"

	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

// Users holds the dependencies for the admin-only /usuarios handlers. Creating users
// and resetting passwords stay CLI-only (cladexctl, 1.4) — the plan's Auth design is
// explicit that provisioning is out-of-band with no self-service surface, so this page
// only manages role and enabled/disabled state for existing accounts.
type Users struct {
	store *store.Store
}

func NewUsers(s *store.Store) *Users {
	return &Users{store: s}
}

func (u *Users) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sort, dir := sortParams(r)
	filters := filterParams(r)
	users, err := u.store.ListUsers(ctx, sort, dir, filters)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	users, pager := paginate(r, users)
	lv := views.ListView{Base: "/usuarios", Sort: sort, Dir: dir, Filters: filters, Pager: pager}
	authUser, _ := UserFromContext(ctx)
	successMsg := ""
	if r.URL.Query().Get("guardado") == "1" {
		successMsg = "Cambios guardados."
	}
	views.UsersList(users, authUser.ID, lv, successMsg, "", navUserView(authUser)).Render(ctx, w)
}

// SetRole handles POST /usuarios/{id}/rol. An admin can't change their own role
// here — the UI already hides the control for their own row, but the handler
// re-checks it so a crafted request can't be used to self-lock-out of admin.
func (u *Users) SetRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	authUser, _ := UserFromContext(ctx)
	if id == authUser.ID {
		u.renderListError(w, r, "No puedes cambiar tu propio rol.")
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "solicitud inválida", http.StatusBadRequest)
		return
	}
	role := r.FormValue("role")
	if role != "admin" && role != "vendedor" {
		u.renderListError(w, r, "Rol inválido.")
		return
	}
	if err := u.store.SetUserRole(ctx, id, role); err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/usuarios?guardado=1", http.StatusSeeOther)
}

// SetDisabled handles POST /usuarios/{id}/deshabilitar and .../habilitar. An admin
// can't disable their own account — same self-lockout guard as SetRole.
func (u *Users) SetDisabled(disabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		authUser, _ := UserFromContext(ctx)
		if id == authUser.ID {
			u.renderListError(w, r, "No puedes deshabilitar tu propia cuenta.")
			return
		}
		if err := u.store.SetUserDisabled(ctx, id, disabled); err != nil {
			http.Error(w, "error interno", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/usuarios?guardado=1", http.StatusSeeOther)
	}
}

func (u *Users) renderListError(w http.ResponseWriter, r *http.Request, errorMsg string) {
	ctx := r.Context()
	sort, dir := sortParams(r)
	filters := filterParams(r)
	users, err := u.store.ListUsers(ctx, sort, dir, filters)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	users, pager := paginate(r, users)
	lv := views.ListView{Base: "/usuarios", Sort: sort, Dir: dir, Filters: filters, Pager: pager}
	authUser, _ := UserFromContext(ctx)
	w.WriteHeader(http.StatusUnprocessableEntity)
	views.UsersList(users, authUser.ID, lv, "", errorMsg, navUserView(authUser)).Render(ctx, w)
}
