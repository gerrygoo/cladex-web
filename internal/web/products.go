package web

import (
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
	return v
}

// List renders /productos: the full page normally, or just the table body when
// called via htmx live search (identified by the HX-Request header).
func (p *Products) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	products, err := p.store.ListProducts(ctx, query)
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
	views.ProductsList(products, query, successMsg, navUserView(user)).Render(ctx, w)
}

// NewPage renders the empty create form at /productos/nuevo.
func (p *Products) NewPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	families, err := p.store.ListFamilies(ctx)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	user, _ := UserFromContext(ctx)
	values := views.ProductFormValues{Currency: "MXN"}
	views.ProductForm("Nuevo producto", "/productos/nuevo", values, nil, "", families, navUserView(user)).Render(ctx, w)
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
		p.renderFormError(w, r, "Nuevo producto", "/productos/nuevo", form, fieldErrors)
		return
	}

	id, err := p.store.CreateProduct(ctx, form.toProduct())
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/productos/%d?guardado=1", id), http.StatusSeeOther)
}

// EditPage renders the edit form at GET /productos/{id}.
func (p *Products) EditPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	product, err := p.store.ProductByID(ctx, id)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	if product == nil {
		http.NotFound(w, r)
		return
	}
	families, err := p.store.ListFamilies(ctx)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	successMsg := ""
	if r.URL.Query().Get("guardado") == "1" {
		successMsg = "Producto guardado."
	}
	user, _ := UserFromContext(ctx)
	action := fmt.Sprintf("/productos/%d", id)
	views.ProductForm("Editar producto", action, productToValues(*product), nil, successMsg, families, navUserView(user)).Render(ctx, w)
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
		p.renderFormError(w, r, "Editar producto", fmt.Sprintf("/productos/%d", id), form, fieldErrors)
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

func (p *Products) renderFormError(w http.ResponseWriter, r *http.Request, title, action string, form ProductForm, fieldErrors map[string]string) {
	ctx := r.Context()
	families, err := p.store.ListFamilies(ctx)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	user, _ := UserFromContext(ctx)
	w.WriteHeader(http.StatusUnprocessableEntity)
	views.ProductForm(title, action, form.toValues(), fieldErrors, "", families, navUserView(user)).Render(ctx, w)
}
