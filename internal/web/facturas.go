package web

import (
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gerrygoo/cladex-web/internal/facturacion"
	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/pdf"
	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

// Facturas holds the /facturas handlers: a comprobante's page, its draft, validation,
// stamping, cancellation and downloads (#20, M6–M8). Anyone signed in can look at them and
// download them; every action is admin-only (decision 6 of the milestones doc), wired with
// RequireAdmin in the router.
type Facturas struct {
	store  *store.Store
	svc    *facturacion.Service
	logger *slog.Logger
}

func NewFacturas(s *store.Store, svc *facturacion.Service, logger *slog.Logger) *Facturas {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Facturas{store: s, svc: svc, logger: logger}
}

// List handles GET /facturas: the proyectos with the client's O.C. received and their
// Estado de facturación (#63), sortable by every column and filterable by estado,
// cliente and vendedor. Each factura opens from its folio fiscal or its proyecto.
func (f *Facturas) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sortCol, dir := sortParams(r)
	filters := filterParams(r)
	rows, err := f.store.ListBilling(ctx, sortCol, dir, filters)
	if err != nil {
		serverError(w, r, err)
		return
	}
	customers, owners, err := f.store.ProjectFilterChoices(ctx)
	if err != nil {
		serverError(w, r, err)
		return
	}
	user, _ := UserFromContext(ctx)
	lv := views.ListView{Base: "/facturas", Sort: sortCol, Dir: dir, Filters: filters}
	views.FacturasPage(views.Billing{Rows: rows, Today: today(r)}, lv, customers, owners, navUserView(user)).Render(ctx, w)
}

func (f *Facturas) load(w http.ResponseWriter, r *http.Request) *store.Comprobante {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return nil
	}
	c, err := f.store.ComprobanteByID(r.Context(), id)
	if err != nil {
		serverError(w, r, err)
		return nil
	}
	if c == nil {
		http.NotFound(w, r)
		return nil
	}
	return c
}

// Page handles GET /facturas/{id}.
func (f *Facturas) Page(w http.ResponseWriter, r *http.Request) {
	c := f.load(w, r)
	if c == nil {
		return
	}
	success := map[string]string{
		"guardado": "Borrador guardado.", "validada": "Validada: lista para timbrar.", "timbrada": "Timbrada.",
		"cancelada": "Cancelada.", "pendiente": "La cancelación espera la respuesta del receptor.",
		"registrada": "Factura registrada.", "complemento": "Complemento de pago registrado.",
		"datos": "Datos guardados. Revisa la factura.",
	}[r.URL.Query().Get("ok")]
	f.render(w, r, c, success, "", http.StatusOK)
}

