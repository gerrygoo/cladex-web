package web

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/pdf"
	"github.com/gerrygoo/cladex-web/internal/pricing"
	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

// Quotes holds the dependencies for the /cotizaciones handlers. logger receives the
// warning PDF logs when an issued quote no longer renders to the hash it was issued
// with.
type Quotes struct {
	store  *store.Store
	logger *slog.Logger
}

// NewQuotes builds the /cotizaciones handlers; a nil logger discards.
func NewQuotes(s *store.Store, logger *slog.Logger) *Quotes {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Quotes{store: s, logger: logger}
}

// defaultValidityDays is how long an issued quote is valid for when no other input
// sets it — no UI exists yet to pick a custom validity per quote, so every issued
// quote gets this fixed window. Matches a typical commercial-quote validity period.
const defaultValidityDays = 30

// copperMaterialName is the catalog material whose sale price per kg a CCS quote's
// margin can be set by: its cost / (1 - margin) is the "copper price" salespeople quote.
const copperMaterialName = "CCS 30%"

// draftMargin is the margin a draft is priced with: a custom margin typed on the
// builder (or saved on the quote), else the quote's own margin_option_id or the one the
// builder form just submitted. Drafts follow an option live, so its current value is
// what prices them. option is nil when neither names an existing option; picker is the
// builder's margin controls for this state.
type draftMargin struct {
	option *store.MarginOption
	custom *money.Micros
	picker views.MarginPicker
}

// usable reports whether the draft can be priced and issued with its margin: it has a
// custom margin, or names an option that hasn't been retired.
func (m draftMargin) usable() bool {
	return m.custom != nil || (m.option != nil && m.option.Active())
}

// value is the margin to price with, or nil when unusable — internal/pricing then fails
// every catalog line with ErrNoMargin (free lines don't need one).
func (m draftMargin) value() *money.Micros {
	if !m.usable() {
		return nil
	}
	if m.custom != nil {
		v := *m.custom
		return &v
	}
	v := m.option.ValueMicros
	return &v
}

// name is what the margin is called in snapshots: the option's name, or "Personalizado".
func (m draftMargin) name() string {
	if m.custom != nil {
		return views.CustomMarginName
	}
	if m.option != nil {
		return m.option.Name
	}
	return ""
}

// id is the option to save onto the draft, or nil to leave its current one.
func (m draftMargin) id() *int64 {
	if m.option == nil {
		return nil
	}
	return &m.option.ID
}

// resolveMargin works out a draft's margin. form is the builder form's values (nil when
// the request didn't carry one, e.g. a page load); its margin controls win over the
// quote's saved margin so an unsaved change in the dropdown prices the lines.
func (q *Quotes) resolveMargin(ctx context.Context, quote *store.Quote, form url.Values) (draftMargin, error) {
	// A margin of its own is only offered on CCS quotes for now.
	allowCustom := quote.SeriesFamily == views.CCSFamilyName
	selected := quote.MarginOptionID
	var custom *money.Micros
	if allowCustom {
		custom = quote.CustomMarginMicros
	}
	customMode := custom != nil
	submitted := false
	if v, ok := form["margin_option_id"]; ok && len(v) > 0 {
		v0 := strings.TrimSpace(v[0])
		customMode = allowCustom && v0 == views.CustomMarginValue
		submitted = true
		custom = nil
		if id, err := strconv.ParseInt(v0, 10, 64); err == nil {
			selected = &id
		}
	}
	opts, err := q.store.ListMarginOptions(ctx)
	if err != nil {
		return draftMargin{}, fmt.Errorf("resolve margin: %w", err)
	}
	var copper *store.Material
	if !allowCustom {
		// no copper price outside CCS quotes
	} else if materials, err := q.store.ListMaterials(ctx); err != nil {
		return draftMargin{}, fmt.Errorf("resolve margin: %w", err)
	} else {
		for i := range materials {
			if materials[i].Name == copperMaterialName {
				copper = &materials[i]
			}
		}
	}

	var m draftMargin
	if selected != nil {
		for i := range opts {
			if opts[i].ID == *selected {
				m.option = &opts[i]
				break
			}
		}
	}
	if customMode {
		m.picker = views.NewMarginPicker(opts, nil)
		m.picker.NoneChosen = false
		m.picker.AllowCustom = true
		m.picker.Custom = true
		if copper != nil {
			m.picker.HasCopper = true
			m.picker.CopperName = copper.Name
			m.picker.CopperCost = views.CopperPriceText(copper.PriceMicros)
		}
		var errMsg string
		pctRaw, copperRaw := strings.TrimSpace(form.Get("margin_pct")), strings.TrimSpace(form.Get("copper_price"))
		if submitted {
			custom, errMsg = parseCustomMargin(form.Get("margin_edited"), pctRaw, copperRaw, copper)
		}
		m.custom = custom
		if custom != nil {
			m.picker.SetMarginDisplay(*custom, copper)
		} else {
			m.picker.CustomPct, m.picker.CopperPrice = pctRaw, copperRaw
			if errMsg == "" {
				errMsg = invalidCustomMarginMsg
			}
			m.picker.Error = errMsg
		}
		return m, nil
	}

	m.picker = views.NewMarginPicker(opts, m.id())
	m.picker.AllowCustom = allowCustom
	switch {
	case m.option == nil:
		m.picker.Error = "Elige un margen para esta cotización."
	case !m.option.Active():
		m.picker.Error = "El margen elegido ya no está disponible; elige otro."
	}
	if copper != nil {
		m.picker.HasCopper = true
		m.picker.CopperName = copper.Name
		m.picker.CopperCost = views.CopperPriceText(copper.PriceMicros)
	}
	// The fields show the chosen option's margin read-only, so the copper price it
	// implies is always in view.
	if allowCustom && m.option != nil {
		m.picker.SetMarginDisplay(m.option.ValueMicros, copper)
	}
	return m, nil
}

