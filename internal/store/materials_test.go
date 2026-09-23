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
	abl := add(Product{FamilyID: fam("ABASTILUM"), SKU: "abl-poste", Description: "Poste", KgPerMMicros: micros(2_000_000), UnitPriceMicros: micros(1)})
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
