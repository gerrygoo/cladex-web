package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gerrygoo/cladex-web"
	"github.com/gerrygoo/cladex-web/internal/money"
)

func TestForecastRelevant(t *testing.T) {
	for _, c := range []struct {
		stage string
		p     int
		want  bool
	}{
		{"prospecto", 10, false}, {"prospecto", 50, false}, {"prospecto", 75, true}, {"prospecto", 90, true},
		{"en_curso", 90, false}, {"", 90, false},
	} {
		if got := ForecastRelevant(c.stage, c.p); got != c.want {
			t.Errorf("ForecastRelevant(%q, %d) = %v, want %v", c.stage, c.p, got, c.want)
		}
	}
	if ProbabilityLabel(75) != "Alta" || ProbabilityLabel(40) != "" {
		t.Errorf("ProbabilityLabel(75) = %q, ProbabilityLabel(40) = %q", ProbabilityLabel(75), ProbabilityLabel(40))
	}
}

// testOC is a complete purchase order for tests that only need a proyecto past prospecto.
var testOC = OC{Number: "4411", Date: "2026-10-07"}

// testInvoice is a factura typed in, for tests that only need a proyecto en curso with
// its factura.
var testInvoice = Document{Ref: "F-1001", Date: "2026-10-08"}

// projectFixture issues one quote through the store, so it has a proyecto, and returns
// the quote.
func projectFixture(t *testing.T, s *Store) (quote *Quote, userID int64) {
	t.Helper()
	ctx := context.Background()
	customerID, err := s.CreateCustomer(ctx, Customer{Name: "Grupo PEME"})
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	userID, err = s.CreateUser(ctx, "rfm", "Rodolfo", "hash", "vendedor")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	q, err := s.CreateDraftQuote(ctx, customerID, userID, "QA")
	if err != nil {
		t.Fatalf("CreateDraftQuote: %v", err)
	}
	if q.ProjectID != nil {
		t.Fatalf("a first draft already has a proyecto: %+v", q)
	}
	if err := s.IssueQuote(ctx, q.ID, testIssue("sha1")); err != nil {
		t.Fatalf("IssueQuote: %v", err)
	}
	q, err = s.QuoteByID(ctx, q.ID)
	if err != nil || q == nil {
		t.Fatalf("QuoteByID: %v", err)
	}
	return q, userID
}

