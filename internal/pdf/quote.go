package pdf

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"time"
)

//go:embed assets/cladex-logo.png
var cladexLogoPNG []byte

//go:embed assets/copperclad.png
var coppercladLogoPNG []byte

// Letterhead picks the logo block in the PDF's top-left corner. The zero value is the
// Cladex logo, so a document that doesn't set one renders as it always did.
type Letterhead int

const (
	LetterheadCladex     Letterhead = iota // the Cladex logo
	LetterheadCopperclad                   // the copperclad logo, a little larger (issue #16)
)

// QuoteLineDoc is one priced line as it should appear on the rendered PDF — already
// display-formatted (money as "$1,234.56", qty via money.Milli.String()) by the caller;
// this package does no money math of its own.
type QuoteLineDoc struct {
	Description string
	Qty         string
	UnitPrice   string
	Total       string
	// KgPerUnit and Kg are the line's weight per metre and in all, printed only when
	// the document's ShowKg is set; empty for a line without one.
	KgPerUnit, Kg string
}

// QuoteDocument is everything RenderQuote needs to lay out a cotización PDF. Every
// field is already display-formatted.
type QuoteDocument struct {
	Folio        string
	CustomerName string
	Vendedor     string
	Fecha        string
	Vencimiento  string // empty when not set — rendered as "—"
	// Currency (MXN or USD) names the currency the amounts are in, on the totals and the
	// unit price header; empty for a series that prices in pesos without saying so.
	Currency string
	Lines    []QuoteLineDoc
	Subtotal string
	IVA      string
	Total    string
	Terms    []string
	// Letterhead is the logo block the PDF opens with; the caller picks it by familia.
	Letterhead Letterhead
	// Watermark, when set, is printed large and faint across every page, so a PDF from a
	// test instance can't pass for a real quote. Empty in production, which keeps its
	// bytes as they were.
	Watermark string
	// ShowKg adds Kg por metro and Kg columns, with quantity and unit price per metre
	// (CCS quotes, issue #34).
	ShowKg bool
	// Created is the PDF's embedded creation date: issued_at for an issued quote, so
	// every reprint is byte-identical; zero (render time) for a draft preview.
	Created time.Time
}

// RenderQuote lays out a cotización PDF matching the legacy workbook's Cotizador
// sheets: letterhead, folio/cliente/fecha header, a line-item table (numbered by
// partida, issue #31), subtotal/IVA/
// total, and the family's terms block.
func RenderQuote(ctx context.Context, doc QuoteDocument) ([]byte, error) {
	return Render(ctx, buildQuoteTypst(doc), doc.Created)
}

// buildQuoteTypst generates the Typst source for doc. Every dynamic value (customer
// name, product descriptions, free-text lines — none of it under this app's control)
// is bound via #let to an escaped Typst string literal, then interpolated with #name;
// interpolating a str value inserts its literal text, never re-parsed as markup or
// code, so arbitrary content (including a stray "#", "*", "[" or backslash) can never
// break out of its cell or execute as Typst code. See typstStringLiteral.
func buildQuoteTypst(doc QuoteDocument) string {
	var b strings.Builder

	b.WriteString("#set page(paper: \"us-letter\", margin: 2cm)\n")
	if doc.Watermark != "" {
		fmt.Fprintf(&b, "#set page(background: rotate(-35deg, text(size: 96pt, weight: \"bold\", fill: rgb(200, 0, 0, 40))[#%s]))\n",
			typstStringLiteral(doc.Watermark))
	}
	b.WriteString("#set text(size: 10pt)\n")
	writeLetterhead(&b, doc.Letterhead)

	fmt.Fprintf(&b, "#let customerName = %s\n", typstStringLiteral(doc.CustomerName))
	fmt.Fprintf(&b, "#let folio = %s\n", typstStringLiteral(doc.Folio))
	fmt.Fprintf(&b, "#let fecha = %s\n", typstStringLiteral(doc.Fecha))
	fmt.Fprintf(&b, "#let vencimiento = %s\n", typstStringLiteral(orDash(doc.Vencimiento)))
	fmt.Fprintf(&b, "#let vendedor = %s\n", typstStringLiteral(doc.Vendedor))
	fmt.Fprintf(&b, "#let subtotal = %s\n", typstStringLiteral(doc.Subtotal))
	fmt.Fprintf(&b, "#let iva = %s\n", typstStringLiteral(doc.IVA))
	fmt.Fprintf(&b, "#let total = %s\n", typstStringLiteral(doc.Total))

	b.WriteString("#let lines = (\n")
	for _, l := range doc.Lines {
		if doc.ShowKg {
			fmt.Fprintf(&b, "  (%s, %s, %s, %s, %s, %s),\n",
				typstStringLiteral(l.Description), typstStringLiteral(l.Qty),
				typstStringLiteral(l.KgPerUnit), typstStringLiteral(l.Kg),
				typstStringLiteral(l.UnitPrice), typstStringLiteral(l.Total))
			continue
		}
		fmt.Fprintf(&b, "  (%s, %s, %s, %s),\n",
			typstStringLiteral(l.Description), typstStringLiteral(l.Qty),
			typstStringLiteral(l.UnitPrice), typstStringLiteral(l.Total))
	}
	b.WriteString(")\n")

	b.WriteString("#let terms = (\n")
	for _, t := range doc.Terms {
		fmt.Fprintf(&b, "  %s,\n", typstStringLiteral(t))
	}
	b.WriteString(")\n")

	layout := quoteLayout
	if doc.ShowKg {
		layout = strings.Replace(layout, quoteTable, kgQuoteTable, 1)
	}
	b.WriteString(currencyLabels(doc.Currency, layout))

	return b.String()
}

