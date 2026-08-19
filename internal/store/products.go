package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/pricing"
)

// Product is a catalog row. KgPerMMicros, UnitPriceMicros, and CostMicros are nil when
// the corresponding column is NULL — see migrations/0001_init.sql and
// migrations/0002_add_product_cost.sql for what each means. ID and FamilyName are
// populated by the CRUD read paths (ListProducts, ProductByID, ProductBySKU); they're
// left zero by the import path, which only ever upserts by SKU.
type Product struct {
	ID              int64
	FamilyID        int64
	FamilyName      string
	SKU             string
	Description     string
	KgPerMMicros    *money.Micros
	UnitPriceMicros *money.Micros
	CostMicros      *money.Micros
	Currency        string // "MXN" or "USD"; defaults to "MXN" if empty
	UnitID          *int64
	UnitCode        string // populated by the CRUD read paths when UnitID is set
	UnitName        string // populated by the CRUD read paths when UnitID is set
}

// ProductFamily is a product_families row, for populating the product form's family
// selector.
type ProductFamily struct {
	ID   int64
	Name string
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
			cost_micros, currency, unit_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (sku) DO UPDATE SET
			family_id          = excluded.family_id,
			description         = excluded.description,
			kg_per_m_micros    = excluded.kg_per_m_micros,
			unit_price_micros  = excluded.unit_price_micros,
			cost_micros        = excluded.cost_micros,
			currency            = excluded.currency,
			unit_id             = excluded.unit_id,
			updated_at          = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')`,
		p.FamilyID, p.SKU, p.Description, microsPtr(p.KgPerMMicros),
		microsPtr(p.UnitPriceMicros), microsPtr(p.CostMicros), currency, idPtr(p.UnitID),
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

func idPtr(id *int64) any {
	if id == nil {
		return nil
	}
	return *id
}

const productSelectCols = `
	p.id, p.family_id, pf.name, p.sku, p.description,
	p.kg_per_m_micros, p.unit_price_micros, p.cost_micros, p.currency,
	p.unit_id, u.code, u.name`

const productFrom = `
	FROM products p
	JOIN product_families pf ON pf.id = p.family_id
	LEFT JOIN units u ON u.id = p.unit_id`

func scanProduct(row interface{ Scan(...any) error }) (*Product, error) {
	var p Product
	var kgPerM, unitPrice, cost, unitID sql.NullInt64
	var unitCode, unitName sql.NullString
	err := row.Scan(&p.ID, &p.FamilyID, &p.FamilyName, &p.SKU, &p.Description,
		&kgPerM, &unitPrice, &cost, &p.Currency, &unitID, &unitCode, &unitName)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if kgPerM.Valid {
		m := money.Micros(kgPerM.Int64)
		p.KgPerMMicros = &m
	}
	if unitPrice.Valid {
		m := money.Micros(unitPrice.Int64)
		p.UnitPriceMicros = &m
	}
	if cost.Valid {
		m := money.Micros(cost.Int64)
		p.CostMicros = &m
	}
	if unitID.Valid {
		id := unitID.Int64
		p.UnitID = &id
		p.UnitCode = unitCode.String
		p.UnitName = unitName.String
	}
	return &p, nil
}

// ListFamilies returns all product families ordered by name.
func (s *Store) ListFamilies(ctx context.Context) ([]ProductFamily, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name FROM product_families ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("store: list families: %w", err)
	}
	defer rows.Close()

	var families []ProductFamily
	for rows.Next() {
		var f ProductFamily
		if err := rows.Scan(&f.ID, &f.Name); err != nil {
			return nil, fmt.Errorf("store: list families: %w", err)
		}
		families = append(families, f)
	}
	return families, rows.Err()
}

// productSortColumns is the sortable-column whitelist for ListProducts; the first
// entry (description) is the default when sort doesn't match a known column.
var productSortColumns = []sortColumn{
	{"description", "p.description"},
	{"sku", "p.sku"},
	{"familia", "pf.name"},
	{"precio", "COALESCE(p.unit_price_micros, p.cost_micros)"},
	{"moneda", "p.currency"},
	{"unidad", "u.code"},
}

// ListProducts returns non-deleted products, optionally filtered by a case-insensitive
// substring match on SKU or description, and sorted per sort/dir (see
// productSortColumns for the allowed sort column names; dir is "asc" or "desc").
func (s *Store) ListProducts(ctx context.Context, query, sort, dir string) ([]Product, error) {
	like := "%" + escapeLike(query) + "%"
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+productSelectCols+`
		`+productFrom+`
		WHERE p.deleted_at IS NULL
		  AND (? = '' OR p.sku LIKE ? ESCAPE '\' COLLATE NOCASE OR p.description LIKE ? ESCAPE '\' COLLATE NOCASE)
		`+orderByClause(productSortColumns, sort, dir), query, like, like,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list products: %w", err)
	}
	defer rows.Close()

	var products []Product
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list products: %w", err)
		}
		products = append(products, *p)
	}
	return products, rows.Err()
}

