// Package facturacion runs Cladex's CFDIs end to end: what a draft still needs, the exact
// request Facturoporti gets, and the stamp and cancel calls with their before-and-after
// records (docs/planning/facturas-timbrado-y-envio.md). The store keeps the state; this
// package decides and talks to the PAC.
package facturacion

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gerrygoo/cladex-web/internal/facturoporti"
	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/store"
)

// Service is facturación for one instance. Client is nil when facturación is off
// (Status says why); everything that would talk to Facturoporti then refuses.
type Service struct {
	Store  *store.Store
	Client *facturoporti.Client
	Status facturoporti.Status
	Now    func() time.Time
}

// New builds the service from what facturoporti.Load returned.
func New(s *store.Store, cfg *facturoporti.Config, st facturoporti.Status) *Service {
	svc := &Service{Store: s, Status: st, Now: time.Now}
	if cfg != nil && st.Enabled {
		svc.Client = facturoporti.NewClient(cfg)
	}
	return svc
}

// Enabled reports whether this instance can stamp.
func (s *Service) Enabled() bool { return s.Client != nil }

// Sandbox reports whether this instance stamps against Facturoporti's sandbox.
func (s *Service) Sandbox() bool { return s.Client == nil || s.Client.Sandbox }

// ProblemsError lists what keeps a draft from being validated, in words for the user.
type ProblemsError struct{ Problems []string }

func (e *ProblemsError) Error() string { return strings.Join(e.Problems, "; ") }

var (
	prodServPattern = regexp.MustCompile(`^[0-9]{8}$`)
	unitPattern     = regexp.MustCompile(`^[A-Z0-9]{2,3}$`)
)

// Problems returns what a draft still needs before it can be validated, empty when
// nothing. related is the factura a pago pays (nil for a factura).
func (s *Service) Problems(ctx context.Context, c *store.Comprobante, related *store.Comprobante) ([]string, error) {
	var p []string
	if !s.Enabled() {
		p = append(p, "La facturación está apagada en este servidor: "+s.Status.Problem+".")
	}
	emisor, err := s.Store.EmisorSettings(ctx)
	if err != nil {
		return nil, err
	}
	if len(emisor.Errors()) > 0 {
		p = append(p, "Faltan los datos del emisor en Configuración del sistema → Emisor.")
	} else if s.Status.RFC != "" && s.Status.RFC != emisor.RFC {
		p = append(p, fmt.Sprintf("El certificado instalado es del RFC %s, pero el emisor es %s.", s.Status.RFC, emisor.RFC))
	}
	cu := c.Customer
	if !store.ValidRFC(cu.RFC) {
		p = append(p, "El cliente no tiene un RFC válido.")
	}
	if strings.TrimSpace(cu.LegalName) == "" {
		p = append(p, "El cliente no tiene razón social.")
	}
	if !store.ValidSATCode(store.TaxRegimes, cu.TaxRegime) {
		p = append(p, "El cliente no tiene régimen fiscal.")
	}
	if !store.ValidPostalCode(cu.PostalCode) {
		p = append(p, "El cliente no tiene código postal fiscal de 5 dígitos.")
	}
	for _, r := range splitRecipients(c.Recipients) {
		if _, err := mail.ParseAddress(r); err != nil {
			p = append(p, "El correo "+r+" no es válido.")
		}
	}
	if c.Moneda == "USD" && (c.TipoCambioMicros == nil || *c.TipoCambioMicros <= 0) {
		p = append(p, "Escribe el tipo de cambio del dólar.")
	}
	switch c.Tipo {
	case store.TipoFactura:
		if !store.ValidSATCode(store.CFDIUses, c.UsoCFDI) {
			p = append(p, "Elige el uso de CFDI.")
		}
		switch {
		case c.MetodoPago == "PPD" && c.FormaPago != "99":
			p = append(p, "Una factura P.P.D. lleva forma de pago 99 (Por definir).")
		case c.MetodoPago == "PUE" && (!store.ValidSATCode(store.PaymentForms, c.FormaPago) || c.FormaPago == "99"):
			p = append(p, "Elige la forma de pago con que el cliente pagó.")
		}
		if len(c.Conceptos) == 0 {
			p = append(p, "La factura no tiene conceptos.")
		}
		for _, l := range c.Conceptos {
			name := fmt.Sprintf("Partida %d", l.LineNo)
			if !prodServPattern.MatchString(l.SATProductCode) {
				p = append(p, name+": falta la clave de producto SAT (8 dígitos).")
			}
			if !unitPattern.MatchString(l.SATUnitCode) {
				p = append(p, name+": falta la clave de unidad SAT (p. ej. MTR, KGM, H87).")
			}
			if strings.TrimSpace(l.Description) == "" {
				p = append(p, name+": falta la descripción.")
			}
		}
		if c.Total <= 0 {
			p = append(p, "El total debe ser mayor a cero.")
		}
	case store.TipoPago:
		if related == nil || related.Status != store.CompTimbrada || related.UUID == "" {
			p = append(p, "La factura que se paga no está timbrada.")
		}
		if !store.ValidSATCode(store.PaymentForms, c.FormaPago) || c.FormaPago == "99" {
			p = append(p, "Elige la forma de pago.")
		}
		if d, err := time.Parse("2006-01-02", c.PagoFecha); err != nil {
			p = append(p, "Escribe la fecha del pago.")
		} else if d.After(s.Now().In(mexicoCity)) {
			p = append(p, "La fecha del pago no puede ser futura.")
		}
	}
	return p, nil
}

