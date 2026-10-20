package store

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	cladex "github.com/gerrygoo/cladex-web"
)

func TestNormalizeName(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"SONEPAR S.A. de C.V.", "sonepar"},
		{"Sonepar", "sonepar"},
		{"  Zasa  ", "zasa"},
		{"Construcciones Galván, S. de R.L. de C.V.", "construcciones galvan"},
		{"Grupo Peñoles S.A.P.I. de C.V.", "grupo penoles"},
		{"Despacho Ruiz S.C.", "despacho ruiz"},
		{"Industrias SA", "industrias"},
		{"Casa", "casa"}, // a name that merely ends in "sa" keeps it
	} {
		if got := normalizeName(c.in); got != c.want {
			t.Errorf("normalizeName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestSimilarCustomers pins issue #29's duplicate check: the same RFC (spaces, hyphens and
// case aside), or a normalized name equal to, containing or contained in another.
func TestSimilarCustomers(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for _, c := range []Customer{
		{Name: "SONEPAR", ContactName: "Alfonso Felix"},
		{Name: "Grupo PEME", RFC: "GPE-010101-AB1"},
		{Name: "ZASA"},
		{Name: "Pruebas"},
	} {
		if _, err := s.CreateCustomer(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	names := func(cs []Customer) []string {
		var out []string
		for _, c := range cs {
			out = append(out, c.Name)
		}
		return out
	}
	for _, c := range []struct {
		name, rfc string
		want      []string
	}{
		{"Sonepar S.A. de C.V.", "", []string{"SONEPAR"}},
		{"zasa", "", []string{"ZASA"}},
		{"Otra empresa", "gpe 010101 ab1", []string{"Grupo PEME"}},
		{"Pruebas Industriales del Norte", "", []string{"Pruebas"}},
		{"Nadie Parecido", "XAXX010101000", nil},
		{"Su", "", nil}, // too short to count as contained in anything
	} {
		got, err := s.SimilarCustomers(ctx, c.name, c.rfc)
		if err != nil {
			t.Fatal(err)
		}
		if g := names(got); len(g) != len(c.want) || (len(g) > 0 && g[0] != c.want[0]) {
			t.Errorf("SimilarCustomers(%q, %q) = %v, want %v", c.name, c.rfc, g, c.want)
		}
	}
}

// TestMigrationCustomerFiscalFields pins 0022 (issue #17): a free-text régimen that starts
// with a SAT code keeps just the code; anything else is cleared and moved to the notes;
// the new fields start empty, and the audit trail now covers the fiscal fields.
func TestMigrationCustomerFiscalFields(t *testing.T) {
	ctx := context.Background()
	before := fstest.MapFS{}
	entries, _ := fs.ReadDir(cladex.MigrationsFS, "migrations")
	for _, e := range entries {
		if e.Name() >= "0022" {
			continue
		}
		b, _ := fs.ReadFile(cladex.MigrationsFS, "migrations/"+e.Name())
		before["migrations/"+e.Name()] = &fstest.MapFile{Data: b}
	}
	dsn := filepath.Join(t.TempDir(), "test.db")
	old, err := Open(ctx, dsn, before)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.db.ExecContext(ctx, `
		INSERT INTO customers (name, tax_regime, notes) VALUES
			('A', '601 - General de Ley Personas Morales', NULL),
			('B', 'Persona moral', 'cliente antiguo'),
			('C', '  626', NULL),
			('D', '999 algo', NULL),
			('E', '', NULL),
			('F', NULL, NULL)`); err != nil {
		t.Fatal(err)
	}
	old.Close()

	s, err := Open(ctx, dsn, cladex.MigrationsFS)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	all, err := s.ListCustomers(ctx, "", "name", "asc", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][2]string{
		"A": {"601", ""},
		"B": {"", "cliente antiguo\nRégimen fiscal anterior: Persona moral"},
		"C": {"626", ""},
		"D": {"", "Régimen fiscal anterior: 999 algo"},
		"E": {"", ""},
		"F": {"", ""},
	}
	for _, c := range all {
		if w := want[c.Name]; c.TaxRegime != w[0] || c.Notes != w[1] || c.LegalName != "" || c.CFDIUse != "" {
			t.Errorf("%s after 0022: regime %q notes %q legal %q use %q; want %q, %q", c.Name, c.TaxRegime, c.Notes, c.LegalName, c.CFDIUse, w[0], w[1])
		}
	}
	all[0].LegalName, all[0].CFDIUse = "EMPRESA A", "G01"
	if err := s.UpdateCustomer(ctx, all[0]); err != nil {
		t.Fatal(err)
	}
	var newValues string
	if err := s.db.QueryRowContext(ctx, `SELECT new_values FROM audit_log WHERE table_name = 'customers' ORDER BY id DESC LIMIT 1`).Scan(&newValues); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"legal_name":"EMPRESA A"`, `"cfdi_use":"G01"`, `"tax_regime":"601"`} {
		if !strings.Contains(newValues, want) {
			t.Errorf("audit new_values %s missing %s", newValues, want)
		}
	}
}
