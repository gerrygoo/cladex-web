package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gerrygoo/cladex-web/internal/pdf"
	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

// PurchaseOrders holds the handlers of a proyecto's órdenes de compra a proveedor
// (issue #65), under /proyectos/{folio}/ordenes-de-compra. The proyecto's vendedor or an
// admin generates and edits a borrador; only an admin confirms one (the router gates
// it) or opens a revision. Anyone signed in can see them and their PDFs, which never
// show a sale price or a margin.
type PurchaseOrders struct {
	store *store.Store
	// watermark marks every PDF of a test instance (see Quotes.watermark).
	watermark string
}

func NewPurchaseOrders(s *store.Store) *PurchaseOrders {
	return &PurchaseOrders{store: s}
}

// draftWatermark is printed across a borrador's preview.
const draftWatermark = "BORRADOR"

// canEditPO reports whether user may generate and edit the proyecto's borradores: its
// vendedor or an admin.
func canEditPO(user *store.AuthenticatedUser, project *store.Project) bool {
	return user != nil && (user.Role == store.RoleAdmin || user.ID == project.UserID)
}

// projectPOs builds the proyecto page's section.
func projectPOs(ctx context.Context, s *store.Store, user *store.AuthenticatedUser, project *store.Project, quote *store.Quote) (views.ProjectPOs, error) {
	orders, err := s.ListProjectPurchaseOrders(ctx, project.ID)
	if err != nil {
		return views.ProjectPOs{}, err
	}
	sec := views.ProjectPOs{Orders: orders, Show: len(orders) > 0 || store.ProjectTakesPurchaseOrders(project.Status)}
	if !store.ProjectTakesPurchaseOrders(project.Status) || quote == nil {
		return sec, nil
	}
	inForce := false
	for _, o := range orders {
		if o.Status == store.POBorrador || o.Status == store.POConfirmada {
			inForce = true
		}
	}
	switch {
	case inForce && !quote.SeriesFreeLines:
		// One O.C. per proyecto outside QL; it is corrected by a revision.
	case !canEditPO(user, project):
		sec.Hint = "El vendedor del proyecto o un administrador genera la orden de compra."
	default:
		sec.CanGenerate, sec.Another = true, inForce
	}
	return sec, nil
}