const invalidCustomMarginMsg = "Margen personalizado inválido: escribe un porcentaje de 0 a 99.9999, p. ej. 12.34."

// parseCustomMargin reads a custom margin from the builder's two fields. edited says
// which one the salesperson touched last ("copper" or anything else for the
// percentage): the other is derived from it, so they can never disagree. copper is the
// material the price field is measured against (nil when the catalog has none, in
// which case only the percentage exists). errMsg is set when the value is unusable.
func parseCustomMargin(edited, pctRaw, copperRaw string, copper *store.Material) (margin *money.Micros, errMsg string) {
	if edited == "copper" && copper != nil {
		price, err := money.ParseMicros(strings.TrimPrefix(copperRaw, "$"))
		if err != nil {
			return nil, "Precio de " + copper.Name + " inválido: escribe un monto por kg, p. ej. 220.00."
		}
		m, err := pricing.MarginFromPrice(copper.PriceMicros, price)
		if err != nil {
			return nil, "El precio de " + copper.Name + " no puede ser menor que su costo (" + views.CopperPriceText(copper.PriceMicros) + " por kg)."
		}
		return &m, ""
	}
	m, err := parseMarginPercent(pctRaw)
	if err != nil {
		return nil, invalidCustomMarginMsg
	}
	return &m, ""
}

// List renders /cotizaciones: the full page normally, or just the table body when
// called via htmx live search — same pattern as Products.List.
func (q *Quotes) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	sortCol, dir := sortParams(r)
	filters := filterParams(r)
	quotes, err := q.store.ListQuotes(ctx, query, sortCol, dir, filters)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	quotes, pager := paginate(r, quotes)
	lv := views.ListView{Base: "/cotizaciones", Query: query, Sort: sortCol, Dir: dir, Filters: filters, Pager: pager}
	if r.Header.Get("HX-Request") == "true" {
		views.QuotesTableBody(quotes).Render(ctx, w)
		views.ListPagerOOB(lv).Render(ctx, w)
		return
	}
	successMsg := ""
	if r.URL.Query().Get("creada") == "1" {
		successMsg = "Cotización creada."
	}
	customers, authors, err := q.store.QuoteFilterChoices(ctx)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	user, _ := UserFromContext(ctx)
	views.QuotesList(quotes, lv, customers, authors, successMsg, navUserView(user)).Render(ctx, w)
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

// Validate checks the form; series are the prefixes a quote can start in. Per the
// user's decision, the series is a free choice at quote-creation time — a folio series
// label, not a restriction on which products a quote can contain.
func (f QuoteNewForm) Validate(series []store.Series) map[string]string {
	errs := map[string]string{}
	if id, err := strconv.ParseInt(f.CustomerID, 10, 64); f.CustomerID == "" || err != nil || id <= 0 {
		errs["customer_id"] = "Selecciona un cliente."
	}
	if !slices.ContainsFunc(series, func(sr store.Series) bool { return sr.Prefix == f.Prefix }) {
		errs["prefix"] = "Selecciona una serie de folio."
	}
	return errs
}

