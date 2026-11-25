package web

import (
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/pdf"
	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

// PriceLists holds the /listas-de-precios handlers (#82, slice L3 of
// docs/planning/leads-y-listas-de-precios.md). A price list is every product of one
// tipo's familia, priced exactly as a draft quote of that tipo would be: the builder's
// margin resolution (Quotes.resolveMargin) and line pricing (Quotes.computeQuoteLines)
// run on a draft that exists only in memory, and the PDF is the quote's layout with a
// price-list header. The generated PDF is kept as it is, so its prices hold for its
// vigencia.
type PriceLists struct {
	q     *Quotes
	store *store.Store
}

func NewPriceLists(q *Quotes) *PriceLists {
	return &PriceLists{q: q, store: q.store}
}

// priceListTypes are the tipos a list can be made of: those with a familia of catalog
// products.
func priceListTypes(types []store.QuoteType) []store.QuoteType {
	var out []store.QuoteType
	for _, t := range types {
		if !t.FreeLinesOnly && t.FamilyID != nil {
			out = append(out, t)
		}
	}
	return out
}

// List renders /listas-de-precios: the lists made so far and the form to start one.
func (h *PriceLists) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lists, err := h.store.ListPriceLists(ctx, 0)
	if err != nil {
		serverError(w, r, err)
		return
	}
	types, err := h.store.ListQuoteTypes(ctx)
	if err != nil {
		serverError(w, r, err)
		return
	}
	lists, pager := paginate(r, lists)
	user, _ := UserFromContext(ctx)
	views.PriceListsPage(lists, priceListTypes(types), views.ListView{Base: "/listas-de-precios", Pager: pager}, navUserView(user)).Render(ctx, w)
}

// draftOf is the in-memory draft quote of type t a price list is priced as.
func draftOf(t store.QuoteType) *store.Quote {
	return &store.Quote{
		QuoteTypeID: t.ID, Prefix: t.Prefix, Status: store.QuoteBorrador,
		SeriesFamily: t.FamilyName, SeriesTerms: t.Terms, Currency: "MXN",
		KgPriced: t.KgPriced, CopperPriceMargin: t.CopperPriceMargin,
		HideAddress: t.HideAddress, Letterhead: t.Letterhead,
	}
}

// priceListState is a price list being built: its tipo, lead, rows and pricing.
type priceListState struct {
	view    views.PriceListBuilder
	quote   *store.Quote
	margin  draftMargin
	lines   []views.QuoteLineView // the priced rows (quantity above zero), in order
	units   []string              // each priced row's unit, for the PDF's Cantidad
	invalid bool                  // a priced row has an error, or a quantity can't be read
}

// build works out the price list for the request: the tipo (?tipo= or the form's), the
// lead (?lead=), and, on a POST, the margin, quantities, recipient and delivery time
// typed. It writes a 404 and returns nil for an unknown tipo or lead.
func (h *PriceLists) build(w http.ResponseWriter, r *http.Request) *priceListState {
	ctx := r.Context()
	var form url.Values
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "solicitud inválida", http.StatusBadRequest)
			return nil
		}
		form = r.PostForm
	}
	typeID, _ := strconv.ParseInt(r.FormValue("tipo"), 10, 64)
	qt, err := h.store.QuoteTypeByID(ctx, typeID)
	if err != nil {
		serverError(w, r, err)
		return nil
	}
	if qt == nil || qt.FreeLinesOnly || qt.FamilyID == nil {
		http.NotFound(w, r)
		return nil
	}
	st := &priceListState{quote: draftOf(*qt)}
	st.view.Type = *qt
	if raw := r.FormValue("lead"); raw != "" {
		leadID, _ := strconv.ParseInt(raw, 10, 64)
		lead, err := h.store.LeadByID(ctx, leadID)
		if err != nil {
			serverError(w, r, err)
			return nil
		}
		if lead == nil {
			http.NotFound(w, r)
			return nil
		}
		st.view.Lead = lead
		st.view.Recipient = lead.Label()
	}
	if form != nil {
		st.view.Recipient = strings.TrimSpace(form.Get("para"))
		st.quote.DeliveryTime = strings.TrimSpace(form.Get("delivery_time"))
	}
	st.view.DeliveryTime = st.quote.DeliveryTime
	st.view.AsksDeliveryTime = strings.Contains(qt.Terms, store.DeliveryTimeToken)

	// A new list starts on the tipo's default margin, as a new quote does.
	opts, err := h.store.QuoteMarginOptions(ctx, qt.ID)
	if err != nil {
		serverError(w, r, err)
		return nil
	}
	for _, o := range opts {
		if o.IsDefault && o.Active() {
			id := o.ID
			st.quote.MarginOptionID = &id
		}
	}

	products, err := h.store.ListProducts(ctx, "", "awg", "asc", nil)
	if err != nil {
		serverError(w, r, err)
		return nil
	}
	var inputs []quoteLineInput
	for _, p := range products {
		if p.FamilyName != qt.FamilyName {
			continue
		}
		qty := "1"
		if form != nil {
			qty = strings.TrimSpace(form.Get("qty_" + strconv.FormatInt(p.ID, 10)))
		}
		row := views.PriceListRow{ProductID: p.ID, Label: p.SKU + " — " + p.Description, Qty: qty, UnitCode: p.UnitCode}
		if p.UnitCode == "" && !qt.KgPriced {
			st.view.NoUnit = append(st.view.NoUnit, p.SKU)
		}
		if m, err := money.ParseMilli(qty); qty == "" || (err == nil && m == 0) {
			row.Excluded = true
		} else if err != nil || m < 0 {
			row.Error = "Cantidad inválida."
			st.invalid = true
		} else {
			row.Line = len(inputs)
			inputs = append(inputs, quoteLineInput{Key: strconv.Itoa(len(inputs)), Kind: "product", ProductID: strconv.FormatInt(p.ID, 10), Qty: qty})
			st.units = append(st.units, p.UnitCode)
		}
		st.view.Rows = append(st.view.Rows, row)
	}

	st.margin, err = h.q.resolveMargin(ctx, st.quote, form, inputs)
	if err != nil {
		serverError(w, r, err)
		return nil
	}
	st.lines = h.q.computeQuoteLines(ctx, inputs, st.margin, false, qt.FamilyName, qt.KgPriced)
	for i := range st.view.Rows {
		row := &st.view.Rows[i]
		if row.Excluded || row.Error != "" {
			continue
		}
		l := st.lines[row.Line]
		row.Priced = &l
		if l.Error != "" {
			st.invalid = true
		}
	}
	st.view.Margin = st.margin.picker
	st.view.Totals = computeQuoteTotals(st.lines)
	st.view.ShowKg = views.ShowsKg(*st.quote)
	st.view.ValidUntil = store.ValidUntil(time.Now().In(store.LocationFromContext(ctx)), qt.ValidityDays)
	return st
}