// escapeLike escapes SQL LIKE wildcards in user-supplied search text so a SKU or
// description containing "%" or "_" is matched literally.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// ProductByID returns the non-deleted product with the given id, or nil if none exists.
func (s *Store) ProductByID(ctx context.Context, id int64) (*Product, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+productSelectCols+`
		`+productFrom+`
		WHERE p.id = ? AND p.deleted_at IS NULL`, id,
	)
	p, err := scanProduct(row)
	if err != nil {
		return nil, fmt.Errorf("store: product by id %d: %w", id, err)
	}
	return p, nil
}

// ProductBySKU returns the non-deleted product with the given SKU, or nil if none
// exists. Used to give a friendly "SKU already taken" form error instead of a raw
// UNIQUE constraint failure.
func (s *Store) ProductBySKU(ctx context.Context, sku string) (*Product, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+productSelectCols+`
		`+productFrom+`
		WHERE p.sku = ? AND p.deleted_at IS NULL`, sku,
	)
	p, err := scanProduct(row)
	if err != nil {
		return nil, fmt.Errorf("store: product by sku %q: %w", sku, err)
	}
	return p, nil
}

// CreateProduct inserts a new product, returning its id.
func (s *Store) CreateProduct(ctx context.Context, p Product) (int64, error) {
	currency := p.Currency
	if currency == "" {
		currency = "MXN"
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO products (
			family_id, sku, description, kg_per_m_micros, unit_price_micros,
			cost_micros, currency, unit_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.FamilyID, p.SKU, p.Description, microsPtr(p.KgPerMMicros),
		microsPtr(p.UnitPriceMicros), microsPtr(p.CostMicros), currency, idPtr(p.UnitID),
	)
	if err != nil {
		return 0, fmt.Errorf("store: create product %q: %w", p.SKU, err)
	}
	return res.LastInsertId()
}

// UpdateProduct overwrites an existing product's editable fields, identified by p.ID.
func (s *Store) UpdateProduct(ctx context.Context, p Product) error {
	currency := p.Currency
	if currency == "" {
		currency = "MXN"
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE products SET
			family_id          = ?,
			sku                = ?,
			description        = ?,
			kg_per_m_micros    = ?,
			unit_price_micros  = ?,
			cost_micros        = ?,
			currency           = ?,
			unit_id            = ?,
			updated_at         = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?`,
		p.FamilyID, p.SKU, p.Description, microsPtr(p.KgPerMMicros),
		microsPtr(p.UnitPriceMicros), microsPtr(p.CostMicros), currency, idPtr(p.UnitID), p.ID,
	)
	if err != nil {
		return fmt.Errorf("store: update product %d: %w", p.ID, err)
	}
	return nil
}

// ListPriceBreaks returns a product's quantity-tiered price overrides (see 0001's
// price_breaks table), for feeding directly into pricing.UnitPrice. Currently unused by
// any imported product (ELECTRACLEAN, the only real use case, was deferred at import —
// see docs/PLAN.md's 1.2 footnote) but the quote builder calls this generically rather
// than assuming an always-empty slice.
func (s *Store) ListPriceBreaks(ctx context.Context, productID int64) ([]pricing.PriceBreak, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT min_qty_milli, unit_price_micros
		FROM price_breaks
		WHERE product_id = ?
		ORDER BY min_qty_milli`, productID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list price breaks for product %d: %w", productID, err)
	}
	defer rows.Close()

	var breaks []pricing.PriceBreak
	for rows.Next() {
		var b pricing.PriceBreak
		if err := rows.Scan(&b.MinQty, &b.UnitPriceMicros); err != nil {
			return nil, fmt.Errorf("store: list price breaks for product %d: %w", productID, err)
		}
		breaks = append(breaks, b)
	}
	return breaks, rows.Err()
}

// SoftDeleteProduct sets deleted_at, hiding the product from ListProducts/ProductByID.
// Products are never hard-deleted — old quote_lines may still reference them.
func (s *Store) SoftDeleteProduct(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE products SET deleted_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?`, id,
	)
	if err != nil {
		return fmt.Errorf("store: soft-delete product %d: %w", id, err)
	}
	return nil
}
