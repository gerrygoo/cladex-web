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

// settingDefs is the fixed set of admin-editable single-value knobs (FX rate, metal
// prices). Margins aren't here: they're a list of named options, managed by the
// Margenes handlers below (docs/PLAN.md, M3). settings is a generic key/value table specifically
// so a new knob is a one-line addition here, no migration — see
// migrations/0001_init.sql. Each value is a plain decimal string in the form (e.g.
// "18.50", "0.35" for a 35% margin), stored as fixed-point micros text via
// internal/money, same representation as product prices.
var settingDefs = []struct {
	Key         string
	Label       string
	Placeholder string
}{
	{"fx_rate", "Tipo de cambio (USD/MXN)", "p. ej. 18.50"},
	{"copper_price", "Precio del cobre ($/kg)", "p. ej. 145.30"},
}

// Settings holds the dependencies for the admin-only /ajustes handlers.
type Settings struct {
	store *store.Store
}

func NewSettings(s *store.Store) *Settings {
	return &Settings{store: s}
}

func (s *Settings) Page(w http.ResponseWriter, r *http.Request) {
	successMsg := ""
	switch {
	case r.URL.Query().Get("guardado") == "1":
		successMsg = "Ajustes guardados."
	case r.URL.Query().Get("margen") == "1":
		successMsg = "Margen guardado."
	}
	s.render(w, r, nil, successMsg, "")
}

// render draws /ajustes. fields is nil to show the stored values, or the submitted
// ones (with their errors) after a failed Submit; errorMsg is a failed margin action's
// message. Any error renders with 422.
func (s *Settings) render(w http.ResponseWriter, r *http.Request, fields []views.SettingFieldView, successMsg, errorMsg string) {
	ctx := r.Context()
	if fields == nil {
		var err error
		if fields, err = s.storedFields(r); err != nil {
			http.Error(w, "error interno", http.StatusInternalServerError)
			return
		}
	}
	margins, err := s.store.ListMarginOptions(ctx)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	hasFieldErrors := false
	for _, f := range fields {
		hasFieldErrors = hasFieldErrors || f.Error != ""
	}
	if errorMsg != "" || hasFieldErrors {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	authUser, _ := UserFromContext(ctx)
	views.Ajustes(fields, margins, successMsg, errorMsg, navUserView(authUser)).Render(ctx, w)
}

func (s *Settings) storedFields(r *http.Request) ([]views.SettingFieldView, error) {
	ctx := r.Context()
	keys := make([]string, len(settingDefs))
	for i, d := range settingDefs {
		keys[i] = d.Key
	}
	stored, err := s.store.SettingValues(ctx, keys)
	if err != nil {
		return nil, err
	}

	fields := make([]views.SettingFieldView, len(settingDefs))
	for i, d := range settingDefs {
		value := ""
		if raw, ok := stored[d.Key]; ok {
			if micros, err := strconv.ParseInt(raw, 10, 64); err == nil {
				value = money.Micros(micros).String()
			}
		}
		fields[i] = views.SettingFieldView{Key: d.Key, Label: d.Label, Placeholder: d.Placeholder, Value: value}
	}
	return fields, nil
}

func (s *Settings) Submit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(w, "solicitud inválida", http.StatusBadRequest)
		return
	}

	fields := make([]views.SettingFieldView, len(settingDefs))
	parsed := make(map[string]money.Micros, len(settingDefs))
	hasErrors := false
	for i, d := range settingDefs {
		raw := r.FormValue(d.Key)
		fields[i] = views.SettingFieldView{Key: d.Key, Label: d.Label, Placeholder: d.Placeholder, Value: raw}
		m, err := money.ParseMicros(raw)
		if err != nil {
			fields[i].Error = "Valor inválido; usa un número, p. ej. 18.50."
			hasErrors = true
			continue
		}
		parsed[d.Key] = m
	}

	if hasErrors {
		s.render(w, r, fields, "", "")
		return
	}

	authUser, _ := UserFromContext(ctx)
	for key, m := range parsed {
		if err := s.store.SetSetting(ctx, key, strconv.FormatInt(int64(m), 10), authUser.ID); err != nil {
			http.Error(w, "error interno", http.StatusInternalServerError)
			return
		}
	}
	http.Redirect(w, r, "/ajustes?guardado=1", http.StatusSeeOther)
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
			s.render(w, r, nil, "", msg)
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
		s.render(w, r, nil, "", msg)
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
		s.render(w, r, nil, "", msg)
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
