package views

import (
	"strconv"

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
