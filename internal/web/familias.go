package web

import (
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gerrygoo/cladex-web/internal/satcat"
	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

// Familias holds the dependencies for the admin-only /familias handlers: the familias
// and quote types page (issue #56), which lists each product familia (catalog category)
// with its quote types under it; creating a familia; and renaming one or setting its SAT
// code. Quote types are created and edited by Tipos, which comes back here. Familias are
// never deleted here: products reference them.
type Familias struct {
	store *store.Store
}

// familiasPath is the familias and quote types page.
const familiasPath = "/familias"

func NewFamilias(s *store.Store) *Familias {
	return &Familias{store: s}
}

// satProductPattern is a SAT c_ClaveProdServ: eight digits.
var satProductPattern = regexp.MustCompile(`^[0-9]{8}$`)

// satUnitPattern is a SAT c_ClaveUnidad: two or three capitals or digits.
var satUnitPattern = regexp.MustCompile(`^[A-Z0-9]{2,3}$`)

func (f *Familias) List(w http.ResponseWriter, r *http.Request) {
	successMsg := ""
	switch r.URL.Query().Get("guardado") {
	case "creada":
		successMsg = "Familia creada."
	case "editada":
		successMsg = "Familia guardada."
	case "tipo-creado":
		successMsg = "Tipo de cotización creado."
	case "tipo-editado":
		successMsg = "Tipo de cotización guardado."
	}
	f.renderList(w, r, "", nil, successMsg, http.StatusOK)
}

func (f *Familias) renderList(w http.ResponseWriter, r *http.Request, name string, errs map[string]string, successMsg string, status int) {
	ctx := r.Context()
	families, err := f.store.ListAllFamilies(ctx)
	if err != nil {
		serverError(w, r, err)
		return
	}
	types, err := f.store.ListQuoteTypes(ctx)
	if err != nil {
		serverError(w, r, err)
		return
	}
	groups, orphans := views.GroupTiposByFamilia(families, types)
	user, _ := UserFromContext(ctx)
	w.WriteHeader(status)
	views.FamiliasList(groups, orphans, name, errs, successMsg, navUserView(user)).Render(ctx, w)
}

func (f *Familias) Create(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	errs := map[string]string{}
	if name == "" {
		errs["name"] = "El nombre es obligatorio."
	} else {
		_, err := f.store.CreateFamily(r.Context(), name)
		switch {
		case err == nil:
			http.Redirect(w, r, familiasPath+"?guardado=creada", http.StatusSeeOther)
			return
		case errors.Is(err, store.ErrDuplicateFamilyName):
			errs["name"] = "Ya existe una familia con este nombre."
		default:
			serverError(w, r, err)
			return
		}
	}
	f.renderList(w, r, name, errs, "", http.StatusUnprocessableEntity)
}

// loadFamilyOrNotFound finds the familia named by the {id} path value, writing a 404 and
// returning false if there isn't one.
func (f *Familias) loadFamilyOrNotFound(w http.ResponseWriter, r *http.Request) (store.Family, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err == nil {
		families, lerr := f.store.ListAllFamilies(r.Context())
		if lerr != nil {
			serverError(w, r, lerr)
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
	views.FamiliaEdit(fam.Name, fam, nil, navUserView(user)).Render(r.Context(), w)
}

func (f *Familias) Update(w http.ResponseWriter, r *http.Request) {
	fam, ok := f.loadFamilyOrNotFound(w, r)
	if !ok {
		return
	}
	savedName := fam.Name
	name := strings.TrimSpace(r.FormValue("name"))
	satCode := strings.TrimSpace(r.FormValue("sat_product_code"))
	errs := map[string]string{}
	if name == "" {
		errs["name"] = "El nombre es obligatorio."
	}
	if satCode != "" && !satProductPattern.MatchString(satCode) {
		errs["sat_product_code"] = "La clave SAT son 8 dígitos, p. ej. 26121600."
	} else if _, ok := satcat.ProdServ(satCode); satCode != "" && !ok {
		errs["sat_product_code"] = "La clave SAT " + satCode + " no existe en el catálogo del SAT."
	}
	if len(errs) == 0 {
		err := f.store.UpdateFamily(r.Context(), fam.ID, name, satCode)
		switch {
		case err == nil:
			http.Redirect(w, r, familiasPath+"?guardado=editada", http.StatusSeeOther)
			return
		case errors.Is(err, store.ErrDuplicateFamilyName):
			errs["name"] = "Ya existe una familia con este nombre."
		case errors.Is(err, sql.ErrNoRows):
			http.NotFound(w, r)
			return
		default:
			serverError(w, r, err)
			return
		}
	}
	user, _ := UserFromContext(r.Context())
	w.WriteHeader(http.StatusUnprocessableEntity)
	fam.Name, fam.SATProductCode = name, satCode
	views.FamiliaEdit(savedName, fam, errs, navUserView(user)).Render(r.Context(), w)
}
