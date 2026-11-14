package web

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/store"
)

// SeedPrefix marks every record SeedStaging adds: usernames, SKUs, customer and supplier
// names start with it, so test data is easy to tell from the copy of production's.
const SeedPrefix = "PRUEBA"

// seedUsers are the test users: two admins and three vendedores.
var seedUsers = []struct{ username, name, role string }{
	{"prueba-admin", "Prueba Administrador", store.RoleAdmin},
	{"prueba-admin2", "Prueba Administradora Dos", store.RoleAdmin},
	{"prueba-vend1", "Prueba Vendedor Uno", store.RoleVendedor},
	{"prueba-vend2", "Prueba Vendedora Dos", store.RoleVendedor},
	{"prueba-vend3", "Prueba Vendedor Tres", store.RoleVendedor},
}

// seedCustomers are receptores the SAT's sandbox accepts (checked against Facturoporti's
// test host on 2026-11-14), so staging can stamp facturas to them: two personas morales
// (601) and three personas físicas con actividad empresarial (612).
var seedCustomers = []store.Customer{
	{Name: SeedPrefix + " Indistria Iluminadora", RFC: "IIA040805DZ4", LegalName: "INDISTRIA ILUMINADORA DE ALMACENES",
		TaxRegime: "601", PostalCode: "62661", CFDIUse: "G01", ContactName: "Ana Prueba", Phone: "55 0000 0001", Email: "compras1@example.com"},
	{Name: SeedPrefix + " Xenon Industrial", RFC: "XIA190128J61", LegalName: "XENON INDUSTRIAL ARTICLES",
		TaxRegime: "601", PostalCode: "76343", CFDIUse: "G01", ContactName: "Beto Prueba", Phone: "55 0000 0002", Email: "compras2@example.com"},
	{Name: SeedPrefix + " Xochilt Casas", RFC: "CACX7605101P8", LegalName: "XOCHILT CASAS CHAVEZ",
		TaxRegime: "612", PostalCode: "36257", CFDIUse: "G03", ContactName: "Xochilt Casas", Phone: "55 0000 0003", Email: "compras3@example.com"},
	{Name: SeedPrefix + " Karla Fuente", RFC: "FUNK671228PH6", LegalName: "KARLA FUENTE NOLASCO",
		TaxRegime: "612", PostalCode: "01160", CFDIUse: "G03", ContactName: "Karla Fuente", Phone: "55 0000 0004", Email: "compras4@example.com"},
	{Name: SeedPrefix + " María Martínez", RFC: "MASO451221PM4", LegalName: "MARIA OLIVIA MARTINEZ SAGAZ",
		TaxRegime: "612", PostalCode: "80290", CFDIUse: "G01", ContactName: "María Martínez", Phone: "55 0000 0005", Email: "compras5@example.com"},
}

// Kinds of quote type the seed quotes with. Admins rename quote types, their prefixes
// and familias, so each is found by what it does (seedResolve), never by name.
const (
	seedCopper = "copper" // priced from the copper price, by weight (CCS)
	seedMetre  = "metre"  // cable by the metre, its products with a weight (CCA)
	seedPiece  = "piece"  // pieces, its products without a weight (luminarias)
)

// seedProduct is a test product copied from the first live product of its kind's quote
// type (its cost, weight, unit and materials, so it prices like the real ones), with
// SAT codes.
type seedProduct struct {
	kind, sku, description, satProduct, satUnit string
}

var seedProducts = []seedProduct{
	{seedCopper, "PRUEBA-CCS-KGM", "Cable CCS de prueba, se factura en kilos", "26121600", "KGM"},
	{seedCopper, "PRUEBA-CCS-MTR", "Cable CCS de prueba, se factura en metros", "26121600", "MTR"},
	{seedMetre, "PRUEBA-CCA-MTR", "Cable CCA de prueba", "26121600", "MTR"},
	{seedMetre, "PRUEBA-CCA-SINSAT", "Cable CCA de prueba sin datos para facturar", "", ""},
	{seedPiece, "PRUEBA-ABL-H87", "Luminaria de prueba", "39111600", "H87"},
}

