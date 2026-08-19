package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/pricing"
	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

// Quotes holds the dependencies for the /cotizaciones handlers.
type Quotes struct {
	store *store.Store
}

func NewQuotes(s *store.Store) *Quotes {
	return &Quotes{store: s}
}

// validPrefixes mirrors the quotes.prefix CHECK constraint in migrations/0001_init.sql.
// Per the user's decision, prefix is a free choice at quote-creation time — a folio
// series label, not a restriction on which products a quote can contain.
var validPrefixes = map[string]bool{"QA": true, "QS": true, "QI": true}

// loadPricingSettings reads the three admin-configurable knobs internal/pricing needs
// (internal/web/ajustes.go defines and stores these same three keys, as raw micros
// integers — not decimal strings — so this parses with strconv.ParseInt, not
// money.ParseMicros). An unset key parses as zero; internal/pricing itself rejects a
// zero FX rate or copper price with a clear error only when a line actually needs it.
func (q *Quotes) loadPricingSettings(ctx context.Context) (pricing.Settings, error) {
	raw, err := q.store.SettingValues(ctx, []string{"fx_rate", "copper_price", "default_margin"})
	if err != nil {
		return pricing.Settings{}, fmt.Errorf("load pricing settings: %w", err)
	}
	parse := func(key string) money.Micros {
		v, ok := raw[key]
		if !ok {
			return 0
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0
		}
		return money.Micros(n)
	}
	return pricing.Settings{
		FXRate:        parse("fx_rate"),
		CopperPrice:   parse("copper_price"),
		DefaultMargin: parse("default_margin"),
	}, nil
}

// List renders /cotizaciones: the full page normally, or just the table body when
// called via htmx live search — same pattern as Products.List.
func (q *Quotes) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	sortCol, dir := sortParams(r)
	quotes, err := q.store.ListQuotes(ctx, query, sortCol, dir)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	if r.Header.Get("HX-Request") == "true" {
		views.QuotesTableBody(quotes).Render(ctx, w)
		return
	}
	successMsg := ""
	if r.URL.Query().Get("creada") == "1" {
		successMsg = "Cotización creada."
	}
	user, _ := UserFromContext(ctx)
	views.QuotesList(quotes, query, sortCol, dir, successMsg, navUserView(user)).Render(ctx, w)
}

// QuoteNewForm is the /cotizaciones/nueva form.
type QuoteNewForm struct {
	CustomerID string
	Prefix     string
}

func parseQuoteNewForm(r *http.Request) QuoteNewForm {
	return QuoteNewForm{
		CustomerID: strings.TrimSpace(r.FormValue("customer_id")),
		Prefix:     strings.TrimSpace(r.FormValue("prefix")),
	}
}

func (f QuoteNewForm) Validate() map[string]string {
	errs := map[string]string{}
	if id, err := strconv.ParseInt(f.CustomerID, 10, 64); f.CustomerID == "" || err != nil || id <= 0 {
		errs["customer_id"] = "Selecciona un cliente."
	}
	if !validPrefixes[f.Prefix] {
		errs["prefix"] = "Selecciona una serie de folio."
	}
	return errs
}

// NewPage renders the empty "nueva cotización" form.
func (q *Quotes) NewPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	customers, err := q.store.ListCustomers(ctx, "", "name", "asc")
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	user, _ := UserFromContext(ctx)
	views.QuoteNewForm(customers, views.QuoteNewFormValues{}, nil, navUserView(user)).Render(ctx, w)
}

// Create handles POST /cotizaciones/nueva: assigns a folio and redirects into the
// builder for the new draft.
func (q *Quotes) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(w, "solicitud inválida", http.StatusBadRequest)
		return
	}
	form := parseQuoteNewForm(r)
	fieldErrors := form.Validate()
	if len(fieldErrors) > 0 {
		customers, err := q.store.ListCustomers(ctx, "", "name", "asc")
		if err != nil {
			http.Error(w, "error interno", http.StatusInternalServerError)
			return
		}
		user, _ := UserFromContext(ctx)
		w.WriteHeader(http.StatusUnprocessableEntity)
		values := views.QuoteNewFormValues{CustomerID: form.CustomerID, Prefix: form.Prefix}
		views.QuoteNewForm(customers, values, fieldErrors, navUserView(user)).Render(ctx, w)
		return
	}

	customerID, _ := strconv.ParseInt(form.CustomerID, 10, 64)
	user, _ := UserFromContext(ctx)
	quote, err := q.store.CreateDraftQuote(ctx, customerID, user.ID, form.Prefix)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/cotizaciones/"+quote.Folio, http.StatusSeeOther)
}

