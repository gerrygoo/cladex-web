package views

import (
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

// hasStampedFactura reports whether a proyecto's factura is a stamped CFDI, whose payment
// is then a comprobante de pago made from the factura's page, not a folio typed in.
func hasStampedFactura(list []store.Comprobante) bool {
	for _, c := range list {
		if c.Tipo == store.TipoFactura && c.UUID != "" {
			return true
		}
	}
	return false
}
