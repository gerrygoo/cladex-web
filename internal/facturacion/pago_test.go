package facturacion

import (
	"testing"

	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/store"
)

// #83: a complemento pays the whole factura with its own subtotal and IVA, or part of it
// with the subtotal in proportion and 16% of that.
func TestPagoAmounts(t *testing.T) {
	f := &store.Comprobante{Subtotal: 5_384_706, IVA: 861_553, Total: 6_246_259} // $62,462.59
	for _, tc := range []struct {
		name          string
		pago          store.Comprobante
		pagado, saldo money.Centavos
		parcialidad   int
		base, iva     money.Centavos
	}{
		{"before #83: the whole factura", store.Comprobante{}, 6_246_259, 6_246_259, 1, 5_384_706, 861_553},
		{"whole factura", store.Comprobante{Total: 6_246_259, Parcialidad: 1, SaldoAnterior: 6_246_259}, 6_246_259, 6_246_259, 1, 5_384_706, 861_553},
		{"first of two", store.Comprobante{Total: 3_100_000, Parcialidad: 1, SaldoAnterior: 6_246_259}, 3_100_000, 6_246_259, 1, 2_672_414, 427_586},
		{"second of two", store.Comprobante{Total: 3_146_259, Parcialidad: 2, SaldoAnterior: 3_146_259}, 3_146_259, 3_146_259, 2, 2_712_292, 433_967},
	} {
		pagado, saldo, n, base, iva := PagoAmounts(&tc.pago, f)
		if pagado != tc.pagado || saldo != tc.saldo || n != tc.parcialidad || base != tc.base || iva != tc.iva {
			t.Errorf("%s: got %d %d #%d base %d iva %d", tc.name, pagado, saldo, n, base, iva)
		}
	}
}