func TestIssueQuoteOpensProject(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	q, userID := projectFixture(t, s)

	if q.ProjectID == nil || q.ProjectFolio != "QA0001" || q.Stage != "prospecto" || q.Probability != 10 {
		t.Fatalf("issued quote's proyecto = %+v, %q, %q, %d", q.ProjectID, q.ProjectFolio, q.Stage, q.Probability)
	}
	if q.Status != "emitida" || !q.Revisable() {
		t.Fatalf("status = %q, revisable = %v", q.Status, q.Revisable())
	}
	var owner, customer int64
	if err := s.db.QueryRowContext(ctx, `SELECT user_id, customer_id FROM projects WHERE id = ?`, *q.ProjectID).Scan(&owner, &customer); err != nil {
		t.Fatalf("project row: %v", err)
	}
	if owner != userID || customer != q.CustomerID {
		t.Fatalf("project owner = %d, customer = %d", owner, customer)
	}

	// A revision stays in the proyecto, as a draft and once issued; no second proyecto.
	rev, err := s.CreateRevision(ctx, q.ID, userID)
	if err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	if rev.ProjectID == nil || *rev.ProjectID != *q.ProjectID || rev.Status != "borrador" {
		t.Fatalf("revision = %+v", rev)
	}
	if err := s.IssueQuote(ctx, rev.ID, testIssue("sha2")); err != nil {
		t.Fatalf("IssueQuote(revision): %v", err)
	}
	var projects int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM projects`).Scan(&projects); err != nil || projects != 1 {
		t.Fatalf("projects = %d, %v; want 1", projects, err)
	}
	if old, _ := s.QuoteByID(ctx, q.ID); old.Status != "revisada" || old.Revisable() {
		t.Fatalf("superseded quote = %q, revisable %v", old.Status, old.Revisable())
	}

	// The proyecto is audited: opened by the issuing user.
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM audit_log WHERE table_name = 'projects' AND op = 'insert'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("project audit inserts = %d, %v", n, err)
	}
}

func TestReceiveOCStartsEnCurso(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	q, user := projectFixture(t, s)
	project := *q.ProjectID
	load := func() *Project {
		t.Helper()
		p, err := s.ProjectByFolio(ctx, "QA0001")
		if err != nil || p == nil {
			t.Fatalf("ProjectByFolio: %v, %v", p, err)
		}
		return p
	}

	// A prospecto's states are derived too, but nothing is on record yet.
	if p := load(); p.Stage() != StageProspecto || p.States != (ProjectStates{Facturacion: "pendiente", Entrega: "sin_oc", Cobro: "pendiente"}) {
		t.Fatalf("prospecto = %q, %+v", p.Stage(), p.States)
	}
	// A typed factura needs the proyecto en curso.
	if err := s.RecordInvoice(ctx, project, user, testInvoice, "PUE", ""); !errors.Is(err, ErrBadTransition) {
		t.Errorf("RecordInvoice(prospecto) = %v, want ErrBadTransition", err)
	}
	if err := s.ReceiveOC(ctx, project, user, testOC, nil, "Llegó la OC 4411"); err != nil {
		t.Fatalf("ReceiveOC: %v", err)
	}
	p := load()
	if p.Status != StageEnCurso || p.Stage() != StageEnCurso || p.States.Facturacion != FacturacionPendiente ||
		p.States.Entrega != EntregaSinOC || p.States.Cobro != CobroPendiente || p.States.Cerrado {
		t.Fatalf("en curso = %q, %+v", p.Status, p.States)
	}
	// Once the purchase order is in, the quote is locked.
	if _, err := s.CreateRevision(ctx, q.ID, user); !errors.Is(err, ErrProjectLocked) {
		t.Errorf("CreateRevision(en_curso) = %v, want ErrProjectLocked", err)
	}

	// The typed factura makes it facturada and moves nothing; a second one is refused,
	// and so is correcting the purchase order it bills.
	if err := s.RecordInvoice(ctx, project, user, testInvoice, "PUE", "Factura enviada"); err != nil {
		t.Fatalf("RecordInvoice: %v", err)
	}
	if p = load(); p.Status != StageEnCurso || p.States.Facturacion != FacturacionFacturada || p.Invoice != testInvoice {
		t.Fatalf("after the factura = %q, %+v, %+v", p.Status, p.States, p.Invoice)
	}
	if err := s.RecordInvoice(ctx, project, user, Document{Ref: "F-2", Date: "2026-10-09"}, "PUE", ""); !errors.Is(err, ErrBadTransition) {
		t.Errorf("RecordInvoice(again) = %v, want ErrBadTransition", err)
	}
	if err := s.ReceiveOC(ctx, project, user, OC{Number: "4412", Date: "2026-10-07"}, nil, ""); !errors.Is(err, ErrBadTransition) {
		t.Errorf("ReceiveOC(facturada) = %v, want ErrBadTransition", err)
	}
	// The quote itself never left emitida.
	if got, _ := s.QuoteByID(ctx, q.ID); got.Status != "emitida" || got.Stage != StageEnCurso || got.Revisable() {
		t.Errorf("quote status = %q, stage = %q, revisable = %v", got.Status, got.Stage, got.Revisable())
	}

	comments, err := s.ListQuoteComments(ctx, q.ID)
	if err != nil || len(comments) != 2 {
		t.Fatalf("comments = %d, %v; want 2", len(comments), err)
	}
	if comments[1].Body != "Pasó a En curso.\nO.C. 4411 · 07/10/2026\nLlegó la OC 4411" {
		t.Errorf("first comment = %q", comments[1].Body)
	}
	if comments[0].Body != "Factura registrada: F-1001 · 08/10/2026 · P.U.E.\nFactura enviada" {
		t.Errorf("newest comment = %q", comments[0].Body)
	}
}

// A prospecto whose current quote is a revision still in borrador can't receive the
// purchase order, and its moves and comments land on whichever quote is current.
func TestMoveProjectFollowsTheCurrentQuote(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	q, user := projectFixture(t, s)
	project := *q.ProjectID

	rev, err := s.CreateRevision(ctx, q.ID, user)
	if err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	if err := s.ReceiveOC(ctx, project, user, testOC, nil, ""); !errors.Is(err, ErrBadTransition) {
		t.Fatalf("MoveProject with a draft revision = %v, want ErrBadTransition", err)
	}
	if err := s.IssueQuote(ctx, rev.ID, testIssue("sha2")); err != nil {
		t.Fatalf("IssueQuote(revision): %v", err)
	}
	if err := s.ReceiveOC(ctx, project, user, testOC, nil, ""); err != nil {
		t.Fatalf("MoveProject: %v", err)
	}
	if c, _ := s.ListQuoteComments(ctx, rev.ID); len(c) != 1 {
		t.Errorf("comments on the current quote = %d, want 1", len(c))
	}
	if c, _ := s.ListQuoteComments(ctx, q.ID); len(c) != 0 {
		t.Errorf("comments on the superseded quote = %d, want 0", len(c))
	}
}

func TestSetProjectProbability(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	q, user := projectFixture(t, s)
	project := *q.ProjectID
	probability := func() int {
		q, err := s.QuoteByID(ctx, q.ID)
		if err != nil || q == nil {
			t.Fatalf("QuoteByID: %v", err)
		}
		return q.Probability
	}

	for _, bad := range []int{0, 40, 100, -75} {
		if err := s.SetProjectProbability(ctx, project, user, bad, ""); !errors.Is(err, ErrBadTransition) {
			t.Errorf("SetProjectProbability(%d) = %v, want ErrBadTransition", bad, err)
		}
	}
	if err := s.SetProjectProbability(ctx, project, user, 75, "Compras ya lo aprobó"); err != nil {
		t.Fatalf("SetProjectProbability: %v", err)
	}
	if got := probability(); got != 75 {
		t.Fatalf("probability = %d, want 75", got)
	}
	// The same value again changes nothing and says nothing; with a note, the note stays.
	if err := s.SetProjectProbability(ctx, project, user, 75, ""); err != nil {
		t.Fatalf("SetProjectProbability(same): %v", err)
	}
	if err := s.SetProjectProbability(ctx, project, user, 75, "Sigue igual"); err != nil {
		t.Fatalf("SetProjectProbability(same, note): %v", err)
	}
	comments, err := s.ListQuoteComments(ctx, q.ID)
	if err != nil || len(comments) != 2 {
		t.Fatalf("comments = %d, %v; want 2", len(comments), err)
	}
	if comments[1].Body != "Probabilidad: Inicial → Alta.\nCompras ya lo aprobó" || comments[0].Body != "Sigue igual" {
		t.Errorf("comments = %q, %q", comments[1].Body, comments[0].Body)
	}

	// Only a prospecto has a probability to set; the value it had is kept.
	if err := s.ReceiveOC(ctx, project, user, testOC, nil, ""); err != nil {
		t.Fatalf("MoveProject: %v", err)
	}
	if err := s.SetProjectProbability(ctx, project, user, 90, ""); !errors.Is(err, ErrBadTransition) {
		t.Errorf("SetProjectProbability(en_curso) = %v, want ErrBadTransition", err)
	}
	if got := probability(); got != 75 {
		t.Errorf("probability after leaving prospecto = %d, want 75", got)
	}
}

// TestMigration0016 opens a database as it was with the stages on quotes.status, and
// checks what each kind of quote became.
func TestMigration0016(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "up.db")
	old, err := Open(ctx, dsn, migrationsBefore(t, "0016"))
	if err != nil {
		t.Fatalf("open at 0015: %v", err)
	}
	for _, q := range []string{
		`INSERT INTO users (username, name, password_hash, role) VALUES ('ana', 'Ana', 'x', 'admin'), ('beto', 'Beto', 'x', 'vendedor')`,
		`INSERT INTO customers (name) VALUES ('ACME'), ('PEME')`,
		// 1 a draft never issued; 2 emitida; 3 pipeline; 4 oc_emitida; 5 entregada;
		// 6 cerrada; 7 → 8 → 9 a chain whose live quote is in pipeline; 10 → 11 a chain
		// whose live quote is a draft revision, made by another user.
		`INSERT INTO quotes (id, folio, prefix, customer_id, user_id, status, total, issued_at, supersedes_quote_id, pdf_sha256) VALUES
			(1, 'QA0001', 'QA', 1, 1, 'borrador', 100, NULL, NULL, NULL),
			(2, 'QA0002', 'QA', 1, 1, 'emitida', 200, '2026-09-01T10:00:00.000Z', NULL, 'sha2'),
			(3, 'QA0003', 'QA', 2, 2, 'pipeline', 300, '2026-09-02T10:00:00.000Z', NULL, 'sha3'),
			(4, 'QS0001', 'QS', 1, 1, 'oc_emitida', 400, '2026-09-03T10:00:00.000Z', NULL, 'sha4'),
			(5, 'QA0004', 'QA', 1, 1, 'entregada', 500, '2026-09-04T10:00:00.000Z', NULL, 'sha5'),
			(6, 'QA0005', 'QA', 1, 1, 'cerrada', 600, '2026-09-05T10:00:00.000Z', NULL, 'sha6'),
			(7, 'QA0006', 'QA', 2, 2, 'revisada', 700, '2026-09-06T10:00:00.000Z', NULL, 'sha7'),
			(8, 'QA0006-R1', 'QA', 2, 2, 'revisada', 710, '2026-09-07T10:00:00.000Z', 7, 'sha8'),
			(9, 'QA0006-R2', 'QA', 2, 2, 'pipeline', 720, '2026-09-08T10:00:00.000Z', 8, 'sha9'),
			(10, 'QA0007', 'QA', 1, 2, 'revisada', 800, '2026-09-09T10:00:00.000Z', NULL, 'sha10'),
			(11, 'QA0007-R1', 'QA', 1, 1, 'borrador', 810, NULL, 10, NULL)`,
		`INSERT INTO quote_lines (quote_id, line_no, description_snapshot, qty_milli, unit_price_micros, line_total)
		 VALUES (2, 1, 'Cable', 1000, 2000000, 200)`,
		`INSERT INTO quote_comments (quote_id, user_id, body) VALUES (3, 2, 'Pasó a Pipeline.')`,
	} {
		if _, err := old.db.ExecContext(ctx, q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	var auditBefore int
	if err := old.db.QueryRowContext(ctx, `SELECT count(*) FROM audit_log`).Scan(&auditBefore); err != nil {
		t.Fatalf("audit count: %v", err)
	}
	old.Close()

	s, err := Open(ctx, dsn, cladex.MigrationsFS)
	if err != nil {
		t.Fatalf("open at 0016: %v", err)
	}
	defer s.Close()

	var rows int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM pragma_foreign_key_check`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("foreign_key_check rows = %d, err = %v", rows, err)
	}
	// The copy is not an edit: nothing new in the audit log.
	var auditAfter int
	if err := s.db.QueryRowContext(ctx, countAuditWithoutSupplierSeed).Scan(&auditAfter); err != nil || auditAfter != auditBefore {
		t.Fatalf("audit_log rows = %d, were %d (%v)", auditAfter, auditBefore, err)
	}

	for _, c := range []struct {
		folio, status, project, stage string
		probability                   int
	}{
		{"QA0001", "borrador", "", "", 0},
		{"QA0002", "emitida", "QA0002", "prospecto", 10},
		{"QA0003", "emitida", "QA0003", "prospecto", 75},
		{"QS0001", "emitida", "QS0001", "en_curso", 10},
		{"QA0004", "emitida", "QA0004", "en_curso", 10},
		{"QA0005", "emitida", "QA0005", "en_curso", 10},
		{"QA0006", "revisada", "QA0006", "prospecto", 75},
		{"QA0006-R1", "revisada", "QA0006", "prospecto", 75},
		{"QA0006-R2", "emitida", "QA0006", "prospecto", 75},
		{"QA0007", "revisada", "QA0007", "prospecto", 10},
		{"QA0007-R1", "borrador", "QA0007", "prospecto", 10},
	} {
		q, err := s.QuoteByFolio(ctx, c.folio)
		if err != nil || q == nil {
			t.Fatalf("QuoteByFolio(%s): %v", c.folio, err)
		}
		if q.Status != c.status || q.ProjectFolio != c.project || q.Stage != c.stage || q.Probability != c.probability {
			t.Errorf("%s = status %q, proyecto %q, stage %q, probability %d; want %q, %q, %q, %d",
				c.folio, q.Status, q.ProjectFolio, q.Stage, q.Probability, c.status, c.project, c.stage, c.probability)
		}
	}

	// A proyecto takes its customer, owner and date from the quote that opened it.
	var customer, owner int64
	var created string
	if err := s.db.QueryRowContext(ctx,
		`SELECT customer_id, user_id, created_at FROM projects WHERE folio = 'QA0007'`).Scan(&customer, &owner, &created); err != nil {
		t.Fatalf("project QA0007: %v", err)
	}
	if customer != 1 || owner != 2 || created != "2026-09-09T10:00:00.000Z" {
		t.Errorf("project QA0007 = customer %d, owner %d, created %q", customer, owner, created)
	}

	// What hangs off a quote is untouched.
	q, _ := s.QuoteByFolio(ctx, "QA0002")
	if q.PDFSHA256 == nil || *q.PDFSHA256 != "sha2" || q.Total != 200 {
		t.Errorf("QA0002 after migration = %+v", q)
	}
	if lines, err := s.ListQuoteLines(ctx, q.ID); err != nil || len(lines) != 1 {
		t.Errorf("lines = %d, %v", len(lines), err)
	}
	q3, _ := s.QuoteByFolio(ctx, "QA0003")
	if c, err := s.ListQuoteComments(ctx, q3.ID); err != nil || len(c) != 1 {
		t.Errorf("comments = %d, %v", len(c), err)
	}

	// The migrated database behaves like a new one: the revision in progress can be
	// issued and its proyecto moved on, and a second live quote in a proyecto is refused.
	if err := s.IssueQuote(ctx, 11, testIssue("sha11")); err != nil {
		t.Fatalf("IssueQuote(QA0007-R1): %v", err)
	}
	q11, _ := s.QuoteByID(ctx, 11)
	if err := s.ReceiveOC(ctx, *q11.ProjectID, 1, testOC, nil, ""); err != nil {
		t.Fatalf("MoveProject: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE quotes SET status = 'emitida' WHERE id = 10`); err == nil {
		t.Error("a proyecto accepted two quotes that aren't revisada")
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE quotes SET status = 'pipeline' WHERE id = 2`); err == nil {
		t.Error("quotes.status still accepts a pipeline stage")
	}
}

// TestProjectReads covers what the proyecto screens read: one proyecto by folio, the
// list with its search, sort and filters, a proyecto's quotes and its history.
func TestProjectReads(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	q, user := projectFixture(t, s)
	other, err := s.CreateCustomer(ctx, Customer{Name: "Constructora X"})
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	insertProject(t, s, "QS0001", other, user, "en_curso", 90, 900_00)
	insertProject(t, s, "QA0002", other, user, "prospecto", 90, 300_00)

	if err := s.AddQuoteComment(ctx, q.ID, user, "Primera llamada"); err != nil {
		t.Fatalf("AddQuoteComment: %v", err)
	}
	rev, err := s.CreateRevision(ctx, q.ID, user)
	if err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE quotes SET total = 150000 WHERE id = ?`, rev.ID); err != nil {
		t.Fatalf("set total: %v", err)
	}
	if err := s.SetProjectProbability(ctx, *q.ProjectID, user, 50, ""); err != nil {
		t.Fatalf("SetProjectProbability: %v", err)
	}

	p, err := s.ProjectByFolio(ctx, "QA0001")
	if err != nil || p == nil {
		t.Fatalf("ProjectByFolio: %v, %v", p, err)
	}
	if p.CustomerName != "Grupo PEME" || p.UserName != "Rodolfo" || p.Status != "prospecto" || p.Probability != 50 ||
		p.CurrentQuoteFolio != "QA0001-R1" || p.CurrentQuoteStatus != "borrador" || p.Total != 150000 || p.ForecastRelevant() {
		t.Errorf("project = %+v", p)
	}
	if none, err := s.ProjectByFolio(ctx, "QA0001-R1"); err != nil || none != nil {
		t.Errorf("ProjectByFolio(a quote's folio) = %v, %v; want nil", none, err)
	}

	quotes, err := s.ListProjectQuotes(ctx, p.ID)
	if err != nil || len(quotes) != 2 || quotes[0].Folio != "QA0001-R1" || quotes[1].Status != "revisada" {
		t.Errorf("ListProjectQuotes = %+v, %v", quotes, err)
	}
	// The history spans both quotes: the note on the original, the change on the revision.
	comments, err := s.ListProjectComments(ctx, p.ID)
	if err != nil || len(comments) != 2 {
		t.Fatalf("ListProjectComments = %d, %v; want 2", len(comments), err)
	}
	if comments[0].Body != "Probabilidad: Inicial → Media." || comments[0].QuoteFolio != "QA0001-R1" ||
		comments[1].Body != "Primera llamada" || comments[1].QuoteFolio != "QA0001" {
		t.Errorf("comments = %+v", comments)
	}

	folios := func(query, sort, dir string, f Filters) string {
		t.Helper()
		ps, err := s.ListProjects(ctx, query, sort, dir, f)
		if err != nil {
			t.Fatalf("ListProjects: %v", err)
		}
		var out []string
		for _, p := range ps {
			out = append(out, p.Folio)
		}
		return strings.Join(out, ",")
	}
	for _, c := range []struct {
		name, query, sort, dir string
		filters                Filters
		want                   string
	}{
		{"by total", "", "total", "desc", nil, "QA0001,QS0001,QA0002"},
		{"by stage, in lifecycle order", "", "etapa", "desc", nil, "QS0001,QA0001,QA0002"},
		{"search by customer", "constructora", "folio", "asc", nil, "QA0002,QS0001"},
		{"search by folio", "qs", "", "", nil, "QS0001"},
		{"stage filter", "", "folio", "asc", Filters{"etapa": {"prospecto"}}, "QA0001,QA0002"},
		// QS0001 kept its 90 but is past prospecto, so the probability filter skips it.
		{"probability filter", "", "folio", "asc", Filters{"probabilidad": {"90"}}, "QA0002"},
		{"probability filter, two steps", "", "folio", "asc", Filters{"probabilidad": {"50", "90"}}, "QA0001,QA0002"},
		// None is Cerrado · Ganado here (TestDeliveries has one); the filter still runs.
		{"Cerrado · Ganado filter", "", "folio", "asc", Filters{"etapa": {"cerrado"}}, ""},
		{"cobro filter, en curso only", "", "folio", "asc", Filters{"cobro": {"pendiente"}}, "QS0001"},
	} {
		if got := folios(c.query, c.sort, c.dir, c.filters); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}

	customers, owners, err := s.ProjectFilterChoices(ctx)
	if err != nil || strings.Join(customers, ",") != "Constructora X,Grupo PEME" || strings.Join(owners, ",") != "Rodolfo" {
		t.Errorf("ProjectFilterChoices = %v, %v, %v", customers, owners, err)
	}
}

