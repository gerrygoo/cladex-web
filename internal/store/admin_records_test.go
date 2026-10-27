package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
)

// rowCount is COUNT(*) of table; the name is the test's own, never user input.
func rowCount(t *testing.T, s *Store, table string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// fullProject builds a proyecto that has a row in every table DeleteProject has to
// clear: a revision chain, comments, an OC file, a supplier quote file, and a factura
// with a concepto, an event and a pago pointing at it.
func fullProject(t *testing.T, s *Store, factStatus string) (projectID, adminID int64) {
	t.Helper()
	ctx := context.Background()
	q, userID := projectFixture(t, s)
	adminID, err := s.CreateUser(ctx, "ana", "Ana", "hash", "admin")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	lineRes, err := s.db.Exec(`INSERT INTO quote_lines (quote_id, line_no, description_snapshot, qty_milli, unit_price_micros, line_total)
		VALUES (?, 1, 'linea', 1000, 1, 1)`, q.ID)
	if err != nil {
		t.Fatalf("quote_lines: %v", err)
	}
	lineID, _ := lineRes.LastInsertId()
	if _, err := s.CreateRevision(ctx, q.ID, userID); err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	if err := s.AddQuoteComment(ctx, q.ID, userID, "hola"); err != nil {
		t.Fatalf("AddQuoteComment: %v", err)
	}
	if err := s.AddSupplierQuoteFiles(ctx, q.ID, []NewFile{{Filename: "p.pdf", ContentType: "application/pdf", Data: []byte("x")}}, userID); err != nil {
		t.Fatalf("AddSupplierQuoteFiles: %v", err)
	}
	if err := s.SetSupplierQuoteInfo(ctx, q.ID, "Prov", "7", userID); err != nil {
		t.Fatalf("SetSupplierQuoteInfo: %v", err)
	}
	projectID = *q.ProjectID
	if _, err := s.db.Exec(`INSERT INTO project_files (project_id, kind, filename, content_type, size, data, uploaded_by)
		VALUES (?, 'oc', 'oc.pdf', 'application/pdf', 1, x'00', ?)`, projectID, userID); err != nil {
		t.Fatalf("project_files: %v", err)
	}
	res, err := s.db.Exec(`INSERT INTO comprobantes (tipo, project_id, quote_id, status, created_by) VALUES ('factura', ?, ?, ?, ?)`,
		projectID, q.ID, factStatus, userID)
	if err != nil {
		t.Fatalf("comprobantes: %v", err)
	}
	factID, _ := res.LastInsertId()
	if _, err := s.db.Exec(`INSERT INTO comprobantes (tipo, project_id, related_id, status, created_by) VALUES ('pago', ?, ?, 'borrador', ?)`,
		projectID, factID, userID); err != nil {
		t.Fatalf("pago: %v", err)
	}
	if _, err := s.db.Exec(`INSERT INTO comprobante_conceptos (comprobante_id, line_no, quote_line_id, description, qty_milli, unit_price_micros, importe, iva)
		VALUES (?, 1, ?, 'x', 1000, 1, 1, 0)`, factID, lineID); err != nil {
		t.Fatalf("conceptos: %v", err)
	}
	if _, err := s.db.Exec(`INSERT INTO comprobante_events (comprobante_id, event) VALUES (?, 'creada')`, factID); err != nil {
		t.Fatalf("events: %v", err)
	}
	return projectID, adminID
}

func TestDeleteProjectRemovesEverything(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	projectID, _ := fullProject(t, s, "borrador")
	// A second proyecto must be left alone.
	otherID, _ := func() (int64, int64) {
		customerID, _ := s.CreateCustomer(ctx, Customer{Name: "Otro"})
		userID, _ := s.CreateUser(ctx, "otro", "Otro", "hash", "vendedor")
		q, _ := s.CreateDraftQuote(ctx, customerID, userID, "QA")
		if err := s.IssueQuote(ctx, q.ID, testIssue("sha2")); err != nil {
			t.Fatalf("IssueQuote: %v", err)
		}
		q, _ = s.QuoteByID(ctx, q.ID)
		return *q.ProjectID, userID
	}()

	if err := s.DeleteProject(ctx, projectID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	for table, want := range map[string]int{
		"projects": 1, "quotes": 1, "comprobantes": 0, "comprobante_conceptos": 0,
		"comprobante_events": 0, "quote_lines": 0, "quote_comments": 0, "project_files": 0, "supplier_quotes": 0, "supplier_quote_files": 0,
	} {
		if got := rowCount(t, s, table); got != want {
			t.Errorf("%s: %d rows left, want %d", table, got, want)
		}
	}
	// What is left must still be consistent, and the delete must have been audited.
	if rows, err := s.db.Query(`PRAGMA foreign_key_check`); err != nil || rows.Next() {
		t.Errorf("foreign_key_check found violations (err=%v)", err)
	}
	var audited int
	s.db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE table_name = 'projects' AND op = 'delete' AND row_key = ?`, projectID).Scan(&audited)
	if audited != 1 {
		t.Errorf("audit_log has %d delete entries for the project, want 1", audited)
	}
	var left int
	s.db.QueryRow(`SELECT COUNT(*) FROM projects WHERE id = ?`, otherID).Scan(&left)
	if left != 1 {
		t.Errorf("the other proyecto was touched")
	}
	if err := s.DeleteProject(ctx, projectID); err == nil {
		t.Errorf("deleting an already deleted proyecto succeeded")
	}
}

func TestDeleteProjectRefusesStampedInvoices(t *testing.T) {
	for _, status := range []string{"timbrando", "incierto", "timbrada", "cancelacion_pendiente", "cancelada"} {
		t.Run(status, func(t *testing.T) {
			s := newTestStore(t)
			projectID, _ := fullProject(t, s, status)
			before := map[string]int{}
			for _, tb := range []string{"projects", "quotes", "comprobantes", "comprobante_events"} {
				before[tb] = rowCount(t, s, tb)
			}
			if err := s.DeleteProject(context.Background(), projectID); !errors.Is(err, ErrHasStampedInvoice) {
				t.Fatalf("DeleteProject = %v, want ErrHasStampedInvoice", err)
			}
			for tb, n := range before {
				if got := rowCount(t, s, tb); got != n {
					t.Errorf("%s changed from %d to %d rows", tb, n, got)
				}
			}
		})
	}
}

func TestDeleteDraftQuote(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	customerID, _ := s.CreateCustomer(ctx, Customer{Name: "C"})
	userID, _ := s.CreateUser(ctx, "u", "U", "hash", "vendedor")
	draft, err := s.CreateDraftQuote(ctx, customerID, userID, "QA")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddQuoteComment(ctx, draft.ID, userID, "nota"); err != nil {
		t.Fatal(err)
	}
	issued, _ := s.CreateDraftQuote(ctx, customerID, userID, "QA")
	if err := s.IssueQuote(ctx, issued.ID, testIssue("sha")); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteDraftQuote(ctx, issued.ID); !errors.Is(err, ErrNotDeletable) {
		t.Errorf("deleting an issued quote = %v, want ErrNotDeletable", err)
	}
	if err := s.DeleteDraftQuote(ctx, draft.ID); err != nil {
		t.Fatalf("DeleteDraftQuote: %v", err)
	}
	if got := rowCount(t, s, "quotes"); got != 1 {
		t.Errorf("%d quotes left, want 1", got)
	}
	if got := rowCount(t, s, "quote_comments"); got != 0 {
		t.Errorf("%d comments left, want 0", got)
	}
}

func TestReassign(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	projectID, adminID := fullProject(t, s, "borrador")
	gerardo, _ := s.CreateUser(ctx, "gg", "Gerardo", "hash", "vendedor")
	off, _ := s.CreateUser(ctx, "off", "Baja", "hash", "vendedor")
	if err := s.SetUserDisabled(ctx, off, true); err != nil {
		t.Fatalf("SetUserDisabled: %v", err)
	}

	if err := s.ReassignProject(ctx, projectID, adminID, off); !errors.Is(err, ErrBadOwner) {
		t.Errorf("reassign to a disabled user = %v, want ErrBadOwner", err)
	}
	if err := s.ReassignProject(ctx, projectID, adminID, gerardo); err != nil {
		t.Fatalf("ReassignProject: %v", err)
	}
	var stale int
	s.db.QueryRow(`SELECT (SELECT COUNT(*) FROM projects WHERE id = ?1 AND user_id != ?2)
		+ (SELECT COUNT(*) FROM quotes WHERE project_id = ?1 AND user_id != ?2)`, projectID, gerardo).Scan(&stale)
	if stale != 0 {
		t.Errorf("%d rows still belong to the old owner", stale)
	}
	var note string
	s.db.QueryRow(`SELECT body FROM quote_comments ORDER BY id DESC LIMIT 1`).Scan(&note)
	if !strings.Contains(note, "Rodolfo") || !strings.Contains(note, "Gerardo") {
		t.Errorf("history note = %q, want it to name both owners", note)
	}
	users, err := s.ActiveUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range users {
		if u.ID == off {
			t.Errorf("ActiveUsers lists a disabled user")
		}
	}
}

// TestDeleteCoversEveryReference fails when a table starts referencing quotes, quote
// lines, comprobantes or projects without DeleteProject/DeleteDraftQuote knowing: no FK
// cascades, so a forgotten child would turn every delete into a constraint error.
func TestDeleteCoversEveryReference(t *testing.T) {
	s := newTestStore(t)
	rows, err := s.db.Query(`
		SELECT m.name || '.' || f."from" || ' -> ' || f."table"
		FROM sqlite_master m, pragma_foreign_key_list(m.name) f
		WHERE m.type = 'table' AND f."table" IN ('quotes', 'quote_lines', 'projects', 'comprobantes')`)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var r string
		rows.Scan(&r)
		got = append(got, r)
	}
	sort.Strings(got)
	want := []string{
		"comprobante_conceptos.comprobante_id -> comprobantes",
		"comprobante_conceptos.quote_line_id -> quote_lines",
		"comprobante_events.comprobante_id -> comprobantes",
		"comprobantes.project_id -> projects",
		"comprobantes.quote_id -> quotes",
		"comprobantes.related_id -> comprobantes",
		"project_files.project_id -> projects",
		"quote_comments.quote_id -> quotes",
		"quote_lines.quote_id -> quotes",
		"quotes.project_id -> projects",
		"quotes.supersedes_quote_id -> quotes",
		"supplier_quote_files.root_quote_id -> quotes",
		"supplier_quotes.root_quote_id -> quotes",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("foreign keys into quotes/projects/comprobantes changed; teach DeleteProject and DeleteDraftQuote about the new one, then update this list.\n got: %v\nwant: %v", got, want)
	}
}
