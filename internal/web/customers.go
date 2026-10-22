package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
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
// contact, phone, email, address, postal code, tax regime, and notes are all optional
// free text, matching the customers table. PostalCode/TaxRegime are kept separate from
// Address so a future invoicing/billing portal integration can read them directly; a
// customer may still be a prospect with no fiscal data yet.
type CustomerForm struct {
	Name        string
	RFC         string
	ContactName string
	Phone       string
	Email       string
	Address     string
	PostalCode  string
	TaxRegime   string
	Notes       string
	LegalName   string
	CFDIUse     string
}

func parseCustomerForm(r *http.Request) CustomerForm {
	return CustomerForm{
		Name:        strings.TrimSpace(r.FormValue("name")),
		RFC:         strings.ToUpper(strings.TrimSpace(r.FormValue("rfc"))),
		ContactName: strings.TrimSpace(r.FormValue("contact_name")),
		Phone:       strings.TrimSpace(r.FormValue("phone")),
		Email:       strings.TrimSpace(r.FormValue("email")),
		Address:     strings.TrimSpace(r.FormValue("address")),
		PostalCode:  strings.TrimSpace(r.FormValue("postal_code")),
		TaxRegime:   strings.TrimSpace(r.FormValue("tax_regime")),
		Notes:       strings.TrimSpace(r.FormValue("notes")),
		LegalName:   strings.TrimSpace(r.FormValue("legal_name")),
		CFDIUse:     strings.TrimSpace(r.FormValue("cfdi_use")),
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
	if !store.ValidSATCode(store.TaxRegimes, f.TaxRegime) {
		errs["tax_regime"] = "Elige un régimen fiscal de la lista."
	}
	if !store.ValidSATCode(store.CFDIUses, f.CFDIUse) {
		errs["cfdi_use"] = "Elige un uso de CFDI de la lista."
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
		PostalCode:  f.PostalCode,
		TaxRegime:   f.TaxRegime,
		Notes:       f.Notes,
		LegalName:   f.LegalName,
		CFDIUse:     f.CFDIUse,
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
		PostalCode:  f.PostalCode,
		TaxRegime:   f.TaxRegime,
		Notes:       f.Notes,
		LegalName:   f.LegalName,
		CFDIUse:     f.CFDIUse,
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
		PostalCode:  c.PostalCode,
		TaxRegime:   c.TaxRegime,
		Notes:       c.Notes,
		LegalName:   c.LegalName,
		CFDIUse:     c.CFDIUse,
	}
}

// List renders /clientes: the full page normally, or just the table body when called
// via htmx live search (identified by the HX-Request header).
func (cs *Customers) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	sort, dir := sortParams(r)
	filters := filterParams(r)
	customers, err := cs.store.ListCustomers(ctx, query, sort, dir, filters)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}

	customers, pager := paginate(r, customers)
	lv := views.ListView{Base: "/clientes", Query: query, Sort: sort, Dir: dir, Filters: filters, Pager: pager}

	if r.Header.Get("HX-Request") == "true" {
		views.CustomersTableBody(customers, isAdmin(ctx)).Render(ctx, w)
		views.ListPagerOOB(lv).Render(ctx, w)
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
	views.CustomersList(customers, lv, successMsg, navUserView(user)).Render(ctx, w)
}

// returnTo is where the new-customer form came from (issue #29): "cotizacion" when it was
// opened from Nueva cotización's "Agregar nuevo cliente", with the series already picked
// there, so saving or cancelling goes back to it. Zero means the catalog.
type returnTo struct {
	Volver, Serie string
}

func parseReturnTo(r *http.Request) returnTo {
	if r.FormValue("volver") != "cotizacion" {
		return returnTo{}
	}
	return returnTo{Volver: "cotizacion", Serie: strings.TrimSpace(r.FormValue("serie"))}
}

