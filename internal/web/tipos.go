package web

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

// Tipos holds the admin-only /tipos-de-cotizacion handlers: list, create and edit the
// quote types (docs/planning/tipos-de-cotizacion.md). A type's prefix can change at any
// time; folios already handed out keep the prefix they were printed with. Types are
// never deleted: quotes reference them.
type Tipos struct {
	store *store.Store
}

func NewTipos(s *store.Store) *Tipos {
	return &Tipos{store: s}
}

const tiposPath = "/tipos-de-cotizacion"

func (t *Tipos) List(w http.ResponseWriter, r *http.Request) {
	successMsg := ""
	switch r.URL.Query().Get("guardado") {
	case "creado":
		successMsg = "Tipo de cotización creado."
	case "editado":
		successMsg = "Tipo de cotización guardado."
	}
	t.renderList(w, r, store.QuoteType{}, nil, successMsg, http.StatusOK)
}

func (t *Tipos) renderList(w http.ResponseWriter, r *http.Request, values store.QuoteType, errs map[string]string, successMsg string, status int) {
	ctx := r.Context()
	types, err := t.store.ListQuoteTypes(ctx)
	if err != nil {
		serverError(w, r, err)
		return
	}
	families, err := t.store.ListAllFamilies(ctx)
	if err != nil {
		serverError(w, r, err)
		return
	}
	user, _ := UserFromContext(ctx)
	w.WriteHeader(status)
	views.TiposList(types, families, values, errs, successMsg, navUserView(user)).Render(ctx, w)
}

// parseTipoForm reads the type form onto a QuoteType. The prefix is upper-cased and the
// terms tidied one per line; anything else is checked by the store.
func parseTipoForm(r *http.Request) store.QuoteType {
	qt := store.QuoteType{
		Prefix: strings.ToUpper(strings.TrimSpace(r.FormValue("prefix"))),
		Name:   strings.TrimSpace(r.FormValue("name")),
		Terms:  strings.Join(store.SplitTerms(r.FormValue("terms")), "\n"),
		QuoteTypeBehavior: store.QuoteTypeBehavior{
			FreeLinesOnly:     r.FormValue("free_lines_only") != "",
			HideAddress:       r.FormValue("hide_address") != "",
			SortLast:          r.FormValue("sort_last") != "",
			KgPriced:          r.FormValue("kg_priced") != "",
			CopperPriceMargin: r.FormValue("copper_price_margin") != "",
			Letterhead:        r.FormValue("letterhead"),
		},
	}
	if id, err := strconv.ParseInt(r.FormValue("family_id"), 10, 64); err == nil && id > 0 {
		qt.FamilyID = &id
	}
	return qt
}

// tipoErrors turns what the store refused into the form's field errors, or returns
// false for an error that isn't the form's fault.
func tipoErrors(err error) (map[string]string, bool) {
	switch {
	case errors.Is(err, store.ErrBadPrefix):
		return map[string]string{"prefix": "La serie es una Q seguida de 1 a 5 letras, p. ej. QCCA."}, true
	case errors.Is(err, store.ErrDuplicateSeries):
		return map[string]string{"prefix": "Esa serie ya la tiene o la usó otro tipo de cotización; elige otra."}, true
	case errors.Is(err, store.ErrBadLetterhead):
		return map[string]string{"letterhead": "Elige un membrete de la lista."}, true
	}
	return nil, false
}

func (t *Tipos) Create(w http.ResponseWriter, r *http.Request) {
	values := parseTipoForm(r)
	_, err := t.store.CreateQuoteType(r.Context(), values)
	if err == nil {
		http.Redirect(w, r, tiposPath+"?guardado=creado", http.StatusSeeOther)
		return
	}
	errs, ok := tipoErrors(err)
	if !ok {
		serverError(w, r, err)
		return
	}
	t.renderList(w, r, values, errs, "", http.StatusUnprocessableEntity)
}

// loadTipoOrNotFound finds the type named by the {id} path value, writing a 404 and
// returning nil if there isn't one.
func (t *Tipos) loadTipoOrNotFound(w http.ResponseWriter, r *http.Request) *store.QuoteType {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return nil
	}
	qt, err := t.store.QuoteTypeByID(r.Context(), id)
	if err != nil {
		serverError(w, r, err)
		return nil
	}
	if qt == nil {
		http.NotFound(w, r)
	}
	return qt
}

func (t *Tipos) EditPage(w http.ResponseWriter, r *http.Request) {
	qt := t.loadTipoOrNotFound(w, r)
	if qt == nil {
		return
	}
	t.renderEdit(w, r, *qt, *qt, nil, http.StatusOK)
}

func (t *Tipos) renderEdit(w http.ResponseWriter, r *http.Request, saved, values store.QuoteType, errs map[string]string, status int) {
	ctx := r.Context()
	families, err := t.store.ListAllFamilies(ctx)
	if err != nil {
		serverError(w, r, err)
		return
	}
	user, _ := UserFromContext(ctx)
	w.WriteHeader(status)
	views.TipoEdit(saved, values, families, errs, navUserView(user)).Render(ctx, w)
}

func (t *Tipos) Update(w http.ResponseWriter, r *http.Request) {
	saved := t.loadTipoOrNotFound(w, r)
	if saved == nil {
		return
	}
	values := parseTipoForm(r)
	values.ID = saved.ID
	values.FreeLinesOnly = saved.FreeLinesOnly
	err := t.store.UpdateQuoteType(r.Context(), values)
	switch {
	case err == nil:
		http.Redirect(w, r, tiposPath+"?guardado=editado", http.StatusSeeOther)
		return
	case errors.Is(err, sql.ErrNoRows):
		http.NotFound(w, r)
		return
	}
	errs, ok := tipoErrors(err)
	if !ok {
		serverError(w, r, err)
		return
	}
	t.renderEdit(w, r, *saved, values, errs, http.StatusUnprocessableEntity)
}
