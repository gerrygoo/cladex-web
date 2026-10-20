package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gerrygoo/cladex-web/internal/money"
)

// Comprobante statuses (migrations/0024_facturacion.sql).
const (
	CompBorrador     = "borrador"
	CompLista        = "lista"
	CompTimbrando    = "timbrando"
	CompIncierto     = "incierto"
	CompRechazada    = "rechazada"
	CompTimbrada     = "timbrada"
	CompCancelPend   = "cancelacion_pendiente"
	CompCancelada    = "cancelada"
	TipoFactura      = "factura"
	TipoPago         = "pago"
	staleStampWindow = 2 * time.Minute
)

// ComprobanteStatusLabels are the words users see for each status.
var ComprobanteStatusLabels = map[string]string{
	CompBorrador:   "Borrador",
	CompLista:      "Lista para timbrar",
	CompTimbrando:  "Timbrando",
	CompIncierto:   "Timbrado incierto",
	CompRechazada:  "Rechazada",
	CompTimbrada:   "Timbrada",
	CompCancelPend: "Cancelación en espera",
	CompCancelada:  "Cancelada",
}

// ErrComprobanteState is returned when an action doesn't fit the comprobante's (or its
// proyecto's) current state; the caller re-reads it and says why.
var ErrComprobanteState = errors.New("store: comprobante is not in a state for that")

// Comprobante is a comprobantes row with its conceptos and what its screens show next to
// it.
type Comprobante struct {
	ID                int64
	Tipo              string
	ProjectID         int64
	ProjectFolio      string
	ProjectStatus     string
	QuoteID           int64
	QuoteFolio        string
	RelatedID         int64
	Status            string
	Serie             string
	Folio             int64 // 0 until the first validation
	UsoCFDI           string
	MetodoPago        string
	FormaPago         string
	Moneda            string
	TipoCambioMicros  *money.Micros
	Recipients        string
	PagoFecha         string
	Subtotal          money.Centavos
	IVA               money.Centavos
	Total             money.Centavos
	Request           string
	RequestSHA256     string
	Sandbox           bool
	Error             string
	UUID              string
	XML               string
	StampedAt         string
	EmailSent         bool
	CancelMotivo      string
	CancelSustitucion string
	CancelAcuse       string
	CancelledAt       string
	CreatedByName     string
	CreatedAt         string
	UpdatedAt         string
	// Customer is the receptor, read live from the customer while the comprobante is a
	// draft (the stamped XML is what counts after).
	Customer  Customer
	Conceptos []ComprobanteConcepto
}

// SerieFolio is how a comprobante is named: "F-12", or "" before its first validation.
func (c Comprobante) SerieFolio() string {
	if c.Folio == 0 {
		return ""
	}
	return fmt.Sprintf("%s-%d", c.Serie, c.Folio)
}

// Editable reports whether the comprobante is still a draft or can go back to one.
func (c Comprobante) Editable() bool {
	return c.Status == CompBorrador || c.Status == CompLista || c.Status == CompRechazada
}

// ComprobanteConcepto is one line of a factura.
type ComprobanteConcepto struct {
	ID              int64
	LineNo          int
	SATProductCode  string
	SATUnitCode     string
	UnitName        string
	Description     string
	QtyMilli        money.Milli
	UnitPriceMicros money.Micros
	Importe         money.Centavos
	IVA             money.Centavos
}

// ComprobanteEvent is one entry of a comprobante's history.
type ComprobanteEvent struct {
	Event     string
	Detail    string
	UserName  string
	CreatedAt string
}

const comprobanteCols = `
	c.id, c.tipo, c.project_id, p.folio, p.status, COALESCE(c.quote_id, 0), COALESCE(q.folio, ''),
	COALESCE(c.related_id, 0), c.status, COALESCE(c.serie, ''), COALESCE(c.folio, 0),
	COALESCE(c.uso_cfdi, ''), COALESCE(c.metodo_pago, ''), COALESCE(c.forma_pago, ''), c.moneda,
	c.tipo_cambio_micros, COALESCE(c.recipients, ''), COALESCE(c.pago_fecha, ''),
	c.subtotal, c.iva, c.total, COALESCE(c.request, ''), COALESCE(c.request_sha256, ''),
	COALESCE(c.sandbox, 0), COALESCE(c.error, ''), COALESCE(c.uuid, ''), COALESCE(c.xml, ''),
	COALESCE(c.stamped_at, ''), COALESCE(c.email_sent, 0), COALESCE(c.cancel_motivo, ''),
	COALESCE(c.cancel_sustitucion, ''), COALESCE(c.cancel_acuse, ''), COALESCE(c.cancelled_at, ''),
	u.name, c.created_at, c.updated_at, p.customer_id`

