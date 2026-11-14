package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/pricing"
)

// Órdenes de compra a proveedor (issue #65; migrations/0034_purchase_orders.sql). A
// proyecto past O.C. recibida generates them from its current quote, at cost: a
// borrador, which its vendedor or an admin fills in, and which only an admin confirms.
// Confirming numbers it and freezes its PDF; a confirmed O.C. is corrected by a
// revision, which replaces it once confirmed.

// Purchase order statuses, as stored in purchase_orders.status. An admin cancels a
// confirmed O.C. (CancelPurchaseOrder, #66); a cancelled one counts for nothing.
const (
	POBorrador    = "borrador"
	POConfirmada  = "confirmada"
	POReemplazada = "reemplazada"
	POCancelada   = "cancelada"
)

// POStatusLabels are the words users see for each status.
var POStatusLabels = map[string]string{
	POBorrador:    "Borrador",
	POConfirmada:  "Confirmada",
	POReemplazada: "Reemplazada",
	POCancelada:   "Cancelada",
}

// Formas de pago of an O.C., in the order the form offers them.
const (
	POPayTransfer = "transferencia"
	POPayCash     = "contado"
	POPayCredit   = "credito"
	POPayOther    = "otra"
)

// POPaymentForms are the formas de pago with their labels, the default first.
var POPaymentForms = []SATCode{
	{POPayTransfer, "Transferencia electrónica por adelantado"},
	{POPayCash, "Pago de contado"},
	{POPayCredit, "Crédito"},
	{POPayOther, "Otra"},
}

// POCurrencies are the monedas an O.C. can be in; MXN is the default. Only the printed
// condition changes: nothing is converted.
var POCurrencies = []string{"MXN", "USD"}

// PurchaseOrder is a purchase_orders row with what its screens show next to it: its
// proyecto's folio, the supplier's current name, who generated and confirmed it, and the
// number of the O.C. it revises or of the revision that replaced it.
type PurchaseOrder struct {
	ID           int64
	ProjectID    int64
	ProjectFolio string
	QuoteID      int64
	SupersedesID *int64
	// SupersedesNumber is the number of the O.C. this one revises; ReplacedByNumber that
	// of the confirmed revision that replaced it.
	SupersedesNumber string
	ReplacedByNumber string
	// PendingRevisionID is the id of a revision of this O.C. still in borrador, 0 if none.
	PendingRevisionID  int64
	Seq                int64 // 0 on a new borrador
	Revision           int
	Number             string // "" on a borrador
	Status             string
	SupplierID         *int64
	SupplierName       string
	DeliveryAddress    string
	DeliveryReferences string
	Currency           string
	PaymentForm        string
	CreditDays         *int64
	PaymentOther       string
	// CFDIUse is the uso de CFDI the O.C. asks the supplier to invoice with, chosen per
	// O.C. (c_UsoCFDI); "" on an O.C. from before it was, whose document has the default.
	CFDIUse            string
	Subtotal           money.Centavos
	IVA                money.Centavos
	Total              money.Centavos
	RevisionReason     string
	CreatedBy          int64
	CreatedByName      string
	CreatedAt          string
	ConfirmedByName    string
	ConfirmedAt        string
	// Document is a confirmed O.C.'s frozen PDF content (JSON, pdf.PurchaseOrderDocument)
	// and PDFSHA256 its hash; both empty on a borrador.
	Document  string
	PDFSHA256 string
	// CancelReason, CancelledByName and CancelledAt say why, by whom and when a cancelada
	// O.C. was cancelled; empty on any other.
	CancelReason    string
	CancelledByName string
	CancelledAt     string
}

// Label is how the O.C. is named on screen: its number, or "Borrador" plus what it
// revises.
func (po PurchaseOrder) Label() string {
	if po.Number != "" {
		return po.Number
	}
	if po.SupersedesNumber != "" {
		return "Borrador de revisión de " + po.SupersedesNumber
	}
	return "Borrador"
}

