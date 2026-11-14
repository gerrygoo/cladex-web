package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gerrygoo/cladex-web/internal/cfdi"
	"github.com/gerrygoo/cladex-web/internal/money"
)

// Comprobante origins (migrations/0044_facturas_manuales.sql).
const (
	OrigenTimbrado = "timbrado"
	OrigenManual   = "manual"
)

// ManualError is why an uploaded CFDI can't be recorded, in words for the user.
type ManualError struct{ Msg string }

func (e *ManualError) Error() string { return e.Msg }

func manualErr(format string, args ...any) error { return &ManualError{fmt.Sprintf(format, args...)} }

// ManualFactura is a factura issued outside the app (#63): its stamped XML, required, its
// PDF, optional, and for a P.U.E. one the day the client paid (it is only issued once
// paid).
type ManualFactura struct {
	XML    []byte
	PDF    *NewFile
	PaidOn string
}

// MaxCFDISize caps an uploaded CFDI XML.
const MaxCFDISize = 1 << 20

// xmlFolio is how a CFDI names itself: "A-153", "153" without a serie, or "" without a folio.
func xmlFolio(x *cfdi.Comprobante) string {
	switch {
	case x.Folio == "":
		return ""
	case x.Serie == "":
		return x.Folio
	}
	return x.Serie + "-" + x.Folio
}

// xmlCentavos is an XML amount in centavos.
func xmlCentavos(s string) (money.Centavos, error) {
	m, err := money.ParseMicros(s)
	if err != nil {
		return 0, err
	}
	return m.ToCentavosHalfUp(), nil
}

