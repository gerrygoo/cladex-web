package store

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/gerrygoo/cladex-web/internal/money"
)

// LineKgPerMetre is a quote line's weight per metre as it was priced, from its
// pricing_inputs: the kg per unit a QG line is costed with plus every material measured
// in kg (the copper of a QS line). nil when the line has no weight.
func LineKgPerMetre(inputs *string) *money.Micros {
	if inputs == nil {
		return nil
	}
	var snap struct {
		KgPerUnit string `json:"kg_per_unit"`
		Materials []struct {
			QtyPerUnit string `json:"qty_per_unit"`
			Unit       string `json:"unit"`
		} `json:"materials"`
	}
	if err := json.Unmarshal([]byte(*inputs), &snap); err != nil {
		return nil
	}
	var kg money.Micros
	found := false
	if snap.KgPerUnit != "" {
		q, err := money.ParseMicros(snap.KgPerUnit)
		if err != nil {
			return nil
		}
		kg, found = q, true
	}
	for _, m := range snap.Materials {
		if m.Unit != "kg" {
			continue
		}
		q, err := money.ParseMicros(m.QtyPerUnit)
		if err != nil {
			return nil
		}
		kg += q
		found = true
	}
	if !found {
		return nil
	}
	return &kg
}

// KilosOf is qty metres of a line weighing kgPerMetre, in kilos to the hundredth (as the
// quote shows them), as Milli.
func KilosOf(qty money.Milli, kgPerMetre money.Micros) money.Milli {
	// Milli (1e-3) × Micros (1e-6) is 1e-9; hundredths are 1e-2, then ×10 is Milli.
	return money.Milli(money.RoundHalfUp(int64(qty)*int64(kgPerMetre), 10_000_000) * 10)
}

// UnitPriceFor is the unit price that makes qty units add up to importe: importe ÷ qty,
// to the micro, rounded half up. The importe stays the quote's; only the price adapts
// (#63: rounding never changes a line's total).
func UnitPriceFor(importe money.Centavos, qty money.Milli) money.Micros {
	if qty <= 0 {
		return 0
	}
	// Centavos (1e-2) ÷ Milli (1e-3) → Micros (1e-6): ×1e4 to micros, ×1e3 for the milli.
	return money.Micros(money.RoundHalfUp(int64(importe)*10_000_000, int64(qty)))
}

// invoiceInKilos turns a draft's conceptos invoiced in KGM, on a quote quoted per metre
// by weight (QG, QS), into kilos (#63 point 5): the quantity is the kilos the quote
// computed (metres × kg per metre, to the hundredth) and the unit price the line's
// importe over them, so the importe is the quote's to the centavo. A line without a
// weight can't be turned into kilos and stays in metres (MTR). A line already in kilos
// (its quantity no longer the quote's metres) is left alone.
func invoiceInKilos(ctx context.Context, tx *sql.Tx, id int64) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT c.id, c.qty_milli, c.importe, l.pricing_inputs
		FROM comprobante_conceptos c JOIN quote_lines l ON l.id = c.quote_line_id
		WHERE c.comprobante_id = ? AND c.sat_unit_code = 'KGM' AND c.qty_milli = l.qty_milli`, id)
	if err != nil {
		return err
	}
	type line struct {
		id      int64
		qty     money.Milli
		importe money.Centavos
		inputs  *string
	}
	var lines []line
	for rows.Next() {
		var l line
		if err := rows.Scan(&l.id, &l.qty, &l.importe, &l.inputs); err != nil {
			rows.Close()
			return err
		}
		lines = append(lines, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, l := range lines {
		kgm := LineKgPerMetre(l.inputs)
		var kilos money.Milli
		if kgm != nil {
			kilos = KilosOf(l.qty, *kgm)
		}
		if kilos <= 0 {
			if _, err := tx.ExecContext(ctx, `UPDATE comprobante_conceptos SET sat_unit_code = 'MTR', unit_name = 'Metro' WHERE id = ?`, l.id); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE comprobante_conceptos SET qty_milli = ?, unit_price_micros = ?, unit_name = 'Kilogramo' WHERE id = ?`,
			kilos, UnitPriceFor(l.importe, kilos), l.id); err != nil {
			return err
		}
	}
	return nil
}

// RefreshDraftSATCodes fills a factura draft's conceptos that still lack a clave de
// producto or de unidad SAT with their product's own "Datos para facturar", after an
// admin completed them from the draft (#63 point 4), and turns the new KGM lines of a
// quote priced by weight into kilos. Codes already on a concepto (edited on the draft)
// are kept. Anything but a borrador is left alone.
func (s *Store) RefreshDraftSATCodes(ctx context.Context, id int64) error {
	return s.inTx(ctx, "refresh draft SAT codes", func(tx *sql.Tx) error {
		var byWeight bool
		err := tx.QueryRowContext(ctx, `
			SELECT COALESCE(q.kg_priced_snapshot, t.kg_priced) OR COALESCE(q.copper_price_margin_snapshot, t.copper_price_margin)
			FROM comprobantes c JOIN quotes q ON q.id = c.quote_id JOIN quote_types t ON t.id = q.quote_type_id
			WHERE c.id = ? AND c.status = 'borrador' AND c.tipo = 'factura'`, id).Scan(&byWeight)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE comprobante_conceptos SET sat_product_code = (
				SELECT pr.sat_product_code FROM quote_lines l JOIN products pr ON pr.id = l.product_id WHERE l.id = comprobante_conceptos.quote_line_id)
			WHERE comprobante_id = ? AND COALESCE(sat_product_code, '') = ''`, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE comprobante_conceptos SET unit_name = NULL, sat_unit_code = (
				SELECT pr.sat_unit_code FROM quote_lines l JOIN products pr ON pr.id = l.product_id WHERE l.id = comprobante_conceptos.quote_line_id)
			WHERE comprobante_id = ? AND COALESCE(sat_unit_code, '') = ''`, id); err != nil {
			return err
		}
		if err := nameSATUnits(ctx, tx, id); err != nil {
			return err
		}
		if byWeight {
			return invoiceInKilos(ctx, tx, id)
		}
		return nil
	})
}
