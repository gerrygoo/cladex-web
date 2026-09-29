package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

// Products holds the dependencies for the /productos handlers.
type Products struct {
	store *store.Store
}

func NewProducts(s *store.Store) *Products {
	return &Products{store: s}
}

// ProductForm is the /productos/nuevo and /productos/{id} form, per the plan's "one
// struct per form with Validate()" convention. Fields are raw strings so a validation
// error can redisplay exactly what the user typed.
type ProductForm struct {
	FamilyID    string
	SKU         string
	Description string
	Cost        string
	KgPerM      string
	UnitID      string
}

func parseProductForm(r *http.Request) ProductForm {
	return ProductForm{
		FamilyID:    r.FormValue("family_id"),
		SKU:         strings.TrimSpace(r.FormValue("sku")),
		Description: strings.TrimSpace(r.FormValue("description")),
		Cost:        strings.TrimSpace(r.FormValue("cost")),
		KgPerM:      strings.TrimSpace(r.FormValue("kg_per_m")),
		UnitID:      strings.TrimSpace(r.FormValue("unit_id")),
	}
}

func (f ProductForm) Validate() map[string]string {
	errs := map[string]string{}
	if f.SKU == "" {
		errs["sku"] = "El SKU es obligatorio."
	}
	if f.Description == "" {
		errs["description"] = "La descripción es obligatoria."
	}
	if familyID, err := strconv.ParseInt(f.FamilyID, 10, 64); f.FamilyID == "" || err != nil || familyID <= 0 {
		errs["family_id"] = "Selecciona una familia."
	}
	if f.Cost != "" {
		if _, err := money.ParseMicros(f.Cost); err != nil {
			errs["cost"] = "Costo inválido; usa un número, p. ej. 123.45."
		}
	}
	if f.KgPerM != "" {
		if _, err := money.ParseMicros(f.KgPerM); err != nil {
			errs["kg_per_m"] = "Peso inválido; usa un número, p. ej. 0.123."
		}
	}
	return errs
}

func (f ProductForm) toValues() views.ProductFormValues {
	return views.ProductFormValues{
		FamilyID:    f.FamilyID,
		SKU:         f.SKU,
		Description: f.Description,
		Cost:        f.Cost,
		KgPerM:      f.KgPerM,
		UnitID:      f.UnitID,
	}
}

// toProduct converts a validated form into a store.Product. Only call after
// Validate() returns no errors — parse errors are discarded here since they've
// already been surfaced as field errors.
func (f ProductForm) toProduct() store.Product {
	familyID, _ := strconv.ParseInt(f.FamilyID, 10, 64)
	p := store.Product{
		FamilyID:    familyID,
		SKU:         f.SKU,
		Description: f.Description,
	}
	if f.Cost != "" {
		m, _ := money.ParseMicros(f.Cost)
		p.CostMicros = &m
	}
	if f.KgPerM != "" {
		m, _ := money.ParseMicros(f.KgPerM)
		p.KgPerMMicros = &m
	}
	if f.UnitID != "" {
		uid, _ := strconv.ParseInt(f.UnitID, 10, 64)
		p.UnitID = &uid
	}
	return p
}

func productToValues(p store.Product) views.ProductFormValues {
	v := views.ProductFormValues{
		FamilyID:    strconv.FormatInt(p.FamilyID, 10),
		SKU:         p.SKU,
		Description: p.Description,
	}
	if p.CostMicros != nil {
		v.Cost = p.CostMicros.String()
	}
	if p.KgPerMMicros != nil {
		v.KgPerM = p.KgPerMMicros.String()
	}
	if p.UnitID != nil {
		v.UnitID = strconv.FormatInt(*p.UnitID, 10)
	}
	return v
}