// seedKind is a quote type of one kind and the live product its test products copy.
type seedKind struct {
	typeID int64
	base   store.Product
}

// seedResolve finds, for each kind, the first quote type of that kind with a product to
// copy: copper, the type priced from the copper price (a product with materials);
// metre and piece, plain types (not free lines, not by weight) with a priced product
// with or without a weight.
func seedResolve(ctx context.Context, st *store.Store) (map[string]seedKind, error) {
	types, err := st.ListQuoteTypes(ctx)
	if err != nil {
		return nil, err
	}
	list, err := st.ListProducts(ctx, "", "sku", "", nil)
	if err != nil {
		return nil, err
	}
	out := map[string]seedKind{}
	for _, kind := range []string{seedCopper, seedMetre, seedPiece} {
		for _, t := range types {
			if t.FamilyID == nil || t.FreeLinesOnly || t.KgPriced || t.CopperPriceMargin != (kind == seedCopper) {
				continue
			}
			for _, p := range list {
				if p.FamilyID != *t.FamilyID || strings.HasPrefix(p.SKU, SeedPrefix) {
					continue
				}
				ok := false
				switch kind {
				case seedCopper:
					ok = p.HasMaterials
				case seedMetre:
					ok = p.CostMicros != nil && p.KgPerMMicros != nil
				case seedPiece:
					ok = p.CostMicros != nil && p.KgPerMMicros == nil
				}
				if ok {
					out[kind] = seedKind{typeID: t.ID, base: p}
					break
				}
			}
			if _, found := out[kind]; found {
				break
			}
		}
		if _, found := out[kind]; !found {
			return nil, fmt.Errorf("no quote type with a product to copy for %s", kind)
		}
	}
	return out, nil
}

// seedDoneKey is the setting SeedStaging writes last: a database that has it is seeded.
const seedDoneKey = "seed_staging_at"

