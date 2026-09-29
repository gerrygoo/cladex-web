package web

import (
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

// Familias holds the dependencies for the admin-only /familias handlers: list and
// create the product familias, each of which also starts a quote series (its folio
// prefix and its "Términos y condiciones" block), and edit a familia's terms. Familias
// are never deleted or renamed here: products and quotes reference them, and a series
// prefix is printed in every folio it has issued.
type Familias struct {
	store *store.Store
}

func NewFamilias(s *store.Store) *Familias {
	return &Familias{store: s}
}

// seriesPattern is what a series prefix looks like: Q and one more letter, like QA.
var seriesPattern = regexp.MustCompile(`^Q[A-Z]$`)

func (f *Familias) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	families, err := f.store.ListAllFamilies(ctx)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	successMsg := ""
	switch r.URL.Query().Get("guardado") {
	case "creada":
		successMsg = "Familia creada."
	case "editada":
		successMsg = "Familia guardada."
	}
	user, _ := UserFromContext(ctx)
	views.FamiliasList(families, views.FamiliaFormValues{}, nil, successMsg, navUserView(user)).Render(ctx, w)
}

func (f *Familias) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(w, "solicitud inválida", http.StatusBadRequest)
		return
	}
	values := views.FamiliaFormValues{
		Name:          strings.TrimSpace(r.FormValue("name")),
		Series:        strings.ToUpper(strings.TrimSpace(r.FormValue("series"))),
		SeriesLabel:   strings.TrimSpace(r.FormValue("series_label")),
		Terms:         r.FormValue("terms"),
		FreeLinesOnly: r.FormValue("free_lines_only") != "",
	}
	errs := map[string]string{}
	if values.Name == "" {
		errs["name"] = "El nombre es obligatorio."
	}
	if !seriesPattern.MatchString(values.Series) {
		errs["series"] = "La serie son dos letras y empieza con Q, p. ej. QF."
	}
	if len(errs) == 0 {
		_, err := f.store.CreateFamily(ctx, store.Family{
			Name:          values.Name,
			Series:        values.Series,
			SeriesLabel:   values.SeriesLabel,
			Terms:         strings.Join(store.SplitTerms(values.Terms), "\n"),
			FreeLinesOnly: values.FreeLinesOnly,
		})
		switch {
		case err == nil:
			http.Redirect(w, r, "/familias?guardado=creada", http.StatusSeeOther)
			return
		case errors.Is(err, store.ErrDuplicateFamilyName):
			errs["name"] = "Ya existe una familia con este nombre."
		case errors.Is(err, store.ErrDuplicateSeries):
			errs["series"] = "Ya existe una familia con esta serie."
		default:
			http.Error(w, "error interno", http.StatusInternalServerError)
			return
		}
	}

	families, err := f.store.ListAllFamilies(ctx)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	user, _ := UserFromContext(ctx)
	w.WriteHeader(http.StatusUnprocessableEntity)
	views.FamiliasList(families, values, errs, "", navUserView(user)).Render(ctx, w)
}

// loadFamilyOrNotFound finds the familia named by the {id} path value, writing a 404 and
// returning false if there isn't one.
func (f *Familias) loadFamilyOrNotFound(w http.ResponseWriter, r *http.Request) (store.Family, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err == nil {
		families, lerr := f.store.ListAllFamilies(r.Context())
		if lerr != nil {
			http.Error(w, "error interno", http.StatusInternalServerError)
			return store.Family{}, false
		}
		for _, fam := range families {
			if fam.ID == id {
				return fam, true
			}
		}
	}
	http.NotFound(w, r)
	return store.Family{}, false
}

func (f *Familias) EditPage(w http.ResponseWriter, r *http.Request) {
	fam, ok := f.loadFamilyOrNotFound(w, r)
	if !ok {
		return
	}
	user, _ := UserFromContext(r.Context())
	views.FamiliaEdit(fam, fam.SeriesLabel, fam.Terms, navUserView(user)).Render(r.Context(), w)
}

func (f *Familias) Update(w http.ResponseWriter, r *http.Request) {
	fam, ok := f.loadFamilyOrNotFound(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "solicitud inválida", http.StatusBadRequest)
		return
	}
	label := strings.TrimSpace(r.FormValue("series_label"))
	terms := strings.Join(store.SplitTerms(r.FormValue("terms")), "\n")
	if err := f.store.UpdateFamilyTerms(r.Context(), fam.ID, label, terms); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/familias?guardado=editada", http.StatusSeeOther)
}