// List renders /productos: the full page normally, or just the table body when
// called via htmx live search (identified by the HX-Request header).
func (p *Products) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	sort, dir := sortParams(r)
	filters := filterParams(r)
	products, err := p.store.ListProducts(ctx, query, sort, dir, filters)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}

	products, pager := paginate(r, products)
	lv := views.ListView{Base: "/productos", Query: query, Sort: sort, Dir: dir, Filters: filters, Pager: pager}

	if r.Header.Get("HX-Request") == "true" {
		views.ProductsTableBody(products).Render(ctx, w)
		views.ListPagerOOB(lv).Render(ctx, w)
		return
	}

	successMsg := ""
	switch {
	case r.URL.Query().Get("guardado") == "1":
		successMsg = "Producto guardado."
	case r.URL.Query().Get("eliminado") == "1":
		successMsg = "Producto eliminado."
	}
	families, err := p.store.ListFamilies(ctx)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	units, err := p.store.ListUnits(ctx)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	var familyOptions, unitOptions []views.FilterOption
	for _, f := range families {
		familyOptions = append(familyOptions, views.FilterOption{Value: f.Name, Label: f.Name})
	}
	for _, u := range units {
		unitOptions = append(unitOptions, views.FilterOption{Value: u.Code, Label: u.Code + " — " + u.Name})
	}
	user, _ := UserFromContext(ctx)
	views.ProductsList(products, lv, familyOptions, unitOptions, successMsg, navUserView(user)).Render(ctx, w)
}

// NewPage renders the empty create form at /productos/nuevo.
func (p *Products) NewPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	families, err := p.store.ListFamilies(ctx)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	units, err := p.store.ListUnits(ctx)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	user, _ := UserFromContext(ctx)
	values := views.ProductFormValues{}
	views.ProductForm("Nuevo producto", "/productos/nuevo", values, nil, "", families, units, 0, views.ProductEditSections{}, navUserView(user)).Render(ctx, w)
}

// Create handles POST /productos/nuevo.
func (p *Products) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(w, "solicitud inválida", http.StatusBadRequest)
		return
	}
	form := parseProductForm(r)
	fieldErrors := form.Validate()

	if fieldErrors["sku"] == "" {
		existing, err := p.store.ProductBySKU(ctx, form.SKU)
		if err != nil {
			http.Error(w, "error interno", http.StatusInternalServerError)
			return
		}
		if existing != nil {
			fieldErrors["sku"] = "Ya existe un producto con este SKU."
		}
	}

	if len(fieldErrors) > 0 {
		p.renderFormError(w, r, "Nuevo producto", "/productos/nuevo", 0, form, fieldErrors)
		return
	}

	id, err := p.store.CreateProduct(ctx, form.toProduct())
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/productos/%d?guardado=1", id), http.StatusSeeOther)
}

// loadEditData loads everything the edit form and its per-product sections (unit
// conversions, materials) need in one place, since both EditPage and the section
// mutation handlers' error paths need the same reads. Returns a nil product, with no
// error, if id doesn't exist.
func (p *Products) loadEditData(ctx context.Context, id int64) (*store.Product, []store.ProductFamily, []store.Unit, views.ProductEditSections, error) {
	var sections views.ProductEditSections
	product, err := p.store.ProductByID(ctx, id)
	if err != nil || product == nil {
		return nil, nil, nil, sections, err
	}
	families, err := p.store.ListFamilies(ctx)
	if err != nil {
		return nil, nil, nil, sections, err
	}
	units, err := p.store.ListUnits(ctx)
	if err != nil {
		return nil, nil, nil, sections, err
	}
	if sections, err = p.loadEditSections(ctx, id); err != nil {
		return nil, nil, nil, sections, err
	}
	return product, families, units, sections, nil
}