// PurchaseOrderLine is one partida, at cost without margin (see LineFromQuote).
// KgPerUnit and PricePerKg are set on the types priced by weight; PricePerKg stays nil
// when the line's cost isn't all one price per kg.
type PurchaseOrderLine struct {
	LineNo          int
	QuoteLineNo     int
	Description     string
	QtyMilli        money.Milli
	KgPerUnit       *money.Micros
	PricePerKg      *money.Micros
	UnitPriceMicros money.Micros // the cost per unit, a whole number of centavos
	LineTotal       money.Centavos
}

// Errors the purchase order writes return.
var (
	// ErrPONotDraft: the O.C. isn't a borrador any more (it was confirmed meanwhile).
	ErrPONotDraft = errors.New("store: purchase order is not a draft")
	// ErrPONotAllowed: the proyecto can't take another O.C. (it isn't past O.C. recibida,
	// or it isn't QL and already has one), or the O.C. can't be revised (not confirmed,
	// or a revision is already in borrador).
	ErrPONotAllowed = errors.New("store: purchase order not allowed")
)

// ProjectTakesPurchaseOrders reports whether a proyecto in stage has supplier O.C.s:
// while it is en curso, once the client's O.C. is received; not while a prospecto, nor
// once lost or cancelled.
func ProjectTakesPurchaseOrders(stage string) bool {
	return stage == StageEnCurso
}

const poSelectCols = `
	po.id, po.project_id, p.folio, po.quote_id, po.supersedes_id,
	COALESCE((SELECT o.number FROM purchase_orders o WHERE o.id = po.supersedes_id), ''),
	COALESCE((SELECT r.number FROM purchase_orders r WHERE r.supersedes_id = po.id AND r.status != 'borrador'), ''),
	COALESCE((SELECT r.id FROM purchase_orders r WHERE r.supersedes_id = po.id AND r.status = 'borrador'), 0),
	COALESCE(po.seq, 0), po.revision, COALESCE(po.number, ''), po.status,
	po.supplier_id, COALESCE(s.name, ''), po.delivery_address, po.delivery_references,
	po.currency, po.payment_form, po.credit_days, po.payment_other, COALESCE(po.cfdi_use, ''),
	po.subtotal, po.iva, po.total, COALESCE(po.revision_reason, ''),
	po.created_by, cu.name, po.created_at, COALESCE(fu.name, ''), COALESCE(po.confirmed_at, ''),
	COALESCE(po.document, ''), COALESCE(po.pdf_sha256, ''),
	COALESCE(po.cancel_reason, ''), COALESCE(xu.name, ''), COALESCE(po.cancelled_at, '')`

const poFrom = `
	FROM purchase_orders po
	JOIN projects p ON p.id = po.project_id
	JOIN users cu ON cu.id = po.created_by
	LEFT JOIN users fu ON fu.id = po.confirmed_by
	LEFT JOIN users xu ON xu.id = po.cancelled_by
	LEFT JOIN suppliers s ON s.id = po.supplier_id`