// loadQuoteOrNotFound looks up the quote named by the {folio} path value, writing a 404
// and returning nil if it doesn't exist. Callers must check for a nil return.
func (q *Quotes) loadQuoteOrNotFound(w http.ResponseWriter, r *http.Request) *store.Quote {
	quote, err := q.store.QuoteByFolio(r.Context(), r.PathValue("folio"))
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return nil
	}
	if quote == nil {
		http.NotFound(w, r)
		return nil
	}
	return quote
}

// quoteLineInput is the raw, unvalidated shape of one builder line as submitted by the
// form — see internal/views/quotes.templ for the exact field names
// (line_keys, lines[K][kind|product_id|description|qty|unit_price]).
type quoteLineInput struct {
	Key         string
	Kind        string // "product" | "free"
	ProductID   string
	Description string
	Qty         string
	UnitPrice   string // free lines only
}

// parseQuoteLineInputs reads the builder form's full current line set and applies at
// most one delta from this particular submit (add_product_id, add_free=1, or
// remove_key) — see the M2.2 plan's "line editing model": every mutation resubmits the
// whole set, and this function alone decides what the new working set is. Keys are
// reassigned "0","1",... in order for a stable re-render.
func parseQuoteLineInputs(r *http.Request) []quoteLineInput {
	var keys []string
	if raw := strings.TrimSpace(r.FormValue("line_keys")); raw != "" {
		keys = strings.Split(raw, ",")
	}
	removeKey := r.FormValue("remove_key")

	var out []quoteLineInput
	for _, k := range keys {
		if k == removeKey {
			continue
		}
		out = append(out, quoteLineInput{
			Kind:        r.FormValue("lines[" + k + "][kind]"),
			ProductID:   r.FormValue("lines[" + k + "][product_id]"),
			Description: r.FormValue("lines[" + k + "][description]"),
			Qty:         r.FormValue("lines[" + k + "][qty]"),
			UnitPrice:   r.FormValue("lines[" + k + "][unit_price]"),
		})
	}
	if pid := strings.TrimSpace(r.FormValue("add_product_id")); pid != "" {
		out = append(out, quoteLineInput{Kind: "product", ProductID: pid, Qty: "1"})
	}
	if r.FormValue("add_free") == "1" {
		out = append(out, quoteLineInput{Kind: "free", Qty: "1"})
	}
	for i := range out {
		out[i].Key = strconv.Itoa(i)
	}
	return out
}

// inputsFromPersisted converts a draft's already-saved quote_lines into the same
// quoteLineInput shape the builder form submits, so loading the builder page and
// recomputing after an edit go through one code path.
func inputsFromPersisted(lines []store.QuoteLine) []quoteLineInput {
	out := make([]quoteLineInput, len(lines))
	for i, l := range lines {
		in := quoteLineInput{Key: strconv.Itoa(i), Qty: l.QtyMilli.String()}
		if l.ProductID != nil {
			in.Kind = "product"
			in.ProductID = strconv.FormatInt(*l.ProductID, 10)
		} else {
			in.Kind = "free"
			in.Description = l.DescriptionSnapshot
			in.UnitPrice = l.UnitPriceMicros.String()
		}
		out[i] = in
	}
	return out
}

// friendlyPricingError maps internal/pricing's plain-English errors to a Spanish
// message pointing at the fix, matching the rest of the UI's language.
func friendlyPricingError(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "FX rate"):
		return "Falta configurar el tipo de cambio en Ajustes."
	case strings.Contains(msg, "copper price"):
		return "Falta configurar el precio del cobre en Ajustes."
	case strings.Contains(msg, "margin"):
		return "Falta configurar el margen por defecto en Ajustes."
	case strings.Contains(msg, "no pricing data"):
		return "Este producto no tiene datos de precio configurados."
	default:
		return "No se pudo calcular el precio de esta línea."
	}
}

// pricingInputsSnapshot is the JSON shape stored in quote_lines.pricing_inputs — the
// inputs that produced a product line's price, per docs/PLAN.md's "Quote persistence"
// design (kg/m, margin, metal price, FX, whichever applied).
type pricingInputsSnapshot struct {
	FXRate        string `json:"fx_rate,omitempty"`
	CopperPrice   string `json:"copper_price,omitempty"`
	DefaultMargin string `json:"default_margin,omitempty"`
	KgPerM        string `json:"kg_per_m,omitempty"`
	Cost          string `json:"cost,omitempty"`
	UnitPrice     string `json:"unit_price,omitempty"`
}

