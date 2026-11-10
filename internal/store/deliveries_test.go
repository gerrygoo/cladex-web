package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	cladex "github.com/gerrygoo/cladex-web"
)

// confirmTestPO creates an O.C. with lines and confirms it with number and seq.
func confirmTestPO(t *testing.T, s *Store, q *Quote, userID int64, lines []PurchaseOrderLine, number string, seq int64) int64 {
	t.Helper()
	ctx := context.Background()
	id, err := s.CreatePurchaseOrder(ctx, *q.ProjectID, q.ID, userID, true, PODraft{Lines: lines})
	if err != nil {
		t.Fatalf("CreatePurchaseOrder: %v", err)
	}
	if err := s.ConfirmPurchaseOrder(ctx, id, PODraft{Lines: lines},
		POConfirmation{Number: number, Seq: seq, ConfirmedAt: "2026-10-30T18:00:00Z", Document: "{}", UserID: userID}); err != nil {
		t.Fatalf("ConfirmPurchaseOrder: %v", err)
	}
	return id
}

// TestDeliveries walks #66 through the store: two supplier O.C.s, each marked delivered on
// its own, the comprobante added later, an admin's undo, cancelling an O.C. (not while it
// is delivered), a revision after delivery, and Cerrado · Ganado.
func TestDeliveries(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	q, user := poFixture(t, s)
	project := *q.ProjectID
	if _, err := s.db.ExecContext(ctx, `UPDATE quotes SET subtotal = 10000, iva = 1600, total = 11600 WHERE id = ?`, q.ID); err != nil {
		t.Fatal(err)
	}
	load := func() *Project {
		t.Helper()
		p, err := s.ProjectByFolio(ctx, "QA0001")
		if err != nil || p == nil {
			t.Fatalf("ProjectByFolio: %v, %v", p, err)
		}
		return p
	}
	lines, _ := s.ListQuoteLines(ctx, q.ID)
	l1, _ := LineFromQuote(lines[0], false)
	l2, _ := LineFromQuote(lines[1], false)
	first := confirmTestPO(t, s, q, user, []PurchaseOrderLine{l1}, "OC-QA0001-301026", 1)
	second := confirmTestPO(t, s, q, user, []PurchaseOrderLine{l2}, "OC-QA0001-301026-2", 2)
	if got := load().States.Entrega; got != EntregaEnProceso {
		t.Fatalf("two confirmed O.C.s = %q", got)
	}
	pdf := &NewFile{Filename: "entrega.pdf", ContentType: "application/pdf", Data: []byte("%PDF")}

	// What is refused before anything is written.
	for name, err := range map[string]error{
		"bad day":          s.MarkDelivered(ctx, first, user, "05/11/2026", nil),
		"empty file":       s.MarkDelivered(ctx, first, user, "2026-11-05", &NewFile{Filename: "x.pdf"}),
		"no such O.C":      s.MarkDelivered(ctx, 999, user, "2026-11-05", nil),
		"no file":          s.AddDeliveryFile(ctx, first, user, nil),
		"no motivo":        s.UndoDelivery(ctx, first, user, " "),
		"no cancel motivo": s.CancelPurchaseOrder(ctx, first, user, ""),
	} {
		if !errors.Is(err, ErrBadTransition) {
			t.Errorf("%s = %v, want ErrBadTransition", name, err)
		}
	}
	if err := s.AddDeliveryFile(ctx, first, user, pdf); !errors.Is(err, ErrPONotDelivered) {
		t.Errorf("AddDeliveryFile(in process) = %v", err)
	}
	if err := s.UndoDelivery(ctx, first, user, "x"); !errors.Is(err, ErrPONotDelivered) {
		t.Errorf("UndoDelivery(in process) = %v", err)
	}

	// The first, without a comprobante: one of two, still in process.
	if err := s.MarkDelivered(ctx, first, user, "2026-11-05", nil); err != nil {
		t.Fatalf("MarkDelivered: %v", err)
	}
	if err := s.MarkDelivered(ctx, first, user, "2026-11-05", nil); !errors.Is(err, ErrPODelivered) {
		t.Errorf("MarkDelivered(twice) = %v", err)
	}
	p := load()
	if p.States.Entrega != EntregaEnProceso || p.LastBody != "Marcado como entregado el 05/11/2026 por Rodolfo: orden de compra OC-QA0001-301026." {
		t.Errorf("after one = %q, %q", p.States.Entrega, p.LastBody)
	}
	// Its comprobante, later; only once.
	if err := s.AddDeliveryFile(ctx, first, user, pdf); err != nil {
		t.Fatalf("AddDeliveryFile: %v", err)
	}
	if err := s.AddDeliveryFile(ctx, first, user, pdf); !errors.Is(err, ErrDeliveryHasFile) {
		t.Errorf("AddDeliveryFile(twice) = %v", err)
	}
	ds, err := s.ListDeliveries(ctx, project)
	if err != nil || len(ds) != 1 || ds[0].Seq != 1 || ds[0].DeliveredOn != "2026-11-05" || ds[0].FileName != "entrega.pdf" || ds[0].CreatedByName != "Rodolfo" {
		t.Fatalf("ListDeliveries = %+v, %v", ds, err)
	}
	if files, _ := s.ListProjectFiles(ctx, project, "entrega"); len(files) != 1 {
		t.Errorf("entrega files = %d", len(files))
	}
	if p = load(); p.LastBody != "Comprobante de entrega agregado por Rodolfo: orden de compra OC-QA0001-301026." {
		t.Errorf("history = %q", p.LastBody)
	}

	// Delivered, it can't be cancelled; undone (the motivo in the history), it can, and a
	// revision of it in borrador goes with it.
	if err := s.CancelPurchaseOrder(ctx, first, user, "Otro proveedor"); !errors.Is(err, ErrPODelivered) {
		t.Errorf("CancelPurchaseOrder(delivered) = %v", err)
	}
	if err := s.UndoDelivery(ctx, first, user, " Se marcó la equivocada "); err != nil {
		t.Fatalf("UndoDelivery: %v", err)
	}
	if p = load(); p.LastBody != "Se regresó la orden de compra OC-QA0001-301026 a En proceso de entrega.\nMotivo: Se marcó la equivocada" {
		t.Errorf("history = %q", p.LastBody)
	}
	if ds, _ := s.ListDeliveries(ctx, project); len(ds) != 0 {
		t.Errorf("deliveries after undo = %+v", ds)
	}
	draft, err := s.CreatePurchaseOrderRevision(ctx, first, user, "Precio")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CancelPurchaseOrder(ctx, first, user, " Otro proveedor "); err != nil {
		t.Fatalf("CancelPurchaseOrder: %v", err)
	}
	po, _ := s.PurchaseOrderByID(ctx, first)
	if po.Status != POCancelada || po.CancelReason != "Otro proveedor" || po.CancelledByName != "Rodolfo" || po.CancelledAt == "" {
		t.Errorf("cancelled O.C. = %+v", po)
	}
	if gone, _ := s.PurchaseOrderByID(ctx, draft); gone != nil {
		t.Error("the revision in borrador outlived its O.C.")
	}
	if p = load(); p.LastBody != "Se canceló la orden de compra OC-QA0001-301026.\nMotivo: Otro proveedor" {
		t.Errorf("history = %q", p.LastBody)
	}
	for name, err := range map[string]error{
		"cancel again":   s.CancelPurchaseOrder(ctx, first, user, "x"),
		"mark cancelled": s.MarkDelivered(ctx, first, user, "2026-11-05", nil),
	} {
		if !errors.Is(err, ErrBadTransition) {
			t.Errorf("%s = %v, want ErrBadTransition", name, err)
		}
	}
	// Its partida is free for another O.C.
	if taken, _ := s.ActivePOLines(ctx, project, 0); len(taken) != 1 || taken[2] == "" {
		t.Errorf("ActivePOLines after cancelling = %v", taken)
	}

	// The cancelled one doesn't count: the other one, delivered, makes it Entregado.
	if err := s.MarkDelivered(ctx, second, user, "2026-11-06", pdf); err != nil {
		t.Fatalf("MarkDelivered(second): %v", err)
	}
	if p = load(); p.States.Entrega != EntregaEntregado || p.LastBody != "Marcado como entregado el 06/11/2026 por Rodolfo con comprobante: orden de compra OC-QA0001-301026-2." {
		t.Errorf("after the second = %q, %q", p.States.Entrega, p.LastBody)
	}

	// A revision confirmed after the delivery keeps it: delivery belongs to the O.C.
	revID, err := s.CreatePurchaseOrderRevision(ctx, second, user, "Dirección")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ConfirmPurchaseOrder(ctx, revID, PODraft{Lines: []PurchaseOrderLine{l2}},
		POConfirmation{Number: "OC-QA0001-301026-2-R1", Seq: 2, ConfirmedAt: "2026-11-07T18:00:00Z", Document: "{}", UserID: user}); err != nil {
		t.Fatal(err)
	}
	rev, _ := s.PurchaseOrderByID(ctx, revID)
	if d, err := s.DeliveryOf(ctx, *rev); err != nil || d == nil || d.DeliveredOn != "2026-11-06" || d.PurchaseOrderID != second {
		t.Errorf("revision's delivery = %+v, %v", d, err)
	}
	if p = load(); p.States.Entrega != EntregaEntregado || p.Stage() != StageEnCurso {
		t.Errorf("after the revision = %q, %q", p.States.Entrega, p.Stage())
	}

	// Facturada, entregado and pagado: Cerrado · Ganado, out of the default list.
	if err := s.RecordInvoice(ctx, project, user, testInvoice, "PUE", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordPayment(ctx, project, user, 11600, "2026-11-08", nil); err != nil {
		t.Fatal(err)
	}
	if p = load(); p.Stage() != StageCerrado || !p.States.Cerrado {
		t.Errorf("complete = %q, %+v", p.Stage(), p.States)
	}
	list := func(f Filters) int {
		ps, err := s.ListProjects(ctx, "", "", "", f)
		if err != nil {
			t.Fatal(err)
		}
		return len(ps)
	}
	if list(nil) != 0 || list(Filters{"etapa": {"cerrado"}}) != 1 {
		t.Errorf("list = %d, with the Cerrado · Ganado filter = %d", list(nil), list(Filters{"etapa": {"cerrado"}}))
	}
	// Undoing the delivery reopens it.
	if err := s.UndoDelivery(ctx, revID, user, "Faltó material"); err != nil {
		t.Fatal(err)
	}
	if p = load(); p.Stage() != StageEnCurso || p.States.Entrega != EntregaEnProceso {
		t.Errorf("undone = %q, %q", p.Stage(), p.States.Entrega)
	}

	// With every O.C. cancelled, it is Sin emitir O.C. again, and cancelling the proyecto
	// (a typed factura doesn't block it) goes through.
	if err := s.CancelPurchaseOrder(ctx, revID, user, "Se cayó"); err != nil {
		t.Fatal(err)
	}
	if p = load(); p.States.Entrega != EntregaSinOC {
		t.Errorf("all cancelled = %q", p.States.Entrega)
	}
	if err := s.CancelProject(ctx, project, user, "Se cayó el pedido"); err != nil {
		t.Errorf("CancelProject with only cancelled O.C.s = %v", err)
	}

	// Each write is audited.
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM audit_log WHERE table_name = 'deliveries'`).Scan(&n); err != nil || n != 5 {
		t.Errorf("deliveries audit rows = %d, %v", n, err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM audit_log WHERE table_name = 'purchase_orders'
		AND json_extract(new_values, '$.cancel_reason') = 'Otro proveedor'`).Scan(&n); err != nil || n != 1 {
		t.Errorf("cancellation audit rows = %d, %v", n, err)
	}
	// Deleting the proyecto takes its deliveries.
	if err := s.DeleteProject(ctx, project); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
}