const comprobanteFrom = `
	FROM comprobantes c
	JOIN projects p ON p.id = c.project_id
	JOIN users u ON u.id = c.created_by
	LEFT JOIN quotes q ON q.id = c.quote_id`

func scanComprobante(row interface{ Scan(...any) error }) (*Comprobante, error) {
	var c Comprobante
	var tc sql.NullInt64
	var sandbox, email int
	err := row.Scan(&c.ID, &c.Tipo, &c.ProjectID, &c.ProjectFolio, &c.ProjectStatus, &c.QuoteID, &c.QuoteFolio,
		&c.RelatedID, &c.Status, &c.Serie, &c.Folio, &c.UsoCFDI, &c.MetodoPago, &c.FormaPago, &c.Moneda,
		&tc, &c.Recipients, &c.PagoFecha, &c.Subtotal, &c.IVA, &c.Total, &c.Request, &c.RequestSHA256,
		&sandbox, &c.Error, &c.UUID, &c.XML, &c.StampedAt, &email, &c.CancelMotivo, &c.CancelSustitucion,
		&c.CancelAcuse, &c.CancelledAt, &c.CreatedByName, &c.CreatedAt, &c.UpdatedAt, &c.Customer.ID)
	if err != nil {
		return nil, err
	}
	if tc.Valid {
		m := money.Micros(tc.Int64)
		c.TipoCambioMicros = &m
	}
	c.Sandbox, c.EmailSent = sandbox == 1, email == 1
	return &c, nil
}

