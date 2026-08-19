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
	Currency    string
	UnitPrice   string
	Cost        string
	KgPerM      string
	UnitID      string
}

func parseProductForm(r *http.Request) ProductForm {
	currency := r.FormValue("currency")
	if currency == "" {
		currency = "MXN"
	}
	return ProductForm{
		FamilyID:    r.FormValue("family_id"),
		SKU:         strings.TrimSpace(r.FormValue("sku")),
		Description: strings.TrimSpace(r.FormValue("description")),
		Currency:    currency,
		UnitPrice:   strings.TrimSpace(r.FormValue("unit_price")),
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
	if f.Currency != "MXN" && f.Currency != "USD" {
		errs["currency"] = "Moneda inválida."
	}
	if f.UnitPrice != "" {
		if _, err := money.ParseMicros(f.UnitPrice); err != nil {
			errs["unit_price"] = "Precio inválido; usa un número, p. ej. 123.45."
		}
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
		Currency:    f.Currency,
		UnitPrice:   f.UnitPrice,
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
		Currency:    f.Currency,
	}
	if f.UnitPrice != "" {
		m, _ := money.ParseMicros(f.UnitPrice)
		p.UnitPriceMicros = &m
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
		Currency:    p.Currency,
	}
	if p.UnitPriceMicros != nil {
		v.UnitPrice = p.UnitPriceMicros.String()
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
	products, err := p.store.ListProducts(ctx, query, sort, dir)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		views.ProductsTableBody(products).Render(ctx, w)
		return
	}

	successMsg := ""
	switch {
	case r.URL.Query().Get("guardado") == "1":
		successMsg = "Producto guardado."
	case r.URL.Query().Get("eliminado") == "1":
		successMsg = "Producto eliminado."
	}
	user, _ := UserFromContext(ctx)
	views.ProductsList(products, query, sort, dir, successMsg, navUserView(user)).Render(ctx, w)
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
	values := views.ProductFormValues{Currency: "MXN"}
	views.ProductForm("Nuevo producto", "/productos/nuevo", values, nil, "", families, units, 0, nil, "", navUserView(user)).Render(ctx, w)
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

// loadEditData loads everything the edit form (and its unit-conversions section)
// needs in one place, since both EditPage and the conversion mutation handlers'
// error paths need the same four reads. Returns a nil product, with no error, if id
// doesn't exist.
func (p *Products) loadEditData(ctx context.Context, id int64) (*store.Product, []store.ProductFamily, []store.Unit, []store.ProductUnitConversion, error) {
	product, err := p.store.ProductByID(ctx, id)
	if err != nil || product == nil {
		return nil, nil, nil, nil, err
	}
	families, err := p.store.ListFamilies(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	units, err := p.store.ListUnits(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	conversions, err := p.store.ListConversionsByProduct(ctx, id)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return product, families, units, conversions, nil
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
	product, families, units, conversions, err := p.loadEditData(ctx, id)
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
	views.ProductForm("Editar producto", action, productToValues(*product), nil, successMsg, families, units, id, conversions, "", navUserView(user)).Render(ctx, w)
}

// renderConversionsError re-renders the edit form with a validation error scoped to
// the unit-conversions section — the main product fields are unaffected, so their
// values come straight from the stored product, not re-parsed form input.
func (p *Products) renderConversionsError(w http.ResponseWriter, r *http.Request, id int64, errorMsg string) {
	ctx := r.Context()
	product, families, units, conversions, err := p.loadEditData(ctx, id)
	if err != nil || product == nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	user, _ := UserFromContext(ctx)
	w.WriteHeader(http.StatusUnprocessableEntity)
	action := fmt.Sprintf("/productos/%d", id)
	views.ProductForm("Editar producto", action, productToValues(*product), nil, "", families, units, id, conversions, errorMsg, navUserView(user)).Render(ctx, w)
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
// errors. id is 0 for the create form (no conversions section); for the edit form,
// its existing conversions are loaded and shown alongside the re-displayed field
// values.
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
	var conversions []store.ProductUnitConversion
	if id != 0 {
		conversions, err = p.store.ListConversionsByProduct(ctx, id)
		if err != nil {
			http.Error(w, "error interno", http.StatusInternalServerError)
			return
		}
	}
	user, _ := UserFromContext(ctx)
	w.WriteHeader(http.StatusUnprocessableEntity)
	views.ProductForm(title, action, form.toValues(), fieldErrors, "", families, units, id, conversions, "", navUserView(user)).Render(ctx, w)
}
