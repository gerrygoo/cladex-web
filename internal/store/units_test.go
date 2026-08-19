package store

import (
	"context"
	"errors"
	"testing"

	"github.com/gerrygoo/cladex-web/internal/money"
)

func unitByCode(t *testing.T, units []Unit, code string) Unit {
	t.Helper()
	for _, u := range units {
		if u.Code == code {
			return u
		}
	}
	t.Fatalf("no seeded unit with code %q in %+v", code, units)
	return Unit{}
}

func TestListUnits_Seeded(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	units, err := s.ListUnits(ctx)
	if err != nil {
		t.Fatalf("ListUnits: %v", err)
	}
	wantCodes := map[string]bool{"m": true, "kg": true, "pza": true, "rollo": true}
	if len(units) != len(wantCodes) {
		t.Fatalf("ListUnits = %+v, want %d seeded units", units, len(wantCodes))
	}
	for _, u := range units {
		if !wantCodes[u.Code] {
			t.Errorf("unexpected seeded unit %+v", u)
		}
	}
}

func TestCreateUnit_DuplicateCode(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.CreateUnit(ctx, "caja", "Caja"); err != nil {
		t.Fatalf("CreateUnit: %v", err)
	}
	if _, err := s.CreateUnit(ctx, "caja", "Caja (otra)"); !errors.Is(err, ErrDuplicateUnitCode) {
		t.Fatalf("CreateUnit(duplicate code) = %v, want ErrDuplicateUnitCode", err)
	}
}

func TestProductUnitID_RoundTrips(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	familyID, err := s.UpsertFamily(ctx, "ABASTILUM", "Abastilum")
	if err != nil {
		t.Fatalf("UpsertFamily: %v", err)
	}
	units, err := s.ListUnits(ctx)
	if err != nil {
		t.Fatalf("ListUnits: %v", err)
	}
	m := unitByCode(t, units, "m")

	price := money.Micros(1_000_000)
	id, err := s.CreateProduct(ctx, Product{
		FamilyID: familyID, SKU: "sku-1", Description: "desc", Currency: "MXN",
		UnitPriceMicros: &price, UnitID: &m.ID,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	got, err := s.ProductByID(ctx, id)
	if err != nil {
		t.Fatalf("ProductByID: %v", err)
	}
	if got.UnitID == nil || *got.UnitID != m.ID || got.UnitCode != "m" || got.UnitName != "Metro" {
		t.Fatalf("ProductByID unit fields = %+v, want unit %+v", got, m)
	}

	// Clearing the unit on update should round-trip back to nil.
	got.UnitID = nil
	if err := s.UpdateProduct(ctx, *got); err != nil {
		t.Fatalf("UpdateProduct: %v", err)
	}
	got, err = s.ProductByID(ctx, id)
	if err != nil {
		t.Fatalf("ProductByID: %v", err)
	}
	if got.UnitID != nil {
		t.Fatalf("UnitID after clearing = %v, want nil", got.UnitID)
	}
}

func TestProductUnitConversions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	familyID, err := s.UpsertFamily(ctx, "ABASTILUM", "Abastilum")
	if err != nil {
		t.Fatalf("UpsertFamily: %v", err)
	}
	price := money.Micros(1_000_000)
	productID, err := s.CreateProduct(ctx, Product{
		FamilyID: familyID, SKU: "sku-1", Description: "desc", Currency: "MXN",
		UnitPriceMicros: &price,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	units, err := s.ListUnits(ctx)
	if err != nil {
		t.Fatalf("ListUnits: %v", err)
	}
	m := unitByCode(t, units, "m")
	rollo := unitByCode(t, units, "rollo")
	kg := unitByCode(t, units, "kg")

	rate := money.MicrosFromFloat(100)
	cid, err := s.CreateConversion(ctx, productID, rollo.ID, m.ID, rate)
	if err != nil {
		t.Fatalf("CreateConversion: %v", err)
	}

	conversions, err := s.ListConversionsByProduct(ctx, productID)
	if err != nil {
		t.Fatalf("ListConversionsByProduct: %v", err)
	}
	if len(conversions) != 1 || conversions[0].ID != cid ||
		conversions[0].FromUnitCode != "rollo" || conversions[0].ToUnitCode != "m" ||
		conversions[0].RateMicros != rate {
		t.Fatalf("ListConversionsByProduct = %+v", conversions)
	}

	// Same pair, reversed direction: rejected as a duplicate unordered pair.
	if _, err := s.CreateConversion(ctx, productID, m.ID, rollo.ID, rate); !errors.Is(err, ErrDuplicateUnitPair) {
		t.Fatalf("CreateConversion(reverse of existing pair) = %v, want ErrDuplicateUnitPair", err)
	}

	// A different pair for the same product is fine.
	if _, err := s.CreateConversion(ctx, productID, kg.ID, m.ID, money.MicrosFromFloat(3.937)); err != nil {
		t.Fatalf("CreateConversion(different pair): %v", err)
	}

	if err := s.DeleteConversion(ctx, cid); err != nil {
		t.Fatalf("DeleteConversion: %v", err)
	}
	conversions, err = s.ListConversionsByProduct(ctx, productID)
	if err != nil {
		t.Fatalf("ListConversionsByProduct after delete: %v", err)
	}
	if len(conversions) != 1 || conversions[0].FromUnitCode != "kg" {
		t.Fatalf("ListConversionsByProduct after delete = %+v, want just the kg->m row", conversions)
	}
}

func TestListProducts_Sort(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	familyID, err := s.UpsertFamily(ctx, "ABASTILUM", "Abastilum")
	if err != nil {
		t.Fatalf("UpsertFamily: %v", err)
	}
	price := money.Micros(1_000_000)
	for _, sku := range []string{"b-sku", "a-sku", "c-sku"} {
		if _, err := s.CreateProduct(ctx, Product{
			FamilyID: familyID, SKU: sku, Description: sku, Currency: "MXN", UnitPriceMicros: &price,
		}); err != nil {
			t.Fatalf("CreateProduct(%q): %v", sku, err)
		}
	}

	asc, err := s.ListProducts(ctx, "", "sku", "asc")
	if err != nil {
		t.Fatalf("ListProducts(sort=sku,asc): %v", err)
	}
	wantAsc := []string{"a-sku", "b-sku", "c-sku"}
	for i, p := range asc {
		if p.SKU != wantAsc[i] {
			t.Fatalf("ListProducts(asc) = %v, want %v", skus(asc), wantAsc)
		}
	}

	desc, err := s.ListProducts(ctx, "", "sku", "desc")
	if err != nil {
		t.Fatalf("ListProducts(sort=sku,desc): %v", err)
	}
	wantDesc := []string{"c-sku", "b-sku", "a-sku"}
	for i, p := range desc {
		if p.SKU != wantDesc[i] {
			t.Fatalf("ListProducts(desc) = %v, want %v", skus(desc), wantDesc)
		}
	}

	// An unrecognized sort column falls back to the default (description) rather
	// than erroring or building unsafe SQL.
	if _, err := s.ListProducts(ctx, "", "'; DROP TABLE products; --", "asc"); err != nil {
		t.Fatalf("ListProducts(unrecognized sort column): %v", err)
	}
}

func skus(products []Product) []string {
	out := make([]string, len(products))
	for i, p := range products {
		out[i] = p.SKU
	}
	return out
}