func TestLoseAndReopenProject(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	q, user := projectFixture(t, s)
	project := *q.ProjectID
	load := func() *Project {
		t.Helper()
		p, err := s.ProjectByFolio(ctx, "QA0001")
		if err != nil || p == nil {
			t.Fatalf("ProjectByFolio: %v, %v", p, err)
		}
		return p
	}

	// A reason is required, and the stage has to be the one the proyecto is in.
	for _, c := range []struct{ from, reason string }{
		{"prospecto", ""}, {"prospecto", "   "}, {"en_curso", "No hubo presupuesto"}, {"cerrado", "x"},
	} {
		if err := s.LoseProject(ctx, project, user, c.from, c.reason); !errors.Is(err, ErrBadTransition) {
			t.Errorf("LoseProject(%q, %q) = %v, want ErrBadTransition", c.from, c.reason, err)
		}
	}
	if err := s.ReopenProject(ctx, project, user, nil, ""); !errors.Is(err, ErrBadTransition) {
		t.Errorf("ReopenProject(not lost) = %v, want ErrBadTransition", err)
	}

	if err := s.SetProjectProbability(ctx, project, user, 75, ""); err != nil {
		t.Fatalf("SetProjectProbability: %v", err)
	}
	if err := s.LoseProject(ctx, project, user, "prospecto", "  Se fueron con otro proveedor  "); err != nil {
		t.Fatalf("LoseProject: %v", err)
	}
	p := load()
	if p.Status != "perdido" || p.LostReason != "Se fueron con otro proveedor" || p.LostFrom != "prospecto" || p.LostAt == "" {
		t.Fatalf("lost project = %+v", p)
	}
	// Lost is the end of the line: no forecast, no purchase order, no probability, no
	// revision.
	if p.ForecastRelevant() {
		t.Error("a lost proyecto still counts for the forecast")
	}
	if err := s.ReceiveOC(ctx, project, user, testOC, nil, ""); !errors.Is(err, ErrBadTransition) {
		t.Errorf("ReceiveOC(perdido) = %v, want ErrBadTransition", err)
	}
	if err := s.SetProjectProbability(ctx, project, user, 90, ""); !errors.Is(err, ErrBadTransition) {
		t.Errorf("SetProjectProbability(perdido) = %v, want ErrBadTransition", err)
	}
	if err := s.LoseProject(ctx, project, user, "perdido", "otra vez"); !errors.Is(err, ErrBadTransition) {
		t.Errorf("LoseProject(perdido) = %v, want ErrBadTransition", err)
	}
	if _, err := s.CreateRevision(ctx, q.ID, user); !errors.Is(err, ErrProjectLocked) {
		t.Errorf("CreateRevision(perdido) = %v, want ErrProjectLocked", err)
	}
	// It is off the dashboard: only active proyectos count there.
	dash, err := s.SalesDashboard(ctx, "2026-10-07")
	if err != nil {
		t.Fatalf("SalesDashboard: %v", err)
	}
	for _, row := range dash.Sellers {
		if len(row.Projects) != 0 || row.Pipeline != 0 {
			t.Errorf("a lost proyecto counts for %s: %+v", row.Name, row)
		}
	}

	// Reopening returns it to where it was, with the probability it had.
	if err := s.ReopenProject(ctx, project, user, nil, "Volvieron a llamar"); err != nil {
		t.Fatalf("ReopenProject: %v", err)
	}
	p = load()
	if p.Status != "prospecto" || p.Probability != 75 || p.LostReason != "" || p.LostFrom != "" || p.LostAt != "" {
		t.Fatalf("reopened project = %+v", p)
	}
	comments, err := s.ListProjectComments(ctx, project)
	if err != nil || len(comments) != 3 {
		t.Fatalf("comments = %d, %v; want 3", len(comments), err)
	}
	if comments[0].Body != "Se reabrió como Prospecto.\nVolvieron a llamar" || comments[1].Body != "Se perdió.\nSe fueron con otro proveedor" {
		t.Errorf("comments = %q, %q", comments[0].Body, comments[1].Body)
	}

	// A proyecto lost from prospecto can come back at another probability; a value that
	// isn't a step is refused.
	if err := s.LoseProject(ctx, project, user, "prospecto", "Pausaron el proyecto"); err != nil {
		t.Fatalf("LoseProject: %v", err)
	}
	bad := 100
	if err := s.ReopenProject(ctx, project, user, &bad, ""); !errors.Is(err, ErrBadTransition) {
		t.Errorf("ReopenProject(100) = %v, want ErrBadTransition", err)
	}
	low := 25
	if err := s.ReopenProject(ctx, project, user, &low, "Se reactivó"); err != nil {
		t.Fatalf("ReopenProject(25): %v", err)
	}
	if p = load(); p.Status != "prospecto" || p.Probability != 25 {
		t.Fatalf("reopened at 25: %+v", p)
	}
	comments, err = s.ListProjectComments(ctx, project)
	if err != nil || comments[0].Body != "Se reabrió como Prospecto.\nProbabilidad: Alta → Baja.\nSe reactivó" {
		t.Errorf("reopen comment = %q, %v", comments[0].Body, err)
	}

	// En curso it can't be lost: a sale that falls through then is cancelled.
	if err := s.ReceiveOC(ctx, project, user, testOC, nil, ""); err != nil {
		t.Fatalf("ReceiveOC: %v", err)
	}
	if err := s.LoseProject(ctx, project, user, "en_curso", "Cancelaron la OC"); !errors.Is(err, ErrBadTransition) {
		t.Errorf("LoseProject(en_curso) = %v, want ErrBadTransition", err)
	}

	// One lost with its O.C. in before Cancelado existed (lost_from oc_recibida, migrated
	// to en_curso) is reopened en curso.
	if _, err := s.db.ExecContext(ctx, `UPDATE projects SET status = 'perdido', lost_reason = 'Cancelaron la OC',
		lost_from = 'en_curso', lost_at = '2026-10-09T10:00:00Z' WHERE id = ?`, project); err != nil {
		t.Fatalf("legacy lost: %v", err)
	}
	if err := s.ReopenProject(ctx, project, user, nil, ""); err != nil {
		t.Fatalf("ReopenProject: %v", err)
	}
	if p = load(); p.Status != StageEnCurso {
		t.Fatalf("reopened to %q, want en_curso", p.Status)
	}

	// Losing and reopening are audited with the reason.
	var n int
	if err := s.db.QueryRowContext(ctx, `
		SELECT count(*) FROM audit_log WHERE table_name = 'projects' AND op = 'update'
		AND json_extract(old_values, '$.lost_reason') = 'Cancelaron la OC'`).Scan(&n); err != nil || n != 1 {
		t.Errorf("audit rows with the reason = %d, %v", n, err)
	}
}

