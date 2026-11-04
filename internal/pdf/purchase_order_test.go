package pdf

import (
	"context"
	"strings"
	"testing"
	"time"
)

func samplePurchaseOrder() PurchaseOrderDocument {
	return PurchaseOrderDocument{
		Number:          "OC-QA0037-301026",
		Fecha:           "30/10/2026",
		Vendedor:        "Gerardo Galván",
		Issuer:          "CLADEX S.A. de C.V. · RFC CLA260326Q19",
		SupplierName:    "CABLE ANTI ROBO",
		SupplierAddress: "Camino Real de Carretas 356, Milenio III, C.P. 76060, Querétaro, Querétaro",
		BillingName:     "CLADEX S.A. de C.V.",
		BillingRFC:      "CLA260326Q19",
		BillingRegime:   "601 - General de Ley Personas Morales",
		BillingAddress:  "Calle Circuito Valle del Silencio 143, Col. Lomas de Valle Escondido, C.P. 52930, Atizapán de Zaragoza, Estado de México",
		BillingCFDIUse:  "G01 - Adquisición de mercancías",
		DeliveryAddress: "Av. Siempre Viva 742",
		Lines: []PurchaseOrderLineDoc{
			{Description: "CABLE THW/THHW CONDUCCASA CALIBRE 14 AWG verde", Qty: "200", UnitPrice: "$5.54", Total: "$1,108.00"},
		},
		Subtotal:   "$1,108.00",
		IVA:        "$177.28",
		Total:      "$1,285.28",
		Conditions: []string{"Precios en pesos mexicanos (MXN), más IVA."},
	}
}

func TestRenderPurchaseOrder(t *testing.T) {
	doc := samplePurchaseOrder()
	src := buildPurchaseOrderTypst(doc)
	for _, want := range []string{
		`#let number = "OC-QA0037-301026"`,
		`#let billingRegime = "601 - General de Ley Personas Morales"`,
		`("CABLE THW/THHW CONDUCCASA CALIBRE 14 AWG verde", "200", "$5.54", "$1,108.00",)`,
		"head[Precio unitario]",
		"#let letterhead = image(logo",
		"ORDEN DE COMPRA", "PROVEEDOR", "DATOS DE FACTURACIÓN", "DIRECCIÓN DE ENTREGA", "CONDICIONES",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("source lacks %q", want)
		}
	}
	if strings.Contains(src, "Precio por kg") || strings.Contains(src, "rotate(") {
		t.Error("a per-unit O.C. without a watermark has no kg columns and no watermark")
	}
	out, err := RenderPurchaseOrder(context.Background(), doc)
	if err != nil {
		t.Fatalf("RenderPurchaseOrder: %v", err)
	}
	if !strings.HasPrefix(string(out), "%PDF") {
		t.Fatal("output isn't a PDF")
	}

	// Pinned to a creation date, a confirmed O.C. reprints byte for byte.
	doc.Created = time.Date(2026, 10, 30, 18, 0, 0, 0, time.UTC)
	a, err := RenderPurchaseOrder(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	b, err := RenderPurchaseOrder(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Error("two renders of the same confirmed O.C. differ")
	}
}

func TestRenderPurchaseOrderKgAndDraft(t *testing.T) {
	doc := samplePurchaseOrder()
	doc.ShowKg = true
	doc.Watermark = "BORRADOR"
	doc.SupplierName = `Proveedor "raro" #read("x") *b* [c] \ $m$`
	doc.Lines = []PurchaseOrderLineDoc{{
		Description: "ALC-C7#8 Alumoclad 7#8", Qty: "111", KgPerUnit: "0.3896", Kg: "43.25",
		PricePerKg: "$95.00", UnitPrice: "$37.01", Total: "$4,108.11",
	}}
	src := buildPurchaseOrderTypst(doc)
	for _, want := range []string{
		`("ALC-C7#8 Alumoclad 7#8", "111", "0.3896", "43.25", "$95.00", "$37.01", "$4,108.11",)`,
		"head[Cantidad (m)]", "head[Kg por metro]", "head[Kg]", "head[Precio por kg]",
		`[#"BORRADOR"]`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("source lacks %q", want)
		}
	}
	if _, err := RenderPurchaseOrder(context.Background(), doc); err != nil {
		t.Fatalf("RenderPurchaseOrder: %v", err)
	}
}
