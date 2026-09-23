package store

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"strconv"
	"testing"
	"testing/fstest"

	cladex "github.com/gerrygoo/cladex-web"
	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/pricing"
)

func TestNextFolio(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if f, err := s.NextFolio(ctx, "QA"); err != nil || f != "QA0001" {
		t.Fatalf("NextFolio(QA) #1 = %q, %v; want QA0001, nil", f, err)
	}
	if f, err := s.NextFolio(ctx, "QA"); err != nil || f != "QA0002" {
		t.Fatalf("NextFolio(QA) #2 = %q, %v; want QA0002, nil", f, err)
	}
	// A different prefix has its own independent counter.
	if f, err := s.NextFolio(ctx, "QS"); err != nil || f != "QS0001" {
		t.Fatalf("NextFolio(QS) #1 = %q, %v; want QS0001, nil", f, err)
	}
	if f, err := s.NextFolio(ctx, "QA"); err != nil || f != "QA0003" {
		t.Fatalf("NextFolio(QA) #3 = %q, %v; want QA0003, nil", f, err)
	}
}

// seedQuoteFixtures creates one customer, one user, and two products (a flat-priced one
// and a cost+margin one) for the quote tests below.
func seedQuoteFixtures(t *testing.T, s *Store, ctx context.Context) (customerID, userID, flatProductID, costProductID int64) {
	t.Helper()
	customerID, err := s.CreateCustomer(ctx, Customer{Name: "Grupo PEME"})
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	userID, err = s.CreateUser(ctx, "vendedor1", "Vendedor Uno", "hash", "vendedor")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	familyID, err := s.UpsertFamily(ctx, "ABASTILUM", "")
	if err != nil {
		t.Fatalf("UpsertFamily: %v", err)
	}
	flatPrice := money.Micros(100_000_000) // $100.00
	flatProductID, err = s.CreateProduct(ctx, Product{
		FamilyID: familyID, SKU: "abl-foco", Description: "Foco LED", UnitPriceMicros: &flatPrice,
	})
	if err != nil {
		t.Fatalf("CreateProduct(flat): %v", err)
	}
	cost := money.Micros(45_000_000) // $45.00 cost
	costProductID, err = s.CreateProduct(ctx, Product{
		FamilyID: familyID, SKU: "cca-c14", Description: "Cable THW 14", CostMicros: &cost,
	})
	if err != nil {
		t.Fatalf("CreateProduct(cost): %v", err)
	}
	return customerID, userID, flatProductID, costProductID
}

func TestCreateDraftQuote(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	customerID, userID, _, _ := seedQuoteFixtures(t, s, ctx)

	q, err := s.CreateDraftQuote(ctx, customerID, userID, "QI")
	if err != nil {
		t.Fatalf("CreateDraftQuote: %v", err)
	}
	if q.Folio != "QI0001" || q.Prefix != "QI" || q.Status != "borrador" || q.Currency != "MXN" {
		t.Fatalf("CreateDraftQuote = %+v", q)
	}
	if q.CustomerID != customerID || q.CustomerName != "Grupo PEME" || q.UserID != userID {
		t.Fatalf("CreateDraftQuote customer/user mismatch: %+v", q)
	}
	if q.Subtotal != 0 || q.IVA != 0 || q.Total != 0 {
		t.Fatalf("new draft should have zero totals, got %+v", q)
	}

	byFolio, err := s.QuoteByFolio(ctx, "QI0001")
	if err != nil || byFolio == nil || byFolio.ID != q.ID {
		t.Fatalf("QuoteByFolio = %+v, %v", byFolio, err)
	}

	quotes, err := s.ListQuotes(ctx, "", "", "")
	if err != nil || len(quotes) != 1 {
		t.Fatalf("ListQuotes = %d, %v; want 1, nil", len(quotes), err)
	}
	if quotes, err = s.ListQuotes(ctx, "peme", "", ""); err != nil || len(quotes) != 1 {
		t.Fatalf("ListQuotes(customer match) = %d, %v; want 1, nil", len(quotes), err)
	}
	if quotes, err = s.ListQuotes(ctx, "no existe", "", ""); err != nil || len(quotes) != 0 {
		t.Fatalf("ListQuotes(no match) = %d, %v; want 0, nil", len(quotes), err)
	}
}