func TestFollowUpProject(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	q, user := projectFixture(t, s)
	project := *q.ProjectID
	str := func(v string) *string { return &v }
	num := func(v int) *int { return &v }
	load := func() *Project {
		t.Helper()
		p, err := s.ProjectByFolio(ctx, "QA0001")
		if err != nil || p == nil {
			t.Fatalf("ProjectByFolio: %v, %v", p, err)
		}
		return p
	}

	for name, bad := range map[string]FollowUp{
		"not a step":     {Probability: num(40)},
		"malformed date": {ExpectedOC: str("15/10/2026")},
		"impossible day": {NextFollowUp: str("2026-02-31")},
	} {
		if err := s.FollowUpProject(ctx, project, user, bad); !errors.Is(err, ErrBadTransition) {
			t.Errorf("FollowUpProject(%s) = %v, want ErrBadTransition", name, err)
		}
	}

	err := s.FollowUpProject(ctx, project, user, FollowUp{
		Probability: num(75), ExpectedOC: str("2026-10-15"), NextFollowUp: str("2026-10-09"), Note: "Compras pidió ajustar entrega",
	})
	if err != nil {
		t.Fatalf("FollowUpProject: %v", err)
	}
	p := load()
	if p.Probability != 75 || p.ExpectedOCDate != "2026-10-15" || p.NextFollowUpDate != "2026-10-09" {
		t.Fatalf("project = %+v", p)
	}
	want := "Probabilidad: Inicial → Alta.\nO.C. esperada: 15/10/2026.\nPróximo seguimiento: 09/10/2026.\nCompras pidió ajustar entrega"
	if p.LastBody != want || p.LastUserName != "Rodolfo" || p.LastAt == "" {
		t.Errorf("last comment = %q by %q", p.LastBody, p.LastUserName)
	}
	if p.FollowUpDue("2026-10-08") || !p.FollowUpDue("2026-10-09") || !p.FollowUpDue("2026-10-20") {
		t.Error("FollowUpDue is wrong around the follow-up day")
	}

	// Nil leaves a field alone, the same value says nothing, "" clears a date.
	if err := s.FollowUpProject(ctx, project, user, FollowUp{Probability: num(75), ExpectedOC: str("2026-10-15")}); err != nil {
		t.Fatalf("FollowUpProject(unchanged): %v", err)
	}
	if err := s.FollowUpProject(ctx, project, user, FollowUp{NextFollowUp: str("")}); err != nil {
		t.Fatalf("FollowUpProject(clear): %v", err)
	}
	p = load()
	if p.Probability != 75 || p.ExpectedOCDate != "2026-10-15" || p.NextFollowUpDate != "" || p.LastBody != "Próximo seguimiento: sin fecha." {
		t.Errorf("after clearing = %+v", p)
	}
	if comments, _ := s.ListProjectComments(ctx, project); len(comments) != 2 {
		t.Errorf("comments = %d, want 2 (the unchanged save wrote none)", len(comments))
	}
	if p.FollowUpDue("2026-12-31") {
		t.Error("a prospecto with no follow-up date is due")
	}

	// The dates are audited, and only a prospecto takes a follow-up.
	var n int
	if err := s.db.QueryRowContext(ctx, `
		SELECT count(*) FROM audit_log WHERE table_name = 'projects'
		AND json_extract(new_values, '$.expected_oc_date') = '2026-10-15'`).Scan(&n); err != nil || n == 0 {
		t.Errorf("audit rows with the expected date = %d, %v", n, err)
	}
	if err := s.ReceiveOC(ctx, project, user, testOC, nil, ""); err != nil {
		t.Fatalf("MoveProject: %v", err)
	}
	if err := s.FollowUpProject(ctx, project, user, FollowUp{ExpectedOC: str("2026-11-01")}); !errors.Is(err, ErrBadTransition) {
		t.Errorf("FollowUpProject(en_curso) = %v, want ErrBadTransition", err)
	}
}