// ComprobanteByID returns a comprobante with its conceptos and customer, or nil.
func (s *Store) ComprobanteByID(ctx context.Context, id int64) (*Comprobante, error) {
	c, err := scanComprobante(s.db.QueryRowContext(ctx, `SELECT `+comprobanteCols+comprobanteFrom+` WHERE c.id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: comprobante %d: %w", id, err)
	}
	cust, err := s.CustomerByID(ctx, c.Customer.ID)
	if err != nil {
		return nil, err
	}
	if cust == nil { // a soft-deleted customer still is the receptor
		row := s.db.QueryRowContext(ctx, `SELECT `+customerSelectCols+` FROM customers WHERE id = ?`, c.Customer.ID)
		if cust, err = scanCustomer(row); err != nil || cust == nil {
			return nil, fmt.Errorf("store: comprobante %d customer: %w", id, err)
		}
	}
	c.Customer = *cust
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, line_no, COALESCE(sat_product_code, ''), COALESCE(sat_unit_code, ''), COALESCE(unit_name, ''),
			description, qty_milli, unit_price_micros, importe, iva
		FROM comprobante_conceptos WHERE comprobante_id = ? ORDER BY line_no`, id)
	if err != nil {
		return nil, fmt.Errorf("store: comprobante %d conceptos: %w", id, err)
	}
	defer rows.Close()
	for rows.Next() {
		var l ComprobanteConcepto
		if err := rows.Scan(&l.ID, &l.LineNo, &l.SATProductCode, &l.SATUnitCode, &l.UnitName, &l.Description,
			&l.QtyMilli, &l.UnitPriceMicros, &l.Importe, &l.IVA); err != nil {
			return nil, fmt.Errorf("store: comprobante %d conceptos: %w", id, err)
		}
		c.Conceptos = append(c.Conceptos, l)
	}
	return c, rows.Err()
}

// ListComprobantes returns comprobantes, newest first, without conceptos: a proyecto's
// when projectID isn't 0, every one otherwise.
func (s *Store) ListComprobantes(ctx context.Context, projectID int64) ([]Comprobante, error) {
	where, args := "", []any{}
	if projectID != 0 {
		where, args = " WHERE c.project_id = ?", append(args, projectID)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+comprobanteCols+`, cu.name`+comprobanteFrom+`
		JOIN customers cu ON cu.id = p.customer_id`+where+` ORDER BY c.id DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: comprobantes: %w", err)
	}
	defer rows.Close()
	var out []Comprobante
	for rows.Next() {
		var name string
		c, err := scanComprobante(scanAppend{rows, &name})
		if err != nil {
			return nil, fmt.Errorf("store: comprobantes: %w", err)
		}
		c.Customer.Name = name
		out = append(out, *c)
	}
	return out, rows.Err()
}

// scanAppend scans a row with extra trailing columns into extra.
type scanAppend struct {
	rows  *sql.Rows
	extra *string
}

func (s scanAppend) Scan(dest ...any) error { return s.rows.Scan(append(dest, s.extra)...) }

// ListComprobanteEvents returns a comprobante's history, oldest first.
func (s *Store) ListComprobanteEvents(ctx context.Context, id int64) ([]ComprobanteEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.event, COALESCE(e.detail, ''), COALESCE(u.name, ''), e.created_at
		FROM comprobante_events e LEFT JOIN users u ON u.id = e.user_id
		WHERE e.comprobante_id = ? ORDER BY e.id`, id)
	if err != nil {
		return nil, fmt.Errorf("store: comprobante %d events: %w", id, err)
	}
	defer rows.Close()
	var out []ComprobanteEvent
	for rows.Next() {
		var e ComprobanteEvent
		if err := rows.Scan(&e.Event, &e.Detail, &e.UserName, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func addEvent(ctx context.Context, tx *sql.Tx, id, userID int64, event string, detail any) error {
	var d any
	switch v := detail.(type) {
	case nil:
	case string:
		d = v
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		d = string(b)
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO comprobante_events (comprobante_id, event, detail, user_id) VALUES (?, ?, ?, ?)`,
		id, event, d, nullIfZero(userID))
	return err
}

// ivaOf is 16% of an importe, rounded half up to the centavo.
func ivaOf(importe money.Centavos) money.Centavos {
	return money.Centavos(money.RoundHalfUp(int64(importe)*16, 100))
}

func (s *Store) inTx(ctx context.Context, what string, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: %s: %w", what, err)
	}
	defer tx.Rollback()
	if err := stampActor(ctx, tx); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		if errors.Is(err, ErrComprobanteState) {
			return err
		}
		return fmt.Errorf("store: %s: %w", what, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: %s: %w", what, err)
	}
	return nil
}

// activeComprobante is a proyecto's factura (not a replacement) that isn't cancelled, 0
// when none.
func activeFactura(ctx context.Context, tx *sql.Tx, projectID int64) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `
		SELECT id FROM comprobantes
		WHERE project_id = ? AND tipo = 'factura' AND related_id IS NULL AND status != 'cancelada'
		ORDER BY id DESC LIMIT 1`, projectID).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return id, err
}

// CreateFacturaDraft starts the factura of a proyecto in O.C. recibida from its issued
// quote: its lines (amounts as quoted), the customer's uso de CFDI and email, the purchase
// order's método de pago (P.P.D. facturas carry forma de pago 99), and SAT codes from each
// product's unit and the series' familia. A proyecto has one factura at a time; a
// cancelled one doesn't count.
func (s *Store) CreateFacturaDraft(ctx context.Context, projectID, userID int64) (int64, error) {
	var id int64
	err := s.inTx(ctx, "create factura draft", func(tx *sql.Tx) error {
		var status, method, use, email, currency, familyCode string
		var quoteID int64
		err := tx.QueryRowContext(ctx, `
			SELECT p.status, COALESCE(p.payment_method, ''), COALESCE(cu.cfdi_use, ''), COALESCE(cu.email, ''),
				q.id, COALESCE(q.currency, 'MXN'), COALESCE(f.sat_product_code, '')
			FROM projects p
			JOIN customers cu ON cu.id = p.customer_id
			JOIN quotes q ON q.project_id = p.id AND q.status = 'emitida'
			LEFT JOIN product_families f ON f.series = q.prefix
			WHERE p.id = ?`, projectID).Scan(&status, &method, &use, &email, &quoteID, &currency, &familyCode)
		if err == sql.ErrNoRows || err == nil && (status != "oc_recibida" || method == "") {
			return ErrComprobanteState
		}
		if err != nil {
			return err
		}
		if existing, err := activeFactura(ctx, tx, projectID); err != nil || existing != 0 {
			if err != nil {
				return err
			}
			return ErrComprobanteState
		}
		forma := ""
		if method == "PPD" {
			forma = "99"
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO comprobantes (tipo, project_id, quote_id, uso_cfdi, metodo_pago, forma_pago, moneda, recipients, created_by)
			VALUES ('factura', ?, ?, NULLIF(?, ''), ?, NULLIF(?, ''), ?, NULLIF(?, ''), ?)`,
			projectID, quoteID, use, method, forma, currency, strings.TrimSpace(email), userID)
		if err != nil {
			return err
		}
		if id, err = res.LastInsertId(); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO comprobante_conceptos (comprobante_id, line_no, quote_line_id, sat_product_code, sat_unit_code,
				unit_name, description, qty_milli, unit_price_micros, importe, iva)
			SELECT ?, l.line_no, l.id, NULLIF(?, ''), un.sat_code, un.name, l.description_snapshot, l.qty_milli,
				l.unit_price_micros, l.line_total, 0
			FROM quote_lines l
			LEFT JOIN products pr ON pr.id = l.product_id
			LEFT JOIN units un ON un.id = pr.unit_id
			WHERE l.quote_id = ? ORDER BY l.line_no`, id, familyCode, quoteID); err != nil {
			return err
		}
		if err := recomputeTotals(ctx, tx, id); err != nil {
			return err
		}
		return addEvent(ctx, tx, id, userID, "created", nil)
	})
	return id, err
}

// recomputeTotals sets each concepto's IVA and the comprobante's totals from its
// conceptos: IVA per line, rounded, then added up, as the CFDI states it.
func recomputeTotals(ctx context.Context, tx *sql.Tx, id int64) error {
	rows, err := tx.QueryContext(ctx, `SELECT id, importe FROM comprobante_conceptos WHERE comprobante_id = ?`, id)
	if err != nil {
		return err
	}
	type line struct {
		id      int64
		importe money.Centavos
	}
	var lines []line
	for rows.Next() {
		var l line
		if err := rows.Scan(&l.id, &l.importe); err != nil {
			rows.Close()
			return err
		}
		lines = append(lines, l)
	}
	rows.Close()
	var sub, iva money.Centavos
	for _, l := range lines {
		li := ivaOf(l.importe)
		if _, err := tx.ExecContext(ctx, `UPDATE comprobante_conceptos SET iva = ? WHERE id = ?`, li, l.id); err != nil {
			return err
		}
		sub, iva = sub+l.importe, iva+li
	}
	_, err = tx.ExecContext(ctx, `UPDATE comprobantes SET subtotal = ?, iva = ?, total = ? WHERE id = ?`, sub, iva, sub+iva, id)
	return err
}

// CreatePagoDraft starts the comprobante de pago for a stamped P.P.D. factura: one payment
// for its whole total, dated today, its forma de pago still to pick.
func (s *Store) CreatePagoDraft(ctx context.Context, facturaID, userID int64, today string) (int64, error) {
	var id int64
	err := s.inTx(ctx, "create pago draft", func(tx *sql.Tx) error {
		var projectID int64
		var status, method, moneda string
		var tc sql.NullInt64
		var total money.Centavos
		err := tx.QueryRowContext(ctx, `
			SELECT project_id, status, COALESCE(metodo_pago, ''), moneda, tipo_cambio_micros, total
			FROM comprobantes WHERE id = ? AND tipo = 'factura'`, facturaID).Scan(&projectID, &status, &method, &moneda, &tc, &total)
		if err == sql.ErrNoRows || err == nil && (status != CompTimbrada || method != "PPD") {
			return ErrComprobanteState
		}
		if err != nil {
			return err
		}
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM comprobantes WHERE related_id = ? AND tipo = 'pago' AND status != 'cancelada'`,
			facturaID).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrComprobanteState
		}
		var email string
		tx.QueryRowContext(ctx, `SELECT COALESCE(recipients, '') FROM comprobantes WHERE id = ?`, facturaID).Scan(&email)
		res, err := tx.ExecContext(ctx, `
			INSERT INTO comprobantes (tipo, project_id, related_id, moneda, tipo_cambio_micros, recipients, pago_fecha, subtotal, iva, total, created_by)
			VALUES ('pago', ?, ?, ?, ?, NULLIF(?, ''), ?, 0, 0, ?, ?)`,
			projectID, facturaID, moneda, tc, email, today, total, userID)
		if err != nil {
			return err
		}
		if id, err = res.LastInsertId(); err != nil {
			return err
		}
		return addEvent(ctx, tx, id, userID, "created", nil)
	})
	return id, err
}