func buildPricingInputsJSON(p *store.Product, s pricing.Settings) *string {
	snap := pricingInputsSnapshot{}
	if p.Currency == "USD" {
		snap.FXRate = s.FXRate.String()
	}
	switch {
	case p.UnitPriceMicros != nil:
		snap.UnitPrice = p.UnitPriceMicros.String()
	case p.CostMicros != nil:
		snap.Cost = p.CostMicros.String()
		snap.DefaultMargin = s.DefaultMargin.String()
	case p.KgPerMMicros != nil:
		snap.KgPerM = p.KgPerMMicros.String()
		snap.CopperPrice = s.CopperPrice.String()
	}
	b, err := json.Marshal(snap)
	if err != nil {
		return nil
	}
	str := string(b)
	return &str
}

// computeQuoteLines prices every input line via internal/pricing (product lines) or the
// manually entered price (free lines) — the same code path whether this is a live
// htmx recalculation or the final "Guardar borrador" save, so nothing saved ever
// differs from what was last shown on screen. A bad line (invalid qty, unknown
// product, missing FX/copper settings) gets a row-level Error instead of failing the
// whole request.
func (q *Quotes) computeQuoteLines(ctx context.Context, inputs []quoteLineInput, settings pricing.Settings) []views.QuoteLineView {
	result := make([]views.QuoteLineView, len(inputs))
	for i, in := range inputs {
		v := views.QuoteLineView{Key: in.Key, IsFree: in.Kind == "free", QtyRaw: in.Qty}

		qty, err := money.ParseMilli(in.Qty)
		if err != nil || qty <= 0 {
			v.Error = "Cantidad inválida."
			result[i] = v
			continue
		}
		v.QtyMilli = qty

		if in.Kind == "free" {
			v.Description = in.Description
			v.UnitPriceRaw = in.UnitPrice
			if strings.TrimSpace(in.Description) == "" {
				v.Error = "Descripción obligatoria."
				result[i] = v
				continue
			}
			price, err := money.ParseMicros(in.UnitPrice)
			if err != nil || price < 0 {
				v.Error = "Precio inválido."
				result[i] = v
				continue
			}
			v.UnitPriceMicros = price
			v.LineTotal = money.LineTotalCentavos(price, qty)
			result[i] = v
			continue
		}

		productID, err := strconv.ParseInt(in.ProductID, 10, 64)
		if err != nil {
			v.Error = "Producto inválido."
			result[i] = v
			continue
		}
		product, err := q.store.ProductByID(ctx, productID)
		if err != nil || product == nil {
			v.Error = "Producto no encontrado."
			result[i] = v
			continue
		}
		breaks, err := q.store.ListPriceBreaks(ctx, productID)
		if err != nil {
			v.Error = "Error al cargar el producto."
			result[i] = v
			continue
		}

		pid := productID
		v.ProductID = &pid
		v.ProductLabel = product.SKU + " — " + product.Description
		v.Description = product.Description
		v.UnitCode = product.UnitCode

		unitPrice, err := pricing.UnitPrice(pricing.Product{
			UnitPriceMicros: product.UnitPriceMicros,
			CostMicros:      product.CostMicros,
			KgPerMMicros:    product.KgPerMMicros,
			Currency:        product.Currency,
		}, qty, breaks, settings)
		if err != nil {
			v.Error = friendlyPricingError(err)
			result[i] = v
			continue
		}
		v.UnitPriceMicros = unitPrice
		v.LineTotal = money.LineTotalCentavos(unitPrice, qty)
		v.PricingInputsJSON = buildPricingInputsJSON(product, settings)
		result[i] = v
	}
	return result
}

// computeQuoteTotals sums every line without a row-level error — an invalid line
// contributes nothing until it's fixed, rather than 500ing the whole recalculation.
func computeQuoteTotals(lines []views.QuoteLineView) pricing.Totals {
	var pl []pricing.Line
	for _, l := range lines {
		if l.Error == "" {
			pl = append(pl, pricing.Line{UnitPriceMicros: l.UnitPriceMicros, QtyMilli: l.QtyMilli})
		}
	}
	return pricing.ComputeTotals(pl)
}

// productSearchResults runs the builder's "add product" search, capped to a manageable
// result count, for embedding either into the full builder page (no-JS fallback) or the
// htmx results fragment.
func (q *Quotes) productSearchResults(ctx context.Context, query string) ([]store.Product, error) {
	if query == "" {
		return nil, nil
	}
	products, err := q.store.ListProducts(ctx, query, "description", "asc")
	if err != nil {
		return nil, err
	}
	const maxResults = 20
	if len(products) > maxResults {
		products = products[:maxResults]
	}
	return products, nil
}

