package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

// Leads holds the /leads handlers (#82, slice L2 of
// docs/planning/leads-y-listas-de-precios.md): people who ask for prices before there's
// a proyecto. Everyone sees and works every lead, as with proyectos.
type Leads struct {
	store *store.Store
}

func NewLeads(s *store.Store) *Leads {
	return &Leads{store: s}
}

// List renders /leads. ?estado= abierto (the default), perdido, convertido or todos;
// ?mios=1 keeps only the viewer's own.
func (h *Leads) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, _ := UserFromContext(ctx)
	f := views.LeadFilter{Status: store.LeadAbierto, Mine: r.URL.Query().Get("mios") == "1"}
	switch st := r.URL.Query().Get("estado"); st {
	case store.LeadPerdido, store.LeadConvertido:
		f.Status = st
	case "todos":
		f.Status = ""
	}
	var owner int64
	if f.Mine {
		owner = user.ID
	}
	leads, err := h.store.ListLeads(ctx, f.Status, owner)
	if err != nil {
		serverError(w, r, err)
		return
	}
	views.LeadsList(leads, f, today(r), navUserView(user)).Render(ctx, w)
}

// parseLeadForm reads the lead form into what it shows and what the store takes.
func parseLeadForm(r *http.Request) (views.LeadFormValues, store.LeadInput) {
	v := views.LeadFormValues{
		ContactName:  strings.TrimSpace(r.FormValue("contact_name")),
		CustomerID:   r.FormValue("customer_id"),
		Company:      strings.TrimSpace(r.FormValue("company")),
		Phone:        strings.TrimSpace(r.FormValue("phone")),
		Email:        strings.TrimSpace(r.FormValue("email")),
		UserID:       r.FormValue("user_id"),
		NextFollowUp: strings.TrimSpace(r.FormValue("next_followup")),
		Note:         strings.TrimSpace(r.FormValue("note")),
	}
	in := store.LeadInput{
		ContactName: v.ContactName, Company: v.Company, Phone: v.Phone, Email: v.Email,
		NextFollowUp: v.NextFollowUp,
	}
	if id, err := strconv.ParseInt(v.CustomerID, 10, 64); err == nil && id > 0 {
		in.CustomerID = &id
	}
	in.UserID, _ = strconv.ParseInt(v.UserID, 10, 64)
	return v, in
}

// checkLeadRefs adds to errs what only the database can tell: the customer and the
// vendedor exist (and the vendedor is active).
func (h *Leads) checkLeadRefs(r *http.Request, in store.LeadInput, errs map[string]string) (map[string]string, error) {
	ctx := r.Context()
	if errs == nil {
		errs = map[string]string{}
	}
	if in.CustomerID != nil {
		c, err := h.store.CustomerByID(ctx, *in.CustomerID)
		if err != nil {
			return nil, err
		}
		if c == nil {
			errs["customer_id"] = "Ese cliente no existe."
		}
	}
	if in.UserID != 0 {
		users, err := h.store.ActiveUsers(ctx)
		if err != nil {
			return nil, err
		}
		found := false
		for _, u := range users {
			found = found || u.ID == in.UserID
		}
		if !found {
			errs["user_id"] = "Elige el vendedor responsable."
		}
	}
	if utf8.RuneCountInString(r.FormValue("note")) > maxCommentLen {
		errs["note"] = "El comentario no puede pasar de 2000 caracteres."
	}
	if len(errs) == 0 {
		return nil, nil
	}
	return errs, nil
}

func (h *Leads) formChoices(r *http.Request) ([]store.Customer, []store.User, error) {
	customers, err := h.store.ListCustomers(r.Context(), "", "name", "asc", nil)
	if err != nil {
		return nil, nil, err
	}
	users, err := h.store.ActiveUsers(r.Context())
	return customers, users, err
}

// NewPage renders /leads/nuevo, with the viewer as the vendedor and, with ?cliente=,
// that customer chosen.
func (h *Leads) NewPage(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	v := views.LeadFormValues{UserID: strconv.FormatInt(user.ID, 10), CustomerID: r.URL.Query().Get("cliente")}
	h.renderNew(w, r, v, nil, http.StatusOK)
}