// NewPage renders the empty "nueva cotización" form.
func (q *Quotes) NewPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	customers, err := q.store.ListCustomers(ctx, "", "name", "asc", nil)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	series, err := q.store.ListSeries(ctx)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	user, _ := UserFromContext(ctx)
	views.QuoteNewForm(customers, series, views.QuoteNewFormValues{}, nil, navUserView(user)).Render(ctx, w)
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
	series, err := q.store.ListSeries(ctx)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	fieldErrors := form.Validate(series)
	if len(fieldErrors) > 0 {
		customers, err := q.store.ListCustomers(ctx, "", "name", "asc", nil)
		if err != nil {
			http.Error(w, "error interno", http.StatusInternalServerError)
			return
		}
		user, _ := UserFromContext(ctx)
		w.WriteHeader(http.StatusUnprocessableEntity)
		values := views.QuoteNewFormValues{CustomerID: form.CustomerID, Prefix: form.Prefix}
		views.QuoteNewForm(customers, series, values, fieldErrors, navUserView(user)).Render(ctx, w)
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
	switch {
	case errors.Is(err, pricing.ErrNoMargin):
		return "Elige un margen disponible para esta cotización."
	case errors.Is(err, pricing.ErrNoCost):
		return "Este producto no tiene datos de precio configurados."
	default:
		return "No se pudo calcular el precio de esta línea."
	}
}

// pricingInputsSnapshot is the JSON shape stored in quote_lines.pricing_inputs — the
// inputs that produced a product line's price, per docs/PLAN.md's "Quote persistence"
// design: its flat cost and/or materials, and the margin applied to them.
type pricingInputsSnapshot struct {
	Margin       string                   `json:"margin"`
	MarginOption string                   `json:"margin_option"`
	Cost         string                   `json:"cost,omitempty"`
	Materials    []materialInputsSnapshot `json:"materials,omitempty"`
}

// materialInputsSnapshot is one material behind a line's cost, as it stood when priced.
type materialInputsSnapshot struct {
	Material   string `json:"material"`
	QtyPerUnit string `json:"qty_per_unit"`
	Unit       string `json:"unit"`
	Price      string `json:"price"`
}

