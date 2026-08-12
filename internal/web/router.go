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

	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticFS)))

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "ok %s", buildSHA)
	})

	mux.HandleFunc("GET /login", auth.LoginPage)
	mux.HandleFunc("POST /login", auth.LoginSubmit)
	mux.HandleFunc("POST /logout", auth.Logout)

	mux.Handle("GET /{$}", auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count, err := db.QuoteCount(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		user, _ := UserFromContext(r.Context())
		views.Home(buildSHA, count, &views.NavUser{Username: user.Username}).Render(r.Context(), w)
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