func (h *Leads) renderNew(w http.ResponseWriter, r *http.Request, v views.LeadFormValues, errs map[string]string, status int) {
	customers, users, err := h.formChoices(r)
	if err != nil {
		serverError(w, r, err)
		return
	}
	user, _ := UserFromContext(r.Context())
	w.WriteHeader(status)
	views.LeadNew(v, customers, users, errs, navUserView(user)).Render(r.Context(), w)
}

// Create handles POST /leads.
func (h *Leads) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v, in := parseLeadForm(r)
	errs, err := h.checkLeadRefs(r, in, in.Errors())
	if err != nil {
		serverError(w, r, err)
		return
	}
	if len(errs) > 0 {
		h.renderNew(w, r, v, errs, http.StatusUnprocessableEntity)
		return
	}
	user, _ := UserFromContext(ctx)
	id, err := h.store.CreateLead(ctx, in, v.Note, user.ID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/leads/"+strconv.FormatInt(id, 10)+"?guardado=1", http.StatusSeeOther)
}

// loadLead finds the lead named by {id}, writing a 404 when there's none.
func (h *Leads) loadLead(w http.ResponseWriter, r *http.Request) *store.Lead {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return nil
	}
	l, err := h.store.LeadByID(r.Context(), id)
	if err != nil {
		serverError(w, r, err)
		return nil
	}
	if l == nil {
		http.NotFound(w, r)
	}
	return l
}

func leadFormValues(l store.Lead) views.LeadFormValues {
	v := views.LeadFormValues{
		ContactName: l.ContactName, Company: l.Company, Phone: l.Phone, Email: l.Email,
		UserID: strconv.FormatInt(l.UserID, 10), NextFollowUp: l.NextFollowUp,
	}
	if l.CustomerID != nil {
		v.CustomerID = strconv.FormatInt(*l.CustomerID, 10)
	}
	return v
}

// Page renders /leads/{id}.
func (h *Leads) Page(w http.ResponseWriter, r *http.Request) {
	l := h.loadLead(w, r)
	if l == nil {
		return
	}
	success := ""
	switch {
	case r.URL.Query().Get("guardado") == "1":
		success = "Lead guardado."
	case r.URL.Query().Get("seguimiento") == "1":
		success = "Seguimiento guardado."
	}
	h.render(w, r, *l, leadFormValues(*l), nil, success, "", http.StatusOK)
}

