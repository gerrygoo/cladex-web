package web

import (
	"net/http"
	"strconv"

	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

// settingDefs is the fixed set of admin-editable knobs the plan calls out (FX rate,
// metal prices, default margins). settings is a generic key/value table specifically
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
	{"default_margin", "Margen por defecto (fracción, ej. 0.35 = 35%)", "p. ej. 0.35"},
}

// Settings holds the dependencies for the admin-only /ajustes handlers.
type Settings struct {
	store *store.Store
}

func NewSettings(s *store.Store) *Settings {
	return &Settings{store: s}
}

func (s *Settings) Page(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	keys := make([]string, len(settingDefs))
	for i, d := range settingDefs {
		keys[i] = d.Key
	}
	stored, err := s.store.SettingValues(ctx, keys)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
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

	successMsg := ""
	if r.URL.Query().Get("guardado") == "1" {
		successMsg = "Ajustes guardados."
	}
	authUser, _ := UserFromContext(ctx)
	views.Ajustes(fields, successMsg, navUserView(authUser)).Render(ctx, w)
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
		authUser, _ := UserFromContext(ctx)
		w.WriteHeader(http.StatusUnprocessableEntity)
		views.Ajustes(fields, "", navUserView(authUser)).Render(ctx, w)
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
