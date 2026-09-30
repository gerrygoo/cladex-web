package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/store"
)

// The tests in this file pin the user-visible strings and behaviors that docs/guia/
// quotes verbatim to the sellers. The guide's whole premise is that it names buttons,
// fields, and messages exactly as they appear on screen, so a reworded message silently
// makes the guide wrong — these tests turn that into a failing build instead. Each block
// names the guide page and section it backs; change a message here and change the guide
// in the same commit.

// --- docs/guia/acceso.md ---

// "Si te equivocas de contraseña varias veces seguidas, la app te hace esperar antes
// del siguiente intento: Demasiados intentos. Intenta de nuevo en N segundos."
func TestGuiaAccesoRateLimitMessage(t *testing.T) {
	a := newTestAuth(t)
	createTestUser(t, a, "rodolfo", "vendedor", "hunter2")

	var rec *httptest.ResponseRecorder
	for range 6 {
		form := url.Values{"username": {"rodolfo"}, "password": {"wrong"}}
		req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = "203.0.113.9:1234"
		rec = httptest.NewRecorder()
		a.LoginSubmit(rec, req)
	}

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("6th attempt status = %d, want 429", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Demasiados intentos. Intenta de nuevo en ") || !strings.Contains(body, " segundos.") {
		t.Errorf("body missing the documented wait message: %s", body)
	}
}

// "Escribe la Contraseña nueva (mínimo 8 caracteres) y repítela en Confirmar
// contraseña nueva." — the three ways that form can be wrong.
func TestGuiaMiCuentaMessages(t *testing.T) {
	cases := map[string]struct {
		form url.Values
		want string
	}{
		"contraseña actual vacía": {
			url.Values{"current_password": {""}, "new_password": {"nuevapass1"}, "confirm_password": {"nuevapass1"}},
			"La contraseña actual es obligatoria.",
		},
		"contraseña nueva corta": {
			url.Values{"current_password": {"hunter2"}, "new_password": {"corta1"}, "confirm_password": {"corta1"}},
			"La contraseña nueva debe tener al menos 8 caracteres.",
		},
		"confirmación distinta": {
			url.Values{"current_password": {"hunter2"}, "new_password": {"nuevapass1"}, "confirm_password": {"otracosa9"}},
			"Las contraseñas no coinciden.",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			a := newTestAuth(t)
			userID := createTestUser(t, a, "rodolfo", "vendedor", "hunter2")

			rec := doForm(t, a, userID, a.MiCuentaSubmit, "POST", "/mi-cuenta", nil, tc.form, false)

			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422; body = %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.want) {
				t.Errorf("body missing %q: %s", tc.want, rec.Body.String())
			}
		})
	}
}

// --- docs/guia/README.md, "Quién puede hacer qué" ---

// "Si en el menú Base de datos solo ves Productos y Clientes, tu cuenta es de vendedor."
func TestGuiaNavLinksByRole(t *testing.T) {
	a := newTestAuth(t)
	products := NewProducts(a.store)
	vendedorID := createTestUser(t, a, "rodolfo", "vendedor", "hunter2")
	adminID := createTestUser(t, a, "ana", "admin", "hunter2")

	adminLinks := []string{`href="/usuarios"`, `href="/unidades"`, `href="/familias"`, `href="/ajustes"`}

	rec := doForm(t, a, vendedorID, products.List, "GET", "/productos", nil, nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("vendedor status = %d, want 200", rec.Code)
	}
	for _, link := range adminLinks {
		if strings.Contains(rec.Body.String(), link) {
			t.Errorf("vendedor's menu shows the admin link %s", link)
		}
	}
	// The seller-facing links are there for both roles.
	for _, link := range []string{`href="/cotizaciones"`, `href="/productos"`, `href="/clientes"`} {
		if !strings.Contains(rec.Body.String(), link) {
			t.Errorf("vendedor's menu is missing %s", link)
		}
	}

	rec = doForm(t, a, adminID, products.List, "GET", "/productos", nil, nil, false)
	for _, link := range adminLinks {
		if !strings.Contains(rec.Body.String(), link) {
			t.Errorf("admin's menu is missing %s", link)
		}
	}
}

// --- docs/guia/clientes.md ---

