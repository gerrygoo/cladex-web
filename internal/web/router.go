package web

import (
	"fmt"
	"io/fs"
	"net/http"
	"time"

	"github.com/gerrygoo/cladex-web/internal/pdf"
	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

const sampleTypst = `
= Cladex

Hello world — Typst render pipeline is up.
`

// NewMux builds the application router. buildSHA is surfaced on /healthz. cookieSecure
// controls the session cookie's Secure flag — true in production (behind the
// TLS-terminating proxy), false for local plain-HTTP dev.
func NewMux(buildSHA string, staticFS fs.FS, db *store.Store, cookieSecure bool) http.Handler {
	mux := http.NewServeMux()
	auth := NewAuth(db, cookieSecure)
	products := NewProducts(db)
	customers := NewCustomers(db)
	users := NewUsers(db)
	settings := NewSettings(db)

	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticFS)))

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "ok %s", buildSHA)
	})

	mux.HandleFunc("GET /login", auth.LoginPage)
	mux.HandleFunc("POST /login", auth.LoginSubmit)
	mux.HandleFunc("POST /logout", auth.Logout)
	mux.Handle("GET /mi-cuenta", auth.RequireAuth(http.HandlerFunc(auth.MiCuentaPage)))
	mux.Handle("POST /mi-cuenta", auth.RequireAuth(http.HandlerFunc(auth.MiCuentaSubmit)))

	mux.Handle("GET /productos", auth.RequireAuth(http.HandlerFunc(products.List)))
	mux.Handle("GET /productos/nuevo", auth.RequireAuth(http.HandlerFunc(products.NewPage)))
	mux.Handle("POST /productos/nuevo", auth.RequireAuth(http.HandlerFunc(products.Create)))
	mux.Handle("GET /productos/{id}", auth.RequireAuth(http.HandlerFunc(products.EditPage)))
	mux.Handle("POST /productos/{id}", auth.RequireAuth(http.HandlerFunc(products.Update)))
	mux.Handle("POST /productos/{id}/eliminar", auth.RequireAuth(http.HandlerFunc(products.Delete)))

	mux.Handle("GET /clientes", auth.RequireAuth(http.HandlerFunc(customers.List)))
	mux.Handle("GET /clientes/nuevo", auth.RequireAuth(http.HandlerFunc(customers.NewPage)))
	mux.Handle("POST /clientes/nuevo", auth.RequireAuth(http.HandlerFunc(customers.Create)))
	mux.Handle("GET /clientes/{id}", auth.RequireAuth(http.HandlerFunc(customers.EditPage)))
	mux.Handle("POST /clientes/{id}", auth.RequireAuth(http.HandlerFunc(customers.Update)))
	mux.Handle("POST /clientes/{id}/eliminar", auth.RequireAuth(http.HandlerFunc(customers.Delete)))

	mux.Handle("GET /usuarios", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(users.List))))
	mux.Handle("POST /usuarios/{id}/rol", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(users.SetRole))))
	mux.Handle("POST /usuarios/{id}/deshabilitar", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(users.SetDisabled(true)))))
	mux.Handle("POST /usuarios/{id}/habilitar", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(users.SetDisabled(false)))))

	mux.Handle("GET /ajustes", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(settings.Page))))
	mux.Handle("POST /ajustes", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(settings.Submit))))

	mux.Handle("GET /{$}", auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count, err := db.QuoteCount(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		user, _ := UserFromContext(r.Context())
		views.Home(buildSHA, count, navUserView(user)).Render(r.Context(), w)
	})))

	mux.HandleFunc("GET /saludo", func(w http.ResponseWriter, r *http.Request) {
		mensaje := fmt.Sprintf("Hola — htmx funciona. Hora del servidor: %s", time.Now().Format(time.TimeOnly))
		views.Saludo(mensaje).Render(r.Context(), w)
	})

	mux.HandleFunc("GET /sample.pdf", func(w http.ResponseWriter, r *http.Request) {
		bytes, err := pdf.Render(r.Context(), sampleTypst)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Write(bytes)
	})

	csrf := http.NewCrossOriginProtection()
	return csrf.Handler(mux)
}