// Builder renders the full quote-builder page at GET /cotizaciones/{folio}, starting
// from whatever's currently persisted in quote_lines. An optional ?q= runs the product
// search server-side and embeds the results directly in the page — the no-JS fallback
// for the picker, which htmx otherwise enhances into a live, in-place search against
// BuscarProductos below.
func (q *Quotes) Builder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	quote := q.loadQuoteOrNotFound(w, r)
	if quote == nil {
		return
	}
	persisted, err := q.store.ListQuoteLines(ctx, quote.ID)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	settings, err := q.loadPricingSettings(ctx)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	lines := q.computeQuoteLines(ctx, inputsFromPersisted(persisted), settings)
	totals := computeQuoteTotals(lines)

	searchQuery := strings.TrimSpace(r.URL.Query().Get("q"))
	searchResults, err := q.productSearchResults(ctx, searchQuery)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}

	successMsg := ""
	if r.URL.Query().Get("guardado") == "1" {
		successMsg = "Borrador guardado."
	}
	user, _ := UserFromContext(ctx)
	views.QuoteBuilder(*quote, lines, totals, successMsg, searchQuery, searchResults, navUserView(user)).Render(ctx, w)
}

// Recalcular handles POST /cotizaciones/{folio}/recalcular: the endpoint behind every
// add/remove/qty-change in the builder. It never writes to the database — it reparses
// the submitted line set, recomputes prices and totals, and re-renders. An htmx request
// gets just the line-table+totals fragment swapped in place; a plain form submit (JS
// disabled) gets the full builder page re-rendered with the same edited-but-unsaved
// state, so editing still works end to end without JS — a bare fragment would otherwise
// leave a no-JS user on a broken, un-navigable page.
func (q *Quotes) Recalcular(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	quote := q.loadQuoteOrNotFound(w, r)
	if quote == nil {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "solicitud inválida", http.StatusBadRequest)
		return
	}
	settings, err := q.loadPricingSettings(ctx)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	lines := q.computeQuoteLines(ctx, parseQuoteLineInputs(r), settings)
	totals := computeQuoteTotals(lines)

	if r.Header.Get("HX-Request") == "true" {
		views.QuoteLinesFragment(*quote, lines, totals).Render(ctx, w)
		return
	}
	user, _ := UserFromContext(ctx)
	views.QuoteBuilder(*quote, lines, totals, "", "", nil, navUserView(user)).Render(ctx, w)
}

// BuscarProductos handles GET /cotizaciones/{folio}/productos: the htmx-driven live
// product search behind the builder's "add line" picker (Builder's own ?q= handling is
// the no-JS fallback for the same search). Not scoped by the quote's own prefix — per
// the user's decision, any product may go on any quote.
func (q *Quotes) BuscarProductos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if quote := q.loadQuoteOrNotFound(w, r); quote == nil {
		return
	}
	products, err := q.productSearchResults(ctx, strings.TrimSpace(r.URL.Query().Get("q")))
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	views.ProductSearchResults(products).Render(ctx, w)
}

// Guardar handles POST /cotizaciones/{folio}/guardar: the one and only place a draft's
// lines get written, per the M2.2 design (no per-edit log row). Re-renders the full
// builder with row-level errors, without saving, if any line is currently invalid.
func (q *Quotes) Guardar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	quote := q.loadQuoteOrNotFound(w, r)
	if quote == nil {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "solicitud inválida", http.StatusBadRequest)
		return
	}
	settings, err := q.loadPricingSettings(ctx)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	lines := q.computeQuoteLines(ctx, parseQuoteLineInputs(r), settings)
	totals := computeQuoteTotals(lines)

	for _, l := range lines {
		if l.Error != "" {
			user, _ := UserFromContext(ctx)
			w.WriteHeader(http.StatusUnprocessableEntity)
			views.QuoteBuilder(*quote, lines, totals, "", "", nil, navUserView(user)).Render(ctx, w)
			return
		}
	}

	storeLines := make([]store.QuoteLine, len(lines))
	for i, l := range lines {
		storeLines[i] = store.QuoteLine{
			ProductID:           l.ProductID,
			DescriptionSnapshot: l.Description,
			QtyMilli:            l.QtyMilli,
			UnitPriceMicros:     l.UnitPriceMicros,
			LineTotal:           l.LineTotal,
			PricingInputsJSON:   l.PricingInputsJSON,
			Source:              "manual",
		}
	}
	if err := q.store.ReplaceQuoteLines(ctx, quote.ID, storeLines, totals); err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/cotizaciones/%s?guardado=1", quote.Folio), http.StatusSeeOther)
}