// "Verás Cliente guardado. y te quedas en la ficha del cliente", plus the page's
// "Problemas comunes" table.
func TestGuiaClientesMessages(t *testing.T) {
	a := newTestAuth(t)
	cs := NewCustomers(a.store)
	userID := createTestUser(t, a, "rodolfo", "vendedor", "hunter2")

	// Saved: the redirect lands on the customer's own page, which shows the message.
	rec := doForm(t, a, userID, cs.Create, "POST", "/clientes/nuevo", nil,
		url.Values{"name": {"Grupo PEME"}}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Create status = %d, want 303; body = %s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.HasSuffix(loc, "?guardado=1") {
		t.Fatalf("Location = %q, want the edit page with ?guardado=1", loc)
	}
	id := strings.TrimSuffix(strings.TrimPrefix(loc, "/clientes/"), "?guardado=1")
	rec = doForm(t, a, userID, cs.EditPage, "GET", loc, map[string]string{"id": id}, nil, false)
	if !strings.Contains(rec.Body.String(), "Cliente guardado.") {
		t.Errorf("customer page missing %q: %s", "Cliente guardado.", rec.Body.String())
	}

	// Problemas comunes.
	rec = doForm(t, a, userID, cs.Create, "POST", "/clientes/nuevo", nil,
		url.Values{"name": {""}, "email": {"no-es-un-correo"}}, false)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	for _, want := range []string{"El nombre es obligatorio.", "Email inválido."} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("body missing %q: %s", want, rec.Body.String())
		}
	}
}

// "El cliente desaparece de la lista y ya no se puede elegir para cotizaciones nuevas.
// Sus cotizaciones existentes se conservan y siguen apareciendo en Cotizaciones."
func TestGuiaClienteEliminadoConservaCotizaciones(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	cs := NewCustomers(a.store)
	userID := createTestUser(t, a, "rodolfo", "vendedor", "hunter2")
	ctx := context.Background()

	customerID, err := a.store.CreateCustomer(ctx, store.Customer{Name: "Cliente Descontinuado"})
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	quote, err := a.store.CreateDraftQuote(ctx, customerID, userID, "QA")
	if err != nil {
		t.Fatalf("CreateDraftQuote: %v", err)
	}

	rec := doForm(t, a, userID, cs.Delete, "POST", "/clientes/1/eliminar",
		map[string]string{"id": strconv.FormatInt(customerID, 10)}, url.Values{}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Delete status = %d, want 303; body = %s", rec.Code, rec.Body.String())
	}

	// Gone from the customer list...
	rec = doForm(t, a, userID, cs.List, "GET", "/clientes", nil, nil, false)
	if strings.Contains(rec.Body.String(), "Cliente Descontinuado") {
		t.Errorf("deleted customer still listed in /clientes")
	}
	// ...and no longer offered when starting a new quote.
	rec = doForm(t, a, userID, q.NewPage, "GET", "/cotizaciones/nueva", nil, nil, false)
	if strings.Contains(rec.Body.String(), "Cliente Descontinuado") {
		t.Errorf("deleted customer still selectable on /cotizaciones/nueva")
	}
	// But their existing quote is untouched and still listed.
	rec = doForm(t, a, userID, q.List, "GET", "/cotizaciones", nil, nil, false)
	if !strings.Contains(rec.Body.String(), quote.Folio) {
		t.Errorf("quote %s vanished from /cotizaciones with its deleted customer: %s", quote.Folio, rec.Body.String())
	}
}

// --- docs/guia/productos.md ---

