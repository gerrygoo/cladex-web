package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gerrygoo/cladex-web/internal/money"
)

func ptrStr(s string) *string { return &s }

func catalogLine(no int, desc string, qty money.Milli, inputs string) QuoteLine {
	pid := int64(no)
	return QuoteLine{LineNo: no, ProductID: &pid, DescriptionSnapshot: desc, QtyMilli: qty, PricingInputsJSON: ptrStr(inputs)}
}

// LineFromQuote takes the cost the quote froze, never the live catalog, and rounds the
// unit price to the centavo the way the O.C. prints it.
func TestLineFromQuote(t *testing.T) {
	cost := money.Micros(12_345_678)
	for _, c := range []struct {
		name     string
		line     QuoteLine
		byWeight bool
		ok       bool
		unit     money.Micros
		total    money.Centavos
		kg       *money.Micros
		perKg    *money.Micros
	}{
		{name: "CCA, flat cost", line: catalogLine(1, "CCA 14", 200_000, `{"margin":"0.3","cost":"5.54"}`),
			ok: true, unit: 5_540_000, total: 110_800},
		{name: "a flat cost to the micro rounds to the centavo", line: catalogLine(2, "x", 3_000, `{"margin":"0.3","cost":"7.765"}`),
			ok: true, unit: 7_770_000, total: 2_331},
		// The issue's Alumoclad 7#8: 95.00/kg × 0.3896 kg/m = 37.012 → $37.01 per metre;
		// 111 m × $37.01 = $4,108.11 (not 111 × 37.012 = 4,108.33).
		{name: "Alumoclad, cost per kg", line: catalogLine(3, "ALC-C7#8", 111_000, `{"margin":"0.1","cost":"95","kg_per_unit":"0.3896"}`),
			byWeight: true, ok: true, unit: 37_010_000, total: 410_811,
			kg: mp(389_600), perKg: mp(95_000_000)},
		{name: "CCS, one kg material", line: catalogLine(4, "CCS 30% ALAMBRE 4", 1_000, `{"margin":"0.2955","materials":[{"material":"CCS 30%","qty_per_unit":"0.1723","unit":"kg","price":"160"}]}`),
			byWeight: true, ok: true, unit: 27_570_000, total: 2_757,
			kg: mp(172_300), perKg: mp(160_000_000)},
		{name: "a cost that isn't all per kg prints no price per kg", line: catalogLine(5, "mixto", 1_000, `{"margin":"0.2","cost":"2","materials":[{"material":"CCS 30%","qty_per_unit":"0.1723","unit":"kg","price":"160"}]}`),
			byWeight: true, ok: true, unit: 29_570_000, total: 2_957, kg: mp(172_300)},
		{name: "per piece, no kg columns", line: catalogLine(6, "pieza", 2_000, `{"margin":"0.2","materials":[{"material":"CCS 30%","qty_per_unit":"0.1723","unit":"kg","price":"160"}]}`),
			ok: true, unit: 27_570_000, total: 5_514},
		{name: "free line with its cost", line: QuoteLine{LineNo: 7, DescriptionSnapshot: "Poste", QtyMilli: 1_000, CostMicros: &cost},
			ok: true, unit: 12_350_000, total: 1_235},
		{name: "free line without a cost", line: QuoteLine{LineNo: 8, DescriptionSnapshot: "Flete", QtyMilli: 1_000}},
		{name: "catalog line without inputs", line: QuoteLine{LineNo: 9, ProductID: new(int64), QtyMilli: 1_000}},
		{name: "catalog line with no cost", line: catalogLine(10, "x", 1_000, `{"margin":"0.2"}`)},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, ok := LineFromQuote(c.line, c.byWeight)
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v", ok, c.ok)
			}
			if !ok {
				return
			}
			if got.UnitPriceMicros != c.unit || got.LineTotal != c.total || got.QuoteLineNo != c.line.LineNo {
				t.Errorf("unit %v total %v (partida %d), want %v %v", got.UnitPriceMicros, got.LineTotal, got.QuoteLineNo, c.unit, c.total)
			}
			if !sameMicros(got.KgPerUnit, c.kg) || !sameMicros(got.PricePerKg, c.perKg) {
				t.Errorf("kg %v per kg %v, want %v %v", got.KgPerUnit, got.PricePerKg, c.kg, c.perKg)
			}
		})
	}
}

