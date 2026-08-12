package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

// Customers holds the dependencies for the /clientes handlers.
type Customers struct {
	store *store.Store
}

func NewCustomers(s *store.Store) *Customers {
	return &Customers{store: s}
}

// CustomerForm is the /clientes/nuevo and /clientes/{id} form, per the plan's "one
// struct per form with Validate()" convention. Fields are raw strings so a validation
// error can redisplay exactly what the user typed. Only Name is required — RFC,
// contact, phone, email, address, and notes are all optional free text, matching the
// customers table.
type CustomerForm struct {
	Name        string
	RFC         string
	ContactName string
	Phone       string
	Email       string
	Address     string
	Notes       string
}

func parseCustomerForm(r *http.Request) CustomerForm {
	return CustomerForm{
		Name:        strings.TrimSpace(r.FormValue("name")),
		RFC:         strings.TrimSpace(r.FormValue("rfc")),
		ContactName: strings.TrimSpace(r.FormValue("contact_name")),
		Phone:       strings.TrimSpace(r.FormValue("phone")),
		Email:       strings.TrimSpace(r.FormValue("email")),
		Address:     strings.TrimSpace(r.FormValue("address")),
		Notes:       strings.TrimSpace(r.FormValue("notes")),
	}
}

func (f CustomerForm) Validate() map[string]string {
	errs := map[string]string{}
	if f.Name == "" {
		errs["name"] = "El nombre es obligatorio."
	}
	if f.Email != "" && !strings.Contains(f.Email, "@") {
		errs["email"] = "Email inválido."
	}
	return errs
}

func (f CustomerForm) toValues() views.CustomerFormValues {
	return views.CustomerFormValues{
		Name:        f.Name,
		RFC:         f.RFC,
		ContactName: f.ContactName,
		Phone:       f.Phone,
		Email:       f.Email,
		Address:     f.Address,
		Notes:       f.Notes,
	}
}

func (f CustomerForm) toCustomer() store.Customer {
	return store.Customer{
		Name:        f.Name,
		RFC:         f.RFC,
		ContactName: f.ContactName,
		Phone:       f.Phone,
		Email:       f.Email,
		Address:     f.Address,
		Notes:       f.Notes,
	}
}

func customerToValues(c store.Customer) views.CustomerFormValues {
	return views.CustomerFormValues{
		Name:        c.Name,
		RFC:         c.RFC,
		ContactName: c.ContactName,
		Phone:       c.Phone,
		Email:       c.Email,
		Address:     c.Address,
		Notes:       c.Notes,
	}
}

// List renders /clientes: the full page normally, or just the table body when called
// via htmx live search (identified by the HX-Request header).
func (cs *Customers) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	customers, err := cs.store.ListCustomers(ctx, query)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		views.CustomersTableBody(customers).Render(ctx, w)
		return
	}

	successMsg := ""
	switch {
	case r.URL.Query().Get("guardado") == "1":
		successMsg = "Cliente guardado."
	case r.URL.Query().Get("eliminado") == "1":
		successMsg = "Cliente eliminado."
	}
	user, _ := UserFromContext(ctx)
	views.CustomersList(customers, query, successMsg, navUserView(user)).Render(ctx, w)
}

// NewPage renders the empty create form at /clientes/nuevo.
func (cs *Customers) NewPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, _ := UserFromContext(ctx)
	views.CustomerForm("Nuevo cliente", "/clientes/nuevo", views.CustomerFormValues{}, nil, "", navUserView(user)).Render(ctx, w)
}

// Create handles POST /clientes/nuevo.
func (cs *Customers) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(w, "solicitud inválida", http.StatusBadRequest)
		return
	}
	form := parseCustomerForm(r)
	fieldErrors := form.Validate()
	if len(fieldErrors) > 0 {
		cs.renderFormError(w, r, "Nuevo cliente", "/clientes/nuevo", form, fieldErrors)
		return
	}

	id, err := cs.store.CreateCustomer(ctx, form.toCustomer())
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/clientes/%d?guardado=1", id), http.StatusSeeOther)
}

// EditPage renders the edit form at GET /clientes/{id}.
func (cs *Customers) EditPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	customer, err := cs.store.CustomerByID(ctx, id)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	if customer == nil {
		http.NotFound(w, r)
		return
	}
	successMsg := ""
	if r.URL.Query().Get("guardado") == "1" {
		successMsg = "Cliente guardado."
	}
	user, _ := UserFromContext(ctx)
	action := fmt.Sprintf("/clientes/%d", id)
	views.CustomerForm("Editar cliente", action, customerToValues(*customer), nil, successMsg, navUserView(user)).Render(ctx, w)
}

// Update handles POST /clientes/{id}.
func (cs *Customers) Update(w http.ResponseWriter, r *http.Request) {
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
	form := parseCustomerForm(r)
	fieldErrors := form.Validate()
	if len(fieldErrors) > 0 {
		cs.renderFormError(w, r, "Editar cliente", fmt.Sprintf("/clientes/%d", id), form, fieldErrors)
		return
	}

	customer := form.toCustomer()
	customer.ID = id
	if err := cs.store.UpdateCustomer(ctx, customer); err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/clientes/%d?guardado=1", id), http.StatusSeeOther)
}

// Delete handles POST /clientes/{id}/eliminar — soft-delete.
func (cs *Customers) Delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := cs.store.SoftDeleteCustomer(ctx, id); err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/clientes?eliminado=1", http.StatusSeeOther)
}

func (cs *Customers) renderFormError(w http.ResponseWriter, r *http.Request, title, action string, form CustomerForm, fieldErrors map[string]string) {
	ctx := r.Context()
	user, _ := UserFromContext(ctx)
	w.WriteHeader(http.StatusUnprocessableEntity)
	views.CustomerForm(title, action, form.toValues(), fieldErrors, "", navUserView(user)).Render(ctx, w)
}
