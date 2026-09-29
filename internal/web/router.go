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

// Build identifies the running binary: the commit it was built from and when. A zero
// Time means a local dev build.
type Build struct {
	SHA  string
	Time time.Time
}

// NewMux builds the application router. build.SHA is surfaced on /healthz; the home
// page shows build.Time instead. cookieSecure
// controls the session cookie's Secure flag — true in production (behind the
// TLS-terminating proxy), false for local plain-HTTP dev. logger receives the access
// log; pass nil to discard it, as the handler tests do. guide is the pre-rendered
// user guide served under /ayuda.
func NewMux(build Build, staticFS fs.FS, guide *guia.Guide, db *store.Store, cookieSecure bool, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	mux := http.NewServeMux()
	auth := NewAuth(db, cookieSecure)
	products := NewProducts(db)
	customers := NewCustomers(db)
	quotes := NewQuotes(db, logger)
	users := NewUsers(db)
	settings := NewSettings(db)
	units := NewUnits(db)
	help := NewHelp(guide)

	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticFS)))

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "ok %s", build.SHA)
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
	mux.Handle("POST /productos/{id}/materiales", auth.RequireAuth(http.HandlerFunc(products.AddMaterial)))
	mux.Handle("POST /productos/{id}/materiales/{mid}/eliminar", auth.RequireAuth(http.HandlerFunc(products.DeleteMaterial)))

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
	mux.Handle("POST /cotizaciones/{folio}/etapa", auth.RequireAuth(http.HandlerFunc(quotes.Etapa)))
	mux.Handle("POST /cotizaciones/{folio}/comentarios", auth.RequireAuth(http.HandlerFunc(quotes.Comentar)))
	mux.Handle("GET /cotizaciones/{folio}/pdf", auth.RequireAuth(http.HandlerFunc(quotes.PDF)))

	mux.Handle("GET /usuarios", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(users.List))))
	mux.Handle("POST /usuarios/{id}/rol", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(users.SetRole))))
	mux.Handle("POST /usuarios/{id}/deshabilitar", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(users.SetDisabled(true)))))
	mux.Handle("POST /usuarios/{id}/habilitar", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(users.SetDisabled(false)))))

	mux.Handle("GET /ajustes", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(settings.Page))))
	mux.Handle("POST /ajustes/margenes", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(settings.CreateMargen))))
	mux.Handle("POST /ajustes/margenes/{id}", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(settings.UpdateMargen))))
	mux.Handle("POST /ajustes/margenes/{id}/predeterminado", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(settings.PredeterminarMargen))))
	mux.Handle("POST /ajustes/margenes/{id}/retirar", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(settings.RetirarMargen))))
	mux.Handle("POST /ajustes/margenes/{id}/restaurar", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(settings.RestaurarMargen))))
	mux.Handle("POST /ajustes/materiales", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(settings.CreateMaterial))))
	mux.Handle("POST /ajustes/materiales/{id}", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(settings.UpdateMaterial))))

	mux.Handle("GET /unidades", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(units.List))))
	mux.Handle("POST /unidades", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(units.Create))))

	mux.Handle("GET /ayuda", auth.RequireAuth(http.HandlerFunc(help.Page)))
	mux.Handle("GET /ayuda/{pagina}", auth.RequireAuth(http.HandlerFunc(help.Page)))

	mux.Handle("GET /{$}", auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ov, err := db.QuoteOverview(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		user, _ := UserFromContext(r.Context())
		views.Home(build.Time, time.Now(), build.SHA, ov, navUserView(user)).Render(r.Context(), w)
	})))

	mux.HandleFunc("GET /saludo", func(w http.ResponseWriter, r *http.Request) {
		mensaje := fmt.Sprintf("Hola — htmx funciona. Hora del servidor: %s", time.Now().Format(time.TimeOnly))
		views.Saludo(mensaje).Render(r.Context(), w)
	})

	mux.HandleFunc("GET /sample.pdf", func(w http.ResponseWriter, r *http.Request) {
		bytes, err := pdf.Render(r.Context(), sampleTypst, time.Now())
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
