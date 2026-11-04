package web

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

// Suppliers holds the dependencies for the /proveedores handlers (issue #64). Admins
// create, edit, deactivate and delete suppliers; a vendedor sees the list and each one
// read-only, never its bank data.
type Suppliers struct {
	store *store.Store
}

func NewSuppliers(s *store.Store) *Suppliers {
	return &Suppliers{store: s}
}

// blankContactSlots is how many empty contact slots the form offers below the saved
// ones (static/app.js shows them one at a time).
const blankContactSlots = 3

// maxContactSlots bounds the contact slots a post may carry.
const maxContactSlots = 100

// ProveedorForm is the new and edit supplier form, as posted.
type ProveedorForm struct {
	views.SupplierFormValues
}

func parseProveedorForm(r *http.Request) ProveedorForm {
	v := views.SupplierFormValues{
		Name:         strings.TrimSpace(r.FormValue("name")),
		RFC:          strings.ToUpper(strings.ReplaceAll(strings.Join(strings.Fields(r.FormValue("rfc")), ""), "-", "")),
		LegalName:    strings.TrimSpace(r.FormValue("legal_name")),
		Street:       strings.TrimSpace(r.FormValue("street")),
		ExtNumber:    strings.TrimSpace(r.FormValue("ext_number")),
		Neighborhood: strings.TrimSpace(r.FormValue("neighborhood")),
		Municipality: strings.TrimSpace(r.FormValue("municipality")),
		State:        strings.TrimSpace(r.FormValue("state")),
		PostalCode:   strings.TrimSpace(r.FormValue("postal_code")),
		TaxRegime:    strings.TrimSpace(r.FormValue("tax_regime")),
		Notes:        strings.TrimSpace(r.FormValue("notes")),
		CreditDays:   strings.TrimSpace(r.FormValue("credit_days")),
		Currency:     strings.TrimSpace(r.FormValue("currency")),
		PaymentTerms: strings.TrimSpace(r.FormValue("payment_terms")),
		LeadTime:     strings.TrimSpace(r.FormValue("lead_time")),
		Bank:         strings.TrimSpace(r.FormValue("bank")),
		CLABE:        strings.Join(strings.Fields(r.FormValue("clabe")), ""),
		Beneficiary:  strings.TrimSpace(r.FormValue("beneficiary")),
		FamilyIDs:    map[int64]bool{},
		Confirmed:    r.FormValue("confirmar") == "1",
	}
	n, _ := strconv.Atoi(r.FormValue("contactos"))
	n = min(max(n, 0), maxContactSlots)
	for i := range n {
		field := func(name string) string {
			return strings.TrimSpace(r.FormValue("contacto-" + strconv.Itoa(i) + "-" + name))
		}
		v.Contacts = append(v.Contacts, views.SupplierContactValues{
			ID:         field("id"),
			Name:       field("nombre"),
			Role:       field("puesto"),
			Phone:      field("telefono"),
			Email:      field("email"),
			ReceivesPO: field("oc") == "1",
			Remove:     field("quitar") == "1",
		})
	}
	for _, s := range r.Form["familia"] {
		if id, err := strconv.ParseInt(s, 10, 64); err == nil {
			v.FamilyIDs[id] = true
		}
	}
	return ProveedorForm{v}
}

