package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/gerrygoo/cladex-web"
)

func TestNextAndPrevStage(t *testing.T) {
	for _, c := range []struct{ stage, next, prev string }{
		{"", "", ""},
		{"emitida", "", ""},
		{"perdido", "", ""},
		{"prospecto", "oc_recibida", ""},
		{"oc_recibida", "en_entrega", "prospecto"},
		{"en_entrega", "cerrado", "oc_recibida"},
		{"cerrado", "", "en_entrega"},
	} {
		if got := NextStage(c.stage); got != c.next {
			t.Errorf("NextStage(%q) = %q, want %q", c.stage, got, c.next)
		}
		if got := PrevStage(c.stage); got != c.prev {
			t.Errorf("PrevStage(%q) = %q, want %q", c.stage, got, c.prev)
		}
	}
}

func TestForecastRelevant(t *testing.T) {
	for _, c := range []struct {
		stage string
		p     int
		want  bool
	}{
		{"prospecto", 10, false}, {"prospecto", 50, false}, {"prospecto", 75, true}, {"prospecto", 90, true},
		{"oc_recibida", 90, false}, {"", 90, false},
	} {
		if got := ForecastRelevant(c.stage, c.p); got != c.want {
			t.Errorf("ForecastRelevant(%q, %d) = %v, want %v", c.stage, c.p, got, c.want)
		}
	}
	if ProbabilityLabel(75) != "Alta" || ProbabilityLabel(40) != "" {
		t.Errorf("ProbabilityLabel(75) = %q, ProbabilityLabel(40) = %q", ProbabilityLabel(75), ProbabilityLabel(40))
	}
}

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
	if q.Status != "emitida" || q.DisplayStatus() != "prospecto" {
		t.Fatalf("status = %q, display = %q", q.Status, q.DisplayStatus())
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
	if rev.ProjectID == nil || *rev.ProjectID != *q.ProjectID || rev.DisplayStatus() != "borrador" {
		t.Fatalf("revision = %+v", rev)
	}
	if err := s.IssueQuote(ctx, rev.ID, testIssue("sha2")); err != nil {
		t.Fatalf("IssueQuote(revision): %v", err)
	}
	var projects int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM projects`).Scan(&projects); err != nil || projects != 1 {
		t.Fatalf("projects = %d, %v; want 1", projects, err)
	}
	if old, _ := s.QuoteByID(ctx, q.ID); old.DisplayStatus() != "revisada" {
		t.Fatalf("superseded quote shows %q", old.DisplayStatus())
	}

	// The proyecto is audited: opened by the issuing user.
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM audit_log WHERE table_name = 'projects' AND op = 'insert'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("project audit inserts = %d, %v", n, err)
	}
}

func TestMoveProject(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	q, user := projectFixture(t, s)
	project := *q.ProjectID
	stage := func() string {
		q, err := s.QuoteByID(ctx, q.ID)
		if err != nil || q == nil {
			t.Fatalf("QuoteByID: %v", err)
		}
		return q.Stage
	}

	// Skipping a stage, or a stage the proyecto isn't in, is refused and changes nothing.
	for _, c := range []struct{ from, to string }{
		{"prospecto", "en_entrega"}, {"prospecto", "prospecto"}, {"prospecto", ""}, {"prospecto", "perdido"},
		{"prospecto", "facturado"},
		{"oc_recibida", "en_entrega"}, // right step, but the proyecto is still a prospecto
	} {
		if err := s.MoveProject(ctx, project, user, c.from, c.to, ""); !errors.Is(err, ErrBadTransition) {
			t.Errorf("MoveProject(%q → %q) = %v, want ErrBadTransition", c.from, c.to, err)
		}
	}
	if got := stage(); got != "prospecto" {
		t.Fatalf("stage after refused moves = %q", got)
	}

	if err := s.MoveProject(ctx, project, user, "prospecto", "oc_recibida", "Llegó la OC 4411"); err != nil {
		t.Fatalf("MoveProject forward: %v", err)
	}
	if got := stage(); got != "oc_recibida" {
		t.Fatalf("stage = %q, want oc_recibida", got)
	}
	// Once the purchase order is in, the quote is locked.
	if _, err := s.CreateRevision(ctx, q.ID, user); !errors.Is(err, ErrProjectLocked) {
		t.Errorf("CreateRevision(oc_recibida) = %v, want ErrProjectLocked", err)
	}
	if err := s.MoveProject(ctx, project, user, "oc_recibida", "prospecto", ""); err != nil {
		t.Fatalf("MoveProject back: %v", err)
	}
	for _, to := range []string{"oc_recibida", "en_entrega", "cerrado"} {
		from := PrevStage(to)
		if err := s.MoveProject(ctx, project, user, from, to, ""); err != nil {
			t.Fatalf("MoveProject %s → %s: %v", from, to, err)
		}
	}
	if err := s.MoveProject(ctx, project, user, "cerrado", "", ""); !errors.Is(err, ErrBadTransition) {
		t.Errorf("MoveProject past the last stage = %v", err)
	}
	// The quote itself never left emitida.
	if got, _ := s.QuoteByID(ctx, q.ID); got.Status != "emitida" || got.DisplayStatus() != "cerrado" {
		t.Errorf("quote status = %q, display = %q", got.Status, got.DisplayStatus())
	}

	// Each move left a comment, the first with the user's note.
	comments, err := s.ListQuoteComments(ctx, q.ID)
	if err != nil {
		t.Fatalf("ListQuoteComments: %v", err)
	}
	if len(comments) != 5 {
		t.Fatalf("comments = %d, want 5", len(comments))
	}
	if last := comments[len(comments)-1]; last.Body != "Pasó a O.C. recibida.\nLlegó la OC 4411" {
		t.Errorf("first comment = %q", last.Body)
	}
	if comments[len(comments)-2].Body != "Regresó a Prospecto." {
		t.Errorf("second comment = %q", comments[len(comments)-2].Body)
	}
	if comments[0].Body != "Pasó a Cerrado." {
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
	if err := s.MoveProject(ctx, project, user, "prospecto", "oc_recibida", ""); !errors.Is(err, ErrBadTransition) {
		t.Fatalf("MoveProject with a draft revision = %v, want ErrBadTransition", err)
	}
	if err := s.IssueQuote(ctx, rev.ID, testIssue("sha2")); err != nil {
		t.Fatalf("IssueQuote(revision): %v", err)
	}
	if err := s.MoveProject(ctx, project, user, "prospecto", "oc_recibida", ""); err != nil {
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
	if err := s.MoveProject(ctx, project, user, "prospecto", "oc_recibida", ""); err != nil {
		t.Fatalf("MoveProject: %v", err)
	}
	if err := s.SetProjectProbability(ctx, project, user, 90, ""); !errors.Is(err, ErrBadTransition) {
		t.Errorf("SetProjectProbability(oc_recibida) = %v, want ErrBadTransition", err)
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
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM audit_log`).Scan(&auditAfter); err != nil || auditAfter != auditBefore {
		t.Fatalf("audit_log rows = %d, were %d (%v)", auditAfter, auditBefore, err)
	}

	for _, c := range []struct {
		folio, status, project, stage string
		probability                   int
	}{
		{"QA0001", "borrador", "", "", 0},
		{"QA0002", "emitida", "QA0002", "prospecto", 10},
		{"QA0003", "emitida", "QA0003", "prospecto", 75},
		{"QS0001", "emitida", "QS0001", "oc_recibida", 10},
		{"QA0004", "emitida", "QA0004", "en_entrega", 10},
		{"QA0005", "emitida", "QA0005", "cerrado", 10},
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
	if err := s.MoveProject(ctx, *q11.ProjectID, 1, "prospecto", "oc_recibida", ""); err != nil {
		t.Fatalf("MoveProject: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE quotes SET status = 'emitida' WHERE id = 10`); err == nil {
		t.Error("a proyecto accepted two quotes that aren't revisada")
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE quotes SET status = 'pipeline' WHERE id = 2`); err == nil {
		t.Error("quotes.status still accepts a pipeline stage")
	}
}
