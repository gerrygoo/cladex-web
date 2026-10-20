package facturacion

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gerrygoo/cladex-web/internal/cfdi"
	"github.com/gerrygoo/cladex-web/internal/pdf"
	"github.com/gerrygoo/cladex-web/internal/store"
)

var tiposComprobante = map[string]string{"I": "I - Ingreso", "E": "E - Egreso", "P": "P - Pago", "T": "T - Traslado"}

var metodosPago = map[string]string{"PUE": "PUE - Pago en una sola exhibición", "PPD": "PPD - Pago en parcialidades o diferido"}

var tiposRelacion = map[string]string{"04": "04 - Sustitución de los CFDI previos"}

func label(catalog []store.SATCode, code string) string {
	if l := store.SATLabel(catalog, code); l != "" {
		return l
	}
	return code
}

// Printed is the printed representation of a stamped comprobante, from its XML.
func Printed(c *store.Comprobante) (pdf.CFDIDocument, error) {
	x, err := cfdi.Parse(c.XML)
	if err != nil {
		return pdf.CFDIDocument{}, err
	}
	doc := pdf.CFDIDocument{
		Title:      "Factura",
		SerieFolio: x.Serie + "-" + x.Folio,
		Fecha:      strings.Replace(x.Fecha, "T", " ", 1),
		UUID:       x.UUID(),
		TipoLabel:  tiposComprobante[x.TipoDeComprobante],
		Emisor: []string{x.Emisor.Nombre, "RFC " + x.Emisor.Rfc, "Régimen fiscal " + label(store.TaxRegimes, x.Emisor.RegimenFiscal),
			"Lugar de expedición " + x.LugarExpedicion},
		Receptor: []string{x.Receptor.Nombre, "RFC " + x.Receptor.Rfc, "Régimen fiscal " + label(store.TaxRegimes, x.Receptor.RegimenFiscalReceptor),
			"Domicilio fiscal (C.P.) " + x.Receptor.DomicilioFiscalReceptor},
		SelloCFD:         x.Timbre.SelloCFD,
		SelloSAT:         x.Timbre.SelloSAT,
		CadenaOriginal:   x.CadenaOriginalTimbre(),
		NoCertificado:    x.NoCertificado,
		NoCertificadoSAT: x.Timbre.NoCertificadoSAT,
		FechaTimbrado:    strings.Replace(x.Timbre.FechaTimbrado, "T", " ", 1),
		RfcProvCertif:    x.Timbre.RfcProvCertif,
		VerificationURL:  x.VerificationURL(),
		Sandbox:          c.Sandbox,
	}
	if t, err := time.Parse(time.RFC3339Nano, c.StampedAt); err == nil {
		doc.Created = t // the same PDF bytes every time
	}
	if x.Relacionados != nil && len(x.Relacionados.CFDIs) > 0 {
		var uuids []string
		for _, r := range x.Relacionados.CFDIs {
			uuids = append(uuids, strings.ToUpper(r.UUID))
		}
		rel := tiposRelacion[x.Relacionados.TipoRelacion]
		if rel == "" {
			rel = x.Relacionados.TipoRelacion
		}
		doc.Relacionados = rel + ": " + strings.Join(uuids, ", ")
	}
	if x.TipoDeComprobante == "P" {
		doc.Title = "Comprobante de pago"
		doc.Details = [][2]string{{"Uso de CFDI", x.Receptor.UsoCFDI + " - Pagos"}, {"Exportación", x.Exportacion + " - No aplica"}}
		if x.Pagos != nil {
			for _, p := range x.Pagos.Pagos {
				cp := pdf.CFDIPago{Fecha: strings.Replace(p.FechaPago, "T", " ", 1), Forma: label(store.PaymentForms, p.FormaDePagoP),
					Moneda: p.MonedaP, Monto: amount(p.Monto)}
				for _, d := range p.Documentos {
					cp.Documentos = append(cp.Documentos, []string{strings.ToUpper(d.IdDocumento), d.Serie + "-" + d.Folio, d.NumParcialidad,
						amount(d.ImpSaldoAnt), amount(d.ImpPagado), amount(d.ImpSaldoInsoluto)})
				}
				doc.Pagos = append(doc.Pagos, cp)
			}
			doc.Totals = [][2]string{{"Total pagado", amount(x.Pagos.Totales.MontoTotalPagos)}}
			doc.TotalLetra = enLetra(x.Pagos.Totales.MontoTotalPagos, "MXN")
		}
		return doc, nil
	}
	doc.Details = [][2]string{
		{"Método de pago", metodosPago[x.MetodoPago]},
		{"Forma de pago", label(store.PaymentForms, x.FormaPago)},
		{"Uso de CFDI", label(store.CFDIUses, x.Receptor.UsoCFDI)},
		{"Moneda", x.Moneda},
	}
	if x.Moneda != "MXN" && x.TipoCambio != "" {
		doc.Details = append(doc.Details, [2]string{"Tipo de cambio", x.TipoCambio})
	}
	doc.Details = append(doc.Details, [2]string{"Exportación", x.Exportacion + " - No aplica"})
	for _, k := range x.Conceptos {
		unidad := k.ClaveUnidad
		if k.Unidad != "" {
			unidad += " " + k.Unidad
		}
		doc.Conceptos = append(doc.Conceptos, pdf.CFDIConcepto{ClaveProdServ: k.ClaveProdServ, Cantidad: k.Cantidad, Unidad: unidad,
			Descripcion: k.Descripcion, ValorUnitario: amount(k.ValorUnitario), Importe: amount(k.Importe)})
	}
	iva := "IVA 16%"
	doc.Totals = [][2]string{{"Subtotal", amount(x.SubTotal)}, {iva, amount(x.Impuestos.TotalImpuestosTrasladados)}, {"Total", amount(x.Total)}}
	doc.TotalLetra = enLetra(x.Total, x.Moneda)
	return doc, nil
}

