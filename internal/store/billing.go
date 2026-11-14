package store

import (
	"context"
	"fmt"
)

// BillingRow is one proyecto of the Facturas table (#63): a proyecto en curso, with the
// factura that bills it, if any.
type BillingRow struct {
	Project
	// FacturaID is its valid factura (timbrada, or with a cancellation pending), 0 when
	// there is none; FolioFiscal is that factura's UUID, or the folio typed in before
	// facturación was in the app.
	FacturaID   int64
	FolioFiscal string
}

// billingFactura selects one column of the proyecto's newest valid factura, the one the
// project_states view counts for Facturación.
func billingFactura(col string) string {
	return `SELECT ` + col + ` FROM comprobantes bf
		WHERE bf.project_id = p.id AND bf.tipo = 'factura' AND bf.status IN ('timbrada', 'cancelacion_pendiente')
		ORDER BY bf.id DESC LIMIT 1`
}

// billingStateOrder puts Pendiente first, then Pendiente complemento de pago, then
// Facturada.
const billingStateOrder = `CASE ps.facturacion WHEN 'pendiente' THEN 0 WHEN 'pendiente_complemento' THEN 1 ELSE 2 END`

// billingSortColumns is the sortable-column whitelist for ListBilling. Días pendiente only
// counts while the factura is pending; the rest sort below every pending one.
var billingSortColumns = []sortColumn{
	{"folio", "p.folio"},
	{"estado", billingStateOrder},
	{"cliente", "c.name COLLATE NOCASE"},
	{"costo", "(q.subtotal - " + quoteProfitSQL + ")"},
	{"precio", "q.subtotal"},
	{"utilidad", quoteProfitSQL},
	{"margen", "CASE WHEN q.subtotal = 0 THEN 0 ELSE 1.0 * " + quoteProfitSQL + " / q.subtotal END"},
	{"vendedor", "u.name COLLATE NOCASE"},
	{"oc", "COALESCE(p.oc_number, '') COLLATE NOCASE"},
	{"fecha_oc", "COALESCE(p.oc_date, '')"},
	{"dias", "CASE WHEN ps.facturacion = 'pendiente' THEN julianday('now') - julianday(p.oc_date) ELSE -1 END"},
	{"folio_fiscal", "COALESCE((" + billingFactura("bf.uuid") + "), p.invoice_ref, '')"},
}

// billingDefaultOrder is the Facturas order when no column is chosen: by Estado de
// facturación (billingStateOrder), oldest O.C. first within each.
const billingDefaultOrder = `ORDER BY ` + billingStateOrder + `, COALESCE(p.oc_date, ''), p.id`

// billingFilterColumns are the filters of the Facturas table.
var billingFilterColumns = []filterColumn{
	{name: "estado", expr: "ps.facturacion", kind: FilterEnum},
	{name: "cliente", expr: "c.name", kind: FilterText},
	{name: "vendedor", expr: "u.name", kind: FilterText},
}

// ListBilling returns the proyectos with the client's O.C. received (en curso, Cerrado ·
// Ganado included) for the Facturas table, sorted per sort/dir (see billingSortColumns)
// or by billingDefaultOrder, and filtered per billingFilterColumns.
func (s *Store) ListBilling(ctx context.Context, sort, dir string, filters Filters) ([]BillingRow, error) {
	order := billingDefaultOrder
	for _, c := range billingSortColumns {
		if c.name == sort {
			order = orderByClause(billingSortColumns, sort, dir) + ", p.id"
			break
		}
	}
	extra, extraArgs := filterWhere(ctx, billingFilterColumns, filters)
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+projectSelectCols+`,
			COALESCE((`+billingFactura("bf.id")+`), 0),
			COALESCE((`+billingFactura("bf.uuid")+`), p.invoice_ref, '')`+projectFrom+`
		WHERE p.status = 'en_curso'`+extra+`
		`+order, extraArgs...)
	if err != nil {
		return nil, fmt.Errorf("store: list billing: %w", err)
	}
	defer rows.Close()
	var out []BillingRow
	for rows.Next() {
		var r BillingRow
		p, err := scanProject(scanTail{rows, []any{&r.FacturaID, &r.FolioFiscal}})
		if err != nil {
			return nil, fmt.Errorf("store: list billing: %w", err)
		}
		r.Project = *p
		out = append(out, r)
	}
	return out, rows.Err()
}

// scanTail scans a row whose leading columns are read by another scanner (scanProject)
// and its trailing ones into tail.
type scanTail struct {
	row  interface{ Scan(...any) error }
	tail []any
}

func (s scanTail) Scan(dest ...any) error { return s.row.Scan(append(dest, s.tail...)...) }
