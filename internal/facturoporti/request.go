package facturoporti

import (
	"encoding/json"
	"fmt"
	"strings"
)

// The JSON Facturoporti's /servicios/timbrar/json takes, limited to what Cladex sends:
// facturas de ingreso and comprobantes de pago (complemento Pagos 2.0). Field names follow
// their examples (Ejemplos.zip). Amounts are json.Number built by Dec, never float64: the
// API keeps the decimals it is sent, and the SAT checks them (an IVA rate of 0.16 instead
// of 0.160000 is rejected with CFDI40179).

// StampRequest is one CFDI to stamp. The CSD fields are left empty in what the app stores
// and filled by Client.Stamp at send time, so no secret is ever written to the database.
type StampRequest struct {
	DatosGenerales DatosGenerales
	Encabezado     Encabezado
	Conceptos      []Concepto
	Complemento    *Complemento `json:",omitempty"`
}

type DatosGenerales struct {
	Version         string
	CSD             string `json:",omitempty"`
	LlavePrivada    string `json:",omitempty"`
	CSDPassword     string `json:",omitempty"`
	GeneraPDF       bool
	CFDI            string // Factura, Pago
	OpcionDecimales int    // 2: round
	NumeroDecimales int
	TipoCFDI        string // Ingreso, Pago
	EnviaEmail      bool
	ReceptorEmail   string `json:",omitempty"` // several, separated by ";"
	EmailMensaje    string `json:",omitempty"`
}

type Encabezado struct {
	CFDIsRelacionados string `json:",omitempty"` // UUIDs separated by ";"
	TipoRelacion      string `json:",omitempty"`
	Emisor            Emisor
	Receptor          Receptor
	Fecha             string // yyyy-MM-ddTHH:mm:ss, local time of the lugar de expedición
	Serie             string
	Folio             string
	MetodoPago        string      `json:",omitempty"`
	FormaPago         string      `json:",omitempty"`
	Moneda            string      `json:",omitempty"`
	TipoCambio        json.Number `json:",omitempty"`
	LugarExpedicion   string
	SubTotal          json.Number
	Total             json.Number
}

type Emisor struct {
	RFC               string
	NombreRazonSocial string
	RegimenFiscal     string
}

type Receptor struct {
	RFC               string
	NombreRazonSocial string
	UsoCFDI           string
	RegimenFiscal     string
	Direccion         Direccion
}

type Direccion struct {
	CodigoPostal string
}

type Concepto struct {
	Cantidad         json.Number
	CodigoUnidad     string
	Unidad           string `json:",omitempty"`
	CodigoProducto   string
	Producto         string
	PrecioUnitario   json.Number
	Importe          json.Number
	ObjetoDeImpuesto string
	Impuestos        []Impuesto
}

// Impuesto is a concepto's tax. Cladex only charges IVA trasladado at 16%.
type Impuesto struct {
	TipoImpuesto    int // 1: trasladado
	Impuesto        int // 2: IVA
	Factor          int // 1: tasa
	Base            json.Number
	Tasa            json.Number
	ImpuestoImporte json.Number
}

// Complemento carries Pagos 2.0 (TipoComplemento 28) for a comprobante de pago.
type Complemento struct {
	TipoComplemento int
	PagosV20        *PagosV20 `json:",omitempty"`
}

type PagosV20 struct {
	Pagos   []Pago
	Totales PagosTotales
}

type Pago struct {
	FechaPago              string
	FormaPago              string
	Moneda                 string
	TipoCambio             json.Number
	DocumentosRelacionados []DocumentoRelacionado
	Impuestos              ImpuestosPago
}

type DocumentoRelacionado struct {
	IdDocumento          string
	Serie                string `json:",omitempty"`
	Folio                string `json:",omitempty"`
	Moneda               string
	Equivalencia         json.Number
	NumeroParcialidad    int
	ImporteSaldoAnterior json.Number
	ImportePagado        json.Number
	ImporteSaldoInsoluto json.Number
	ObjetoDeImpuesto     string
	Impuestos            ImpuestosPago
}

type ImpuestosPago struct {
	Trasladados []TrasladoPago
}

type TrasladoPago struct {
	Impuesto int
	Factor   int
	Base     json.Number
	Tasa     json.Number
	Importe  json.Number
}

type PagosTotales struct {
	TotalTrasladosBaseIVA16     json.Number
	TotalTrasladosImpuestoIVA16 json.Number
	MontoTotalPagos             json.Number
}

// TipoComplementoPagos20 is Pagos 2.0 in Facturoporti's list of complementos.
const TipoComplementoPagos20 = 28

// IVA16 is the 16% IVA rate as the SAT's c_TasaOCuota writes it.
const IVA16 = json.Number("0.160000")

// Dec writes v, a count of 10^-places units, as a decimal literal with exactly that many
// places: Dec(11600, 2) is 116.00, Dec(39129876, 6) is 39.129876.
func Dec(v int64, places int) json.Number {
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	if places == 0 {
		return json.Number(fmt.Sprintf("%s%d", sign, v))
	}
	scale := int64(1)
	for range places {
		scale *= 10
	}
	return json.Number(fmt.Sprintf("%s%d.%0*d", sign, v/scale, places, v%scale))
}

// Marshal encodes a request exactly as it is stored and sent. It is deterministic: the
// same request always gives the same bytes, which is what makes a retry safe (see Client.Stamp).
func Marshal(r StampRequest) ([]byte, error) {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(r); err != nil {
		return nil, err
	}
	return []byte(strings.TrimSuffix(b.String(), "\n")), nil
}
