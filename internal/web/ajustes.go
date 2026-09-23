package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

// Settings holds the dependencies for the admin-only /ajustes handlers: the margin
// options and the materials catalog (docs/PLAN.md, M3). The generic settings table
// holds no pricing input any more; it's kept for future non-pricing settings.
type Settings struct {
	store *store.Store
}

func NewSettings(s *store.Store) *Settings {
	return &Settings{store: s}
}

func (s *Settings) Page(w http.ResponseWriter, r *http.Request) {
	successMsg := ""
	switch {
	case r.URL.Query().Get("margen") == "1":
		successMsg = "Margen guardado."
	case r.URL.Query().Get("material") == "1":
		successMsg = "Material guardado."
	}
	s.render(w, r, successMsg, "")
}

// render draws /ajustes. errorMsg is a refused margin or material action's message,
// rendered with 422.
func (s *Settings) render(w http.ResponseWriter, r *http.Request, successMsg, errorMsg string) {
	ctx := r.Context()
	margins, err := s.store.ListMarginOptions(ctx)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	materials, err := s.store.ListMaterials(ctx)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	units, err := s.store.ListUnits(ctx)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	if errorMsg != "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	authUser, _ := UserFromContext(ctx)
	views.Ajustes(margins, materials, units, successMsg, errorMsg, navUserView(authUser)).Render(ctx, w)
}

// parseMarginPercent reads a margin as the percentage an admin types ("12.34", "12.34%")
// into its stored fraction in micros (123_400). Four decimals is the most a micros
// fraction can hold exactly; margins of 100% or more would divide by zero or go
// negative in cost / (1 - margin), so they're rejected too.
func parseMarginPercent(raw string) (money.Micros, error) {
	pct, err := money.ParseMicros(strings.TrimSuffix(strings.TrimSpace(raw), "%"))
	if err != nil {
		return 0, err
	}
	if pct < 0 || pct >= 100_000_000 || pct%100 != 0 {
		return 0, errors.New("margin out of range")
	}
	return pct / 100, nil
}

const invalidMarginMsg = "Margen inválido: escribe un porcentaje de 0 a 99.9999, p. ej. 12.34."

// parseMarginForm reads the name and percentage shared by the create and edit forms.
func parseMarginForm(r *http.Request) (name string, value money.Micros, errorMsg string) {
	name = strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		return "", 0, "El nombre del margen es obligatorio."
	}
	value, err := parseMarginPercent(r.FormValue("value"))
	if err != nil {
		return "", 0, invalidMarginMsg
	}
	return name, value, ""
}

// marginActionError maps a store error from a margin action to the message shown on
// /ajustes, or "" for an unexpected error (a 500).
func marginActionError(err error) string {
	switch {
	case errors.Is(err, store.ErrDuplicateMarginName):
		return "Ya existe un margen con ese nombre."
	case errors.Is(err, store.ErrMarginOptionIsDefault):
		return "No puedes retirar el margen predeterminado; primero haz predeterminado otro."
	case errors.Is(err, store.ErrMarginOptionRetired):
		return "Un margen retirado no puede ser el predeterminado; primero restáuralo."
	case errors.Is(err, store.ErrMarginOptionNotFound):
		return "Ese margen no existe."
	}
	return ""
}

// marginAction runs one margin change and redirects back to /ajustes, or re-renders it
// with the reason the change was refused.
func (s *Settings) marginAction(w http.ResponseWriter, r *http.Request, act func(adminID int64) error) {
	authUser, _ := UserFromContext(r.Context())
	if err := act(authUser.ID); err != nil {
		if msg := marginActionError(err); msg != "" {
			s.render(w, r, "", msg)
			return
		}
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/ajustes?margen=1", http.StatusSeeOther)
}

// marginID reads the {id} path value; a non-numeric id is a margin that doesn't exist.
func marginID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id
}

// CreateMargen handles POST /ajustes/margenes: adds a new, non-default option.
func (s *Settings) CreateMargen(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "solicitud inválida", http.StatusBadRequest)
		return
	}
	name, value, msg := parseMarginForm(r)
	if msg != "" {
		s.render(w, r, "", msg)
		return
	}
	s.marginAction(w, r, func(adminID int64) error {
		_, err := s.store.CreateMarginOption(r.Context(), name, value, adminID)
		return err
	})
}

