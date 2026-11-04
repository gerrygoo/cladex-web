package pdf

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// PurchaseOrderLineDoc is one partida of an orden de compra as printed, already
// display-formatted by the caller. KgPerUnit, Kg and PricePerKg are printed only when
// the document's ShowKg is set.
type PurchaseOrderLineDoc struct {
	Description string `json:"description"`
	Qty         string `json:"qty"`
	KgPerUnit   string `json:"kg_per_unit,omitempty"`
	Kg          string `json:"kg,omitempty"`
	PricePerKg  string `json:"price_per_kg,omitempty"`
	UnitPrice   string `json:"unit_price"`
	Total       string `json:"total"`
}

// PurchaseOrderDocument is everything RenderPurchaseOrder lays out: an orden de compra
// to a supplier (issue #65), in the format of Cladex's "Orden de Compra" workbook. Every
// field is display-ready. A confirmed O.C. stores this document as JSON and reprints
// from it, so the tags are part of what is stored; Watermark is not.
type PurchaseOrderDocument struct {
	Number   string `json:"number"`
	Fecha    string `json:"fecha"`
	Vendedor string `json:"vendedor"`
	// Issuer is the line under the title: "CLADEX S.A. de C.V. · RFC CLA260326Q19".
	Issuer          string `json:"issuer"`
	SupplierName    string `json:"supplier_name"`
	SupplierAddress string `json:"supplier_address"`
	// The Datos de facturación block, Cladex's own.
	BillingName     string                 `json:"billing_name"`
	BillingRFC      string                 `json:"billing_rfc"`
	BillingRegime   string                 `json:"billing_regime"`
	BillingAddress  string                 `json:"billing_address"`
	BillingCFDIUse  string                 `json:"billing_cfdi_use"`
	DeliveryAddress string                 `json:"delivery_address"`
	DeliveryRefs    string                 `json:"delivery_references"`
	ShowKg          bool                   `json:"show_kg"`
	Lines           []PurchaseOrderLineDoc `json:"lines"`
	Subtotal        string                 `json:"subtotal"`
	IVA             string                 `json:"iva"`
	Total           string                 `json:"total"`
	Conditions      []string               `json:"conditions"`
	// Watermark is printed large and faint across the page: "BORRADOR" on a preview, the
	// instance's name on a test instance. Never stored.
	Watermark string `json:"-"`
	// Created is the PDF's embedded creation date: the confirmation time for a confirmed
	// O.C., so every reprint is byte-identical; zero (render time) for a preview.
	Created time.Time `json:"created"`
}

// RenderPurchaseOrder renders doc to PDF bytes.
func RenderPurchaseOrder(ctx context.Context, doc PurchaseOrderDocument) ([]byte, error) {
	return Render(ctx, buildPurchaseOrderTypst(doc), doc.Created)
}

// buildPurchaseOrderTypst generates the Typst source for doc. As in buildQuoteTypst,
// every value is bound with #let to an escaped string literal, so nothing typed into a
// supplier, an address or a description can break out of its cell.
func buildPurchaseOrderTypst(doc PurchaseOrderDocument) string {
	var b strings.Builder
	b.WriteString("#set page(paper: \"us-letter\", margin: 1.6cm)\n")
	if doc.Watermark != "" {
		fmt.Fprintf(&b, "#set page(background: rotate(-35deg, text(size: 96pt, weight: \"bold\", fill: rgb(200, 0, 0, 40))[#%s]))\n",
			typstStringLiteral(doc.Watermark))
	}
	b.WriteString("#set text(size: 9pt)\n")
	writeLetterhead(&b, LetterheadCladex)

	vars := []struct{ name, value string }{
		{"number", doc.Number}, {"fecha", doc.Fecha}, {"vendedor", doc.Vendedor},
		{"issuer", doc.Issuer}, {"supplierName", doc.SupplierName}, {"supplierAddress", doc.SupplierAddress},
		{"billingName", doc.BillingName}, {"billingRFC", doc.BillingRFC}, {"billingRegime", doc.BillingRegime},
		{"billingAddress", doc.BillingAddress}, {"billingUse", doc.BillingCFDIUse},
		{"deliveryAddress", doc.DeliveryAddress}, {"deliveryRefs", doc.DeliveryRefs},
		{"subtotal", doc.Subtotal}, {"iva", doc.IVA}, {"total", doc.Total},
	}
	for _, v := range vars {
		fmt.Fprintf(&b, "#let %s = %s\n", v.name, typstStringLiteral(v.value))
	}

	b.WriteString("#let lines = (\n")
	for _, l := range doc.Lines {
		cells := []string{l.Description, l.Qty, l.UnitPrice, l.Total}
		if doc.ShowKg {
			cells = []string{l.Description, l.Qty, l.KgPerUnit, l.Kg, l.PricePerKg, l.UnitPrice, l.Total}
		}
		for i, c := range cells {
			cells[i] = typstStringLiteral(c)
		}
		fmt.Fprintf(&b, "  (%s,),\n", strings.Join(cells, ", "))
	}
	b.WriteString(")\n")

	b.WriteString("#let conditions = (\n")
	for _, c := range doc.Conditions {
		fmt.Fprintf(&b, "  %s,\n", typstStringLiteral(c))
	}
	b.WriteString(")\n")

	table := purchaseOrderTable
	if doc.ShowKg {
		table = purchaseOrderKgTable
	}
	b.WriteString(strings.Replace(purchaseOrderLayout, "{{table}}", table, 1))
	return b.String()
}