// SeedStaging adds a small, useful set of test data to a staging database that is a copy
// of production's, so it becomes a superset: five users, customers (receptores valid in
// the SAT's sandbox), suppliers and products (with and without SAT codes, in kilos and
// metres), and proyectos in the states worth trying (prospecto with an overdue follow-up,
// Inminente, en curso with a confirmed supplier O.C. ready to invoice in kilos, en curso
// with a P.P.D. factura recorded by hand and an anticipo, Perdido) plus a draft quote.
// Everything goes through the same code the screens use, quotes priced as they would be.
//
// It does nothing when the test users already exist. It never runs on production: the
// caller checks APP_ENV. The test users' password is generated and written to out.
func SeedStaging(ctx context.Context, st *store.Store, now time.Time, out io.Writer) error {
	if done, err := st.SettingValues(ctx, []string{seedDoneKey}); err != nil {
		return err
	} else if done[seedDoneKey] != "" {
		fmt.Fprintln(out, "staging ya tiene los datos de prueba (desde "+done[seedDoneKey]+"); no se agregó nada")
		return nil
	}

	password, err := seedPassword()
	if err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return err
	}
	users := make([]*store.AuthenticatedUser, len(seedUsers))
	for i, u := range seedUsers {
		var id int64
		if existing, err := st.UserByUsername(ctx, u.username); err != nil {
			return err
		} else if existing != nil { // left by a seed that failed halfway
			id = existing.ID
			if err := st.UpdatePassword(ctx, id, string(hash)); err != nil {
				return err
			}
		} else if id, err = st.CreateUser(ctx, u.username, u.name, string(hash), u.role); err != nil {
			return fmt.Errorf("seed user %s: %w", u.username, err)
		}
		users[i] = &store.AuthenticatedUser{ID: id, Username: u.username, Name: u.name, Role: u.role}
	}
	admin, vend1, vend2, vend3 := users[0], users[2], users[3], users[4]
	actx := store.WithActor(ctx, store.Actor{UserID: admin.ID, Source: store.SourceCLI})

	existingCustomers, err := st.ListCustomers(ctx, SeedPrefix, "", "", nil)
	if err != nil {
		return err
	}
	customers := make([]int64, len(seedCustomers))
	for i, c := range seedCustomers {
		for _, e := range existingCustomers {
			if e.Name == c.Name {
				customers[i] = e.ID
			}
		}
		if customers[i] == 0 {
			if customers[i], err = st.CreateCustomer(actx, c); err != nil {
				return fmt.Errorf("seed customer %s: %w", c.Name, err)
			}
		}
	}
	suppliers := make([]int64, 5)
	for i := range suppliers {
		n := strconv.Itoa(i + 1)
		if id, err := st.SupplierByName(ctx, SeedPrefix+" Proveedor "+n); err != nil {
			return err
		} else if id != nil {
			suppliers[i] = *id
			continue
		}
		if suppliers[i], err = st.CreateSupplier(actx, store.Supplier{Name: SeedPrefix + " Proveedor " + n, RFC: "EKU9003173C9",
			LegalName: "PROVEEDOR DE PRUEBA " + n, Street: "Calle de Prueba", ExtNumber: n, Neighborhood: "Centro",
			Municipality: "Monterrey", State: "Nuevo León", PostalCode: "64000", TaxRegime: "601"}); err != nil {
			return fmt.Errorf("seed supplier %s: %w", n, err)
		}
	}
	kinds, err := seedResolve(ctx, st)
	if err != nil {
		return err
	}
	products := map[string]int64{}
	for _, sp := range seedProducts {
		if p, err := st.ProductBySKU(ctx, sp.sku); err != nil {
			return err
		} else if p != nil {
			products[sp.sku] = p.ID
			continue
		}
		id, err := seedCopyProduct(actx, st, sp, kinds[sp.kind].base)
		if err != nil {
			return fmt.Errorf("seed product %s: %w", sp.sku, err)
		}
		products[sp.sku] = id
	}

	q := NewQuotes(st, nil)
	pos := NewPurchaseOrders(st)
	// issue creates and issues a quote with lines, pairs of SKU and quantity in order.
	issue := func(u *store.AuthenticatedUser, kind string, customer int64, lines ...string) (*store.Project, error) {
		quote, err := st.CreateDraftQuoteOfType(withSeedUser(ctx, u), customer, u.ID, kinds[kind].typeID)
		if err != nil {
			return nil, err
		}
		form := url.Values{}
		var keys []string
		for i := 0; i+1 < len(lines); i += 2 {
			k := strconv.Itoa(i / 2)
			keys = append(keys, k)
			form.Set("lines["+k+"][kind]", "product")
			form.Set("lines["+k+"][product_id]", strconv.FormatInt(products[lines[i]], 10))
			form.Set("lines["+k+"][qty]", lines[i+1])
		}
		form.Set("line_keys", strings.Join(keys, ","))
		if err := seedPost(ctx, u, q.Emitir, "/cotizaciones/"+quote.Folio+"/emitir", map[string]string{"folio": quote.Folio}, form); err != nil {
			return nil, fmt.Errorf("issue %s: %w", quote.Folio, err)
		}
		return st.ProjectByFolio(ctx, quote.Folio)
	}
	today := now.Format("2006-01-02")
	var folios []string

	// 1. Prospecto whose follow-up was yesterday (avisos A/B).
	p1, err := issue(vend1, seedMetre, customers[0], "PRUEBA-CCA-MTR", "500")
	if err != nil {
		return err
	}
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
	if err := st.FollowUpProject(withSeedUser(ctx, vend1), p1.ID, vend1.ID, store.FollowUp{NextFollowUp: &yesterday,
		Note: "Prospecto de prueba con seguimiento vencido."}); err != nil {
		return fmt.Errorf("follow-up %s: %w", p1.Folio, err)
	}
	folios = append(folios, p1.Folio+": prospecto con seguimiento vencido")

	// 2. Prospecto at Inminente, in the Pronóstico.
	p2, err := issue(vend2, seedCopper, customers[1], "PRUEBA-CCS-MTR", "1000")
	if err != nil {
		return err
	}
	if err := st.SetProjectProbability(withSeedUser(ctx, vend2), p2.ID, vend2.ID, 90, "Prospecto de prueba a punto de cerrar."); err != nil {
		return err
	}
	folios = append(folios, p2.Folio+": prospecto Inminente (Pronóstico)")

	// 3. En curso with its client O.C. and a confirmed supplier O.C., to invoice in kilos
	// (stamp it in the sandbox).
	p3, err := issue(vend1, seedCopper, customers[1], "PRUEBA-CCS-KGM", "1000", "PRUEBA-CCS-MTR", "250")
	if err != nil {
		return err
	}
	if err := seedReceiveOC(ctx, st, vend1, p3, "PRUEBA-OC-3", today); err != nil {
		return err
	}
	if err := seedSupplierOC(ctx, pos, st, admin, p3, suppliers[0]); err != nil {
		return err
	}
	folios = append(folios, p3.Folio+": en curso, O.C. al proveedor confirmada, lista para facturar (una partida en KGM)")

	// 4. En curso with an anticipo and a P.P.D. factura recorded by hand: Pendiente
	// complemento de pago, with the complemento's deadline running.
	p4, err := issue(vend3, seedMetre, customers[3], "PRUEBA-CCA-MTR", "300", "PRUEBA-CCA-SINSAT", "200")
	if err != nil {
		return err
	}
	if err := seedReceiveOC(ctx, st, vend3, p4, "PRUEBA-OC-4", today); err != nil {
		return err
	}
	if p4, err = st.ProjectByFolio(ctx, p4.Folio); err != nil {
		return err
	}
	if err := st.RecordPayment(withSeedUser(ctx, admin), p4.ID, admin.ID, p4.Total/2, today, nil); err != nil {
		return fmt.Errorf("anticipo %s: %w", p4.Folio, err)
	}
	if _, err := st.RecordManualFactura(withSeedUser(ctx, admin), p4.ID, admin.ID, store.ManualFactura{
		XML: seedCFDI(seedCustomers[3], p4.Total, "PPD", now)}); err != nil {
		return fmt.Errorf("factura %s: %w", p4.Folio, err)
	}
	folios = append(folios, p4.Folio+": en curso, factura P.P.D. registrada a mano y anticipo del 50% (falta complemento)")

	// 5. Perdido.
	p5, err := issue(vend2, seedPiece, customers[4], "PRUEBA-ABL-H87", "12")
	if err != nil {
		return err
	}
	if err := st.LoseProject(withSeedUser(ctx, vend2), p5.ID, vend2.ID, store.StageProspecto, "Prueba: el cliente eligió otro proveedor por precio."); err != nil {
		return fmt.Errorf("lose %s: %w", p5.Folio, err)
	}
	folios = append(folios, p5.Folio+": perdido")

	// A draft quote, never issued.
	draft, err := st.CreateDraftQuoteOfType(withSeedUser(ctx, vend3), customers[2], vend3.ID, kinds[seedMetre].typeID)
	if err != nil {
		return err
	}
	folios = append(folios, draft.Folio+": cotización en borrador")

	if err := st.SetSetting(actx, seedDoneKey, now.Format(time.RFC3339), admin.ID); err != nil {
		return err
	}
	fmt.Fprintln(out, "Datos de prueba agregados a staging.")
	fmt.Fprintln(out, "Usuarios (contraseña para todos: "+password+"):")
	for _, u := range seedUsers {
		fmt.Fprintf(out, "  %s (%s)\n", u.username, u.role)
	}
	fmt.Fprintln(out, "Clientes: 5, con RFC de prueba del SAT. Proveedores: 5. Productos: "+strings.Join(seedProductSKUs(), ", ")+".")
	fmt.Fprintln(out, "Proyectos:")
	for _, f := range folios {
		fmt.Fprintln(out, "  "+f)
	}
	return nil
}