func mp(n int64) *money.Micros { m := money.Micros(n); return &m }

func sameMicros(a, b *money.Micros) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

// The issue's acceptance example, QA0037 to Cable Anti Robo: six partidas of 200 m.
func TestPOTotalsQA0037(t *testing.T) {
	var lines []PurchaseOrderLine
	for i, cost := range []string{"5.54", "5.54", "7.77", "7.77", "12.30", "12.30"} {
		l, ok := LineFromQuote(catalogLine(i+1, "CABLE", 200_000, `{"margin":"0.3","cost":"`+cost+`"}`), false)
		if !ok {
			t.Fatal("line not ok")
		}
		lines = append(lines, l)
	}
	tot := POTotals(lines)
	if tot.Subtotal.String() != "$10,244.00" || tot.IVA.String() != "$1,639.04" || tot.Total.String() != "$11,883.04" {
		t.Errorf("totals = %s, %s, %s", tot.Subtotal, tot.IVA, tot.Total)
	}
}

func TestPOConditions(t *testing.T) {
	days := int64(30)
	got := strings.Join(POConditions("MXN", POPayTransfer, nil, ""), "|")
	want := "Precios en pesos mexicanos (MXN), más IVA.|Forma de pago: transferencia electrónica, por adelantado conforme a la cotización de referencia.|" +
		"La mercancía se recibe sujeta a revisión de cantidad y calidad.|Favor de emitir el CFDI con los datos de facturación indicados en esta orden."
	if got != want {
		t.Errorf("default conditions = %s", got)
	}
	for _, c := range []struct {
		cur, form, other string
		days             *int64
		want             string
	}{
		{"USD", POPayCash, "", nil, "Precios en dólares americanos (USD), más IVA.|Forma de pago: de contado, contra entrega."},
		{"MXN", POPayCredit, "", &days, "Forma de pago: crédito a 30 días."},
		{"MXN", POPayOther, " 50% anticipo y 50% contra entrega. ", nil, "|50% anticipo y 50% contra entrega.|"},
	} {
		if got := strings.Join(POConditions(c.cur, c.form, c.days, c.other), "|"); !strings.Contains(got, c.want) {
			t.Errorf("%s/%s: %s lacks %s", c.cur, c.form, got, c.want)
		}
	}
}