func splitRecipients(s string) []string {
	var out []string
	for _, r := range strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == ',' || r == ' ' || r == '\n' }) {
		if r = strings.TrimSpace(r); r != "" {
			out = append(out, r)
		}
	}
	return out
}

// NormalizeRecipients writes a recipients list the way it is stored and sent: addresses
// separated by ";".
func NormalizeRecipients(s string) string { return strings.Join(splitRecipients(s), ";") }

var mexicoCity = func() *time.Location {
	if l, err := time.LoadLocation("America/Mexico_City"); err == nil {
		return l
	}
	return time.FixedZone("CST", -6*3600)
}()

// ErrWrongEnvironment is returned for a comprobante validated for the other Facturoporti
// host (a sandbox factura in a copy of the database on a production instance, or the
// reverse): it can't be stamped or cancelled here.
var ErrWrongEnvironment = errors.New("facturacion: comprobante belongs to the other environment")

// Validate checks a draft and freezes it as lista with its exact request.
func (s *Service) Validate(ctx context.Context, id, userID int64) error {
	c, related, err := s.load(ctx, id)
	if err != nil {
		return err
	}
	problems, err := s.Problems(ctx, c, related)
	if err != nil {
		return err
	}
	if len(problems) > 0 {
		return &ProblemsError{problems}
	}
	emisor, err := s.Store.EmisorSettings(ctx)
	if err != nil {
		return err
	}
	serie := emisor.SerieFactura
	if c.Tipo == store.TipoPago {
		serie = emisor.SeriePago
	}
	now := s.Now()
	return s.Store.Validate(ctx, id, userID, serie, s.Sandbox(), func(serie string, folio int64) ([]byte, error) {
		return facturoporti.Marshal(BuildRequest(c, related, emisor, serie, folio, now))
	})
}

func (s *Service) load(ctx context.Context, id int64) (c, related *store.Comprobante, err error) {
	if c, err = s.Store.ComprobanteByID(ctx, id); err != nil || c == nil {
		if c == nil && err == nil {
			err = store.ErrComprobanteState
		}
		return nil, nil, err
	}
	if c.RelatedID != 0 {
		if related, err = s.Store.ComprobanteByID(ctx, c.RelatedID); err != nil {
			return nil, nil, err
		}
	}
	return c, related, nil
}

