package views

import (
	"strconv"
	"strings"

	"github.com/gerrygoo/cladex-web/internal/pricing"
	"github.com/gerrygoo/cladex-web/internal/store"
)

// pricingTotals is a comprobante's totals in the shape the lines tables' footer takes.
func pricingTotals(c store.Comprobante) pricing.Totals {
	return pricing.Totals{Subtotal: c.Subtotal, IVA: c.IVA, Total: c.Total}
}

// orDash is s, or "—" when it is empty.
func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// hasActiveFactura reports whether a proyecto already has a factura in the works or
// stamped (a cancelled one doesn't count).
func hasActiveFactura(list []store.Comprobante) bool {
	for _, c := range list {
		if c.Tipo == store.TipoFactura && c.RelatedID == 0 && c.Status != store.CompCancelada {
			return true
		}
	}
	return false
}

// Billing is the Facturas page's data (#63): the proyectos with the client's O.C.
// received and the viewer's today.
type Billing struct {
	Rows  []store.BillingRow
	Today string
}

// Totals adds up the listed proyectos before IVA, as Forecast.Totals does.
func (b Billing) Totals() ForecastTotals {
	var price, profit int64
	for _, r := range b.Rows {
		price += int64(r.Subtotal)
		profit += int64(r.Profit)
	}
	return totalsOf(price, profit)
}

// DaysPendingFactura is the whole days since the client's O.C. while the proyecto's
// factura is still pending, "" once it is issued (or without an O.C. date).
func DaysPendingFactura(p store.Project, today string) string {
	if p.States.Facturacion != store.FacturacionPendiente {
		return ""
	}
	if d, ok := daysBetween(p.OC.Date, today); ok {
		return strconv.Itoa(max(d, 0))
	}
	return ""
}

// billingStateFilter is the Estado de facturación filter of the Facturas table.
var billingStateFilter = enumFilter(
	FilterOption{store.FacturacionPendiente, store.FacturacionLabels[store.FacturacionPendiente]},
	FilterOption{store.FacturacionPendienteComplemento, store.FacturacionLabels[store.FacturacionPendienteComplemento]},
	FilterOption{store.FacturacionFacturada, store.FacturacionLabels[store.FacturacionFacturada]})

// complementoWarnDays is how close to its deadline a missing complemento is flagged.
const complementoWarnDays = 5

// ComplementoWarning flags a proyecto Pendiente complemento de pago whose oldest payment
// without a complemento is near (complementoWarnDays) or past the SAT's deadline, the 5th
// of the next month: "Complemento vence el 05/12/2026" or "Complemento vencido (05/12/2026)".
// "" otherwise.
func ComplementoWarning(r store.BillingRow, today string) string {
	if r.States.Facturacion != store.FacturacionPendienteComplemento || r.UncoveredPaymentSince == "" {
		return ""
	}
	deadline := store.ComplementoDeadline(r.UncoveredPaymentSince)
	d, ok := daysBetween(today, deadline)
	switch {
	case !ok:
		return ""
	case d < 0:
		return "Complemento vencido (" + dateText(deadline) + ")"
	case d <= complementoWarnDays:
		return "Complemento vence el " + dateText(deadline)
	}
	return ""
}

// MissingCustomerData is what a factura draft's receptor still lacks to be invoiced (#63
// point 3): RFC, razón social, código postal and régimen fiscal, by the names the
// customer form uses. They stay optional on the customer until a factura needs them.
func MissingCustomerData(c store.Comprobante) []string {
	cu := c.Customer
	var out []string
	if !store.ValidRFC(cu.RFC) {
		out = append(out, "RFC")
	}
	if strings.TrimSpace(cu.LegalName) == "" {
		out = append(out, "razón social")
	}
	if !store.ValidPostalCode(cu.PostalCode) {
		out = append(out, "código postal fiscal")
	}
	if cu.TaxRegime == "" {
		out = append(out, "régimen fiscal")
	}
	return out
}

// MissingKeyLines are a factura draft's catalog lines without a clave de producto or de
// unidad SAT (#63 point 4), each fixed on its product. Free lines (QL) take theirs on
// the draft itself.
func MissingKeyLines(c store.Comprobante) []store.ComprobanteConcepto {
	var out []store.ComprobanteConcepto
	for _, l := range c.Conceptos {
		if l.ProductID != 0 && (l.SATProductCode == "" || l.SATUnitCode == "") {
			out = append(out, l)
		}
	}
	return out
}

// cfdiUseOptions are the usos de CFDI the draft offers: those valid for the customer's
// régimen, plus the one already chosen if it isn't (so validation can say why).
func cfdiUseOptions(c store.Comprobante) []store.SATCode {
	opts := store.CFDIUsesFor(c.Customer.TaxRegime)
	for _, o := range opts {
		if o.Code == c.UsoCFDI {
			return opts
		}
	}
	for _, u := range store.CFDIUses {
		if u.Code == c.UsoCFDI {
			return append(opts, u)
		}
	}
	return opts
}

// returnToFactura is the query that brings an edit page back to factura id on saving.
func returnToFactura(id int64) string { return "?factura=" + strconv.FormatInt(id, 10) }