// "Haz clic en Guardar. Verás Producto guardado.", plus the page's "Problemas comunes"
// table — including the Peso variant the guide calls "el equivalente de Peso", and the
// two Materiales rows.
func TestGuiaProductosMessages(t *testing.T) {
	a := newTestAuth(t)
	p := NewProducts(a.store)
	userID := createTestUser(t, a, "rodolfo", "vendedor", "hunter2")
	ctx := context.Background()

	familyID, err := a.store.UpsertFamily(ctx, "ABASTILUM", "")
	if err != nil {
		t.Fatalf("UpsertFamily: %v", err)
	}

	rec := doForm(t, a, userID, p.Create, "POST", "/productos/nuevo", nil, url.Values{
		"sku":         {"abl-foco"},
		"description": {"Foco LED"},
		"family_id":   {strconv.FormatInt(familyID, 10)},
		"cost":        {"45.00"},
	}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Create status = %d, want 303; body = %s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	id := strings.TrimSuffix(strings.TrimPrefix(loc, "/productos/"), "?guardado=1")
	rec = doForm(t, a, userID, p.EditPage, "GET", loc, map[string]string{"id": id}, nil, false)
	if !strings.Contains(rec.Body.String(), "Producto guardado.") {
		t.Errorf("product page missing %q: %s", "Producto guardado.", rec.Body.String())
	}

	rec = doForm(t, a, userID, p.Create, "POST", "/productos/nuevo", nil, url.Values{
		"sku":         {""},
		"description": {""},
		"family_id":   {""},
		"cost":        {"45,00"},
		"kg_per_m":    {"cero punto uno"},
	}, false)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	for _, want := range []string{
		"El SKU es obligatorio.",
		"La descripción es obligatoria.",
		"Selecciona una familia.",
		"Costo inválido; usa un número, p. ej. 123.45.",
		"Peso inválido; usa un número, p. ej. 0.123.",
	} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("body missing %q: %s", want, rec.Body.String())
		}
	}

	// Materiales: a zero quantity, then the same material twice.
	materials, err := a.store.ListMaterials(ctx)
	if err != nil || len(materials) == 0 {
		t.Fatalf("ListMaterials = %v, %v; want the seeded material", materials, err)
	}
	pv := map[string]string{"id": id}
	path := "/productos/" + id + "/materiales"
	add := func(qty string) *httptest.ResponseRecorder {
		return doForm(t, a, userID, p.AddMaterial, "POST", path, pv, url.Values{
			"material_id": {strconv.FormatInt(materials[0].ID, 10)},
			"qty":         {qty},
		}, false)
	}
	if rec := add("0"); !strings.Contains(rec.Body.String(), "Cantidad inválida; usa un número positivo, p. ej. 0.1723.") {
		t.Errorf("qty 0: body missing the documented message (status %d): %s", rec.Code, rec.Body.String())
	}
	if rec := add("0.1723"); rec.Code != http.StatusSeeOther {
		t.Fatalf("AddMaterial status = %d, want 303; body = %s", rec.Code, rec.Body.String())
	}
	if rec := add("0.2"); !strings.Contains(rec.Body.String(),
		"Este producto ya tiene ese material; elimínalo y vuelve a agregarlo para cambiar la cantidad.") {
		t.Errorf("duplicate material: body missing the documented message (status %d): %s", rec.Code, rec.Body.String())
	}
}

// --- docs/guia/cotizaciones.md ---

// The "Problemas comunes" rows for the Nueva cotización form.
func TestGuiaCotizacionNuevaMessages(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	userID := createTestUser(t, a, "rodolfo", "vendedor", "hunter2")

	rec := doForm(t, a, userID, q.Create, "POST", "/cotizaciones/nueva", nil,
		url.Values{"customer_id": {""}, "prefix": {""}}, false)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	for _, want := range []string{"Selecciona un cliente.", "Selecciona una serie de folio."} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("body missing %q: %s", want, rec.Body.String())
		}
	}
}