// Validate checks the form. The fiscal data and full fiscal address are required; the
// RFC is 12 or 13 characters, the CP five digits and a CLABE 18 digits with a valid
// check digit.
func (f ProveedorForm) Validate() map[string]string {
	errs := map[string]string{}
	required := []struct{ field, value, msg string }{
		{"name", f.Name, "El nombre comercial es obligatorio."},
		{"rfc", f.RFC, "El RFC es obligatorio."},
		{"legal_name", f.LegalName, "La razón social es obligatoria."},
		{"street", f.Street, "La calle es obligatoria."},
		{"ext_number", f.ExtNumber, "El número es obligatorio."},
		{"neighborhood", f.Neighborhood, "La colonia es obligatoria."},
		{"municipality", f.Municipality, "El municipio o alcaldía es obligatorio."},
		{"state", f.State, "El estado es obligatorio."},
		{"postal_code", f.PostalCode, "El código postal es obligatorio."},
	}
	for _, r := range required {
		if r.value == "" {
			errs[r.field] = r.msg
		}
	}
	if f.RFC != "" && !store.ValidRFC(f.RFC) {
		errs["rfc"] = "El RFC debe tener 12 caracteres (persona moral) o 13 (persona física), p. ej. CAR191126QX6."
	}
	if f.PostalCode != "" && !store.ValidPostalCode(f.PostalCode) {
		errs["postal_code"] = "El código postal son 5 dígitos, p. ej. 76060."
	}
	if !store.ValidSATCode(store.TaxRegimes, f.TaxRegime) {
		errs["tax_regime"] = "Elige un régimen fiscal de la lista."
	}
	if f.CreditDays != "" {
		if n, err := strconv.ParseInt(f.CreditDays, 10, 64); err != nil || n < 0 {
			errs["credit_days"] = "Los días de crédito son un número entero, p. ej. 30."
		}
	}
	if f.Currency != "" && !store.ValidCurrency(f.Currency) {
		errs["currency"] = "Elige MXN o USD."
	}
	if f.CLABE != "" && !store.ValidCLABE(f.CLABE) {
		errs["clabe"] = "La CLABE son 18 dígitos y no coincide con su dígito verificador; revísala."
	}
	for i, c := range f.Contacts {
		if c.Blank() || c.Remove {
			continue
		}
		if c.Name == "" {
			errs["contacto-"+strconv.Itoa(i)+"-nombre"] = "Escribe el nombre del contacto."
		}
		if c.Email != "" && !strings.Contains(c.Email, "@") {
			errs["contacto-"+strconv.Itoa(i)+"-email"] = "Email inválido."
		}
	}
	return errs
}

// toSupplier is the form as a store.Supplier: empty slots and removed contacts left out,
// and only familias among options.
func (f ProveedorForm) toSupplier(options []store.SupplierFamilyOption) store.Supplier {
	sup := store.Supplier{
		Name: f.Name, RFC: f.RFC, LegalName: f.LegalName,
		Street: f.Street, ExtNumber: f.ExtNumber, Neighborhood: f.Neighborhood,
		Municipality: f.Municipality, State: f.State, PostalCode: f.PostalCode,
		TaxRegime: f.TaxRegime, Notes: f.Notes, Currency: f.Currency,
		PaymentTerms: f.PaymentTerms, LeadTime: f.LeadTime,
		Bank: f.Bank, CLABE: f.CLABE, Beneficiary: f.Beneficiary,
	}
	if n, err := strconv.ParseInt(f.CreditDays, 10, 64); err == nil {
		sup.CreditDays = &n
	}
	for _, c := range f.Contacts {
		if c.Blank() || c.Remove {
			continue
		}
		id, _ := strconv.ParseInt(c.ID, 10, 64)
		sup.Contacts = append(sup.Contacts, store.SupplierContact{
			ID: id, Name: c.Name, Role: c.Role, Phone: c.Phone, Email: c.Email, ReceivesPO: c.ReceivesPO,
		})
	}
	for _, o := range options {
		if f.FamilyIDs[o.ID] {
			sup.FamilyIDs = append(sup.FamilyIDs, o.ID)
		}
	}
	return sup
}

// reassignments are the familias the form would take from another supplier than id.
func (f ProveedorForm) reassignments(id int64, options []store.SupplierFamilyOption) []views.SupplierReassignment {
	var out []views.SupplierReassignment
	for _, o := range options {
		if f.FamilyIDs[o.ID] && o.SupplierID != 0 && o.SupplierID != id {
			out = append(out, views.SupplierReassignment{Familia: o.Name, Supplier: o.SupplierName})
		}
	}
	return out
}