func scanPO(row interface{ Scan(...any) error }) (*PurchaseOrder, error) {
	var po PurchaseOrder
	var supersedes, supplier, credit sql.NullInt64
	err := row.Scan(&po.ID, &po.ProjectID, &po.ProjectFolio, &po.QuoteID, &supersedes,
		&po.SupersedesNumber, &po.ReplacedByNumber, &po.PendingRevisionID,
		&po.Seq, &po.Revision, &po.Number, &po.Status,
		&supplier, &po.SupplierName, &po.DeliveryAddress, &po.DeliveryReferences,
		&po.Currency, &po.PaymentForm, &credit, &po.PaymentOther, &po.CFDIUse,
		&po.Subtotal, &po.IVA, &po.Total, &po.RevisionReason,
		&po.CreatedBy, &po.CreatedByName, &po.CreatedAt, &po.ConfirmedByName, &po.ConfirmedAt,
		&po.Document, &po.PDFSHA256, &po.CancelReason, &po.CancelledByName, &po.CancelledAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if supersedes.Valid {
		po.SupersedesID = &supersedes.Int64
	}
	if supplier.Valid {
		po.SupplierID = &supplier.Int64
	}
	if credit.Valid {
		po.CreditDays = &credit.Int64
	}
	return &po, nil
}

// PurchaseOrderByID returns the O.C., or nil if there is none.
func (s *Store) PurchaseOrderByID(ctx context.Context, id int64) (*PurchaseOrder, error) {
	po, err := scanPO(s.db.QueryRowContext(ctx, `SELECT `+poSelectCols+poFrom+` WHERE po.id = ?`, id))
	if err != nil {
		return nil, fmt.Errorf("store: purchase order %d: %w", id, err)
	}
	return po, nil
}

// ListProjectPurchaseOrders returns a proyecto's O.C.s: the numbered ones by number
// (each revision after the one it replaced), then the borradores, oldest first.
func (s *Store) ListProjectPurchaseOrders(ctx context.Context, projectID int64) ([]PurchaseOrder, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+poSelectCols+poFrom+`
		WHERE po.project_id = ?
		ORDER BY po.seq IS NULL, po.seq, po.revision, po.id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("store: purchase orders of project %d: %w", projectID, err)
	}
	defer rows.Close()
	var out []PurchaseOrder
	for rows.Next() {
		po, err := scanPO(rows)
		if err != nil {
			return nil, fmt.Errorf("store: purchase orders of project %d: %w", projectID, err)
		}
		out = append(out, *po)
	}
	return out, rows.Err()
}

// PurchaseOrderLines returns an O.C.'s partidas in order.
func (s *Store) PurchaseOrderLines(ctx context.Context, id int64) ([]PurchaseOrderLine, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT line_no, quote_line_no, description, qty_milli, kg_per_unit_micros,
			price_per_kg_micros, unit_price_micros, line_total
		FROM purchase_order_lines WHERE purchase_order_id = ? ORDER BY line_no`, id)
	if err != nil {
		return nil, fmt.Errorf("store: lines of purchase order %d: %w", id, err)
	}
	defer rows.Close()
	var out []PurchaseOrderLine
	for rows.Next() {
		var l PurchaseOrderLine
		var kg, perKg sql.NullInt64
		if err := rows.Scan(&l.LineNo, &l.QuoteLineNo, &l.Description, &l.QtyMilli, &kg, &perKg,
			&l.UnitPriceMicros, &l.LineTotal); err != nil {
			return nil, fmt.Errorf("store: lines of purchase order %d: %w", id, err)
		}
		if kg.Valid {
			v := money.Micros(kg.Int64)
			l.KgPerUnit = &v
		}
		if perKg.Valid {
			v := money.Micros(perKg.Int64)
			l.PricePerKg = &v
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ActivePOLines maps each partida of the proyecto's quote that is already in one of its
// O.C.s in force (borrador or confirmada), other than exceptID and the O.C. exceptID
// revises, to that O.C.'s number ("un borrador" for one without): on QL each O.C. takes
// its own partidas.
func (s *Store) ActivePOLines(ctx context.Context, projectID, exceptID int64) (map[int]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT l.quote_line_no, COALESCE(po.number, '')
		FROM purchase_order_lines l JOIN purchase_orders po ON po.id = l.purchase_order_id
		WHERE po.project_id = ? AND po.id != ? AND po.status IN ('borrador', 'confirmada')
			AND po.id != COALESCE((SELECT supersedes_id FROM purchase_orders WHERE id = ?), 0)
		ORDER BY po.id`, projectID, exceptID, exceptID)
	if err != nil {
		return nil, fmt.Errorf("store: purchase order lines of project %d: %w", projectID, err)
	}
	defer rows.Close()
	out := map[int]string{}
	for rows.Next() {
		var n int
		var number string
		if err := rows.Scan(&n, &number); err != nil {
			return nil, fmt.Errorf("store: purchase order lines of project %d: %w", projectID, err)
		}
		if number == "" {
			number = "un borrador"
		}
		if _, seen := out[n]; !seen {
			out[n] = number
		}
	}
	return out, rows.Err()
}

// inputsSnapshot is the part of quote_lines.pricing_inputs (web.pricingInputsSnapshot)
// a line's cost comes from: its flat cost (per kg when KgPerUnit is set, on the types
// costed per kg) and its materials, each as it stood when the quote was priced.
type inputsSnapshot struct {
	Cost      string `json:"cost"`
	KgPerUnit string `json:"kg_per_unit"`
	Materials []struct {
		QtyPerUnit string `json:"qty_per_unit"`
		Unit       string `json:"unit"`
		Price      string `json:"price"`
	} `json:"materials"`
}

// LineFromQuote is quote line l as an O.C. partida, at the cost it was quoted with:
// never the live catalog. A catalog line's cost is rebuilt from its frozen pricing
// inputs the way pricing.Product.Cost adds it up (flat cost plus each material's
// quantity × price); a free line's is its own cost. byWeight (the types quoted per
// metre by weight, views.ShowsKg) also gives the kg per metre and, when every part of
// the cost is one price per kg, that price: then the unit price is price per kg × kg per
// metre, as the O.C. prints it. The unit price is rounded half up to the centavo, as
// printed, and the total is quantity × that unit price, so the PDF's arithmetic checks
// by hand (Alumoclad 7#8: $95.00/kg × 0.3896 kg/m = $37.01/m; 111 m = $4,108.11). ok is
// false for a line with no cost to buy at (a free line without one).
func LineFromQuote(l QuoteLine, byWeight bool) (line PurchaseOrderLine, ok bool) {
	line = PurchaseOrderLine{QuoteLineNo: l.LineNo, Description: l.DescriptionSnapshot, QtyMilli: l.QtyMilli}
	var cost money.Micros
	switch {
	case l.ProductID == nil:
		if l.CostMicros == nil {
			return line, false
		}
		cost = *l.CostMicros
	default:
		if l.PricingInputsJSON == nil {
			return line, false
		}
		var snap inputsSnapshot
		if err := json.Unmarshal([]byte(*l.PricingInputsJSON), &snap); err != nil {
			return line, false
		}
		var p pricing.Product
		var kg money.Micros
		hasKg, onlyKg := false, true
		kgPrices := map[money.Micros]bool{}
		if snap.Cost != "" {
			c, err := money.ParseMicros(snap.Cost)
			if err != nil {
				return line, false
			}
			if snap.KgPerUnit != "" {
				q, err := money.ParseMicros(snap.KgPerUnit)
				if err != nil {
					return line, false
				}
				p.Materials = append(p.Materials, pricing.MaterialContent{QtyPerUnit: q, Price: c})
				kg, hasKg = kg+q, true
				kgPrices[c] = true
			} else {
				p.CostMicros = &c
				onlyKg = false
			}
		}
		for _, m := range snap.Materials {
			q, err := money.ParseMicros(m.QtyPerUnit)
			if err != nil {
				return line, false
			}
			price, err := money.ParseMicros(m.Price)
			if err != nil {
				return line, false
			}
			p.Materials = append(p.Materials, pricing.MaterialContent{QtyPerUnit: q, Price: price})
			if m.Unit == "kg" {
				kg, hasKg = kg+q, true
				kgPrices[price] = true
			} else {
				onlyKg = false
			}
		}
		c, has := p.Cost()
		if !has {
			return line, false
		}
		cost = c
		if byWeight && hasKg {
			line.KgPerUnit = &kg
			if onlyKg && len(kgPrices) == 1 {
				for price := range kgPrices {
					line.PricePerKg = &price
				}
				cost = money.Micros(money.RoundHalfUp(int64(kg)*int64(*line.PricePerKg), 1_000_000))
			}
		}
	}
	line.UnitPriceMicros = money.Micros(int64(cost.ToCentavosHalfUp()) * 10_000)
	line.LineTotal = money.LineTotalCentavos(line.UnitPriceMicros, line.QtyMilli)
	return line, true
}

// POTotals adds up an O.C.'s lines: subtotal, IVA at 16% on it, and total.
func POTotals(lines []PurchaseOrderLine) pricing.Totals {
	pl := make([]pricing.Line, len(lines))
	for i, l := range lines {
		pl[i] = pricing.Line{UnitPriceMicros: l.UnitPriceMicros, QtyMilli: l.QtyMilli}
	}
	return pricing.ComputeTotals(pl)
}

// PODraft is what a borrador's form saves: the supplier, the dirección de entrega, the
// conditions and its partidas. Nothing is required until it is confirmed (see
// PurchaseOrder problems in the web layer).
type PODraft struct {
	SupplierID         *int64
	DeliveryAddress    string
	DeliveryReferences string
	Currency           string
	PaymentForm        string
	CreditDays         *int64
	PaymentOther       string
	CFDIUse            string
	Lines              []PurchaseOrderLine
}

func (d PODraft) args() []any {
	var supplier, credit any
	if d.SupplierID != nil {
		supplier = *d.SupplierID
	}
	if d.CreditDays != nil {
		credit = *d.CreditDays
	}
	currency, form := d.Currency, d.PaymentForm
	if currency == "" {
		currency = "MXN"
	}
	if form == "" {
		form = POPayTransfer
	}
	t := POTotals(d.Lines)
	return []any{supplier, strings.TrimSpace(d.DeliveryAddress), strings.TrimSpace(d.DeliveryReferences),
		currency, form, credit, strings.TrimSpace(d.PaymentOther), nullIfEmpty(d.CFDIUse),
		int64(t.Subtotal), int64(t.IVA), int64(t.Total)}
}

// replacePOLines writes an O.C.'s lines in the caller's transaction, numbered in order.
func replacePOLines(ctx context.Context, tx *sql.Tx, id int64, lines []PurchaseOrderLine) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM purchase_order_lines WHERE purchase_order_id = ?`, id); err != nil {
		return fmt.Errorf("store: lines of purchase order %d: %w", id, err)
	}
	for i, l := range lines {
		var kg, perKg any
		if l.KgPerUnit != nil {
			kg = int64(*l.KgPerUnit)
		}
		if l.PricePerKg != nil {
			perKg = int64(*l.PricePerKg)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO purchase_order_lines (purchase_order_id, line_no, quote_line_no, description,
				qty_milli, kg_per_unit_micros, price_per_kg_micros, unit_price_micros, line_total)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, i+1, l.QuoteLineNo, l.Description, int64(l.QtyMilli), kg, perKg,
			int64(l.UnitPriceMicros), int64(l.LineTotal)); err != nil {
			return fmt.Errorf("store: lines of purchase order %d: line %d: %w", id, i+1, err)
		}
	}
	return nil
}

// CreatePurchaseOrder opens a borrador O.C. for the proyecto from its current quote,
// quoteID, with d's data, generated by userID. The proyecto has to be past O.C.
// recibida, and unless multiple (QL) it may not already have one in force (borrador or
// confirmada): ErrPONotAllowed otherwise.
func (s *Store) CreatePurchaseOrder(ctx context.Context, projectID, quoteID, userID int64, multiple bool, d PODraft) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("store: create purchase order: %w", err)
	}
	defer tx.Rollback()
	var stage string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM projects WHERE id = ?`, projectID).Scan(&stage); err != nil {
		return 0, fmt.Errorf("store: create purchase order: %w", err)
	}
	if !ProjectTakesPurchaseOrders(stage) {
		return 0, ErrPONotAllowed
	}
	if !multiple {
		var n int
		if err := tx.QueryRowContext(ctx, `
			SELECT count(*) FROM purchase_orders WHERE project_id = ? AND status IN ('borrador', 'confirmada')`,
			projectID).Scan(&n); err != nil {
			return 0, fmt.Errorf("store: create purchase order: %w", err)
		}
		if n > 0 {
			return 0, ErrPONotAllowed
		}
	}
	if err := stampActor(ctx, tx); err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO purchase_orders (supplier_id, delivery_address, delivery_references, currency,
			payment_form, credit_days, payment_other, cfdi_use, subtotal, iva, total,
			project_id, quote_id, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		append(d.args(), projectID, quoteID, userID)...)
	if err != nil {
		return 0, fmt.Errorf("store: create purchase order: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := replacePOLines(ctx, tx, id, d.Lines); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("store: create purchase order: %w", err)
	}
	return id, nil
}

// SavePurchaseOrderDraft overwrites a borrador with d. ErrPONotDraft if it isn't one.
func (s *Store) SavePurchaseOrderDraft(ctx context.Context, id int64, d PODraft) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: save purchase order %d: %w", id, err)
	}
	defer tx.Rollback()
	if err := stampActor(ctx, tx); err != nil {
		return err
	}
	if err := saveDraftTx(ctx, tx, id, d); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: save purchase order %d: %w", id, err)
	}
	return nil
}

func saveDraftTx(ctx context.Context, tx *sql.Tx, id int64, d PODraft) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE purchase_orders SET supplier_id = ?, delivery_address = ?, delivery_references = ?,
			currency = ?, payment_form = ?, credit_days = ?, payment_other = ?, cfdi_use = ?,
			subtotal = ?, iva = ?, total = ?
		WHERE id = ? AND status = 'borrador'`, append(d.args(), id)...)
	if err != nil {
		return fmt.Errorf("store: save purchase order %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrPONotDraft
	}
	return replacePOLines(ctx, tx, id, d.Lines)
}