// BuildRequest is the request for a comprobante as of now, with its serie and folio.
// related is the factura a pago pays, or the factura a replacement replaces.
func BuildRequest(c, related *store.Comprobante, emisor store.Emisor, serie string, folio int64, now time.Time) facturoporti.StampRequest {
	cu := c.Customer
	recipients := NormalizeRecipients(c.Recipients)
	r := facturoporti.StampRequest{
		DatosGenerales: facturoporti.DatosGenerales{
			Version: "4.0", OpcionDecimales: 2, NumeroDecimales: 2,
			GeneraPDF: recipients != "", EnviaEmail: recipients != "", ReceptorEmail: recipients,
		},
		Encabezado: facturoporti.Encabezado{
			Emisor: facturoporti.Emisor{RFC: emisor.RFC, NombreRazonSocial: emisor.Name, RegimenFiscal: emisor.TaxRegime},
			Receptor: facturoporti.Receptor{RFC: cu.RFC, NombreRazonSocial: strings.TrimSpace(cu.LegalName), UsoCFDI: c.UsoCFDI,
				RegimenFiscal: cu.TaxRegime, Direccion: facturoporti.Direccion{CodigoPostal: cu.PostalCode}},
			Fecha:           now.In(mexicoCity).Format("2006-01-02T15:04:05"),
			Serie:           serie,
			Folio:           strconv.FormatInt(folio, 10),
			LugarExpedicion: emisor.PostalCode,
		},
	}
	name := fmt.Sprintf("%s-%d", serie, folio)
	if c.Tipo == store.TipoPago {
		r.DatosGenerales.CFDI, r.DatosGenerales.TipoCFDI = "Pago", "Pago"
		r.DatosGenerales.EmailMensaje = "Comprobante de pago " + name + " de " + emisor.Name
		r.Encabezado.Receptor.UsoCFDI = "CP01"
		r.Encabezado.SubTotal, r.Encabezado.Total = facturoporti.Dec(0, 0), facturoporti.Dec(0, 0)
		r.Conceptos = []facturoporti.Concepto{}
		tc, equiv := facturoporti.Dec(1, 0), facturoporti.Dec(1, 0)
		toMXN := func(v money.Centavos) money.Centavos { return v }
		if c.Moneda == "USD" && c.TipoCambioMicros != nil {
			tc = facturoporti.Dec(int64(*c.TipoCambioMicros), 6)
			toMXN = func(v money.Centavos) money.Centavos {
				return money.Centavos(money.RoundHalfUp(int64(v)*int64(*c.TipoCambioMicros), 1_000_000))
			}
		}
		traslado := facturoporti.TrasladoPago{Impuesto: 2, Factor: 1, Base: facturoporti.Dec(int64(related.Subtotal), 2),
			Tasa: facturoporti.IVA16, Importe: facturoporti.Dec(int64(related.IVA), 2)}
		pago := facturoporti.Pago{
			FechaPago: c.PagoFecha + "T12:00:00", FormaPago: c.FormaPago, Moneda: c.Moneda, TipoCambio: tc,
			DocumentosRelacionados: []facturoporti.DocumentoRelacionado{{
				IdDocumento: related.UUID, Serie: related.Serie, Folio: strconv.FormatInt(related.Folio, 10),
				Moneda: related.Moneda, Equivalencia: equiv, NumeroParcialidad: 1,
				ImporteSaldoAnterior: facturoporti.Dec(int64(related.Total), 2), ImportePagado: facturoporti.Dec(int64(related.Total), 2),
				ImporteSaldoInsoluto: facturoporti.Dec(0, 2), ObjetoDeImpuesto: "02",
				Impuestos: facturoporti.ImpuestosPago{Trasladados: []facturoporti.TrasladoPago{traslado}},
			}},
			Impuestos: facturoporti.ImpuestosPago{Trasladados: []facturoporti.TrasladoPago{traslado}},
		}
		r.Complemento = &facturoporti.Complemento{TipoComplemento: facturoporti.TipoComplementoPagos20, PagosV20: &facturoporti.PagosV20{
			Pagos: []facturoporti.Pago{pago},
			Totales: facturoporti.PagosTotales{
				TotalTrasladosBaseIVA16:     facturoporti.Dec(int64(toMXN(related.Subtotal)), 2),
				TotalTrasladosImpuestoIVA16: facturoporti.Dec(int64(toMXN(related.IVA)), 2),
				MontoTotalPagos:             facturoporti.Dec(int64(toMXN(related.Total)), 2),
			},
		}}
		return r
	}
	r.DatosGenerales.CFDI, r.DatosGenerales.TipoCFDI = "Factura", "Ingreso"
	r.DatosGenerales.EmailMensaje = "Factura " + name + " de " + emisor.Name
	r.Encabezado.MetodoPago, r.Encabezado.FormaPago, r.Encabezado.Moneda = c.MetodoPago, c.FormaPago, c.Moneda
	if c.Moneda == "USD" && c.TipoCambioMicros != nil {
		r.Encabezado.TipoCambio = facturoporti.Dec(int64(*c.TipoCambioMicros), 6)
	}
	r.Encabezado.SubTotal = facturoporti.Dec(int64(c.Subtotal), 2)
	r.Encabezado.Total = facturoporti.Dec(int64(c.Total), 2)
	if related != nil && related.UUID != "" {
		r.Encabezado.CFDIsRelacionados, r.Encabezado.TipoRelacion = related.UUID, "04"
	}
	for _, l := range c.Conceptos {
		r.Conceptos = append(r.Conceptos, facturoporti.Concepto{
			Cantidad: facturoporti.Dec(int64(l.QtyMilli), 3), CodigoUnidad: l.SATUnitCode, Unidad: l.UnitName,
			CodigoProducto: l.SATProductCode, Producto: strings.TrimSpace(l.Description),
			PrecioUnitario: facturoporti.Dec(int64(l.UnitPriceMicros), 6), Importe: facturoporti.Dec(int64(l.Importe), 2),
			ObjetoDeImpuesto: "02",
			Impuestos: []facturoporti.Impuesto{{TipoImpuesto: 1, Impuesto: 2, Factor: 1,
				Base: facturoporti.Dec(int64(l.Importe), 2), Tasa: facturoporti.IVA16, ImpuestoImporte: facturoporti.Dec(int64(l.IVA), 2)}},
		})
	}
	return r
}