func (f *Facturas) render(w http.ResponseWriter, r *http.Request, c *store.Comprobante, success, errMsg string, status int) {
	ctx := r.Context()
	v := views.FacturaView{C: *c, Enabled: f.svc.Enabled(), Sandbox: f.svc.Sandbox(), Today: today(r), Success: success, Error: errMsg}
	var err error
	if c.RelatedID != 0 {
		if v.Related, err = f.store.ComprobanteByID(ctx, c.RelatedID); err != nil {
			serverError(w, r, err)
			return
		}
	}
	siblings, err := f.store.ListComprobantes(ctx, c.ProjectID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	if c.Tipo == store.TipoFactura {
		v.Saldo = c.Total
		for i := len(siblings) - 1; i >= 0; i-- { // oldest first
			s := siblings[i]
			if s.RelatedID != c.ID || s.Tipo != store.TipoPago {
				continue
			}
			v.Pagos = append(v.Pagos, s)
			if s.Status == store.CompTimbrada || s.Status == store.CompCancelPend {
				v.Saldo -= s.Total
			}
		}
		if c.MetodoPago == "PPD" && c.Status == store.CompTimbrada {
			payments, err := f.store.ListPayments(ctx, c.ProjectID)
			if err != nil {
				serverError(w, r, err)
				return
			}
			for _, p := range payments {
				if p.ComprobanteID == 0 {
					v.Unlinked = append(v.Unlinked, p)
				}
			}
		}
	}
	for i := range siblings {
		s := siblings[i]
		if s.RelatedID != c.ID || s.Status == store.CompCancelada {
			continue
		}
		if s.Tipo == store.TipoPago && v.Pago == nil {
			v.Pago = &s
		} else if s.Tipo == store.TipoFactura && v.Replacement == nil {
			v.Replacement = &s
		}
	}
	if c.Status == store.CompBorrador {
		if v.Problems, err = f.svc.Problems(ctx, c, v.Related); err != nil {
			serverError(w, r, err)
			return
		}
	}
	if c.Status != store.CompBorrador && c.RequestSHA256 != "" && f.svc.Enabled() && c.Sandbox != f.svc.Sandbox() {
		v.WrongEnv = true
	}
	if v.Events, err = f.store.ListComprobanteEvents(ctx, c.ID); err != nil {
		serverError(w, r, err)
		return
	}
	user, _ := UserFromContext(ctx)
	w.WriteHeader(status)
	views.FacturaPage(v, navUserView(user)).Render(ctx, w)
}

// Crear handles POST /proyectos/{folio}/factura/nueva: starts the proyecto's factura.
func (f *Facturas) Crear(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project, err := f.store.ProjectByFolio(ctx, r.PathValue("folio"))
	if err != nil {
		serverError(w, r, err)
		return
	}
	if project == nil {
		http.NotFound(w, r)
		return
	}
	user, _ := UserFromContext(ctx)
	id, err := f.store.CreateFacturaDraft(ctx, project.ID, user.ID)
	if errors.Is(err, store.ErrComprobanteState) {
		http.Error(w, "este proyecto ya tiene factura, o no está En curso con su orden de compra", http.StatusConflict)
		return
	}
	if err != nil {
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/facturas/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// draftEdit reads the draft form. A factura's "metodo_pago" (PUE or PPD) is chosen on
// the draft (issue #61), except on one that replaces a stamped factura, which keeps the
// original's; left out, it stays as it was. P.P.D. means forma de pago 99, and a forma 99
// left over from P.P.D. is cleared when the método changes to P.U.E.
func draftEdit(r *http.Request, c *store.Comprobante) (store.DraftEdit, string) {
	e := store.DraftEdit{
		UsoCFDI:    r.FormValue("uso_cfdi"),
		MetodoPago: c.MetodoPago,
		FormaPago:  r.FormValue("forma_pago"),
		Recipients: facturacion.NormalizeRecipients(r.FormValue("recipients")),
		PagoFecha:  strings.TrimSpace(r.FormValue("pago_fecha")),
	}
	if m := r.FormValue("metodo_pago"); c.Tipo == store.TipoFactura && c.RelatedID == 0 && m != "" {
		if store.PaymentMethodLabels[m] == "" {
			return e, "Elige el método de pago: P.U.E. o P.P.D."
		}
		e.MetodoPago = m
	}
	switch {
	case c.Tipo == store.TipoFactura && e.MetodoPago == "PPD":
		e.FormaPago = "99"
	case c.Tipo == store.TipoFactura && e.FormaPago == "99":
		e.FormaPago = ""
	}
	if c.Moneda == "USD" {
		if raw := strings.TrimSpace(r.FormValue("tipo_cambio")); raw != "" {
			m, err := money.ParseMicros(raw)
			if err != nil || m <= 0 {
				return e, "El tipo de cambio no es un número válido."
			}
			e.TipoCambioMicros = &m
		}
	}
	for _, raw := range r.Form["concepto_id"] {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			continue
		}
		e.Conceptos = append(e.Conceptos, store.ConceptoEdit{
			ID:             id,
			SATProductCode: r.FormValue("sat_product_" + raw),
			SATUnitCode:    r.FormValue("sat_unit_" + raw),
			UnitName:       r.FormValue("unit_name_" + raw),
			Description:    r.FormValue("description_" + raw),
		})
	}
	for _, l := range e.Conceptos {
		if strings.TrimSpace(l.Description) == "" {
			return e, "Ninguna partida puede quedar sin descripción."
		}
	}
	return e, ""
}

func (f *Facturas) save(w http.ResponseWriter, r *http.Request, c *store.Comprobante) bool {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "solicitud inválida", http.StatusBadRequest)
		return false
	}
	e, msg := draftEdit(r, c)
	if msg != "" {
		f.render(w, r, c, "", msg, http.StatusUnprocessableEntity)
		return false
	}
	user, _ := UserFromContext(r.Context())
	if err := f.store.UpdateDraft(r.Context(), c.ID, user.ID, e); err != nil {
		f.stateError(w, r, c.ID, err)
		return false
	}
	return true
}