const purchaseOrderTable = `#table(
  columns: (auto, 1fr, auto, auto, auto),
  align: (center, left, center, right, right),
  stroke: 0.5pt + rgb("#cccccc"),
  fill: (col, row) => if row == 0 { accent } else if col == 0 { white } else { cream },
  table.header(
    head[Partida], head[Descripción], head[Cantidad], head[Precio unitario], head[Total],
  ),
  ..lines.enumerate().map(((i, l)) => (str(i + 1), ..l)).flatten()
)`

const purchaseOrderKgTable = `#table(
  columns: (auto, 1fr, auto, auto, auto, auto, auto, auto),
  align: (center, left, center, center, center, right, right, right),
  stroke: 0.5pt + rgb("#cccccc"),
  fill: (col, row) => if row == 0 { accent } else if col == 0 { white } else { cream },
  table.header(
    head[Partida], head[Descripción], head[Cantidad (m)], head[Kg por metro], head[Kg],
    head[Precio por kg], head[Precio unitario], head[Total],
  ),
  ..lines.enumerate().map(((i, l)) => (str(i + 1), ..l)).flatten()
)`

const purchaseOrderLayout = `
#let accent = rgb("#AC5424")
#let cream = rgb("#FDF5E6")
#let muted = rgb("#555555")
#let head(body) = text(fill: white, weight: "bold", body)
#let bar(title) = block(width: 100%, fill: accent, inset: (x: 6pt, y: 4pt), above: 12pt, below: 6pt,
  text(fill: white, weight: "bold", title))
#let fieldname(body) = text(size: 7.5pt, weight: "bold", fill: muted, body)
#let boxed(body) = box(width: 100%, fill: cream, stroke: 0.5pt + rgb("#d9c9a8"), inset: 3pt, body)
#grid(
  columns: (auto, 1fr),
  align: (left + horizon, right + horizon),
  letterhead,
  [#text(size: 15pt, weight: "bold")[ORDEN DE COMPRA] \ #text(size: 8pt, fill: muted)[#issuer]],
)
#v(4pt)
#line(length: 100%, stroke: 1pt + accent)
#v(6pt)
#grid(
  columns: (auto, 1fr, auto, 1fr),
  column-gutter: 8pt,
  row-gutter: 4pt,
  align: horizon,
  [*No. de OC:*], boxed(number), [*Fecha:*], boxed(fecha),
  [], [], [*Vendedor:*], boxed(vendedor),
)
#bar[PROVEEDOR]
#grid(
  columns: (2.8cm, 1fr),
  row-gutter: 5pt,
  fieldname[Empresa], supplierName,
  fieldname[Domicilio], supplierAddress,
)
#bar[DATOS DE FACTURACIÓN]
#grid(
  columns: (2.8cm, 1fr),
  row-gutter: 5pt,
  fieldname[Razón social], billingName,
  fieldname[RFC], billingRFC,
  fieldname[Régimen fiscal], billingRegime,
  fieldname[Domicilio fiscal], billingAddress,
  fieldname[Uso de CFDI], billingUse,
)
#bar[DIRECCIÓN DE ENTREGA]
#grid(
  columns: (2.8cm, 1fr),
  row-gutter: 5pt,
  fieldname[Domicilio], deliveryAddress,
  fieldname[Referencias], deliveryRefs,
)
#v(10pt)
{{table}}
#v(6pt)
#align(right)[
  #grid(
    columns: (auto, auto, auto),
    column-gutter: 14pt,
    row-gutter: 5pt,
    align: (right, right, right),
    [], [Subtotal], subtotal,
    text(size: 7.5pt, fill: muted)[16%], [IVA], iva,
    [], [*Total*], [*#total*],
  )
]
#bar[CONDICIONES]
#for c in conditions [
  - #c
]
`
