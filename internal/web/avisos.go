package web

import (
	"net/http"
	"net/mail"
	"regexp"
	"strconv"
	"strings"

	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

// e164 is a phone number with its country code, as WhatsApp addresses it.
var e164 = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)

// normalizeWhatsApp reads a number as people type it ("33 1234 5678", "(33) 1234-5678",
// "+52 1 33 …"): ten digits are a Mexican number, anything else needs its "+" and
// country code. It reports false when the result isn't a valid number.
func normalizeWhatsApp(raw string) (string, bool) {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, raw)
	switch {
	case strings.HasPrefix(strings.TrimSpace(raw), "+"):
		digits = "+" + digits
	case len(digits) == 10:
		digits = "+52" + digits
	}
	return digits, e164.MatchString(digits)
}

// parseContactForm reads the Avisos form, shared by Mi cuenta and an admin's edit of
// another user. Errors key by the input's name. Only an admin turns avisos on or off: for
// anyone else, saved carries the toggles as they are, and the form's are ignored. Their
// clearing an address still turns its avisos off (SetUserContact).
func parseContactForm(r *http.Request, saved *store.UserContact) (store.UserContact, map[string]string) {
	c := store.UserContact{
		Email:          strings.TrimSpace(r.FormValue("email")),
		WhatsApp:       strings.TrimSpace(r.FormValue("whatsapp")),
		NotifyEmail:    r.FormValue("notify_email") == "1",
		NotifyWhatsApp: r.FormValue("notify_whatsapp") == "1",
	}
	if saved != nil {
		c.NotifyEmail, c.NotifyWhatsApp = saved.NotifyEmail, saved.NotifyWhatsApp
	}
	errs := map[string]string{}
	if c.Email != "" {
		if a, err := mail.ParseAddress(c.Email); err != nil || a.Name != "" {
			errs["email"] = "Correo inválido; escribe solo la dirección, p. ej. ana@cladex.com.mx."
		}
	}
	if c.WhatsApp != "" {
		n, ok := normalizeWhatsApp(c.WhatsApp)
		if !ok {
			errs["whatsapp"] = "Número de WhatsApp inválido: escribe los 10 dígitos, p. ej. 33 1234 5678, o con lada de país (+1 …)."
		} else {
			c.WhatsApp = n
		}
	}
	if saved != nil {
		return c, errs
	}
	if c.NotifyEmail && c.Email == "" {
		errs["email"] = "Escribe el correo para recibir avisos por correo."
	}
	if c.NotifyWhatsApp && c.WhatsApp == "" {
		errs["whatsapp"] = "Escribe el número para recibir avisos por WhatsApp."
	}
	return c, errs
}

// MiCuentaAvisos handles POST /mi-cuenta/avisos: where the user wants to be told things.
func (a *Auth) MiCuentaAvisos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, _ := UserFromContext(ctx)
	var saved *store.UserContact
	if user.Role != "admin" {
		s, err := a.store.UserContactByID(ctx, user.ID)
		if err != nil {
			serverError(w, r, err)
			return
		}
		saved = &s
	}
	c, errs := parseContactForm(r, saved)
	if len(errs) > 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
		views.MiCuenta(views.MiCuentaView{Contact: c, ContactErrors: errs}, navUserView(user)).Render(ctx, w)
		return
	}
	if err := a.store.SetUserContact(ctx, user.ID, c); err != nil {
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/mi-cuenta?avisos=1#avisos", http.StatusSeeOther)
}

// AvisosPage handles GET /usuarios/{id}/avisos: an admin edits another user's Avisos, as
// they would their own.
func (u *Users) AvisosPage(w http.ResponseWriter, r *http.Request) {
	u.renderAvisos(w, r, nil, nil)
}

// Avisos handles POST /usuarios/{id}/avisos.
func (u *Users) Avisos(w http.ResponseWriter, r *http.Request) {
	c, errs := parseContactForm(r, nil)
	if len(errs) > 0 {
		u.renderAvisos(w, r, &c, errs)
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	target, err := u.store.UserByID(r.Context(), id)
	if err != nil {
		serverError(w, r, err)
		return
	}
	if target == nil {
		http.NotFound(w, r)
		return
	}
	if err := u.store.SetUserContact(r.Context(), id, c); err != nil {
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/usuarios/"+strconv.FormatInt(id, 10)+"/avisos?guardado=1", http.StatusSeeOther)
}

// renderAvisos draws an admin's edit of a user's Avisos; contact and errs are a refused
// submission, nil to show what is saved.
func (u *Users) renderAvisos(w http.ResponseWriter, r *http.Request, contact *store.UserContact, errs map[string]string) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	target, err := u.store.UserByID(ctx, id)
	if err != nil {
		serverError(w, r, err)
		return
	}
	if target == nil {
		http.NotFound(w, r)
		return
	}
	if contact == nil {
		c, err := u.store.UserContactByID(ctx, id)
		if err != nil {
			serverError(w, r, err)
			return
		}
		contact = &c
	}
	msg := ""
	if r.URL.Query().Get("guardado") == "1" {
		msg = "Avisos guardados."
	}
	if len(errs) > 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	user, _ := UserFromContext(ctx)
	views.UsuarioAvisos(*target, *contact, errs, msg, navUserView(user)).Render(ctx, w)
}
