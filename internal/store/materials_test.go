package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	cladex "github.com/gerrygoo/cladex-web"
	"github.com/gerrygoo/cladex-web/internal/money"
)

func kgUnitID(t *testing.T, s *Store, ctx context.Context) int64 {
	t.Helper()
	units, err := s.ListUnits(ctx)
	if err != nil {
		t.Fatalf("ListUnits: %v", err)
	}
	for _, u := range units {
		if u.Code == "kg" {
			return u.ID
		}
	}
	t.Fatal("no kg unit")
	return 0
}

func TestMaterialsAndProductContent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	_, userID, flatProductID, costProductID := seedQuoteFixtures(t, s, ctx)
	kg := kgUnitID(t, s, ctx)

	// CCS 30% is seeded by the migration.
	materials, err := s.ListMaterials(ctx)
	if err != nil || len(materials) != 1 || materials[0].Name != "CCS 30%" ||
		materials[0].UnitCode != "kg" || materials[0].PriceMicros != 160_000_000 {
		t.Fatalf("seeded materials = %+v, %v", materials, err)
	}

	cobre, err := s.CreateMaterial(ctx, "Cobre", kg, 180_000_000, userID)
	if err != nil {
		t.Fatalf("CreateMaterial: %v", err)
	}
	if _, err := s.CreateMaterial(ctx, "Cobre", kg, 1, userID); !errors.Is(err, ErrDuplicateMaterialName) {
		t.Fatalf("duplicate CreateMaterial error = %v, want ErrDuplicateMaterialName", err)
	}
	if err := s.UpdateMaterial(ctx, cobre, "CCS 30%", 1, userID); !errors.Is(err, ErrDuplicateMaterialName) {
		t.Fatalf("rename onto an existing name error = %v, want ErrDuplicateMaterialName", err)
	}
	if err := s.UpdateMaterial(ctx, cobre, "Cobre", 190_000_000, userID); err != nil {
		t.Fatalf("UpdateMaterial: %v", err)
	}
	if err := s.UpdateMaterial(ctx, 9999, "X", 1, userID); !errors.Is(err, ErrMaterialNotFound) {
		t.Fatalf("UpdateMaterial(missing) error = %v, want ErrMaterialNotFound", err)
	}

	pmID, err := s.AddProductMaterial(ctx, costProductID, cobre, 18_000)
	if err != nil {
		t.Fatalf("AddProductMaterial: %v", err)
	}
	if _, err := s.AddProductMaterial(ctx, costProductID, cobre, 1); !errors.Is(err, ErrDuplicateProductMaterial) {
		t.Fatalf("duplicate AddProductMaterial error = %v, want ErrDuplicateProductMaterial", err)
	}
	content, err := s.ListProductMaterials(ctx, costProductID)
	if err != nil || len(content) != 1 || content[0].QtyPerUnitMicros != 18_000 ||
		content[0].Material.Name != "Cobre" || content[0].Material.PriceMicros != 190_000_000 {
		t.Fatalf("ListProductMaterials = %+v, %v", content, err)
	}
	if p, _ := s.ProductByID(ctx, costProductID); !p.HasMaterials {
		t.Error("product with a material has HasMaterials = false")
	}
	if p, _ := s.ProductByID(ctx, flatProductID); p.HasMaterials {
		t.Error("product without materials has HasMaterials = true")
	}

	// The delete is scoped to the product in the URL.
	if err := s.DeleteProductMaterial(ctx, flatProductID, pmID); err != nil {
		t.Fatalf("DeleteProductMaterial(wrong product): %v", err)
	}
	if content, _ := s.ListProductMaterials(ctx, costProductID); len(content) != 1 {
		t.Fatal("a delete under another product's id removed the row")
	}
	if err := s.DeleteProductMaterial(ctx, costProductID, pmID); err != nil {
		t.Fatalf("DeleteProductMaterial: %v", err)
	}
	if content, _ := s.ListProductMaterials(ctx, costProductID); len(content) != 0 {
		t.Fatalf("after delete = %+v", content)
	}
}