func seedProductSKUs() []string {
	var out []string
	for _, p := range seedProducts {
		out = append(out, p.sku)
	}
	return out
}

// seedPassword is 16 random letters and digits.
func seedPassword() (string, error) {
	const alphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 16)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		b[i] = alphabet[n.Int64()]
	}
	return string(b), nil
}

// withSeedUser is ctx as a request from u would carry it: the user and the audit actor.
func withSeedUser(ctx context.Context, u *store.AuthenticatedUser) context.Context {
	ctx = context.WithValue(ctx, userContextKey, u)
	return store.WithActor(ctx, store.Actor{UserID: u.ID, Source: store.SourceCLI})
}

// seedPost runs a handler on a form POST as u, in-process; anything but a redirect is an
// error carrying the response.
func seedPost(ctx context.Context, u *store.AuthenticatedUser, h http.HandlerFunc, path string, pathValues map[string]string, form url.Values) error {
	_, err := seedPostLocation(ctx, u, h, path, pathValues, form)
	return err
}

func seedPostLocation(ctx context.Context, u *store.AuthenticatedUser, h http.HandlerFunc, path string, pathValues map[string]string, form url.Values) (string, error) {
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode())).WithContext(withSeedUser(ctx, u))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range pathValues {
		req.SetPathValue(k, v)
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		body := rec.Body.String()
		if len(body) > 300 {
			body = body[:300]
		}
		return "", fmt.Errorf("POST %s = %d: %s", path, rec.Code, strings.TrimSpace(body))
	}
	return rec.Header().Get("Location"), nil
}