func TestListProspectsAndWeights(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	customerID, err := s.CreateCustomer(ctx, Customer{Name: "Grupo PEME", ContactName: "Ing. Pérez", Phone: "55 1234 5678", Email: "perez@peme.mx"})
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	user, err := s.CreateUser(ctx, "rfm", "Rodolfo", "hash", "vendedor")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	insertProject(t, s, "QA0001", customerID, user, "prospecto", 75, 1_000_00)
	insertProject(t, s, "QA0002", customerID, user, "prospecto", 90, 2_000_00)
	insertProject(t, s, "QA0003", customerID, user, "prospecto", 75, 3_000_00)
	insertProject(t, s, "QA0004", customerID, user, "prospecto", 25, 4_000_01)
	insertProject(t, s, "QA0005", customerID, user, "en_curso", 90, 5_000_00)
	if _, err := s.db.ExecContext(ctx, `
		UPDATE projects SET expected_oc_date = CASE folio WHEN 'QA0001' THEN '2026-11-01' WHEN 'QA0003' THEN '2026-10-15' END,
			next_followup_date = CASE folio WHEN 'QA0003' THEN '2026-10-07' WHEN 'QA0004' THEN '2026-10-06' WHEN 'QA0002' THEN '2026-10-08' END;
		UPDATE quotes SET valid_until = '2026-10-30', issued_at = '2026-09-30T15:00:00.000Z' WHERE folio = 'QA0003'`); err != nil {
		t.Fatalf("seed dates: %v", err)
	}
	folios := func(all bool) string {
		t.Helper()
		ps, err := s.ListProspects(ctx, all, "", "", nil)
		if err != nil {
			t.Fatalf("ListProspects: %v", err)
		}
		var out []string
		for _, p := range ps {
			out = append(out, p.Folio)
		}
		return strings.Join(out, ",")
	}
	// Soonest expected O.C. first, undated last; QA0005 is past prospecto.
	if got := folios(false); got != "QA0003,QA0001,QA0002" {
		t.Errorf("forecast prospects = %s", got)
	}
	if got := folios(true); got != "QA0003,QA0001,QA0002,QA0004" {
		t.Errorf("all prospects = %s", got)
	}
	ps, _ := s.ListProspects(ctx, false, "", "", nil)
	if p := ps[0]; p.ContactName != "Ing. Pérez" || p.ContactPhone != "55 1234 5678" || p.ContactEmail != "perez@peme.mx" ||
		p.QuoteValidUntil != "2026-10-30" || p.QuoteIssuedAt == "" || p.LastBody != "" {
		t.Errorf("first prospect = %+v", p)
	}
}

func TestReceiveOC(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	q, user := projectFixture(t, s)
	project := *q.ProjectID
	load := func() *Project {
		t.Helper()
		p, err := s.ProjectByFolio(ctx, "QA0001")
		if err != nil || p == nil {
			t.Fatalf("ProjectByFolio: %v, %v", p, err)
		}
		return p
	}

	// Both pieces of the purchase order are required, and a file can't be empty. The
	// método de pago isn't one of them: it is chosen with the factura (issue #61).
	for name, oc := range map[string]OC{
		"no number":      {Number: "  ", Date: "2026-10-07"},
		"no date":        {Number: "4411"},
		"malformed date": {Number: "4411", Date: "07/10/2026"},
	} {
		if err := s.ReceiveOC(ctx, project, user, oc, nil, ""); !errors.Is(err, ErrBadTransition) {
			t.Errorf("ReceiveOC(%s) = %v, want ErrBadTransition", name, err)
		}
	}
	if err := s.ReceiveOC(ctx, project, user, testOC, &NewFile{Filename: "oc.pdf"}, ""); !errors.Is(err, ErrBadTransition) {
		t.Errorf("ReceiveOC(empty file) = %v, want ErrBadTransition", err)
	}
	if p := load(); p.Status != "prospecto" || p.HasOC() {
		t.Fatalf("after refused attempts = %+v", p)
	}

	// While a revision is in draft there is no issued quote for the O.C. to answer.
	rev, err := s.CreateRevision(ctx, q.ID, user)
	if err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	if err := s.ReceiveOC(ctx, project, user, testOC, nil, ""); !errors.Is(err, ErrBadTransition) {
		t.Fatalf("ReceiveOC with a draft revision = %v, want ErrBadTransition", err)
	}
	if err := s.IssueQuote(ctx, rev.ID, testIssue("sha2")); err != nil {
		t.Fatalf("IssueQuote(revision): %v", err)
	}

	pdf := &NewFile{Filename: " OC-4411.pdf ", ContentType: "application/pdf", Data: []byte("%PDF-1.7 one")}
	if err := s.ReceiveOC(ctx, project, user, OC{Number: " 4411 ", Date: "2026-10-07"}, pdf, "Llegó por correo"); err != nil {
		t.Fatalf("ReceiveOC: %v", err)
	}
	p := load()
	if p.Status != "en_curso" || p.OC != (OC{Number: "4411", Date: "2026-10-07"}) || !p.HasOC() || p.PaymentMethod != "" {
		t.Fatalf("after ReceiveOC = %+v", p)
	}
	want := "Pasó a En curso.\nO.C. 4411 · 07/10/2026\nArchivo de la O.C.: OC-4411.pdf\nLlegó por correo"
	if p.LastBody != want {
		t.Errorf("history = %q", p.LastBody)
	}
	// Who received it and when come from the audit log.
	if r, err := s.ProjectOCReceipt(ctx, project); err != nil || r.UserName != "Rodolfo" || r.At == "" {
		t.Errorf("ProjectOCReceipt = %+v, %v", r, err)
	}

	// En curso it corrects the data; saving the same thing says nothing.
	if err := s.ReceiveOC(ctx, project, user, p.OC, nil, ""); err != nil {
		t.Fatalf("ReceiveOC(unchanged): %v", err)
	}
	if got := load().LastBody; got != want {
		t.Errorf("an unchanged save wrote to the history: %q", got)
	}
	corrected := &NewFile{Filename: "OC-4411-corregida.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1.7 two")}
	if err := s.ReceiveOC(ctx, project, user, OC{Number: "4411-A", Date: "2026-10-08"}, corrected, ""); err != nil {
		t.Fatalf("ReceiveOC(correction): %v", err)
	}
	p = load()
	if p.OC.Number != "4411-A" ||
		p.LastBody != "O.C. actualizada: 4411-A · 08/10/2026\nArchivo de la O.C.: OC-4411-corregida.pdf" {
		t.Errorf("after correction = %+v", p)
	}

	// Both files are kept, newest first, and only served to their own proyecto.
	files, err := s.ListProjectFiles(ctx, project, "oc")
	if err != nil || len(files) != 2 || files[0].Filename != "OC-4411-corregida.pdf" || files[1].Filename != "OC-4411.pdf" ||
		files[1].Size != int64(len(pdf.Data)) || files[0].UploadedByName != "Rodolfo" {
		t.Fatalf("ListProjectFiles = %+v, %v", files, err)
	}
	f, data, err := s.ProjectFileData(ctx, project, files[1].ID)
	if err != nil || f == nil || string(data) != "%PDF-1.7 one" || f.ContentType != "application/pdf" {
		t.Errorf("ProjectFileData = %+v, %q, %v", f, data, err)
	}
	if f, _, err := s.ProjectFileData(ctx, project+1, files[1].ID); err != nil || f != nil {
		t.Errorf("another proyecto's file = %+v, %v; want nil", f, err)
	}

	// Undone by an admin, it goes back to prospecto without the order's number and date;
	// receiving it again is ReceiveOC as the first time, and the files stay.
	if err := s.UndoReceiveOC(ctx, project, user, "Era de otro proyecto"); err != nil {
		t.Fatalf("UndoReceiveOC: %v", err)
	}
	if p = load(); p.Status != StageProspecto || p.HasOC() || p.Probability != ReceiveOCProbability {
		t.Fatalf("after undo = %+v", p)
	}
	if p.LastBody != "Se deshizo la O.C. recibida (O.C. 4411-A · 08/10/2026).\nRegresó a Prospecto · Inminente 90%.\nEra de otro proyecto" {
		t.Errorf("undo history = %q", p.LastBody)
	}
	if err := s.ReceiveOC(ctx, project, user, testOC, nil, ""); err != nil {
		t.Fatalf("ReceiveOC again: %v", err)
	}
	if files, _ := s.ListProjectFiles(ctx, project, "oc"); len(files) != 2 {
		t.Errorf("files after undo and receiving again = %d, want 2", len(files))
	}
	if err := s.RecordInvoice(ctx, project, user, testInvoice, "PUE", ""); err != nil {
		t.Fatalf("RecordInvoice: %v", err)
	}
	var n int
	if err := s.db.QueryRowContext(ctx, `
		SELECT count(*) FROM audit_log WHERE table_name = 'projects'
		AND json_extract(new_values, '$.oc_number') = '4411' AND json_extract(new_values, '$.payment_method') = 'PUE'`).Scan(&n); err != nil || n == 0 {
		t.Errorf("audit rows with the purchase order = %d, %v", n, err)
	}
}

