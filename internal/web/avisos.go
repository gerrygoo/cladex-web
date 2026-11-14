package web

import (
	"net/http"
	"net/mail"
	"regexp"
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

// parseContactForm reads Mi cuenta's Avisos form. Errors key by the input's name. Each
// user sets their own contact, channels and avisos, and only they do (#69).
func parseContactForm(r *http.Request) (store.UserContact, map[string]string) {
	c := store.UserContact{
		Email:          strings.TrimSpace(r.FormValue("email")),
		WhatsApp:       strings.TrimSpace(r.FormValue("whatsapp")),
		NotifyEmail:    r.FormValue("notify_email") == "1",
		NotifyWhatsApp: r.FormValue("notify_whatsapp") == "1",
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
	if c.NotifyEmail && c.Email == "" {
		errs["email"] = "Escribe el correo para recibir avisos por correo."
	}
	if c.NotifyWhatsApp && c.WhatsApp == "" {
		errs["whatsapp"] = "Escribe el número para recibir avisos por WhatsApp."
	}
	return c, errs
}

// MiCuentaAvisos handles POST /mi-cuenta/avisos: where and about what the user wants to
// be told. Avisos only an admin may have (store.AvisoKind.AdminOnly) are left as they are
// for anyone else.
func (a *Auth) MiCuentaAvisos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, _ := UserFromContext(ctx)
	c, errs := parseContactForm(r)
	kinds := avisosFromForm(r)
	if len(errs) > 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
		views.MiCuenta(views.MiCuentaView{Contact: c, Avisos: kinds, ContactErrors: errs}, navUserView(user)).Render(ctx, w)
		return
	}
	if err := a.store.SetUserContact(ctx, user.ID, c); err != nil {
		serverError(w, r, err)
		return
	}
	if err := a.store.SetUserAvisos(ctx, user.ID, user.Role, r.Form["aviso"]); err != nil {
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/mi-cuenta?avisos=1#avisos", http.StatusSeeOther)
}

// avisosFromForm reads the aviso checkboxes of a submitted Avisos form.
func avisosFromForm(r *http.Request) map[string]bool {
	on := map[string]bool{}
	for _, k := range r.Form["aviso"] {
		on[k] = true
	}
	return on
}