// Guardar handles POST /facturas/{id}: saves the draft.
func (f *Facturas) Guardar(w http.ResponseWriter, r *http.Request) {
	c := f.load(w, r)
	if c == nil || !f.save(w, r, c) {
		return
	}
	http.Redirect(w, r, facturaURL(c.ID, "guardado"), http.StatusSeeOther)
}

// Validar handles POST /facturas/{id}/validar: saves the draft and freezes it as lista.
func (f *Facturas) Validar(w http.ResponseWriter, r *http.Request) {
	c := f.load(w, r)
	if c == nil || !f.save(w, r, c) {
		return
	}
	user, _ := UserFromContext(r.Context())
	err := f.svc.Validate(r.Context(), c.ID, user.ID)
	var pe *facturacion.ProblemsError
	if errors.As(err, &pe) {
		c, _ = f.store.ComprobanteByID(r.Context(), c.ID)
		f.render(w, r, c, "Borrador guardado.", "Todavía no se puede validar; revisa lo que falta.", http.StatusUnprocessableEntity)
		return
	}
	if err != nil {
		f.stateError(w, r, c.ID, err)
		return
	}
	http.Redirect(w, r, facturaURL(c.ID, "validada"), http.StatusSeeOther)
}

// Editar handles POST /facturas/{id}/editar: a lista or rechazada one back to borrador.
func (f *Facturas) Editar(w http.ResponseWriter, r *http.Request) {
	c := f.load(w, r)
	if c == nil {
		return
	}
	user, _ := UserFromContext(r.Context())
	if err := f.store.ToBorrador(r.Context(), c.ID, user.ID); err != nil {
		f.stateError(w, r, c.ID, err)
		return
	}
	http.Redirect(w, r, facturaPathURL(c.ID), http.StatusSeeOther)
}

