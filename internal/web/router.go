package web

import (
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/gerrygoo/cladex-web/internal/appenv"
	"github.com/gerrygoo/cladex-web/internal/facturacion"
	"github.com/gerrygoo/cladex-web/internal/facturoporti"
	"github.com/gerrygoo/cladex-web/internal/guia"
	"github.com/gerrygoo/cladex-web/internal/pdf"
	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

const sampleTypst = `
= Cladex

Hello world — Typst render pipeline is up.
`

// Build identifies the running binary and instance: the commit it was built from and
// when (a zero Time means a local dev build), which instance it is (production, staging,
// development; see internal/appenv), and whether facturación is set up.
type Build struct {
	SHA         string
	Time        time.Time
	Env         appenv.Env
	Facturacion facturoporti.Status
}

// NewMux builds the application router. build.SHA is surfaced on /healthz; the home
// page shows build.Time instead. cookieSecure
// controls the session cookie's Secure flag — true in production (behind the
// TLS-terminating proxy), false for local plain-HTTP dev. logger receives the access
// log; pass nil to discard it, as the handler tests do. guide is the pre-rendered
// user guide served under /ayuda.
//
// fact is facturación (internal/facturacion); nil builds one that is off, as when the
// Facturoporti secrets aren't installed.
func NewMux(build Build, staticFS fs.FS, guide *guia.Guide, db *store.Store, cookieSecure bool, logger *slog.Logger, fact *facturacion.Service) http.Handler {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	if fact == nil {
		fact = facturacion.New(db, nil, build.Facturacion)
	}
	views.EnvironmentBanner = build.Env.Banner()
	mux := http.NewServeMux()
	auth := NewAuth(db, cookieSecure)
	products := NewProducts(db)
	customers := NewCustomers(db)
	quotes := NewQuotes(db, logger)
	quotes.watermark = build.Env.Watermark()
	projects := NewProjects(db)
	projects.fact = fact
	facturas := NewFacturas(db, fact, logger)
	users := NewUsers(db)
	settings := NewSettings(db)
	settings.facturacion = build.Facturacion
	units := NewUnits(db)
	familias := NewFamilias(db)
	documentos := NewDocumentos(db)
	help := NewHelp(guide)

	static, err := newStaticHandler(staticFS)
	if err != nil {
		panic("static assets: " + err.Error())
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", static))

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
	mux.Handle("GET /cotizaciones/{folio}/pdf", auth.RequireAuth(http.HandlerFunc(quotes.PDF)))
	mux.Handle("POST /cotizaciones/{folio}/proveedor", auth.RequireAuth(http.HandlerFunc(quotes.Proveedor)))
	mux.Handle("GET /cotizaciones/{folio}/proveedor/{id}", auth.RequireAuth(http.HandlerFunc(quotes.ProveedorArchivo)))
	mux.Handle("POST /cotizaciones/{folio}/proveedor/{id}/eliminar", auth.RequireAuth(http.HandlerFunc(quotes.ProveedorEliminar)))

	mux.Handle("GET /proyectos", auth.RequireAuth(http.HandlerFunc(projects.List)))
	mux.Handle("GET /proyectos/{folio}", auth.RequireAuth(http.HandlerFunc(projects.Page)))
	mux.Handle("POST /proyectos/{folio}/etapa", auth.RequireAuth(http.HandlerFunc(projects.Etapa)))
	mux.Handle("POST /proyectos/{folio}/seguimiento", auth.RequireAuth(http.HandlerFunc(projects.Seguimiento)))
	mux.Handle("GET /pronostico", auth.RequireAuth(http.HandlerFunc(projects.Pronostico)))
	mux.Handle("POST /proyectos/{folio}/comentarios", auth.RequireAuth(http.HandlerFunc(projects.Comentar)))
	mux.Handle("POST /proyectos/{folio}/oc", auth.RequireAuth(http.HandlerFunc(projects.OC)))
	mux.Handle("POST /proyectos/{folio}/factura", auth.RequireAuth(http.HandlerFunc(projects.Factura)))
	mux.Handle("POST /proyectos/{folio}/pago", auth.RequireAuth(http.HandlerFunc(projects.Pago)))
	mux.Handle("GET /proyectos/{folio}/archivos/{id}", auth.RequireAuth(http.HandlerFunc(projects.Archivo)))
	mux.Handle("POST /proyectos/{folio}/proveedor", auth.RequireAuth(http.HandlerFunc(projects.Proveedor)))
	mux.Handle("POST /proyectos/{folio}/perder", auth.RequireAuth(http.HandlerFunc(projects.Perder)))
	mux.Handle("POST /proyectos/{folio}/reabrir", auth.RequireAuth(http.HandlerFunc(projects.Reabrir)))

	mux.Handle("GET /facturas", auth.RequireAuth(http.HandlerFunc(facturas.List)))
	mux.Handle("GET /facturas/{id}", auth.RequireAuth(http.HandlerFunc(facturas.Page)))
	mux.Handle("GET /facturas/{id}/xml", auth.RequireAuth(http.HandlerFunc(facturas.XML)))
	mux.Handle("GET /facturas/{id}/pdf", auth.RequireAuth(http.HandlerFunc(facturas.PDF)))
	admin := func(h http.HandlerFunc) http.Handler { return auth.RequireAuth(auth.RequireAdmin(h)) }
	mux.Handle("POST /proyectos/{folio}/factura/nueva", admin(facturas.Crear))
	mux.Handle("POST /facturas/{id}", admin(facturas.Guardar))
	mux.Handle("POST /facturas/{id}/validar", admin(facturas.Validar))
	mux.Handle("POST /facturas/{id}/editar", admin(facturas.Editar))
	mux.Handle("POST /facturas/{id}/timbrar", admin(facturas.Timbrar))
	mux.Handle("POST /facturas/{id}/cancelar", admin(facturas.Cancelar))
	mux.Handle("POST /facturas/{id}/consultar", admin(facturas.Consultar))
	mux.Handle("POST /facturas/{id}/sustituir", admin(facturas.Sustituir))
	mux.Handle("POST /facturas/{id}/pago", admin(facturas.Pago))

	mux.Handle("GET /documentos", auth.RequireAuth(http.HandlerFunc(documentos.List)))
	mux.Handle("GET /documentos/{id}", auth.RequireAuth(http.HandlerFunc(documentos.Archivo)))
	mux.Handle("POST /documentos", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(documentos.Subir))))
	mux.Handle("POST /documentos/{id}/eliminar", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(documentos.Eliminar))))

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
	mux.Handle("POST /ajustes/emisor", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(settings.Emisor))))
	mux.Handle("POST /ajustes/materiales", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(settings.CreateMaterial))))
	mux.Handle("POST /ajustes/materiales/{id}", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(settings.UpdateMaterial))))

	mux.Handle("GET /unidades", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(units.List))))
	mux.Handle("POST /unidades", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(units.Create))))
	mux.Handle("POST /unidades/{id}/sat", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(units.SetSAT))))

	mux.Handle("GET /familias", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(familias.List))))
	mux.Handle("POST /familias", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(familias.Create))))
	mux.Handle("GET /familias/{id}", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(familias.EditPage))))
	mux.Handle("POST /familias/{id}", auth.RequireAuth(auth.RequireAdmin(http.HandlerFunc(familias.Update))))

	mux.Handle("GET /ayuda", auth.RequireAuth(http.HandlerFunc(help.Page)))
	mux.Handle("GET /ayuda/{pagina}", auth.RequireAuth(http.HandlerFunc(help.Page)))

	mux.Handle("GET /{$}", auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dash, err := db.SalesDashboard(r.Context(), today(r))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		user, _ := UserFromContext(r.Context())
		views.Home(build.Time, time.Now(), build.SHA, dash, navUserView(user)).Render(r.Context(), w)
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