// quoteTable is the line-item table of quoteLayout; kgQuoteTable replaces it when the
// document shows weights.
const quoteTable = `#table(
  columns: (auto, 1fr, auto, auto, auto),
  align: (right, left, right, right, right),
  stroke: 0.5pt + rgb("#cccccc"),
  fill: (col, row) => if row == 0 { rgb("#AC5424") } else { white },
  table.header(
    [#text(fill: white, weight: "bold")[Partida]],
    [#text(fill: white, weight: "bold")[Descripción]],
    [#text(fill: white, weight: "bold")[Cantidad]],
    [#text(fill: white, weight: "bold")[Precio unitario]],
    [#text(fill: white, weight: "bold")[Total]],
  ),
  ..lines.enumerate().map(((i, l)) => (str(i + 1), l.at(0), l.at(1), l.at(2), l.at(3))).flatten()
)`

const kgQuoteTable = `#table(
  columns: (auto, 1fr, auto, auto, auto, auto, auto),
  align: (right, left, right, right, right, right, right),
  stroke: 0.5pt + rgb("#cccccc"),
  fill: (col, row) => if row == 0 { rgb("#AC5424") } else { white },
  table.header(
    [#text(fill: white, weight: "bold")[Partida]],
    [#text(fill: white, weight: "bold")[Descripción]],
    [#text(fill: white, weight: "bold")[Cantidad (m)]],
    [#text(fill: white, weight: "bold")[Kg por metro]],
    [#text(fill: white, weight: "bold")[Kg]],
    [#text(fill: white, weight: "bold")[Precio unitario]],
    [#text(fill: white, weight: "bold")[Total]],
  ),
  ..lines.enumerate().map(((i, l)) => (str(i + 1), l.at(0), l.at(1), l.at(2), l.at(3), l.at(4), l.at(5))).flatten()
)`

const quoteLayout = `
#grid(
  columns: (auto, 1fr),
  align: (left, right),
  letterhead,
  align(right)[
    #text(size: 8pt)[
      Camino Real de Carretas 356 \
      Milenio III \
      76060 Querétaro, QUE \
      México
    ]
  ],
)
#v(0.6cm)
#grid(
  columns: (1fr, 1fr),
  gutter: 1em,
  [*Cliente:* #customerName], [*Número de cotización:* #folio],
  [*Fecha de cotización:* #fecha], [*Vencimiento:* #vencimiento],
  [*Vendedor:* #vendedor], [],
)
#v(0.5cm)
#table(
  columns: (auto, 1fr, auto, auto, auto),
  align: (right, left, right, right, right),
  stroke: 0.5pt + rgb("#cccccc"),
  fill: (col, row) => if row == 0 { rgb("#AC5424") } else { white },
  table.header(
    [#text(fill: white, weight: "bold")[Partida]],
    [#text(fill: white, weight: "bold")[Descripción]],
    [#text(fill: white, weight: "bold")[Cantidad]],
    [#text(fill: white, weight: "bold")[Precio unitario]],
    [#text(fill: white, weight: "bold")[Total]],
  ),
  ..lines.enumerate().map(((i, l)) => (str(i + 1), l.at(0), l.at(1), l.at(2), l.at(3))).flatten()
)
#v(0.4cm)
#align(right)[
  #grid(
    columns: (auto, auto),
    gutter: 0.6em,
    [Subtotal], [#subtotal],
    [IVA], [#iva],
    [*Total*], [*#total*],
  )
]
#v(0.8cm)
== Términos y condiciones
#for t in terms [
  - #t
]
`

// writeLetterhead binds #letterhead to l's logo block. Only the logos it shows are
// embedded, and the Cladex block lays out exactly as before letterheads existed, so
// quotes that keep it reprint byte-for-byte.
func writeLetterhead(b *strings.Builder, l Letterhead) {
	switch l {
	case LetterheadCopperclad:
		fmt.Fprintf(b, "#let copperclad = bytes((%s))\n", pngBytesLiteral(coppercladLogoPNG))
		b.WriteString("#let letterhead = image(copperclad, width: 4.5cm)\n")
	default:
		fmt.Fprintf(b, "#let logo = bytes((%s))\n", pngBytesLiteral(cladexLogoPNG))
		b.WriteString("#let letterhead = image(logo, width: 3.2cm)\n")
	}
}

// currencyLabels names the currency on the amount labels of the static layout. With no
// currency the layout is returned untouched, so quotes that don't carry one render
// byte-for-byte as they always did.
func currencyLabels(currency, layout string) string {
	if currency == "" {
		return layout
	}
	return strings.NewReplacer(
		"[Precio unitario]", "[Precio unitario ("+currency+")]",
		"[Subtotal]", "[Subtotal ("+currency+")]",
		"[IVA]", "[IVA ("+currency+")]",
		"[*Total*]", "[*Total ("+currency+")*]",
	).Replace(layout)
}

// orDash returns s, or "—" when s is empty — used for optional display fields like
// vencimiento, which a draft only has once its series asks the salesperson to type one.
func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// typstStringLiteral renders s as a double-quoted Typst string literal. Only
// backslash, double-quote, and control characters need escaping — everything else
// (including Typst's own markup metacharacters: #, *, _, [, ], $, <, @) is inert
// inside a string literal, since it's read as raw text, not parsed as markup.
func typstStringLiteral(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r', '\t':
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// pngBytesLiteral renders data as a comma-separated decimal byte list for Typst's
// bytes() constructor — the logo is embedded directly in the generated source so
// Render's pure stdin/stdout subprocess interface never needs filesystem access.
func pngBytesLiteral(data []byte) string {
	var b strings.Builder
	b.Grow(len(data) * 4)
	for i, by := range data {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%d", by)
	}
	return b.String()
}