// NewPage renders /listas-de-precios/nueva?tipo=…[&lead=…]: every product of the tipo's
// familia at quantity 1, on its default margin.
func (h *PriceLists) NewPage(w http.ResponseWriter, r *http.Request) {
	st := h.build(w, r)
	if st == nil {
		return
	}
	h.render(w, r, st, "", http.StatusOK)
}

func (h *PriceLists) render(w http.ResponseWriter, r *http.Request, st *priceListState, errMsg string, status int) {
	user, _ := UserFromContext(r.Context())
	st.view.Error = errMsg
	w.WriteHeader(status)
	views.PriceListBuilderPage(st.view, navUserView(user)).Render(r.Context(), w)
}

// Build handles POST /listas-de-precios/nueva: "Recalcular" prices what was typed;
// "Generar PDF" (generar=1) renders the list, keeps it, and goes to it.
func (h *PriceLists) Build(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st := h.build(w, r)
	if st == nil {
		return
	}
	if r.PostForm.Get("generar") != "1" {
		h.render(w, r, st, "", http.StatusOK)
		return
	}
	switch {
	case !st.margin.usable():
		h.render(w, r, st, "Elige un margen para la lista.", http.StatusUnprocessableEntity)
		return
	case st.invalid:
		h.render(w, r, st, "Hay productos con error. Corrige su cantidad o ponla en 0 para quitarlos de la lista.", http.StatusUnprocessableEntity)
		return
	case len(st.lines) == 0:
		h.render(w, r, st, "La lista no tiene productos: pon una cantidad en al menos uno.", http.StatusUnprocessableEntity)
		return
	case st.view.AsksDeliveryTime && st.quote.DeliveryTime == "":
		h.render(w, r, st, "Escribe el tiempo de entrega: los términos y condiciones de este tipo lo llevan.", http.StatusUnprocessableEntity)
		return
	}

	user, _ := UserFromContext(ctx)
	now := time.Now()
	qt := st.view.Type
	doc := draftQuoteDocument(*st.quote, st.lines, st.view.Totals)
	doc.PriceList = true
	doc.Folio = priceListTitle(qt)
	doc.CustomerName = st.view.Recipient
	if doc.CustomerName == "" {
		doc.CustomerName = "—"
	}
	doc.Vendedor = user.Name
	doc.Fecha = now.In(store.LocationFromContext(ctx)).Format("02/01/2006")
	doc.Vencimiento = formatFechaDate(st.view.ValidUntil)
	doc.Created = now
	doc.Watermark = h.q.watermark
	if !doc.ShowKg {
		for i := range doc.Lines {
			if u := st.units[i]; u != "" {
				doc.Lines[i].Qty += " " + u
			}
		}
	}
	data, err := pdf.RenderQuote(ctx, doc)
	if err != nil {
		serverError(w, r, err)
		return
	}

	margin := *st.margin.value()
	pl := store.NewPriceList{
		QuoteTypeID: qt.ID, FamilyName: qt.FamilyName, Recipient: st.view.Recipient, UserID: user.ID,
		MarginName: st.margin.name(), MarginMicros: margin, ValidUntil: st.view.ValidUntil,
		Lines: len(st.lines), PDF: data,
	}
	if st.view.Lead != nil {
		pl.LeadID = &st.view.Lead.ID
		pl.Note = fmt.Sprintf("Lista de precios %s: margen %s, %d productos, vigente hasta el %s.",
			priceListTitle(qt), views.MarginLabel(pl.MarginName, margin), pl.Lines, formatFechaDate(pl.ValidUntil))
	}
	id, err := h.store.CreatePriceList(ctx, pl)
	if err != nil {
		serverError(w, r, err)
		return
	}
	if st.view.Lead != nil {
		http.Redirect(w, r, fmt.Sprintf("/leads/%d?lista=%d#listas", st.view.Lead.ID, id), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/listas-de-precios?generada=%d", id), http.StatusSeeOther)
}

// priceListTitle names a tipo's list: its name ("Cable CCA"), else its familia.
func priceListTitle(t store.QuoteType) string {
	if t.Name != "" {
		return t.Name
	}
	return t.FamilyName
}

// PDF handles GET /listas-de-precios/{id}/pdf/{name}: the list as it was generated.
func (h *PriceLists) PDF(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	pl, err := h.store.PriceListByID(r.Context(), id)
	if err != nil {
		serverError(w, r, err)
		return
	}
	if pl == nil {
		http.NotFound(w, r)
		return
	}
	data, err := h.store.PriceListPDF(r.Context(), id)
	if err != nil {
		serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": views.PriceListPDFName(*pl)}))
	w.Write(data)
}
