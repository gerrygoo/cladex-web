package store

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
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
	if got, _ := s.QuoteByID(ctx, q.ID); got.Status != "emitida" || got.Stage != "cerrado" || got.Revisable() {
		t.Errorf("quote status = %q, stage = %q, revisable = %v", got.Status, got.Stage, got.Revisable())
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
	insertProject(t, s, "QS0001", other, user, "oc_recibida", 90, 900_00)
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
		{"prospecto", ""}, {"prospecto", "   "}, {"oc_recibida", "No hubo presupuesto"}, {"cerrado", "x"},
	} {
		if err := s.LoseProject(ctx, project, user, c.from, c.reason); !errors.Is(err, ErrBadTransition) {
			t.Errorf("LoseProject(%q, %q) = %v, want ErrBadTransition", c.from, c.reason, err)
		}
	}
	if err := s.ReopenProject(ctx, project, user, ""); !errors.Is(err, ErrBadTransition) {
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
	// Lost is the end of the line: no forecast, no moves, no probability, no revision.
	if p.ForecastRelevant() {
		t.Error("a lost proyecto still counts for the forecast")
	}
	if err := s.MoveProject(ctx, project, user, "perdido", "oc_recibida", ""); !errors.Is(err, ErrBadTransition) {
		t.Errorf("MoveProject(perdido) = %v, want ErrBadTransition", err)
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
	// It is off the board, counted apart.
	ov, err := s.ProjectOverview(ctx, "2026-10-07")
	if err != nil {
		t.Fatalf("ProjectOverview: %v", err)
	}
	if ov.Stages[0].Count != 0 || ov.Lost.Count != 1 || len(ov.Vendedores) != 0 {
		t.Errorf("overview with one lost proyecto: prospectos %d, lost %d, vendedores %d", ov.Stages[0].Count, ov.Lost.Count, len(ov.Vendedores))
	}

	// Reopening returns it to where it was, with the probability it had.
	if err := s.ReopenProject(ctx, project, user, "Volvieron a llamar"); err != nil {
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

	// From O.C. recibida it goes back to O.C. recibida; later stages can't be lost.
	if err := s.MoveProject(ctx, project, user, "prospecto", "oc_recibida", ""); err != nil {
		t.Fatalf("MoveProject: %v", err)
	}
	if err := s.LoseProject(ctx, project, user, "oc_recibida", "Cancelaron la OC"); err != nil {
		t.Fatalf("LoseProject(oc_recibida): %v", err)
	}
	if err := s.ReopenProject(ctx, project, user, ""); err != nil {
		t.Fatalf("ReopenProject: %v", err)
	}
	if p = load(); p.Status != "oc_recibida" {
		t.Fatalf("reopened to %q, want oc_recibida", p.Status)
	}
	if err := s.MoveProject(ctx, project, user, "oc_recibida", "en_entrega", ""); err != nil {
		t.Fatalf("MoveProject: %v", err)
	}
	if err := s.LoseProject(ctx, project, user, "en_entrega", "tarde"); !errors.Is(err, ErrBadTransition) {
		t.Errorf("LoseProject(en_entrega) = %v, want ErrBadTransition", err)
	}

	// Losing and reopening are audited with the reason.
	var n int
	if err := s.db.QueryRowContext(ctx, `
		SELECT count(*) FROM audit_log WHERE table_name = 'projects' AND op = 'update'
		AND json_extract(new_values, '$.lost_reason') = 'Cancelaron la OC'`).Scan(&n); err != nil || n != 1 {
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
	if err := s.MoveProject(ctx, project, user, "prospecto", "oc_recibida", ""); err != nil {
		t.Fatalf("MoveProject: %v", err)
	}
	if err := s.FollowUpProject(ctx, project, user, FollowUp{ExpectedOC: str("2026-11-01")}); !errors.Is(err, ErrBadTransition) {
		t.Errorf("FollowUpProject(oc_recibida) = %v, want ErrBadTransition", err)
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
	insertProject(t, s, "QA0005", customerID, user, "oc_recibida", 90, 5_000_00)
	if _, err := s.db.ExecContext(ctx, `
		UPDATE projects SET expected_oc_date = CASE folio WHEN 'QA0001' THEN '2026-11-01' WHEN 'QA0003' THEN '2026-10-15' END,
			next_followup_date = CASE folio WHEN 'QA0003' THEN '2026-10-07' WHEN 'QA0004' THEN '2026-10-06' WHEN 'QA0002' THEN '2026-10-08' END;
		UPDATE quotes SET valid_until = '2026-10-30', issued_at = '2026-09-30T15:00:00.000Z' WHERE folio = 'QA0003'`); err != nil {
		t.Fatalf("seed dates: %v", err)
	}
	folios := func(all bool) string {
		t.Helper()
		ps, err := s.ListProspects(ctx, all)
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
	ps, _ := s.ListProspects(ctx, false)
	if p := ps[0]; p.ContactName != "Ing. Pérez" || p.ContactPhone != "55 1234 5678" || p.ContactEmail != "perez@peme.mx" ||
		p.QuoteValidUntil != "2026-10-30" || p.QuoteIssuedAt == "" || p.LastBody != "" {
		t.Errorf("first prospect = %+v", p)
	}

	// 1,000 × 75% + 2,000 × 90% + 3,000 × 75% + 4,000.01 × 25% = 5,800.0025 → $5,800.00.
	ov, err := s.ProjectOverview(ctx, "2026-10-07")
	if err != nil {
		t.Fatalf("ProjectOverview: %v", err)
	}
	if pros := ov.Stages[0]; pros.Weighted != 5_800_00 || pros.FollowUpsDue != 2 {
		t.Errorf("prospectos weighted = %v, follow-ups due = %d; want $5,800.00 and 2", pros.Weighted, pros.FollowUpsDue)
	}
	if oc := ov.Stages[1]; oc.Weighted != 0 || oc.FollowUpsDue != 0 {
		t.Errorf("oc_recibida weighted = %v, due = %d; want none", oc.Weighted, oc.FollowUpsDue)
	}
}
