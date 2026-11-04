package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strconv"
	"testing"

	cladex "github.com/gerrygoo/cladex-web"
)

// countAuditWithoutSupplierSeed counts the audit trail leaving out what 0033's carga
// inicial wrote (Cable Anti Robo, its contacts and its familias), for the migration
// tests that check an earlier migration added no rows of its own.
const countAuditWithoutSupplierSeed = `SELECT count(*) FROM audit_log
	WHERE table_name NOT IN ('suppliers', 'supplier_contacts')
	  AND NOT (table_name = 'product_families' AND actor_id IS NULL
	           AND json_extract(old_values, '$.default_supplier_id') IS NULL
	           AND json_extract(new_values, '$.default_supplier_id') IS NOT NULL)`

func familyIDByName(t *testing.T, s *Store, name string) int64 {
	t.Helper()
	all, err := s.ListAllFamilies(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range all {
		if f.Name == name {
			return f.ID
		}
	}
	t.Fatalf("no familia %q in %+v", name, all)
	return 0
}

func seededSupplier(t *testing.T, s *Store) *Supplier {
	t.Helper()
	rows, err := s.ListSuppliers(context.Background(), "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.RFC == "CAR191126QX6" {
			sup, err := s.SupplierByID(context.Background(), r.ID)
			if err != nil || sup == nil {
				t.Fatalf("SupplierByID: %v, %v", sup, err)
			}
			return sup
		}
	}
	t.Fatalf("Cable Anti Robo not seeded: %+v", rows)
	return nil
}

// Issue #64's carga inicial: Cable Anti Robo, three contacts that receive the O.C., and
// the familias it supplies that exist here (the test database has no Alumoclad).
func TestSupplierSeed(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	sup := seededSupplier(t, s)
	if sup.Name != "Cable Anti Robo" || sup.LegalName != "CABLE ANTI ROBO" || sup.Street != "Camino Real de Carretas" ||
		sup.ExtNumber != "356" || sup.Neighborhood != "Milenio III" || sup.Municipality != "Querétaro" ||
		sup.State != "Querétaro" || sup.PostalCode != "76060" || sup.Inactive {
		t.Errorf("seeded supplier = %+v", sup)
	}
	if len(sup.Contacts) != 3 {
		t.Fatalf("contacts = %+v", sup.Contacts)
	}
	want := []SupplierContact{
		{Name: "Luis Manuel Enríquez (Sr.)", Phone: "+52 442 452 1950", Email: "direccion@cableantirobo.com", ReceivesPO: true},
		{Name: "Luis Enríquez (Jr.)", Phone: "+52 442 157 7006", Email: "luis.enriquez@conduccasa.com", ReceivesPO: true},
		{Name: "Cotizaciones", Role: "Correo general", Email: "cotizaciones@cableantirobo.com", ReceivesPO: true},
	}
	for i, c := range sup.Contacts {
		c.ID = 0
		if c != want[i] {
			t.Errorf("contact %d = %+v, want %+v", i, c, want[i])
		}
	}
	all, _ := s.ListAllFamilies(ctx)
	for _, f := range all {
		supplies := f.Name == "CCA" || f.Name == "CCS & AC"
		if (f.DefaultSupplierID == sup.ID) != supplies {
			t.Errorf("familia %s default supplier = %d (%s)", f.Name, f.DefaultSupplierID, f.DefaultSupplierName)
		}
	}
	entries, err := s.SupplierAuditLog(ctx, sup.ID)
	if err != nil || len(entries) != 6 { // the supplier, three contacts, two familias
		t.Fatalf("seed audit = %+v, %v", entries, err)
	}
	for _, e := range entries {
		if e.ActorID != nil || e.Source != "" {
			t.Errorf("seed audit entry is attributed: %+v", e)
		}
	}
}

