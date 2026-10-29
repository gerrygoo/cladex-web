package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/gerrygoo/cladex-web"
)

// TestMigration0029 replays the quote-type migration over a database shaped like
// production's on 2026-10-28: five familias with series, quotes with revisions, a
// counter that ran ahead of its folios, one that lags behind them, and a CFDI counter.
func TestMigration0029(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "0029.db")
	old, err := Open(ctx, dsn, migrationsBefore(t, "0029"))
	if err != nil {
		t.Fatalf("open at 0028: %v", err)
	}
	for _, q := range []string{
		`INSERT INTO users (username, name, password_hash, role) VALUES ('ana', 'Ana', 'x', 'admin')`,
		`INSERT INTO customers (name) VALUES ('ACME')`,
		`INSERT INTO product_families (name, series, series_label, terms) VALUES ('Alumoclad', 'QG', 'Cable Alumoclad', 'Término QG')`,
		`INSERT INTO quotes (folio, prefix, customer_id, user_id, status) VALUES
			('QA0037', 'QA', 1, 1, 'revisada'), ('QS0010', 'QS', 1, 1, 'borrador'), ('QG0007', 'QG', 1, 1, 'borrador')`,
		`INSERT INTO quotes (folio, prefix, customer_id, user_id, status, supersedes_quote_id) VALUES ('QA0037-R1', 'QA', 1, 1, 'borrador', 1)`,
		// QA ran ahead (folios were deleted); QS lags behind its own QS0010.
		`INSERT INTO folio_sequences (prefix, next_number) VALUES ('QA', 46), ('QS', 3), ('QG', 8), ('cfdi:F', 12)
			ON CONFLICT (prefix) DO UPDATE SET next_number = excluded.next_number`,
	} {
		if _, err := old.db.ExecContext(ctx, q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	old.Close()

	s, err := Open(ctx, dsn, cladex.MigrationsFS)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	defer s.Close()

	type flags struct {
		family                       string
		free, hide, last, kg, copper bool
		letterhead                   string
	}
	want := map[string]flags{
		"QA": {family: "CCA", letterhead: "conduccasa"},
		"QS": {family: "CCS & AC", copper: true, letterhead: "copperclad"},
		"QI": {family: "ABASTILUM", hide: true, last: true, letterhead: "cladex_large"},
		"QL": {family: "QL", free: true, hide: true, letterhead: "cladex_large"},
		"QG": {family: "Alumoclad", kg: true, letterhead: "alumoclad"},
	}
	for prefix, w := range want {
		var got flags
		err := s.db.QueryRowContext(ctx, `
			SELECT COALESCE(f.name, ''), t.free_lines_only, t.hide_address, t.sort_last, t.kg_priced,
				t.copper_price_margin, t.letterhead
			FROM quote_types t LEFT JOIN product_families f ON f.id = t.family_id WHERE t.prefix = ?`, prefix).
			Scan(&got.family, &got.free, &got.hide, &got.last, &got.kg, &got.copper, &got.letterhead)
		if err != nil {
			t.Fatalf("quote type %s: %v", prefix, err)
		}
		if got != w {
			t.Errorf("quote type %s = %+v, want %+v", prefix, got, w)
		}
	}
	for folio, wantNum := range map[string]int64{"QA0037": 37, "QA0037-R1": 37, "QS0010": 10, "QG0007": 7} {
		q, err := s.QuoteByFolio(ctx, folio)
		if err != nil || q == nil || q.FolioNumber != wantNum || q.Prefix != folio[:2] {
			t.Errorf("%s after migration = %+v, %v; want number %d", folio, q, err, wantNum)
		}
	}
	if q, _ := s.QuoteByFolio(ctx, "QG0007"); q == nil || !q.KgPriced || q.Letterhead != "alumoclad" || q.SeriesTerms != "Término QG" {
		t.Errorf("QG0007 lost its type's behavior or terms: %+v", q)
	}
	// Counters carry over, and never point at a folio that exists.
	for prefix, want := range map[string]string{"QA": "QA0046", "QS": "QS0011", "QG": "QG0008", "QI": "QI0001"} {
		if got := nextFolioText(t, s, prefix); got != want {
			t.Errorf("next %s folio = %s, want %s", prefix, got, want)
		}
	}
	var cfdi int
	if err := s.db.QueryRowContext(ctx, `SELECT next_number FROM cfdi_folio_sequences WHERE serie = 'F'`).Scan(&cfdi); err != nil || cfdi != 12 {
		t.Errorf("CFDI counter F = %d, %v; want 12", cfdi, err)
	}
}

// TestRenamingAPrefixKeepsEverything is why the migration exists (issue #53): once a
// type's prefix changes, its old quotes keep their folios and still belong to the type,
// numbering carries on under the new prefix, and the old prefix can't be handed out again.
func TestRenamingAPrefixKeepsEverything(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	customerID, userID, _, _ := seedQuoteFixtures(t, s, ctx)
	old, err := s.CreateDraftQuote(ctx, customerID, userID, "QA")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE quote_types SET prefix = 'QCCA' WHERE prefix = 'QA'`); err != nil {
		t.Fatal(err)
	}

	again, err := s.QuoteByID(ctx, old.ID)
	if err != nil || again.Folio != old.Folio || again.SeriesFamily != "CCA" || again.Letterhead != "conduccasa" {
		t.Fatalf("old quote after the rename = %+v, %v; want %s, still CCA", again, err, old.Folio)
	}
	next, err := s.CreateDraftQuote(ctx, customerID, userID, "QCCA")
	if err != nil {
		t.Fatal(err)
	}
	if next.Folio != FolioText("QCCA", old.FolioNumber+1) || next.QuoteTypeID != old.QuoteTypeID {
		t.Errorf("first quote after the rename = %s (type %d), want QCCA%04d of type %d", next.Folio, next.QuoteTypeID, old.FolioNumber+1, old.QuoteTypeID)
	}
	dup, err := s.DuplicateQuote(ctx, old.ID, customerID, userID)
	if err != nil {
		t.Fatal(err)
	}
	if dup.Prefix != "QCCA" {
		t.Errorf("a copy of %s = %s; copies take the type's current prefix", old.Folio, dup.Folio)
	}
	if _, err := s.CreateDraftQuote(ctx, customerID, userID, "QA"); !errors.Is(err, ErrUnknownQuoteType) {
		t.Errorf("a quote in the retired QA = %v, want ErrUnknownQuoteType", err)
	}
	if _, err := s.CreateFamily(ctx, Family{Name: "Otra", Series: "QA"}); !errors.Is(err, ErrDuplicateSeries) {
		t.Errorf("a new familia taking the retired QA = %v, want ErrDuplicateSeries", err)
	}
}
