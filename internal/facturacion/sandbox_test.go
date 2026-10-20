package facturacion

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gerrygoo/cladex-web/internal/facturoporti"
	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/store"
)

// testFactura is a two-line P.P.D. factura from the SAT's test emisor to a test receptor,
// with amounts as Cladex's quotes make them (unit prices to the micro).
func testFactura() *store.Comprobante {
	c := &store.Comprobante{Tipo: store.TipoFactura, UsoCFDI: "G01", MetodoPago: "PPD", FormaPago: "99", Moneda: "MXN",
		Customer: store.Customer{RFC: "IIA040805DZ4", LegalName: "INDISTRIA ILUMINADORA DE ALMACENES", TaxRegime: "601", PostalCode: "62661"},
		Conceptos: []store.ComprobanteConcepto{
			{LineNo: 1, SATProductCode: "26121600", SATUnitCode: "MTR", UnitName: "Metro", Description: "Cable CCS 30% ALAMBRE 4 (4 AWG)",
				QtyMilli: 1_250_000, UnitPriceMicros: 39_129_876, Importe: 4_891_235},
			{LineNo: 2, SATProductCode: "39111500", SATUnitCode: "H87", UnitName: "Pieza", Description: "Luminaria LED 100 W",
				QtyMilli: 3_000, UnitPriceMicros: 1_234_567_891, Importe: 370_370},
		}}
	for i := range c.Conceptos {
		c.Conceptos[i].IVA = money.Centavos(money.RoundHalfUp(int64(c.Conceptos[i].Importe)*16, 100))
		c.Subtotal += c.Conceptos[i].Importe
		c.IVA += c.Conceptos[i].IVA
	}
	c.Total = c.Subtotal + c.IVA
	return c
}

var testEmisor = store.Emisor{RFC: "EKU9003173C9", Name: "ESCUELA KEMPER URGATE", TaxRegime: "601", PostalCode: "42501", SerieFactura: "TF", SeriePago: "TP"}

// TestSandboxFacturaYPago stamps a P.P.D. factura and its comprobante de pago against the
// real sandbox, when FACTUROPORTI_SANDBOX=1: the check that BuildRequest's requests pass
// the SAT's rules.
func TestSandboxFacturaYPago(t *testing.T) {
	if os.Getenv("FACTUROPORTI_SANDBOX") != "1" {
		t.Skip("set FACTUROPORTI_SANDBOX=1 to run against testapi.facturoporti.com.mx")
	}
	resp, err := http.Get(facturoporti.SandboxURL + "/token/crear?Usuario=PruebasTimbrado&Password=" + url.QueryEscape("@Notiene1"))
	if err != nil {
		t.Fatal(err)
	}
	var tok struct{ Token string }
	json.NewDecoder(resp.Body).Decode(&tok)
	resp.Body.Close()
	cer, _ := os.ReadFile("../facturoporti/testdata/eku9003173c9.cer")
	key, _ := os.ReadFile("../facturoporti/testdata/eku9003173c9.key")
	csd, err := facturoporti.ParseCSD(cer, key, "12345678a")
	if err != nil {
		t.Fatal(err)
	}
	client := &facturoporti.Client{BaseURL: facturoporti.SandboxURL, Sandbox: true, Token: tok.Token, CSD: csd, HTTP: &http.Client{Timeout: 90 * time.Second}}
	ctx := context.Background()
	folio := time.Now().Unix() % 1_000_000

	f := testFactura()
	body, _ := facturoporti.Marshal(BuildRequest(f, nil, testEmisor, "TF", folio, time.Now()))
	r := client.Stamp(ctx, body)
	if r.Outcome != facturoporti.Done {
		t.Fatalf("factura: %+v\n%s", r, body)
	}
	t.Logf("factura %s", r.UUID)
	if !strings.Contains(r.XML, `Total="`+facturoporti.Dec(int64(f.Total), 2).String()+`"`) {
		t.Errorf("stamped total differs from ours (%s): %s", f.Total, r.XML)
	}
	f.Status, f.UUID, f.Serie, f.Folio = store.CompTimbrada, r.UUID, "TF", folio

	p := &store.Comprobante{Tipo: store.TipoPago, Moneda: "MXN", FormaPago: "03", PagoFecha: time.Now().AddDate(0, 0, -1).Format("2006-01-02"), Customer: f.Customer}
	body, _ = facturoporti.Marshal(BuildRequest(p, f, testEmisor, "TP", folio, time.Now()))
	r = client.Stamp(ctx, body)
	if r.Outcome != facturoporti.Done {
		t.Fatalf("pago: %+v\n%s", r, body)
	}
	t.Logf("pago %s", r.UUID)

	// Replace the factura (relación 04), then cancel the original with motivo 01.
	rep := testFactura()
	body, _ = facturoporti.Marshal(BuildRequest(rep, f, testEmisor, "TF", folio+1, time.Now()))
	if !strings.Contains(string(body), `"TipoRelacion":"04"`) {
		t.Fatalf("replacement has no relación: %s", body)
	}
	r = client.Stamp(ctx, body)
	if r.Outcome != facturoporti.Done {
		t.Fatalf("replacement: %+v", r)
	}
	cr := client.Cancel(ctx, facturoporti.CancelRequest{RFCEmisor: testEmisor.RFC, RFCReceptor: f.Customer.RFC, UUID: f.UUID,
		Total: facturoporti.Dec(int64(f.Total), 2), Motivo: "01", Sustitucion: r.UUID})
	if cr.Outcome != facturoporti.Done && cr.Outcome != facturoporti.Pending {
		t.Fatalf("cancel 01: %+v", cr)
	}
	t.Logf("replacement %s; original %s", r.UUID, cr.Estado)
}