// Stamp sends a lista (or incierto) comprobante's stored request and records the outcome.
// It returns the outcome as FinishStamp recorded it.
func (s *Service) Stamp(ctx context.Context, id, userID int64) (store.StampOutcome, error) {
	if !s.Enabled() {
		return store.StampOutcome{}, &ProblemsError{[]string{"La facturación está apagada en este servidor: " + s.Status.Problem + "."}}
	}
	c, err := s.Store.ComprobanteByID(ctx, id)
	if err != nil || c == nil {
		return store.StampOutcome{}, cmpErr(err)
	}
	if c.Sandbox != s.Client.Sandbox {
		return store.StampOutcome{}, ErrWrongEnvironment
	}
	request, attempt, err := s.Store.BeginStamp(ctx, id, userID, s.Now())
	if err != nil {
		return store.StampOutcome{}, err
	}
	// The call runs to completion even if the user's request goes away: its outcome must
	// be recorded either way.
	callCtx := context.WithoutCancel(ctx)
	res := s.Client.Stamp(callCtx, []byte(request))
	o := store.StampOutcome{UUID: res.UUID, XML: res.XML, EmailSent: res.EmailSent, Message: res.Message, Raw: res.Raw, Attempt: attempt}
	switch res.Outcome {
	case facturoporti.Done:
		o.Result = "stamped"
	case facturoporti.Refused:
		o.Result = "refused"
	default:
		o.Result = "unknown"
	}
	return o, s.Store.FinishStamp(callCtx, id, userID, o, s.Now())
}

func cmpErr(err error) error {
	if err == nil {
		return store.ErrComprobanteState
	}
	return err
}