// loadPO finds the proyecto and its O.C. named by the path, writing a 404 and
// returning nils if either is missing or they don't belong together.
func (h *PurchaseOrders) loadPO(w http.ResponseWriter, r *http.Request) (*store.Project, *store.PurchaseOrder) {
	ctx := r.Context()
	project, err := h.store.ProjectByFolio(ctx, r.PathValue("folio"))
	if err != nil {
		serverError(w, r, err)
		return nil, nil
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var po *store.PurchaseOrder
	if project != nil {
		if po, err = h.store.PurchaseOrderByID(ctx, id); err != nil {
			serverError(w, r, err)
			return nil, nil
		}
	}
	if project == nil || po == nil || po.ProjectID != project.ID {
		http.NotFound(w, r)
		return nil, nil
	}
	return project, po
}

// Generar handles POST /proyectos/{folio}/ordenes-de-compra: a new borrador from the
// proyecto's current quote. Outside QL it takes every partida with a cost and the
// familia's default supplier; on QL the partidas no other O.C. has yet, and the supplier
// of the quote's "Cotización del proveedor" when it matches one in the catalog.
func (h *PurchaseOrders) Generar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project, err := h.store.ProjectByFolio(ctx, r.PathValue("folio"))
	if err != nil {
		serverError(w, r, err)
		return
	}
	if project == nil {
		http.NotFound(w, r)
		return
	}
	user, _ := UserFromContext(ctx)
	if !canEditPO(user, project) {
		http.Error(w, "solo el vendedor del proyecto o un administrador puede generar la orden de compra", http.StatusForbidden)
		return
	}
	quote, err := h.store.QuoteByID(ctx, project.CurrentQuoteID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	if !store.ProjectTakesPurchaseOrders(project.Status) || quote == nil || quote.Status != store.QuoteEmitida {
		http.Error(w, "la orden de compra al proveedor se genera cuando ya se recibió la orden de compra del cliente", http.StatusConflict)
		return
	}
	var supplierID *int64
	if quote.SeriesFreeLines {
		sq, err := h.store.SupplierQuoteOf(ctx, quote.ID)
		if err != nil {
			serverError(w, r, err)
			return
		}
		if sq != nil {
			if supplierID, err = h.store.SupplierByName(ctx, sq.Supplier); err != nil {
				serverError(w, r, err)
				return
			}
		}
	} else if supplierID, err = h.store.DefaultSupplierOfQuoteType(ctx, quote.QuoteTypeID); err != nil {
		serverError(w, r, err)
		return
	}
	draft := store.PODraft{SupplierID: supplierID}
	if supplierID != nil {
		sup, err := h.store.SupplierByID(ctx, *supplierID)
		if err != nil {
			serverError(w, r, err)
			return
		}
		if sup != nil {
			draft.CreditDays = sup.CreditDays
		}
	}
	quoteLines, err := h.store.ListQuoteLines(ctx, quote.ID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	taken, err := h.store.ActivePOLines(ctx, project.ID, 0)
	if err != nil {
		serverError(w, r, err)
		return
	}
	for _, ql := range quoteLines {
		if _, ok := taken[ql.LineNo]; ok {
			continue
		}
		if l, ok := store.LineFromQuote(ql, views.ShowsKg(*quote)); ok {
			draft.Lines = append(draft.Lines, l)
		}
	}
	id, err := h.store.CreatePurchaseOrder(ctx, project.ID, quote.ID, user.ID, quote.SeriesFreeLines, draft)
	if errors.Is(err, store.ErrPONotAllowed) {
		http.Error(w, "este proyecto ya tiene su orden de compra al proveedor", http.StatusConflict)
		return
	}
	if err != nil {
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, views.POPath(project.Folio, id, ""), http.StatusSeeOther)
}

// Page handles GET /proyectos/{folio}/ordenes-de-compra/{id}.
func (h *PurchaseOrders) Page(w http.ResponseWriter, r *http.Request) {
	project, po := h.loadPO(w, r)
	if po == nil {
		return
	}
	page, err := h.page(r.Context(), project, po, nil)
	if err != nil {
		serverError(w, r, err)
		return
	}
	q := r.URL.Query()
	switch {
	case q.Get("confirmada") == "1":
		page.SuccessMsg = "Orden de compra confirmada: " + po.Number + "."
		page.Download = views.POPDFPath(project.Folio, po.ID, true)
	case q.Get("guardado") == "1":
		page.SuccessMsg = "Borrador guardado."
	case q.Get("proveedor") == "1":
		page.SuccessMsg = "Proveedor dado de alta y elegido."
	case q.Get("revision") == "1":
		page.SuccessMsg = "Revisión creada. Corrige lo que haga falta y confírmala."
	}
	h.render(w, r, http.StatusOK, page)
}

func (h *PurchaseOrders) render(w http.ResponseWriter, r *http.Request, status int, page views.PurchaseOrderPage) {
	user, _ := UserFromContext(r.Context())
	w.WriteHeader(status)
	views.PurchaseOrderView(page, navUserView(user)).Render(r.Context(), w)
}

// poForm is a borrador's form as posted.
type poForm struct {
	values  views.POFormValues
	draft   store.PODraft
	chosen  map[int]bool
	errors  map[string]string
	quote   *store.Quote
	byQuote []store.QuoteLine
}

// page builds the O.C.'s page. form, when not nil, is a refused post to show again.
func (h *PurchaseOrders) page(ctx context.Context, project *store.Project, po *store.PurchaseOrder, form *poForm) (views.PurchaseOrderPage, error) {
	user, _ := UserFromContext(ctx)
	quote, err := h.store.QuoteByID(ctx, po.QuoteID)
	if err != nil {
		return views.PurchaseOrderPage{}, err
	}
	if quote == nil {
		return views.PurchaseOrderPage{}, fmt.Errorf("purchase order %d: quote %d missing", po.ID, po.QuoteID)
	}
	page := views.PurchaseOrderPage{
		Project: *project, PO: *po,
		Editable:  po.Status == store.POBorrador && canEditPO(user, project),
		IsAdmin:   user != nil && user.Role == store.RoleAdmin,
		FreeLines: quote.SeriesFreeLines,
		ShowKg:    views.ShowsKg(*quote),
		Values: views.POFormValues{
			DeliveryAddress: po.DeliveryAddress, DeliveryReferences: po.DeliveryReferences,
			Currency: po.Currency, PaymentForm: po.PaymentForm, PaymentOther: po.PaymentOther,
		},
		Conditions: store.POConditions(po.Currency, po.PaymentForm, po.CreditDays, po.PaymentOther),
	}
	if po.SupplierID != nil {
		page.Values.SupplierID = strconv.FormatInt(*po.SupplierID, 10)
	}
	if po.CreditDays != nil {
		page.Values.CreditDays = strconv.FormatInt(*po.CreditDays, 10)
	}
	saved, err := h.store.PurchaseOrderLines(ctx, po.ID)
	if err != nil {
		return page, err
	}
	var current []store.PurchaseOrderLine
	if page.Editable {
		chosen := map[int]bool{}
		for _, l := range saved {
			chosen[l.QuoteLineNo] = true
		}
		if form != nil {
			page.Values, chosen, page.Errors = form.values, form.chosen, form.errors
		}
		quoteLines, err := h.store.ListQuoteLines(ctx, quote.ID)
		if err != nil {
			return page, err
		}
		taken, err := h.store.ActivePOLines(ctx, project.ID, po.ID)
		if err != nil {
			return page, err
		}
		for _, ql := range quoteLines {
			l, ok := store.LineFromQuote(ql, page.ShowKg)
			v := poLineView(l, page.ShowKg)
			v.NoCost, v.TakenBy, v.Checked = !ok, taken[ql.LineNo], ok && chosen[ql.LineNo]
			if !ok {
				v.UnitPrice, v.Total = "—", "—"
			}
			page.Lines = append(page.Lines, v)
			if v.Checked {
				current = append(current, l)
			}
		}
		if page.Suppliers, err = h.store.POSupplierOptions(ctx); err != nil {
			return page, err
		}
		if page.Billing, err = h.billing(ctx); err != nil {
			return page, err
		}
	} else {
		current = saved
		for _, l := range saved {
			v := poLineView(l, page.ShowKg)
			v.Checked = true
			page.Lines = append(page.Lines, v)
		}
	}
	t := store.POTotals(current)
	page.Subtotal, page.IVA, page.Total = t.Subtotal.String(), t.IVA.String(), t.Total.String()
	return page, nil
}

func poLineView(l store.PurchaseOrderLine, kg bool) views.POLineView {
	v := views.POLineView{
		QuoteLineNo: l.QuoteLineNo, Description: l.Description, Qty: l.QtyMilli.String(),
		UnitPrice: l.UnitPriceMicros.ToCentavosHalfUp().String(), Total: l.LineTotal.String(),
	}
	if kg && l.KgPerUnit != nil {
		v.KgPerUnit, v.Kg = views.KgPerMetreText(*l.KgPerUnit), views.KgTotalText(l.QtyMilli, *l.KgPerUnit)
	}
	if kg && l.PricePerKg != nil {
		v.PricePerKg = l.PricePerKg.ToCentavosHalfUp().String()
	}
	return v
}

// billing is the Datos de facturación block from the emisor and the O.C. settings.
func (h *PurchaseOrders) billing(ctx context.Context) (views.POBillingView, error) {
	e, err := h.store.EmisorSettings(ctx)
	if err != nil {
		return views.POBillingView{}, err
	}
	b, err := h.store.POBillingSettings(ctx)
	if err != nil {
		return views.POBillingView{}, err
	}
	v := views.POBillingView{Name: b.Name, RFC: e.RFC, Address: b.Address}
	if e.TaxRegime != "" {
		v.Regime = store.SATLabel(store.TaxRegimes, e.TaxRegime)
	}
	if b.CFDIUse != "" {
		v.CFDIUse = store.SATLabel(store.CFDIUses, b.CFDIUse)
	}
	v.Missing = len(b.Errors()) > 0 || !store.ValidRFC(e.RFC) || e.TaxRegime == ""
	return v, nil
}

// parseForm reads a borrador's form. Its partidas are recomputed from the quote, so
// only which ones are chosen comes from the post. Errors are the format ones that keep
// even a borrador from saving: días de crédito that aren't a whole number, an unknown
// supplier, moneda or forma de pago.
func (h *PurchaseOrders) parseForm(r *http.Request, po *store.PurchaseOrder) (*poForm, error) {
	ctx := r.Context()
	f := &poForm{chosen: map[int]bool{}, errors: map[string]string{}}
	f.values = views.POFormValues{
		SupplierID:         strings.TrimSpace(r.FormValue("proveedor")),
		DeliveryAddress:    strings.TrimSpace(r.FormValue("domicilio")),
		DeliveryReferences: strings.TrimSpace(r.FormValue("referencias")),
		Currency:           r.FormValue("moneda"),
		PaymentForm:        r.FormValue("forma_pago"),
		CreditDays:         strings.TrimSpace(r.FormValue("dias_credito")),
		PaymentOther:       strings.TrimSpace(r.FormValue("forma_pago_otra")),
	}
	v := f.values
	d := store.PODraft{DeliveryAddress: v.DeliveryAddress, DeliveryReferences: v.DeliveryReferences,
		Currency: v.Currency, PaymentForm: v.PaymentForm, PaymentOther: v.PaymentOther}
	if utf8.RuneCountInString(v.DeliveryAddress) > 500 || utf8.RuneCountInString(v.DeliveryReferences) > 500 ||
		utf8.RuneCountInString(v.PaymentOther) > 300 {
		f.errors["domicilio"] = "El domicilio y las referencias son de hasta 500 caracteres, y la forma de pago de hasta 300."
	}
	if v.SupplierID != "" {
		id, err := strconv.ParseInt(v.SupplierID, 10, 64)
		var sup *store.Supplier
		if err == nil {
			if sup, err = h.store.SupplierByID(ctx, id); err != nil {
				return nil, err
			}
		}
		if sup == nil {
			f.errors["proveedor"] = "Elige un proveedor de la lista."
		} else {
			d.SupplierID = &sup.ID
			// Crédito starts from the supplier's días de crédito.
			if v.PaymentForm == store.POPayCredit && v.CreditDays == "" && sup.CreditDays != nil {
				f.values.CreditDays = strconv.FormatInt(*sup.CreditDays, 10)
			}
		}
	}
	if !validCode(store.POCurrencies, v.Currency) {
		f.errors["moneda"] = "Elige MXN o USD."
	}
	known := false
	for _, pf := range store.POPaymentForms {
		known = known || pf.Code == v.PaymentForm
	}
	if !known {
		f.errors["forma_pago"] = "Elige la forma de pago."
	}
	if f.values.CreditDays != "" {
		n, err := strconv.ParseInt(f.values.CreditDays, 10, 64)
		if err != nil || n < 0 || n > 9999 {
			f.errors["dias_credito"] = "Los días de crédito son un número entero, p. ej. 30."
		} else {
			d.CreditDays = &n
		}
	}
	quote, err := h.store.QuoteByID(ctx, po.QuoteID)
	if err != nil {
		return nil, err
	}
	if quote == nil {
		return nil, fmt.Errorf("purchase order %d: quote %d missing", po.ID, po.QuoteID)
	}
	f.quote = quote
	for _, raw := range r.Form["partida"] {
		if n, err := strconv.Atoi(raw); err == nil {
			f.chosen[n] = true
		}
	}
	if f.byQuote, err = h.store.ListQuoteLines(ctx, quote.ID); err != nil {
		return nil, err
	}
	for _, ql := range f.byQuote {
		if !f.chosen[ql.LineNo] {
			continue
		}
		if l, ok := store.LineFromQuote(ql, views.ShowsKg(*quote)); ok {
			d.Lines = append(d.Lines, l)
		}
	}
	f.draft = d
	return f, nil
}

func validCode(codes []string, c string) bool {
	for _, x := range codes {
		if x == c {
			return true
		}
	}
	return false
}

// confirmProblems is what keeps a borrador from being confirmed, by field.
func confirmProblems(d store.PODraft) map[string]string {
	errs := map[string]string{}
	if d.SupplierID == nil {
		errs["proveedor"] = "Elige el proveedor."
	}
	if strings.TrimSpace(d.DeliveryAddress) == "" {
		errs["domicilio"] = "Escribe el domicilio de entrega."
	}
	if len(d.Lines) == 0 {
		errs["partidas"] = "Elige al menos una partida."
	}
	switch d.PaymentForm {
	case store.POPayCredit:
		if d.CreditDays == nil {
			errs["dias_credito"] = "Escribe los días de crédito."
		}
	case store.POPayOther:
		if strings.TrimSpace(d.PaymentOther) == "" {
			errs["forma_pago_otra"] = "Escribe la forma de pago."
		}
	}
	return errs
}

// editable loads the O.C. for a write to its borrador, answering 403 or 409 and
// returning nils when the viewer can't edit it or it isn't a borrador any more.
func (h *PurchaseOrders) editable(w http.ResponseWriter, r *http.Request) (*store.Project, *store.PurchaseOrder) {
	project, po := h.loadPO(w, r)
	if po == nil {
		return nil, nil
	}
	user, _ := UserFromContext(r.Context())
	if !canEditPO(user, project) {
		http.Error(w, "solo el vendedor del proyecto o un administrador puede editar la orden de compra", http.StatusForbidden)
		return nil, nil
	}
	if po.Status != store.POBorrador {
		http.Error(w, "esta orden de compra ya fue confirmada y no se puede editar; crea una revisión", http.StatusConflict)
		return nil, nil
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "solicitud inválida", http.StatusBadRequest)
		return nil, nil
	}
	return project, po
}

// save parses and saves the borrador, or re-renders it with its errors (422) and
// returns nil.
func (h *PurchaseOrders) save(w http.ResponseWriter, r *http.Request, project *store.Project, po *store.PurchaseOrder) *poForm {
	ctx := r.Context()
	form, err := h.parseForm(r, po)
	if err != nil {
		serverError(w, r, err)
		return nil
	}
	if len(form.errors) > 0 {
		h.renderRefused(w, r, project, po, form, "")
		return nil
	}
	if err := h.store.SavePurchaseOrderDraft(ctx, po.ID, form.draft); err != nil {
		if errors.Is(err, store.ErrPONotDraft) {
			http.Error(w, "esta orden de compra ya fue confirmada y no se puede editar; crea una revisión", http.StatusConflict)
			return nil
		}
		serverError(w, r, err)
		return nil
	}
	return form
}

func (h *PurchaseOrders) renderRefused(w http.ResponseWriter, r *http.Request, project *store.Project, po *store.PurchaseOrder, form *poForm, msg string) {
	page, err := h.page(r.Context(), project, po, form)
	if err != nil {
		serverError(w, r, err)
		return
	}
	page.ErrorMsg = msg
	h.render(w, r, http.StatusUnprocessableEntity, page)
}

// Guardar handles POST /proyectos/{folio}/ordenes-de-compra/{id}: saves the borrador.
func (h *PurchaseOrders) Guardar(w http.ResponseWriter, r *http.Request) {
	project, po := h.editable(w, r)
	if po == nil {
		return
	}
	if h.save(w, r, project, po) == nil {
		return
	}
	http.Redirect(w, r, views.POPath(project.Folio, po.ID, "")+"?guardado=1", http.StatusSeeOther)
}

// VerPDF handles POST .../{id}/ver-pdf, the borrador's "Ver PDF": saves it and shows its
// preview, so the PDF is what the form says.
func (h *PurchaseOrders) VerPDF(w http.ResponseWriter, r *http.Request) {
	project, po := h.editable(w, r)
	if po == nil {
		return
	}
	if h.save(w, r, project, po) == nil {
		return
	}
	http.Redirect(w, r, views.POPDFPath(project.Folio, po.ID, false), http.StatusSeeOther)
}

// Confirmar handles POST .../{id}/confirmar (admin): saves the borrador and, when it has
// a supplier, a delivery address, partidas, its forma de pago complete and Cladex's
// billing data is set, confirms it: numbered with today's date in México City, its PDF
// frozen, and the page then downloads it.
func (h *PurchaseOrders) Confirmar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project, po := h.editable(w, r)
	if po == nil {
		return
	}
	form, err := h.parseForm(r, po)
	if err != nil {
		serverError(w, r, err)
		return
	}
	if len(form.errors) == 0 {
		form.errors = confirmProblems(form.draft)
	}
	billing, err := h.billing(ctx)
	if err != nil {
		serverError(w, r, err)
		return
	}
	msg := ""
	if billing.Missing {
		msg = "Faltan datos de facturación de Cladex: un administrador los completa en Configuración del sistema, en «Datos de facturación en órdenes de compra»."
	}
	if len(form.errors) > 0 || msg != "" {
		if len(form.errors) > 0 && msg == "" {
			msg = "La orden de compra no se confirmó: corrige lo marcado."
		}
		h.renderRefused(w, r, project, po, form, msg)
		return
	}
	now := time.Now().UTC().Truncate(time.Second)
	number, seq, err := h.store.PONumber(ctx, *po, now)
	if err != nil {
		serverError(w, r, err)
		return
	}
	sup, err := h.store.SupplierByID(ctx, *form.draft.SupplierID)
	if err != nil || sup == nil {
		serverError(w, r, fmt.Errorf("supplier %d: %v", *form.draft.SupplierID, err))
		return
	}
	doc := poDocument(*project, *sup, billing, form.draft, views.ShowsKg(*form.quote))
	doc.Number = number
	doc.Fecha = store.InMexicoCity(now).Format("02/01/2006")
	doc.Created = now
	bytes, err := pdf.RenderPurchaseOrder(ctx, doc)
	if err != nil {
		serverError(w, r, err)
		return
	}
	frozen, err := json.Marshal(doc)
	if err != nil {
		serverError(w, r, err)
		return
	}
	user, _ := UserFromContext(ctx)
	totals := store.POTotals(form.draft.Lines)
	conf := store.POConfirmation{
		Number: number, Seq: seq, ConfirmedAt: now.Format(time.RFC3339), Document: string(frozen),
		PDFSHA256: sha256Hex(bytes), UserID: user.ID,
		HistoryNote: "Orden de compra al proveedor " + number + " confirmada: " + sup.Name + ", total " + totals.Total.String() + ".",
	}
	if err := h.store.ConfirmPurchaseOrder(ctx, po.ID, form.draft, conf); err != nil {
		switch {
		case errors.Is(err, store.ErrPONotDraft):
			http.Error(w, "esta orden de compra ya fue confirmada", http.StatusConflict)
		case errors.Is(err, store.ErrPONotAllowed) || strings.Contains(err.Error(), "UNIQUE"):
			http.Error(w, "otra orden de compra de este proyecto se confirmó al mismo tiempo; vuelve a intentarlo", http.StatusConflict)
		default:
			serverError(w, r, err)
		}
		return
	}
	http.Redirect(w, r, views.POPath(project.Folio, po.ID, "")+"?confirmada=1", http.StatusSeeOther)
}

