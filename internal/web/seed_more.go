package web

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/store"
)

// seedEnv is what SeedStaging made that the extra proyectos build on.
type seedEnv struct {
	admin      *store.AuthenticatedUser
	vendedores []*store.AuthenticatedUser
	customers  []int64
	suppliers  []int64
}

// seedIssue creates and issues a quote of a kind for a customer, with SKU/quantity pairs.
type seedIssue func(u *store.AuthenticatedUser, kind string, customer int64, lines ...string) (*store.Project, error)

// seedRealUsers are the real users who can sign in to staging (STAGING_USERS: they have
// a staging password), so some proyectos are theirs and their dashboards aren't empty.
func seedRealUsers(ctx context.Context, st *store.Store) ([]*store.AuthenticatedUser, error) {
	users, err := st.ListUsers(ctx, "", "", nil)
	if err != nil {
		return nil, err
	}
	var out []*store.AuthenticatedUser
	for _, u := range users {
		if strings.HasPrefix(u.Username, "prueba-") || u.PasswordHash == NoPasswordHash || u.DisabledAt != nil {
			continue
		}
		out = append(out, &store.AuthenticatedUser{ID: u.ID, Username: u.Username, Name: u.Name, Role: u.Role})
	}
	return out, nil
}

// seedMoreProjects adds proyectos so staging shows every stage and state without real
// data (#81): prospectos at each probabilidad, with and without follow-up dates, a
// quote revision; en curso delivered, facturado P.U.E. and P.P.D. with complementos, a
// cancelled factura; Cerrado · Ganado both ways; Cancelado and Perdido. Some belong to
// the real STAGING_USERS. It returns a line per proyecto for the summary.
func seedMoreProjects(ctx context.Context, st *store.Store, pos *PurchaseOrders, now time.Time, issue seedIssue, e seedEnv) ([]string, error) {
	real, err := seedRealUsers(ctx, st)
	if err != nil {
		return nil, err
	}
	owners := append(append([]*store.AuthenticatedUser{}, real...), e.vendedores...)
	n := 0
	owner := func() *store.AuthenticatedUser { n++; return owners[(n-1)%len(owners)] }
	customer := func(i int) (int64, store.Customer) {
		return e.customers[i%len(e.customers)], seedCustomers[i%len(seedCustomers)]
	}
	day := func(offset int) string { return now.AddDate(0, 0, offset).Format("2006-01-02") }
	admin := e.admin
	var out []string
	note := func(p *store.Project, what string) { out = append(out, p.Folio+": "+what) }

	// Prospectos at each probabilidad, with what the follow-up says.
	type prospect struct {
		percent       int
		next, oc      *int // days from today
		kind, sku, qt string
		what          string
	}
	ptr := func(i int) *int { return &i }
	for i, pr := range []prospect{
		{10, ptr(7), nil, seedMetre, "PRUEBA-CCA-MTR", "800", "prospecto Inicial, seguimiento en una semana"},
		{25, ptr(14), ptr(45), seedPiece, "PRUEBA-ABL-H87", "40", "prospecto Baja, O.C. esperada en 45 días"},
		{50, ptr(0), ptr(20), seedCopper, "PRUEBA-CCS-MTR", "600", "prospecto Media, seguimiento hoy"},
		{75, ptr(3), ptr(10), seedCopper, "PRUEBA-CCS-KGM", "1500", "prospecto Alta (Pronóstico)"},
		{90, ptr(1), ptr(5), seedMetre, "PRUEBA-CCA-MTR", "2000", "prospecto Inminente (Pronóstico)"},
		{25, nil, nil, seedPiece, "PRUEBA-ABL-H87", "8", "prospecto sin fecha de seguimiento"},
	} {
		u := owner()
		cid, _ := customer(i)
		p, err := issue(u, pr.kind, cid, pr.sku, pr.qt)
		if err != nil {
			return nil, err
		}
		f := store.FollowUp{Probability: &pr.percent, Note: "Seguimiento de prueba."}
		if pr.next != nil {
			d := day(*pr.next)
			f.NextFollowUp = &d
		}
		if pr.oc != nil {
			d := day(*pr.oc)
			f.ExpectedOC = &d
		}
		if err := st.FollowUpProject(withSeedUser(ctx, u), p.ID, u.ID, f); err != nil {
			return nil, fmt.Errorf("follow-up %s: %w", p.Folio, err)
		}
		note(p, pr.what+" ("+u.Username+")")
	}

	// A prospecto whose quote was revised (R1), with a conversation in its history.
	{
		u := owner()
		cid, _ := customer(1)
		p, err := issue(u, seedMetre, cid, "PRUEBA-CCA-MTR", "1200")
		if err != nil {
			return nil, err
		}
		uctx := withSeedUser(ctx, u)
		if err := st.AddQuoteComment(uctx, p.CurrentQuoteID, u.ID, "El cliente pide mejor precio por volumen."); err != nil {
			return nil, err
		}
		rev, err := st.CreateRevision(uctx, p.CurrentQuoteID, u.ID)
		if err != nil {
			return nil, fmt.Errorf("revise %s: %w", p.Folio, err)
		}
		if err := seedEmit(ctx, st, u, rev, "PRUEBA-CCA-MTR", "1500"); err != nil {
			return nil, err
		}
		if p, err = st.ProjectByFolio(ctx, p.Folio); err != nil {
			return nil, err
		}
		if err := st.AddQuoteComment(uctx, p.CurrentQuoteID, u.ID, "Se mandó la revisión con 1,500 m."); err != nil {
			return nil, err
		}
		if err := st.SetProjectName(ctx, p.ID, "Ampliación bodega"); err != nil {
			return nil, err
		}
		note(p, "prospecto con cotización revisada (R1) y comentarios ("+u.Username+")")
	}

	// enCurso issues a quote and receives the client's O.C.; withPO also confirms the
	// supplier O.C., and delivered marks it delivered.
	folio := 10
	enCurso := func(i int, kind string, withPO, delivered bool, lines ...string) (*store.Project, *store.AuthenticatedUser, store.Customer, error) {
		u := owner()
		cid, c := customer(i)
		p, err := issue(u, kind, cid, lines...)
		if err != nil {
			return nil, nil, c, err
		}
		if err := seedReceiveOC(ctx, st, u, p, fmt.Sprintf("PRUEBA-OC-%d", 100+i), day(-30)); err != nil {
			return nil, nil, c, err
		}
		if withPO {
			if err := seedSupplierOC(ctx, pos, st, admin, p, e.suppliers[i%len(e.suppliers)]); err != nil {
				return nil, nil, c, err
			}
		}
		if delivered {
			list, err := st.ListProjectPurchaseOrders(ctx, p.ID)
			if err != nil {
				return nil, nil, c, err
			}
			for _, po := range list {
				if po.Status == store.POConfirmada {
					file := &store.NewFile{Filename: "entrega.pdf", ContentType: "application/pdf", Data: seedPDF}
					if err := st.MarkDelivered(withSeedUser(ctx, u), po.ID, u.ID, day(-7), file); err != nil {
						return nil, nil, c, fmt.Errorf("deliver %s: %w", p.Folio, err)
					}
				}
			}
		}
		p, err = st.ProjectByFolio(ctx, p.Folio)
		return p, u, c, err
	}
	actx := withSeedUser(ctx, admin)
	factura := func(p *store.Project, c store.Customer, metodo, paidOn string) (*store.Comprobante, error) {
		folio++
		id, err := st.RecordManualFactura(actx, p.ID, admin.ID, store.ManualFactura{XML: seedCFDI(c, p.Total, metodo, now.AddDate(0, 0, -20), folio), PaidOn: paidOn})
		if err != nil {
			return nil, fmt.Errorf("factura %s: %w", p.Folio, err)
		}
		return st.ComprobanteByID(ctx, id)
	}
	// complemento pays amount of factura f as payment number parcialidad, linked to the
	// payment already in Cobro when link isn't 0.
	complemento := func(f *store.Comprobante, c store.Customer, amount, saldo money.Centavos, parcialidad int, paidOn string, link int64) error {
		_, err := st.RecordManualComplemento(actx, f.ID, admin.ID, store.ManualComplemento{
			XML: seedCFDIPago(c, f.UUID, amount, saldo, parcialidad, paidOn), LinkPaymentID: link})
		if err != nil {
			return fmt.Errorf("complemento of %s: %w", f.UUID, err)
		}
		return nil
	}

	// En curso, delivered, not invoiced yet.
	p, u, _, err := enCurso(2, seedCopper, true, true, "PRUEBA-CCS-KGM", "800")
	if err != nil {
		return nil, err
	}
	note(p, "en curso, entregado, falta facturar ("+u.Username+")")

	// En curso, P.U.E. factura paid with it, not delivered.
	p, u, c, err := enCurso(3, seedPiece, true, false, "PRUEBA-ABL-H87", "30")
	if err != nil {
		return nil, err
	}
	if _, err := factura(p, c, "PUE", day(-15)); err != nil {
		return nil, err
	}
	note(p, "en curso, factura P.U.E. pagada, en proceso de entrega ("+u.Username+")")

	// En curso, P.P.D. factura with the anticipo's complemento: saldo pending.
	p, u, c, err = enCurso(0, seedMetre, true, true, "PRUEBA-CCA-MTR", "1000")
	if err != nil {
		return nil, err
	}
	half := p.Total / 2
	if err := st.RecordPayment(actx, p.ID, admin.ID, half, day(-25), nil); err != nil {
		return nil, err
	}
	f, err := factura(p, c, "PPD", "")
	if err != nil {
		return nil, err
	}
	pays, err := st.ListPayments(ctx, p.ID)
	if err != nil || len(pays) == 0 {
		return nil, fmt.Errorf("payments of %s: %v", p.Folio, err)
	}
	if err := complemento(f, c, half, p.Total, 1, day(-25), pays[0].ID); err != nil {
		return nil, err
	}
	note(p, "en curso, factura P.P.D. con un complemento (saldo pendiente) ("+u.Username+")")

	// En curso whose factura was cancelled: back to Pendiente de facturar.
	p, u, c, err = enCurso(4, seedPiece, false, false, "PRUEBA-ABL-H87", "15")
	if err != nil {
		return nil, err
	}
	f, err = factura(p, c, "PPD", "")
	if err != nil {
		return nil, err
	}
	if err := st.CancelManualFactura(actx, f.ID, admin.ID, "02", "Prueba: el RFC del cliente venía mal."); err != nil {
		return nil, fmt.Errorf("cancel factura of %s: %w", p.Folio, err)
	}
	note(p, "en curso, con una factura cancelada ("+u.Username+")")

	// Cerrado · Ganado with a P.U.E. factura.
	p, u, c, err = enCurso(1, seedCopper, true, true, "PRUEBA-CCS-MTR", "400")
	if err != nil {
		return nil, err
	}
	if _, err := factura(p, c, "PUE", day(-10)); err != nil {
		return nil, err
	}
	note(p, "cerrado ganado, factura P.U.E. ("+u.Username+")")

	// Cerrado · Ganado with a P.P.D. factura paid in two complementos.
	p, u, c, err = enCurso(2, seedMetre, true, true, "PRUEBA-CCA-MTR", "700")
	if err != nil {
		return nil, err
	}
	f, err = factura(p, c, "PPD", "")
	if err != nil {
		return nil, err
	}
	first := p.Total * 4 / 10
	if err := complemento(f, c, first, p.Total, 1, day(-12), 0); err != nil {
		return nil, err
	}
	if err := complemento(f, c, p.Total-first, p.Total-first, 2, day(-2), 0); err != nil {
		return nil, err
	}
	note(p, "cerrado ganado, factura P.P.D. pagada en dos complementos ("+u.Username+")")

	// Cancelado after the client's O.C.
	p, u, _, err = enCurso(3, seedMetre, false, false, "PRUEBA-CCA-MTR", "250")
	if err != nil {
		return nil, err
	}
	if err := st.CancelProject(actx, p.ID, admin.ID, "Prueba: el cliente canceló el pedido."); err != nil {
		return nil, fmt.Errorf("cancel %s: %w", p.Folio, err)
	}
	note(p, "cancelado después de la O.C. ("+u.Username+")")

	// Perdido from Media.
	u = owner()
	cid, _ := customer(0)
	if p, err = issue(u, seedCopper, cid, "PRUEBA-CCS-MTR", "300"); err != nil {
		return nil, err
	}
	if err := st.SetProjectProbability(withSeedUser(ctx, u), p.ID, u.ID, 50, ""); err != nil {
		return nil, err
	}
	if err := st.LoseProject(withSeedUser(ctx, u), p.ID, u.ID, store.StageProspecto, "Prueba: el proyecto del cliente se pospuso un año."); err != nil {
		return nil, fmt.Errorf("lose %s: %w", p.Folio, err)
	}
	note(p, "perdido ("+u.Username+")")

	// Pendientes for every owner, open and one done.
	for i, o := range owners {
		for j, pr := range []string{store.PriorityAlta, store.PriorityMedia} {
			id, err := st.AddPendiente(ctx, o.ID, fmt.Sprintf("Pendiente de prueba %d de %s", j+1, o.Name), pr)
			if err != nil {
				return nil, err
			}
			if i == 0 && j == 1 {
				if err := st.CompletePendiente(ctx, o.ID, id, now); err != nil {
					return nil, err
				}
			}
		}
	}
	return out, nil
}

