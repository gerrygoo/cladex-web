package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gerrygoo/cladex-web/internal/money"
)

// Payments the client made toward a proyecto (#67; migrations/0041_project_states.sql).
// Each one is a row of payments, whatever recorded it: "Registrar pago" on the proyecto
// (manual), or, with #63, a P.U.E. factura or a complemento de pago. Their sum against the
// proyecto's total with IVA is its Estado de cobro (the project_states view).

// Payment sources (payments.source).
const (
	PaymentSourceManual      = "manual"
	PaymentSourceFactura     = "factura"
	PaymentSourceComplemento = "complemento"
)

// Payment is one payment of a proyecto, as its Cobro section lists it: the amount (with
// IVA) and the day it came in, the comprobante de pago the user uploaded (FileID and
// FileName, zero without one), and who recorded it and when (CreatedByName is the user's
// current name).
type Payment struct {
	ID            int64
	ProjectID     int64
	Amount        money.Centavos
	PaidOn        string
	Source        string
	ComprobanteID int64
	FileID        int64
	FileName      string
	CreatedByName string
	CreatedAt     string
	// FormaPago is how the client paid (SAT c_FormaPago, #87): typed with a manual
	// payment, the comprobante's for one a factura or complemento recorded, "" for manual
	// payments recorded before it was asked.
	FormaPago string
	// The comprobante linked to it (ComprobanteID; #83): its tipo, status and folio
	// fiscal, "" when none.
	ComprobanteTipo   string
	ComprobanteStatus string
	ComprobanteUUID   string
}

// Covered reports whether a comprobante in force (a stamped factura or complemento whose
// cancellation hasn't gone through) covers the payment. A draft doesn't, nor a cancelled
// one: that payment still needs its complemento.
func (p Payment) Covered() bool {
	return p.ComprobanteStatus == CompTimbrada || p.ComprobanteStatus == CompCancelPend
}

// PagoDraft reports whether the payment's complemento de pago is on its way: a draft, or
// being stamped.
func (p Payment) PagoDraft() bool {
	return p.ComprobanteTipo == TipoPago && p.ComprobanteStatus != "" && !p.Covered() && p.ComprobanteStatus != CompCancelada
}

// ErrPaymentHasComplemento is returned when deleting a payment that a complemento de pago
// in force (or being stamped) covers: it is cancelled first.
var ErrPaymentHasComplemento = errors.New("store: payment has a complemento de pago")

// uncoveredPayment is the SQL condition of a payment (alias pm) that no comprobante in
// force covers.
const uncoveredPayment = `NOT EXISTS (SELECT 1 FROM comprobantes uc WHERE uc.id = pm.comprobante_id AND uc.status IN ('timbrada', 'cancelacion_pendiente'))`

// ErrPaymentOverBalance is returned when a payment would take what the client paid past
// the proyecto's total with IVA.
var ErrPaymentOverBalance = errors.New("store: payment is more than the balance")