// poDocument is the PDF of an O.C. with d's data, to the given supplier, without its
// number and date (the caller's: the confirmation's, or a preview's).
func poDocument(project store.Project, sup store.Supplier, billing views.POBillingView, d store.PODraft, showKg bool) pdf.PurchaseOrderDocument {
	doc := pdf.PurchaseOrderDocument{
		Vendedor:        project.UserName,
		Issuer:          strings.TrimSpace(billing.Name + " · RFC " + billing.RFC),
		SupplierName:    sup.LegalName,
		SupplierAddress: store.SupplierAddress(sup),
		BillingName:     billing.Name, BillingRFC: billing.RFC, BillingRegime: billing.Regime,
		BillingAddress: billing.Address, BillingCFDIUse: billing.CFDIUse,
		DeliveryAddress: d.DeliveryAddress, DeliveryRefs: d.DeliveryReferences,
		ShowKg:     showKg,
		Conditions: store.POConditions(d.Currency, d.PaymentForm, d.CreditDays, d.PaymentOther),
	}
	for _, l := range d.Lines {
		v := poLineView(l, showKg)
		doc.Lines = append(doc.Lines, pdf.PurchaseOrderLineDoc{
			Description: l.Description, Qty: v.Qty, KgPerUnit: v.KgPerUnit, Kg: v.Kg,
			PricePerKg: v.PricePerKg, UnitPrice: v.UnitPrice, Total: v.Total,
		})
	}
	t := store.POTotals(d.Lines)
	doc.Subtotal, doc.IVA, doc.Total = t.Subtotal.String(), t.IVA.String(), t.Total.String()
	return doc
}

