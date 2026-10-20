package pdf

import (
	"context"
	"fmt"
	"strings"
	"time"

	"rsc.io/qr"
)

// CFDIDocument is the printed representation of a stamped CFDI, every field already
// display-formatted by the caller from the stamped XML (internal/cfdi), which stays the
// legal document. The PDF is rendered on demand and never stored.
type CFDIDocument struct {
	Title        string // "Factura" or "Comprobante de pago"
	SerieFolio   string
	Fecha        string
	UUID         string
	TipoLabel    string // "I - Ingreso"
	Emisor       []string
	Receptor     []string
	Details      [][2]string // label, value: método/forma de pago, moneda, uso de CFDI…
	Relacionados string      // "04 - Sustitución de los CFDI previos: <uuid>"
	Conceptos    []CFDIConcepto
	Totals       [][2]string // Subtotal, IVA 16%, Total; the last one is printed bold
	Pagos        []CFDIPago
	TotalLetra   string

	SelloCFD        string
	SelloSAT        string
	CadenaOriginal  string
	NoCertificado   string
	NoCertificadoSAT string
	FechaTimbrado   string
	RfcProvCertif   string
	VerificationURL string

	// Sandbox CFDIs carry a watermark: they have no fiscal value.
	Sandbox bool
	Created time.Time
}

type CFDIConcepto struct {
	ClaveProdServ, Cantidad, Unidad, Descripcion, ValorUnitario, Importe string
}

type CFDIPago struct {
	Fecha, Forma, Moneda, Monto string
	Documentos              [][]string // UUID, serie-folio, parcialidad, saldo anterior, pagado, insoluto
}

// RenderCFDI lays out a CFDI's printed representation.
func RenderCFDI(ctx context.Context, doc CFDIDocument) ([]byte, error) {
	src, err := buildCFDITypst(doc)
	if err != nil {
		return nil, err
	}
	return Render(ctx, src, doc.Created)
}