// ListPayments returns a proyecto's payments, not the deleted ones, oldest first by the
// day they came in.
func (s *Store) ListPayments(ctx context.Context, projectID int64) ([]Payment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT pm.id, pm.project_id, pm.amount, pm.paid_on, pm.source, COALESCE(pm.comprobante_id, 0),
			COALESCE(pm.file_id, 0), COALESCE(f.filename, ''), u.name, pm.created_at,
			COALESCE(c.tipo, ''), COALESCE(c.status, ''), COALESCE(c.uuid, ''),
			COALESCE(pm.forma_pago, CASE WHEN pm.source != 'manual' THEN c.forma_pago END, '')
		FROM payments pm
		JOIN users u ON u.id = pm.created_by
		LEFT JOIN project_files f ON f.id = pm.file_id
		LEFT JOIN comprobantes c ON c.id = pm.comprobante_id
		WHERE pm.project_id = ? AND pm.deleted_at IS NULL
		ORDER BY pm.paid_on, pm.id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("store: payments of project %d: %w", projectID, err)
	}
	defer rows.Close()
	var out []Payment
	for rows.Next() {
		var p Payment
		if err := rows.Scan(&p.ID, &p.ProjectID, &p.Amount, &p.PaidOn, &p.Source, &p.ComprobanteID,
			&p.FileID, &p.FileName, &p.CreatedByName, &p.CreatedAt,
			&p.ComprobanteTipo, &p.ComprobanteStatus, &p.ComprobanteUUID, &p.FormaPago); err != nil {
			return nil, fmt.Errorf("store: payments of project %d: %w", projectID, err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// RecordPayment records a payment the client made toward an en curso proyecto by hand
// ("Registrar pago"): amount (centavos with IVA, more than zero and no more than what is
// still owed, else ErrPaymentOverBalance), the day it came in (YYYY-MM-DD; whether it may
// be in the future is the caller's decision), how the client paid (formaPago, a SAT
// c_FormaPago code other than 99 Por definir, #87) and, optionally, the comprobante de pago,
// stored with the proyecto's files. The history gets "Pago registrado: $X el [fecha] por
// [usuario]", plus "con comprobante" when there is a file. Anything else is
// ErrBadTransition.
func (s *Store) RecordPayment(ctx context.Context, projectID, userID int64, amount money.Centavos, paidOn, formaPago string, file *NewFile) error {
	if _, err := time.Parse("2006-01-02", paidOn); err != nil || amount <= 0 || !PaymentFormKnown(formaPago) ||
		file != nil && (strings.TrimSpace(file.Filename) == "" || len(file.Data) == 0) {
		return ErrBadTransition
	}
	return s.inTx(ctx, "record payment", func(tx *sql.Tx) error {
		var total, paid money.Centavos
		err := tx.QueryRowContext(ctx, `
			SELECT q.total, ps.paid FROM projects p
			JOIN quotes q ON q.project_id = p.id AND q.status != 'revisada'
			JOIN project_states ps ON ps.project_id = p.id
			WHERE p.id = ? AND p.status = 'en_curso'`, projectID).Scan(&total, &paid)
		if err == sql.ErrNoRows {
			return ErrBadTransition
		}
		if err != nil {
			return err
		}
		if amount > total-paid {
			return ErrPaymentOverBalance
		}
		var fileID any
		if file != nil {
			id, err := insertProjectFile(ctx, tx, projectID, userID, "pago", file)
			if err != nil {
				return err
			}
			fileID = id
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO payments (project_id, amount, paid_on, source, file_id, forma_pago, created_by)
			VALUES (?, ?, ?, 'manual', ?, ?, ?)`, projectID, amount, paidOn, fileID, formaPago, userID); err != nil {
			return err
		}
		var name string
		if err := tx.QueryRowContext(ctx, `SELECT name FROM users WHERE id = ?`, userID).Scan(&name); err != nil {
			return err
		}
		body := "Pago registrado: " + amount.String() + " el " + dayText(paidOn) + " por " + name
		if file != nil {
			body += " con comprobante"
		}
		quoteID, _, err := currentQuote(ctx, tx, projectID)
		if err != nil {
			return err
		}
		return addProjectComment(ctx, tx, quoteID, userID, body+".")
	})
}

// DeletePayment takes a payment recorded by mistake out of the proyecto's cobro, with the
// reason, which is required. The row stays, marked deleted with who did it, when and why,
// and the history gets "Pago eliminado: $X del [fecha]." plus the reason. Its comprobante
// file stays with the proyecto's files. Who may delete (an admin) is the caller's
// decision. A payment that isn't the proyecto's, or is already deleted, is
// ErrBadTransition. One with a complemento de pago in force or being stamped is
// ErrPaymentHasComplemento; a complemento still in draft is discarded with it (#83).
func (s *Store) DeletePayment(ctx context.Context, projectID, paymentID, userID int64, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ErrBadTransition
	}
	return s.inTx(ctx, "delete payment", func(tx *sql.Tx) error {
		var amount money.Centavos
		var paidOn, tipo, status string
		var compID int64
		err := tx.QueryRowContext(ctx, `
			SELECT pm.amount, pm.paid_on, COALESCE(c.id, 0), COALESCE(c.tipo, ''), COALESCE(c.status, '')
			FROM payments pm LEFT JOIN comprobantes c ON c.id = pm.comprobante_id
			WHERE pm.id = ? AND pm.project_id = ? AND pm.deleted_at IS NULL`,
			paymentID, projectID).Scan(&amount, &paidOn, &compID, &tipo, &status)
		if err == sql.ErrNoRows {
			return ErrBadTransition
		}
		if err != nil {
			return err
		}
		if tipo == TipoPago {
			switch status {
			case CompBorrador, CompLista, CompRechazada:
				if _, err := tx.ExecContext(ctx, `UPDATE comprobantes SET status = 'cancelada', request = NULL, request_sha256 = NULL,
					updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`, compID); err != nil {
					return err
				}
				if err := addEvent(ctx, tx, compID, userID, "discarded", map[string]any{"payment": paymentID}); err != nil {
					return err
				}
			case CompCancelada:
			default:
				return ErrPaymentHasComplemento
			}
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE payments SET deleted_by = ?, deleted_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), delete_reason = ?
			WHERE id = ?`, userID, reason, paymentID); err != nil {
			return err
		}
		quoteID, _, err := currentQuote(ctx, tx, projectID)
		if err != nil {
			return err
		}
		return addProjectComment(ctx, tx, quoteID, userID,
			"Pago eliminado: "+amount.String()+" del "+dayText(paidOn)+".\n"+reason)
	})
}