// PDF handles GET .../{id}/pdf: a confirmed or replaced O.C. reprinted from what was
// frozen when it was confirmed; a borrador's preview, marked BORRADOR, from its saved
// state and the supplier and settings as they are now. ?descargar=1 downloads it.
func (h *PurchaseOrders) PDF(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project, po := h.loadPO(w, r)
	if po == nil {
		return
	}
	var doc pdf.PurchaseOrderDocument
	name := po.Number
	if po.Status == store.POBorrador {
		d, err := h.previewDocument(ctx, project, po)
		if err != nil {
			serverError(w, r, err)
			return
		}
		doc = d
		doc.Watermark = draftWatermark
		name = "Borrador OC-" + project.Folio
	} else {
		if err := json.Unmarshal([]byte(po.Document), &doc); err != nil {
			serverError(w, r, fmt.Errorf("purchase order %d: frozen document: %w", po.ID, err))
			return
		}
		doc.Watermark = h.watermark
	}
	bytes, err := pdf.RenderPurchaseOrder(ctx, doc)
	if err != nil {
		serverError(w, r, err)
		return
	}
	disposition := "inline"
	if r.URL.Query().Get("descargar") == "1" {
		disposition = "attachment"
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": name + ".pdf"}))
	w.Write(bytes)
}

// previewDocument is a borrador's PDF content: its saved data, with no number yet and
// today's date.
func (h *PurchaseOrders) previewDocument(ctx context.Context, project *store.Project, po *store.PurchaseOrder) (pdf.PurchaseOrderDocument, error) {
	quote, err := h.store.QuoteByID(ctx, po.QuoteID)
	if err != nil || quote == nil {
		return pdf.PurchaseOrderDocument{}, fmt.Errorf("purchase order %d: quote: %v", po.ID, err)
	}
	lines, err := h.store.PurchaseOrderLines(ctx, po.ID)
	if err != nil {
		return pdf.PurchaseOrderDocument{}, err
	}
	billing, err := h.billing(ctx)
	if err != nil {
		return pdf.PurchaseOrderDocument{}, err
	}
	var sup store.Supplier
	if po.SupplierID != nil {
		s, err := h.store.SupplierByID(ctx, *po.SupplierID)
		if err != nil {
			return pdf.PurchaseOrderDocument{}, err
		}
		if s != nil {
			sup = *s
		}
	}
	d := store.PODraft{SupplierID: po.SupplierID, DeliveryAddress: po.DeliveryAddress,
		DeliveryReferences: po.DeliveryReferences, Currency: po.Currency, PaymentForm: po.PaymentForm,
		CreditDays: po.CreditDays, PaymentOther: po.PaymentOther, Lines: lines}
	doc := poDocument(*project, sup, billing, d, views.ShowsKg(*quote))
	doc.Number = "Se asigna al confirmar"
	doc.Fecha = store.InMexicoCity(time.Now()).Format("02/01/2006")
	return doc, nil
}

// Eliminar handles POST .../{id}/eliminar: discards a borrador.
func (h *PurchaseOrders) Eliminar(w http.ResponseWriter, r *http.Request) {
	project, po := h.editable(w, r)
	if po == nil {
		return
	}
	if err := h.store.DeletePurchaseOrderDraft(r.Context(), po.ID); err != nil {
		if errors.Is(err, store.ErrPONotDraft) {
			http.Error(w, "esta orden de compra ya fue confirmada y no se puede eliminar", http.StatusConflict)
			return
		}
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/proyectos/"+project.Folio+"#oc-proveedor", http.StatusSeeOther)
}

// Revision handles POST .../{id}/revision (admin): opens a borrador correcting a
// confirmed O.C., with the required "motivo" in the proyecto's history.
func (h *PurchaseOrders) Revision(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project, po := h.loadPO(w, r)
	if po == nil {
		return
	}
	reason := strings.TrimSpace(r.FormValue("motivo"))
	if reason == "" || utf8.RuneCountInString(reason) > maxCommentLen {
		http.Error(w, "escribe el motivo de la revisión, de no más de 2000 caracteres", http.StatusBadRequest)
		return
	}
	user, _ := UserFromContext(ctx)
	id, err := h.store.CreatePurchaseOrderRevision(ctx, po.ID, user.ID, reason)
	if errors.Is(err, store.ErrPONotAllowed) {
		http.Error(w, "solo una orden de compra confirmada, sin otra revisión en borrador, se puede revisar", http.StatusConflict)
		return
	}
	if err != nil {
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, views.POPath(project.Folio, id, "")+"?revision=1", http.StatusSeeOther)
}

// ProveedorNuevo handles POST .../{id}/proveedor-nuevo (admin, QL): saves the borrador
// as typed, adds the supplier from the "nuevo_" fields and picks it, without leaving
// the page. A possible duplicate is asked about first, as on Proveedores.
func (h *PurchaseOrders) ProveedorNuevo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project, po := h.editable(w, r)
	if po == nil {
		return
	}
	form, err := h.parseForm(r, po)
	if err != nil {
		serverError(w, r, err)
		return
	}
	sf := parseProveedorFields(r, "nuevo_")
	refuse := func(errs map[string]string, similar []store.SupplierRow) {
		page, err := h.page(ctx, project, po, form)
		if err != nil {
			serverError(w, r, err)
			return
		}
		page.NewSupplier, page.Similar, page.NewSupplierOpen = sf.SupplierFormValues, similar, true
		page.NewSupplierErrors = map[string]string{}
		for k, v := range errs {
			page.NewSupplierErrors["nuevo_"+k] = v
		}
		h.render(w, r, http.StatusUnprocessableEntity, page)
	}
	if len(form.errors) > 0 {
		refuse(nil, nil)
		return
	}
	if errs := sf.Validate(); len(errs) > 0 {
		refuse(errs, nil)
		return
	}
	if !sf.Confirmed {
		similar, err := h.store.SimilarSuppliers(ctx, sf.Name, sf.LegalName, sf.RFC)
		if err != nil {
			serverError(w, r, err)
			return
		}
		if len(similar) > 0 {
			refuse(nil, similar)
			return
		}
	}
	id, err := h.store.CreateSupplier(ctx, sf.toSupplier(nil))
	if err != nil {
		serverError(w, r, err)
		return
	}
	form.draft.SupplierID = &id
	if form.draft.CreditDays == nil && sf.CreditDays != "" {
		if n, err := strconv.ParseInt(sf.CreditDays, 10, 64); err == nil {
			form.draft.CreditDays = &n
		}
	}
	if err := h.store.SavePurchaseOrderDraft(ctx, po.ID, form.draft); err != nil {
		if errors.Is(err, store.ErrPONotDraft) {
			http.Error(w, "esta orden de compra ya fue confirmada y no se puede editar; crea una revisión", http.StatusConflict)
			return
		}
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, views.POPath(project.Folio, po.ID, "")+"?proveedor=1", http.StatusSeeOther)
}