// The line-level rows of the same table, plus "Verás Borrador guardado." — every one of
// these is a message a seller reads off the builder screen.
func TestGuiaBuilderLineMessages(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	userID := createTestUser(t, a, "rodolfo", "vendedor", "hunter2")
	customerID, _, _ := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, userID)

	quote, err := a.store.CreateDraftQuote(context.Background(), customerID, userID, "QA")
	if err != nil {
		t.Fatalf("CreateDraftQuote: %v", err)
	}

	// One line per documented error. A line reports only its first problem, so the bad
	// description, the bad price, and the bad quantity each need their own row.
	// line_keys is one comma-separated field, the way the builder form posts it.
	form := url.Values{
		"line_keys":             {"0,1,2"},
		"lines[0][kind]":        {"free"},
		"lines[0][description]": {""},
		"lines[0][qty]":         {"1"},
		"lines[0][unit_price]":  {"500.00"},
		"lines[1][kind]":        {"free"},
		"lines[1][description]": {"Flete"},
		"lines[1][qty]":         {"1"},
		"lines[1][unit_price]":  {"$123.45"},
		"lines[2][kind]":        {"free"},
		"lines[2][description]": {"Maniobras"},
		"lines[2][qty]":         {"0"},
		"lines[2][unit_price]":  {"500.00"},
	}
	rec := doForm(t, a, userID, q.Guardar, "POST", "/cotizaciones/"+quote.Folio+"/guardar",
		map[string]string{"folio": quote.Folio}, form, false)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{"Descripción obligatoria.", "Precio inválido.", "Cantidad inválida."} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("body missing %q: %s", want, rec.Body.String())
		}
	}

	// A valid save, then the success banner the guide promises.
	form = url.Values{
		"line_keys":             {"0"},
		"lines[0][kind]":        {"free"},
		"lines[0][description]": {"Flete"},
		"lines[0][qty]":         {"1"},
		"lines[0][unit_price]":  {"500.00"},
	}
	rec = doForm(t, a, userID, q.Guardar, "POST", "/cotizaciones/"+quote.Folio+"/guardar",
		map[string]string{"folio": quote.Folio}, form, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Guardar status = %d, want 303; body = %s", rec.Code, rec.Body.String())
	}
	rec = doForm(t, a, userID, q.Builder, "GET", "/cotizaciones/"+quote.Folio+"?guardado=1",
		map[string]string{"folio": quote.Folio}, nil, false)
	if !strings.Contains(rec.Body.String(), "Borrador guardado.") {
		t.Errorf("builder missing %q: %s", "Borrador guardado.", rec.Body.String())
	}
}

// "Los borradores que ya lo tenían muestran Producto no encontrado. en esa línea; hay
// que quitarla antes de guardar o emitir." (docs/guia/productos.md, "Eliminar un
// producto")
func TestGuiaProductoEliminadoEnBorrador(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	p := NewProducts(a.store)
	userID := createTestUser(t, a, "rodolfo", "vendedor", "hunter2")
	customerID, flatProductID, _ := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, userID)
	ctx := context.Background()

	quote, err := a.store.CreateDraftQuote(ctx, customerID, userID, "QA")
	if err != nil {
		t.Fatalf("CreateDraftQuote: %v", err)
	}
	form := url.Values{
		"line_keys":            {"0"},
		"lines[0][kind]":       {"product"},
		"lines[0][product_id]": {strconv.FormatInt(flatProductID, 10)},
		"lines[0][qty]":        {"2"},
	}
	if rec := doForm(t, a, userID, q.Guardar, "POST", "/cotizaciones/"+quote.Folio+"/guardar",
		map[string]string{"folio": quote.Folio}, form, false); rec.Code != http.StatusSeeOther {
		t.Fatalf("Guardar status = %d, want 303; body = %s", rec.Code, rec.Body.String())
	}

	if rec := doForm(t, a, userID, p.Delete, "POST", "/productos/1/eliminar",
		map[string]string{"id": strconv.FormatInt(flatProductID, 10)}, url.Values{}, false); rec.Code != http.StatusSeeOther {
		t.Fatalf("product Delete status = %d, want 303", rec.Code)
	}

	// Reopening the draft flags the orphaned line rather than pricing it or 500ing.
	rec := doForm(t, a, userID, q.Builder, "GET", "/cotizaciones/"+quote.Folio,
		map[string]string{"folio": quote.Folio}, nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("Builder status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Producto no encontrado.") {
		t.Errorf("draft missing %q for the deleted product: %s", "Producto no encontrado.", rec.Body.String())
	}

	// And the guide's rule that the line has to be removed first: saving it as-is is
	// refused, exactly like any other line error.
	rec = doForm(t, a, userID, q.Guardar, "POST", "/cotizaciones/"+quote.Folio+"/guardar",
		map[string]string{"folio": quote.Folio}, form, false)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("Guardar with an orphaned line = %d, want 422", rec.Code)
	}
}

