package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Estado de entrega (#66; migrations/0042_entregas.sql). Delivery is marked per supplier
// O.C.: each confirmed one is "En proceso de entrega" until its vendedor or an admin marks
// it delivered, with the day and, optionally, a comprobante de entrega. A delivery
// belongs to the O.C. (its seq), not to one revision of it, so a revision keeps it. The
// proyecto is Entregado when every confirmed O.C. is (the project_states view).

// Delivery is the delivery in force of one supplier O.C. (the proyecto's O.C. with that
// Seq): the day, the comprobante de entrega (FileID and FileName, zero without one), and
// who marked it and when (CreatedByName is the user's current name).
type Delivery struct {
	ID              int64
	ProjectID       int64
	Seq             int64
	PurchaseOrderID int64
	DeliveredOn     string
	FileID          int64
	FileName        string
	CreatedByName   string
	CreatedAt       string
}

// Why a delivery or a cancellation is refused, besides ErrBadTransition for an O.C. that
// isn't confirmed, a proyecto that isn't en curso, or a missing motivo or file.
var (
	// ErrPODelivered: the O.C. is already delivered (marking it again, or cancelling it).
	ErrPODelivered = errors.New("store: purchase order is delivered")
	// ErrPONotDelivered: the O.C. has no delivery in force (adding a comprobante, undoing).
	ErrPONotDelivered = errors.New("store: purchase order is not delivered")
	// ErrDeliveryHasFile: the delivery already has its comprobante.
	ErrDeliveryHasFile = errors.New("store: delivery already has a comprobante")
)

// ListDeliveries returns the deliveries in force of a proyecto's supplier O.C.s, by seq.
func (s *Store) ListDeliveries(ctx context.Context, projectID int64) ([]Delivery, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.id, d.project_id, d.seq, d.purchase_order_id, d.delivered_on,
			COALESCE(d.file_id, 0), COALESCE(f.filename, ''), u.name, d.created_at
		FROM deliveries d
		JOIN users u ON u.id = d.created_by
		LEFT JOIN project_files f ON f.id = d.file_id
		WHERE d.project_id = ? AND d.undone_at IS NULL
		ORDER BY d.seq`, projectID)
	if err != nil {
		return nil, fmt.Errorf("store: deliveries of project %d: %w", projectID, err)
	}
	defer rows.Close()
	var out []Delivery
	for rows.Next() {
		var d Delivery
		if err := rows.Scan(&d.ID, &d.ProjectID, &d.Seq, &d.PurchaseOrderID, &d.DeliveredOn,
			&d.FileID, &d.FileName, &d.CreatedByName, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: deliveries of project %d: %w", projectID, err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeliveryOf is the delivery in force of O.C. po (by its proyecto and seq), or nil.
func (s *Store) DeliveryOf(ctx context.Context, po PurchaseOrder) (*Delivery, error) {
	if po.Seq == 0 {
		return nil, nil
	}
	all, err := s.ListDeliveries(ctx, po.ProjectID)
	if err != nil {
		return nil, err
	}
	for _, d := range all {
		if d.Seq == po.Seq {
			return &d, nil
		}
	}
	return nil, nil
}

// poInForce is a confirmed O.C. as the delivery writes read it in their transaction: its
// proyecto, seq and number, the proyecto's stage and the id of its delivery in force (0
// with none). ErrBadTransition when the O.C. doesn't exist or isn't confirmada.
type poInForce struct {
	projectID, seq int64
	number, stage  string
	deliveryID     int64
	hasFile        bool
}

func confirmedPO(ctx context.Context, tx *sql.Tx, poID int64) (poInForce, error) {
	var po poInForce
	err := tx.QueryRowContext(ctx, `
		SELECT po.project_id, po.seq, po.number, p.status, COALESCE(d.id, 0), d.file_id IS NOT NULL
		FROM purchase_orders po
		JOIN projects p ON p.id = po.project_id
		LEFT JOIN deliveries d ON d.project_id = po.project_id AND d.seq = po.seq AND d.undone_at IS NULL
		WHERE po.id = ? AND po.status = 'confirmada'`, poID).
		Scan(&po.projectID, &po.seq, &po.number, &po.stage, &po.deliveryID, &po.hasFile)
	if err == sql.ErrNoRows {
		return po, ErrBadTransition
	}
	return po, err
}

func validFile(file *NewFile) bool {
	return file != nil && strings.TrimSpace(file.Filename) != "" && len(file.Data) > 0
}

// MarkDelivered marks confirmed supplier O.C. poID of an en curso proyecto as delivered on
// day (YYYY-MM-DD; whether it may be in the future is the caller's decision), with the
// comprobante de entrega if file isn't nil, stored with the proyecto's files. The history
// gets "Marcado como entregado el [fecha] por [usuario]", plus "con comprobante", and the
// O.C.'s number. Who may mark it (the proyecto's vendedor or an admin) is the caller's
// decision. ErrPODelivered if it already is; anything else is ErrBadTransition.
func (s *Store) MarkDelivered(ctx context.Context, poID, userID int64, day string, file *NewFile) error {
	if _, err := time.Parse("2006-01-02", day); err != nil || file != nil && !validFile(file) {
		return ErrBadTransition
	}
	return s.inTx(ctx, "mark delivered", func(tx *sql.Tx) error {
		po, err := confirmedPO(ctx, tx, poID)
		if err != nil {
			return err
		}
		if po.stage != StageEnCurso {
			return ErrBadTransition
		}
		if po.deliveryID != 0 {
			return ErrPODelivered
		}
		var fileID any
		if file != nil {
			id, err := insertProjectFile(ctx, tx, po.projectID, userID, "entrega", file)
			if err != nil {
				return err
			}
			fileID = id
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO deliveries (project_id, seq, purchase_order_id, delivered_on, file_id, created_by)
			VALUES (?, ?, ?, ?, ?, ?)`, po.projectID, po.seq, poID, day, fileID, userID); err != nil {
			return err
		}
		name, err := userName(ctx, tx, userID)
		if err != nil {
			return err
		}
		body := "Marcado como entregado el " + dayText(day) + " por " + name
		if file != nil {
			body += " con comprobante"
		}
		return projectHistory(ctx, tx, po.projectID, userID, body+": orden de compra "+po.number+".")
	})
}