// breakable lets Typst wrap a long unbroken string (a seal is hundreds of base64
// characters) by putting a zero-width space every 16 characters.
func breakable(s string) string {
	var b strings.Builder
	for i, r := range []rune(s) {
		if i > 0 && i%16 == 0 {
			b.WriteRune('​')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func typstList(items []string) string {
	var b strings.Builder
	b.WriteString("(")
	for _, s := range items {
		b.WriteString(typstStringLiteral(s))
		b.WriteString(", ")
	}
	b.WriteString(")")
	return b.String()
}

func buildCFDITypst(doc CFDIDocument) (string, error) {
	code, err := qr.Encode(doc.VerificationURL, qr.M)
	if err != nil {
		return "", fmt.Errorf("pdf: qr: %w", err)
	}
	var b strings.Builder
	b.WriteString("#set page(paper: \"us-letter\", margin: 1.6cm")
	if doc.Sandbox {
		b.WriteString(`, background: rotate(-35deg, text(size: 64pt, weight: "bold", fill: rgb(200, 0, 0, 40))[SIN VALIDEZ FISCAL])`)
	}
	b.WriteString(")\n#set text(size: 8.5pt)\n")
	writeLetterhead(&b, LetterheadCladex)
	fmt.Fprintf(&b, "#let qrcode = bytes((%s))\n", pngBytesLiteral(code.PNG()))
	lets := map[string]string{
		"title": doc.Title, "seriefolio": doc.SerieFolio, "fecha": doc.Fecha, "uuid": doc.UUID, "tipo": doc.TipoLabel,
		"relacionados": doc.Relacionados, "letra": doc.TotalLetra,
		"sellocfd": breakable(doc.SelloCFD), "sellosat": breakable(doc.SelloSAT), "cadena": breakable(doc.CadenaOriginal),
		"nocert": doc.NoCertificado, "nocertsat": doc.NoCertificadoSAT, "fechatimbrado": doc.FechaTimbrado, "pac": doc.RfcProvCertif,
	}
	for _, k := range []string{"title", "seriefolio", "fecha", "uuid", "tipo", "relacionados", "letra", "sellocfd", "sellosat", "cadena", "nocert", "nocertsat", "fechatimbrado", "pac"} {
		fmt.Fprintf(&b, "#let %s = %s\n", k, typstStringLiteral(lets[k]))
	}
	fmt.Fprintf(&b, "#let emisor = %s\n#let receptor = %s\n", typstList(doc.Emisor), typstList(doc.Receptor))
	b.WriteString("#let details = (")
	for _, d := range doc.Details {
		fmt.Fprintf(&b, "(%s, %s), ", typstStringLiteral(d[0]), typstStringLiteral(d[1]))
	}
	b.WriteString(")\n#let totals = (")
	for _, d := range doc.Totals {
		fmt.Fprintf(&b, "(%s, %s), ", typstStringLiteral(d[0]), typstStringLiteral(d[1]))
	}
	b.WriteString(")\n#let conceptos = (")
	for _, c := range doc.Conceptos {
		fmt.Fprintf(&b, "%s, ", typstList([]string{c.ClaveProdServ, c.Cantidad, c.Unidad, c.Descripcion, c.ValorUnitario, c.Importe}))
	}
	b.WriteString(")\n")

	b.WriteString(`
#grid(
  columns: (auto, 1fr),
  align: (left + horizon, right),
  letterhead,
  [
    #text(size: 14pt, weight: "bold")[#title #seriefolio] \
    Fecha: #fecha \
    Folio fiscal: #uuid \
    Tipo de comprobante: #tipo
  ],
)
#v(0.4cm)
#grid(
  columns: (1fr, 1fr),
  gutter: 1.2em,
  [*Emisor* \ #emisor.join(linebreak())],
  [*Receptor* \ #receptor.join(linebreak())],
)
#v(0.3cm)
#grid(columns: (auto, 1fr, auto, 1fr), column-gutter: 0.6em, row-gutter: 0.4em,
  ..details.map(((k, v)) => ([*#k:*], v)).flatten())
#if relacionados != "" [
  #v(0.2cm)
  *CFDI relacionado:* #relacionados
]
#v(0.3cm)
`)
	if len(doc.Conceptos) > 0 {
		b.WriteString(`#table(
  columns: (auto, auto, auto, 1fr, auto, auto),
  align: (left, right, left, left, right, right),
  stroke: 0.5pt + rgb("#cccccc"),
  fill: (col, row) => if row == 0 { rgb("#3d3836") } else { white },
  table.header(..("Clave SAT", "Cantidad", "Unidad", "Descripción", "Valor unitario", "Importe").map(h => text(fill: white, weight: "bold")[#h])),
  ..conceptos.flatten()
)
`)
	}
	for _, p := range doc.Pagos {
		fmt.Fprintf(&b, "*Pago:* #%s · forma de pago #%s · #%s · monto #%s\n",
			typstStringLiteral(p.Fecha), typstStringLiteral(p.Forma), typstStringLiteral(p.Moneda), typstStringLiteral(p.Monto))
		b.WriteString("#table(columns: (1fr, auto, auto, auto, auto, auto), stroke: 0.5pt + rgb(\"#cccccc\"),\n")
		b.WriteString(`  table.header(..("Documento relacionado", "Serie y folio", "Parcialidad", "Saldo anterior", "Pagado", "Saldo insoluto").map(h => [*#h*])),` + "\n")
		for _, d := range p.Documentos {
			fmt.Fprintf(&b, "  ..%s,\n", typstList(d))
		}
		b.WriteString(")\n")
	}
	b.WriteString(`
#align(right)[
  #grid(columns: (auto, auto), gutter: 0.6em,
    ..totals.enumerate().map(((i, kv)) => if i == totals.len() - 1 { ([*#kv.at(0)*], [*#kv.at(1)*]) } else { ([#kv.at(0)], [#kv.at(1)]) }).flatten())
]
#if letra != "" [ #align(right)[#emph(letra)] ]
#v(0.4cm)
#grid(
  columns: (3.2cm, 1fr),
  gutter: 0.8em,
  image(qrcode, width: 3.2cm),
  [
    #set text(size: 6.5pt)
    *Sello digital del CFDI:* \ #sellocfd \
    *Sello del SAT:* \ #sellosat \
    *Cadena original del complemento de certificación digital del SAT:* \ #cadena \
    *No. de serie del certificado del emisor:* #nocert · *del SAT:* #nocertsat \
    *Fecha y hora de certificación:* #fechatimbrado · *RFC del proveedor de certificación:* #pac
  ],
)
#v(0.3cm)
#align(center)[#text(size: 7pt)[Este documento es una representación impresa de un CFDI.]]
`)
	return b.String(), nil
}
