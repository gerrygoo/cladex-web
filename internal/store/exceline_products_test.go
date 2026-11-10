package store

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	cladex "github.com/gerrygoo/cladex-web"
)

// openWithExceline replays the migrations before 0038 onto a database that has the
// familia Exceline, as production does (it was created in the app), runs seed there,
// then applies the rest.
func openWithExceline(t *testing.T, seed ...string) *Store {
	t.Helper()
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "up.db")
	old, err := Open(ctx, dsn, migrationsBefore(t, "0038"))
	if err != nil {
		t.Fatalf("open at 0037: %v", err)
	}
	for _, q := range append([]string{`INSERT INTO product_families (name) VALUES ('Exceline')`}, seed...) {
		if _, err := old.db.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	old.Close()
	s, err := Open(ctx, dsn, cladex.MigrationsFS)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func excelineCount(t *testing.T, s *Store) (n int, costMicros int64) {
	t.Helper()
	if err := s.db.QueryRowContext(context.Background(), `
		SELECT count(*), COALESCE(sum(p.cost_micros), 0) FROM products p
		JOIN product_families f ON f.id = p.family_id WHERE f.name = 'Exceline'`,
	).Scan(&n, &costMicros); err != nil {
		t.Fatal(err)
	}
	return n, costMicros
}

// Issue #60: the 60 products of Emilio's Exceline list, filed as the product form would.
func TestExcelineProductsMigration(t *testing.T) {
	ctx := context.Background()
	s := openWithExceline(t)

	// Every row of the Excel, at its cost to the centavo: the list adds up to $201,674.70.
	if n, cost := excelineCount(t, s); n != 60 || cost != 201_674_700_000 {
		t.Fatalf("Exceline products = %d costing %d micros; want 60 costing 201674700000", n, cost)
	}

	familia := familyIDByName(t, s, "Exceline")
	var pza int64
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM units WHERE code = 'pza'`).Scan(&pza); err != nil {
		t.Fatal(err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT sku FROM products WHERE family_id = ? ORDER BY id`, familia)
	if err != nil {
		t.Fatal(err)
	}
	var skus []string
	for rows.Next() {
		var sku string
		if err := rows.Scan(&sku); err != nil {
			t.Fatal(err)
		}
		skus = append(skus, sku)
	}
	rows.Close()
	if skus[0] != "EXL-GSM-N" || skus[59] != "EXL-GCG-D125-480" {
		t.Errorf("products not in the Excel's order: first %s, last %s", skus[0], skus[59])
	}
	for _, sku := range skus {
		p, err := s.ProductBySKU(ctx, sku)
		if err != nil || p == nil {
			t.Fatalf("ProductBySKU(%s) = %v, %v", sku, p, err)
		}
		if p.FamilyID != familia || p.UnitID == nil || *p.UnitID != pza || p.UnitName != "Pieza" ||
			p.KgPerMMicros != nil || p.CostMicros == nil || *p.CostMicros <= 0 ||
			p.SATProductCode != "" || p.SATUnitCode != "" || p.HasMaterials {
			t.Errorf("%s = %+v", sku, p)
		}
		if strings.ContainsAny(p.Description, "\r\n") || p.Description != strings.TrimSpace(p.Description) ||
			strings.Contains(p.Description, "  ") {
			t.Errorf("%s description isn't one clean paragraph: %q", sku, p.Description)
		}
	}

	// The two minisplit protectors, as Emilio corrected them, and a few costs to the
	// centavo (EXL-GBP-060's comes out of Excel as 778.0500000000001).
	for sku, want := range map[string]struct {
		cost   int64
		ending string
	}{
		"EXL-GSM-AS120BS":  {534_600_000, "No afecta la apariencia estética del minisplit. 120 V, 20 A RLA, 18000 BTU."},
		"EXL-GSM-AS220BS":  {534_600_000, "No afecta la apariencia estética del minisplit. 220 V, 20 A RLA, 48000 BTU."},
		"EXL-GTC-B1C120S":  {438_750_000, "120 V~. Salida con contacto aislado SPDT, capacidad 3.5 A."},
		"EXL-GTC-B1C220S":  {438_750_000, "220 V~. Salida con contacto aislado SPDT, capacidad 3.5 A."},
		"EXL-GTC-B1CMV":    {438_750_000, "Multivoltaje 120/220 V~. Salida con contacto aislado SPDT, capacidad 3.5 A."},
		"EXL-GBP-060":      {778_050_000, "120 V~, 1/2 HP, caudal máx. 42 L/min o altura máx. 36 m."},
		"EXL-GIII+480000S": {11_612_250_000, "440/480 V~, para uso con CT externos (se adquieren por separado), corrientes de 200 a 660 A."},
	} {
		p, _ := s.ProductBySKU(ctx, sku)
		if p == nil || int64(*p.CostMicros) != want.cost || !strings.HasSuffix(p.Description, want.ending) {
			t.Errorf("%s = %+v; want cost %d and a description ending %q", sku, p, want.cost, want.ending)
		}
	}
	p, _ := s.ProductBySKU(ctx, "EXL-GSM-N")
	if want := "Protector de voltaje para refrigeradores hasta 28 pies cúbicos. Ideal para refrigerador convencional y equipos Inverter, enfriadores de agua, congeladores domésticos, enfriadores de aire hasta 5000 BTU. Voltaje 120 V~, capacidad 10 amperios (1270 W)."; p.Description != want {
		t.Errorf("EXL-GSM-N description = %q, want it word for word:\n%q", p.Description, want)
	}

	// Each insert is in the products' Historial, as "Sistema".
	entries, err := s.AuditLog(ctx, AuditFilter{TableName: "products", RowKey: strconv.FormatInt(p.ID, 10)})
	if err != nil || len(entries) != 1 || entries[0].Op != "insert" || entries[0].ActorID != nil || entries[0].Source != "" {
		t.Errorf("EXL-GSM-N audit = %+v, %v; want one unattributed insert", entries, err)
	}
}