// seedEmit issues a draft quote with SKU/quantity pairs, as the quote screen does.
func seedEmit(ctx context.Context, st *store.Store, u *store.AuthenticatedUser, quote *store.Quote, lines ...string) error {
	form, err := seedLinesForm(ctx, st, lines...)
	if err != nil {
		return err
	}
	if err := seedPost(ctx, u, NewQuotes(st, nil).Emitir, "/cotizaciones/"+quote.Folio+"/emitir", map[string]string{"folio": quote.Folio}, form); err != nil {
		return fmt.Errorf("issue %s: %w", quote.Folio, err)
	}
	return nil
}

func seedLinesForm(ctx context.Context, st *store.Store, lines ...string) (map[string][]string, error) {
	form := map[string][]string{}
	var keys []string
	for i := 0; i+1 < len(lines); i += 2 {
		p, err := st.ProductBySKU(ctx, lines[i])
		if err != nil || p == nil {
			return nil, fmt.Errorf("seed product %s: %v", lines[i], err)
		}
		k := strconv.Itoa(i / 2)
		keys = append(keys, k)
		form["lines["+k+"][kind]"] = []string{"product"}
		form["lines["+k+"][product_id]"] = []string{strconv.FormatInt(p.ID, 10)}
		form["lines["+k+"][qty]"] = []string{lines[i+1]}
	}
	form["line_keys"] = []string{strings.Join(keys, ",")}
	return form, nil
}