// UpdateMargen handles POST /ajustes/margenes/{id}: renames or revalues an option.
// Drafts on it reprice with the new value; issued quotes keep theirs.
func (s *Settings) UpdateMargen(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "solicitud inválida", http.StatusBadRequest)
		return
	}
	name, value, msg := parseMarginForm(r)
	if msg != "" {
		s.render(w, r, "", msg)
		return
	}
	s.marginAction(w, r, func(adminID int64) error {
		return s.store.UpdateMarginOption(r.Context(), marginID(r), name, value, adminID)
	})
}

// PredeterminarMargen handles POST /ajustes/margenes/{id}/predeterminado.
func (s *Settings) PredeterminarMargen(w http.ResponseWriter, r *http.Request) {
	s.marginAction(w, r, func(adminID int64) error {
		return s.store.SetDefaultMarginOption(r.Context(), marginID(r), adminID)
	})
}

// RetirarMargen handles POST /ajustes/margenes/{id}/retirar.
func (s *Settings) RetirarMargen(w http.ResponseWriter, r *http.Request) {
	s.marginAction(w, r, func(adminID int64) error {
		return s.store.RetireMarginOption(r.Context(), marginID(r), adminID)
	})
}

// RestaurarMargen handles POST /ajustes/margenes/{id}/restaurar.
func (s *Settings) RestaurarMargen(w http.ResponseWriter, r *http.Request) {
	s.marginAction(w, r, func(adminID int64) error {
		return s.store.RestoreMarginOption(r.Context(), marginID(r), adminID)
	})
}

// parseMaterialForm reads the name and price shared by the material create and edit
// forms. The price is pure cost per the material's unit, e.g. 155 for $155/kg.
func parseMaterialForm(r *http.Request) (name string, price money.Micros, errorMsg string) {
	name = strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		return "", 0, "El nombre del material es obligatorio."
	}
	price, err := money.ParseMicros(strings.TrimSpace(r.FormValue("price")))
	if err != nil || price < 0 {
		return "", 0, "Precio de material inválido; usa un número, p. ej. 160.00."
	}
	return name, price, ""
}

// materialAction runs one material change and redirects back to /ajustes, or
// re-renders it with the reason the change was refused.
func (s *Settings) materialAction(w http.ResponseWriter, r *http.Request, act func(adminID int64) error) {
	authUser, _ := UserFromContext(r.Context())
	if err := act(authUser.ID); err != nil {
		switch {
		case errors.Is(err, store.ErrDuplicateMaterialName):
			s.render(w, r, "", "Ya existe un material con ese nombre.")
		case errors.Is(err, store.ErrMaterialNotFound):
			s.render(w, r, "", "Ese material no existe.")
		default:
			http.Error(w, "error interno", http.StatusInternalServerError)
		}
		return
	}
	http.Redirect(w, r, "/ajustes?material=1", http.StatusSeeOther)
}

// CreateMaterial handles POST /ajustes/materiales.
func (s *Settings) CreateMaterial(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "solicitud inválida", http.StatusBadRequest)
		return
	}
	name, price, msg := parseMaterialForm(r)
	if msg != "" {
		s.render(w, r, "", msg)
		return
	}
	unitID, err := strconv.ParseInt(r.FormValue("unit_id"), 10, 64)
	if err != nil {
		s.render(w, r, "", "Selecciona la unidad del material.")
		return
	}
	s.materialAction(w, r, func(adminID int64) error {
		_, err := s.store.CreateMaterial(r.Context(), name, unitID, price, adminID)
		return err
	})
}

// UpdateMaterial handles POST /ajustes/materiales/{id}: renames or reprices a material.
// Drafts quoting products made of it reprice; issued quotes keep theirs.
func (s *Settings) UpdateMaterial(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "solicitud inválida", http.StatusBadRequest)
		return
	}
	name, price, msg := parseMaterialForm(r)
	if msg != "" {
		s.render(w, r, "", msg)
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	s.materialAction(w, r, func(adminID int64) error {
		return s.store.UpdateMaterial(r.Context(), id, name, price, adminID)
	})
}