// CreateReplacementDraft starts a factura that replaces a stamped one (the SAT's
// sustitución, tipo de relación 04): a copy of its lines and data, to correct before
// stamping. Once it is stamped, the original is cancelled with motivo 01 pointing at it.
func (s *Store) CreateReplacementDraft(ctx context.Context, facturaID, userID int64) (int64, error) {
	var id int64
	err := s.inTx(ctx, "create replacement draft", func(tx *sql.Tx) error {
		var status, tipo string
		if err := tx.QueryRowContext(ctx, `SELECT status, tipo FROM comprobantes WHERE id = ?`, facturaID).Scan(&status, &tipo); err != nil {
			if err == sql.ErrNoRows {
				return ErrComprobanteState
			}
			return err
		}
		if tipo != TipoFactura || status != CompTimbrada {
			return ErrComprobanteState
		}
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM comprobantes WHERE related_id = ? AND tipo = 'factura' AND status != 'cancelada'`,
			facturaID).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrComprobanteState
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO comprobantes (tipo, project_id, quote_id, related_id, uso_cfdi, metodo_pago, forma_pago, moneda,
				tipo_cambio_micros, recipients, created_by)
			SELECT 'factura', project_id, quote_id, id, uso_cfdi, metodo_pago, forma_pago, moneda, tipo_cambio_micros, recipients, ?
			FROM comprobantes WHERE id = ?`, userID, facturaID)
		if err != nil {
			return err
		}
		if id, err = res.LastInsertId(); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO comprobante_conceptos (comprobante_id, line_no, quote_line_id, sat_product_code, sat_unit_code,
				unit_name, description, qty_milli, unit_price_micros, importe, iva)
			SELECT ?, line_no, quote_line_id, sat_product_code, sat_unit_code, unit_name, description, qty_milli,
				unit_price_micros, importe, iva
			FROM comprobante_conceptos WHERE comprobante_id = ?`, id, facturaID); err != nil {
			return err
		}
		if err := recomputeTotals(ctx, tx, id); err != nil {
			return err
		}
		return addEvent(ctx, tx, id, userID, "created", map[string]any{"sustituye": facturaID})
	})
	return id, err
}

// DraftEdit is what can change on a draft. Conceptos are matched by id; amounts never
// change.
type DraftEdit struct {
	UsoCFDI          string
	FormaPago        string
	TipoCambioMicros *money.Micros
	Recipients       string
	PagoFecha        string
	Conceptos        []ConceptoEdit
}

type ConceptoEdit struct {
	ID             int64
	SATProductCode string
	SATUnitCode    string
	UnitName       string
	Description    string
}

// UpdateDraft saves a draft's editable fields. A comprobante that is lista or rechazada
// goes back to borrador: its frozen request no longer matches.
func (s *Store) UpdateDraft(ctx context.Context, id, userID int64, e DraftEdit) error {
	return s.inTx(ctx, "update comprobante draft", func(tx *sql.Tx) error {
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM comprobantes WHERE id = ?`, id).Scan(&status); err != nil {
			return err
		}
		if status != CompBorrador && status != CompLista && status != CompRechazada {
			return ErrComprobanteState
		}
		var tc any
		if e.TipoCambioMicros != nil {
			tc = int64(*e.TipoCambioMicros)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE comprobantes SET status = 'borrador', uso_cfdi = NULLIF(?, ''), forma_pago = NULLIF(?, ''),
				tipo_cambio_micros = ?, recipients = NULLIF(?, ''), pago_fecha = NULLIF(?, ''),
				request = NULL, request_sha256 = NULL, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
			WHERE id = ?`, e.UsoCFDI, e.FormaPago, tc, strings.TrimSpace(e.Recipients), e.PagoFecha, id); err != nil {
			return err
		}
		for _, c := range e.Conceptos {
			if _, err := tx.ExecContext(ctx, `
				UPDATE comprobante_conceptos SET sat_product_code = NULLIF(?, ''), sat_unit_code = NULLIF(?, ''),
					unit_name = NULLIF(?, ''), description = ?
				WHERE id = ? AND comprobante_id = ?`,
				strings.TrimSpace(c.SATProductCode), strings.ToUpper(strings.TrimSpace(c.SATUnitCode)),
				strings.TrimSpace(c.UnitName), strings.TrimSpace(c.Description), c.ID, id); err != nil {
				return err
			}
		}
		if status != CompBorrador {
			return addEvent(ctx, tx, id, userID, "edited", map[string]string{"from": status})
		}
		return nil
	})
}

// Validate freezes a draft as lista: it assigns the serie and folio on the first
// validation (serie from the emisor), calls build with them for the exact request to
// send, and stores it and its hash. sandbox records which Facturoporti host it is meant
// for. build's error is returned as is, leaving the draft unchanged (its folio aside).
func (s *Store) Validate(ctx context.Context, id, userID int64, serie string, sandbox bool, build func(serie string, folio int64) ([]byte, error)) error {
	return s.inTx(ctx, "validate comprobante", func(tx *sql.Tx) error {
		var status, curSerie string
		var folio int64
		if err := tx.QueryRowContext(ctx, `SELECT status, COALESCE(serie, ''), COALESCE(folio, 0) FROM comprobantes WHERE id = ?`,
			id).Scan(&status, &curSerie, &folio); err != nil {
			return err
		}
		if status != CompBorrador {
			return ErrComprobanteState
		}
		if folio == 0 {
			curSerie = serie
			if err := tx.QueryRowContext(ctx, `
				INSERT INTO folio_sequences (prefix, next_number) VALUES (?, 2)
				ON CONFLICT (prefix) DO UPDATE SET next_number = next_number + 1
				RETURNING next_number - 1`, "cfdi:"+serie).Scan(&folio); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE comprobantes SET serie = ?, folio = ? WHERE id = ?`, curSerie, folio, id); err != nil {
				return err
			}
		}
		req, err := build(curSerie, folio)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(req)
		hash := hex.EncodeToString(sum[:])
		if _, err := tx.ExecContext(ctx, `
			UPDATE comprobantes SET status = 'lista', request = ?, request_sha256 = ?, sandbox = ?, error = NULL,
				updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
			WHERE id = ?`, string(req), hash, sandbox, id); err != nil {
			return err
		}
		return addEvent(ctx, tx, id, userID, "validated", map[string]any{"serie": curSerie, "folio": folio, "sha256": hash, "sandbox": sandbox})
	})
}