// seedCFDIPago is a stamped-looking complemento de pago from the SAT's test emisor to c,
// paying amount of factura facturaUUID as payment number parcialidad, saldo before it.
func seedCFDIPago(c store.Customer, facturaUUID string, amount, saldo money.Centavos, parcialidad int, day string) []byte {
	amt := func(x money.Centavos) string { return fmt.Sprintf("%d.%02d", int64(x)/100, int64(x)%100) }
	esc := strings.NewReplacer("&", "&amp;", `"`, "&quot;", "<", "&lt;", ">", "&gt;").Replace
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<cfdi:Comprobante xmlns:cfdi="http://www.sat.gob.mx/cfd/4" xmlns:pago20="http://www.sat.gob.mx/Pagos20" xmlns:tfd="http://www.sat.gob.mx/TimbreFiscalDigital" Version="4.0" Serie="PRUEBAP" Folio="` + strconv.Itoa(parcialidad) + `" Fecha="` + day + `T12:00:00" SubTotal="0" Moneda="XXX" Total="0" TipoDeComprobante="P" Exportacion="01" LugarExpedicion="42501" Sello="PRUEBA" NoCertificado="30001000000500003416">
  <cfdi:Emisor Rfc="EKU9003173C9" Nombre="ESCUELA KEMPER URGATE" RegimenFiscal="601"/>
  <cfdi:Receptor Rfc="` + c.RFC + `" Nombre="` + esc(c.LegalName) + `" DomicilioFiscalReceptor="` + c.PostalCode + `" RegimenFiscalReceptor="` + c.TaxRegime + `" UsoCFDI="CP01"/>
  <cfdi:Conceptos><cfdi:Concepto ClaveProdServ="84111506" Cantidad="1" ClaveUnidad="ACT" Descripcion="Pago" ValorUnitario="0" Importe="0" ObjetoImp="01"/></cfdi:Conceptos>
  <cfdi:Complemento>
    <pago20:Pagos Version="2.0">
      <pago20:Totales MontoTotalPagos="` + amt(amount) + `"/>
      <pago20:Pago FechaPago="` + day + `T12:00:00" FormaDePagoP="03" MonedaP="MXN" TipoCambioP="1" Monto="` + amt(amount) + `">
        <pago20:DoctoRelacionado IdDocumento="` + facturaUUID + `" MonedaDR="MXN" NumParcialidad="` + strconv.Itoa(parcialidad) + `" ImpSaldoAnt="` + amt(saldo) + `" ImpPagado="` + amt(amount) + `" ImpSaldoInsoluto="` + amt(saldo-amount) + `" ObjetoImpDR="01"/>
      </pago20:Pago>
    </pago20:Pagos>
    <tfd:TimbreFiscalDigital Version="1.1" UUID="` + seedUUID() + `" FechaTimbrado="` + day + `T12:01:00" RfcProvCertif="SPR190613I52" SelloCFD="PRUEBA" NoCertificadoSAT="30001000000500003456" SelloSAT="PRUEBA"/>
  </cfdi:Complemento>
</cfdi:Comprobante>`)
}