// DeletePurchaseOrderDraft discards a borrador (a revision's too, leaving the O.C. it
// revised confirmed). ErrPONotDraft if it isn't one.
func (s *Store) DeletePurchaseOrderDraft(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: delete purchase order %d: %w", id, err)
	}
	defer tx.Rollback()
	if err := stampActor(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM purchase_order_lines WHERE purchase_order_id = ?
			AND EXISTS (SELECT 1 FROM purchase_orders WHERE id = ? AND status = 'borrador')`, id, id); err != nil {
		return fmt.Errorf("store: delete purchase order %d: %w", id, err)
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM purchase_orders WHERE id = ? AND status = 'borrador'`, id)
	if err != nil {
		return fmt.Errorf("store: delete purchase order %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrPONotDraft
	}
	return tx.Commit()
}

// InMexicoCity is t in México City, where Cladex is: an O.C.'s date is that day there.
func InMexicoCity(t time.Time) time.Time { return t.In(mexicoCity) }

// PONumber is the number confirming po on day (any time zone; the México City day
// counts) would give it, with the seq it takes: OC-<proyecto>-<DDMMAA>, plus -seq from
// the proyecto's second O.C. on (QL), plus -Rn on a revision, which keeps the number,
// date included, of the O.C. it revises (OC-QA0037-301026-R1).
func (s *Store) PONumber(ctx context.Context, po PurchaseOrder, day time.Time) (number string, seq int64, err error) {
	if po.SupersedesID != nil {
		prev, err := s.PurchaseOrderByID(ctx, *po.SupersedesID)
		if err != nil {
			return "", 0, err
		}
		if prev == nil || prev.Number == "" {
			return "", 0, ErrPONotAllowed
		}
		base := prev.Number
		if prev.Revision > 0 {
			base = strings.TrimSuffix(base, "-R"+strconv.Itoa(prev.Revision))
		}
		return base + "-R" + strconv.Itoa(po.Revision), prev.Seq, nil
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(seq), 0) + 1 FROM purchase_orders WHERE project_id = ?`, po.ProjectID).Scan(&seq); err != nil {
		return "", 0, fmt.Errorf("store: next purchase order of project %d: %w", po.ProjectID, err)
	}
	number = "OC-" + po.ProjectFolio + "-" + day.In(mexicoCity).Format("020106")
	if seq > 1 {
		number += "-" + strconv.FormatInt(seq, 10)
	}
	return number, seq, nil
}

// POConfirmation is what confirming freezes onto a borrador: its number and seq (from
// PONumber), the moment (UTC, whole seconds, the PDF's creation date), the PDF's content
// as JSON and its hash, and who confirmed it.
type POConfirmation struct {
	Number      string
	Seq         int64
	ConfirmedAt string
	Document    string
	PDFSHA256   string
	UserID      int64
	// HistoryNote is what the proyecto's history says about it.
	HistoryNote string
}

// ConfirmPurchaseOrder saves a borrador's last state (d) and confirms it with c, in one
// transaction: a revision first marks the O.C. it revises reemplazada. ErrPONotDraft if
// it was confirmed meanwhile; a number taken meanwhile fails on the unique indexes.
func (s *Store) ConfirmPurchaseOrder(ctx context.Context, id int64, d PODraft, c POConfirmation) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: confirm purchase order %d: %w", id, err)
	}
	defer tx.Rollback()
	if err := stampActor(ctx, tx); err != nil {
		return err
	}
	if err := saveDraftTx(ctx, tx, id, d); err != nil {
		return err
	}
	var projectID int64
	var supersedes sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT project_id, supersedes_id FROM purchase_orders WHERE id = ?`, id).
		Scan(&projectID, &supersedes); err != nil {
		return fmt.Errorf("store: confirm purchase order %d: %w", id, err)
	}
	if supersedes.Valid {
		res, err := tx.ExecContext(ctx, `
			UPDATE purchase_orders SET status = 'reemplazada' WHERE id = ? AND status = 'confirmada'`, supersedes.Int64)
		if err != nil {
			return fmt.Errorf("store: confirm purchase order %d: replace: %w", id, err)
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return ErrPONotAllowed
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE purchase_orders SET status = 'confirmada', number = ?, seq = ?, confirmed_at = ?,
			confirmed_by = ?, document = ?, pdf_sha256 = ?
		WHERE id = ? AND status = 'borrador'`,
		c.Number, c.Seq, c.ConfirmedAt, c.UserID, c.Document, c.PDFSHA256, id); err != nil {
		return fmt.Errorf("store: confirm purchase order %d: %w", id, err)
	}
	if err := projectHistory(ctx, tx, projectID, c.UserID, c.HistoryNote); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: confirm purchase order %d: %w", id, err)
	}
	return nil
}

// projectHistory adds note to the proyecto's history (a comment on its current quote).
func projectHistory(ctx context.Context, tx *sql.Tx, projectID, userID int64, note string) error {
	if strings.TrimSpace(note) == "" {
		return nil
	}
	quoteID, _, err := currentQuote(ctx, tx, projectID)
	if err != nil || quoteID == 0 {
		return fmt.Errorf("store: history of project %d: %w", projectID, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO quote_comments (quote_id, user_id, body) VALUES (?, ?, ?)`, quoteID, userID, note); err != nil {
		return fmt.Errorf("store: history of project %d: %w", projectID, err)
	}
	return nil
}