// ToBorrador takes a lista or rechazada comprobante back to borrador.
func (s *Store) ToBorrador(ctx context.Context, id, userID int64) error {
	return s.inTx(ctx, "comprobante to borrador", func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE comprobantes SET status = 'borrador', request = NULL, request_sha256 = NULL,
				updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
			WHERE id = ? AND status IN ('lista', 'rechazada')`, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return ErrComprobanteState
		}
		return addEvent(ctx, tx, id, userID, "edited", nil)
	})
}

// BeginStamp marks a comprobante timbrando and records the attempt before the call, so a
// crash mid-call leaves evidence. It takes a lista or incierto one, or one stuck
// timbrando for longer than a call can take. It returns the stored request to send:
// always the same bytes, which is what makes a resend after an unknown outcome safe.
func (s *Store) BeginStamp(ctx context.Context, id, userID int64, now time.Time) (request string, attempt int, err error) {
	err = s.inTx(ctx, "begin stamp", func(tx *sql.Tx) error {
		var status, updated, hash string
		if err := tx.QueryRowContext(ctx, `SELECT status, updated_at, COALESCE(request, ''), COALESCE(request_sha256, '') FROM comprobantes WHERE id = ?`,
			id).Scan(&status, &updated, &request, &hash); err != nil {
			return err
		}
		stale := false
		if t, err := time.Parse(time.RFC3339Nano, updated); err == nil && now.Sub(t) > staleStampWindow {
			stale = true
		}
		if request == "" || !(status == CompLista || status == CompIncierto || status == CompTimbrando && stale) {
			return ErrComprobanteState
		}
		if err := tx.QueryRowContext(ctx, `SELECT count(*) + 1 FROM comprobante_events WHERE comprobante_id = ? AND event = 'stamp_requested'`,
			id).Scan(&attempt); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE comprobantes SET status = 'timbrando', updated_at = ? WHERE id = ?`,
			now.UTC().Format(time.RFC3339Nano), id); err != nil {
			return err
		}
		return addEvent(ctx, tx, id, userID, "stamp_requested", map[string]any{"attempt": attempt, "sha256": hash})
	})
	return request, attempt, err
}