// poFixture is a proyecto with its client's O.C. received, whose quote has two free
// lines with their cost.
func poFixture(t *testing.T, s *Store) (q *Quote, userID int64) {
	t.Helper()
	ctx := context.Background()
	q, userID = projectFixture(t, s)
	for i, cost := range []int64{5_540_000, 7_770_000} {
		if _, err := s.db.Exec(`INSERT INTO quote_lines (quote_id, line_no, description_snapshot, qty_milli, unit_price_micros, line_total, cost_micros)
			VALUES (?, ?, 'Cable', 200000, 1, 1, ?)`, q.ID, i+1, cost); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.ReceiveOC(ctx, *q.ProjectID, userID, testOC, nil, ""); err != nil {
		t.Fatalf("ReceiveOC: %v", err)
	}
	return q, userID
}

func TestPurchaseOrderLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	q, userID := projectFixture(t, s)
	projectID := *q.ProjectID
	sup := seededSupplier(t, s)
	line, _ := LineFromQuote(catalogLine(1, "CCA 14", 200_000, `{"cost":"5.54"}`), false)
	draft := PODraft{SupplierID: &sup.ID, Lines: []PurchaseOrderLine{line}}

	// Not before the client's O.C.
	if _, err := s.CreatePurchaseOrder(ctx, projectID, q.ID, userID, false, draft); !errors.Is(err, ErrPONotAllowed) {
		t.Fatalf("O.C. on a prospecto: %v", err)
	}
	if err := s.ReceiveOC(ctx, projectID, userID, testOC, nil, ""); err != nil {
		t.Fatal(err)
	}
	id, err := s.CreatePurchaseOrder(ctx, projectID, q.ID, userID, false, draft)
	if err != nil {
		t.Fatalf("CreatePurchaseOrder: %v", err)
	}
	// One per proyecto outside QL.
	if _, err := s.CreatePurchaseOrder(ctx, projectID, q.ID, userID, false, draft); !errors.Is(err, ErrPONotAllowed) {
		t.Fatalf("second O.C. on QA: %v", err)
	}
	po, err := s.PurchaseOrderByID(ctx, id)
	if err != nil || po == nil {
		t.Fatal(err)
	}
	if po.Status != POBorrador || po.Number != "" || po.Currency != "MXN" || po.PaymentForm != POPayTransfer ||
		po.Total.String() != "$1,285.28" || po.SupplierName != "Cable Anti Robo" || po.Label() != "Borrador" {
		t.Fatalf("new draft = %+v", po)
	}

	days := int64(45)
	draft.DeliveryAddress, draft.PaymentForm, draft.CreditDays, draft.Currency = " Bodega Norte ", POPayCredit, &days, "USD"
	if err := s.SavePurchaseOrderDraft(ctx, id, draft); err != nil {
		t.Fatal(err)
	}
	po, _ = s.PurchaseOrderByID(ctx, id)
	if po.DeliveryAddress != "Bodega Norte" || *po.CreditDays != 45 || po.Currency != "USD" {
		t.Fatalf("saved draft = %+v", po)
	}

	// Late on 30/10 in México is already the 31st in UTC: the number takes México's day.
	day := time.Date(2026, 10, 31, 3, 0, 0, 0, time.UTC)
	number, seq, err := s.PONumber(ctx, *po, day)
	if err != nil || number != "OC-QA0001-301026" || seq != 1 {
		t.Fatalf("PONumber = %q, %d, %v", number, seq, err)
	}
	conf := POConfirmation{Number: number, Seq: seq, ConfirmedAt: "2026-10-31T03:00:00Z", Document: "{}",
		PDFSHA256: "abc", UserID: userID, HistoryNote: "Orden de compra " + number + " confirmada."}
	if err := s.ConfirmPurchaseOrder(ctx, id, draft, conf); err != nil {
		t.Fatalf("ConfirmPurchaseOrder: %v", err)
	}
	if err := s.SavePurchaseOrderDraft(ctx, id, draft); !errors.Is(err, ErrPONotDraft) {
		t.Fatalf("saving a confirmed O.C.: %v", err)
	}
	if err := s.ConfirmPurchaseOrder(ctx, id, draft, conf); !errors.Is(err, ErrPONotDraft) {
		t.Fatalf("confirming twice: %v", err)
	}
	if err := s.DeletePurchaseOrderDraft(ctx, id); !errors.Is(err, ErrPONotDraft) {
		t.Fatalf("deleting a confirmed O.C.: %v", err)
	}

	// A revision: needs a motivo, keeps the number's date, and replaces the original
	// only once confirmed.
	if _, err := s.CreatePurchaseOrderRevision(ctx, id, userID, "  "); !errors.Is(err, ErrPONotAllowed) {
		t.Fatalf("revision without a motivo: %v", err)
	}
	revID, err := s.CreatePurchaseOrderRevision(ctx, id, userID, "Se corrigió la dirección de entrega")
	if err != nil {
		t.Fatalf("CreatePurchaseOrderRevision: %v", err)
	}
	if _, err := s.CreatePurchaseOrderRevision(ctx, id, userID, "otra"); !errors.Is(err, ErrPONotAllowed) {
		t.Fatalf("a second revision in borrador: %v", err)
	}
	rev, _ := s.PurchaseOrderByID(ctx, revID)
	orig, _ := s.PurchaseOrderByID(ctx, id)
	revLines, _ := s.PurchaseOrderLines(ctx, revID)
	if rev.Status != POBorrador || rev.Revision != 1 || rev.Seq != 1 || rev.DeliveryAddress != "Bodega Norte" ||
		len(revLines) != 1 || rev.Label() != "Borrador de revisión de OC-QA0001-301026" ||
		orig.Status != POConfirmada || orig.PendingRevisionID != revID {
		t.Fatalf("revision %+v / original %+v", rev, orig)
	}
	comments, _ := s.ListProjectComments(ctx, projectID)
	if !strings.Contains(comments[0].Body, "Revisión de la orden de compra OC-QA0001-301026.\nMotivo: Se corrigió la dirección de entrega") {
		t.Fatalf("history = %q", comments[0].Body)
	}
	number, seq, err = s.PONumber(ctx, *rev, time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC))
	if err != nil || number != "OC-QA0001-301026-R1" || seq != 1 {
		t.Fatalf("revision number = %q, %d, %v", number, seq, err)
	}
	draft.DeliveryAddress = "Bodega Sur"
	if err := s.ConfirmPurchaseOrder(ctx, revID, draft, POConfirmation{Number: number, Seq: seq,
		ConfirmedAt: "2026-11-05T12:00:00Z", Document: "{}", PDFSHA256: "def", UserID: userID}); err != nil {
		t.Fatalf("confirm revision: %v", err)
	}
	orig, _ = s.PurchaseOrderByID(ctx, id)
	rev, _ = s.PurchaseOrderByID(ctx, revID)
	if orig.Status != POReemplazada || orig.ReplacedByNumber != number || rev.Status != POConfirmada ||
		rev.SupersedesNumber != "OC-QA0001-301026" || rev.DeliveryAddress != "Bodega Sur" {
		t.Fatalf("after revision: original %+v, revision %+v", orig, rev)
	}
	// A revision of the revision is R2 of the same base.
	rev2ID, err := s.CreatePurchaseOrderRevision(ctx, revID, userID, "Cambio de proveedor")
	if err != nil {
		t.Fatal(err)
	}
	rev2, _ := s.PurchaseOrderByID(ctx, rev2ID)
	if number, _, _ := s.PONumber(ctx, *rev2, time.Now()); number != "OC-QA0001-301026-R2" {
		t.Fatalf("second revision number = %q", number)
	}
	// Discarding the revision's borrador leaves the confirmed one as it was.
	if err := s.DeletePurchaseOrderDraft(ctx, rev2ID); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListProjectPurchaseOrders(ctx, projectID)
	if err != nil || len(list) != 2 || list[0].ID != id || list[1].ID != revID || list[1].PendingRevisionID != 0 {
		t.Fatalf("list = %+v, %v", list, err)
	}

	// The supplier can't be deleted while it has O.C.s; the proyecto still can.
	if err := s.UpdateSupplier(ctx, Supplier{ID: sup.ID, Name: sup.Name, RFC: sup.RFC, LegalName: sup.LegalName,
		Street: sup.Street, ExtNumber: sup.ExtNumber, Neighborhood: sup.Neighborhood,
		Municipality: sup.Municipality, State: sup.State, PostalCode: sup.PostalCode}); err != nil {
		t.Fatal(err)
	}
	var inUse *SupplierInUseError
	if err := s.DeleteSupplier(ctx, sup.ID); !errors.As(err, &inUse) || len(inUse.Familias) != 0 ||
		strings.Join(inUse.OrdenesCompra, ",") != "OC-QA0001-301026,OC-QA0001-301026-R1" {
		t.Fatalf("DeleteSupplier with O.C.s: %v %+v", err, inUse)
	}
	if err := s.DeleteProject(ctx, projectID); err != nil {
		t.Fatalf("DeleteProject with O.C.s: %v", err)
	}
	if n := rowCount(t, s, "purchase_orders") + rowCount(t, s, "purchase_order_lines"); n != 0 {
		t.Fatalf("%d O.C. rows left", n)
	}
	if err := s.DeleteSupplier(ctx, sup.ID); err != nil {
		t.Fatalf("DeleteSupplier once its O.C.s are gone: %v", err)
	}
}