// A database where CCS & AC was renamed "CCS" and that has Alumoclad: both get it, by
// the QS quote type and by name.
func TestSupplierSeedFindsRenamedFamilias(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "up.db")
	old, err := Open(ctx, dsn, migrationsBefore(t, "0033"))
	if err != nil {
		t.Fatalf("open at 0032: %v", err)
	}
	for _, q := range []string{
		`UPDATE product_families SET name = 'CCS' WHERE name = 'CCS & AC'`,
		`INSERT INTO product_families (name) VALUES ('Alumoclad'), ('Exceline'), ('ASTRO')`,
	} {
		if _, err := old.db.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	old.Close()
	s, err := Open(ctx, dsn, cladex.MigrationsFS)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	sup := seededSupplier(t, s)
	all, _ := s.ListAllFamilies(ctx)
	got := map[string]bool{}
	for _, f := range all {
		got[f.Name] = f.DefaultSupplierID == sup.ID
	}
	wantSupplied := map[string]bool{"CCA": true, "CCS": true, "Alumoclad": true, "ABASTILUM": false, "QL": false, "Exceline": false, "ASTRO": false}
	for name, want := range wantSupplied {
		if got[name] != want {
			t.Errorf("familia %s supplied by Cable Anti Robo = %v, want %v", name, got[name], want)
		}
	}
}

func TestValidCLABE(t *testing.T) {
	for clabe, want := range map[string]bool{
		"002010077777777771": true, // the standard example: Banamex, check digit 1
		"032180000118359719": true,
		"002010077777777772": false, // wrong check digit
		"00201007777777777":  false, // 17 digits
		"00201007777777777a": false,
		"":                   false,
	} {
		if got := ValidCLABE(clabe); got != want {
			t.Errorf("ValidCLABE(%q) = %v, want %v", clabe, got, want)
		}
	}
}

func testSupplier(name, rfc string) Supplier {
	return Supplier{Name: name, RFC: rfc, LegalName: name, Street: "Av. Uno", ExtNumber: "10",
		Neighborhood: "Centro", Municipality: "Monterrey", State: "Nuevo León", PostalCode: "64000"}
}