func supplierToValues(sup store.Supplier) views.SupplierFormValues {
	v := views.SupplierFormValues{
		Name: sup.Name, RFC: sup.RFC, LegalName: sup.LegalName,
		Street: sup.Street, ExtNumber: sup.ExtNumber, Neighborhood: sup.Neighborhood,
		Municipality: sup.Municipality, State: sup.State, PostalCode: sup.PostalCode,
		TaxRegime: sup.TaxRegime, Notes: sup.Notes, Currency: sup.Currency,
		PaymentTerms: sup.PaymentTerms, LeadTime: sup.LeadTime,
		Bank: sup.Bank, CLABE: sup.CLABE, Beneficiary: sup.Beneficiary,
		FamilyIDs: map[int64]bool{},
	}
	if sup.CreditDays != nil {
		v.CreditDays = strconv.FormatInt(*sup.CreditDays, 10)
	}
	for _, c := range sup.Contacts {
		v.Contacts = append(v.Contacts, views.SupplierContactValues{
			ID: strconv.FormatInt(c.ID, 10), Name: c.Name, Role: c.Role, Phone: c.Phone,
			Email: c.Email, ReceivesPO: c.ReceivesPO,
		})
	}
	for _, id := range sup.FamilyIDs {
		v.FamilyIDs[id] = true
	}
	return v
}

// withBlankContacts adds empty contact slots until there are blankContactSlots of them.
func withBlankContacts(v views.SupplierFormValues) views.SupplierFormValues {
	blank := 0
	for _, c := range v.Contacts {
		if c.Blank() {
			blank++
		}
	}
	for ; blank < blankContactSlots; blank++ {
		v.Contacts = append(v.Contacts, views.SupplierContactValues{})
	}
	return v
}

// List renders /proveedores, or only the table body for the htmx live search.
func (sp *Suppliers) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	sort, dir := sortParams(r)
	filters := filterParams(r)
	suppliers, err := sp.store.ListSuppliers(ctx, query, sort, dir, filters)
	if err != nil {
		serverError(w, r, err)
		return
	}
	suppliers, pager := paginate(r, suppliers)
	lv := views.ListView{Base: "/proveedores", Query: query, Sort: sort, Dir: dir, Filters: filters, Pager: pager}
	if r.Header.Get("HX-Request") == "true" {
		views.SuppliersTableBody(suppliers, isAdmin(ctx)).Render(ctx, w)
		views.ListPagerOOB(lv).Render(ctx, w)
		return
	}
	successMsg := ""
	if r.URL.Query().Get("eliminado") == "1" {
		successMsg = "Proveedor eliminado."
	}
	user, _ := UserFromContext(ctx)
	views.SuppliersList(suppliers, lv, successMsg, navUserView(user)).Render(ctx, w)
}

// NewPage renders the empty form at /proveedores/nuevo.
func (sp *Suppliers) NewPage(w http.ResponseWriter, r *http.Request) {
	sp.renderForm(w, r, http.StatusOK, views.SupplierFormPage{
		Values: withBlankContacts(views.SupplierFormValues{FamilyIDs: map[int64]bool{}}),
	})
}

// renderForm fills in the familia options and renders p with status.
func (sp *Suppliers) renderForm(w http.ResponseWriter, r *http.Request, status int, p views.SupplierFormPage) {
	ctx := r.Context()
	if p.Families == nil {
		options, err := sp.store.SupplierFamilyOptions(ctx)
		if err != nil {
			serverError(w, r, err)
			return
		}
		p.Families = options
	}
	user, _ := UserFromContext(ctx)
	w.WriteHeader(status)
	views.SupplierForm(p, navUserView(user)).Render(ctx, w)
}

// Create handles POST /proveedores/nuevo. Before saving it asks about possible
// duplicates (unless confirmar=1), then about familias that belong to another supplier
// (unless reasignar=1), the way Clientes asks about duplicates.
func (sp *Suppliers) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(w, "solicitud inválida", http.StatusBadRequest)
		return
	}
	form := parseProveedorForm(r)
	options, err := sp.store.SupplierFamilyOptions(ctx)
	if err != nil {
		serverError(w, r, err)
		return
	}
	page := views.SupplierFormPage{Values: withBlankContacts(form.SupplierFormValues), Families: options}
	if errs := form.Validate(); len(errs) > 0 {
		page.Errors = errs
		sp.renderForm(w, r, http.StatusUnprocessableEntity, page)
		return
	}
	if !form.Confirmed {
		similar, err := sp.store.SimilarSuppliers(ctx, form.Name, form.LegalName, form.RFC)
		if err != nil {
			serverError(w, r, err)
			return
		}
		if len(similar) > 0 {
			page.Similar = similar
			sp.renderForm(w, r, http.StatusUnprocessableEntity, page)
			return
		}
	}
	if r.FormValue("reasignar") != "1" {
		if re := form.reassignments(0, options); len(re) > 0 {
			page.Reassign = re
			sp.renderForm(w, r, http.StatusUnprocessableEntity, page)
			return
		}
	}
	id, err := sp.store.CreateSupplier(ctx, form.toSupplier(options))
	if err != nil {
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/proveedores/%d?guardado=1", id), http.StatusSeeOther)
}

