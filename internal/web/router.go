package web

import (
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/gerrygoo/cladex-web/internal/guia"
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
// TLS-terminating proxy), false for local plain-HTTP dev. logger receives the access
// log; pass nil to discard it, as the handler tests do. dataDir is the app's data
// directory (alongside the SQLite file) — issued quote PDFs are written under
// <dataDir>/quotes/, matching docs/PLAN.md's "Quote persistence" design. guide is the
// pre-rendered user guide served under /ayuda.
func NewMux(buildSHA string, staticFS fs.FS, guide *guia.Guide, db *store.Store, cookieSecure bool, logger *slog.Logger, dataDir string) http.Handler {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	mux := http.NewServeMux()
	auth := NewAuth(db, cookieSecure)
	products := NewProducts(db)
	customers := NewCustomers(db)
	quotes := NewQuotes(db, dataDir)
	users := NewUsers(db)
	settings := NewSettings(db)
	units := NewUnits(db)
	help := NewHelp(guide)

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
	mux.Handle("POST /productos/{id}/conversiones", auth.RequireAuth(http.HandlerFunc(products.CreateConversion)))
	mux.Handle("POST /productos/{id}/conversiones/{cid}/eliminar", auth.RequireAuth(http.HandlerFunc(products.DeleteConversion)))

	mux.Handle("GET /clientes", auth.RequireAuth(http.HandlerFunc(customers.List)))
	mux.Handle("GET /clientes/nuevo", auth.RequireAuth(http.HandlerFunc(customers.NewPage)))
	mux.Handle("POST /clientes/nuevo", auth.RequireAuth(http.HandlerFunc(customers.Create)))
	mux.Handle("GET /clientes/{id}", auth.RequireAuth(http.HandlerFunc(customers.EditPage)))
	mux.Handle("POST /clientes/{id}", auth.RequireAuth(http.HandlerFunc(customers.Update)))
	mux.Handle("POST /clientes/{id}/eliminar", auth.RequireAuth(http.HandlerFunc(customers.Delete)))

	mux.Handle("GET /cotizaciones", auth.RequireAuth(http.HandlerFunc(quotes.List)))
	mux.Handle("GET /cotizaciones/nueva", auth.RequireAuth(http.HandlerFunc(quotes.NewPage)))
	mux.Handle("POST /cotizaciones/nueva", auth.RequireAuth(http.HandlerFunc(quotes.Create)))
	mux.Handle("GET /cotizaciones/{folio}", auth.RequireAuth(http.HandlerFunc(quotes.Builder)))
	mux.Handle("POST /cotizaciones/{folio}/recalcular", auth.RequireAuth(http.HandlerFunc(quotes.Recalcular)))
	mux.Handle("GET /cotizaciones/{folio}/productos", auth.RequireAuth(http.HandlerFunc(quotes.BuscarProductos)))
	mux.Handle("POST /cotizaciones/{folio}/guardar", auth.RequireAuth(http.HandlerFunc(quotes.Guardar)))
	mux.Handle("POST /cotizaciones/{folio}/emitir", auth.RequireAuth(http.HandlerFunc(quotes.Emitir)))
	mux.Handle("POST /cotizaciones/{folio}/revisar", auth.RequireAuth(http.HandlerFunc(quotes.Revisar)))
	mux.Handle("GET /cotizaciones/{folio}/pdf", auth.RequireAuth(http.HandlerFunc(quotes.PDF)))

	mux.Handle("GET /usuarios", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(users.List))))
	mux.Handle("POST /usuarios/{id}/rol", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(users.SetRole))))
	mux.Handle("POST /usuarios/{id}/deshabilitar", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(users.SetDisabled(true)))))
	mux.Handle("POST /usuarios/{id}/habilitar", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(users.SetDisabled(false)))))

	mux.Handle("GET /ajustes", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(settings.Page))))
	mux.Handle("POST /ajustes", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(settings.Submit))))

	mux.Handle("GET /unidades", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(units.List))))
	mux.Handle("POST /unidades", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(units.Create))))

	mux.Handle("GET /ayuda", auth.RequireAuth(http.HandlerFunc(help.Page)))
	mux.Handle("GET /ayuda/{pagina}", auth.RequireAuth(http.HandlerFunc(help.Page)))

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

	// Outermost first: the access log has to see the status the CSRF check produces, and
	// its panic recovery has to cover everything below it.
	csrf := http.NewCrossOriginProtection()
	return AccessLog(logger)(csrf.Handler(mux))
}