// StampOutcome is what a stamp call returned, as FinishStamp records it.
type StampOutcome struct {
	Result    string // "stamped", "refused" or "unknown"
	UUID      string
	XML       string
	EmailSent bool
	Message   string
	Raw       string
	Attempt   int
}

// FinishStamp records a stamp call's outcome. Stamped: the comprobante is timbrada, for
// good, and its proyecto moves with it, in the same transaction: a factura records the
// invoice (O.C. recibida → Facturado, as RecordInvoice), a replacement updates the
// proyecto's factura reference, a pago records the payment (as RecordPayment). If the
// proyecto can't move (someone moved it by hand meanwhile), the stamp is still recorded
// and the history says so. Refused: rechazada, with the reason. Unknown: incierto.
func (s *Store) FinishStamp(ctx context.Context, id, userID int64, o StampOutcome, now time.Time) error {
	return s.inTx(ctx, "finish stamp", func(tx *sql.Tx) error {
		var status, tipo, serie string
		var folio, projectID, relatedID int64
		if err := tx.QueryRowContext(ctx, `SELECT status, tipo, COALESCE(serie, ''), COALESCE(folio, 0), project_id, COALESCE(related_id, 0)
			FROM comprobantes WHERE id = ?`, id).Scan(&status, &tipo, &serie, &folio, &projectID, &relatedID); err != nil {
			return err
		}
		if status != CompTimbrando {
			return ErrComprobanteState
		}
		detail := map[string]any{"attempt": o.Attempt, "message": o.Message, "response": o.Raw}
		switch o.Result {
		case "stamped":
			stampedAt := now.UTC().Format(time.RFC3339Nano)
			if _, err := tx.ExecContext(ctx, `
				UPDATE comprobantes SET status = 'timbrada', uuid = ?, xml = ?, stamped_at = ?, email_sent = ?, error = NULL,
					updated_at = ?
				WHERE id = ?`, o.UUID, o.XML, stampedAt, o.EmailSent, stampedAt, id); err != nil {
				return err
			}
			detail["uuid"] = o.UUID
			if err := addEvent(ctx, tx, id, userID, "stamp_succeeded", detail); err != nil {
				return err
			}
			return projectAfterStamp(ctx, tx, id, userID, tipo, relatedID, projectID, fmt.Sprintf("%s-%d", serie, folio), o.UUID, now)
		case "refused":
			if _, err := tx.ExecContext(ctx, `UPDATE comprobantes SET status = 'rechazada', error = ?, updated_at = ? WHERE id = ?`,
				o.Message, now.UTC().Format(time.RFC3339Nano), id); err != nil {
				return err
			}
			return addEvent(ctx, tx, id, userID, "stamp_refused", detail)
		default:
			if _, err := tx.ExecContext(ctx, `UPDATE comprobantes SET status = 'incierto', error = ?, updated_at = ? WHERE id = ?`,
				o.Message, now.UTC().Format(time.RFC3339Nano), id); err != nil {
				return err
			}
			return addEvent(ctx, tx, id, userID, "stamp_unknown", detail)
		}
	})
}