// seedCopyProduct creates sp as a copy of base (its materials too), with sp's SKU,
// description and SAT codes.
func seedCopyProduct(ctx context.Context, st *store.Store, sp seedProduct, b store.Product) (int64, error) {
	base := &b
	np := *base
	np.ID, np.SKU, np.Description, np.SATProductCode, np.SATUnitCode = 0, sp.sku, sp.description, sp.satProduct, sp.satUnit
	id, err := st.CreateProduct(ctx, np)
	if err != nil {
		return 0, err
	}
	materials, err := st.ListProductMaterials(ctx, base.ID)
	if err != nil {
		return 0, err
	}
	for _, m := range materials {
		if _, err := st.AddProductMaterial(ctx, id, m.Material.ID, m.QtyPerUnitMicros); err != nil {
			return 0, err
		}
	}
	return id, nil
}

// seedReceiveOC moves a prospecto en curso with the client's O.C.
func seedReceiveOC(ctx context.Context, st *store.Store, u *store.AuthenticatedUser, p *store.Project, number, day string) error {
	uctx := withSeedUser(ctx, u)
	if err := st.SetProjectProbability(uctx, p.ID, u.ID, 90, ""); err != nil {
		return err
	}
	file := &store.NewFile{Filename: number + ".pdf", ContentType: "application/pdf", Data: seedPDF}
	if err := st.ReceiveOC(uctx, p.ID, u.ID, store.OC{Number: number, Date: day}, file, ""); err != nil {
		return fmt.Errorf("receive O.C. %s: %w", p.Folio, err)
	}
	return nil
}

// seedPDF is a minimal PDF for the files the seed attaches.
var seedPDF = []byte("%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj 2 0 obj<</Type/Pages/Kids[]/Count 0>>endobj\ntrailer<</Root 1 0 R>>\n%%EOF\n")