// loadSupplier finds the supplier named by the {id} path value, writing a 404 and
// returning nil if there is none.
func (sp *Suppliers) loadSupplier(w http.ResponseWriter, r *http.Request) *store.Supplier {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return nil
	}
	sup, err := sp.store.SupplierByID(r.Context(), id)
	if err != nil {
		serverError(w, r, err)
		return nil
	}
	if sup == nil {
		http.NotFound(w, r)
	}
	return sup
}

// Page renders GET /proveedores/{id}: the edit form for an admin, the read-only page for
// a vendedor. ?eliminar=bloqueado follows a delete that its familias prevented.
func (sp *Suppliers) Page(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sup := sp.loadSupplier(w, r)
	if sup == nil {
		return
	}
	admin := isAdmin(ctx)
	history, err := sp.history(ctx, sup.ID, admin)
	if err != nil {
		serverError(w, r, err)
		return
	}
	options, err := sp.store.SupplierFamilyOptions(ctx)
	if err != nil {
		serverError(w, r, err)
		return
	}
	if !admin {
		var families []string
		for _, o := range options {
			if o.SupplierID == sup.ID {
				families = append(families, views.FamilyOptionLabel(o))
			}
		}
		user, _ := UserFromContext(ctx)
		views.SupplierDetail(views.SupplierDetailPage{Supplier: *sup, Families: families, History: history}, navUserView(user)).Render(ctx, w)
		return
	}
	page := views.SupplierFormPage{
		ID: sup.ID, Values: withBlankContacts(supplierToValues(*sup)), Families: options,
		SavedName: sup.Name, Inactive: sup.Inactive, History: history,
	}
	switch r.URL.Query().Get("guardado") {
	case "1":
		page.SuccessMsg = "Proveedor guardado."
	case "inactivo":
		page.SuccessMsg = "Proveedor marcado como inactivo."
	case "activo":
		page.SuccessMsg = "Proveedor reactivado."
	}
	status := http.StatusOK
	if r.URL.Query().Get("eliminar") == "bloqueado" {
		for _, o := range options {
			if o.SupplierID == sup.ID {
				page.InUse = append(page.InUse, o.Name)
			}
		}
	}
	sp.renderForm(w, r, status, page)
}

// Update handles POST /proveedores/{id}, asking first about familias that belong to
// another supplier (unless reasignar=1).
func (sp *Suppliers) Update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sup := sp.loadSupplier(w, r)
	if sup == nil {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "solicitud inválida", http.StatusBadRequest)
		return
	}
	form := parseProveedorForm(r)
	options, err := sp.store.SupplierFamilyOptions(ctx)
	if err != nil {
		serverError(w, r, err)
		return
	}
	page := views.SupplierFormPage{
		ID: sup.ID, Values: withBlankContacts(form.SupplierFormValues), Families: options,
		SavedName: sup.Name, Inactive: sup.Inactive,
	}
	errs := form.Validate()
	re := form.reassignments(sup.ID, options)
	if len(errs) > 0 || (len(re) > 0 && r.FormValue("reasignar") != "1") {
		if len(errs) > 0 {
			page.Errors = errs
		} else {
			page.Reassign = re
		}
		if page.History, err = sp.history(ctx, sup.ID, true); err != nil {
			serverError(w, r, err)
			return
		}
		sp.renderForm(w, r, http.StatusUnprocessableEntity, page)
		return
	}
	updated := form.toSupplier(options)
	updated.ID = sup.ID
	if err := sp.store.UpdateSupplier(ctx, updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/proveedores/%d?guardado=1", sup.ID), http.StatusSeeOther)
}

