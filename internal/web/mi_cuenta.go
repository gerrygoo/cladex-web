package web

import (
	"net/http"

	"golang.org/x/crypto/bcrypt"

	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

// ChangePasswordForm is the /mi-cuenta form. Field-level errors key by the input's
// name attribute, per the plan's "one struct per form with Validate()" convention.
type ChangePasswordForm struct {
	CurrentPassword string
	NewPassword     string
	ConfirmPassword string
}

func (f ChangePasswordForm) Validate() map[string]string {
	errs := map[string]string{}
	if f.CurrentPassword == "" {
		errs["current_password"] = "La contraseña actual es obligatoria."
	}
	if len(f.NewPassword) < 8 {
		errs["new_password"] = "La contraseña nueva debe tener al menos 8 caracteres."
	}
	if f.NewPassword != f.ConfirmPassword {
		errs["confirm_password"] = "Las contraseñas no coinciden."
	}
	return errs
}

func (a *Auth) MiCuentaPage(w http.ResponseWriter, r *http.Request) {
	a.renderMiCuenta(w, r, views.MiCuentaView{ContactSaved: r.URL.Query().Get("avisos") == "1"})
}

// renderMiCuenta draws /mi-cuenta with the user's saved Avisos.
func (a *Auth) renderMiCuenta(w http.ResponseWriter, r *http.Request, m views.MiCuentaView) {
	ctx := r.Context()
	user, _ := UserFromContext(ctx)
	c, err := a.store.UserContactByID(ctx, user.ID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	m.Contact = c
	if len(m.PasswordErrors) > 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	views.MiCuenta(m, navUserView(user)).Render(ctx, w)
}

func (a *Auth) MiCuentaSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authUser, _ := UserFromContext(ctx)

	if err := r.ParseForm(); err != nil {
		http.Error(w, "solicitud inválida", http.StatusBadRequest)
		return
	}
	form := ChangePasswordForm{
		CurrentPassword: r.FormValue("current_password"),
		NewPassword:     r.FormValue("new_password"),
		ConfirmPassword: r.FormValue("confirm_password"),
	}
	fieldErrors := form.Validate()

	var user *store.User
	if len(fieldErrors) == 0 || fieldErrors["current_password"] == "" {
		var err error
		user, err = a.store.UserByID(ctx, authUser.ID)
		if err != nil {
			serverError(w, r, err)
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(form.CurrentPassword)) != nil {
			fieldErrors["current_password"] = "Contraseña actual incorrecta."
		}
	}

	if len(fieldErrors) > 0 {
		a.renderMiCuenta(w, r, views.MiCuentaView{PasswordErrors: fieldErrors})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(form.NewPassword), 12)
	if err != nil {
		serverError(w, r, err)
		return
	}
	if err := a.store.UpdatePassword(ctx, user.ID, string(hash)); err != nil {
		serverError(w, r, err)
		return
	}

	a.renderMiCuenta(w, r, views.MiCuentaView{Success: "Contraseña actualizada."})
}