// mexicoCity is where Cladex issues its CFDIs; a stamp's day is that day there.
var mexicoCity = func() *time.Location {
	if l, err := time.LoadLocation("America/Mexico_City"); err == nil {
		return l
	}
	return time.FixedZone("CST", -6*3600)
}()

func projectAfterStamp(ctx context.Context, tx *sql.Tx, id, userID int64, tipo string, relatedID, projectID int64, ref, uuid string, now time.Time) error {
	day := now.In(mexicoCity).Format("2006-01-02")
	doc := Document{Ref: ref, Date: day}
	note := "Folio fiscal " + uuid
	var err error
	switch {
	case tipo == TipoPago:
		err = recordPaymentTx(ctx, tx, projectID, userID, doc, note)
	case relatedID != 0:
		var res sql.Result
		res, err = tx.ExecContext(ctx, `UPDATE projects SET invoice_ref = ? WHERE id = ? AND invoice_ref IS NOT NULL`, ref, projectID)
		if err == nil {
			if n, _ := res.RowsAffected(); n == 1 {
				var quoteID int64
				if quoteID, _, err = currentQuote(ctx, tx, projectID); err == nil && quoteID != 0 {
					_, err = tx.ExecContext(ctx, `INSERT INTO quote_comments (quote_id, user_id, body) VALUES (?, ?, ?)`,
						quoteID, userID, "Factura sustituida por "+ref+" · "+note)
				}
			} else {
				err = ErrBadTransition
			}
		}
	default:
		err = recordInvoiceTx(ctx, tx, projectID, userID, doc, note)
	}
	if errors.Is(err, ErrBadTransition) {
		// The stamp is real whatever the proyecto's state; say so instead of failing.
		return addEvent(ctx, tx, id, userID, "project_not_moved", "el proyecto ya no estaba en la etapa esperada; actualízalo a mano")
	}
	return err
}