func TestReplaceQuoteLines(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	customerID, userID, flatProductID, costProductID := seedQuoteFixtures(t, s, ctx)

	q, err := s.CreateDraftQuote(ctx, customerID, userID, "QA")
	if err != nil {
		t.Fatalf("CreateDraftQuote: %v", err)
	}

	lines := []QuoteLine{
		{ProductID: &flatProductID, DescriptionSnapshot: "Foco LED", QtyMilli: money.Milli(2_000), UnitPriceMicros: money.Micros(100_000_000), LineTotal: money.Centavos(20_000), Source: "manual"},
		{ProductID: &costProductID, DescriptionSnapshot: "Cable THW 14", QtyMilli: money.Milli(10_000), UnitPriceMicros: money.Micros(90_000_000), LineTotal: money.Centavos(90_000), Source: "manual"},
	}
	totals := pricing.ComputeTotals([]pricing.Line{
		{UnitPriceMicros: lines[0].UnitPriceMicros, QtyMilli: lines[0].QtyMilli},
		{UnitPriceMicros: lines[1].UnitPriceMicros, QtyMilli: lines[1].QtyMilli},
	})

	if err := s.ReplaceQuoteLines(ctx, q.ID, nil, lines, totals); err != nil {
		t.Fatalf("ReplaceQuoteLines: %v", err)
	}

	got, err := s.ListQuoteLines(ctx, q.ID)
	if err != nil {
		t.Fatalf("ListQuoteLines: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListQuoteLines = %d lines, want 2", len(got))
	}
	if got[0].LineNo != 1 || got[1].LineNo != 2 {
		t.Fatalf("line_no not assigned from slice order: %+v, %+v", got[0], got[1])
	}
	if *got[0].ProductID != flatProductID || got[0].DescriptionSnapshot != "Foco LED" {
		t.Fatalf("line 0 mismatch: %+v", got[0])
	}

	reloaded, err := s.QuoteByID(ctx, q.ID)
	if err != nil {
		t.Fatalf("QuoteByID: %v", err)
	}
	if reloaded.Subtotal != totals.Subtotal || reloaded.IVA != totals.IVA || reloaded.Total != totals.Total {
		t.Fatalf("quote totals not persisted: got %+v, want %+v", reloaded, totals)
	}

	// Replacing again with fewer lines should leave exactly the new set — no stale rows.
	single := []QuoteLine{lines[0]}
	singleTotals := pricing.ComputeTotals([]pricing.Line{{UnitPriceMicros: lines[0].UnitPriceMicros, QtyMilli: lines[0].QtyMilli}})
	if err := s.ReplaceQuoteLines(ctx, q.ID, nil, single, singleTotals); err != nil {
		t.Fatalf("ReplaceQuoteLines (second call): %v", err)
	}
	got, err = s.ListQuoteLines(ctx, q.ID)
	if err != nil {
		t.Fatalf("ListQuoteLines after second replace: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListQuoteLines after second replace = %d lines, want 1", len(got))
	}
}