// amount writes an XML amount ("48912.35") as money ("$48,912.35"), keeping its decimals.
func amount(s string) string {
	if s == "" {
		return ""
	}
	whole, frac, _ := strings.Cut(s, ".")
	neg := strings.HasPrefix(whole, "-")
	whole = strings.TrimPrefix(whole, "-")
	var b strings.Builder
	for i, r := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	out := "$" + b.String()
	if frac != "" {
		out += "." + frac
	}
	if neg {
		out = "-" + out
	}
	return out
}

// enLetra writes an amount in words, as Mexican invoices do: "CIENTO DIECISÉIS PESOS
// 00/100 M.N.".
func enLetra(s, moneda string) string {
	whole, frac, _ := strings.Cut(s, ".")
	n, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || n < 0 {
		return ""
	}
	cents := (frac + "00")[:2]
	unit, suffix := "PESOS", "M.N."
	if n == 1 {
		unit = "PESO"
	}
	if moneda == "USD" {
		unit, suffix = "DÓLARES", "USD"
		if n == 1 {
			unit = "DÓLAR"
		}
	}
	words := numberWords(n)
	if n%1_000_000 == 0 && n > 0 {
		words += " DE"
	}
	return fmt.Sprintf("%s %s %s/100 %s", words, unit, cents, suffix)
}

var (
	unidades = []string{"CERO", "UN", "DOS", "TRES", "CUATRO", "CINCO", "SEIS", "SIETE", "OCHO", "NUEVE", "DIEZ",
		"ONCE", "DOCE", "TRECE", "CATORCE", "QUINCE", "DIECISÉIS", "DIECISIETE", "DIECIOCHO", "DIECINUEVE", "VEINTE",
		"VEINTIÚN", "VEINTIDÓS", "VEINTITRÉS", "VEINTICUATRO", "VEINTICINCO", "VEINTISÉIS", "VEINTISIETE", "VEINTIOCHO", "VEINTINUEVE"}
	decenas  = []string{"", "", "", "TREINTA", "CUARENTA", "CINCUENTA", "SESENTA", "SETENTA", "OCHENTA", "NOVENTA"}
	centenas = []string{"", "CIENTO", "DOSCIENTOS", "TRESCIENTOS", "CUATROCIENTOS", "QUINIENTOS", "SEISCIENTOS", "SETECIENTOS", "OCHOCIENTOS", "NOVECIENTOS"}
)

func numberWords(n int64) string {
	switch {
	case n < 30:
		return unidades[n]
	case n < 100:
		if n%10 == 0 {
			return decenas[n/10]
		}
		return decenas[n/10] + " Y " + unidades[n%10]
	case n == 100:
		return "CIEN"
	case n < 1000:
		if n%100 == 0 {
			return centenas[n/100]
		}
		return centenas[n/100] + " " + numberWords(n%100)
	case n < 1_000_000:
		miles := "MIL"
		if n/1000 > 1 {
			miles = numberWords(n/1000) + " MIL"
		}
		if n%1000 == 0 {
			return miles
		}
		return miles + " " + numberWords(n%1000)
	default:
		millones := "UN MILLÓN"
		if n/1_000_000 > 1 {
			millones = numberWords(n/1_000_000) + " MILLONES"
		}
		if n%1_000_000 == 0 {
			return millones
		}
		return millones + " " + numberWords(n%1_000_000)
	}
}