// Timbrar handles POST /facturas/{id}/timbrar: stamps a lista one, or resends an incierto
// one's same request.
func (f *Facturas) Timbrar(w http.ResponseWriter, r *http.Request) {
	c := f.load(w, r)
	if c == nil {
		return
	}
	user, _ := UserFromContext(r.Context())
	o, err := f.svc.Stamp(r.Context(), c.ID, user.ID)
	if err != nil {
		f.stateError(w, r, c.ID, err)
		return
	}
	f.logger.Info("stamp", slog.Int64("comprobante", c.ID), slog.String("result", o.Result), slog.String("uuid", o.UUID))
	if o.Result == "stamped" {
		http.Redirect(w, r, facturaURL(c.ID, "timbrada"), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, facturaPathURL(c.ID), http.StatusSeeOther)
}

// Cancelar handles POST /facturas/{id}/cancelar: "motivo", and "sustitucion" for motivo 01.
func (f *Facturas) Cancelar(w http.ResponseWriter, r *http.Request) {
	c := f.load(w, r)
	if c == nil {
		return
	}
	user, _ := UserFromContext(r.Context())
	o, err := f.svc.Cancel(r.Context(), c.ID, user.ID, r.FormValue("motivo"), strings.TrimSpace(r.FormValue("sustitucion")))
	if err != nil {
		f.stateError(w, r, c.ID, err)
		return
	}
	f.afterCancel(w, r, c.ID, o)
}

// Consultar handles POST /facturas/{id}/consultar: a pending cancellation's state.
func (f *Facturas) Consultar(w http.ResponseWriter, r *http.Request) {
	c := f.load(w, r)
	if c == nil {
		return
	}
	user, _ := UserFromContext(r.Context())
	o, err := f.svc.CheckCancellation(r.Context(), c.ID, user.ID)
	if err != nil {
		f.stateError(w, r, c.ID, err)
		return
	}
	f.afterCancel(w, r, c.ID, o)
}

func (f *Facturas) afterCancel(w http.ResponseWriter, r *http.Request, id int64, o store.CancelOutcome) {
	switch o.Result {
	case "cancelled":
		http.Redirect(w, r, facturaURL(id, "cancelada"), http.StatusSeeOther)
	case "pending":
		http.Redirect(w, r, facturaURL(id, "pendiente"), http.StatusSeeOther)
	default:
		c, err := f.store.ComprobanteByID(r.Context(), id)
		if err == nil && c == nil {
			err = fmt.Errorf("comprobante %d vanished", id)
		}
		if err != nil {
			serverError(w, r, err)
			return
		}
		msg := "No se canceló: " + o.Message
		if o.Result == "unknown" {
			msg = "No sabemos si se canceló (" + o.Message + "). Vuelve a intentarlo: pedirla otra vez no hace daño."
		}
		if o.Result == "vigente" {
			msg = "El receptor rechazó la cancelación; la factura sigue vigente."
		}
		f.render(w, r, c, "", msg, http.StatusOK)
	}
}

// Sustituir handles POST /facturas/{id}/sustituir: starts the factura that replaces it.
func (f *Facturas) Sustituir(w http.ResponseWriter, r *http.Request) {
	c := f.load(w, r)
	if c == nil {
		return
	}
	user, _ := UserFromContext(r.Context())
	id, err := f.store.CreateReplacementDraft(r.Context(), c.ID, user.ID)
	if err != nil {
		f.stateError(w, r, c.ID, err)
		return
	}
	http.Redirect(w, r, facturaPathURL(id), http.StatusSeeOther)
}

// Pago handles POST /facturas/{id}/pago: starts the comprobante de pago of a P.P.D. factura.
func (f *Facturas) Pago(w http.ResponseWriter, r *http.Request) {
	c := f.load(w, r)
	if c == nil {
		return
	}
	user, _ := UserFromContext(r.Context())
	id, err := f.store.CreatePagoDraft(r.Context(), c.ID, user.ID, today(r))
	if err != nil {
		f.stateError(w, r, c.ID, err)
		return
	}
	http.Redirect(w, r, facturaPathURL(id), http.StatusSeeOther)
}

// XML handles GET /facturas/{id}/xml: the stamped CFDI, the legal document.
func (f *Facturas) XML(w http.ResponseWriter, r *http.Request) {
	c := f.load(w, r)
	if c == nil {
		return
	}
	if c.XML == "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": downloadName(c) + ".xml"}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write([]byte(c.XML))
}