// "La vigencia es de 30 días a partir de hoy" (docs/guia/cotizaciones.md, "Emitir la
// cotización"; README.md, "Conceptos"). Requires the typst CLI, like the other Emitir
// tests.
func TestGuiaVigencia30Dias(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	userID := createTestUser(t, a, "rodolfo", "vendedor", "hunter2")
	customerID, flatProductID, _ := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, userID)
	ctx := context.Background()

	quote, err := a.store.CreateDraftQuote(ctx, customerID, userID, "QA")
	if err != nil {
		t.Fatalf("CreateDraftQuote: %v", err)
	}
	form := url.Values{
		"line_keys":            {"0"},
		"lines[0][kind]":       {"product"},
		"lines[0][product_id]": {strconv.FormatInt(flatProductID, 10)},
		"lines[0][qty]":        {"1"},
	}
	if rec := doForm(t, a, userID, q.Emitir, "POST", "/cotizaciones/"+quote.Folio+"/emitir",
		map[string]string{"folio": quote.Folio}, form, false); rec.Code != http.StatusSeeOther {
		t.Fatalf("Emitir status = %d, want 303; body = %s", rec.Code, rec.Body.String())
	}

	issued, err := a.store.QuoteByID(ctx, quote.ID)
	if err != nil {
		t.Fatalf("QuoteByID: %v", err)
	}
	want := time.Now().AddDate(0, 0, 30).Format("2006-01-02")
	if issued.ValidUntil == nil || *issued.ValidUntil != want {
		t.Errorf("ValidUntil = %v, want %s (30 días a partir de hoy)", issued.ValidUntil, want)
	}

	// "Verás Cotización emitida. y Estado: emitida."
	rec := doForm(t, a, userID, q.Builder, "GET", "/cotizaciones/"+quote.Folio+"?emitida=1",
		map[string]string{"folio": quote.Folio}, nil, false)
	if !strings.Contains(rec.Body.String(), "Cotización emitida.") {
		t.Errorf("issued page missing %q: %s", "Cotización emitida.", rec.Body.String())
	}
}

// "Al hacer clic en Emitir cotización la página se recarga sin mensaje. → La cotización
// no tiene líneas." — the last row of the guide's troubleshooting table.
func TestGuiaEmitirSinLineas(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	userID := createTestUser(t, a, "rodolfo", "vendedor", "hunter2")
	customerID, _, _ := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, userID)

	quote, err := a.store.CreateDraftQuote(context.Background(), customerID, userID, "QA")
	if err != nil {
		t.Fatalf("CreateDraftQuote: %v", err)
	}

	rec := doForm(t, a, userID, q.Emitir, "POST", "/cotizaciones/"+quote.Folio+"/emitir",
		map[string]string{"folio": quote.Folio}, url.Values{"line_keys": {}}, false)

	// Re-renders the builder (the "página se recarga" the guide describes) and leaves
	// the quote a draft.
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "Cotización emitida.") {
		t.Errorf("empty quote reported itself as issued")
	}
	after, err := a.store.QuoteByID(context.Background(), quote.ID)
	if err != nil {
		t.Fatalf("QuoteByID: %v", err)
	}
	if after.Status != "borrador" {
		t.Errorf("Status = %q, want borrador — an empty quote must not issue", after.Status)
	}
}