func TestSupplierCreateUpdateDelete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seeded := seededSupplier(t, s)
	cca := familyIDByName(t, s, "CCA")
	abastilum := familyIDByName(t, s, "ABASTILUM")
	ql := familyIDByName(t, s, "QL")

	sup := testSupplier("Postes del Norte", "PNO010101AB1")
	days := int64(30)
	sup.CreditDays, sup.Currency, sup.CLABE = &days, "USD", "002010077777777771"
	sup.Contacts = []SupplierContact{{Name: "Ana", Email: "ana@pn.mx", ReceivesPO: true}, {Name: "Beto", Phone: "81 1234 5678"}}
	sup.FamilyIDs = []int64{abastilum, cca, ql} // CCA is taken from Cable Anti Robo; QL is never assigned
	id, err := s.CreateSupplier(ctx, sup)
	if err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}
	got, err := s.SupplierByID(ctx, id)
	if err != nil || got == nil {
		t.Fatalf("SupplierByID: %v, %v", got, err)
	}
	if *got.CreditDays != 30 || got.Currency != "USD" || got.CLABE != sup.CLABE || len(got.Contacts) != 2 ||
		!got.Contacts[0].ReceivesPO || got.Contacts[1].ReceivesPO || len(got.FamilyIDs) != 2 ||
		got.FamilyIDs[0] != cca || got.FamilyIDs[1] != abastilum {
		t.Errorf("created supplier = %+v", got)
	}
	if again, _ := s.SupplierByID(ctx, seeded.ID); len(again.FamilyIDs) != 1 {
		t.Errorf("Cable Anti Robo still has %v after CCA was reassigned", again.FamilyIDs)
	}

	// Edit one contact, drop the other, add a third, and give up ABASTILUM.
	got.Contacts = []SupplierContact{{ID: got.Contacts[0].ID, Name: "Ana Pérez", Email: "ana@pn.mx", ReceivesPO: true}, {Name: "Carla"}}
	got.FamilyIDs = []int64{cca}
	got.CreditDays = nil
	if err := s.UpdateSupplier(ctx, *got); err != nil {
		t.Fatalf("UpdateSupplier: %v", err)
	}
	after, _ := s.SupplierByID(ctx, id)
	if len(after.Contacts) != 2 || after.Contacts[0].Name != "Ana Pérez" || after.Contacts[1].Name != "Carla" ||
		after.CreditDays != nil || len(after.FamilyIDs) != 1 || after.FamilyIDs[0] != cca {
		t.Errorf("updated supplier = %+v", after)
	}

	// Saving the same thing again writes nothing.
	trail := func() int {
		entries, err := s.SupplierAuditLog(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return len(entries)
	}
	before := trail()
	if err := s.UpdateSupplier(ctx, *after); err != nil {
		t.Fatalf("no-op UpdateSupplier: %v", err)
	}
	if n := trail(); n != before {
		t.Errorf("no-op save added %d audit rows", n-before)
	}

	// It supplies CCA, so it can't be deleted; inactive it keeps it.
	var inUse *SupplierInUseError
	if err := s.DeleteSupplier(ctx, id); !errors.As(err, &inUse) || len(inUse.Familias) != 1 || inUse.Familias[0] != "CCA" {
		t.Fatalf("DeleteSupplier with familias = %v", err)
	}
	if err := s.SetSupplierInactive(ctx, id, true); err != nil {
		t.Fatal(err)
	}
	if again, _ := s.SupplierByID(ctx, id); !again.Inactive || len(again.FamilyIDs) != 1 {
		t.Errorf("inactive supplier = %+v", again)
	}
	after.FamilyIDs = nil
	if err := s.UpdateSupplier(ctx, *after); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSupplier(ctx, id); err != nil {
		t.Fatalf("DeleteSupplier without familias: %v", err)
	}
	if gone, _ := s.SupplierByID(ctx, id); gone != nil {
		t.Errorf("deleted supplier still there: %+v", gone)
	}
	var contacts int
	s.db.QueryRowContext(ctx, `SELECT count(*) FROM supplier_contacts WHERE supplier_id = ?`, id).Scan(&contacts)
	if contacts != 0 {
		t.Errorf("%d contacts outlived their supplier", contacts)
	}
	if err := s.DeleteSupplier(ctx, id); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("deleting it again = %v, want sql.ErrNoRows", err)
	}
	if err := s.UpdateSupplier(ctx, Supplier{ID: id}); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("updating a missing supplier = %v", err)
	}
	// The trail keeps the deleted supplier.
	entries, _ := s.AuditLog(ctx, AuditFilter{TableName: "suppliers", RowKey: strconv.FormatInt(id, 10)})
	if len(entries) == 0 || entries[0].Op != "delete" {
		t.Errorf("supplier delete not audited: %+v", entries)
	}
}

func TestSimilarSuppliers(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for _, tc := range []struct {
		name, legal, rfc string
		want             int
	}{
		{"Otro", "Otra", "car191126qx6", 1},                      // same RFC, any case
		{"Cable Anti Robo S.A. de C.V.", "X", "XXX010101AB1", 1}, // same name, razón social ending ignored
		{"Distribuidora", "Cable anti robo", "XXX010101AB1", 1},  // same razón social
		{"Postes del Norte", "Postes", "PNO010101AB1", 0},
	} {
		got, err := s.SimilarSuppliers(ctx, tc.name, tc.legal, tc.rfc)
		if err != nil || len(got) != tc.want {
			t.Errorf("SimilarSuppliers(%q, %q, %q) = %+v, %v; want %d", tc.name, tc.legal, tc.rfc, got, err, tc.want)
		}
	}
}

// "Familias que surte" leaves QL out and labels each familia with its quote types.
func TestSupplierFamilyOptions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	options, err := s.SupplierFamilyOptions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]SupplierFamilyOption{}
	for _, o := range options {
		byName[o.Name] = o
	}
	if _, ok := byName["QL"]; ok {
		t.Error("QL is offered")
	}
	cca := byName["CCA"]
	if len(cca.QuoteTypes) != 1 || cca.QuoteTypes[0] != (FamilyQuoteType{"Cable CCA", "QA"}) || cca.SupplierName != "Cable Anti Robo" {
		t.Errorf("CCA option = %+v", cca)
	}
	if a := byName["ABASTILUM"]; a.SupplierID != 0 || len(a.QuoteTypes) != 1 {
		t.Errorf("ABASTILUM option = %+v", a)
	}
}