// buildPricingInputsJSON records what produced a product line's price: the product's
// flat cost and materials, and the quote's margin option by name and value.
func buildPricingInputsJSON(p *store.Product, materials []store.ProductMaterial, margin money.Micros, marginName string) *string {
	snap := pricingInputsSnapshot{Margin: margin.String(), MarginOption: marginName}
	if p.CostMicros != nil {
		snap.Cost = p.CostMicros.String()
	}
	for _, m := range materials {
		snap.Materials = append(snap.Materials, materialInputsSnapshot{
			Material:   m.Material.Name,
			QtyPerUnit: m.QtyPerUnitMicros.String(),
			Unit:       m.Material.UnitCode,
			Price:      m.Material.PriceMicros.String(),
		})
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
// product, no cost, no usable margin) gets a row-level Error instead of failing the
// whole request.
func (q *Quotes) computeQuoteLines(ctx context.Context, inputs []quoteLineInput, margin draftMargin) []views.QuoteLineView {
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
		materials, err := q.store.ListProductMaterials(ctx, productID)
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

		pp := pricing.Product{CostMicros: product.CostMicros}
		for _, m := range materials {
			pp.Materials = append(pp.Materials, pricing.MaterialContent{QtyPerUnit: m.QtyPerUnitMicros, Price: m.Material.PriceMicros})
			v.Materials = append(v.Materials, m.Material)
		}
		unitPrice, err := pricing.UnitPrice(pp, margin.value())
		if err != nil {
			v.Error = friendlyPricingError(err)
			result[i] = v
			continue
		}
		v.UnitPriceMicros = unitPrice
		v.LineTotal = money.LineTotalCentavos(unitPrice, qty)
		v.PricingInputsJSON = buildPricingInputsJSON(product, materials, *margin.value(), margin.name())
		result[i] = v
	}
	return result
}

// frozenQuoteLines renders an issued or revised quote's lines exactly as stored at issue
// time — description snapshot, quantity, unit price, line total — without consulting
// the live catalog or settings. Only drafts are ever recomputed; past 'borrador' the
// page shows what the customer received (the same numbers as the stored PDF), even if a
// quoted product has since been repriced or deleted.
func frozenQuoteLines(persisted []store.QuoteLine) []views.QuoteLineView {
	out := make([]views.QuoteLineView, len(persisted))
	for i, l := range persisted {
		out[i] = views.QuoteLineView{
			Key:             strconv.Itoa(i),
			IsFree:          l.ProductID == nil,
			ProductID:       l.ProductID,
			ProductLabel:    l.DescriptionSnapshot,
			Description:     l.DescriptionSnapshot,
			QtyMilli:        l.QtyMilli,
			UnitPriceMicros: l.UnitPriceMicros,
			LineTotal:       l.LineTotal,
		}
	}
	return out
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

// productPageSize is how many products the builder's picker shows per page.
const productPageSize = 10

// productPicker builds the builder's "add product" picker for either the full builder
// page (initial load and every no-JS re-render) or the htmx results fragment. An empty
// query browses: it lists every product in the series family (or the whole catalog with
// the family toggle off) so a vendedor can pick without typing. Results are paginated
// productPageSize at a time via ?pagina=, clamped to the available range. It reads
// r.Form, so q/solo_familia/pagina come from either the URL (GET) or a POSTed builder
// form, keeping the picker's state across a no-JS Recalcular.
func (q *Quotes) productPicker(ctx context.Context, r *http.Request, quote *store.Quote) (views.ProductPicker, error) {
	if err := r.ParseForm(); err != nil {
		return views.ProductPicker{}, err
	}
	soloFamilia := soloFamiliaParam(r)
	picker := views.ProductPicker{
		Folio:       quote.Folio,
		Query:       strings.TrimSpace(r.Form.Get("q")),
		SoloFamilia: soloFamilia,
		Page:        1,
	}
	// Gauge (AWG) order, matching the Productos page default (see store.ListProducts).
	products, err := q.store.ListProducts(ctx, picker.Query, "awg", "asc", nil)
	if err != nil {
		return views.ProductPicker{}, err
	}
	if family := searchFamily(quote, soloFamilia); family != "" {
		filtered := products[:0]
		for _, p := range products {
			if p.FamilyName == family {
				filtered = append(filtered, p)
			}
		}
		products = filtered
	}
	picker.Total = len(products)
	picker.TotalPages = max(1, (len(products)+productPageSize-1)/productPageSize)
	if n, err := strconv.Atoi(r.Form.Get("pagina")); err == nil {
		picker.Page = min(max(n, 1), picker.TotalPages)
	}
	start := (picker.Page - 1) * productPageSize
	picker.Products = products[start:min(start+productPageSize, len(products))]
	return picker, nil
}

// soloFamiliaParam reads the picker's "solo productos de la familia" toggle, which
// defaults to on. The form pairs a hidden solo_familia=0 with the checkbox's
// solo_familia=1, so an unchecked box still sends "0" — only a request that carries
// neither (first page load) falls back to the default. Reads r.Form, so the caller
// must have parsed it.
func soloFamiliaParam(r *http.Request) bool {
	values := r.Form["solo_familia"]
	if len(values) == 0 {
		return true
	}
	return slices.Contains(values, "1")
}

// searchFamily is the family the builder's product search filters to: the quote's
// series family while the toggle is on, or "" (no filter) otherwise.
func searchFamily(quote *store.Quote, soloFamilia bool) string {
	if !soloFamilia {
		return ""
	}
	return quote.SeriesFamily
}

// Builder renders the full quote-builder page at GET /cotizaciones/{folio}, starting
// from whatever's currently persisted in quote_lines. The product picker is rendered
// server-side into the page — with no ?q= it lists page 1 of the series family — and
// ?q=/?pagina= are the no-JS fallback for searching and paging, which htmx otherwise
// enhances into in-place requests against BuscarProductos below.
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
	var lines []views.QuoteLineView
	var totals pricing.Totals
	var margin draftMargin
	if quote.Status == "borrador" {
		margin, err = q.resolveMargin(ctx, quote, nil)
		if err != nil {
			http.Error(w, "error interno", http.StatusInternalServerError)
			return
		}
		lines = q.computeQuoteLines(ctx, inputsFromPersisted(persisted), margin)
		totals = computeQuoteTotals(lines)
	} else {
		lines = frozenQuoteLines(persisted)
		totals = pricing.Totals{Subtotal: quote.Subtotal, IVA: quote.IVA, Total: quote.Total}
	}

	successMsg := ""
	switch {
	case r.URL.Query().Get("guardado") == "1":
		successMsg = "Borrador guardado."
	case r.URL.Query().Get("emitida") == "1":
		successMsg = "Cotización emitida."
	}
	q.renderBuilder(w, r, quote, lines, totals, margin.picker, successMsg, http.StatusOK, nil)
}

// maxDeliveryTime caps the free-text delivery time, which is printed on the PDF.
const maxDeliveryTime = 120

// takeTermsInputs reads the builder form's per-quote inputs onto quote for this request
// (the caller decides whether to save them): the delivery time and currency when the
// series' terms hold their tokens, and the validity date for a free-lines-only series.
// Inputs a series doesn't ask for are left untouched.
func takeTermsInputs(r *http.Request, quote *store.Quote) {
	if store.RequiresDeliveryTime(quote.SeriesTerms) {
		v := strings.TrimSpace(r.FormValue("delivery_time"))
		if runes := []rune(v); len(runes) > maxDeliveryTime {
			v = string(runes[:maxDeliveryTime])
		}
		quote.DeliveryTime = v
	}
	if store.RequiresCurrency(quote.SeriesTerms) {
		quote.Currency = ""
		if c := r.FormValue("currency"); store.ValidCurrency(c) {
			quote.Currency = c
		}
	}
	if quote.SeriesFreeLines {
		quote.ValidUntil = nil
		// An unparseable date (a browser without date inputs) is kept out of the row;
		// Emitir then asks for it again.
		if v := strings.TrimSpace(r.FormValue("valid_until")); v != "" {
			if _, err := time.Parse("2006-01-02", v); err == nil {
				quote.ValidUntil = &v
			}
		}
	}
}

// termsInputs is the quote's per-quote values as the terms tokens take them.
func termsInputs(quote store.Quote) store.TermsInputs {
	return store.TermsInputs{DeliveryTime: quote.DeliveryTime, Currency: quote.Currency}
}

// saveTermsInputs persists what takeTermsInputs read.
func (q *Quotes) saveTermsInputs(ctx context.Context, quote *store.Quote) error {
	validUntil := ""
	if quote.ValidUntil != nil {
		validUntil = *quote.ValidUntil
	}
	return q.store.SetQuoteTermsInputs(ctx, quote.ID, quote.DeliveryTime, quote.Currency, validUntil)
}

// missingTermsInputs lists, by form field, what the quote's series still needs before it
// can be issued. Empty means it can be.
func missingTermsInputs(quote *store.Quote, today time.Time) map[string]string {
	missing := map[string]string{}
	if store.RequiresDeliveryTime(quote.SeriesTerms) && quote.DeliveryTime == "" {
		missing["delivery_time"] = "Escribe el tiempo de entrega para poder emitir la cotización."
	}
	if store.RequiresCurrency(quote.SeriesTerms) && quote.Currency == "" {
		missing["currency"] = "Elige la moneda (MXN o USD) para poder emitir la cotización."
	}
	if quote.SeriesFreeLines {
		switch {
		case quote.ValidUntil == nil:
			missing["valid_until"] = "Escribe hasta qué fecha es vigente la cotización para poder emitirla."
		case *quote.ValidUntil < today.Format("2006-01-02"):
			missing["valid_until"] = "La vigencia no puede ser anterior a hoy."
		}
	}
	return missing
}

// renderBuilder renders the full builder page with the given (possibly unsaved) lines,
// carrying the product picker's q/solo_familia/pagina over from the request so a no-JS
// round trip keeps the picker where the user left it.
func (q *Quotes) renderBuilder(w http.ResponseWriter, r *http.Request, quote *store.Quote, lines []views.QuoteLineView, totals pricing.Totals, margin views.MarginPicker, successMsg string, status int, fieldErrs map[string]string) {
	ctx := r.Context()
	var picker views.ProductPicker
	if quote.Status == "borrador" && !quote.SeriesFreeLines {
		var err error
		picker, err = q.productPicker(ctx, r, quote)
		if err != nil {
			http.Error(w, "error interno", http.StatusInternalServerError)
			return
		}
	}
	comments, err := q.store.ListQuoteComments(ctx, quote.ID)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	comments, pager := paginate(r, comments)
	commentsLV := views.ListView{Base: "/cotizaciones/" + quote.Folio, Pager: pager, Anchor: "comentarios"}
	user, _ := UserFromContext(ctx)
	w.WriteHeader(status)
	views.QuoteBuilder(*quote, lines, totals, margin, successMsg, fieldErrs, picker, comments, commentsLV, navUserView(user)).Render(ctx, w)
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
	if quote.Status != "borrador" {
		http.Error(w, "esta cotización ya no es editable", http.StatusConflict)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "solicitud inválida", http.StatusBadRequest)
		return
	}
	margin, err := q.resolveMargin(ctx, quote, r.Form)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	takeTermsInputs(r, quote)
	lines := q.computeQuoteLines(ctx, parseQuoteLineInputs(r), margin)
	totals := computeQuoteTotals(lines)

	if r.Header.Get("HX-Request") == "true" {
		views.QuoteLinesFragment(*quote, lines, totals, margin.picker.Error).Render(ctx, w)
		return
	}
	q.renderBuilder(w, r, quote, lines, totals, margin.picker, "", http.StatusOK, nil)
}