// Cancel asks for a stamped comprobante's cancellation. Motivo 01 needs the UUID of the
// CFDI that replaces it.
func (s *Service) Cancel(ctx context.Context, id, userID int64, motivo, sustitucion string) (store.CancelOutcome, error) {
	if !s.Enabled() {
		return store.CancelOutcome{}, &ProblemsError{[]string{"La facturación está apagada en este servidor: " + s.Status.Problem + "."}}
	}
	if !store.ValidSATCode(store.CancelMotives, motivo) {
		return store.CancelOutcome{}, &ProblemsError{[]string{"Elige el motivo de cancelación."}}
	}
	if motivo == "01" && sustitucion == "" {
		return store.CancelOutcome{}, &ProblemsError{[]string{"El motivo 01 necesita la factura que sustituye a esta."}}
	}
	if motivo != "01" {
		sustitucion = ""
	}
	c, err := s.Store.ComprobanteByID(ctx, id)
	if err != nil || c == nil {
		return store.CancelOutcome{}, cmpErr(err)
	}
	if c.Sandbox != s.Client.Sandbox {
		return store.CancelOutcome{}, ErrWrongEnvironment
	}
	emisor, err := s.Store.EmisorSettings(ctx)
	if err != nil {
		return store.CancelOutcome{}, err
	}
	if err := s.Store.BeginCancel(ctx, id, userID, motivo, sustitucion); err != nil {
		return store.CancelOutcome{}, err
	}
	callCtx := context.WithoutCancel(ctx)
	total := facturoporti.Dec(int64(c.Total), 2)
	if c.Tipo == store.TipoPago {
		total = facturoporti.Dec(0, 2)
	}
	res := s.Client.Cancel(callCtx, facturoporti.CancelRequest{RFCEmisor: emisor.RFC, RFCReceptor: c.Customer.RFC, UUID: c.UUID,
		Total: total, Motivo: motivo, Sustitucion: sustitucion})
	o := store.CancelOutcome{Message: res.Message, Acuse: res.Acuse, Raw: res.Raw}
	switch res.Outcome {
	case facturoporti.Done:
		o.Result = "cancelled"
	case facturoporti.Pending:
		o.Result = "pending"
	case facturoporti.Refused:
		o.Result = "refused"
	default:
		o.Result = "unknown"
	}
	return o, s.Store.FinishCancel(callCtx, id, userID, o, s.Now())
}

// CheckCancellation asks the SAT how a pending cancellation stands and records it.
func (s *Service) CheckCancellation(ctx context.Context, id, userID int64) (store.CancelOutcome, error) {
	if !s.Enabled() {
		return store.CancelOutcome{}, &ProblemsError{[]string{"La facturación está apagada en este servidor: " + s.Status.Problem + "."}}
	}
	c, err := s.Store.ComprobanteByID(ctx, id)
	if err != nil || c == nil {
		return store.CancelOutcome{}, cmpErr(err)
	}
	if c.Status != store.CompCancelPend {
		return store.CancelOutcome{}, store.ErrComprobanteState
	}
	if c.Sandbox != s.Client.Sandbox {
		return store.CancelOutcome{}, ErrWrongEnvironment
	}
	emisor, err := s.Store.EmisorSettings(ctx)
	if err != nil {
		return store.CancelOutcome{}, err
	}
	total := facturoporti.Dec(int64(c.Total), 2)
	if c.Tipo == store.TipoPago {
		total = facturoporti.Dec(0, 2)
	}
	res := s.Client.Status(ctx, c.UUID, emisor.RFC, c.Customer.RFC, total)
	o := store.CancelOutcome{Message: strings.TrimSpace(res.Estado + " " + res.EstatusCancelacion), Raw: res.Raw}
	switch {
	case res.Outcome != facturoporti.Done:
		o.Result, o.Message = "unknown", res.Message
	case strings.EqualFold(res.Estado, "Cancelado"):
		o.Result = "cancelled"
	case strings.Contains(strings.ToLower(res.EstatusCancelacion), "rechaz"):
		o.Result = "vigente"
	default:
		o.Result = "pending"
	}
	return o, s.Store.FinishCancel(ctx, id, userID, o, s.Now())
}