// Delete handles POST /proveedores/{id}/eliminar. A supplier that still supplies
// familias isn't deleted: its page says which and offers to mark it inactive.
func (sp *Suppliers) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	err = sp.store.DeleteSupplier(r.Context(), id)
	var inUse *store.SupplierInUseError
	switch {
	case err == nil:
		http.Redirect(w, r, "/proveedores?eliminado=1", http.StatusSeeOther)
	case errors.As(err, &inUse):
		http.Redirect(w, r, fmt.Sprintf("/proveedores/%d?eliminar=bloqueado#no-se-puede-eliminar", id), http.StatusSeeOther)
	case errors.Is(err, sql.ErrNoRows):
		http.NotFound(w, r)
	default:
		serverError(w, r, err)
	}
}

// SetInactive handles POST /proveedores/{id}/inactivar and /reactivar.
func (sp *Suppliers) SetInactive(inactive bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		err = sp.store.SetSupplierInactive(r.Context(), id, inactive)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			http.NotFound(w, r)
			return
		case err != nil:
			serverError(w, r, err)
			return
		}
		state := "activo"
		if inactive {
			state = "inactivo"
		}
		http.Redirect(w, r, fmt.Sprintf("/proveedores/%d?guardado=%s", id, state), http.StatusSeeOther)
	}
}

// supplierFieldLabels names each audited supplier column as the form labels it, in the
// form's order. bank is set for the bank data, which a vendedor's Historial leaves out.
var supplierFieldLabels = []struct {
	Column, Label string
	bank          bool
}{
	{"name", "Nombre comercial", false},
	{"rfc", "RFC", false},
	{"legal_name", "Razón social", false},
	{"street", "Calle", false},
	{"ext_number", "Número", false},
	{"neighborhood", "Colonia", false},
	{"municipality", "Municipio o alcaldía", false},
	{"state", "Estado", false},
	{"postal_code", "Código postal", false},
	{"tax_regime", "Régimen fiscal", false},
	{"credit_days", "Días de crédito", false},
	{"currency", "Moneda", false},
	{"payment_terms", "Condiciones de pago", false},
	{"lead_time", "Tiempo de entrega habitual", false},
	{"bank", "Banco", true},
	{"clabe", "CLABE", true},
	{"beneficiary", "Beneficiario", true},
	{"notes", "Notas", false},
	{"inactive", "Estado del proveedor", false},
}

var contactFieldLabels = []struct{ Column, Label string }{
	{"name", "Nombre"},
	{"role", "Puesto"},
	{"phone", "Teléfono"},
	{"email", "Email"},
	{"receives_po", "Recibe O.C."},
}

// history builds supplier id's Historial, newest first, from the trail of its row, its
// contacts and its familias (store.SupplierAuditLog). The rows one save wrote make one
// entry: SQLite has no transaction id, so rows by the same user written within
// sameSaveWindow of each other count as one save. showBank is false for a vendedor,
// whose Historial leaves the bank data out.
func (sp *Suppliers) history(ctx context.Context, id int64, showBank bool) ([]views.CustomerHistoryEntry, error) {
	entries, err := sp.store.SupplierAuditLog(ctx, id)
	if err != nil {
		return nil, err
	}
	var history []views.CustomerHistoryEntry
	var prev store.AuditEntry // the last row that made or joined an entry
	created := false
	for _, e := range entries {
		var changes []views.CustomerFieldChange
		isCreation := false
		switch e.TableName {
		case "suppliers":
			switch e.Op {
			case "insert":
				isCreation = true
			case "update":
				changed := changedSet(e)
				for _, f := range supplierFieldLabels {
					if changed[f.Column] && (showBank || !f.bank) {
						changes = append(changes, views.CustomerFieldChange{
							Label: f.Label,
							Old:   supplierFieldText(f.Column, e.OldValues[f.Column]),
							New:   supplierFieldText(f.Column, e.NewValues[f.Column]),
						})
					}
				}
			}
		case "supplier_contacts":
			changes = contactChanges(e)
		case "product_families":
			changes = familiaChanges(e, id)
		}
		if !isCreation && len(changes) == 0 {
			continue
		}
		joins := len(history) > 0 && sameSave(prev, e)
		prev = e
		if isCreation {
			created = true
			if joins {
				// The contacts and familias saved with it are part of its creation.
				history = history[:len(history)-1]
			}
			history = append(history, views.CustomerHistoryEntry{Who: auditWho(e), At: e.At, Created: true})
			continue
		}
		if joins {
			// Rows come newest first; the save wrote its own fields, then contacts, then
			// familias, and the entry lists them in that order.
			last := &history[len(history)-1]
			if !last.Created {
				last.Changes = append(changes, last.Changes...)
				last.At = e.At
			}
			continue
		}
		history = append(history, views.CustomerHistoryEntry{Who: auditWho(e), At: e.At, Changes: changes})
	}
	if !created {
		at, err := sp.store.SupplierCreatedAt(ctx, id)
		if err != nil {
			return nil, err
		}
		history = append(history, views.CustomerHistoryEntry{At: at, Created: true})
	}
	return history, nil
}