// BeginCancel records a cancellation request before the call.
func (s *Store) BeginCancel(ctx context.Context, id, userID int64, motivo, sustitucion string) error {
	return s.inTx(ctx, "begin cancel", func(tx *sql.Tx) error {
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM comprobantes WHERE id = ?`, id).Scan(&status); err != nil {
			return err
		}
		if status != CompTimbrada && status != CompCancelPend {
			return ErrComprobanteState
		}
		if _, err := tx.ExecContext(ctx, `UPDATE comprobantes SET cancel_motivo = ?, cancel_sustitucion = NULLIF(?, '') WHERE id = ?`,
			motivo, sustitucion, id); err != nil {
			return err
		}
		return addEvent(ctx, tx, id, userID, "cancel_requested", map[string]string{"motivo": motivo, "sustitucion": sustitucion})
	})
}

// CancelOutcome is a cancellation call's or status check's result.
type CancelOutcome struct {
	Result  string // "cancelled", "pending", "refused", "unknown", "vigente"
	Message string
	Acuse   string
	Raw     string
}

// FinishCancel records a cancellation's outcome: cancelled, waiting on the receptor, or
// still timbrada (refused, unknown, or the receptor rejected it: "vigente").
func (s *Store) FinishCancel(ctx context.Context, id, userID int64, o CancelOutcome, now time.Time) error {
	return s.inTx(ctx, "finish cancel", func(tx *sql.Tx) error {
		ts := now.UTC().Format(time.RFC3339Nano)
		detail := map[string]string{"result": o.Result, "message": o.Message, "response": o.Raw}
		var err error
		switch o.Result {
		case "cancelled":
			_, err = tx.ExecContext(ctx, `UPDATE comprobantes SET status = 'cancelada', cancelled_at = ?, cancel_acuse = COALESCE(NULLIF(?, ''), cancel_acuse), updated_at = ?
				WHERE id = ? AND status IN ('timbrada', 'cancelacion_pendiente')`, ts, o.Acuse, ts, id)
		case "pending":
			_, err = tx.ExecContext(ctx, `UPDATE comprobantes SET status = 'cancelacion_pendiente', cancel_acuse = COALESCE(NULLIF(?, ''), cancel_acuse), updated_at = ?
				WHERE id = ? AND status IN ('timbrada', 'cancelacion_pendiente')`, o.Acuse, ts, id)
		case "vigente":
			_, err = tx.ExecContext(ctx, `UPDATE comprobantes SET status = 'timbrada', updated_at = ? WHERE id = ? AND status = 'cancelacion_pendiente'`, ts, id)
		}
		if err != nil {
			return err
		}
		return addEvent(ctx, tx, id, userID, "cancel_"+o.Result, detail)
	})
}