func TestIssueQuote(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	customerID, userID, flatProductID, _ := seedQuoteFixtures(t, s, ctx)

	q, err := s.CreateDraftQuote(ctx, customerID, userID, "QA")
	if err != nil {
		t.Fatalf("CreateDraftQuote: %v", err)
	}
	lines := []QuoteLine{
		{ProductID: &flatProductID, DescriptionSnapshot: "Foco LED", QtyMilli: money.Milli(1_000), UnitPriceMicros: money.Micros(100_000_000), LineTotal: money.Centavos(10_000), Source: "manual"},
	}
	totals := pricing.ComputeTotals([]pricing.Line{{UnitPriceMicros: lines[0].UnitPriceMicros, QtyMilli: lines[0].QtyMilli}})
	if err := s.ReplaceQuoteLines(ctx, q.ID, nil, lines, totals); err != nil {
		t.Fatalf("ReplaceQuoteLines: %v", err)
	}

	validUntil := "2026-09-01"
	fxRate := money.Micros(18_000_000)
	issuedAt := "2026-08-02T15:04:05.000Z"
	if err := s.IssueQuote(ctx, q.ID, Issue{
		IssuedAt: issuedAt, FxRateUsed: fxRate, TermsSnapshot: "term one\nterm two", ValidUntil: &validUntil,
		CustomerNameSnapshot: "Cliente al emitir", VendedorSnapshot: "Vendedor al emitir", PDFSHA256: "deadbeef",
	}); err != nil {
		t.Fatalf("IssueQuote: %v", err)
	}

	issued, err := s.QuoteByID(ctx, q.ID)
	if err != nil {
		t.Fatalf("QuoteByID: %v", err)
	}
	if issued.Status != "emitida" {
		t.Fatalf("Status = %q, want emitida", issued.Status)
	}
	if issued.IssuedAt == nil || *issued.IssuedAt != issuedAt {
		t.Fatalf("IssuedAt = %v, want %s", issued.IssuedAt, issuedAt)
	}
	if issued.FxRateUsedMicros == nil || *issued.FxRateUsedMicros != fxRate {
		t.Fatalf("FxRateUsedMicros = %v, want %v", issued.FxRateUsedMicros, fxRate)
	}
	if issued.TermsSnapshot == nil || *issued.TermsSnapshot != "term one\nterm two" {
		t.Fatalf("TermsSnapshot = %v", issued.TermsSnapshot)
	}
	if issued.ValidUntil == nil || *issued.ValidUntil != validUntil {
		t.Fatalf("ValidUntil = %v, want %v", issued.ValidUntil, validUntil)
	}
	if issued.CustomerNameSnapshot == nil || *issued.CustomerNameSnapshot != "Cliente al emitir" ||
		issued.VendedorSnapshot == nil || *issued.VendedorSnapshot != "Vendedor al emitir" {
		t.Fatalf("name snapshots = %v, %v", issued.CustomerNameSnapshot, issued.VendedorSnapshot)
	}
	if issued.PDFSHA256 == nil || *issued.PDFSHA256 != "deadbeef" {
		t.Fatalf("PDFSHA256 = %v", issued.PDFSHA256)
	}

	// Issuing an already-issued quote fails — not a thing.
	if err := s.IssueQuote(ctx, q.ID, Issue{IssuedAt: issuedAt, TermsSnapshot: "x"}); !errors.Is(err, ErrQuoteNotDraft) {
		t.Fatalf("IssueQuote(already issued) = %v, want ErrQuoteNotDraft", err)
	}
}

// testIssue is a minimal valid Issue for tests that only need a quote to be issued.
func testIssue(sha string) Issue {
	return Issue{
		IssuedAt: "2026-08-02T15:04:05.000Z", FxRateUsed: money.Micros(18_000_000), TermsSnapshot: "term",
		CustomerNameSnapshot: "Cliente", VendedorSnapshot: "Vendedor", PDFSHA256: sha,
	}
}