// seedSupplierOC generates the proyecto's supplier O.C. and confirms it with supplier.
func seedSupplierOC(ctx context.Context, pos *PurchaseOrders, st *store.Store, admin *store.AuthenticatedUser, p *store.Project, supplier int64) error {
	pv := map[string]string{"folio": p.Folio}
	loc, err := seedPostLocation(ctx, admin, pos.Generar, "/proyectos/"+p.Folio+"/ordenes-de-compra", pv, url.Values{})
	if err != nil {
		return fmt.Errorf("supplier O.C. of %s: %w", p.Folio, err)
	}
	id := loc[strings.LastIndex(loc, "/")+1:]
	if i := strings.IndexAny(id, "?#"); i >= 0 {
		id = id[:i]
	}
	lines, err := st.ListQuoteLines(ctx, p.CurrentQuoteID)
	if err != nil {
		return err
	}
	form := url.Values{"proveedor": {strconv.FormatInt(supplier, 10)}, "moneda": {"MXN"}, "forma_pago": {store.POPayTransfer}, "uso_cfdi": {"G01"},
		"domicilio": {"Almacén de prueba, Monterrey, N.L."}}
	for _, l := range lines {
		form.Add("partida", strconv.Itoa(l.LineNo))
	}
	if err := seedPost(ctx, admin, pos.Confirmar, "/proyectos/"+p.Folio+"/ordenes-de-compra/"+id+"/confirmar",
		map[string]string{"folio": p.Folio, "id": id}, form); err != nil {
		return fmt.Errorf("confirm supplier O.C. of %s: %w", p.Folio, err)
	}
	return nil
}

// seedCFDI is a stamped-looking CFDI 4.0 factura from the SAT's test emisor to c for
// total, as if issued in the PAC's portal, for recording by hand in staging. Its folio
// fiscal is random; nothing about it is valid outside staging.
func seedCFDI(c store.Customer, total money.Centavos, metodo string, now time.Time) []byte {
	sub := money.Centavos(money.RoundHalfUp(int64(total)*100, 116))
	amt := func(x money.Centavos) string { return fmt.Sprintf("%d.%02d", int64(x)/100, int64(x)%100) }
	forma := "03"
	if metodo == "PPD" {
		forma = "99"
	}
	stamp := now.Format("2006-01-02T15:04:05")
	esc := func(s string) string {
		return strings.NewReplacer("&", "&amp;", `"`, "&quot;", "<", "&lt;", ">", "&gt;").Replace(s)
	}
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<cfdi:Comprobante xmlns:cfdi="http://www.sat.gob.mx/cfd/4" xmlns:tfd="http://www.sat.gob.mx/TimbreFiscalDigital" Version="4.0" Serie="PRUEBA" Folio="` + strconv.FormatInt(now.Unix()%100000, 10) + `" Fecha="` + stamp + `" FormaPago="` + forma + `" SubTotal="` + amt(sub) + `" Moneda="MXN" Total="` + amt(total) + `" TipoDeComprobante="I" Exportacion="01" MetodoPago="` + metodo + `" LugarExpedicion="42501" Sello="PRUEBA" NoCertificado="30001000000500003416">
  <cfdi:Emisor Rfc="EKU9003173C9" Nombre="ESCUELA KEMPER URGATE" RegimenFiscal="601"/>
  <cfdi:Receptor Rfc="` + c.RFC + `" Nombre="` + esc(c.LegalName) + `" DomicilioFiscalReceptor="` + c.PostalCode + `" RegimenFiscalReceptor="` + c.TaxRegime + `" UsoCFDI="` + c.CFDIUse + `"/>
  <cfdi:Conceptos>
    <cfdi:Concepto ClaveProdServ="26121600" Cantidad="1" ClaveUnidad="H87" Unidad="Pieza" Descripcion="Factura de prueba registrada a mano" ValorUnitario="` + amt(sub) + `" Importe="` + amt(sub) + `" ObjetoImp="02"/>
  </cfdi:Conceptos>
  <cfdi:Impuestos TotalImpuestosTrasladados="` + amt(total-sub) + `"/>
  <cfdi:Complemento>
    <tfd:TimbreFiscalDigital Version="1.1" UUID="` + seedUUID() + `" FechaTimbrado="` + stamp + `" RfcProvCertif="SPR190613I52" SelloCFD="PRUEBA" NoCertificadoSAT="30001000000500003456" SelloSAT="PRUEBA"/>
  </cfdi:Complemento>
</cfdi:Comprobante>`)
}

func seedUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return strings.ToUpper(fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]))
}