// QL takes several O.C.s, numbered -2, -3, each with its own partidas.
func TestPurchaseOrdersQLNumbering(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	q, userID := poFixture(t, s)
	projectID := *q.ProjectID
	lines, _ := s.ListQuoteLines(ctx, q.ID)
	l1, _ := LineFromQuote(lines[0], false)
	l2, _ := LineFromQuote(lines[1], false)
	first, err := s.CreatePurchaseOrder(ctx, projectID, q.ID, userID, true, PODraft{Lines: []PurchaseOrderLine{l1}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.CreatePurchaseOrder(ctx, projectID, q.ID, userID, true, PODraft{Lines: []PurchaseOrderLine{l2}})
	if err != nil {
		t.Fatal(err)
	}
	taken, err := s.ActivePOLines(ctx, projectID, second)
	if err != nil || len(taken) != 1 || taken[1] != "un borrador" {
		t.Fatalf("ActivePOLines = %v, %v", taken, err)
	}
	day := time.Date(2026, 10, 30, 18, 0, 0, 0, time.UTC)
	for i, id := range []int64{first, second} {
		po, _ := s.PurchaseOrderByID(ctx, id)
		number, seq, err := s.PONumber(ctx, *po, day)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"OC-QA0001-301026", "OC-QA0001-301026-2"}[i]
		if number != want || seq != int64(i+1) {
			t.Fatalf("O.C. %d = %q seq %d, want %q", i+1, number, seq, want)
		}
		if err := s.ConfirmPurchaseOrder(ctx, id, PODraft{Lines: []PurchaseOrderLine{[]PurchaseOrderLine{l1, l2}[i]}},
			POConfirmation{Number: number, Seq: seq, ConfirmedAt: "2026-10-30T18:00:00Z", Document: "{}", UserID: userID}); err != nil {
			t.Fatal(err)
		}
	}
	taken, _ = s.ActivePOLines(ctx, projectID, 0)
	if taken[1] != "OC-QA0001-301026" || taken[2] != "OC-QA0001-301026-2" {
		t.Fatalf("ActivePOLines = %v", taken)
	}
}

func TestSupplierByNameAndFamilyDefault(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	sup := seededSupplier(t, s)
	for _, name := range []string{"cable anti robo", " CABLE ANTI ROBO "} {
		if id, err := s.SupplierByName(ctx, name); err != nil || id == nil || *id != sup.ID {
			t.Errorf("SupplierByName(%q) = %v, %v", name, id, err)
		}
	}
	if id, _ := s.SupplierByName(ctx, "Otro"); id != nil {
		t.Error("an unknown name matched")
	}
	if id, err := s.DefaultSupplierOfQuoteType(ctx, mustTypeID(t, s, "QA")); err != nil || id == nil || *id != sup.ID {
		t.Errorf("QA's default supplier = %v, %v", id, err)
	}
	if id, err := s.DefaultSupplierOfQuoteType(ctx, mustTypeID(t, s, "QL")); err != nil || id != nil {
		t.Errorf("QL's default supplier = %v, %v", id, err)
	}
	if got := SupplierAddress(*sup); got != "Camino Real de Carretas 356, Milenio III, C.P. 76060, Querétaro, Querétaro" {
		t.Errorf("SupplierAddress = %q", got)
	}
}

// 0034 seeds the O.C.'s datos de facturación where nothing is set.
func TestPOBillingSeed(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	b, err := s.POBillingSettings(ctx)
	if err != nil || b.Name != "CLADEX S.A. de C.V." || b.CFDIUse != "G01" ||
		!strings.HasPrefix(b.Address, "Calle Circuito Valle del Silencio 143") || len(b.Errors()) != 0 {
		t.Fatalf("POBillingSettings = %+v, %v", b, err)
	}
	e, _ := s.EmisorSettings(ctx)
	if e.RFC != "CLA260326Q19" || e.TaxRegime != "601" {
		t.Fatalf("emisor = %+v", e)
	}
	if errs := (POBilling{}).Errors(); len(errs) != 3 {
		t.Fatalf("empty billing errors = %v", errs)
	}
}