func TestCreateRevision(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	customerID, userID, flatProductID, _ := seedQuoteFixtures(t, s, ctx)

	q, err := s.CreateDraftQuote(ctx, customerID, userID, "QA")
	if err != nil {
		t.Fatalf("CreateDraftQuote: %v", err)
	}
	// A revision of a draft (never issued) should fail.
	if _, err := s.CreateRevision(ctx, q.ID, userID); !errors.Is(err, ErrQuoteNotIssued) {
		t.Fatalf("CreateRevision(draft) = %v, want ErrQuoteNotIssued", err)
	}

	lines := []QuoteLine{
		{ProductID: &flatProductID, DescriptionSnapshot: "Foco LED", QtyMilli: money.Milli(1_000), UnitPriceMicros: money.Micros(100_000_000), LineTotal: money.Centavos(10_000), Source: "manual"},
	}
	totals := pricing.ComputeTotals([]pricing.Line{{UnitPriceMicros: lines[0].UnitPriceMicros, QtyMilli: lines[0].QtyMilli}})
	if err := s.ReplaceQuoteLines(ctx, q.ID, nil, lines, totals); err != nil {
		t.Fatalf("ReplaceQuoteLines: %v", err)
	}
	if err := s.IssueQuote(ctx, q.ID, testIssue("sha1")); err != nil {
		t.Fatalf("IssueQuote: %v", err)
	}

	rev, err := s.CreateRevision(ctx, q.ID, userID)
	if err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	if rev.Folio != "QA0001-R1" || rev.Status != "borrador" || rev.SupersedesQuoteID == nil || *rev.SupersedesQuoteID != q.ID {
		t.Fatalf("revision = %+v", rev)
	}
	if rev.CustomerID != customerID || rev.Prefix != "QA" || rev.Currency != "MXN" {
		t.Fatalf("revision didn't copy customer/prefix/currency: %+v", rev)
	}
	if rev.Subtotal != totals.Subtotal || rev.Total != totals.Total {
		t.Fatalf("revision totals = %+v, want copied from original %+v", rev, totals)
	}
	revLines, err := s.ListQuoteLines(ctx, rev.ID)
	if err != nil || len(revLines) != 1 || *revLines[0].ProductID != flatProductID {
		t.Fatalf("revision lines = %+v, %v", revLines, err)
	}

	// The original is untouched except for its status flip, and knows who supersedes it.
	original, err := s.QuoteByID(ctx, q.ID)
	if err != nil {
		t.Fatalf("QuoteByID(original): %v", err)
	}
	if original.Status != "revisada" {
		t.Fatalf("original.Status = %q, want revisada", original.Status)
	}
	if original.SupersededByFolio != "QA0001-R1" {
		t.Fatalf("original.SupersededByFolio = %q, want QA0001-R1", original.SupersededByFolio)
	}
	if original.PDFSHA256 == nil || *original.PDFSHA256 != "sha1" {
		t.Fatalf("original.PDFSHA256 changed: %+v", original.PDFSHA256)
	}

	// Revising an already-revised (revisada) quote directly should fail — only the
	// current latest revision (still 'emitida') can be revised further.
	if _, err := s.CreateRevision(ctx, q.ID, userID); !errors.Is(err, ErrQuoteNotIssued) {
		t.Fatalf("CreateRevision(revisada) = %v, want ErrQuoteNotIssued", err)
	}

	// Issuing and revising the revision itself produces -R2, chained off the same base.
	if err := s.ReplaceQuoteLines(ctx, rev.ID, nil, lines, totals); err != nil {
		t.Fatalf("ReplaceQuoteLines(rev): %v", err)
	}
	if err := s.IssueQuote(ctx, rev.ID, testIssue("sha2")); err != nil {
		t.Fatalf("IssueQuote(rev): %v", err)
	}
	rev2, err := s.CreateRevision(ctx, rev.ID, userID)
	if err != nil {
		t.Fatalf("CreateRevision(rev): %v", err)
	}
	if rev2.Folio != "QA0001-R2" || rev2.SupersedesQuoteID == nil || *rev2.SupersedesQuoteID != rev.ID {
		t.Fatalf("second revision = %+v", rev2)
	}
}