// sameSaveWindow is how close in time two audit rows by the same user must be to count
// as one save.
const sameSaveWindow = 2 * time.Second

// sameSave reports whether audit rows a and b look like one save: the same user (or
// both unattributed) and written within sameSaveWindow.
func sameSave(a, b store.AuditEntry) bool {
	if auditWho(a) != auditWho(b) || a.Source != b.Source {
		return false
	}
	ta, errA := time.Parse(time.RFC3339Nano, a.At)
	tb, errB := time.Parse(time.RFC3339Nano, b.At)
	if errA != nil || errB != nil {
		return a.At == b.At
	}
	d := ta.Sub(tb)
	return d < sameSaveWindow && d > -sameSaveWindow
}

func changedSet(e store.AuditEntry) map[string]bool {
	changed := map[string]bool{}
	for _, k := range e.ChangedFields() {
		changed[k] = true
	}
	return changed
}

// contactChanges is a contact's audit row as Historial lines.
func contactChanges(e store.AuditEntry) []views.CustomerFieldChange {
	name := func(v map[string]any) string { return supplierFieldText("name", v["name"]) }
	switch e.Op {
	case "insert":
		return []views.CustomerFieldChange{{Text: "Contacto agregado: " + name(e.NewValues)}}
	case "delete":
		return []views.CustomerFieldChange{{Text: "Contacto quitado: " + name(e.OldValues)}}
	}
	changed := changedSet(e)
	var out []views.CustomerFieldChange
	for _, f := range contactFieldLabels {
		if changed[f.Column] {
			out = append(out, views.CustomerFieldChange{
				Label: "Contacto " + name(e.OldValues) + ", " + f.Label,
				Old:   supplierFieldText(f.Column, e.OldValues[f.Column]),
				New:   supplierFieldText(f.Column, e.NewValues[f.Column]),
			})
		}
	}
	return out
}

// familiaChanges is a familia's audit row as supplier id's Historial line: whether the
// familia became or stopped being one it supplies.
func familiaChanges(e store.AuditEntry, id int64) []views.CustomerFieldChange {
	is := func(v map[string]any) bool {
		n, ok := v["default_supplier_id"].(float64)
		return ok && int64(n) == id
	}
	before, after := is(e.OldValues), is(e.NewValues)
	switch {
	case after && !before:
		return []views.CustomerFieldChange{{Text: "Familia que surte agregada: " + supplierFieldText("name", e.NewValues["name"])}}
	case before && !after:
		vals := e.NewValues
		if vals == nil {
			vals = e.OldValues
		}
		return []views.CustomerFieldChange{{Text: "Familia que surte quitada: " + supplierFieldText("name", vals["name"])}}
	}
	return nil
}

// supplierFieldText is how one audited supplier or contact value reads in the Historial.
func supplierFieldText(column string, v any) string {
	text := ""
	if v != nil {
		text = strings.TrimSpace(fmt.Sprint(v))
	}
	switch column {
	case "inactive":
		if text == "1" {
			return "Inactivo"
		}
		return "Activo"
	case "receives_po":
		if text == "1" {
			return "Sí"
		}
		return "No"
	}
	if text == "" {
		return "(vacío)"
	}
	if column == "tax_regime" {
		return satText(store.TaxRegimes, text)
	}
	return text
}
