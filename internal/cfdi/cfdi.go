// Package cfdi reads a stamped CFDI 4.0, the XML Facturoporti returns and the app keeps as
// the legal document, for what the app shows and prints from it: the printed
// representation (internal/pdf) is always rendered from this, never stored.
package cfdi

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"
)

// Comprobante is what the app reads from a stamped CFDI. Amounts stay as the XML writes
// them, since that is what the printed representation must show.
type Comprobante struct {
	Version           string `xml:"Version,attr"`
	Serie             string `xml:"Serie,attr"`
	Folio             string `xml:"Folio,attr"`
	Fecha             string `xml:"Fecha,attr"`
	Sello             string `xml:"Sello,attr"`
	FormaPago         string `xml:"FormaPago,attr"`
	NoCertificado     string `xml:"NoCertificado,attr"`
	SubTotal          string `xml:"SubTotal,attr"`
	Moneda            string `xml:"Moneda,attr"`
	TipoCambio        string `xml:"TipoCambio,attr"`
	Total             string `xml:"Total,attr"`
	TipoDeComprobante string `xml:"TipoDeComprobante,attr"`
	Exportacion       string `xml:"Exportacion,attr"`
	MetodoPago        string `xml:"MetodoPago,attr"`
	LugarExpedicion   string `xml:"LugarExpedicion,attr"`

	Relacionados *struct {
		TipoRelacion string `xml:"TipoRelacion,attr"`
		CFDIs        []struct {
			UUID string `xml:"UUID,attr"`
		} `xml:"CfdiRelacionado"`
	} `xml:"CfdiRelacionados"`
	Emisor struct {
		Rfc           string `xml:"Rfc,attr"`
		Nombre        string `xml:"Nombre,attr"`
		RegimenFiscal string `xml:"RegimenFiscal,attr"`
	} `xml:"Emisor"`
	Receptor struct {
		Rfc                     string `xml:"Rfc,attr"`
		Nombre                  string `xml:"Nombre,attr"`
		DomicilioFiscalReceptor string `xml:"DomicilioFiscalReceptor,attr"`
		RegimenFiscalReceptor   string `xml:"RegimenFiscalReceptor,attr"`
		UsoCFDI                 string `xml:"UsoCFDI,attr"`
	} `xml:"Receptor"`
	Conceptos []Concepto `xml:"Conceptos>Concepto"`
	Impuestos struct {
		TotalImpuestosTrasladados string     `xml:"TotalImpuestosTrasladados,attr"`
		Traslados                 []Traslado `xml:"Traslados>Traslado"`
	} `xml:"Impuestos"`
	Timbre *Timbre `xml:"Complemento>TimbreFiscalDigital"`
	Pagos  *Pagos  `xml:"Complemento>Pagos"`
}

type Concepto struct {
	ClaveProdServ string     `xml:"ClaveProdServ,attr"`
	Cantidad      string     `xml:"Cantidad,attr"`
	ClaveUnidad   string     `xml:"ClaveUnidad,attr"`
	Unidad        string     `xml:"Unidad,attr"`
	Descripcion   string     `xml:"Descripcion,attr"`
	ValorUnitario string     `xml:"ValorUnitario,attr"`
	Importe       string     `xml:"Importe,attr"`
	ObjetoImp     string     `xml:"ObjetoImp,attr"`
	Traslados     []Traslado `xml:"Impuestos>Traslados>Traslado"`
}

type Traslado struct {
	Base       string `xml:"Base,attr"`
	Impuesto   string `xml:"Impuesto,attr"`
	TipoFactor string `xml:"TipoFactor,attr"`
	TasaOCuota string `xml:"TasaOCuota,attr"`
	Importe    string `xml:"Importe,attr"`
}

// Timbre is the SAT's timbre fiscal digital, what makes the CFDI valid.
type Timbre struct {
	Version          string `xml:"Version,attr"`
	UUID             string `xml:"UUID,attr"`
	FechaTimbrado    string `xml:"FechaTimbrado,attr"`
	RfcProvCertif    string `xml:"RfcProvCertif,attr"`
	SelloCFD         string `xml:"SelloCFD,attr"`
	NoCertificadoSAT string `xml:"NoCertificadoSAT,attr"`
	SelloSAT         string `xml:"SelloSAT,attr"`
}

// Pagos is the complemento de pagos 2.0 of a comprobante de pago.
type Pagos struct {
	Totales struct {
		MontoTotalPagos string `xml:"MontoTotalPagos,attr"`
	} `xml:"Totales"`
	Pagos []struct {
		FechaPago    string `xml:"FechaPago,attr"`
		FormaDePagoP string `xml:"FormaDePagoP,attr"`
		MonedaP      string `xml:"MonedaP,attr"`
		TipoCambioP  string `xml:"TipoCambioP,attr"`
		Monto        string `xml:"Monto,attr"`
		Documentos   []struct {
			IdDocumento      string `xml:"IdDocumento,attr"`
			Serie            string `xml:"Serie,attr"`
			Folio            string `xml:"Folio,attr"`
			MonedaDR         string `xml:"MonedaDR,attr"`
			NumParcialidad   string `xml:"NumParcialidad,attr"`
			ImpSaldoAnt      string `xml:"ImpSaldoAnt,attr"`
			ImpPagado        string `xml:"ImpPagado,attr"`
			ImpSaldoInsoluto string `xml:"ImpSaldoInsoluto,attr"`
		} `xml:"DoctoRelacionado"`
	} `xml:"Pago"`
}

// Parse reads a stamped CFDI. It is an error if the XML has no timbre.
func Parse(doc string) (*Comprobante, error) {
	var c Comprobante
	if err := xml.Unmarshal([]byte(doc), &c); err != nil {
		return nil, fmt.Errorf("cfdi: %w", err)
	}
	if c.Timbre == nil || c.Timbre.UUID == "" {
		return nil, fmt.Errorf("cfdi: no timbre fiscal digital")
	}
	return &c, nil
}

// UUID is the folio fiscal, in capitals as the SAT prints it.
func (c *Comprobante) UUID() string { return strings.ToUpper(c.Timbre.UUID) }

// CadenaOriginalTimbre is the timbre's cadena original, which the printed representation
// carries (Anexo 20: ||Version|UUID|FechaTimbrado|RfcProvCertif|SelloCFD|NoCertificadoSAT||).
func (c *Comprobante) CadenaOriginalTimbre() string {
	t := c.Timbre
	return "||" + strings.Join([]string{t.Version, t.UUID, t.FechaTimbrado, t.RfcProvCertif, t.SelloCFD, t.NoCertificadoSAT}, "|") + "||"
}

// VerificationURL is what the printed representation's QR code holds: the SAT's
// verification page for this CFDI.
func (c *Comprobante) VerificationURL() string {
	fe := c.Sello
	if len(fe) > 8 {
		fe = fe[len(fe)-8:]
	}
	q := url.Values{}
	q.Set("id", c.UUID())
	q.Set("re", c.Emisor.Rfc)
	q.Set("rr", c.Receptor.Rfc)
	q.Set("tt", c.Total)
	q.Set("fe", fe)
	// The SAT expects these keys in this order.
	return "https://verificacfdi.facturaelectronica.sat.gob.mx/default.aspx?id=" + q.Get("id") + "&re=" + url.QueryEscape(q.Get("re")) +
		"&rr=" + url.QueryEscape(q.Get("rr")) + "&tt=" + q.Get("tt") + "&fe=" + url.QueryEscape(q.Get("fe"))
}