// TestQuotesAreAudited is a regression test for a gap this store package would
// otherwise have left silently: quotes is an audited table (migrations/0004_audit_log.sql),
// but ReplaceQuoteLines and CreateRevision each compose a quotes write into a larger,
// manually-managed transaction alongside unaudited quote_lines writes, so they can't
// simply call s.exec like a single-statement write would — they have to stamp the
// actor themselves via stampActor. Confirms every quotes write this package makes
// (create, save totals, issue, revise x2 including the original's status flip) is
// correctly attributed rather than silently unattributed or inheriting a stale actor.
func TestQuotesAreAudited(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	customerID, _, flatProductID, _ := seedQuoteFixtures(t, s, ctx)
	userID, err := s.CreateUser(ctx, "vendedora", "Vendedora Uno", "hash", "vendedor")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	actorCtx := WithActor(ctx, Actor{UserID: userID, Source: SourceWeb})

	q, err := s.CreateDraftQuote(actorCtx, customerID, userID, "QA")
	if err != nil {
		t.Fatalf("CreateDraftQuote: %v", err)
	}

	lines := []QuoteLine{
		{ProductID: &flatProductID, DescriptionSnapshot: "Foco LED", QtyMilli: money.Milli(1_000), UnitPriceMicros: money.Micros(100_000_000), LineTotal: money.Centavos(10_000), Source: "manual"},
	}
	totals := pricing.ComputeTotals([]pricing.Line{{UnitPriceMicros: lines[0].UnitPriceMicros, QtyMilli: lines[0].QtyMilli}})
	if err := s.ReplaceQuoteLines(actorCtx, q.ID, nil, lines, totals); err != nil {
		t.Fatalf("ReplaceQuoteLines: %v", err)
	}
	if err := s.IssueQuote(actorCtx, q.ID, testIssue("sha1")); err != nil {
		t.Fatalf("IssueQuote: %v", err)
	}
	rev, err := s.CreateRevision(actorCtx, q.ID, userID)
	if err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}

	entries, err := s.AuditLog(ctx, AuditFilter{TableName: "quotes", RowKey: strconv.FormatInt(q.ID, 10)})
	if err != nil {
		t.Fatalf("AuditLog(original): %v", err)
	}
	// insert (CreateDraftQuote), update (ReplaceQuoteLines totals), update (IssueQuote
	// freeze), update (CreateRevision's status flip to revisada).
	if len(entries) != 4 {
		t.Fatalf("got %d audit entries for the original quote, want 4: %+v", len(entries), entries)
	}
	for _, e := range entries {
		if e.ActorID == nil || *e.ActorID != userID || e.Source != SourceWeb {
			t.Fatalf("entry not attributed to the actor: %+v", e)
		}
	}
	if entries[0].Op != "update" || entries[0].NewValues["status"] != "revisada" {
		t.Fatalf("newest entry should be the revisada status flip: %+v", entries[0])
	}

	revEntries, err := s.AuditLog(ctx, AuditFilter{TableName: "quotes", RowKey: strconv.FormatInt(rev.ID, 10)})
	if err != nil {
		t.Fatalf("AuditLog(revision): %v", err)
	}
	// insert (the new draft) + update (its totals, copied from the original).
	if len(revEntries) != 2 {
		t.Fatalf("got %d audit entries for the revision, want 2: %+v", len(revEntries), revEntries)
	}
	for _, e := range revEntries {
		if e.ActorID == nil || *e.ActorID != userID || e.Source != SourceWeb {
			t.Fatalf("revision entry not attributed to the actor: %+v", e)
		}
	}
}

