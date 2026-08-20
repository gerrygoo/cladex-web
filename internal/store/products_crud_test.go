package store

import (
	"context"
	"testing"

	"github.com/gerrygoo/cladex-web/internal/money"
)

func TestProductCRUDLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	familyID, err := s.UpsertFamily(ctx, "ABASTILUM", "Abastilum")
	if err != nil {
		t.Fatalf("UpsertFamily: %v", err)
	}

	families, err := s.ListFamilies(ctx)
	if err != nil {
		t.Fatalf("ListFamilies: %v", err)
	}
	if len(families) != 1 || families[0].ID != familyID || families[0].Name != "ABASTILUM" {
		t.Fatalf("ListFamilies = %+v", families)
	}

	if p, err := s.ProductBySKU(ctx, "abl-cable-thw-14"); err != nil || p != nil {
		t.Fatalf("ProductBySKU(nonexistent) = %+v, %v; want nil, nil", p, err)
	}

	price := money.Micros(6_319_872)
	weight := money.Micros(123_000)
	id, err := s.CreateProduct(ctx, Product{
		FamilyID:        familyID,
		SKU:             "abl-cable-thw-14",
		Description:     "Cable THW Cal. 14",
		UnitPriceMicros: &price,
		KgPerMMicros:    &weight,
		Currency:        "MXN",
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	got, err := s.ProductByID(ctx, id)
	if err != nil {
		t.Fatalf("ProductByID: %v", err)
	}
	if got == nil {
		t.Fatal("ProductByID = nil, want a product")
	}
	if got.SKU != "abl-cable-thw-14" || got.Description != "Cable THW Cal. 14" || got.FamilyName != "ABASTILUM" {
		t.Fatalf("ProductByID = %+v", got)
	}
	if got.UnitPriceMicros == nil || *got.UnitPriceMicros != price {
		t.Fatalf("UnitPriceMicros = %v, want %v", got.UnitPriceMicros, price)
	}
	if got.CostMicros != nil {
		t.Fatalf("CostMicros = %v, want nil", got.CostMicros)
	}

	bySKU, err := s.ProductBySKU(ctx, "abl-cable-thw-14")
	if err != nil {
		t.Fatalf("ProductBySKU: %v", err)
	}
	if bySKU == nil || bySKU.ID != id {
		t.Fatalf("ProductBySKU = %+v, want id %d", bySKU, id)
	}

	// Update: change description and clear unit price in favor of cost.
	cost := money.Micros(4_500_000)
	got.Description = "Cable THW Cal. 14 AWG"
	got.UnitPriceMicros = nil
	got.CostMicros = &cost
	if err := s.UpdateProduct(ctx, *got); err != nil {
		t.Fatalf("UpdateProduct: %v", err)
	}

	updated, err := s.ProductByID(ctx, id)
	if err != nil {
		t.Fatalf("ProductByID after update: %v", err)
	}
	if updated.Description != "Cable THW Cal. 14 AWG" {
		t.Fatalf("Description = %q, want updated", updated.Description)
	}
	if updated.UnitPriceMicros != nil {
		t.Fatalf("UnitPriceMicros = %v, want nil after clearing", updated.UnitPriceMicros)
	}
	if updated.CostMicros == nil || *updated.CostMicros != cost {
		t.Fatalf("CostMicros = %v, want %v", updated.CostMicros, cost)
	}

	// List + search.
	products, err := s.ListProducts(ctx, "", "", "")
	if err != nil {
		t.Fatalf("ListProducts(\"\"): %v", err)
	}
	if len(products) != 1 {
		t.Fatalf("ListProducts(\"\") = %d products, want 1", len(products))
	}
	if products, err = s.ListProducts(ctx, "cal. 14", "", ""); err != nil || len(products) != 1 {
		t.Fatalf("ListProducts(case-insensitive substring) = %d, %v; want 1, nil", len(products), err)
	}
	if products, err = s.ListProducts(ctx, "no existe", "", ""); err != nil || len(products) != 0 {
		t.Fatalf("ListProducts(no match) = %d, %v; want 0, nil", len(products), err)
	}

	// Soft-delete: disappears from list and ByID, but the row survives.
	if err := s.SoftDeleteProduct(ctx, id); err != nil {
		t.Fatalf("SoftDeleteProduct: %v", err)
	}
	if p, err := s.ProductByID(ctx, id); err != nil || p != nil {
		t.Fatalf("ProductByID after delete = %+v, %v; want nil, nil", p, err)
	}
	if products, err := s.ListProducts(ctx, "", "", ""); err != nil || len(products) != 0 {
		t.Fatalf("ListProducts after delete = %d, %v; want 0, nil", len(products), err)
	}
	var deletedAt *string
	if err := s.db.QueryRowContext(ctx, `SELECT deleted_at FROM products WHERE id = ?`, id).Scan(&deletedAt); err != nil {
		t.Fatalf("check deleted_at: %v", err)
	}
	if deletedAt == nil {
		t.Fatal("deleted_at is still NULL after soft-delete")
	}
}

func TestListProductsSearchEscapesWildcards(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	familyID, err := s.UpsertFamily(ctx, "CCA", "CCA")
	if err != nil {
		t.Fatalf("UpsertFamily: %v", err)
	}
	if _, err := s.CreateProduct(ctx, Product{
		FamilyID: familyID, SKU: "cca-c14", Description: "50% descuento especial",
	}); err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if _, err := s.CreateProduct(ctx, Product{
		FamilyID: familyID, SKU: "cca-c12", Description: "Cable normal",
	}); err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	// A literal "%" in the search text must not act as a wildcard matching everything.
	products, err := s.ListProducts(ctx, "50%", "", "")
	if err != nil {
		t.Fatalf("ListProducts: %v", err)
	}
	if len(products) != 1 || products[0].SKU != "cca-c14" {
		t.Fatalf("ListProducts(%%q) = %+v, want just cca-c14", products)
	}
}

func TestListProductsSortSKUIsNatural(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	familyID, err := s.UpsertFamily(ctx, "CCA", "CCA")
	if err != nil {
		t.Fatalf("UpsertFamily: %v", err)
	}
	// Inserted out of natural order, and with SKUs that would sort differently
	// byte-by-byte ("SKU-10" < "SKU-2" lexicographically) than numerically.
	for _, sku := range []string{"SKU-10", "SKU-2", "SKU-1"} {
		if _, err := s.CreateProduct(ctx, Product{FamilyID: familyID, SKU: sku}); err != nil {
			t.Fatalf("CreateProduct(%q): %v", sku, err)
		}
	}

	asc, err := s.ListProducts(ctx, "", "sku", "asc")
	if err != nil {
		t.Fatalf("ListProducts(sort=sku, asc): %v", err)
	}
	gotAsc := []string{asc[0].SKU, asc[1].SKU, asc[2].SKU}
	wantAsc := []string{"SKU-1", "SKU-2", "SKU-10"}
	for i := range wantAsc {
		if gotAsc[i] != wantAsc[i] {
			t.Fatalf("ListProducts(sort=sku, asc) = %v, want %v", gotAsc, wantAsc)
		}
	}

	desc, err := s.ListProducts(ctx, "", "sku", "desc")
	if err != nil {
		t.Fatalf("ListProducts(sort=sku, desc): %v", err)
	}
	gotDesc := []string{desc[0].SKU, desc[1].SKU, desc[2].SKU}
	wantDesc := []string{"SKU-10", "SKU-2", "SKU-1"}
	for i := range wantDesc {
		if gotDesc[i] != wantDesc[i] {
			t.Fatalf("ListProducts(sort=sku, desc) = %v, want %v", gotDesc, wantDesc)
		}
	}

	// No explicit sort param defaults to natural SKU order, same as sort=sku asc.
	def, err := s.ListProducts(ctx, "", "", "")
	if err != nil {
		t.Fatalf("ListProducts(sort=\"\"): %v", err)
	}
	gotDefault := []string{def[0].SKU, def[1].SKU, def[2].SKU}
	for i := range wantAsc {
		if gotDefault[i] != wantAsc[i] {
			t.Fatalf("ListProducts(sort=\"\") = %v, want %v (natural SKU order)", gotDefault, wantAsc)
		}
	}
}