// loadEditSections loads a product's unit conversions and materials, plus every
// material for the "add material" picker.
func (p *Products) loadEditSections(ctx context.Context, id int64) (views.ProductEditSections, error) {
	var sections views.ProductEditSections
	var err error
	if sections.Conversions, err = p.store.ListConversionsByProduct(ctx, id); err != nil {
		return sections, err
	}
	if sections.Materials, err = p.store.ListProductMaterials(ctx, id); err != nil {
		return sections, err
	}
	if sections.AllMaterials, err = p.store.ListMaterials(ctx); err != nil {
		return sections, err
	}
	return sections, nil
}

// EditPage renders the edit form at GET /productos/{id}, plus the product's unit
// conversion rates (only meaningful once the product exists, so this section doesn't
// appear on the create form).
func (p *Products) EditPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	product, families, units, sections, err := p.loadEditData(ctx, id)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	if product == nil {
		http.NotFound(w, r)
		return
	}
	successMsg := ""
	if r.URL.Query().Get("guardado") == "1" {
		successMsg = "Producto guardado."
	}
	user, _ := UserFromContext(ctx)
	action := fmt.Sprintf("/productos/%d", id)
	views.ProductForm("Editar producto", action, productToValues(*product), nil, successMsg, families, units, id, sections, navUserView(user)).Render(ctx, w)
}

// renderSectionError re-renders the edit form with a validation error scoped to one of
// its per-product sections (setErr picks which) — the main product fields are
// unaffected, so their values come straight from the stored product, not re-parsed
// form input.
func (p *Products) renderSectionError(w http.ResponseWriter, r *http.Request, id int64, setErr func(*views.ProductEditSections)) {
	ctx := r.Context()
	product, families, units, sections, err := p.loadEditData(ctx, id)
	if err != nil || product == nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	setErr(&sections)
	user, _ := UserFromContext(ctx)
	w.WriteHeader(http.StatusUnprocessableEntity)
	action := fmt.Sprintf("/productos/%d", id)
	views.ProductForm("Editar producto", action, productToValues(*product), nil, "", families, units, id, sections, navUserView(user)).Render(ctx, w)
}

// renderConversionsError re-renders the edit form with an error in the unit-conversions
// section.
func (p *Products) renderConversionsError(w http.ResponseWriter, r *http.Request, id int64, errorMsg string) {
	p.renderSectionError(w, r, id, func(s *views.ProductEditSections) { s.ConversionsError = errorMsg })
}

// renderMaterialsError re-renders the edit form with an error in the materials section.
func (p *Products) renderMaterialsError(w http.ResponseWriter, r *http.Request, id int64, errorMsg string) {
	p.renderSectionError(w, r, id, func(s *views.ProductEditSections) { s.MaterialsError = errorMsg })
}

// AddMaterial handles POST /productos/{id}/materiales: records how much of a material
// one unit of the product contains, e.g. 0.1723 kg of CCS 30% per m.
func (p *Products) AddMaterial(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "solicitud inválida", http.StatusBadRequest)
		return
	}
	materialID, err := strconv.ParseInt(r.FormValue("material_id"), 10, 64)
	if err != nil {
		p.renderMaterialsError(w, r, id, "Selecciona un material.")
		return
	}
	qty, err := money.ParseMicros(strings.TrimSpace(r.FormValue("qty")))
	if err != nil || qty <= 0 {
		p.renderMaterialsError(w, r, id, "Cantidad inválida; usa un número positivo, p. ej. 0.1723.")
		return
	}
	if _, err := p.store.AddProductMaterial(r.Context(), id, materialID, qty); err != nil {
		if errors.Is(err, store.ErrDuplicateProductMaterial) {
			p.renderMaterialsError(w, r, id, "Este producto ya tiene ese material; elimínalo y vuelve a agregarlo para cambiar la cantidad.")
			return
		}
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/productos/%d?guardado=1", id), http.StatusSeeOther)
}

