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
	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

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

	// Every route outside these two wrappers is public on purpose (see TestPublicRoutes).
	authed := func(h http.HandlerFunc) http.Handler { return auth.RequireAuth(h) }
	admin := func(h http.HandlerFunc) http.Handler { return auth.RequireAuth(auth.RequireAdmin(h)) }

	mux.HandleFunc("GET /login", auth.LoginPage)
	mux.HandleFunc("POST /login", auth.LoginSubmit)
	mux.HandleFunc("POST /logout", auth.Logout)
	mux.Handle("GET /mi-cuenta", authed(auth.MiCuentaPage))
	mux.Handle("POST /mi-cuenta", authed(auth.MiCuentaSubmit))

	// A vendedor may add products but not change them once created, conversions and
	// materials included since they change the price, nor delete a customer (issue #41).
	mux.Handle("GET /productos", authed(products.List))
	mux.Handle("GET /productos/nuevo", authed(products.NewPage))
	mux.Handle("POST /productos/nuevo", authed(products.Create))
	mux.Handle("GET /productos/{id}", authed(products.EditPage))
	mux.Handle("POST /productos/{id}", admin(products.Update))
	mux.Handle("POST /productos/{id}/eliminar", admin(products.Delete))
	mux.Handle("POST /productos/{id}/conversiones", admin(products.CreateConversion))
	mux.Handle("POST /productos/{id}/conversiones/{cid}/eliminar", admin(products.DeleteConversion))
	mux.Handle("POST /productos/{id}/materiales", admin(products.AddMaterial))
	mux.Handle("POST /productos/{id}/materiales/{mid}/eliminar", admin(products.DeleteMaterial))

	mux.Handle("GET /clientes", authed(customers.List))
	mux.Handle("GET /clientes/nuevo", authed(customers.NewPage))
	mux.Handle("POST /clientes/nuevo", authed(customers.Create))
	mux.Handle("GET /clientes/{id}", authed(customers.EditPage))
	mux.Handle("POST /clientes/{id}", authed(customers.Update))
	mux.Handle("POST /clientes/{id}/eliminar", admin(customers.Delete))

	mux.Handle("GET /cotizaciones", authed(quotes.List))
	mux.Handle("GET /cotizaciones/nueva", authed(quotes.NewPage))
	mux.Handle("POST /cotizaciones/nueva", authed(quotes.Create))
	mux.Handle("GET /cotizaciones/{folio}", authed(quotes.Builder))
	mux.Handle("POST /cotizaciones/{folio}/recalcular", authed(quotes.Recalcular))
	mux.Handle("GET /cotizaciones/{folio}/productos", authed(quotes.BuscarProductos))
	mux.Handle("POST /cotizaciones/{folio}/guardar", authed(quotes.Guardar))
	mux.Handle("POST /cotizaciones/{folio}/emitir", authed(quotes.Emitir))
	mux.Handle("POST /cotizaciones/{folio}/revisar", authed(quotes.Revisar))
	mux.Handle("GET /cotizaciones/{folio}/duplicar", authed(quotes.DuplicarPage))
	mux.Handle("POST /cotizaciones/{folio}/duplicar", authed(quotes.Duplicar))
	mux.Handle("GET /cotizaciones/{folio}/pdf", authed(quotes.PDF))
	mux.Handle("GET /cotizaciones/{folio}/pdf/{name}", authed(quotes.PDF))
	mux.Handle("POST /cotizaciones/{folio}/proveedor", authed(quotes.Proveedor))
	mux.Handle("GET /cotizaciones/{folio}/proveedor/{id}", authed(quotes.ProveedorArchivo))
	mux.Handle("POST /cotizaciones/{folio}/proveedor/{id}/eliminar", authed(quotes.ProveedorEliminar))

	mux.Handle("GET /proyectos", authed(projects.List))
	mux.Handle("GET /proyectos/{folio}", authed(projects.Page))
	mux.Handle("POST /proyectos/{folio}/etapa", authed(projects.Etapa))
	mux.Handle("POST /proyectos/{folio}/seguimiento", authed(projects.Seguimiento))
	mux.Handle("GET /pronostico", authed(projects.Pronostico))
	mux.Handle("POST /proyectos/{folio}/comentarios", authed(projects.Comentar))
	mux.Handle("POST /proyectos/{folio}/oc", authed(projects.OC))
	mux.Handle("POST /proyectos/{folio}/factura", authed(projects.Factura))
	mux.Handle("POST /proyectos/{folio}/pago", authed(projects.Pago))
	mux.Handle("GET /proyectos/{folio}/archivos/{id}", authed(projects.Archivo))
	mux.Handle("POST /proyectos/{folio}/proveedor", authed(projects.Proveedor))
	mux.Handle("POST /proyectos/{folio}/perder", authed(projects.Perder))
	mux.Handle("POST /proyectos/{folio}/reabrir", authed(projects.Reabrir))

	mux.Handle("GET /facturas", authed(facturas.List))
	mux.Handle("GET /facturas/{id}", authed(facturas.Page))
	mux.Handle("GET /facturas/{id}/xml", authed(facturas.XML))
	mux.Handle("GET /facturas/{id}/pdf", authed(facturas.PDF))
	mux.Handle("POST /proyectos/{folio}/factura/nueva", admin(facturas.Crear))
	mux.Handle("POST /facturas/{id}", admin(facturas.Guardar))
	mux.Handle("POST /facturas/{id}/validar", admin(facturas.Validar))
	mux.Handle("POST /facturas/{id}/editar", admin(facturas.Editar))
	mux.Handle("POST /facturas/{id}/timbrar", admin(facturas.Timbrar))
	mux.Handle("POST /facturas/{id}/cancelar", admin(facturas.Cancelar))
	mux.Handle("POST /facturas/{id}/consultar", admin(facturas.Consultar))
	mux.Handle("POST /facturas/{id}/sustituir", admin(facturas.Sustituir))
	mux.Handle("POST /facturas/{id}/pago", admin(facturas.Pago))

	mux.Handle("GET /documentos", authed(documentos.List))
	mux.Handle("GET /documentos/{id}", authed(documentos.Archivo))
	mux.Handle("POST /documentos", admin(documentos.Subir))
	mux.Handle("POST /documentos/{id}/eliminar", admin(documentos.Eliminar))
	mux.Handle("POST /documentos/{id}/editar", admin(documentos.Editar))
	mux.Handle("POST /documentos/secciones/{id}/editar", admin(documentos.RenombrarSeccion))
	mux.Handle("POST /documentos/secciones/{id}/eliminar", admin(documentos.EliminarSeccion))
	mux.Handle("POST /documentos/secciones/{id}/mover", admin(documentos.MoverSeccion))

	mux.Handle("GET /usuarios", admin(users.List))
	mux.Handle("POST /usuarios/{id}/rol", admin(users.SetRole))
	mux.Handle("POST /usuarios/{id}/deshabilitar", admin(users.SetDisabled(true)))
	mux.Handle("POST /usuarios/{id}/habilitar", admin(users.SetDisabled(false)))

	mux.Handle("GET /ajustes", admin(settings.Page))
	mux.Handle("POST /ajustes/margenes", admin(settings.CreateMargen))
	mux.Handle("POST /ajustes/margenes/{id}", admin(settings.UpdateMargen))
	mux.Handle("POST /ajustes/margenes/{id}/predeterminado", admin(settings.PredeterminarMargen))
	mux.Handle("POST /ajustes/margenes/{id}/retirar", admin(settings.RetirarMargen))
	mux.Handle("POST /ajustes/margenes/{id}/restaurar", admin(settings.RestaurarMargen))
	mux.Handle("POST /ajustes/emisor", admin(settings.Emisor))
	mux.Handle("POST /ajustes/materiales", admin(settings.CreateMaterial))
	mux.Handle("POST /ajustes/materiales/{id}", admin(settings.UpdateMaterial))

	mux.Handle("GET /unidades", admin(units.List))
	mux.Handle("POST /unidades", admin(units.Create))
	mux.Handle("POST /unidades/{id}/sat", admin(units.SetSAT))

	mux.Handle("GET /familias", admin(familias.List))
	mux.Handle("POST /familias", admin(familias.Create))
	mux.Handle("GET /familias/{id}", admin(familias.EditPage))
	mux.Handle("POST /familias/{id}", admin(familias.Update))

	mux.Handle("GET /ayuda", authed(help.Page))
	mux.Handle("GET /ayuda/{pagina}", authed(help.Page))

	mux.Handle("GET /{$}", authed(func(w http.ResponseWriter, r *http.Request) {
		dash, err := db.SalesDashboard(r.Context(), today(r))
		if err != nil {
			serverError(w, r, err)
			return
		}
		user, _ := UserFromContext(r.Context())
		views.Home(build.Time, time.Now(), build.SHA, dash, navUserView(user)).Render(r.Context(), w)
	}))

	// Outermost first: the access log has to see the status the CSRF check produces, and
	// its panic recovery has to cover everything below it.
	csrf := http.NewCrossOriginProtection()
	banner := build.Env.Banner()
	withBanner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(views.WithEnvironmentBanner(r.Context(), banner)))
	})
	return AccessLog(logger)(securityHeaders(csrf.Handler(withBanner)))
}

// securityHeaders forbids framing any page, so another site can't overlay the app's
// buttons with its own (clickjacking). Nothing in the app frames its own pages either.
// A handler that sets its own Content-Security-Policy has to keep frame-ancestors in it.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}
