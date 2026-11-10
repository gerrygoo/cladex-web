package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	cladex "github.com/gerrygoo/cladex-web"
)

// openAstroSeed opens a database at 0037, runs setup against it, then migrates it to
// the latest, so 0038's alta masiva of familia Astro (issue #68) runs on that data.
func openAstroSeed(t *testing.T, setup ...string) *Store {
	t.Helper()
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "up.db")
	old, err := Open(ctx, dsn, migrationsBefore(t, "0039"))
	if err != nil {
		t.Fatalf("open at 0037: %v", err)
	}
	for _, q := range setup {
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

type astroRow struct {
	family, description, unitCode, unitName string
	cost                                    int64
	kgPerM, satProduct, satUnit             sql.NullString
}

func astroProduct(t *testing.T, s *Store, sku string) *astroRow {
	t.Helper()
	var r astroRow
	err := s.db.QueryRowContext(context.Background(), `
		SELECT f.name, p.description, u.code, u.name, p.cost_micros,
		       p.kg_per_m_micros, p.sat_product_code, p.sat_unit_code
		FROM products p
		JOIN product_families f ON f.id = p.family_id
		JOIN units u ON u.id = p.unit_id
		WHERE p.sku = ?`, sku).Scan(&r.family, &r.description, &r.unitCode, &r.unitName, &r.cost,
		&r.kgPerM, &r.satProduct, &r.satUnit)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		t.Fatalf("product %s: %v", sku, err)
	}
	return &r
}

func countRows(t *testing.T, s *Store, q string, args ...any) int {
	t.Helper()
	var n int
	if err := s.db.QueryRowContext(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

// Production's familia (named "ASTRO" there) gets the 80 boxes of the supplier's list,
// priced per box, in a new Caja unit, logged as "Sistema".
func TestAstroSeed(t *testing.T) {
	s := openAstroSeed(t, `INSERT INTO product_families (name) VALUES ('ASTRO')`)

	if n := countRows(t, s, `SELECT count(*) FROM products p JOIN product_families f ON f.id = p.family_id
		WHERE f.name = 'ASTRO' AND p.sku LIKE 'AST-%' AND p.deleted_at IS NULL AND p.active = 1`); n != 80 {
		t.Errorf("ASTRO products = %d, want 80", n)
	}
	if n := countRows(t, s, `SELECT count(*) FROM product_families WHERE name = 'Astro' COLLATE NOCASE`); n != 1 {
		t.Errorf("familias named Astro = %d, want the existing one only", n)
	}
	if n := countRows(t, s, `SELECT count(*) FROM units WHERE code = 'caja' AND name = 'Caja' AND sat_code = 'XBX'`); n != 1 {
		t.Errorf("Caja units = %d, want 1 coded caja with XBX", n)
	}
	if n := countRows(t, s, `SELECT count(*) FROM products p JOIN units u ON u.id = p.unit_id
		WHERE p.sku LIKE 'AST-%' AND u.code = 'caja'`); n != 80 {
		t.Errorf("products sold by the Caja = %d, want 80", n)
	}
	if n := countRows(t, s, `SELECT count(*) FROM products
		WHERE sku LIKE 'AST-%' AND description NOT GLOB '*. Caja con [0-9]* piezas.'`); n != 0 {
		t.Errorf("%d descriptions don't end with the box's piezas", n)
	}

	// The sum of the list's Costo column, 65,136.07.
	if n := countRows(t, s, `SELECT sum(cost_micros) FROM products WHERE sku LIKE 'AST-%'`); n != 65_136_070_000 {
		t.Errorf("sum of Astro costs = %d micros, want 65136070000", n)
	}
	for sku, want := range map[string]astroRow{
		"AST-A01B":  {description: "Apagador Sencillo astro Blanco. Caja con 100 piezas.", cost: 453_150_000},
		"AST-C09":   {description: "Protector de Contacto. Caja con 1000 piezas.", cost: 428_820_000},
		"AST-B01":   {description: "Base Astro Line. Caja con 100 piezas.", cost: 1_018_400_000},
		"AST-MC":    {description: "Aislador Manija astro. Caja con 40 piezas.", cost: 1_018_780_000},
		"AST-PDX+C": {description: "Placa Armada de Acero Inoxidable con Contacto Dúplex. Caja con 100 piezas.", cost: 2_114_700_000},
		"AST-BF18":  {description: "Base para Fotocelda. Caja con 100 piezas.", cost: 2_752_150_000},
	} {
		got := astroProduct(t, s, sku)
		if got == nil {
			t.Errorf("%s not loaded", sku)
			continue
		}
		if got.family != "ASTRO" || got.description != want.description || got.cost != want.cost ||
			got.unitName != "Caja" || got.kgPerM.Valid || got.satProduct.Valid || got.satUnit.Valid {
			t.Errorf("%s = %+v, want %q at %d micros", sku, got, want.description, want.cost)
		}
	}

	if n := countRows(t, s, `SELECT count(*) FROM audit_log
		WHERE table_name = 'products' AND op = 'insert' AND json_extract(new_values, '$.sku') LIKE 'AST-%'
		  AND actor_id IS NULL AND source IS NULL`); n != 80 {
		t.Errorf("unattributed product inserts = %d, want 80", n)
	}
}

// A unit already named Caja is reused, and a SKU already taken is left as it was.
func TestAstroSeedKeepsWhatExists(t *testing.T) {
	s := openAstroSeed(t,
		`INSERT INTO product_families (name) VALUES ('Astro')`,
		`INSERT INTO units (code, name) VALUES ('cj', 'Caja')`,
		`INSERT INTO products (family_id, sku, description, cost_micros, unit_id)
		 VALUES ((SELECT id FROM product_families WHERE name = 'CCA'), 'AST-C09', 'Otro', 1000000,
		         (SELECT id FROM units WHERE code = 'pza'))`,
	)
	if n := countRows(t, s, `SELECT count(*) FROM units WHERE name = 'Caja' COLLATE NOCASE OR code = 'caja'`); n != 1 {
		t.Errorf("Caja units = %d, want the existing one only", n)
	}
	got := astroProduct(t, s, "AST-C09")
	if got == nil || got.family != "CCA" || got.description != "Otro" || got.cost != 1_000_000 || got.unitCode != "pza" {
		t.Errorf("existing AST-C09 = %+v, want it untouched", got)
	}
	if n := countRows(t, s, `SELECT count(*) FROM products p JOIN product_families f ON f.id = p.family_id
		JOIN units u ON u.id = p.unit_id WHERE f.name = 'Astro' AND u.code = 'cj'`); n != 79 {
		t.Errorf("Astro products in the existing Caja = %d, want 79", n)
	}
}

// A database without the familia (a fresh one) gets neither the products nor the unit.
func TestAstroSeedSkipsWithoutFamilia(t *testing.T) {
	s := newTestStore(t)
	if n := countRows(t, s, `SELECT count(*) FROM products WHERE sku LIKE 'AST-%'`); n != 0 {
		t.Errorf("AST products = %d, want 0", n)
	}
	if n := countRows(t, s, `SELECT count(*) FROM units WHERE code = 'caja'`); n != 0 {
		t.Errorf("caja units = %d, want 0", n)
	}
}