// TestTypedInvoice covers the factura typed in by hand while facturación can't stamp: it
// counts for the Estado de facturación, never moves the stage, and records no payment.
// The método is chosen when the factura is recorded and kept on the proyecto.
func TestTypedInvoice(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	q, user := projectFixture(t, s)
	project := *q.ProjectID
	if err := s.ReceiveOC(ctx, project, user, testOC, nil, ""); err != nil {
		t.Fatalf("ReceiveOC: %v", err)
	}
	for name, bad := range map[string]Document{"no folio": {Ref: " ", Date: "2026-10-08"}, "no date": {Ref: "F-1"}, "bad date": {Ref: "F-1", Date: "8/10/26"}} {
		if err := s.RecordInvoice(ctx, project, user, bad, "PUE", ""); !errors.Is(err, ErrBadTransition) {
			t.Errorf("RecordInvoice(%s) = %v, want ErrBadTransition", name, err)
		}
	}
	for _, method := range []string{"", "contado"} {
		if err := s.RecordInvoice(ctx, project, user, testInvoice, method, ""); !errors.Is(err, ErrBadTransition) {
			t.Errorf("RecordInvoice(método %q) = %v, want ErrBadTransition", method, err)
		}
	}
	if err := s.RecordInvoice(ctx, project, user, Document{Ref: " A-77 ", Date: "2026-10-08"}, "PPD", "Transferencia"); err != nil {
		t.Fatalf("RecordInvoice: %v", err)
	}
	p, _ := s.ProjectByFolio(ctx, "QA0001")
	if p.Status != StageEnCurso || p.Invoice != (Document{Ref: "A-77", Date: "2026-10-08"}) || p.PaymentMethod != "PPD" ||
		p.States.Facturacion != FacturacionFacturada || p.States.Cobro != CobroPendiente {
		t.Fatalf("after RecordInvoice = %+v", p)
	}
	if p.LastBody != "Factura registrada: A-77 · 08/10/2026 · P.P.D.\nTransferencia" {
		t.Errorf("history = %q", p.LastBody)
	}

	// A proyecto with no purchase order on record can't be invoiced.
	other := insertProject(t, s, "QS0001", q.CustomerID, user, "en_curso", 90, 100_00)
	var otherProject int64
	if err := s.db.QueryRowContext(ctx, `SELECT project_id FROM quotes WHERE id = ?`, other).Scan(&otherProject); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordInvoice(ctx, otherProject, user, testInvoice, "PUE", ""); !errors.Is(err, ErrBadTransition) {
		t.Errorf("RecordInvoice without O.C. = %v, want ErrBadTransition", err)
	}
}

// TestPayments records and deletes the client's payments (#67): what they add up to, the
// balance they can't go past, the comprobante, and the history.
func TestPayments(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	q, user := projectFixture(t, s)
	project := *q.ProjectID
	if _, err := s.db.ExecContext(ctx, `UPDATE quotes SET subtotal = 1287582, iva = 206013, total = 1493595 WHERE id = ?`, q.ID); err != nil {
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
	// Only en curso.
	if err := s.RecordPayment(ctx, project, user, 100, "2026-11-10", nil); !errors.Is(err, ErrBadTransition) {
		t.Errorf("RecordPayment(prospecto) = %v, want ErrBadTransition", err)
	}
	if err := s.ReceiveOC(ctx, project, user, testOC, nil, ""); err != nil {
		t.Fatalf("ReceiveOC: %v", err)
	}
	for name, c := range map[string]struct {
		amount money.Centavos
		day    string
		file   *NewFile
	}{
		"zero":       {0, "2026-11-10", nil},
		"negative":   {-5, "2026-11-10", nil},
		"no date":    {100, "", nil},
		"empty file": {100, "2026-11-10", &NewFile{Filename: "cp.pdf"}},
	} {
		if err := s.RecordPayment(ctx, project, user, c.amount, c.day, c.file); !errors.Is(err, ErrBadTransition) {
			t.Errorf("RecordPayment(%s) = %v, want ErrBadTransition", name, err)
		}
	}
	if err := s.RecordPayment(ctx, project, user, 1493596, "2026-11-10", nil); !errors.Is(err, ErrPaymentOverBalance) {
		t.Errorf("RecordPayment(over the total) = %v, want ErrPaymentOverBalance", err)
	}

	// Half, without a comprobante: Pago parcial.
	if err := s.RecordPayment(ctx, project, user, 746798, "2026-11-09", nil); err != nil {
		t.Fatalf("RecordPayment: %v", err)
	}
	p := load()
	if p.States.Cobro != CobroParcial || p.States.Paid != 746798 || p.Balance() != 746797 || p.Stage() != StageEnCurso {
		t.Fatalf("after the first payment = %+v, balance %s", p.States, p.Balance())
	}
	if p.LastBody != "Pago registrado: $7,467.98 el 09/11/2026 por Rodolfo." {
		t.Errorf("history = %q", p.LastBody)
	}
	if err := s.RecordPayment(ctx, project, user, 746798, "2026-11-10", nil); !errors.Is(err, ErrPaymentOverBalance) {
		t.Errorf("RecordPayment(over the balance) = %v, want ErrPaymentOverBalance", err)
	}

	// The rest, with its comprobante: Pagado, as of the last payment.
	cp := &NewFile{Filename: "transferencia.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1.7 cp")}
	if err := s.RecordPayment(ctx, project, user, 746797, "2026-11-10", cp); err != nil {
		t.Fatalf("RecordPayment(rest): %v", err)
	}
	if p = load(); p.States.Cobro != CobroPagado || p.States.Paid != 1493595 || p.Balance() != 0 || p.States.LastPaidOn != "2026-11-10" {
		t.Fatalf("after paying it all = %+v", p.States)
	}
	if p.LastBody != "Pago registrado: $7,467.97 el 10/11/2026 por Rodolfo con comprobante." {
		t.Errorf("history = %q", p.LastBody)
	}
	payments, err := s.ListPayments(ctx, project)
	if err != nil || len(payments) != 2 || payments[0].Amount != 746798 || payments[0].FileID != 0 ||
		payments[1].FileName != "transferencia.pdf" || payments[1].CreatedByName != "Rodolfo" || payments[1].Source != PaymentSourceManual {
		t.Fatalf("ListPayments = %+v, %v", payments, err)
	}
	if f, data, err := s.ProjectFileData(ctx, project, payments[1].FileID); err != nil || f == nil || f.Kind != "pago" || string(data) != "%PDF-1.7 cp" {
		t.Errorf("comprobante file = %+v, %q, %v", f, data, err)
	}

	// Deleting one needs a motivo, takes it out of the sums and says so in the history.
	if err := s.DeletePayment(ctx, project, payments[0].ID, user, " "); !errors.Is(err, ErrBadTransition) {
		t.Errorf("DeletePayment(no reason) = %v, want ErrBadTransition", err)
	}
	if err := s.DeletePayment(ctx, project+1, payments[0].ID, user, "x"); !errors.Is(err, ErrBadTransition) {
		t.Errorf("DeletePayment(another proyecto's) = %v, want ErrBadTransition", err)
	}
	if err := s.DeletePayment(ctx, project, payments[0].ID, user, "Se capturó dos veces"); err != nil {
		t.Fatalf("DeletePayment: %v", err)
	}
	if p = load(); p.States.Cobro != CobroParcial || p.States.Paid != 746797 ||
		p.LastBody != "Pago eliminado: $7,467.98 del 09/11/2026.\nSe capturó dos veces" {
		t.Fatalf("after deleting = %+v, %q", p.States, p.LastBody)
	}
	if err := s.DeletePayment(ctx, project, payments[0].ID, user, "otra vez"); !errors.Is(err, ErrBadTransition) {
		t.Errorf("DeletePayment(twice) = %v, want ErrBadTransition", err)
	}
	if payments, _ := s.ListPayments(ctx, project); len(payments) != 1 {
		t.Errorf("payments after deleting = %d, want 1", len(payments))
	}
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM audit_log WHERE table_name = 'payments' AND op = 'update'
		AND json_extract(new_values, '$.delete_reason') = 'Se capturó dos veces'`).Scan(&n); err != nil || n != 1 {
		t.Errorf("audit rows for the deletion = %d, %v", n, err)
	}

	// Deleting the proyecto takes its payments with it.
	if err := s.DeleteProject(ctx, project); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM payments`).Scan(&n); err != nil || n != 0 {
		t.Errorf("payments after deleting the proyecto = %d, %v", n, err)
	}
}

