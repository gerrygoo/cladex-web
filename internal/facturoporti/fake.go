package facturoporti

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Fake is an in-process stand-in for Facturoporti, for tests and for running the app
// without credentials. It behaves like the sandbox where that was checked (2026-10-20):
// a byte-identical request gets the stamp it already got, any other request a new UUID,
// errors come as HTTP 200 with a non-000 code, and cancellation of a stamped UUID is
// immediate. Its XML has the shape of a real stamped CFDI 4.0, with placeholder seals.
type Fake struct {
	mu        sync.Mutex
	stamps    map[string]string // request hash → UUID
	xmls      map[string]string // UUID → XML
	cancelled map[string]bool
	n         int

	// Refuse, when set, makes the next stamp answer with this SAT-style error.
	Refuse string
	// Drop makes the next stamp do its work and then fail the HTTP response (a 502), like
	// a connection lost after Facturoporti stamped: the outcome is Unknown, and a resend
	// of the same request returns the stamp.
	Drop bool
	// CancelPending makes cancellations wait on the receptor.
	CancelPending bool
	// Requests counts stamp requests that reached it.
	Requests int
}

func NewFake() *Fake {
	return &Fake{stamps: map[string]string{}, xmls: map[string]string{}, cancelled: map[string]bool{}}
}

// Stamps is how many distinct CFDIs the fake has stamped.
func (f *Fake) Stamps() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.xmls)
}

func (f *Fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		http.Error(w, "", http.StatusUnauthorized)
		return
	}
	switch r.URL.Path {
	case "/servicios/timbrar/json":
		f.stamp(w, body)
	case "/servicios/cancelar/csd":
		f.cancel(w, body)
	case "/validar/estatuscfdi":
		f.status(w, body)
	default:
		http.NotFound(w, r)
	}
}

func (f *Fake) stamp(w http.ResponseWriter, body []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Requests++
	var req StampRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, `{"title":"One or more validation errors occurred.","status":400}`, http.StatusBadRequest)
		return
	}
	if f.Refuse != "" {
		msg := f.Refuse
		f.Refuse = ""
		writeJSON(w, map[string]any{"estatus": map[string]any{"codigo": "017", "descripcion": "Error al generar el timbrado", "informacionTecnica": msg}})
		return
	}
	if req.DatosGenerales.CSD == "" || req.DatosGenerales.LlavePrivada == "" {
		writeJSON(w, map[string]any{"estatus": map[string]any{"codigo": "017", "descripcion": "Error al generar el timbrado", "informacionTecnica": "Falta el certificado"}})
		return
	}
	sum := sha256.Sum256(body)
	key := hex.EncodeToString(sum[:])
	uuid, ok := f.stamps[key]
	if !ok {
		f.n++
		uuid = fmt.Sprintf("FA4E0000-0000-4000-8000-%012d", f.n)
		f.stamps[key] = uuid
		f.xmls[uuid] = fakeXML(req, uuid, time.Now())
	}
	if f.Drop {
		f.Drop = false
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{
		"estatus": map[string]any{"codigo": "000", "descripcion": "Timbrado del CFDI realizado con éxito", "informacionTecnica": "Ok"},
		"cfdiTimbrado": map[string]any{"respuesta": map[string]any{
			"uuid": strings.ToLower(uuid), "cfdixml": f.xmls[uuid], "emailEnviado": fmt.Sprint(req.DatosGenerales.EnviaEmail),
			"noCertificado": "30001000000500003416", "idVersionTimbrado": "1.1",
		}},
	})
}

func (f *Fake) cancel(w http.ResponseWriter, body []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var req struct{ UUID, Motivo, FolioFiscalSustitucion string }
	json.Unmarshal(body, &req)
	uuid := strings.ToUpper(req.UUID)
	if _, ok := f.xmls[uuid]; !ok {
		writeJSON(w, map[string]any{"codigo": "205", "mensaje": "UUID no existe"})
		return
	}
	if req.Motivo == "01" && req.FolioFiscalSustitucion == "" {
		writeJSON(w, map[string]any{"codigo": "301", "mensaje": "Falta el folio fiscal que sustituye"})
		return
	}
	if f.CancelPending {
		writeJSON(w, map[string]any{"codigo": "000", "mensaje": "Solicitud enviada", "estado": "Vigente", "estatusCancelacion": "En proceso", "esCancelable": "Cancelable con aceptación", "acuse": "<Acuse/>"})
		return
	}
	f.cancelled[uuid] = true
	writeJSON(w, map[string]any{"codigo": "000", "mensaje": "Se envió con éxito el documento electrónico para ser cancelado al SAT.", "estado": "Cancelado", "esCancelable": "Cancelable sin aceptación", "acuse": "<Acuse/>"})
}