// A database without the familia Exceline (a fresh one, like every other test's) gets
// none of the products.
func TestExcelineProductsNeedTheFamilia(t *testing.T) {
	s := newTestStore(t)
	var n int
	if err := s.db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM products WHERE sku LIKE 'EXL-%'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("EXL- products without the familia = %d, %v; want none", n, err)
	}
}

// A SKU that already exists, live or deleted, in any case, is never overwritten; the
// rest still go in.
func TestExcelineProductsKeepExistingSKUs(t *testing.T) {
	ctx := context.Background()
	s := openWithExceline(t,
		`INSERT INTO products (family_id, sku, description, cost_micros)
		 VALUES ((SELECT id FROM product_families WHERE name = 'CCA'), 'EXL-GSM-N', 'Capturado a mano', 1000000)`,
		`INSERT INTO products (family_id, sku, description, cost_micros, deleted_at)
		 VALUES ((SELECT id FROM product_families WHERE name = 'Exceline'), 'exl-gsm-mp', 'Borrado', 2000000, '2026-01-01T00:00:00Z')`,
	)
	// The deleted exl-gsm-mp still counts as Exceline's: 58 new plus it.
	if n, cost := excelineCount(t, s); n != 59 || cost != 201_674_700_000-278_550_000-280_350_000+2_000_000 {
		t.Fatalf("Exceline products = %d costing %d micros", n, cost)
	}
	p, err := s.ProductBySKU(ctx, "EXL-GSM-N")
	if err != nil || p == nil || p.FamilyName != "CCA" || p.Description != "Capturado a mano" || *p.CostMicros != 1_000_000 {
		t.Errorf("existing EXL-GSM-N was changed: %+v, %v", p, err)
	}
	if p, _ := s.ProductBySKU(ctx, "EXL-GSM-MP"); p != nil {
		t.Errorf("EXL-GSM-MP was added beside the deleted exl-gsm-mp: %+v", p)
	}
}
