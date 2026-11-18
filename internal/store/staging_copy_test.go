package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	cladex "github.com/gerrygoo/cladex-web"
)

// Every table the migrations make is classified for building staging (#81), and
// nothing classified is missing: a new table must say whether its data may reach staging.
func TestStagingTablesClassified(t *testing.T) {
	s := newTestStore(t)
	rows, err := s.db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite\_%' ESCAPE '\'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	var missing []string
	for rows.Next() {
		var name string
		rows.Scan(&name)
		seen[name] = true
		if _, ok := StagingTables[name]; !ok {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("tables not in store.StagingTables (decide copy, synthetic, empty or skip): %v", missing)
	}
	for name := range StagingTables {
		if !seen[name] {
			t.Errorf("StagingTables lists %s, which no migration creates", name)
		}
	}
	for _, c := range stagingCopies {
		if StagingTables[c.table] != StagingCopy {
			t.Errorf("stagingCopies copies %s, which isn't StagingCopy", c.table)
		}
	}
	copied := map[string]bool{}
	for _, c := range stagingCopies {
		copied[c.table] = true
	}
	for name, rule := range StagingTables {
		if rule == StagingCopy && !copied[name] {
			t.Errorf("%s is StagingCopy but stagingCopies doesn't copy it", name)
		}
	}
}

// CopyStagingConfig brings production's configuration and nothing else: only the
// STAGING_USERS, without passwords; costs scaled; no suppliers, customers or emisor;
// documents as placeholders; references to users who aren't copied dropped.
func TestCopyStagingConfig(t *testing.T) {
	ctx := context.Background()
	srcPath := filepath.Join(t.TempDir(), "prod.db")
	src, err := Open(ctx, srcPath, cladex.MigrationsFS)
	if err != nil {
		t.Fatal(err)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := src.db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO users (id, username, name, password_hash, role, email) VALUES
		(1, 'ggo', 'Gerardo', '$2a$real', 'admin', 'g@example.com'), (2, 'efs', 'Emilio', '$2a$real', 'admin', NULL),
		(3, 'rfm', 'Rodolfo', '$2a$real', 'vendedor', 'r@example.com')`)
	exec(`INSERT INTO user_avisos (user_id, kind) VALUES (1, 'A'), (3, 'A')`)
	exec(`INSERT INTO suppliers (id, name, rfc, legal_name, street, ext_number, neighborhood, municipality, state, postal_code)
		VALUES (900, 'Proveedor Real', 'PRO000000AAA', 'Proveedor Real SA', 'Calle', '1', 'Centro', 'Monterrey', 'NL', '64000')`)
	exec(`INSERT INTO product_families (id, name, default_supplier_id) VALUES (100, 'PRUEBA-FAM', 900)`)
	exec(`INSERT INTO products (id, family_id, sku, description, cost_micros) VALUES (200, 100, 'PRUEBA-SKU-1', 'Cable', 10000000)`)
	exec(`INSERT INTO materials (id, name, unit_id, price_micros, updated_by) VALUES (300, 'Cobre', (SELECT min(id) FROM units), 5000000, 3)`)
	exec(`UPDATE margin_options SET updated_by = 1`)
	exec(`INSERT OR REPLACE INTO settings (key, value, updated_by) VALUES ('emisor_rfc', 'CLA000000AAA', 1), ('factura_serie', 'F', 1),
		('oc_razon_social', 'Cladex', 3), ('seed_staging_at', 'x', NULL)`)
	exec(`INSERT INTO customers (id, name) VALUES (1, 'Cliente Real')`)
	exec(`INSERT INTO document_sections (id, name, deleted_by) VALUES (10, 'Rodolfo - Documentos', 3)`)
	exec(`INSERT INTO official_documents (id, section_id, title, filename, content_type, size, data, uploaded_by) VALUES
		(20, 10, 'Ficha técnica', 'ficha.v2.docx', 'application/msword', 5, X'0102030405', 3)`)
	var wantMargins, wantUnits, wantTypes int
	src.db.QueryRow(`SELECT count(*) FROM margin_options`).Scan(&wantMargins)
	src.db.QueryRow(`SELECT count(*) FROM units`).Scan(&wantUnits)
	src.db.QueryRow(`SELECT count(*) FROM quote_types`).Scan(&wantTypes)
	src.Close()

	dst := newTestStore(t)
	if err := dst.copyStagingConfig(ctx, srcPath, []string{" ggo", "nadie", ""}, 0.8); err != nil {
		t.Fatalf("copy: %v", err)
	}
	str := func(q string) string {
		t.Helper()
		var v any
		if err := dst.db.QueryRow(q).Scan(&v); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		switch v := v.(type) {
		case nil:
			return "NULL"
		case []byte:
			return string(v)
		default:
			return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(fmt.Sprint(v), "\n", " "), "  ", " "))
		}
	}
	for q, want := range map[string]string{
		`SELECT group_concat(username) FROM users`:                                                              "ggo",
		`SELECT password_hash FROM users`:                                                                       "!",
		`SELECT email FROM users`:                                                                               "g@example.com",
		`SELECT group_concat(user_id) FROM user_avisos`:                                                         "1",
		`SELECT cost_micros FROM products WHERE id = 200`:                                                       "8000000",
		`SELECT price_micros FROM materials WHERE id = 300`:                                                     "4000000",
		`SELECT updated_by FROM materials WHERE id = 300`:                                                       "NULL",
		`SELECT default_supplier_id FROM product_families WHERE id = 100`:                                       "NULL",
		`SELECT count(*) FROM settings WHERE key LIKE 'emisor%' OR key IN ('factura_serie', 'seed_staging_at')`: "0",
		`SELECT value FROM settings WHERE key = 'oc_razon_social'`:                                              "Cladex",
		`SELECT updated_by FROM settings`:                                                                       "NULL",
		`SELECT count(*) FROM suppliers`:                                                                        "0",
		`SELECT count(*) FROM customers`:                                                                        "0",
		`SELECT deleted_by FROM document_sections`:                                                              "NULL",
		`SELECT filename FROM official_documents`:                                                               "ficha.v2.pdf",
		`SELECT content_type FROM official_documents`:                                                           "application/pdf",
		`SELECT uploaded_by FROM official_documents`:                                                            "1",
		`SELECT count(*) = 1 FROM official_documents WHERE size = length(data)`:                                 "1",
		`SELECT count(*) FROM margin_options WHERE updated_by = 1`:                                              fmt.Sprint(int64(wantMargins)),
		`SELECT count(*) FROM units`:                                                                            fmt.Sprint(int64(wantUnits)),
		`SELECT count(*) FROM quote_types`:                                                                      fmt.Sprint(int64(wantTypes)),
	} {
		if got := str(q); got != want {
			t.Errorf("%s = %q, want %q", q, got, want)
		}
	}
	var data []byte
	dst.db.QueryRow(`SELECT data FROM official_documents`).Scan(&data)
	if !bytes.HasPrefix(data, []byte("%PDF-")) || !bytes.Contains(data, []byte("Documento de prueba")) {
		t.Errorf("document data isn't the placeholder PDF")
	}
	if err := dst.copyStagingConfig(ctx, srcPath, []string{"ggo"}, 0.8); !errors.Is(err, ErrStagingNotEmpty) {
		t.Errorf("a second copy = %v, want ErrStagingNotEmpty", err)
	}
	other := newTestStore(t)
	if err := other.copyStagingConfig(ctx, srcPath, []string{"nadie"}, 0.8); err == nil || !strings.Contains(err.Error(), "none of STAGING_USERS") {
		t.Errorf("copy without any real STAGING_USERS = %v", err)
	}
}

func TestStagingCostFactorAvoidsOne(t *testing.T) {
	for range 200 {
		f, err := stagingCostFactor()
		if err != nil || f < 0.7 || f > 1.3 || (f >= 0.9 && f <= 1.1) {
			t.Fatalf("factor = %v, %v", f, err)
		}
	}
}