// CreatePurchaseOrderRevision opens a borrador revising confirmed O.C. id, a copy of its
// data and partidas, and records the reason in the proyecto's history. The original
// stays confirmada until the revision is confirmed. ErrPONotAllowed when id isn't
// confirmada or already has a revision in borrador.
func (s *Store) CreatePurchaseOrderRevision(ctx context.Context, id, userID int64, reason string) (int64, error) {
	reason = strings.TrimSpace(reason)
	prev, err := s.PurchaseOrderByID(ctx, id)
	if err != nil {
		return 0, err
	}
	if prev == nil || prev.Status != POConfirmada || prev.PendingRevisionID != 0 || reason == "" {
		return 0, ErrPONotAllowed
	}
	lines, err := s.PurchaseOrderLines(ctx, id)
	if err != nil {
		return 0, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("store: revise purchase order %d: %w", id, err)
	}
	defer tx.Rollback()
	if err := stampActor(ctx, tx); err != nil {
		return 0, err
	}
	d := PODraft{SupplierID: prev.SupplierID, DeliveryAddress: prev.DeliveryAddress,
		DeliveryReferences: prev.DeliveryReferences, Currency: prev.Currency, PaymentForm: prev.PaymentForm,
		CreditDays: prev.CreditDays, PaymentOther: prev.PaymentOther, CFDIUse: prev.CFDIUse, Lines: lines}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO purchase_orders (supplier_id, delivery_address, delivery_references, currency,
			payment_form, credit_days, payment_other, cfdi_use, subtotal, iva, total,
			project_id, quote_id, created_by, supersedes_id, seq, revision, revision_reason)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		append(d.args(), prev.ProjectID, prev.QuoteID, userID, id, prev.Seq, prev.Revision+1, reason)...)
	if err != nil {
		return 0, fmt.Errorf("store: revise purchase order %d: %w", id, err)
	}
	newID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := replacePOLines(ctx, tx, newID, lines); err != nil {
		return 0, err
	}
	note := "Revisión de la orden de compra " + prev.Number + ".\nMotivo: " + reason
	if err := projectHistory(ctx, tx, prev.ProjectID, userID, note); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("store: revise purchase order %d: %w", id, err)
	}
	return newID, nil
}