func (h *Leads) render(w http.ResponseWriter, r *http.Request, l store.Lead, form views.LeadFormValues, errs map[string]string, success, errMsg string, status int) {
	ctx := r.Context()
	comments, err := h.store.ListLeadComments(ctx, l.ID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	types, err := h.store.ListQuoteTypes(ctx)
	if err != nil {
		serverError(w, r, err)
		return
	}
	customers, users, err := h.formChoices(r)
	if err != nil {
		serverError(w, r, err)
		return
	}
	user, _ := UserFromContext(ctx)
	w.WriteHeader(status)
	views.LeadPage(views.LeadPageData{
		Lead: l, Comments: comments, Types: types, Customers: customers, Users: users,
		Form: form, Errors: errs, Today: today(r), Success: success, Error: errMsg,
	}, navUserView(user)).Render(ctx, w)
}

// userID is the signed-in user's id.
func userID(r *http.Request) int64 {
	user, _ := UserFromContext(r.Context())
	return user.ID
}

// Update handles POST /leads/{id}: the lead's own data.
func (h *Leads) Update(w http.ResponseWriter, r *http.Request) {
	l := h.loadLead(w, r)
	if l == nil {
		return
	}
	v, in := parseLeadForm(r)
	in.NextFollowUp = ""
	errs, err := h.checkLeadRefs(r, in, in.Errors())
	if err != nil {
		serverError(w, r, err)
		return
	}
	if len(errs) > 0 {
		h.render(w, r, *l, v, errs, "", "Revisa los datos del lead.", http.StatusUnprocessableEntity)
		return
	}
	if err := h.store.UpdateLead(r.Context(), l.ID, in, userID(r)); err != nil {
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/leads/"+strconv.FormatInt(l.ID, 10)+"?guardado=1", http.StatusSeeOther)
}

// stateError answers a change the lead can't take any more (someone else closed it).
func (h *Leads) stateError(w http.ResponseWriter, r *http.Request, l store.Lead, err error, msg string) {
	if errors.Is(err, store.ErrLeadState) || errors.Is(err, store.ErrBadTransition) {
		h.render(w, r, l, leadFormValues(l), nil, "", msg, http.StatusConflict)
		return
	}
	serverError(w, r, err)
}

// Seguimiento handles POST /leads/{id}/seguimiento: a comment, required, and the next
// seguimiento day, which must be after today (blank clears it).
func (h *Leads) Seguimiento(w http.ResponseWriter, r *http.Request) {
	l := h.loadLead(w, r)
	if l == nil {
		return
	}
	note := strings.TrimSpace(r.FormValue("note"))
	next := strings.TrimSpace(r.FormValue("proximo_seguimiento"))
	switch {
	case note == "":
		h.render(w, r, *l, leadFormValues(*l), nil, "", "Escribe un comentario: qué pasó y cuál es el siguiente paso.", http.StatusUnprocessableEntity)
		return
	case utf8.RuneCountInString(note) > maxCommentLen:
		h.render(w, r, *l, leadFormValues(*l), nil, "", "El comentario no puede pasar de 2000 caracteres.", http.StatusUnprocessableEntity)
		return
	}
	if next != "" && next != l.NextFollowUp {
		if _, err := time.Parse("2006-01-02", next); err != nil || next <= today(r) {
			h.render(w, r, *l, leadFormValues(*l), nil, "", "El próximo seguimiento debe ser después de hoy.", http.StatusUnprocessableEntity)
			return
		}
	}
	if err := h.store.FollowUpLead(r.Context(), l.ID, userID(r), &next, note); err != nil {
		h.stateError(w, r, *l, err, "Este lead ya no está abierto.")
		return
	}
	http.Redirect(w, r, "/leads/"+strconv.FormatInt(l.ID, 10)+"?seguimiento=1#historial", http.StatusSeeOther)
}

// Perdido handles POST /leads/{id}/perdido, with its reason.
func (h *Leads) Perdido(w http.ResponseWriter, r *http.Request) {
	l := h.loadLead(w, r)
	if l == nil {
		return
	}
	reason := strings.TrimSpace(r.FormValue("reason"))
	if reason == "" || utf8.RuneCountInString(reason) > maxCommentLen {
		h.render(w, r, *l, leadFormValues(*l), nil, "", "Escribe el motivo, de no más de 2000 caracteres.", http.StatusUnprocessableEntity)
		return
	}
	if err := h.store.LoseLead(r.Context(), l.ID, userID(r), reason); err != nil {
		h.stateError(w, r, *l, err, "Este lead ya no está abierto.")
		return
	}
	http.Redirect(w, r, "/leads/"+strconv.FormatInt(l.ID, 10), http.StatusSeeOther)
}

// Reabrir handles POST /leads/{id}/reabrir.
func (h *Leads) Reabrir(w http.ResponseWriter, r *http.Request) {
	l := h.loadLead(w, r)
	if l == nil {
		return
	}
	if err := h.store.ReopenLead(r.Context(), l.ID, userID(r)); err != nil {
		h.stateError(w, r, *l, err, "Solo un lead perdido se puede reabrir.")
		return
	}
	http.Redirect(w, r, "/leads/"+strconv.FormatInt(l.ID, 10), http.StatusSeeOther)
}

// Convertir handles POST /leads/{id}/convertir: creates the draft quote of the chosen
// type (and the customer, if the lead had none) and opens it.
func (h *Leads) Convertir(w http.ResponseWriter, r *http.Request) {
	l := h.loadLead(w, r)
	if l == nil {
		return
	}
	typeID, _ := strconv.ParseInt(r.FormValue("tipo"), 10, 64)
	qt, err := h.store.QuoteTypeByID(r.Context(), typeID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	if qt == nil {
		h.render(w, r, *l, leadFormValues(*l), nil, "", "Elige un tipo de cotización de la lista.", http.StatusUnprocessableEntity)
		return
	}
	q, err := h.store.ConvertLead(r.Context(), l.ID, userID(r), typeID)
	if err != nil {
		h.stateError(w, r, *l, err, "Este lead ya no está abierto.")
		return
	}
	http.Redirect(w, r, "/cotizaciones/"+q.Folio, http.StatusSeeOther)
}