// "esta cotización ya no es editable — La cotización ya fue emitida (p. ej. la emitiste
// desde otra pestaña)." (docs/guia/cotizaciones.md, "Problemas comunes")
//
// Note the wording: Guardar and Recalcular produce exactly the documented sentence, but
// Emitir answers the same situation with "esta cotización ya fue emitida". Both are 409s
// and mean the same thing; only the first is in the guide. Asserted as-is rather than
// unified, since changing either the message or the guide is a product call.
func TestGuiaCotizacionEmitidaNoEsEditable(t *testing.T) {
	a := newTestAuth(t)
	q := newTestQuotes(t, a)
	userID := createTestUser(t, a, "rodolfo", "vendedor", "hunter2")
	customerID, flatProductID, _ := seedQuoteBuilderFixtures(t, a)
	seedPricingSettings(t, a, userID)
	ctx := context.Background()

	quote, err := a.store.CreateDraftQuote(ctx, customerID, userID, "QA")
	if err != nil {
		t.Fatalf("CreateDraftQuote: %v", err)
	}
	form := url.Values{
		"line_keys":            {"0"},
		"lines[0][kind]":       {"product"},
		"lines[0][product_id]": {strconv.FormatInt(flatProductID, 10)},
		"lines[0][qty]":        {"1"},
	}
	if rec := doForm(t, a, userID, q.Emitir, "POST", "/cotizaciones/"+quote.Folio+"/emitir",
		map[string]string{"folio": quote.Folio}, form, false); rec.Code != http.StatusSeeOther {
		t.Fatalf("Emitir status = %d, want 303; body = %s", rec.Code, rec.Body.String())
	}

	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
		path    string
		want    string
	}{
		{"Guardar", q.Guardar, "/guardar", "esta cotización ya no es editable"},
		{"Recalcular", q.Recalcular, "/recalcular", "esta cotización ya no es editable"},
		{"Emitir", q.Emitir, "/emitir", ""}, // see the note above
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := doForm(t, a, userID, tc.handler, "POST", "/cotizaciones/"+quote.Folio+tc.path,
				map[string]string{"folio": quote.Folio}, form, false)
			if rec.Code != http.StatusConflict {
				t.Fatalf("status = %d, want 409", rec.Code)
			}
			if tc.want != "" && !strings.Contains(rec.Body.String(), tc.want) {
				t.Errorf("body = %q, want the documented %q", strings.TrimSpace(rec.Body.String()), tc.want)
			}
		})
	}
}

// "En la columna Costo verás el costo del producto, sin margen: su costo fijo, seguido
// de "+ materiales" si además tiene materiales, o solo "materiales" si su costo sale de
// sus materiales." (docs/guia/productos.md, "Buscar un producto")
func TestGuiaProductosColumnaCosto(t *testing.T) {
	a := newTestAuth(t)
	p := NewProducts(a.store)
	userID := createTestUser(t, a, "rodolfo", "vendedor", "hunter2")
	ctx := context.Background()

	familyID, err := a.store.UpsertFamily(ctx, "MIXTA", "")
	if err != nil {
		t.Fatalf("UpsertFamily: %v", err)
	}
	materials, err := a.store.ListMaterials(ctx)
	if err != nil || len(materials) == 0 {
		t.Fatalf("ListMaterials = %v, %v; want the seeded material", materials, err)
	}
	cost := money.Micros(45_000_000)
	for _, prod := range []struct {
		p            store.Product
		hasMaterials bool
	}{
		{store.Product{FamilyID: familyID, SKU: "cca-c14", Description: "Cable THW 14", CostMicros: &cost}, false},
		{store.Product{FamilyID: familyID, SKU: "abl-poste", Description: "Poste", CostMicros: &cost}, true},
		{store.Product{FamilyID: familyID, SKU: "ccs-4", Description: "Cable CCS 4"}, true},
	} {
		id, err := a.store.CreateProduct(ctx, prod.p)
		if err != nil {
			t.Fatalf("CreateProduct(%s): %v", prod.p.SKU, err)
		}
		if prod.hasMaterials {
			if _, err := a.store.AddProductMaterial(ctx, id, materials[0].ID, money.Micros(172_300)); err != nil {
				t.Fatalf("AddProductMaterial(%s): %v", prod.p.SKU, err)
			}
		}
	}

	rec := doForm(t, a, userID, p.List, "GET", "/productos", nil, nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	// Each row is checked on its own so a match in a neighbouring cell can't stand in
	// for the one being asserted.
	rowFor := func(sku string) string {
		for _, row := range strings.Split(rec.Body.String(), "<tr") {
			if strings.Contains(row, sku) {
				return row
			}
		}
		t.Fatalf("no row for %s in: %s", sku, rec.Body.String())
		return ""
	}
	if got := rowFor("cca-c14"); !strings.Contains(got, "$45.00") || strings.Contains(got, "materiales") {
		t.Errorf("cost-only row doesn't show just its cost: %s", got)
	}
	if got := rowFor("abl-poste"); !strings.Contains(got, "$45.00 + materiales") {
		t.Errorf("cost+materials row doesn't show %q: %s", "$45.00 + materiales", got)
	}
	if got := rowFor("ccs-4"); !strings.Contains(got, "materiales") || strings.Contains(got, "$") {
		t.Errorf("materials-only row doesn't show just %q: %s", "materiales", got)
	}
}