// TestMigration0008 runs 0008 over a catalog as the importer left it: CCS & AC
// products get their kg/m as CCS 30% content, CCA weights go from kg/km to kg/m, other
// families are untouched, and copper_price is gone.
func TestMigration0008(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "test.db")

	old, err := Open(ctx, dsn, migrationsBefore(t, "0008"))
	if err != nil {
		t.Fatalf("open at 0007: %v", err)
	}
	fam := func(name string) int64 {
		id, err := old.UpsertFamily(ctx, name, "")
		if err != nil {
			t.Fatalf("UpsertFamily: %v", err)
		}
		return id
	}
	micros := func(m money.Micros) *money.Micros { return &m }
	add := func(p Product) int64 {
		id, err := old.CreateProduct(ctx, p)
		if err != nil {
			t.Fatalf("CreateProduct(%s): %v", p.SKU, err)
		}
		return id
	}
	ccs := add(Product{FamilyID: fam("CCS & AC"), SKU: "ccs-alambre-4", Description: "ALAMBRE 4", KgPerMMicros: micros(172_300)})
	// A pre-M2.3 import left "cost before margin" on CCS & AC rows; kept, it would be
	// added on top of the materials.
	ccsStaleCost := add(Product{FamilyID: fam("CCS & AC"), SKU: "ccs-7-10", Description: "7#10", KgPerMMicros: micros(303_100), CostMicros: micros(46_980_500)})
	cca := add(Product{FamilyID: fam("CCA"), SKU: "cca-c14", Description: "THW 14", KgPerMMicros: micros(18_110_000), CostMicros: micros(5_540_000)})
	abl := add(Product{FamilyID: fam("ABASTILUM"), SKU: "abl-poste", Description: "Poste", KgPerMMicros: micros(2_000_000), CostMicros: micros(1)})
	adminID, err := old.CreateUser(ctx, "ana", "Ana", "hash", "admin")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := old.SetSetting(ctx, "copper_price", "220000000", adminID); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	old.Close()

	s, err := Open(ctx, dsn, cladex.MigrationsFS)
	if err != nil {
		t.Fatalf("open with 0008: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	content, err := s.ListProductMaterials(ctx, ccs)
	if err != nil || len(content) != 1 || content[0].Material.Name != "CCS 30%" || content[0].QtyPerUnitMicros != 172_300 {
		t.Fatalf("CCS & AC content = %+v, %v", content, err)
	}
	if p, _ := s.ProductByID(ctx, ccsStaleCost); p.CostMicros != nil || !p.HasMaterials {
		t.Errorf("CCS & AC product with a stale cost = %+v, want cost cleared and materials set", p)
	}
	if p, _ := s.ProductByID(ctx, cca); p.CostMicros == nil || *p.CostMicros != 5_540_000 {
		t.Errorf("CCA cost = %v, want untouched 5.54", p.CostMicros)
	}
	if p, _ := s.ProductByID(ctx, cca); p.KgPerMMicros == nil || *p.KgPerMMicros != 18_110 {
		t.Errorf("CCA kg/m = %v, want 0.01811", p.KgPerMMicros)
	}
	if p, _ := s.ProductByID(ctx, abl); p.KgPerMMicros == nil || *p.KgPerMMicros != 2_000_000 || p.HasMaterials {
		t.Errorf("ABASTILUM product changed: %+v", p)
	}
	if content, _ := s.ListProductMaterials(ctx, cca); len(content) != 0 {
		t.Errorf("CCA product got materials: %+v", content)
	}
	if v, err := s.SettingValue(ctx, "copper_price"); err != nil || v != "" {
		t.Errorf("copper_price = %q, %v; want deleted", v, err)
	}
}

// TestMigration0009 runs 0009 over both catalog shapes it can meet: production's, where
// ABASTILUM holds the workbook's raw costs (postes in USD), and the old importer's, where
// it holds flat margin-included prices. Postes become MXN at x18, flat prices are backed
// out to cost with the margin that produced them, and flat prices, currencies, the FX
// rate and price breaks are gone.
func TestMigration0009(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "test.db")

	old, err := Open(ctx, dsn, migrationsBefore(t, "0009"))
	if err != nil {
		t.Fatalf("open at 0008: %v", err)
	}
	fam, err := old.UpsertFamily(ctx, "ABASTILUM", "")
	if err != nil {
		t.Fatalf("UpsertFamily: %v", err)
	}
	if _, err := old.db.ExecContext(ctx, `
		INSERT INTO products (id, family_id, sku, description, cost_micros, unit_price_micros, currency) VALUES
			(1, ?1, 'poste-raw',  'POSTE METÁLICO CÓNICO CIRCULAR DE 4 MTS. PUNTA', 1680000000, NULL, 'MXN'),
			(2, ?1, 'foco-raw',   'LUMINARIO ROAD FOCUS 35W LED MCA. PHILIPS',      2398000000, NULL, 'MXN'),
			(3, ?1, 'poste-flat', 'POSTE METÁLICO CÓNICO CIRCULAR DE 5 MTS. PUNTA', NULL, 43425000000, 'MXN'),
			(4, ?1, 'led-flat',   'LUMINARIO FLOODLIGHT DE 50W MCA LEDVANCE',       NULL, 709559662, 'MXN'),
			(5, ?1, 'foco-flat',  'REFLECTOR TANGO DE 100W',                        NULL, 3001250000, 'MXN');
		INSERT INTO price_breaks (product_id, min_qty_milli, unit_price_micros) VALUES (5, 100000, 1);
		INSERT INTO settings (key, value) VALUES ('fx_rate', '18000000');`, fam); err != nil {
		t.Fatalf("seed: %v", err)
	}
	old.Close()

	s, err := Open(ctx, dsn, cladex.MigrationsFS)
	if err != nil {
		t.Fatalf("open with 0009: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	for id, want := range map[int64]money.Micros{
		1: 30_240_000_000, // raw USD poste cost, x18
		2: 2_398_000_000,  // raw MXN cost, untouched
		3: 34_740_000_000, // flat poste price, already MXN: x0.8
		4: 622_000_000,    // flat LEDVANCE price: x(1 - 12.34%) ≈ 622
		5: 2_401_000_000,  // other flat price: x0.8
	} {
		p, err := s.ProductByID(ctx, id)
		if err != nil || p == nil || p.CostMicros == nil {
			t.Fatalf("ProductByID(%d) = %+v, %v", id, p, err)
		}
		if diff := *p.CostMicros - want; diff < -1000 || diff > 1000 { // within 0.1 centavo
			t.Errorf("product %d (%s) cost = %v, want %v", id, p.SKU, *p.CostMicros, want)
		}
	}
	for _, c := range []struct{ table, column string }{
		{"products", "unit_price_micros"}, {"products", "currency"},
		// quotes.currency is not checked: 0015 brings the column back as a plain label
		// (no fx rate), for QL quotes.
		{"quotes", "fx_rate_used_micros"},
	} {
		var n int
		if err := s.db.QueryRowContext(ctx,
			`SELECT count(*) FROM pragma_table_info(?) WHERE name = ?`, c.table, c.column).Scan(&n); err != nil || n != 0 {
			t.Errorf("%s.%s still present (count %d, err %v)", c.table, c.column, n, err)
		}
	}
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM sqlite_master WHERE name = 'price_breaks'`).Scan(&n); err != nil || n != 0 {
		t.Errorf("price_breaks still present (count %d, err %v)", n, err)
	}
	if v, err := s.SettingValue(ctx, "fx_rate"); err != nil || v != "" {
		t.Errorf("fx_rate = %q, %v; want deleted", v, err)
	}
}