// DeleteMaterial handles POST /productos/{id}/materiales/{mid}/eliminar.
func (p *Products) DeleteMaterial(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	mid, err := strconv.ParseInt(r.PathValue("mid"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := p.store.DeleteProductMaterial(r.Context(), id, mid); err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/productos/%d?guardado=1", id), http.StatusSeeOther)
}

// CreateConversion handles POST /productos/{id}/conversiones: adds a conversion rate
// between two units for this product.
func (p *Products) CreateConversion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "solicitud inválida", http.StatusBadRequest)
		return
	}
	fromUnitID, errFrom := strconv.ParseInt(r.FormValue("from_unit_id"), 10, 64)
	toUnitID, errTo := strconv.ParseInt(r.FormValue("to_unit_id"), 10, 64)
	if errFrom != nil || errTo != nil {
		p.renderConversionsError(w, r, id, "Selecciona ambas unidades.")
		return
	}
	if fromUnitID == toUnitID {
		p.renderConversionsError(w, r, id, "Las dos unidades deben ser diferentes.")
		return
	}
	rate, err := money.ParseMicros(strings.TrimSpace(r.FormValue("rate")))
	if err != nil || rate <= 0 {
		p.renderConversionsError(w, r, id, "Tasa inválida; usa un número positivo, p. ej. 100.")
		return
	}

	if _, err := p.store.CreateConversion(ctx, id, fromUnitID, toUnitID, rate); err != nil {
		if errors.Is(err, store.ErrDuplicateUnitPair) {
			p.renderConversionsError(w, r, id, "Ya existe una conversión entre estas unidades para este producto.")
			return
		}
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/productos/%d?guardado=1", id), http.StatusSeeOther)
}

// DeleteConversion handles POST /productos/{id}/conversiones/{cid}/eliminar.
func (p *Products) DeleteConversion(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	cid, err := strconv.ParseInt(r.PathValue("cid"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := p.store.DeleteConversion(r.Context(), cid); err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/productos/%d?guardado=1", id), http.StatusSeeOther)
}

// Update handles POST /productos/{id}.
func (p *Products) Update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "solicitud inválida", http.StatusBadRequest)
		return
	}
	form := parseProductForm(r)
	fieldErrors := form.Validate()

	if fieldErrors["sku"] == "" {
		existing, err := p.store.ProductBySKU(ctx, form.SKU)
		if err != nil {
			http.Error(w, "error interno", http.StatusInternalServerError)
			return
		}
		if existing != nil && existing.ID != id {
			fieldErrors["sku"] = "Ya existe un producto con este SKU."
		}
	}

	if len(fieldErrors) > 0 {
		p.renderFormError(w, r, "Editar producto", fmt.Sprintf("/productos/%d", id), id, form, fieldErrors)
		return
	}

	product := form.toProduct()
	product.ID = id
	if err := p.store.UpdateProduct(ctx, product); err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/productos/%d?guardado=1", id), http.StatusSeeOther)
}

// Delete handles POST /productos/{id}/eliminar — soft-delete, per the plan.
func (p *Products) Delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := p.store.SoftDeleteProduct(ctx, id); err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/productos?eliminado=1", http.StatusSeeOther)
}

// renderFormError re-renders the create or edit form with field-level validation
// errors. id is 0 for the create form (no per-product sections); for the edit form,
// its existing conversions and materials are loaded and shown alongside the
// re-displayed field values.
func (p *Products) renderFormError(w http.ResponseWriter, r *http.Request, title, action string, id int64, form ProductForm, fieldErrors map[string]string) {
	ctx := r.Context()
	families, err := p.store.ListFamilies(ctx)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	units, err := p.store.ListUnits(ctx)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	var sections views.ProductEditSections
	if id != 0 {
		if sections, err = p.loadEditSections(ctx, id); err != nil {
			http.Error(w, "error interno", http.StatusInternalServerError)
			return
		}
	}
	user, _ := UserFromContext(ctx)
	w.WriteHeader(http.StatusUnprocessableEntity)
	views.ProductForm(title, action, form.toValues(), fieldErrors, "", families, units, id, sections, navUserView(user)).Render(ctx, w)
}
