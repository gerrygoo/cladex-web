package facturacion

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gerrygoo/cladex-web/internal/pdf"
	"github.com/gerrygoo/cladex-web/internal/store"
)

func TestEnLetra(t *testing.T) {
	for in, want := range map[string]string{
		"116.00":     "CIENTO DIECISÉIS PESOS 00/100 M.N.",
		"1.50":       "UN PESO 50/100 M.N.",
		"56738.33":   "CINCUENTA Y SEIS MIL SETECIENTOS TREINTA Y OCHO PESOS 33/100 M.N.",
		"1000000.00": "UN MILLÓN DE PESOS 00/100 M.N.",
		"2021100.1":  "DOS MILLONES VEINTIÚN MIL CIEN PESOS 10/100 M.N.",
		"100":        "CIEN PESOS 00/100 M.N.",
	} {
		if got := enLetra(in, "MXN"); got != want {
			t.Errorf("enLetra(%s) = %q, want %q", in, got, want)
		}
	}
	if got := enLetra("21.00", "USD"); got != "VEINTIÚN DÓLARES 00/100 USD" {
		t.Errorf("USD = %q", got)
	}
	if amount("48912.35") != "$48,912.35" || amount("116.00") != "$116.00" || amount("1250.000") != "$1,250.000" {
		t.Error("amount formatting")
	}
}

func TestPrintedSandboxFactura(t *testing.T) {
	b, err := os.ReadFile("../cfdi/testdata/sandbox-factura.xml")
	if err != nil {
		t.Fatal(err)
	}
	c := &store.Comprobante{XML: string(b), Sandbox: true, StampedAt: "2026-10-20T22:37:15.000Z"}
	doc, err := Printed(c)
	if err != nil {
		t.Fatalf("Printed: %v", err)
	}
	if doc.SerieFolio != "CLDX-1792535835" || doc.UUID != "679E4179-5AF4-4D67-994D-569A29497250" || len(doc.Conceptos) != 2 ||
		doc.Totals[2][1] != "$116.00" || !strings.Contains(doc.Details[0][1], "PUE") || doc.Details[1][1] != "01 - Efectivo" ||
		doc.TotalLetra != "CIENTO DIECISÉIS PESOS 00/100 M.N." || !doc.Sandbox {
		t.Errorf("Printed = %+v", doc)
	}
	a, err := pdf.RenderCFDI(context.Background(), doc)
	if err != nil {
		t.Fatalf("RenderCFDI: %v", err)
	}
	b2, _ := pdf.RenderCFDI(context.Background(), doc)
	if !strings.HasPrefix(string(a), "%PDF") || string(a) != string(b2) {
		t.Error("the printed PDF is not a deterministic PDF")
	}
	if os.Getenv("PREVIEW_OUT") != "" {
		os.WriteFile(os.Getenv("PREVIEW_OUT"), a, 0o644)
	}
}
