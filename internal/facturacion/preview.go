package facturacion

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gerrygoo/cladex-web/internal/facturoporti"
	"github.com/gerrygoo/cladex-web/internal/pdf"
	"github.com/gerrygoo/cladex-web/internal/store"
)

// Preview is the printed representation of a comprobante that isn't stamped yet (#63),
// so it can be checked before Timbrar. A borrador is laid out from the request it would
// send if validated now; once validated, from the exact request stored to be sent.
func (s *Service) Preview(ctx context.Context, id int64) (pdf.CFDIDocument, error) {
	c, related, err := s.load(ctx, id)
	if err != nil {
		return pdf.CFDIDocument{}, err
	}
	if c.XML != "" {
		return pdf.CFDIDocument{}, errors.New("facturacion: preview of a stamped comprobante")
	}
	var req facturoporti.StampRequest
	if c.Status != store.CompBorrador && c.Request != "" {
		if err := json.Unmarshal([]byte(c.Request), &req); err != nil {
			return pdf.CFDIDocument{}, err
		}
	} else {
		if c.Tipo == store.TipoPago && related == nil {
			return pdf.CFDIDocument{}, errors.New("facturacion: pago without its factura")
		}
		emisor, err := s.Store.EmisorSettings(ctx)
		if err != nil {
			return pdf.CFDIDocument{}, err
		}
		req = BuildRequest(c, related, emisor, c.Serie, c.Folio, s.Now())
	}
	return previewDocument(req), nil
}

// previewDocument lays out a stamp request the way Printed lays out the stamped XML.
func previewDocument(r facturoporti.StampRequest) pdf.CFDIDocument {
	e, rc := r.Encabezado, r.Encabezado.Receptor
	folio := "(sin folio)"
	if e.Folio != "" && e.Folio != "0" {
		folio = e.Serie + "-" + e.Folio
	}
	doc := pdf.CFDIDocument{
		Title:      "Factura",
		SerieFolio: folio,
		Fecha:      strings.Replace(e.Fecha, "T", " ", 1),
		UUID:       "se asigna al timbrar",
		TipoLabel:  tiposComprobante["I"],
		Emisor: []string{e.Emisor.NombreRazonSocial, "RFC " + e.Emisor.RFC, "Régimen fiscal " + label(store.TaxRegimes, e.Emisor.RegimenFiscal),
			"Lugar de expedición " + e.LugarExpedicion},
		Receptor: []string{rc.NombreRazonSocial, "RFC " + rc.RFC, "Régimen fiscal " + label(store.TaxRegimes, rc.RegimenFiscal),
			"Domicilio fiscal (C.P.) " + rc.Direccion.CodigoPostal},
		Draft: true,
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", e.Fecha, mexicoCity); err == nil {
		doc.Created = t
	}
	if e.CFDIsRelacionados != "" {
		doc.Relacionados = tiposRelacion[e.TipoRelacion] + ": " + strings.ToUpper(strings.ReplaceAll(e.CFDIsRelacionados, ";", ", "))
	}
	if r.Complemento != nil && r.Complemento.PagosV20 != nil {
		doc.Title, doc.TipoLabel = "Comprobante de pago", tiposComprobante["P"]
		doc.Details = [][2]string{{"Uso de CFDI", rc.UsoCFDI + " - Pagos"}, {"Exportación", "01 - No aplica"}}
		pv := r.Complemento.PagosV20
		for _, p := range pv.Pagos {
			cp := pdf.CFDIPago{Fecha: strings.Replace(p.FechaPago, "T", " ", 1), Forma: label(store.PaymentForms, p.FormaPago), Moneda: p.Moneda}
			var monto int64
			for _, d := range p.DocumentosRelacionados {
				cp.Documentos = append(cp.Documentos, []string{strings.ToUpper(d.IdDocumento), d.Serie + "-" + d.Folio, strconv.Itoa(d.NumeroParcialidad),
					amount(string(d.ImporteSaldoAnterior)), amount(string(d.ImportePagado)), amount(string(d.ImporteSaldoInsoluto))})
				monto += centavos(d.ImportePagado)
			}
			cp.Monto = amount(string(facturoporti.Dec(monto, 2)))
			doc.Pagos = append(doc.Pagos, cp)
		}
		doc.Totals = [][2]string{{"Total pagado", amount(string(pv.Totales.MontoTotalPagos))}}
		doc.TotalLetra = enLetra(string(pv.Totales.MontoTotalPagos), "MXN")
		return doc
	}
	doc.Details = [][2]string{
		{"Método de pago", metodosPago[e.MetodoPago]},
		{"Forma de pago", label(store.PaymentForms, e.FormaPago)},
		{"Uso de CFDI", label(store.CFDIUses, rc.UsoCFDI)},
		{"Moneda", e.Moneda},
	}
	if e.Moneda != "MXN" && e.TipoCambio != "" {
		doc.Details = append(doc.Details, [2]string{"Tipo de cambio", string(e.TipoCambio)})
	}
	doc.Details = append(doc.Details, [2]string{"Exportación", "01 - No aplica"})
	var iva int64
	for _, k := range r.Conceptos {
		unidad := k.CodigoUnidad
		if k.Unidad != "" {
			unidad += " " + k.Unidad
		}
		doc.Conceptos = append(doc.Conceptos, pdf.CFDIConcepto{ClaveProdServ: k.CodigoProducto, Cantidad: string(k.Cantidad), Unidad: unidad,
			Descripcion: k.Producto, ValorUnitario: amount(string(k.PrecioUnitario)), Importe: amount(string(k.Importe))})
		for _, t := range k.Impuestos {
			iva += centavos(t.ImpuestoImporte)
		}
	}
	doc.Totals = [][2]string{{"Subtotal", amount(string(e.SubTotal))}, {"IVA 16%", amount(string(facturoporti.Dec(iva, 2)))}, {"Total", amount(string(e.Total))}}
	doc.TotalLetra = enLetra(string(e.Total), e.Moneda)
	return doc
}

// centavos reads a two-decimal amount from a request ("116.00") as centavos.
func centavos(n json.Number) int64 {
	whole, frac, _ := strings.Cut(string(n), ".")
	neg := strings.HasPrefix(whole, "-")
	w, _ := strconv.ParseInt(strings.TrimPrefix(whole, "-"), 10, 64)
	f, _ := strconv.ParseInt((frac + "00")[:2], 10, 64)
	v := w*100 + f
	if neg {
		v = -v
	}
	return v
}
