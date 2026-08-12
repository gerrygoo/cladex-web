package store

import (
	"context"
	"fmt"

	"github.com/gerrygoo/cladex-web/internal/money"
)

// Product is a catalog row. KgPerMMicros, UnitPriceMicros, and CostMicros are nil when
// the corresponding column is NULL — see migrations/0001_init.sql and
// migrations/0002_add_product_cost.sql for what each means.
type Product struct {
	FamilyID        int64
	SKU             string
	Description     string
	KgPerMMicros    *money.Micros
	UnitPriceMicros *money.Micros
	CostMicros      *money.Micros
	Currency        string // "MXN" or "USD"; defaults to "MXN" if empty
}

// UpsertFamily inserts a product family by name if it doesn't exist, or updates its
// sheet_name if it does, returning the family's id either way.
func (s *Store) UpsertFamily(ctx context.Context, name, sheetName string) (int64, error) {
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO product_families (name, sheet_name) VALUES (?, ?)
		ON CONFLICT (name) DO UPDATE SET sheet_name = excluded.sheet_name`,
		name, sheetName,
	); err != nil {
		return 0, fmt.Errorf("store: upsert family %q: %w", name, err)
	}
	var id int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM product_families WHERE name = ?`, name,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("store: fetch family %q: %w", name, err)
	}
	return id, nil
}

// UpsertProduct inserts a product by SKU if it doesn't exist, or updates its
// description, family, and pricing fields if it does. SKU is the stable identity across
// re-imports.
func (s *Store) UpsertProduct(ctx context.Context, p Product) error {
	currency := p.Currency
	if currency == "" {
		currency = "MXN"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO products (
			family_id, sku, description, kg_per_m_micros, unit_price_micros,
			cost_micros, currency
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (sku) DO UPDATE SET
			family_id          = excluded.family_id,
			description         = excluded.description,
			kg_per_m_micros    = excluded.kg_per_m_micros,
			unit_price_micros  = excluded.unit_price_micros,
			cost_micros        = excluded.cost_micros,
			currency            = excluded.currency,
			updated_at          = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')`,
		p.FamilyID, p.SKU, p.Description, microsPtr(p.KgPerMMicros),
		microsPtr(p.UnitPriceMicros), microsPtr(p.CostMicros), currency,
	)
	if err != nil {
		return fmt.Errorf("store: upsert product %q: %w", p.SKU, err)
	}
	return nil
}

func microsPtr(m *money.Micros) any {
	if m == nil {
		return nil
	}
	return int64(*m)
}
