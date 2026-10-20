package web

import (
	"errors"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"

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

// List handles GET /facturas.
func (f *Facturas) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := f.store.ListComprobantes(ctx, 0)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	user, _ := UserFromContext(ctx)
	views.FacturasList(list, navUserView(user)).Render(ctx, w)
}

func (f *Facturas) load(w http.ResponseWriter, r *http.Request) *store.Comprobante {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return nil
	}
	c, err := f.store.ComprobanteByID(r.Context(), id)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
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
	}[r.URL.Query().Get("ok")]
	f.render(w, r, c, success, "", http.StatusOK)
}

func (f *Facturas) render(w http.ResponseWriter, r *http.Request, c *store.Comprobante, success, errMsg string, status int) {
	ctx := r.Context()
	v := views.FacturaView{C: *c, Enabled: f.svc.Enabled(), Sandbox: f.svc.Sandbox(), Today: today(r), Success: success, Error: errMsg}
	var err error
	if c.RelatedID != 0 {
		if v.Related, err = f.store.ComprobanteByID(ctx, c.RelatedID); err != nil {
			http.Error(w, "error interno", http.StatusInternalServerError)
			return
		}
	}
	siblings, err := f.store.ListComprobantes(ctx, c.ProjectID)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
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
			http.Error(w, "error interno", http.StatusInternalServerError)
			return
		}
	}
	if c.Status != store.CompBorrador && c.RequestSHA256 != "" && f.svc.Enabled() && c.Sandbox != f.svc.Sandbox() {
		v.WrongEnv = true
	}
	if v.Events, err = f.store.ListComprobanteEvents(ctx, c.ID); err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
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
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	if project == nil {
		http.NotFound(w, r)
		return
	}
	user, _ := UserFromContext(ctx)
	id, err := f.store.CreateFacturaDraft(ctx, project.ID, user.ID)
	if errors.Is(err, store.ErrComprobanteState) {
		http.Error(w, "este proyecto ya tiene factura, o no está en O.C. recibida con su orden de compra", http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/facturas/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// draftEdit reads the draft form.
func draftEdit(r *http.Request, c *store.Comprobante) (store.DraftEdit, string) {
	e := store.DraftEdit{
		UsoCFDI:    r.FormValue("uso_cfdi"),
		FormaPago:  r.FormValue("forma_pago"),
		Recipients: facturacion.NormalizeRecipients(r.FormValue("recipients")),
		PagoFecha:  strings.TrimSpace(r.FormValue("pago_fecha")),
	}
	if c.Tipo == store.TipoFactura && c.MetodoPago == "PPD" {
		e.FormaPago = "99"
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
		if err != nil || c == nil {
			http.Error(w, "error interno", http.StatusInternalServerError)
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
	if lerr != nil || c == nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	f.render(w, r, c, "", msg, http.StatusConflict)
}

func facturaPathURL(id int64) string { return "/facturas/" + strconv.FormatInt(id, 10) }

func facturaURL(id int64, ok string) string { return facturaPathURL(id) + "?ok=" + ok }