// RecordManualFactura records a factura issued outside the app on a proyecto en curso
// with no factura in force: a comprobante with origen manual, status timbrada, its
// amounts, método, forma and uso de CFDI read from the XML, which is kept as the legal
// document. Its PDF, if given, goes with the proyecto's files. The Estado de facturación
// follows from it like from a stamped one.
//
// A P.U.E. factura needs PaidOn: it records the payment of what is still owed on the
// proyecto (up to the factura's total) as paid that day, source factura, so an anticipo
// already recorded isn't counted twice. The history gets "Factura … registrada" with its
// folio fiscal and, for P.U.E., "Pagado el …".
//
// What the XML says that doesn't match the proyecto (total, RFC) is not refused here: the
// factura is what the SAT has, and its page warns (ManualWarnings). Problems with the file
// itself are a *ManualError; a proyecto not en curso, without its O.C. or already
// facturado is ErrBadTransition.
func (s *Store) RecordManualFactura(ctx context.Context, projectID, userID int64, m ManualFactura) (int64, error) {
	if len(m.XML) == 0 || len(m.XML) > MaxCFDISize {
		return 0, manualErr("Sube el XML de la factura (hasta 1 MB).")
	}
	x, err := cfdi.Parse(string(m.XML))
	if err != nil {
		return 0, manualErr("El archivo no es un CFDI timbrado: no trae folio fiscal.")
	}
	if x.TipoDeComprobante != "I" {
		return 0, manualErr("El XML no es una factura (tipo de comprobante I, ingreso); es de tipo %q.", x.TipoDeComprobante)
	}
	if x.MetodoPago != "PUE" && x.MetodoPago != "PPD" {
		return 0, manualErr("El XML no dice si la factura es PUE o PPD.")
	}
	moneda := x.Moneda
	if moneda != "MXN" && moneda != "USD" {
		return 0, manualErr("La factura está en %q; solo se registran facturas en MXN o USD.", moneda)
	}
	subtotal, err1 := xmlCentavos(x.SubTotal)
	total, err2 := xmlCentavos(x.Total)
	if err1 != nil || err2 != nil {
		return 0, manualErr("No se pudo leer el subtotal o el total del XML.")
	}
	var iva money.Centavos
	if x.Impuestos.TotalImpuestosTrasladados != "" {
		if iva, err = xmlCentavos(x.Impuestos.TotalImpuestosTrasladados); err != nil {
			return 0, manualErr("No se pudo leer el IVA del XML.")
		}
	}
	if x.MetodoPago == "PUE" {
		if _, err := time.Parse("2006-01-02", m.PaidOn); err != nil {
			return 0, manualErr("Con PUE la factura se emite ya cobrada: confirma que ya se cobró y escribe la fecha del pago.")
		}
	}
	if m.PDF != nil && (strings.TrimSpace(m.PDF.Filename) == "" || len(m.PDF.Data) == 0) {
		m.PDF = nil
	}
	issued := x.Fecha
	if len(issued) >= 10 {
		issued = issued[:10]
	}
	uuid := x.UUID()

	var id int64
	err = s.inTx(ctx, "record manual factura", func(tx *sql.Tx) error {
		var status, ocNumber string
		var quoteTotal, paid money.Centavos
		err := tx.QueryRowContext(ctx, `
			SELECT p.status, COALESCE(p.oc_number, ''), q.total, ps.paid FROM projects p
			JOIN quotes q ON q.project_id = p.id AND q.status != 'revisada'
			JOIN project_states ps ON ps.project_id = p.id
			WHERE p.id = ?`, projectID).Scan(&status, &ocNumber, &quoteTotal, &paid)
		if err == sql.ErrNoRows || err == nil && (status != StageEnCurso || ocNumber == "") {
			return ErrBadTransition
		}
		if err != nil {
			return err
		}
		if active, err := activeFactura(ctx, tx, projectID); err != nil {
			return err
		} else if active != 0 {
			return ErrBadTransition
		}
		var dup int64
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM comprobantes WHERE uuid = ?`, uuid).Scan(&dup); err != nil {
			return err
		}
		if dup > 0 {
			return manualErr("Esa factura ya está registrada (folio fiscal %s).", uuid)
		}
		quoteID, _, err := currentQuote(ctx, tx, projectID)
		if err != nil {
			return err
		}
		var pdfID any
		if m.PDF != nil {
			fid, err := insertProjectFile(ctx, tx, projectID, userID, "factura", m.PDF)
			if err != nil {
				return err
			}
			pdfID = fid
		}
		ref := xmlFolio(x)
		res, err := tx.ExecContext(ctx, `
			INSERT INTO comprobantes (tipo, project_id, quote_id, status, origen, ext_folio, uso_cfdi, metodo_pago,
				forma_pago, moneda, subtotal, iva, total, uuid, xml, stamped_at, pdf_file_id, created_by)
			VALUES ('factura', ?, ?, 'timbrada', 'manual', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			projectID, quoteID, nullIfEmpty(ref), nullIfEmpty(x.Receptor.UsoCFDI), x.MetodoPago,
			nullIfEmpty(x.FormaPago), moneda, subtotal, iva, total, uuid, string(m.XML), x.Timbre.FechaTimbrado, pdfID, userID)
		if err != nil {
			return err
		}
		if id, err = res.LastInsertId(); err != nil {
			return err
		}
		if err := addEvent(ctx, tx, id, userID, "manual_recorded", map[string]any{
			"uuid": uuid, "total": total.String(), "rfc": x.Receptor.Rfc, "pdf": m.PDF != nil}); err != nil {
			return err
		}
		name := "Factura"
		if ref != "" {
			name += " " + ref
		}
		body := name + " registrada (" + x.MetodoPago + ", emitida el " + dayText(issued) + ") · Folio fiscal " + uuid
		if x.MetodoPago == "PUE" {
			if owed := min(total, quoteTotal-paid); owed > 0 {
				if _, err := tx.ExecContext(ctx, `
					INSERT INTO payments (project_id, amount, paid_on, source, comprobante_id, created_by)
					VALUES (?, ?, ?, 'factura', ?, ?)`, projectID, owed, m.PaidOn, id, userID); err != nil {
					return err
				}
			}
			body = "Pagado el " + dayText(m.PaidOn) + " · " + body
		}
		return addProjectComment(ctx, tx, quoteID, userID, body)
	})
	return id, err
}

// CancelManualFactura marks a manual comprobante (a factura or a complemento de pago)
// cancelled once it was cancelled outside the app, with the SAT motivo and a note, both
// required (the note goes to the history). A cancelled factura puts the Estado de
// facturación back to Pendiente until another is recorded; a cancelled complemento stops
// counting toward its factura's saldo. Payments stay (the money did come in). A factura
// with a complemento de pago in force can't be cancelled before it, as the SAT requires.
func (s *Store) CancelManualFactura(ctx context.Context, id, userID int64, motivo, note string) error {
	note = strings.TrimSpace(note)
	if note == "" || !ValidSATCode(CancelMotives, motivo) {
		return ErrComprobanteState
	}
	return s.inTx(ctx, "cancel manual comprobante", func(tx *sql.Tx) error {
		var projectID int64
		var tipo, ref, uuid string
		err := tx.QueryRowContext(ctx, `
			SELECT project_id, tipo, COALESCE(ext_folio, ''), uuid FROM comprobantes
			WHERE id = ? AND origen = 'manual' AND status = 'timbrada'`, id).Scan(&projectID, &tipo, &ref, &uuid)
		if err == sql.ErrNoRows {
			return ErrComprobanteState
		}
		if err != nil {
			return err
		}
		var pagos int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM comprobantes WHERE related_id = ? AND tipo = 'pago' AND status IN ('timbrada', 'cancelacion_pendiente')`,
			id).Scan(&pagos); err != nil {
			return err
		}
		if tipo == TipoFactura && pagos > 0 {
			return ErrComprobanteState
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE comprobantes SET status = 'cancelada', cancel_motivo = ?, cancelled_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'),
				updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
			WHERE id = ?`, motivo, id); err != nil {
			return err
		}
		if err := addEvent(ctx, tx, id, userID, "manual_cancelled", map[string]string{"motivo": motivo, "note": note}); err != nil {
			return err
		}
		quoteID, _, err := currentQuote(ctx, tx, projectID)
		if err != nil {
			return err
		}
		name, done := "Factura", "marcada como cancelada"
		if tipo == TipoPago {
			name, done = "Complemento de pago", "marcado como cancelado"
		}
		if ref != "" {
			name += " " + ref
		}
		return addProjectComment(ctx, tx, quoteID, userID,
			name+" "+done+" · Folio fiscal "+uuid+" · Motivo: "+motivo+" · "+note)
	})
}

