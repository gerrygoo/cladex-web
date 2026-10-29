package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/gerrygoo/cladex-web/internal/money"
)

// insertProject adds a proyecto and its current (emitida) quote directly, for tests that
// only care about the proyecto's stage, probability and amount.
func insertProject(t *testing.T, s *Store, folio string, customerID, userID int64, stage string, probability int, total int64) int64 {
	t.Helper()
	ctx := context.Background()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO projects (folio, customer_id, user_id, status, probability) VALUES (?, ?, ?, ?, ?)`,
		folio, customerID, userID, stage, probability)
	if err != nil {
		t.Fatalf("insert project %s: %v", folio, err)
	}
	projectID, _ := res.LastInsertId()
	res, err = s.db.ExecContext(ctx,
		`INSERT INTO quotes (folio, prefix, quote_type_id, folio_number, customer_id, user_id, status, subtotal, total, project_id) VALUES (?1, ?2, (SELECT id FROM quote_types WHERE prefix = ?2), CAST(substr(?1, 3, 4) AS INTEGER), ?3, ?4, 'emitida', ?5, ?6, ?7)`,
		folio, folio[:2], customerID, userID, total, total*116/100, projectID)
	if err != nil {
		t.Fatalf("insert quote %s: %v", folio, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func folios(ps []DashboardProject) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Folio
	}
	return out
}

// TestSalesDashboard's amounts are subtotals: every money figure on the dashboard is
// before IVA (issue #24), so the totals seeded with IVA must not show up.
func TestSalesDashboard(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	const today = "2026-10-07"

	// Nobody has anything: the three fixed rows are still there, at zero.
	empty, err := s.SalesDashboard(ctx, today)
	if err != nil {
		t.Fatalf("SalesDashboard(empty): %v", err)
	}
	if len(empty.Sellers) != 3 || empty.Sellers[0].Name != "Emilio Flores" || len(empty.Sellers[0].Projects) != 0 {
		t.Fatalf("empty dashboard = %+v", empty.Sellers)
	}

	customerID, err := s.CreateCustomer(ctx, Customer{Name: "Grupo PEME"})
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	efs, _ := s.CreateUser(ctx, "efs", "Emilio Flores", "hash", "admin")
	rfm, _ := s.CreateUser(ctx, "rfm", "Rodolfo Flores", "hash", "vendedor")
	ggo, _ := s.CreateUser(ctx, "ggo", "Gerardo Galvan Olvera", "hash", "admin") // development account
	// ggr has no user yet, so its row stays at the fallback name.

	// efs: two active (one Alta, tracked; one Inicial, follow-up today), one with the O.C. in,
	// one lost. rfm: one active at Inminente with no follow-up date and an O.C. date gone by.
	insertProject(t, s, "QA0001", customerID, efs, "prospecto", 75, 1_000_00)
	insertProject(t, s, "QA0002", customerID, efs, "prospecto", 10, 500_00)
	insertProject(t, s, "QA0003", customerID, efs, "oc_recibida", 75, 9_000_00)
	insertProject(t, s, "QA0004", customerID, efs, "perdido", 10, 8_000_00)
	insertProject(t, s, "QA0005", customerID, rfm, "prospecto", 90, 2_000_00)
	insertProject(t, s, "QA0006", customerID, ggo, "prospecto", 90, 7_000_00)
	if _, err := s.db.ExecContext(ctx, `
		UPDATE projects SET next_followup_date = CASE folio WHEN 'QA0001' THEN '2026-10-20' WHEN 'QA0002' THEN '2026-10-07' END,
			expected_oc_date = CASE folio WHEN 'QA0002' THEN '2026-10-06' WHEN 'QA0005' THEN '2026-09-01' WHEN 'QA0001' THEN '2026-10-07' END`); err != nil {
		t.Fatalf("seed dates: %v", err)
	}
	// QA0001 sells a $1,000.00 catalog line at a 20% margin: $200.00 of profit. QA0002's only
	// line is free text, which has no cost and adds none.
	familyID, err := s.UpsertFamily(ctx, "CCA", "")
	if err != nil {
		t.Fatalf("UpsertFamily: %v", err)
	}
	cost := money.Micros(45_000_000)
	productID, err := s.CreateProduct(ctx, Product{FamilyID: familyID, SKU: "cca-c14", Description: "Cable THW 14", CostMicros: &cost})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `
		UPDATE quotes SET margin_snapshot_micros = 200000 WHERE folio IN ('QA0001', 'QA0002');
		INSERT INTO quote_lines (quote_id, line_no, product_id, description_snapshot, qty_milli, unit_price_micros, line_total)
		SELECT id, 1, ?, 'Cable', 1000, 1000000000, 100000 FROM quotes WHERE folio = 'QA0001';
		INSERT INTO quote_lines (quote_id, line_no, product_id, description_snapshot, qty_milli, unit_price_micros, line_total)
		SELECT id, 1, NULL, 'Servicio', 1000, 500000000, 50000 FROM quotes WHERE folio = 'QA0002'`, productID); err != nil {
		t.Fatalf("seed lines: %v", err)
	}

	d, err := s.SalesDashboard(ctx, today)
	if err != nil {
		t.Fatalf("SalesDashboard: %v", err)
	}
	if len(d.Sellers) != 3 {
		t.Fatalf("sellers = %d, want the 3 fixed rows (the dev account is not one)", len(d.Sellers))
	}
	by := map[string]SellerRow{}
	for _, r := range d.Sellers {
		by[r.Name] = r
	}
	e, r, g := by["Emilio Flores"], by["Rodolfo Flores"], by["Gerardo Galván Rosillo"]

	// Emilio: only the two prospectos; the O.C. in and the lost one are not active.
	// Every active proyecto is listed, biggest first.
	if len(e.Projects) != 2 || e.Projects[0].Amount < e.Projects[1].Amount || e.Pipeline != 1_500_00 {
		t.Errorf("Emilio projects = %v, pipeline = %v; want 2, biggest first, and $1,500.00", e.Projects, e.Pipeline)
	}
	if e.Profit != 200_00 {
		t.Errorf("Emilio profit = %v, want $200.00", e.Profit)
	}
	if e.ForecastCount != 1 || e.ForecastAmount != 1_000_00 {
		t.Errorf("Emilio forecast = %d, %v; want 1 and $1,000.00", e.ForecastCount, e.ForecastAmount)
	}
	if !reflect.DeepEqual(e.Untracked, []DashboardProject{{"QA0002", "Grupo PEME", 500_00}}) || e.UntrackedAmount != 500_00 {
		t.Errorf("Emilio untracked = %v, %v; want QA0002 and $500.00 (follow-up today counts, a future one doesn't)", e.Untracked, e.UntrackedAmount)
	}
	// An O.C. date of today hasn't passed; yesterday has.
	if !reflect.DeepEqual(folios(e.OverdueOC), []string{"QA0002"}) {
		t.Errorf("Emilio overdue O.C. = %v, want QA0002", e.OverdueOC)
	}
	// Rodolfo: no follow-up date at all is "sin seguimiento".
	if len(r.Projects) != 1 || r.ForecastCount != 1 || !reflect.DeepEqual(folios(r.Untracked), []string{"QA0005"}) || !reflect.DeepEqual(folios(r.OverdueOC), []string{"QA0005"}) {
		t.Errorf("Rodolfo = %+v", r)
	}
	// The third user doesn't exist: still listed, at zero, with the fallback name.
	if len(g.Projects) != 0 || g.Pipeline != 0 || len(g.Untracked) != 0 {
		t.Errorf("Gerardo = %+v", g)
	}
	// Ranked by the amount without follow-up: Rodolfo ($2,000), Emilio ($500), then Gerardo.
	if d.Sellers[0].Name != "Rodolfo Flores" || d.Sellers[1].Name != "Emilio Flores" || d.Sellers[2].Name != "Gerardo Galván Rosillo" {
		t.Errorf("order = %s, %s, %s", d.Sellers[0].Name, d.Sellers[1].Name, d.Sellers[2].Name)
	}
}