// PDF handles GET /facturas/{id}/pdf: its printed representation, rendered from the XML.
func (f *Facturas) PDF(w http.ResponseWriter, r *http.Request) {
	c := f.load(w, r)
	if c == nil {
		return
	}
	if c.XML == "" {
		http.NotFound(w, r)
		return
	}
	doc, err := facturacion.Printed(c)
	if err != nil {
		f.logger.Error("factura pdf", slog.Int64("comprobante", c.ID), slog.Any("err", err))
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	b, err := pdf.RenderCFDI(r.Context(), doc)
	if err != nil {
		f.logger.Error("factura pdf", slog.Int64("comprobante", c.ID), slog.Any("err", err))
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": downloadName(c) + ".pdf"}))
	w.Write(b)
}

func downloadName(c *store.Comprobante) string {
	name := c.SerieFolio()
	if c.UUID != "" {
		name += "_" + c.UUID
	}
	return name
}

// stateError answers an action that didn't fit: the page again with why.
func (f *Facturas) stateError(w http.ResponseWriter, r *http.Request, id int64, err error) {
	msg := ""
	var pe *facturacion.ProblemsError
	switch {
	case errors.As(err, &pe):
		msg = pe.Error()
	case errors.Is(err, store.ErrComprobanteState):
		msg = "Esa acción ya no aplica: la factura cambió de estado. Revisa cómo está ahora."
	case errors.Is(err, facturacion.ErrWrongEnvironment):
		msg = "Este comprobante se validó para el otro ambiente de Facturoporti y aquí no se puede timbrar ni cancelar."
	default:
		f.logger.Error("factura", slog.Int64("comprobante", id), slog.Any("err", err))
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	c, lerr := f.store.ComprobanteByID(r.Context(), id)
	if lerr == nil && c == nil {
		lerr = fmt.Errorf("comprobante %d vanished", id)
	}
	if lerr != nil {
		serverError(w, r, lerr)
		return
	}
	f.render(w, r, c, "", msg, http.StatusConflict)
}

func facturaPathURL(id int64) string { return "/facturas/" + strconv.FormatInt(id, 10) }

func facturaURL(id int64, ok string) string { return facturaPathURL(id) + "?ok=" + ok }

// cfdiXMLTypes are what an uploaded CFDI's bytes may sniff as; cfdi.Parse is the real check.
var cfdiXMLTypes = map[string]bool{"text/xml; charset=utf-8": true, "text/plain; charset=utf-8": true}

// pdfOnly is the type of a factura's or complemento's PDF.
var pdfOnly = map[string]bool{"application/pdf": true}

// Manual handles POST /proyectos/{folio}/factura/manual (#63): an admin records a factura
// issued outside the app. "xml" is its stamped XML (required, up to 1 MB), "pdf" its PDF
// (optional, up to 10 MB). A P.U.E. factura needs "cobrado" checked and "fecha_pago" (not
// after today): it is only issued once paid, and that payment goes to Cobro.
func (f *Facturas) Manual(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project, err := f.store.ProjectByFolio(ctx, r.PathValue("folio"))
	if err != nil {
		serverError(w, r, err)
		return
	}
	if project == nil {
		http.NotFound(w, r)
		return
	}
	limit := int64(store.MaxCFDISize + maxOCFileBytes + 1<<20)
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := r.ParseMultipartForm(limit); err != nil {
		http.Error(w, "sube el XML de la factura (hasta 1 MB) y, si quieres, su PDF (hasta 10 MB)", http.StatusBadRequest)
		return
	}
	xmlFile, msg, status := uploadedFile(r, "xml", store.MaxCFDISize, cfdiXMLTypes,
		"el XML no puede pesar más de 1 MB", "el archivo de la factura debe ser su XML", "factura.xml")
	if msg != "" {
		http.Error(w, msg, status)
		return
	}
	if xmlFile == nil {
		http.Error(w, "sube el XML de la factura: es obligatorio", http.StatusBadRequest)
		return
	}
	pdfFile, msg, status := uploadedFile(r, "pdf", maxOCFileBytes, pdfOnly,
		"el PDF no puede pesar más de 10 MB", "el PDF de la factura debe ser un archivo PDF", "factura.pdf")
	if msg != "" {
		http.Error(w, msg, status)
		return
	}
	m := store.ManualFactura{XML: xmlFile.Data, PDF: pdfFile}
	if r.FormValue("cobrado") == "si" {
		m.PaidOn = strings.TrimSpace(r.FormValue("fecha_pago"))
		if m.PaidOn > today(r) {
			http.Error(w, "la fecha de pago no puede ser futura", http.StatusBadRequest)
			return
		}
	}
	user, _ := UserFromContext(ctx)
	id, err := f.store.RecordManualFactura(ctx, project.ID, user.ID, m)
	var me *store.ManualError
	switch {
	case errors.As(err, &me):
		http.Error(w, me.Msg, http.StatusBadRequest)
		return
	case errors.Is(err, store.ErrBadTransition):
		http.Error(w, "la factura solo se registra en un proyecto En curso con su orden de compra y sin otra factura vigente", http.StatusConflict)
		return
	case err != nil:
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, facturaURL(id, "registrada"), http.StatusSeeOther)
}

// CancelarManual handles POST /facturas/{id}/cancelar-manual: an admin marks a factura
// recorded by hand as cancelled once it was cancelled outside the app, with "motivo" (the
// SAT's) and "nota", both required.
func (f *Facturas) CancelarManual(w http.ResponseWriter, r *http.Request) {
	c := f.load(w, r)
	if c == nil {
		return
	}
	note := strings.TrimSpace(r.FormValue("nota"))
	if note == "" || utf8.RuneCountInString(note) > maxCommentLen {
		f.render(w, r, c, "", "Escribe por qué se canceló (hasta 2000 caracteres).", http.StatusBadRequest)
		return
	}
	user, _ := UserFromContext(r.Context())
	err := f.store.CancelManualFactura(r.Context(), c.ID, user.ID, r.FormValue("motivo"), note)
	if errors.Is(err, store.ErrComprobanteState) {
		f.render(w, r, c, "", "No se puede marcar como cancelada: elige el motivo, y si tiene un complemento de pago vigente, cancélalo antes.", http.StatusConflict)
		return
	}
	if err != nil {
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, facturaURL(c.ID, "cancelada"), http.StatusSeeOther)
}

// ComplementoManual handles POST /facturas/{id}/complemento-manual (#63): an admin records
// a complemento de pago issued outside the app for this P.P.D. factura. "xml" is its
// stamped XML (required), "pdf" its PDF (optional), and "pago" the id of a payment already
// in Cobro that it covers, or "nuevo" to record the payment from the complemento.
func (f *Facturas) ComplementoManual(w http.ResponseWriter, r *http.Request) {
	c := f.load(w, r)
	if c == nil {
		return
	}
	limit := int64(store.MaxCFDISize + maxOCFileBytes + 1<<20)
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := r.ParseMultipartForm(limit); err != nil {
		http.Error(w, "sube el XML del complemento (hasta 1 MB) y, si quieres, su PDF (hasta 10 MB)", http.StatusBadRequest)
		return
	}
	xmlFile, msg, status := uploadedFile(r, "xml", store.MaxCFDISize, cfdiXMLTypes,
		"el XML no puede pesar más de 1 MB", "el archivo del complemento debe ser su XML", "complemento.xml")
	if msg != "" {
		http.Error(w, msg, status)
		return
	}
	if xmlFile == nil {
		http.Error(w, "sube el XML del complemento de pago: es obligatorio", http.StatusBadRequest)
		return
	}
	pdfFile, msg, status := uploadedFile(r, "pdf", maxOCFileBytes, pdfOnly,
		"el PDF no puede pesar más de 10 MB", "el PDF del complemento debe ser un archivo PDF", "complemento.pdf")
	if msg != "" {
		http.Error(w, msg, status)
		return
	}
	m := store.ManualComplemento{XML: xmlFile.Data, PDF: pdfFile}
	switch choice := r.FormValue("pago"); choice {
	case "nuevo":
	case "":
		http.Error(w, "elige si el pago ya está registrado en Cobro", http.StatusBadRequest)
		return
	default:
		id, err := strconv.ParseInt(choice, 10, 64)
		if err != nil || id <= 0 {
			http.Error(w, "elige si el pago ya está registrado en Cobro", http.StatusBadRequest)
			return
		}
		m.LinkPaymentID = id
	}
	user, _ := UserFromContext(r.Context())
	id, err := f.store.RecordManualComplemento(r.Context(), c.ID, user.ID, m)
	var me *store.ManualError
	switch {
	case errors.As(err, &me):
		http.Error(w, me.Msg, http.StatusBadRequest)
		return
	case errors.Is(err, store.ErrPaymentMismatch):
		http.Error(w, "el pago elegido no se puede ligar: debe ser del mismo monto que el complemento y no tener ya un comprobante", http.StatusBadRequest)
		return
	case errors.Is(err, store.ErrComprobanteState):
		http.Error(w, "el complemento solo se registra en una factura P.P.D. vigente de un proyecto En curso", http.StatusConflict)
		return
	case err != nil:
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, facturaURL(id, "complemento"), http.StatusSeeOther)
}

// facturaParam is the "factura" an edit page was opened from to complete what a factura
// draft needs (#63), as digits, or "" when absent or malformed: saving goes back to it.
func facturaParam(r *http.Request) string {
	f := strings.TrimSpace(r.FormValue("factura"))
	if id, err := strconv.ParseInt(f, 10, 64); err != nil || id <= 0 {
		return ""
	}
	return f
}