func TestListPriceBreaks(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	_, _, flatProductID, _ := seedQuoteFixtures(t, s, ctx)

	breaks, err := s.ListPriceBreaks(ctx, flatProductID)
	if err != nil {
		t.Fatalf("ListPriceBreaks(no breaks): %v", err)
	}
	if len(breaks) != 0 {
		t.Fatalf("ListPriceBreaks(no breaks) = %d, want 0", len(breaks))
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO price_breaks (product_id, min_qty_milli, unit_price_micros) VALUES (?, ?, ?)`,
		flatProductID, 100_000, 90_000_000,
	); err != nil {
		t.Fatalf("seed price_breaks: %v", err)
	}

	breaks, err = s.ListPriceBreaks(ctx, flatProductID)
	if err != nil || len(breaks) != 1 {
		t.Fatalf("ListPriceBreaks = %d, %v; want 1, nil", len(breaks), err)
	}
	if breaks[0].MinQty != money.Milli(100_000) || breaks[0].UnitPriceMicros != money.Micros(90_000_000) {
		t.Fatalf("ListPriceBreaks[0] = %+v", breaks[0])
	}
}

// TestMigration0006BackfillsNameSnapshots runs 0006 against a database that already
// has quotes (all migrations up to 0005, then the rest): issued quotes get their name
// snapshots backfilled and their old, unreproducible hashes cleared; drafts are left
// alone; pdf_path is gone.
func TestMigration0006BackfillsNameSnapshots(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "test.db")

	old, err := Open(ctx, dsn, migrationsBefore(t, "0006"))
	if err != nil {
		t.Fatalf("open at 0005: %v", err)
	}
	customerID, userID, _, _ := seedQuoteFixtures(t, old, ctx)
	if _, err := old.db.ExecContext(ctx, `
		INSERT INTO quotes (folio, prefix, customer_id, user_id, status, issued_at, pdf_path, pdf_sha256)
		VALUES ('QA0001', 'QA', ?, ?, 'emitida', '2026-08-02T15:04:05.123Z', 'quotes/QA0001.pdf', 'old'),
		       ('QA0002', 'QA', ?, ?, 'borrador', NULL, NULL, NULL)`,
		customerID, userID, customerID, userID); err != nil {
		t.Fatalf("seed quotes: %v", err)
	}
	old.Close()

	s, err := Open(ctx, dsn, cladex.MigrationsFS)
	if err != nil {
		t.Fatalf("open with 0006: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	issued, err := s.QuoteByFolio(ctx, "QA0001")
	if err != nil || issued == nil {
		t.Fatalf("QuoteByFolio(QA0001): %+v, %v", issued, err)
	}
	if issued.CustomerNameSnapshot == nil || *issued.CustomerNameSnapshot != issued.CustomerName ||
		issued.VendedorSnapshot == nil || *issued.VendedorSnapshot != issued.UserName {
		t.Fatalf("issued snapshots = %v, %v; want %q, %q",
			issued.CustomerNameSnapshot, issued.VendedorSnapshot, issued.CustomerName, issued.UserName)
	}
	if issued.PDFSHA256 != nil {
		t.Fatalf("issued PDFSHA256 = %q, want cleared", *issued.PDFSHA256)
	}
	draft, err := s.QuoteByFolio(ctx, "QA0002")
	if err != nil || draft == nil {
		t.Fatalf("QuoteByFolio(QA0002): %+v, %v", draft, err)
	}
	if draft.CustomerNameSnapshot != nil || draft.VendedorSnapshot != nil {
		t.Fatalf("draft got snapshots: %+v", draft)
	}
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM pragma_table_info('quotes') WHERE name = 'pdf_path'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("pdf_path still present (count %d, err %v)", n, err)
	}
}

// migrationsBefore returns the embedded migrations whose filenames sort before first,
// for opening a database as it was before a migration under test.
func migrationsBefore(t *testing.T, first string) fstest.MapFS {
	t.Helper()
	before := fstest.MapFS{}
	entries, err := fs.ReadDir(cladex.MigrationsFS, "migrations")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if e.Name() >= first {
			continue
		}
		data, err := fs.ReadFile(cladex.MigrationsFS, "migrations/"+e.Name())
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		before["migrations/"+e.Name()] = &fstest.MapFile{Data: data}
	}
	return before
}