// ManualComplemento is a complemento de pago issued outside the app for a P.P.D. factura
// (#63): its stamped XML, required, its PDF, optional, and LinkPaymentID, a payment already
// recorded by hand in Cobro that this complemento covers (0 records a new one).
type ManualComplemento struct {
	XML           []byte
	PDF           *NewFile
	LinkPaymentID int64
}

// ErrPaymentMismatch is returned when the payment chosen to link isn't one that can be
// linked to the complemento: not the proyecto's, already linked or deleted, or a different
// amount.
var ErrPaymentMismatch = errors.New("store: payment can't be linked to that complemento")

// RecordManualComplemento records a complemento de pago issued outside the app for the
// P.P.D. factura facturaID (timbrada, whichever its origen), on a proyecto en curso: a
// comprobante tipo pago, origen manual, related to the factura, with the amount it pays on
// that factura (the sum of its ImpPagado for the factura's UUID) as its total and the
// day of the payment. Several may pay one factura (parcialidades); together they can't
// pass its total. Once they cover it, Facturación is Facturada (project_states).
//
// The payment goes to Cobro: linked to LinkPaymentID when given (an unlinked payment of the
// proyecto for the same amount, else ErrPaymentMismatch), so a payment already recorded
// isn't counted twice; otherwise a new one, source complemento, which can't pass what is
// still owed (a *ManualError says to link it instead). The history gets "Complemento de
// pago … registrado" with the amount, the day, the factura's saldo and the folio fiscal.
func (s *Store) RecordManualComplemento(ctx context.Context, facturaID, userID int64, m ManualComplemento) (int64, error) {
	if len(m.XML) == 0 || len(m.XML) > MaxCFDISize {
		return 0, manualErr("Sube el XML del complemento de pago (hasta 1 MB).")
	}
	x, err := cfdi.Parse(string(m.XML))
	if err != nil {
		return 0, manualErr("El archivo no es un CFDI timbrado: no trae folio fiscal.")
	}
	if x.TipoDeComprobante != "P" || x.Pagos == nil {
		return 0, manualErr("El XML no es un complemento de pago (tipo de comprobante P); es de tipo %q.", x.TipoDeComprobante)
	}
	if m.PDF != nil && (strings.TrimSpace(m.PDF.Filename) == "" || len(m.PDF.Data) == 0) {
		m.PDF = nil
	}
	uuid := x.UUID()

	var id int64
	err = s.inTx(ctx, "record manual complemento", func(tx *sql.Tx) error {
		var projectID int64
		var projectStatus, facturaUUID, facturaRef string
		var facturaTotal, quoteTotal, paid money.Centavos
		err := tx.QueryRowContext(ctx, `
			SELECT f.project_id, p.status, f.uuid, COALESCE(f.ext_folio, CASE WHEN f.folio IS NULL THEN '' ELSE f.serie || '-' || f.folio END),
				f.total, q.total, ps.paid
			FROM comprobantes f
			JOIN projects p ON p.id = f.project_id
			JOIN quotes q ON q.project_id = p.id AND q.status != 'revisada'
			JOIN project_states ps ON ps.project_id = p.id
			WHERE f.id = ? AND f.tipo = 'factura' AND f.status = 'timbrada' AND f.metodo_pago = 'PPD'`,
			facturaID).Scan(&projectID, &projectStatus, &facturaUUID, &facturaRef, &facturaTotal, &quoteTotal, &paid)
		if err == sql.ErrNoRows || err == nil && projectStatus != StageEnCurso {
			return ErrComprobanteState
		}
		if err != nil {
			return err
		}
		var amount money.Centavos
		var paidOn, forma, moneda string
		for _, p := range x.Pagos.Pagos {
			for _, d := range p.Documentos {
				if !strings.EqualFold(strings.TrimSpace(d.IdDocumento), facturaUUID) {
					continue
				}
				c, err := xmlCentavos(d.ImpPagado)
				if err != nil {
					return manualErr("No se pudo leer el importe pagado del complemento.")
				}
				amount += c
				if paidOn == "" {
					paidOn, forma, moneda = p.FechaPago, p.FormaDePagoP, p.MonedaP
				}
			}
		}
		if amount <= 0 {
			return manualErr("El complemento no paga esta factura (folio fiscal %s).", facturaUUID)
		}
		if len(paidOn) >= 10 {
			paidOn = paidOn[:10]
		}
		if _, err := time.Parse("2006-01-02", paidOn); err != nil {
			return manualErr("El complemento no trae la fecha del pago.")
		}
		if moneda != "MXN" && moneda != "USD" {
			moneda = "MXN"
		}
		var covered money.Centavos
		if err := tx.QueryRowContext(ctx, `
			SELECT COALESCE(SUM(total), 0) FROM comprobantes
			WHERE related_id = ? AND tipo = 'pago' AND status IN ('timbrada', 'cancelacion_pendiente')`, facturaID).Scan(&covered); err != nil {
			return err
		}
		if amount > facturaTotal-covered {
			return manualErr("El complemento paga %s, más que el saldo de la factura (%s).", amount, facturaTotal-covered)
		}
		var dup int64
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM comprobantes WHERE uuid = ?`, uuid).Scan(&dup); err != nil {
			return err
		}
		if dup > 0 {
			return manualErr("Ese complemento ya está registrado (folio fiscal %s).", uuid)
		}
		quoteID, _, err := currentQuote(ctx, tx, projectID)
		if err != nil {
			return err
		}
		var pdfID any
		if m.PDF != nil {
			fid, err := insertProjectFile(ctx, tx, projectID, userID, "complemento", m.PDF)
			if err != nil {
				return err
			}
			pdfID = fid
		}
		ref := xmlFolio(x)
		res, err := tx.ExecContext(ctx, `
			INSERT INTO comprobantes (tipo, project_id, quote_id, related_id, status, origen, ext_folio, forma_pago, moneda,
				pago_fecha, subtotal, iva, total, uuid, xml, stamped_at, pdf_file_id, created_by)
			VALUES ('pago', ?, ?, ?, 'timbrada', 'manual', ?, ?, ?, ?, 0, 0, ?, ?, ?, ?, ?, ?)`,
			projectID, quoteID, facturaID, nullIfEmpty(ref), nullIfEmpty(forma), moneda, paidOn, amount, uuid,
			string(m.XML), x.Timbre.FechaTimbrado, pdfID, userID)
		if err != nil {
			return err
		}
		if id, err = res.LastInsertId(); err != nil {
			return err
		}
		linked := ""
		if m.LinkPaymentID != 0 {
			res, err := tx.ExecContext(ctx, `
				UPDATE payments SET comprobante_id = ?
				WHERE id = ? AND project_id = ? AND deleted_at IS NULL AND comprobante_id IS NULL AND amount = ?`,
				id, m.LinkPaymentID, projectID, amount)
			if err != nil {
				return err
			}
			if n, err := res.RowsAffected(); err != nil {
				return err
			} else if n != 1 {
				return ErrPaymentMismatch
			}
			linked = " · Ligado al pago ya registrado"
		} else {
			if amount > quoteTotal-paid {
				return manualErr("El pago del complemento (%s) es más que el saldo pendiente en Cobro (%s). Si ese pago ya está registrado, elígelo para ligarlo.",
					amount, quoteTotal-paid)
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO payments (project_id, amount, paid_on, source, comprobante_id, created_by)
				VALUES (?, ?, ?, 'complemento', ?, ?)`, projectID, amount, paidOn, id, userID); err != nil {
				return err
			}
		}
		if err := addEvent(ctx, tx, id, userID, "manual_recorded", map[string]any{
			"uuid": uuid, "total": amount.String(), "paid_on": paidOn, "pdf": m.PDF != nil, "linked_payment": m.LinkPaymentID}); err != nil {
			return err
		}
		name := "Complemento de pago"
		if ref != "" {
			name += " " + ref
		}
		of := "la factura"
		if facturaRef != "" {
			of += " " + facturaRef
		}
		body := fmt.Sprintf("%s registrado: %s pagado el %s · Saldo de %s: %s%s · Folio fiscal %s",
			name, amount, dayText(paidOn), of, facturaTotal-covered-amount, linked, uuid)
		return addProjectComment(ctx, tx, quoteID, userID, body)
	})
	return id, err
}