// DefaultSupplierOfQuoteType is the default supplier of the familia quote type typeID
// quotes, or nil when it has none (QL never has one).
func (s *Store) DefaultSupplierOfQuoteType(ctx context.Context, typeID int64) (*int64, error) {
	var id sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT f.default_supplier_id FROM quote_types t JOIN product_families f ON f.id = t.family_id
		WHERE t.id = ? AND f.free_lines_only = 0`, typeID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) || err == nil && !id.Valid {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: default supplier of quote type %d: %w", typeID, err)
	}
	return &id.Int64, nil
}

// SupplierByName finds an active supplier whose nombre comercial or razón social is
// name, ignoring case and surrounding spaces: how a QL quote's "Cotización del
// proveedor", typed as text, preselects its O.C.'s supplier. nil when none or several
// match.
func (s *Store) SupplierByName(ctx context.Context, name string) (*int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id FROM suppliers WHERE inactive = 0
			AND (trim(name) = ? COLLATE NOCASE OR trim(legal_name) = ? COLLATE NOCASE)`, name, name)
	if err != nil {
		return nil, fmt.Errorf("store: supplier named %q: %w", name, err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) != 1 {
		return nil, nil
	}
	return &ids[0], nil
}

// POSupplierOption is a supplier as an O.C.'s supplier list offers it, with the días de
// crédito the Crédito forma de pago starts from.
type POSupplierOption struct {
	ID         int64
	Name       string
	LegalName  string
	CreditDays *int64
	Inactive   bool
}