// BuscarProductos handles GET /cotizaciones/{folio}/productos: the htmx-driven live
// product search behind the builder's "add line" picker (Builder's own ?q= handling is
// the no-JS fallback for the same search). Defaults to the quote's series family, but
// the toggle can lift that — per the user's decision, any product may go on any quote.
func (q *Quotes) BuscarProductos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	quote := q.loadQuoteOrNotFound(w, r)
	if quote == nil {
		return
	}
	picker, err := q.productPicker(ctx, r, quote)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	views.ProductSearchResults(picker).Render(ctx, w)
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
	if quote.Status != "borrador" {
		http.Error(w, "esta cotización ya no es editable", http.StatusConflict)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "solicitud inválida", http.StatusBadRequest)
		return
	}
	margin, err := q.resolveMargin(ctx, quote, r.Form)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	takeTermsInputs(r, quote)
	lines := q.computeQuoteLines(ctx, parseQuoteLineInputs(r), margin)
	totals := computeQuoteTotals(lines)

	for _, l := range lines {
		if l.Error != "" {
			q.renderBuilder(w, r, quote, lines, totals, margin.picker, "", http.StatusUnprocessableEntity, nil)
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
	if err := q.store.ReplaceQuoteLines(ctx, quote.ID, margin.id(), margin.custom, storeLines, totals); err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	if err := q.saveTermsInputs(ctx, quote); err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/cotizaciones/%s?guardado=1", quote.Folio), http.StatusSeeOther)
}

// formatFecha renders an ISO-8601 UTC timestamp (SQLite's strftime default, e.g.
// quotes.created_at or issued_at) as a plain DD/MM/YYYY date. Falls back to the raw
// string on a parse miss rather than failing the whole PDF over a display date.
func formatFecha(iso string) string {
	for _, layout := range []string{"2006-01-02T15:04:05.000Z", time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, iso); err == nil {
			return t.Format("02/01/2006")
		}
	}
	return iso
}

