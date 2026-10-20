package cfdi

import (
	"os"
	"strings"
	"testing"
)

// testdata/sandbox-factura.xml is a factura stamped by Facturoporti's sandbox on
// 2026-10-20 (test RFCs, no fiscal value).
func TestParseSandboxFactura(t *testing.T) {
	b, err := os.ReadFile("testdata/sandbox-factura.xml")
	if err != nil {
		t.Fatal(err)
	}
	c, err := Parse(string(b))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if c.UUID() != "679E4179-5AF4-4D67-994D-569A29497250" || c.Total != "116.00" || c.Emisor.Rfc != "EKU9003173C9" ||
		c.Receptor.UsoCFDI != "G03" || len(c.Conceptos) != 2 || c.Conceptos[0].Traslados[0].TasaOCuota != "0.160000" ||
		c.Impuestos.TotalImpuestosTrasladados != "16.00" || c.Timbre.NoCertificadoSAT != "30001000000500003456" {
		t.Errorf("Parse = %+v", c)
	}
	if co := c.CadenaOriginalTimbre(); !strings.HasPrefix(co, "||1.1|679e4179-5af4-4d67-994d-569a29497250|2026-10-20T16:37:15|SPR190613I52|") || !strings.HasSuffix(co, "|30001000000500003456||") {
		t.Errorf("CadenaOriginalTimbre = %s", co)
	}
	u := c.VerificationURL()
	if !strings.HasPrefix(u, "https://verificacfdi.facturaelectronica.sat.gob.mx/default.aspx?id=679E4179-5AF4-4D67-994D-569A29497250&re=EKU9003173C9&rr=SSF1103037F1&tt=116.00&fe=") {
		t.Errorf("VerificationURL = %s", u)
	}
	if _, err := Parse(`<cfdi:Comprobante xmlns:cfdi="http://www.sat.gob.mx/cfd/4"/>`); err == nil {
		t.Error("a CFDI without timbre parsed")
	}
}