// POSupplierOptions lists every supplier by nombre comercial, inactive ones included
// (the form only offers those to an O.C. that already has one).
func (s *Store) POSupplierOptions(ctx context.Context) ([]POSupplierOption, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, legal_name, credit_days, inactive FROM suppliers ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, fmt.Errorf("store: supplier options: %w", err)
	}
	defer rows.Close()
	var out []POSupplierOption
	for rows.Next() {
		var o POSupplierOption
		var credit sql.NullInt64
		if err := rows.Scan(&o.ID, &o.Name, &o.LegalName, &credit, &o.Inactive); err != nil {
			return nil, fmt.Errorf("store: supplier options: %w", err)
		}
		if credit.Valid {
			o.CreditDays = &credit.Int64
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// SupplierPurchaseOrders names the O.C.s made out to the supplier: their numbers, or
// "borrador del proyecto X" for a borrador. A supplier with any can't be deleted.
func (s *Store) SupplierPurchaseOrders(ctx context.Context, supplierID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT COALESCE(po.number, 'borrador del proyecto ' || p.folio)
		FROM purchase_orders po JOIN projects p ON p.id = po.project_id
		WHERE po.supplier_id = ? ORDER BY po.id`, supplierID)
	if err != nil {
		return nil, fmt.Errorf("store: purchase orders of supplier %d: %w", supplierID, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// SupplierAddress is a supplier's fiscal address on one line, as an O.C. prints it:
// "Camino Real de Carretas 356, Milenio III, C.P. 76060, Querétaro, Querétaro".
func SupplierAddress(sup Supplier) string {
	var parts []string
	street := strings.TrimSpace(sup.Street + " " + sup.ExtNumber)
	for _, p := range []string{street, sup.Neighborhood} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if sup.PostalCode != "" {
		parts = append(parts, "C.P. "+sup.PostalCode)
	}
	for _, p := range []string{sup.Municipality, sup.State} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ", ")
}

// POConditions are an O.C.'s printed conditions: the moneda, the forma de pago, and the
// two fixed lines. A forma de pago "Otra" prints exactly as typed.
func POConditions(currency, form string, creditDays *int64, other string) []string {
	cur := "Precios en pesos mexicanos (MXN), más IVA."
	if currency == "USD" {
		cur = "Precios en dólares americanos (USD), más IVA."
	}
	var pay string
	switch form {
	case POPayCash:
		pay = "Forma de pago: de contado, contra entrega."
	case POPayCredit:
		days := ""
		if creditDays != nil {
			days = strconv.FormatInt(*creditDays, 10)
		}
		pay = "Forma de pago: crédito a " + days + " días."
	case POPayOther:
		pay = strings.TrimSpace(other)
	default:
		pay = "Forma de pago: transferencia electrónica, por adelantado conforme a la cotización de referencia."
	}
	out := []string{cur}
	if pay != "" {
		out = append(out, pay)
	}
	return append(out,
		"La mercancía se recibe sujeta a revisión de cantidad y calidad.",
		"Favor de emitir el CFDI con los datos de facturación indicados en esta orden.")
}