// formatFechaDate renders a plain YYYY-MM-DD date (quotes.valid_until's format) as
// DD/MM/YYYY, matching formatFecha's display. Empty/unparseable input renders as "".
func formatFechaDate(isoDate string) string {
	if t, err := time.Parse("2006-01-02", isoDate); err == nil {
		return t.Format("02/01/2006")
	}
	return ""
}

// docLinesFrom converts computed line views into the PDF package's already-formatted
// shape, skipping any row that failed to price (same rule computeQuoteTotals uses).
func docLinesFrom(lines []views.QuoteLineView) []pdf.QuoteLineDoc {
	docLines := make([]pdf.QuoteLineDoc, 0, len(lines))
	for _, l := range lines {
		if l.Error != "" {
			continue
		}
		docLines = append(docLines, pdf.QuoteLineDoc{
			Description: l.Description,
			Qty:         l.QtyMilli.String(),
			UnitPrice:   l.UnitPriceMicros.ToCentavosHalfUp().String(),
			Total:       l.LineTotal.String(),
		})
	}
	return docLines
}

// draftQuoteDocument assembles a draft's live PDF preview from its freshly computed
// lines and totals: current customer and salesperson names, current per-prefix terms,
// no vencimiento yet.
func draftQuoteDocument(quote store.Quote, lines []views.QuoteLineView, totals pricing.Totals) pdf.QuoteDocument {
	vencimiento := ""
	if quote.ValidUntil != nil {
		vencimiento = formatFechaDate(*quote.ValidUntil)
	}
	return pdf.QuoteDocument{
		Vencimiento:  vencimiento,
		Currency:     quote.Currency,
		Folio:        quote.Folio,
		CustomerName: quote.CustomerName,
		Vendedor:     quote.UserName,
		Fecha:        formatFecha(quote.CreatedAt),
		Lines:        docLinesFrom(lines),
		Subtotal:     totals.Subtotal.String(),
		IVA:          totals.IVA.String(),
		Total:        totals.Total.String(),
		Terms:        store.RenderTerms(quote.SeriesTerms, termsInputs(quote)),
	}
}

// issuedQuoteDocument assembles an issued or revised quote's PDF purely from what
// IssueQuote froze: stored lines and totals, name and terms snapshots, valid_until, and
// issued_at as the embedded creation date. Nothing live (catalog, settings, current
// names) is read, so the same frozen row always renders the same bytes, for a given
// template and Typst version. Emitir and PDF both go through here, which is what
// makes a reprint match the hash recorded at issue time.
func issuedQuoteDocument(quote store.Quote, persisted []store.QuoteLine) (pdf.QuoteDocument, error) {
	if quote.IssuedAt == nil || quote.TermsSnapshot == nil || quote.CustomerNameSnapshot == nil || quote.VendedorSnapshot == nil {
		return pdf.QuoteDocument{}, fmt.Errorf("quote %s is missing its issue-time snapshot", quote.Folio)
	}
	issuedAt, err := time.Parse(time.RFC3339Nano, *quote.IssuedAt)
	if err != nil {
		return pdf.QuoteDocument{}, fmt.Errorf("quote %s: parse issued_at: %w", quote.Folio, err)
	}
	vencimiento := ""
	if quote.ValidUntil != nil {
		vencimiento = formatFechaDate(*quote.ValidUntil)
	}
	return pdf.QuoteDocument{
		Folio:        quote.Folio,
		CustomerName: *quote.CustomerNameSnapshot,
		Vendedor:     *quote.VendedorSnapshot,
		Fecha:        formatFecha(quote.CreatedAt),
		Vencimiento:  vencimiento,
		Currency:     quote.Currency,
		Lines:        docLinesFrom(frozenQuoteLines(persisted)),
		Subtotal:     quote.Subtotal.String(),
		IVA:          quote.IVA.String(),
		Total:        quote.Total.String(),
		Terms:        store.SplitTerms(*quote.TermsSnapshot),
		Created:      issuedAt,
	}, nil
}

