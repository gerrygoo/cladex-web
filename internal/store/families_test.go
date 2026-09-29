package store

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	cladex "github.com/gerrygoo/cladex-web"
)

func TestSeededSeries(t *testing.T) {
	s := newTestStore(t)
	series, err := s.ListSeries(context.Background())
	if err != nil {
		t.Fatalf("ListSeries: %v", err)
	}
	var got []string
	for _, sr := range series {
		got = append(got, sr.PickerLabel())
	}
	want := "QA — Cable CCA|QS — Cable CCS & AC|QI — Alumbrado|QL — Líneas libres"
	if strings.Join(got, "|") != want {
		t.Fatalf("series = %v; want %s", got, want)
	}
	if !series[3].FreeLinesOnly || series[0].FreeLinesOnly {
		t.Fatalf("only QL is free-lines-only: %+v", series)
	}
}

func TestCreateFamilyStartsAQuoteSeries(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, err := s.CreateFamily(ctx, Family{Name: "Fire Blanket", Series: "QF", SeriesLabel: "Cobijas", Terms: "Precios en MXN"})
	if err != nil {
		t.Fatalf("CreateFamily: %v", err)
	}
	if ok, err := s.SeriesExists(ctx, "QF"); err != nil || !ok {
		t.Fatalf("SeriesExists(QF) = %v, %v", ok, err)
	}
	if ok, _ := s.SeriesExists(ctx, "QZ"); ok {
		t.Fatal("SeriesExists(QZ) = true for a series nobody owns")
	}

	// The whole point: a quote in the new series numbers itself and carries the familia.
	userID, _ := s.CreateUser(ctx, "ana", "Ana", "pw", "admin")
	custID, _ := s.CreateCustomer(ctx, Customer{Name: "ACME"})
	q, err := s.CreateDraftQuote(ctx, custID, userID, "QF")
	if err != nil {
		t.Fatalf("CreateDraftQuote(QF): %v", err)
	}
	if q.Folio != "QF0001" || q.SeriesFamily != "Fire Blanket" || q.SeriesTerms != "Precios en MXN" {
		t.Fatalf("quote = %+v", q)
	}

	if _, err := s.CreateFamily(ctx, Family{Name: "fire blanket", Series: "QG"}); !errors.Is(err, ErrDuplicateFamilyName) {
		t.Fatalf("duplicate name err = %v", err)
	}
	if _, err := s.CreateFamily(ctx, Family{Name: "Otra", Series: "QF"}); !errors.Is(err, ErrDuplicateSeries) {
		t.Fatalf("duplicate series err = %v", err)
	}

	if err := s.UpdateFamilyTerms(ctx, id, "Cobijas ignífugas", "Uno\n\n  Dos  "); err != nil {
		t.Fatalf("UpdateFamilyTerms: %v", err)
	}
	q, _ = s.QuoteByID(ctx, q.ID)
	if got := strings.Join(SplitTerms(q.SeriesTerms), "|"); got != "Uno|Dos" {
		t.Fatalf("terms after update = %q", got)
	}
	if err := s.UpdateFamilyTerms(ctx, 9999, "", ""); err == nil {
		t.Fatal("UpdateFamilyTerms on a missing familia should fail")
	}
}

func TestFreeLinesOnlyFamilyIsNotAProductFamily(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if _, err := s.CreateFamily(ctx, Family{Name: "Servicios", Series: "QV", FreeLinesOnly: true}); err != nil {
		t.Fatal(err)
	}
	families, _ := s.ListFamilies(ctx)
	for _, f := range families {
		if f.Name == "Servicios" || f.Name == "QL" {
			t.Fatalf("ListFamilies includes free-lines-only familia %q", f.Name)
		}
	}
	all, _ := s.ListAllFamilies(ctx)
	if len(all) != 5 {
		t.Fatalf("ListAllFamilies = %d familias; want 5", len(all))
	}
}

// TestFamilySeriesMigrationKeepsQuotes replays 0013 over a database that already holds
// quotes, lines and folio counters, the way production will receive it: the table
// rebuild must lose nothing, keep the children attached, and lift the prefix CHECK.
func TestFamilySeriesMigrationKeepsQuotes(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "up.db")

	before := fstest.MapFS{}
	entries, err := fs.ReadDir(cladex.MigrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() >= "0013" {
			continue
		}
		b, _ := fs.ReadFile(cladex.MigrationsFS, "migrations/"+e.Name())
		before["migrations/"+e.Name()] = &fstest.MapFile{Data: b}
	}
	old, err := Open(ctx, dsn, before)
	if err != nil {
		t.Fatalf("open at 0012: %v", err)
	}
	for _, q := range []string{
		`INSERT INTO users (username, name, password_hash, role) VALUES ('ana', 'Ana', 'x', 'admin')`,
		`INSERT INTO customers (name) VALUES ('ACME')`,
		`INSERT INTO product_families (name) VALUES ('CCA')`,
		`INSERT INTO quotes (folio, prefix, customer_id, user_id, status, terms_snapshot)
		 VALUES ('QA0001', 'QA', 1, 1, 'emitida', 'frozen terms'), ('QI0001', 'QI', 1, 1, 'borrador', NULL)`,
		`INSERT INTO quote_lines (quote_id, line_no, description_snapshot, qty_milli, unit_price_micros, line_total)
		 VALUES (1, 1, 'Cable', 1000, 5000000, 500)`,
		`INSERT INTO quote_comments (quote_id, user_id, body) VALUES (1, 1, 'ok')`,
		`INSERT INTO folio_sequences (prefix, next_number) VALUES ('QA', 2), ('QI', 2)`,
	} {
		if _, err := old.db.ExecContext(ctx, q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	old.Close()

	s, err := Open(ctx, dsn, cladex.MigrationsFS)
	if err != nil {
		t.Fatalf("open at 0013: %v", err)
	}
	defer s.Close()

	var rows int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM pragma_foreign_key_check`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("foreign_key_check rows = %d, err = %v", rows, err)
	}
	q, err := s.QuoteByFolio(ctx, "QA0001")
	if err != nil || q == nil || q.TermsSnapshot == nil || *q.TermsSnapshot != "frozen terms" {
		t.Fatalf("QA0001 after migration = %+v, %v", q, err)
	}
	if q.SeriesFamily != "CCA" {
		t.Fatalf("QA0001 family = %q; want the pre-existing CCA familia to own QA", q.SeriesFamily)
	}
	lines, err := s.ListQuoteLines(ctx, q.ID)
	if err != nil || len(lines) != 1 {
		t.Fatalf("lines after migration = %d, %v", len(lines), err)
	}
	if next, err := s.NextFolio(ctx, "QA"); err != nil || next != "QA0002" {
		t.Fatalf("NextFolio(QA) = %q, %v; counters must survive", next, err)
	}

	// The CHECK is gone, and enforcement is back on for later connections.
	if _, err := s.db.ExecContext(ctx, `INSERT INTO quotes (folio, prefix, customer_id, user_id, status)
		VALUES ('QL0001', 'QL', 1, 1, 'borrador')`); err != nil {
		t.Fatalf("insert QL quote: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO quote_lines (quote_id, line_no, description_snapshot, qty_milli, unit_price_micros, line_total)
		VALUES (999, 1, 'x', 1, 1, 1)`); err == nil {
		t.Fatal("foreign keys not enforced after the migration")
	}
	var triggers int
	s.db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type = 'trigger' AND tbl_name = 'quotes'`).Scan(&triggers)
	if triggers != 3 {
		t.Fatalf("audit triggers on quotes = %d; want 3", triggers)
	}
}