// AddDeliveryFile adds the comprobante de entrega to the delivery in force of confirmed
// O.C. poID, marked without one. The history gets "Comprobante de entrega agregado por
// [usuario]" with the O.C.'s number. ErrPONotDelivered without a delivery,
// ErrDeliveryHasFile when it has its comprobante already; anything else is
// ErrBadTransition. Who may (the proyecto's vendedor or an admin) is the caller's.
func (s *Store) AddDeliveryFile(ctx context.Context, poID, userID int64, file *NewFile) error {
	if !validFile(file) {
		return ErrBadTransition
	}
	return s.inTx(ctx, "add delivery file", func(tx *sql.Tx) error {
		po, err := confirmedPO(ctx, tx, poID)
		if err != nil {
			return err
		}
		switch {
		case po.deliveryID == 0:
			return ErrPONotDelivered
		case po.hasFile:
			return ErrDeliveryHasFile
		}
		id, err := insertProjectFile(ctx, tx, po.projectID, userID, "entrega", file)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE deliveries SET file_id = ? WHERE id = ? AND file_id IS NULL`, id, po.deliveryID); err != nil {
			return err
		}
		name, err := userName(ctx, tx, userID)
		if err != nil {
			return err
		}
		return projectHistory(ctx, tx, po.projectID, userID,
			"Comprobante de entrega agregado por "+name+": orden de compra "+po.number+".")
	})
}

// UndoDelivery takes confirmed O.C. poID from Entregado back to En proceso de entrega,
// for a delivery marked by mistake, with the reason, which is required. The delivery
// stays, marked undone with who, when and why; its comprobante stays with the proyecto's
// files. The history gets "Se regresó la orden de compra … a En proceso de entrega." and
// the motivo. ErrPONotDelivered without a delivery; anything else is ErrBadTransition.
// Who may (an admin) is the caller's decision.
func (s *Store) UndoDelivery(ctx context.Context, poID, userID int64, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ErrBadTransition
	}
	return s.inTx(ctx, "undo delivery", func(tx *sql.Tx) error {
		po, err := confirmedPO(ctx, tx, poID)
		if err != nil {
			return err
		}
		if po.deliveryID == 0 {
			return ErrPONotDelivered
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE deliveries SET undone_by = ?, undone_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), undo_reason = ?
			WHERE id = ?`, userID, reason, po.deliveryID); err != nil {
			return err
		}
		return projectHistory(ctx, tx, po.projectID, userID,
			"Se regresó la orden de compra "+po.number+" a En proceso de entrega.\nMotivo: "+reason)
	})
}

// CancelPurchaseOrder cancels confirmed supplier O.C. poID (to change supplier, say),
// with the reason, which is required: it stays, as Cancelada with who, when and why, and
// no longer counts for the proyecto's Estado de entrega, nor for what blocks cancelling
// the proyecto or undoing its O.C. recibida. A revision of it still in borrador is
// discarded with it. The history gets "Se canceló la orden de compra …" and the motivo.
// ErrPODelivered when it is delivered (an admin undoes that first); anything else is
// ErrBadTransition. Who may (an admin) is the caller's decision.
func (s *Store) CancelPurchaseOrder(ctx context.Context, poID, userID int64, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ErrBadTransition
	}
	return s.inTx(ctx, "cancel purchase order", func(tx *sql.Tx) error {
		po, err := confirmedPO(ctx, tx, poID)
		if err != nil {
			return err
		}
		if po.deliveryID != 0 {
			return ErrPODelivered
		}
		for _, q := range []string{
			`DELETE FROM purchase_order_lines WHERE purchase_order_id IN
				(SELECT id FROM purchase_orders WHERE supersedes_id = ?1 AND status = 'borrador')`,
			`DELETE FROM purchase_orders WHERE supersedes_id = ?1 AND status = 'borrador'`,
		} {
			if _, err := tx.ExecContext(ctx, q, poID); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE purchase_orders SET status = 'cancelada', cancel_reason = ?, cancelled_by = ?,
				cancelled_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
			WHERE id = ?`, reason, userID, poID); err != nil {
			return err
		}
		return projectHistory(ctx, tx, po.projectID, userID,
			"Se canceló la orden de compra "+po.number+".\nMotivo: "+reason)
	})
}

// userName is the user's current name.
func userName(ctx context.Context, tx *sql.Tx, userID int64) (string, error) {
	var name string
	err := tx.QueryRowContext(ctx, `SELECT name FROM users WHERE id = ?`, userID).Scan(&name)
	return name, err
}