// TestMigration0042 opens a database as it was before 0041, with a proyecto closed under
// the old flow (QS0001: en_entrega, then cerrado, with its payment marked), one only
// facturado (QS0002), and one that went to en_entrega and back (QS0003), each with a
// confirmed supplier O.C., and checks that only the closed one is delivered, on the day it
// went en_entrega (México City), by whoever moved it, and so Cerrado · Ganado again.
func TestMigration0042(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "up.db")
	old, err := Open(ctx, dsn, migrationsBefore(t, "0041"))
	if err != nil {
		t.Fatalf("open at 0040: %v", err)
	}
	q, user := projectFixture(t, old)
	admin, err := old.CreateUser(ctx, "gerardo", "Gerardo", "hash", "admin")
	if err != nil {
		t.Fatal(err)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := old.db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	ids := map[string]int64{}
	for _, folio := range []string{"QS0001", "QS0002", "QS0003"} {
		quoteID := insertProject(t, old, folio, q.CustomerID, user, "oc_recibida", 90, 100_00)
		var projectID int64
		if err := old.db.QueryRowContext(ctx, `SELECT project_id FROM quotes WHERE id = ?`, quoteID).Scan(&projectID); err != nil {
			t.Fatal(err)
		}
		ids[folio] = projectID
		exec(`INSERT INTO purchase_orders (project_id, quote_id, created_by, status, seq, number, document)
			VALUES (?, ?, ?, 'confirmada', 1, ?, '{}')`, ids[folio], quoteID, user, "OC-"+folio+"-301026")
	}
	// Moves under the old flow, each logged by the audit trigger as whoever audit_actor
	// says.
	move := func(folio, status string, actor int64) {
		t.Helper()
		exec(`UPDATE audit_actor SET user_id = ?, source = 'web' WHERE id = 1`, actor)
		exec(`UPDATE projects SET status = ? WHERE id = ?`, status, ids[folio])
	}
	move("QS0001", "facturado", user)
	move("QS0001", "en_entrega", admin)
	var movedAt string
	if err := old.db.QueryRowContext(ctx, `SELECT at FROM audit_log WHERE id = (SELECT MAX(id) FROM audit_log WHERE table_name = 'projects')`).Scan(&movedAt); err != nil {
		t.Fatal(err)
	}
	at, err := time.Parse(time.RFC3339, movedAt)
	if err != nil {
		t.Fatal(err)
	}
	movedOn := at.Add(-6 * time.Hour).Format("2006-01-02") // México City, UTC-6
	move("QS0001", "cerrado", user)
	exec(`UPDATE projects SET invoice_ref = 'F-1', invoice_date = '2025-10-29', payment_status = 'pagado', paid_at = '2025-11-01' WHERE id = ?`, ids["QS0001"])
	move("QS0002", "facturado", user)
	move("QS0003", "facturado", user)
	move("QS0003", "en_entrega", admin)
	move("QS0003", "facturado", admin)
	old.Close()

	s, err := Open(ctx, dsn, cladex.MigrationsFS)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	defer s.Close()
	var rows int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM pragma_foreign_key_check`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("foreign_key_check rows = %d, err = %v", rows, err)
	}
	ds, err := s.ListDeliveries(ctx, ids["QS0001"])
	if err != nil || len(ds) != 1 || ds[0].DeliveredOn != movedOn || ds[0].CreatedByName != "Gerardo" || ds[0].FileID != 0 || ds[0].Seq != 1 {
		t.Fatalf("QS0001 deliveries = %+v, %v", ds, err)
	}
	p, _ := s.ProjectByFolio(ctx, "QS0001")
	if p.States.Entrega != EntregaEntregado || p.Stage() != StageCerrado {
		t.Errorf("QS0001 = %q, %+v", p.Stage(), p.States)
	}
	for _, folio := range []string{"QS0002", "QS0003"} {
		if ds, _ := s.ListDeliveries(ctx, ids[folio]); len(ds) != 0 {
			t.Errorf("%s deliveries = %+v", folio, ds)
		}
		if p, _ := s.ProjectByFolio(ctx, folio); p.States.Entrega != EntregaEnProceso {
			t.Errorf("%s entrega = %q", folio, p.States.Entrega)
		}
	}
	// Written as the system's doing.
	var actor any
	if err := s.db.QueryRowContext(ctx, `SELECT actor_id FROM audit_log WHERE table_name = 'deliveries' AND op = 'insert'`).Scan(&actor); err != nil || actor != nil {
		t.Errorf("migrated delivery logged by %v, %v", actor, err)
	}
}