// AcceptCancellation settles a pending cancellation as the receptor accepting it.
func (f *Fake) AcceptCancellation(uuid string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancelled[strings.ToUpper(uuid)] = true
}

func (f *Fake) status(w http.ResponseWriter, body []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var reqs []struct{ UUID string }
	json.Unmarshal(body, &reqs)
	if len(reqs) == 0 {
		writeJSON(w, map[string]any{"codigo": "001", "cfdi": []any{}})
		return
	}
	uuid := strings.ToUpper(reqs[0].UUID)
	estado, estatus := "No Encontrado", ""
	if _, ok := f.xmls[uuid]; ok {
		estado = "Vigente"
		if f.CancelPending {
			estatus = "En proceso"
		}
		if f.cancelled[uuid] {
			estado, estatus = "Cancelado", "Cancelado sin aceptación"
		}
	}
	writeJSON(w, map[string]any{"codigo": "000", "cfdi": []map[string]any{{"uuid": uuid, "estado": estado, "estatusCancelacion": estatus}}})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// fakeXML is a stamped CFDI 4.0 for req, with the shape Facturoporti returns.
func fakeXML(req StampRequest, uuid string, now time.Time) string {
	a := html.EscapeString
	e := req.Encabezado
	fecha := now.Format("2006-01-02T15:04:05")
	tipo := "I"
	if req.DatosGenerales.TipoCFDI == "Pago" {
		tipo = "P"
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	fmt.Fprintf(&b, `<cfdi:Comprobante Version="4.0" Serie="%s" Folio="%s" Fecha="%s" Sello="FAKESELLO" NoCertificado="30001000000500003416" Certificado="FAKE"`, a(e.Serie), a(e.Folio), fecha)
	if e.FormaPago != "" {
		fmt.Fprintf(&b, ` FormaPago="%s"`, a(e.FormaPago))
	}
	moneda := e.Moneda
	if tipo == "P" {
		moneda = "XXX"
	}
	fmt.Fprintf(&b, ` SubTotal="%s" Moneda="%s" Total="%s" TipoDeComprobante="%s" Exportacion="01"`, e.SubTotal, a(moneda), e.Total, tipo)
	if e.MetodoPago != "" {
		fmt.Fprintf(&b, ` MetodoPago="%s"`, a(e.MetodoPago))
	}
	fmt.Fprintf(&b, ` LugarExpedicion="%s" xmlns:cfdi="http://www.sat.gob.mx/cfd/4" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">`, a(e.LugarExpedicion))
	if e.CFDIsRelacionados != "" {
		fmt.Fprintf(&b, `<cfdi:CfdiRelacionados TipoRelacion="%s">`, a(e.TipoRelacion))
		for _, u := range strings.Split(e.CFDIsRelacionados, ";") {
			fmt.Fprintf(&b, `<cfdi:CfdiRelacionado UUID="%s" />`, a(u))
		}
		b.WriteString(`</cfdi:CfdiRelacionados>`)
	}
	fmt.Fprintf(&b, `<cfdi:Emisor Rfc="%s" Nombre="%s" RegimenFiscal="%s" />`, a(e.Emisor.RFC), a(e.Emisor.NombreRazonSocial), a(e.Emisor.RegimenFiscal))
	fmt.Fprintf(&b, `<cfdi:Receptor Rfc="%s" Nombre="%s" DomicilioFiscalReceptor="%s" RegimenFiscalReceptor="%s" UsoCFDI="%s" />`,
		a(e.Receptor.RFC), a(e.Receptor.NombreRazonSocial), a(e.Receptor.Direccion.CodigoPostal), a(e.Receptor.RegimenFiscal), a(e.Receptor.UsoCFDI))
	b.WriteString(`<cfdi:Conceptos>`)
	var iva, base float64
	for _, c := range req.Conceptos {
		fmt.Fprintf(&b, `<cfdi:Concepto ClaveProdServ="%s" Cantidad="%s" ClaveUnidad="%s"`, a(c.CodigoProducto), c.Cantidad, a(c.CodigoUnidad))
		if c.Unidad != "" {
			fmt.Fprintf(&b, ` Unidad="%s"`, a(c.Unidad))
		}
		fmt.Fprintf(&b, ` Descripcion="%s" ValorUnitario="%s" Importe="%s" ObjetoImp="%s">`, a(c.Producto), c.PrecioUnitario, c.Importe, a(c.ObjetoDeImpuesto))
		if len(c.Impuestos) > 0 {
			b.WriteString(`<cfdi:Impuestos><cfdi:Traslados>`)
			for _, t := range c.Impuestos {
				fmt.Fprintf(&b, `<cfdi:Traslado Base="%s" Impuesto="002" TipoFactor="Tasa" TasaOCuota="%s" Importe="%s" />`, t.Base, t.Tasa, t.ImpuestoImporte)
				x, _ := t.ImpuestoImporte.Float64()
				y, _ := t.Base.Float64()
				iva, base = iva+x, base+y
			}
			b.WriteString(`</cfdi:Traslados></cfdi:Impuestos>`)
		}
		b.WriteString(`</cfdi:Concepto>`)
	}
	b.WriteString(`</cfdi:Conceptos>`)
	if tipo == "I" {
		fmt.Fprintf(&b, `<cfdi:Impuestos TotalImpuestosTrasladados="%.2f"><cfdi:Traslados><cfdi:Traslado Base="%.2f" Impuesto="002" TipoFactor="Tasa" TasaOCuota="0.160000" Importe="%.2f" /></cfdi:Traslados></cfdi:Impuestos>`, iva, base, iva)
	}
	b.WriteString(`<cfdi:Complemento>`)
	if req.Complemento != nil && req.Complemento.PagosV20 != nil {
		p := req.Complemento.PagosV20
		fmt.Fprintf(&b, `<pago20:Pagos Version="2.0" xmlns:pago20="http://www.sat.gob.mx/Pagos20"><pago20:Totales MontoTotalPagos="%s" TotalTrasladosBaseIVA16="%s" TotalTrasladosImpuestoIVA16="%s" />`,
			p.Totales.MontoTotalPagos, p.Totales.TotalTrasladosBaseIVA16, p.Totales.TotalTrasladosImpuestoIVA16)
		for _, pg := range p.Pagos {
			monto := p.Totales.MontoTotalPagos
			fmt.Fprintf(&b, `<pago20:Pago FechaPago="%s" FormaDePagoP="%s" MonedaP="%s" TipoCambioP="1" Monto="%s">`, a(pg.FechaPago), a(pg.FormaPago), a(pg.Moneda), monto)
			for _, d := range pg.DocumentosRelacionados {
				fmt.Fprintf(&b, `<pago20:DoctoRelacionado IdDocumento="%s" Serie="%s" Folio="%s" MonedaDR="%s" EquivalenciaDR="1" NumParcialidad="%d" ImpSaldoAnt="%s" ImpPagado="%s" ImpSaldoInsoluto="%s" ObjetoImpDR="%s" />`,
					a(d.IdDocumento), a(d.Serie), a(d.Folio), a(d.Moneda), d.NumeroParcialidad, d.ImporteSaldoAnterior, d.ImportePagado, d.ImporteSaldoInsoluto, a(d.ObjetoDeImpuesto))
			}
			b.WriteString(`</pago20:Pago>`)
		}
		b.WriteString(`</pago20:Pagos>`)
	}
	fmt.Fprintf(&b, `<tfd:TimbreFiscalDigital Version="1.1" UUID="%s" FechaTimbrado="%s" RfcProvCertif="SPR190613I52" SelloCFD="FAKESELLOCFD12345678" NoCertificadoSAT="30001000000500003456" SelloSAT="FAKESELLOSAT" xmlns:tfd="http://www.sat.gob.mx/TimbreFiscalDigital" />`,
		strings.ToLower(uuid), fecha)
	b.WriteString(`</cfdi:Complemento></cfdi:Comprobante>`)
	return b.String()
}
