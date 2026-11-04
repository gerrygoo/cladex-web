package store

import (
	"context"
	"strconv"
	"testing"
)

// A product's own SAT codes (issue #62): saved, cleared, audited, and found by the
// "Sin datos para facturar" filter.
func TestProductSATCodes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	userID, err := s.CreateUser(ctx, "admin", "Admin", "hash", RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	actx := WithActor(ctx, Actor{UserID: userID, Source: SourceWeb})
	fam, err := s.UpsertFamily(ctx, "CCA", "CCA")
	if err != nil {
		t.Fatal(err)
	}

	full, err := s.CreateProduct(actx, Product{FamilyID: fam, SKU: "CCA-C14", Description: "Cable 14",
		SATProductCode: "26121600", SATUnitCode: "MTR"})
	if err != nil {
		t.Fatal(err)
	}
	half, err := s.CreateProduct(actx, Product{FamilyID: fam, SKU: "CCA-C12", Description: "Cable 12",
		SATProductCode: "26121600"})
	if err != nil {
		t.Fatal(err)
	}
	none, err := s.CreateProduct(actx, Product{FamilyID: fam, SKU: "CCA-C10", Description: "Cable 10"})
	if err != nil {
		t.Fatal(err)
	}

	p, err := s.ProductByID(ctx, full)
	if err != nil || p.SATProductCode != "26121600" || p.SATUnitCode != "MTR" || p.MissingSAT() {
		t.Fatalf("ProductByID = %+v, %v", p, err)
	}
	if p, _ := s.ProductByID(ctx, half); p.SATUnitCode != "" || !p.MissingSAT() {
		t.Errorf("a product with only the product code = %+v; want it missing SAT data", p)
	}

	skus := func(state string) []string {
		ps, err := s.ListProducts(ctx, "", "sku", "", Filters{"sat": {state}})
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, p := range ps {
			out = append(out, p.SKU)
		}
		return out
	}
	if got := skus(SATFaltan); len(got) != 2 || got[0] != "CCA-C10" || got[1] != "CCA-C12" {
		t.Errorf("Sin datos para facturar = %v; want CCA-C10, CCA-C12", got)
	}
	if got := skus(SATCompletos); len(got) != 1 || got[0] != "CCA-C14" {
		t.Errorf("completos = %v; want CCA-C14", got)
	}

	// Updating sets and clears them; NULL, not "", when cleared.
	if err := s.UpdateProduct(actx, Product{ID: none, FamilyID: fam, SKU: "CCA-C10", Description: "Cable 10",
		SATProductCode: "26121609", SATUnitCode: "XRL"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateProduct(actx, Product{ID: full, FamilyID: fam, SKU: "CCA-C14", Description: "Cable 14"}); err != nil {
		t.Fatal(err)
	}
	var nulls int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM products WHERE id = ? AND sat_product_code IS NULL AND sat_unit_code IS NULL`,
		full).Scan(&nulls); err != nil || nulls != 1 {
		t.Errorf("cleared codes aren't NULL: %d, %v", nulls, err)
	}
	if got := skus(SATCompletos); len(got) != 1 || got[0] != "CCA-C10" {
		t.Errorf("completos after the edits = %v; want CCA-C10", got)
	}

	// The audit trail records who set them.
	entries, err := s.AuditLog(ctx, AuditFilter{TableName: "products", RowKey: strconv.FormatInt(none, 10)})
	if err != nil || len(entries) != 2 {
		t.Fatalf("audit for CCA-C10 = %d entries, %v; want insert and update", len(entries), err)
	}
	e := entries[0]
	if e.Op != "update" || e.ActorID == nil || *e.ActorID != userID ||
		e.OldValues["sat_product_code"] != nil || e.NewValues["sat_product_code"] != "26121609" ||
		e.NewValues["sat_unit_code"] != "XRL" {
		t.Errorf("audit update = %+v", e)
	}
	if _, ok := entries[1].NewValues["sat_unit_code"]; !ok {
		t.Errorf("audit insert has no sat_unit_code: %v", entries[1].NewValues)
	}
}

// ByWeight marks the familias whose quotes go per metre by weight: the copper-priced
// one (CCS & AC, QS) and any kg-priced one (QG); not CCA.
func TestListFamiliesByWeight(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	families, err := s.ListFamilies(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, f := range families {
		got[f.Name] = f.ByWeight
	}
	if !got["CCS & AC"] || got["CCA"] {
		t.Errorf("ByWeight = %v; want CCS & AC only", got)
	}
}
