package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

// Units holds the dependencies for the admin-only /unidades handlers — a small
// list+create page for the shared units of measure that products and their
// conversion rates reference (see migrations/0003_add_units_and_conversions.sql).
// Units are never deleted from here: products and conversion rows can reference them,
// so removing one would need the same kind of soft-delete/reference-check machinery
// products and customers already have, which isn't worth it for a handful of rarely-
// changed rows.
type Units struct {
	store *store.Store
}

func NewUnits(s *store.Store) *Units {
	return &Units{store: s}
}

func (u *Units) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	units, err := u.store.ListUnits(ctx)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	successMsg := ""
	if r.URL.Query().Get("guardado") == "1" {
		successMsg = "Unidad guardada."
	}
	user, _ := UserFromContext(ctx)
	views.UnitsList(units, successMsg, "", navUserView(user)).Render(ctx, w)
}

func (u *Units) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(w, "solicitud inválida", http.StatusBadRequest)
		return
	}
	code := strings.TrimSpace(r.FormValue("code"))
	name := strings.TrimSpace(r.FormValue("name"))
	if code == "" || name == "" {
		u.renderListError(w, r, "El código y el nombre son obligatorios.")
		return
	}

	if _, err := u.store.CreateUnit(ctx, code, name); err != nil {
		if errors.Is(err, store.ErrDuplicateUnitCode) {
			u.renderListError(w, r, "Ya existe una unidad con este código.")
			return
		}
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/unidades?guardado=1", http.StatusSeeOther)
}

func (u *Units) renderListError(w http.ResponseWriter, r *http.Request, errorMsg string) {
	ctx := r.Context()
	units, err := u.store.ListUnits(ctx)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	user, _ := UserFromContext(ctx)
	w.WriteHeader(http.StatusUnprocessableEntity)
	views.UnitsList(units, "", errorMsg, navUserView(user)).Render(ctx, w)
}