// PDF handles GET /cotizaciones/{folio}/pdf. Nothing is stored on disk: an issued or
// revised quote is re-rendered from its frozen row via issuedQuoteDocument, so its
// numbers, names and terms never change however settings, the catalog or customer
// records change afterward (a template or Typst change can still alter the layout;
// that shows up as a hash mismatch, logged here). A draft renders a live preview
// through the same computeQuoteLines path as the builder and Guardar, so it always
// matches what's on screen.
func (q *Quotes) PDF(w http.ResponseWriter, r *http.Request) {
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

	var doc pdf.QuoteDocument
	if quote.Status == "borrador" {
		margin, err := q.resolveMargin(ctx, quote, nil)
		if err != nil {
			http.Error(w, "error interno", http.StatusInternalServerError)
			return
		}
		lines := q.computeQuoteLines(ctx, inputsFromPersisted(persisted), margin)
		doc = draftQuoteDocument(*quote, lines, computeQuoteTotals(lines))
	} else {
		doc, err = issuedQuoteDocument(*quote, persisted)
		if err != nil {
			q.logger.Error("issued quote pdf", slog.Any("err", err))
			http.Error(w, "error interno", http.StatusInternalServerError)
			return
		}
	}

	bytes, err := pdf.RenderQuote(ctx, doc)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	if quote.PDFSHA256 != nil && sha256Hex(bytes) != *quote.PDFSHA256 {
		q.logger.Warn("issued quote pdf differs from the one issued; template or typst changed since",
			slog.String("folio", quote.Folio))
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s.pdf"`, quote.Folio))
	w.Write(bytes)
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Emitir handles POST /cotizaciones/{folio}/emitir: the "Emitir cotización" button on
// the builder form, which submits the same full line set Guardar does. It saves that
// state (so issuing also captures any not-yet-saved edit) and then freezes the quote:
// locks the margin, terms text, and customer and salesperson names actually used,
// renders the PDF once to record its SHA-256, and flips status to 'emitida' via
// store.IssueQuote. The PDF isn't stored; PDF regenerates it from the frozen row. Only
// valid on a draft; re-renders the builder with row-level errors, without writing
// anything, if a line is invalid (the same rule Guardar follows) or the margin option
// is missing or retired.
func (q *Quotes) Emitir(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	quote := q.loadQuoteOrNotFound(w, r)
	if quote == nil {
		return
	}
	if quote.Status != "borrador" {
		http.Error(w, "esta cotización ya fue emitida", http.StatusConflict)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "solicitud inválida", http.StatusBadRequest)
		return
	}
	margin, err := q.resolveMargin(ctx, quote, r.Form)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	takeTermsInputs(r, quote)
	lines := q.computeQuoteLines(ctx, parseQuoteLineInputs(r), margin)
	totals := computeQuoteTotals(lines)

	if missing := missingTermsInputs(quote, time.Now()); len(missing) > 0 {
		q.renderBuilder(w, r, quote, lines, totals, margin.picker, "", http.StatusUnprocessableEntity, missing)
		return
	}
	if len(lines) == 0 || !margin.usable() {
		q.renderBuilder(w, r, quote, lines, totals, margin.picker, "", http.StatusUnprocessableEntity, nil)
		return
	}
	for _, l := range lines {
		if l.Error != "" {
			q.renderBuilder(w, r, quote, lines, totals, margin.picker, "", http.StatusUnprocessableEntity, nil)
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
	if err := q.store.ReplaceQuoteLines(ctx, quote.ID, margin.id(), margin.custom, storeLines, totals); err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}

	if err := q.saveTermsInputs(ctx, quote); err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}

	validUntil := time.Now().AddDate(0, 0, defaultValidityDays).Format("2006-01-02")
	if quote.SeriesFreeLines {
		// missingTermsInputs has already checked it is set.
		validUntil = *quote.ValidUntil
	}
	issue := store.Issue{
		// Whole seconds: that's the resolution Typst embeds, and the one reprints
		// parse back out of issued_at.
		IssuedAt:             time.Now().UTC().Truncate(time.Second).Format("2006-01-02T15:04:05.000Z"),
		TermsSnapshot:        strings.Join(store.RenderTerms(quote.SeriesTerms, termsInputs(*quote)), "\n"),
		ValidUntil:           &validUntil,
		CustomerNameSnapshot: quote.CustomerName,
		VendedorSnapshot:     quote.UserName,
		MarginName:           margin.name(),
		MarginMicros:         *margin.value(),
	}

	// Render from the quote exactly as it's about to be frozen, through the same
	// issuedQuoteDocument path reprints use, so the recorded hash is a reprint's hash.
	frozen := *quote
	frozen.IssuedAt = &issue.IssuedAt
	frozen.TermsSnapshot = &issue.TermsSnapshot
	frozen.ValidUntil = issue.ValidUntil
	frozen.CustomerNameSnapshot = &issue.CustomerNameSnapshot
	frozen.VendedorSnapshot = &issue.VendedorSnapshot
	frozen.Subtotal, frozen.IVA, frozen.Total = totals.Subtotal, totals.IVA, totals.Total
	doc, err := issuedQuoteDocument(frozen, storeLines)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	pdfBytes, err := pdf.RenderQuote(ctx, doc)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	issue.PDFSHA256 = sha256Hex(pdfBytes)

	if err := q.store.IssueQuote(ctx, quote.ID, issue); err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/cotizaciones/"+quote.Folio+"?emitida=1", http.StatusSeeOther)
}

// Revisar handles POST /cotizaciones/{folio}/revisar: creates an editable revision of
// an issued quote (a new "<folio>-R<n>" draft, pre-populated with the original's
// lines) and redirects into its builder. Only valid on the currently-active issued
// quote in a lineage — see store.CreateRevision / store.ErrQuoteNotIssued.
func (q *Quotes) Revisar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	quote := q.loadQuoteOrNotFound(w, r)
	if quote == nil {
		return
	}
	user, _ := UserFromContext(ctx)
	rev, err := q.store.CreateRevision(ctx, quote.ID, user.ID)
	if err != nil {
		if errors.Is(err, store.ErrQuoteNotIssued) {
			http.Error(w, "solo se puede revisar una cotización emitida", http.StatusConflict)
			return
		}
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/cotizaciones/"+rev.Folio, http.StatusSeeOther)
}

// Etapa handles POST /cotizaciones/{folio}/etapa: moves an issued quote one stage along
// the pipeline flow ("to" is the stage; the optional "note" is saved as a comment).
// Anyone signed in can move a quote forward; going back a stage is for admins, since it
// undoes what a salesperson reported. Anything but the stage right before or after the
// quote's current one is refused, as is a stale click on a quote that has since moved.
func (q *Quotes) Etapa(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	quote := q.loadQuoteOrNotFound(w, r)
	if quote == nil {
		return
	}
	to := r.FormValue("to")
	note := strings.TrimSpace(r.FormValue("note"))
	if utf8.RuneCountInString(note) > maxCommentLen {
		http.Error(w, "el comentario no puede pasar de 2000 caracteres", http.StatusBadRequest)
		return
	}
	user, _ := UserFromContext(ctx)
	if to != "" && to == store.PrevStatus(quote.Status) && user.Role != "admin" {
		http.Error(w, "solo un administrador puede regresar una cotización de etapa", http.StatusForbidden)
		return
	}
	if err := q.store.MoveQuote(ctx, quote.ID, user.ID, quote.Status, to, note); err != nil {
		if errors.Is(err, store.ErrBadTransition) {
			http.Error(w, "la cotización no puede pasar a esa etapa", http.StatusConflict)
			return
		}
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/cotizaciones/"+quote.Folio+"#comentarios", http.StatusSeeOther)
}

// maxCommentLen caps a comment; the textarea carries the same maxlength.
const maxCommentLen = 2000

// Comentar handles POST /cotizaciones/{folio}/comentarios: any signed-in user adds a
// follow-up note to a quote in any status. Comments are append-only, so this is the
// only write there is.
func (q *Quotes) Comentar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	quote := q.loadQuoteOrNotFound(w, r)
	if quote == nil {
		return
	}
	body := strings.TrimSpace(r.FormValue("comment"))
	if body == "" || utf8.RuneCountInString(body) > maxCommentLen {
		http.Error(w, "el comentario no puede estar vacío ni pasar de 2000 caracteres", http.StatusBadRequest)
		return
	}
	user, _ := UserFromContext(ctx)
	if err := q.store.AddQuoteComment(ctx, quote.ID, user.ID, body); err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/cotizaciones/"+quote.Folio+"#comentarios", http.StatusSeeOther)
}