// quotePath is Nueva cotización with customerID (0: none) and the series selected.
func (rt returnTo) quotePath(customerID int64) string {
	v := url.Values{}
	if customerID != 0 {
		v.Set("cliente", strconv.FormatInt(customerID, 10))
	}
	if rt.Serie != "" {
		v.Set("serie", rt.Serie)
	}
	if len(v) == 0 {
		return "/cotizaciones/nueva"
	}
	return "/cotizaciones/nueva?" + v.Encode()
}

// newValues puts the return trip on the form's values.
func (rt returnTo) newValues(v views.CustomerFormValues) views.CustomerFormValues {
	v.Volver, v.Serie = rt.Volver, rt.Serie
	if rt.Volver != "" {
		v.CancelHref = rt.quotePath(0)
	}
	return v
}

// NewPage renders the empty create form at /clientes/nuevo, or with ?volver=cotizacion
// the same form on a trip from Nueva cotización.
func (cs *Customers) NewPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, _ := UserFromContext(ctx)
	values := parseReturnTo(r).newValues(views.CustomerFormValues{})
	views.CustomerForm("Nuevo cliente", "/clientes/nuevo", values, nil, nil, "", nil, navUserView(user)).Render(ctx, w)
}

// Create handles POST /clientes/nuevo. Before creating, it looks for customers the new
// one may duplicate (store.SimilarCustomers) and, unless the form already says
// confirmar=1, shows them instead of saving: the user picks one of them, creates the new
// one anyway, or goes back to editing (issue #29). Coming from Nueva cotización, it
// returns there with the customer selected.
func (cs *Customers) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(w, "solicitud inválida", http.StatusBadRequest)
		return
	}
	form := parseCustomerForm(r)
	rt := parseReturnTo(r)
	fieldErrors := form.Validate()
	if len(fieldErrors) > 0 {
		cs.renderNewForm(w, r, form, rt, fieldErrors, nil)
		return
	}
	if r.FormValue("confirmar") != "1" {
		similar, err := cs.store.SimilarCustomers(ctx, form.Name, form.RFC)
		if err != nil {
			http.Error(w, "error interno", http.StatusInternalServerError)
			return
		}
		if len(similar) > 0 {
			cs.renderNewForm(w, r, form, rt, nil, similar)
			return
		}
	}

	id, err := cs.store.CreateCustomer(ctx, form.toCustomer())
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	if rt.Volver != "" {
		http.Redirect(w, r, rt.quotePath(id), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/clientes/%d?guardado=1", id), http.StatusSeeOther)
}

// renderNewForm re-renders the new-customer form with its errors, or with the customers
// it may duplicate and, for each, where "Usar este cliente" leads.
func (cs *Customers) renderNewForm(w http.ResponseWriter, r *http.Request, form CustomerForm, rt returnTo, fieldErrors map[string]string, similar []store.Customer) {
	ctx := r.Context()
	user, _ := UserFromContext(ctx)
	var matches []views.SimilarCustomer
	for _, c := range similar {
		use := "/clientes"
		if rt.Volver != "" {
			use = rt.quotePath(c.ID)
		}
		matches = append(matches, views.SimilarCustomer{Customer: c, UseHref: use})
	}
	w.WriteHeader(http.StatusUnprocessableEntity)
	views.CustomerForm("Nuevo cliente", "/clientes/nuevo", rt.newValues(form.toValues()), fieldErrors, matches, "", nil, navUserView(user)).Render(ctx, w)
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
	history, err := cs.history(ctx, id)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	user, _ := UserFromContext(ctx)
	action := fmt.Sprintf("/clientes/%d", id)
	views.CustomerForm("Editar cliente", action, customerToValues(*customer), nil, nil, successMsg, history, navUserView(user)).Render(ctx, w)
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
		cs.renderFormError(w, r, id, form, fieldErrors)
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

// renderFormError re-renders the edit form of customer id with its errors.
func (cs *Customers) renderFormError(w http.ResponseWriter, r *http.Request, id int64, form CustomerForm, fieldErrors map[string]string) {
	ctx := r.Context()
	history, err := cs.history(ctx, id)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	user, _ := UserFromContext(ctx)
	w.WriteHeader(http.StatusUnprocessableEntity)
	views.CustomerForm("Editar cliente", fmt.Sprintf("/clientes/%d", id), form.toValues(), fieldErrors, nil, "", history, navUserView(user)).Render(ctx, w)
}

// customerFieldLabels names each audited customer column as the form labels it, in the
// form's order, which is the order an edit's changes are listed in. deleted_at isn't
// here: a deleted customer has no page.
var customerFieldLabels = []struct{ Column, Label string }{
	{"name", "Nombre"},
	{"rfc", "RFC"},
	{"contact_name", "Contacto"},
	{"phone", "Teléfono"},
	{"email", "Email"},
	{"address", "Dirección"},
	{"legal_name", "Razón social"},
	{"postal_code", "Código postal fiscal"},
	{"tax_regime", "Régimen fiscal"},
	{"cfdi_use", "Uso de CFDI predeterminado"},
	{"notes", "Notas"},
}

// history builds customer id's Historial from the audit trail (migrations/0004 and
// 0022), newest first: each edit with the fields it changed, then its creation. An
// update that changed none of the shown fields is left out. A customer from before the
// trail has no creation entry, so its row's created_at stands in, with nobody named.
func (cs *Customers) history(ctx context.Context, id int64) ([]views.CustomerHistoryEntry, error) {
	entries, err := cs.store.AuditLog(ctx, store.AuditFilter{TableName: "customers", RowKey: strconv.FormatInt(id, 10), Limit: 1000})
	if err != nil {
		return nil, err
	}
	var history []views.CustomerHistoryEntry
	created := false
	for _, e := range entries {
		switch e.Op {
		case "insert":
			created = true
			history = append(history, views.CustomerHistoryEntry{Who: auditWho(e), At: e.At, Created: true})
		case "update":
			changed := map[string]bool{}
			for _, k := range e.ChangedFields() {
				changed[k] = true
			}
			var changes []views.CustomerFieldChange
			for _, f := range customerFieldLabels {
				if changed[f.Column] {
					changes = append(changes, views.CustomerFieldChange{
						Label: f.Label,
						Old:   customerFieldText(f.Column, e.OldValues[f.Column]),
						New:   customerFieldText(f.Column, e.NewValues[f.Column]),
					})
				}
			}
			if len(changes) > 0 {
				history = append(history, views.CustomerHistoryEntry{Who: auditWho(e), At: e.At, Changes: changes})
			}
		}
	}
	if !created {
		at, err := cs.store.CustomerCreatedAt(ctx, id)
		if err != nil {
			return nil, err
		}
		history = append(history, views.CustomerHistoryEntry{At: at, Created: true})
	}
	return history, nil
}

// auditWho is who an audit entry names: the user's full name, or "Sistema" for a write
// with no user behind it (the admin command line, the old workbook import).
func auditWho(e store.AuditEntry) string {
	switch {
	case e.ActorFull != "":
		return e.ActorFull
	case e.ActorName != "":
		return e.ActorName
	}
	return "Sistema"
}

// customerFieldText is how one audited value reads in the Historial: "(vacío)" when
// empty, and a SAT code with its description.
func customerFieldText(column string, v any) string {
	text := ""
	if v != nil {
		text = strings.TrimSpace(fmt.Sprint(v))
	}
	if text == "" {
		return "(vacío)"
	}
	switch column {
	case "tax_regime":
		return satText(store.TaxRegimes, text)
	case "cfdi_use":
		return satText(store.CFDIUses, text)
	}
	return text
}

func satText(catalog []store.SATCode, code string) string {
	for _, c := range catalog {
		if c.Code == code {
			return c.Code + " - " + c.Description
		}
	}
	return code
}