// TestProjectStates checks the project_states view against the records it reads: a
// stamped factura (P.U.E., or P.P.D. and its complementos), a confirmed supplier O.C.,
// and the payments.
func TestProjectStates(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	q, user := projectFixture(t, s)
	project := *q.ProjectID
	if _, err := s.db.ExecContext(ctx, `UPDATE quotes SET subtotal = 10000, iva = 1600, total = 11600 WHERE id = ?`, q.ID); err != nil {
		t.Fatalf("total: %v", err)
	}
	if err := s.ReceiveOC(ctx, project, user, testOC, nil, ""); err != nil {
		t.Fatalf("ReceiveOC: %v", err)
	}
	exec := func(query string, args ...any) int64 {
		t.Helper()
		res, err := s.db.ExecContext(ctx, query, args...)
		if err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	states := func() ProjectStates {
		t.Helper()
		p, err := s.ProjectByFolio(ctx, "QA0001")
		if err != nil || p == nil {
			t.Fatalf("ProjectByFolio: %v, %v", p, err)
		}
		return p.States
	}

	// Facturación: a draft doesn't count; a stamped P.P.D. factura waits for its
	// complementos, which count once stamped and only up to the factura's total.
	factura := exec(`INSERT INTO comprobantes (tipo, project_id, quote_id, metodo_pago, total, created_by) VALUES ('factura', ?, ?, 'PPD', 11600, ?)`, project, q.ID, user)
	if got := states().Facturacion; got != FacturacionPendiente {
		t.Errorf("with a draft = %q", got)
	}
	exec(`UPDATE comprobantes SET status = 'timbrada', uuid = 'U-1' WHERE id = ?`, factura)
	if got := states().Facturacion; got != FacturacionPendienteComplemento {
		t.Errorf("P.P.D. stamped = %q", got)
	}
	exec(`INSERT INTO comprobantes (tipo, project_id, related_id, status, total, uuid, created_by) VALUES ('pago', ?, ?, 'timbrada', 5800, 'U-2', ?)`, project, factura, user)
	exec(`INSERT INTO comprobantes (tipo, project_id, related_id, status, total, created_by) VALUES ('pago', ?, ?, 'borrador', 5800, ?)`, project, factura, user)
	if got := states().Facturacion; got != FacturacionPendienteComplemento {
		t.Errorf("half covered = %q", got)
	}
	exec(`INSERT INTO comprobantes (tipo, project_id, related_id, status, total, uuid, created_by) VALUES ('pago', ?, ?, 'timbrada', 5800, 'U-3', ?)`, project, factura, user)
	if got := states().Facturacion; got != FacturacionFacturada {
		t.Errorf("covered = %q", got)
	}
	// Cancelled, it is pendiente again.
	exec(`UPDATE comprobantes SET status = 'cancelada' WHERE id = ?`, factura)
	if got := states().Facturacion; got != FacturacionPendiente {
		t.Errorf("cancelled = %q", got)
	}
	exec(`INSERT INTO comprobantes (tipo, project_id, quote_id, status, metodo_pago, total, uuid, created_by) VALUES ('factura', ?, ?, 'timbrada', 'PUE', 11600, 'U-4', ?)`, project, q.ID, user)
	if got := states().Facturacion; got != FacturacionFacturada {
		t.Errorf("P.U.E. stamped = %q", got)
	}

	// Entrega: a confirmed supplier O.C. puts it in process; a borrador doesn't.
	po := exec(`INSERT INTO purchase_orders (project_id, quote_id, created_by) VALUES (?, ?, ?)`, project, q.ID, user)
	if got := states().Entrega; got != EntregaSinOC {
		t.Errorf("with a borrador O.C. = %q", got)
	}
	exec(`UPDATE purchase_orders SET status = 'confirmada', seq = 1, number = 'OC-QA0001-071026', document = '{}' WHERE id = ?`, po)
	if got := states().Entrega; got != EntregaEnProceso {
		t.Errorf("with a confirmed O.C. = %q", got)
	}

	// Cobro: the payments not deleted against the total with IVA.
	exec(`INSERT INTO payments (project_id, amount, paid_on, created_by) VALUES (?, 5000, '2026-10-10', ?)`, project, user)
	exec(`INSERT INTO payments (project_id, amount, paid_on, created_by, deleted_by, deleted_at, delete_reason)
		VALUES (?, 6600, '2026-10-11', ?, ?, '2026-10-12T00:00:00Z', 'Duplicado')`, project, user, user)
	if got := states(); got.Cobro != CobroParcial || got.Paid != 5000 || got.LastPaidOn != "2026-10-10" {
		t.Errorf("partly paid = %+v", got)
	}
	exec(`INSERT INTO payments (project_id, amount, paid_on, created_by) VALUES (?, 6600, '2026-10-15', ?)`, project, user)
	got := states()
	if got.Cobro != CobroPagado || got.Paid != 11600 || got.LastPaidOn != "2026-10-15" {
		t.Errorf("paid = %+v", got)
	}
	// Facturada and pagado, but its O.C. isn't delivered: not closed yet (TestDeliveries closes one).
	if got.Cerrado {
		t.Error("closed without being delivered")
	}
}

// TestAdminStageChanges covers Cancelado, Reactivar and undoing the O.C., with what
// blocks each.
func TestAdminStageChanges(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	q, user := projectFixture(t, s)
	project := *q.ProjectID
	load := func() *Project {
		t.Helper()
		p, err := s.ProjectByFolio(ctx, "QA0001")
		if err != nil || p == nil {
			t.Fatalf("ProjectByFolio: %v, %v", p, err)
		}
		return p
	}

	// A motivo is required, and only a cancelled proyecto is reactivated.
	if err := s.CancelProject(ctx, project, user, "  "); !errors.Is(err, ErrBadTransition) {
		t.Errorf("CancelProject(no reason) = %v, want ErrBadTransition", err)
	}
	if err := s.ReactivateProject(ctx, project, user, "x"); !errors.Is(err, ErrBadTransition) {
		t.Errorf("ReactivateProject(prospecto) = %v, want ErrBadTransition", err)
	}
	if err := s.UndoReceiveOC(ctx, project, user, "x"); !errors.Is(err, ErrBadTransition) {
		t.Errorf("UndoReceiveOC(prospecto) = %v, want ErrBadTransition", err)
	}

	if err := s.ReceiveOC(ctx, project, user, testOC, nil, ""); err != nil {
		t.Fatalf("ReceiveOC: %v", err)
	}
	if err := s.CancelProject(ctx, project, user, " Se cayó el pedido "); err != nil {
		t.Fatalf("CancelProject: %v", err)
	}
	p := load()
	if p.Status != StageCancelado || p.Stage() != StageCancelado || p.CancelReason != "Se cayó el pedido" ||
		p.CancelledFrom != StageEnCurso || p.CancelledAt == "" || p.LastBody != "Se canceló.\nSe cayó el pedido" {
		t.Fatalf("cancelled = %+v", p)
	}
	if err := s.CancelProject(ctx, project, user, "otra vez"); !errors.Is(err, ErrBadTransition) {
		t.Errorf("CancelProject(cancelado) = %v, want ErrBadTransition", err)
	}
	if err := s.ReactivateProject(ctx, project, user, "  "); !errors.Is(err, ErrBadTransition) {
		t.Errorf("ReactivateProject(no reason) = %v, want ErrBadTransition", err)
	}
	if err := s.ReactivateProject(ctx, project, user, "Siempre sí"); err != nil {
		t.Fatalf("ReactivateProject: %v", err)
	}
	if p = load(); p.Status != StageEnCurso || p.CancelReason != "" || p.CancelledFrom != "" || p.CancelledAt != "" ||
		p.LastBody != "Se reactivó como En curso.\nSiempre sí" || !p.HasOC() {
		t.Fatalf("reactivated = %+v", p)
	}

	// A confirmed supplier O.C. blocks both; so does a stamped factura, and a typed one
	// blocks the undo only.
	po, err := s.db.ExecContext(ctx, `INSERT INTO purchase_orders (project_id, quote_id, created_by, status, seq, number, document)
		VALUES (?, ?, ?, 'confirmada', 1, 'OC-1', '{}')`, project, q.ID, user)
	if err != nil {
		t.Fatalf("PO: %v", err)
	}
	poID, _ := po.LastInsertId()
	if err := s.CancelProject(ctx, project, user, "x"); !errors.Is(err, ErrHasConfirmedPO) {
		t.Errorf("CancelProject(confirmed O.C.) = %v, want ErrHasConfirmedPO", err)
	}
	if err := s.UndoReceiveOC(ctx, project, user, "x"); !errors.Is(err, ErrHasConfirmedPO) {
		t.Errorf("UndoReceiveOC(confirmed O.C.) = %v, want ErrHasConfirmedPO", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE purchase_orders SET status = 'cancelada' WHERE id = ?`, poID); err != nil {
		t.Fatalf("cancel PO: %v", err)
	}
	if err := s.RecordInvoice(ctx, project, user, testInvoice, "PPD", ""); err != nil {
		t.Fatalf("RecordInvoice: %v", err)
	}
	if err := s.UndoReceiveOC(ctx, project, user, "x"); !errors.Is(err, ErrHasFactura) {
		t.Errorf("UndoReceiveOC(typed factura) = %v, want ErrHasFactura", err)
	}
	factura, err := s.db.ExecContext(ctx, `INSERT INTO comprobantes (tipo, project_id, quote_id, status, uuid, created_by)
		VALUES ('factura', ?, ?, 'timbrada', 'U-1', ?)`, project, q.ID, user)
	if err != nil {
		t.Fatalf("factura: %v", err)
	}
	facturaID, _ := factura.LastInsertId()
	if err := s.CancelProject(ctx, project, user, "x"); !errors.Is(err, ErrHasFactura) {
		t.Errorf("CancelProject(stamped factura) = %v, want ErrHasFactura", err)
	}
	// Once the factura is cancelled and the typed one cleared, the undo goes through and
	// takes the factura drafts with it.
	if _, err := s.db.ExecContext(ctx, `UPDATE comprobantes SET status = 'cancelada' WHERE id = ?`, facturaID); err != nil {
		t.Fatalf("cancel factura: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE projects SET invoice_ref = NULL, invoice_date = NULL WHERE id = ?`, project); err != nil {
		t.Fatalf("clear typed: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO comprobantes (tipo, project_id, quote_id, created_by) VALUES ('factura', ?, ?, ?)`, project, q.ID, user); err != nil {
		t.Fatalf("draft: %v", err)
	}
	if err := s.UndoReceiveOC(ctx, project, user, "Era de otro cliente"); err != nil {
		t.Fatalf("UndoReceiveOC: %v", err)
	}
	var drafts, cancelled int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FILTER (WHERE status = 'borrador'), count(*) FILTER (WHERE status = 'cancelada')
		FROM comprobantes WHERE project_id = ?`, project).Scan(&drafts, &cancelled); err != nil || drafts != 0 || cancelled != 1 {
		t.Errorf("comprobantes after undo: %d drafts, %d cancelled, %v", drafts, cancelled, err)
	}
	if p = load(); p.Status != StageProspecto || p.Probability != 90 || p.HasOC() {
		t.Fatalf("undone = %+v", p)
	}
	// Its quote can be revised again, and a prospecto can be cancelled too, and
	// reactivated as one.
	if p.ForecastRelevant() != true {
		t.Error("an undone proyecto at Inminente isn't relevante para pronóstico")
	}
	if err := s.CancelProject(ctx, project, user, "Pausado"); err != nil {
		t.Fatalf("CancelProject(prospecto): %v", err)
	}
	if err := s.ReactivateProject(ctx, project, user, "Volvió"); err != nil {
		t.Fatalf("ReactivateProject: %v", err)
	}
	if p = load(); p.Status != StageProspecto {
		t.Fatalf("reactivated to %q, want prospecto", p.Status)
	}

	// Each is audited with its motivo.
	var n int
	if err := s.db.QueryRowContext(ctx, `
		SELECT count(*) FROM audit_log WHERE table_name = 'projects' AND op = 'update'
		AND json_extract(new_values, '$.cancel_reason') = 'Se cayó el pedido'
		AND json_extract(new_values, '$.cancelled_from') = 'en_curso'`).Scan(&n); err != nil || n != 1 {
		t.Errorf("audit rows with the cancellation = %d, %v", n, err)
	}
}

// TestMigration0041 opens a database as it was before the stages became prospecto / en
// curso / perdido / cancelado, with one proyecto in each old stage, and checks what each
// became, and that a proyecto marked pagado by hand has its payment.
func TestMigration0041(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "up.db")
	old, err := Open(ctx, dsn, migrationsBefore(t, "0041"))
	if err != nil {
		t.Fatalf("open at 0040: %v", err)
	}
	q, user := projectFixture(t, old)
	for i, st := range []string{"oc_recibida", "facturado", "en_entrega", "cerrado", "perdido"} {
		folio := fmt.Sprintf("QS%04d", i+1)
		insertProject(t, old, folio, q.CustomerID, user, st, 90, 100_00)
	}
	for _, stmt := range []string{
		`UPDATE projects SET lost_reason = 'Se cayó', lost_from = 'oc_recibida', lost_at = '2026-10-01T00:00:00Z' WHERE folio = 'QS0005'`,
		`UPDATE projects SET oc_number = '4411', oc_date = '2026-10-29', payment_method = 'PUE', invoice_ref = 'UUID-A',
			invoice_date = '2026-10-30', payment_status = 'pagado', paid_at = '2026-10-30' WHERE folio = 'QS0004'`,
		`INSERT INTO project_files (project_id, kind, filename, content_type, size, data, uploaded_by)
			SELECT id, 'oc', 'oc.pdf', 'application/pdf', 3, X'414243', ` + fmt.Sprint(user) + ` FROM projects WHERE folio = 'QS0004'`,
	} {
		if _, err := old.db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed %q: %v", stmt, err)
		}
	}
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
	for folio, want := range map[string]string{
		"QA0001": "prospecto", "QS0001": "en_curso", "QS0002": "en_curso", "QS0003": "en_curso", "QS0004": "en_curso", "QS0005": "perdido",
	} {
		p, err := s.ProjectByFolio(ctx, folio)
		if err != nil || p == nil || p.Status != want {
			t.Errorf("%s = %+v, %v; want %s", folio, p, err, want)
		}
	}
	if p, _ := s.ProjectByFolio(ctx, "QS0005"); p.LostFrom != StageEnCurso {
		t.Errorf("lost from = %q, want en_curso", p.LostFrom)
	}
	// The one closed by hand: facturada by its typed folio, its payment carried over for
	// the quote's total on the day it was paid, and still to be delivered.
	p, _ := s.ProjectByFolio(ctx, "QS0004")
	if p.States != (ProjectStates{Facturacion: "facturada", Entrega: "sin_oc", Cobro: "pagado", Paid: 116_00, LastPaidOn: "2026-10-30"}) ||
		p.Stage() != StageEnCurso || p.Invoice.Ref != "UUID-A" {
		t.Errorf("QS0004 = %q, %+v", p.Stage(), p.States)
	}
	var source string
	var createdBy int64
	if err := s.db.QueryRowContext(ctx, `SELECT source, created_by FROM payments WHERE project_id = ?`, p.ID).Scan(&source, &createdBy); err != nil ||
		source != "manual" || createdBy != p.UserID {
		t.Errorf("migrated payment = %q by %d, %v", source, createdBy, err)
	}
	if files, err := s.ListProjectFiles(ctx, p.ID, "oc"); err != nil || len(files) != 1 {
		t.Errorf("files = %d, %v", len(files), err)
	}
	// The new stages and file kinds are accepted, the old ones refused.
	if _, err := s.db.ExecContext(ctx, `UPDATE projects SET status = 'cancelado' WHERE id = ?`, p.ID); err != nil {
		t.Errorf("cancelado refused: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE projects SET status = 'facturado' WHERE id = ?`, p.ID); err == nil {
		t.Error("projects.status still accepts facturado")
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE project_files SET kind = 'pago' WHERE project_id = ?`, p.ID); err != nil {
		t.Errorf("kind pago refused: %v", err)
	}
	// The audit triggers came back with the new columns.
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM audit_log WHERE table_name = 'projects'
		AND json_extract(new_values, '$.status') = 'cancelado' AND json_type(new_values, '$.cancel_reason') = 'null'`).Scan(&n); err != nil || n != 1 {
		t.Errorf("audit rows for the new stage = %d, %v", n, err)
	}
}
