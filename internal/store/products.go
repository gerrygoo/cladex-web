package store

import (
	"context"
	"database/sql"
	"fmt"
	stdsort "sort"
	"strings"

	"github.com/gerrygoo/cladex-web/internal/money"
)

// Product is a catalog row. KgPerMMicros and CostMicros are nil when the column is NULL.
// CostMicros is the flat, pre-margin MXN cost (migrations/0002_add_product_cost.sql);
// what a product is made of adds to it through its product_materials (HasMaterials, set
// by the CRUD read paths; see migrations/0008_materials.sql). KgPerMMicros is reference
// weight only. ID and FamilyName are
// populated by the CRUD read paths (ListProducts, ProductByID, ProductBySKU); they're
// left zero by the import path, which only ever upserts by SKU.
type Product struct {
	ID           int64
	FamilyID     int64
	FamilyName   string
	SKU          string
	Description  string
	KgPerMMicros *money.Micros
	CostMicros   *money.Micros
	UnitID       *int64
	UnitCode     string // populated by the CRUD read paths when UnitID is set
	UnitName     string // populated by the CRUD read paths when UnitID is set
	HasMaterials bool
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
	if _, err := s.exec(ctx, `
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
	p.kg_per_m_micros, p.cost_micros,
	p.unit_id, u.code, u.name,
	EXISTS (SELECT 1 FROM product_materials pm WHERE pm.product_id = p.id)`

const productFrom = `
	FROM products p
	JOIN product_families pf ON pf.id = p.family_id
	LEFT JOIN units u ON u.id = p.unit_id`

func scanProduct(row interface{ Scan(...any) error }) (*Product, error) {
	var p Product
	var kgPerM, cost, unitID sql.NullInt64
	var unitCode, unitName sql.NullString
	err := row.Scan(&p.ID, &p.FamilyID, &p.FamilyName, &p.SKU, &p.Description,
		&kgPerM, &cost, &unitID, &unitCode, &unitName, &p.HasMaterials)
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

// ListFamilies returns the product families that hold catalog products, ordered by
// name. Free-lines-only familias (QL) are left out: products can't be filed under them.
func (s *Store) ListFamilies(ctx context.Context) ([]ProductFamily, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name FROM product_families WHERE free_lines_only = 0 ORDER BY name`)
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
// entry (sku) is the default when sort doesn't match a known column.
var productSortColumns = []sortColumn{
	{"sku", "p.sku"},
	{"description", "p.description"},
	{"familia", "pf.name"},
	{"costo", "p.cost_micros"},
	{"unidad", "u.code"},
}

// productFilterColumns are the per-column filters ListProducts accepts. The cost
// filter skips products costed only from their materials.
var productFilterColumns = []filterColumn{
	{name: "sku", expr: "p.sku", kind: FilterText},
	{name: "description", expr: "p.description", kind: FilterText},
	{name: "familia", expr: "pf.name", kind: FilterEnum},
	{name: "costo", expr: "p.cost_micros", kind: FilterNumber, scale: 1_000_000},
	{name: "unidad", expr: "u.code", kind: FilterEnum},
}

// ListProducts returns non-deleted products, optionally filtered by a case-insensitive
// substring match on SKU or description, and sorted per sort/dir (see
// productSortColumns for the allowed sort column names; dir is "asc" or "desc"). The
// default sort ("awg", also what an empty sort means) orders by wire gauge — see
// awgLess. The "sku" column — including when sort doesn't match a known column — sorts
// naturally (digit runs compare by value, so "SKU-9" < "SKU-10") rather than
// byte-by-byte, since SQL's ORDER BY has no notion of that — see naturalLess.
func (s *Store) ListProducts(ctx context.Context, query, sort, dir string, filters Filters) ([]Product, error) {
	like := "%" + escapeLike(query) + "%"
	sortCol := sort
	if sortCol == "" {
		sortCol = "awg"
	}
	orderBy := orderByClause(productSortColumns, sortCol, dir)
	if sortCol == "sku" || sortCol == "awg" {
		// Re-sorted naturally in Go below; order here only needs to be deterministic.
		orderBy = "ORDER BY p.id ASC"
	}
	extra, extraArgs := filterWhere(ctx, productFilterColumns, filters)
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+productSelectCols+`
		`+productFrom+`
		WHERE p.deleted_at IS NULL
		  AND (? = '' OR p.sku LIKE ? ESCAPE '\' COLLATE NOCASE OR p.description LIKE ? ESCAPE '\' COLLATE NOCASE)`+extra+`
		`+orderBy, append([]any{query, like, like}, extraArgs...)...,
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
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if sortCol == "awg" {
		stdsort.SliceStable(products, func(i, j int) bool {
			return awgLess(products[i], products[j], dir == "desc")
		})
	}
	if sortCol == "sku" {
		stdsort.SliceStable(products, func(i, j int) bool {
			if dir == "desc" {
				return naturalLess(products[j].SKU, products[i].SKU)
			}
			return naturalLess(products[i].SKU, products[j].SKU)
		})
	}
	return products, nil
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
	res, err := s.exec(ctx, `
		INSERT INTO products (family_id, sku, description, kg_per_m_micros, cost_micros, unit_id)
		VALUES (?, ?, ?, ?, ?, ?)`,
		p.FamilyID, p.SKU, p.Description, microsPtr(p.KgPerMMicros), microsPtr(p.CostMicros), idPtr(p.UnitID),
	)
	if err != nil {
		return 0, fmt.Errorf("store: create product %q: %w", p.SKU, err)
	}
	return res.LastInsertId()
}

// UpdateProduct overwrites an existing product's editable fields, identified by p.ID.
func (s *Store) UpdateProduct(ctx context.Context, p Product) error {
	_, err := s.exec(ctx, `
		UPDATE products SET
			family_id          = ?,
			sku                = ?,
			description        = ?,
			kg_per_m_micros    = ?,
			cost_micros        = ?,
			unit_id            = ?,
			updated_at         = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?`,
		p.FamilyID, p.SKU, p.Description, microsPtr(p.KgPerMMicros), microsPtr(p.CostMicros), idPtr(p.UnitID), p.ID,
	)
	if err != nil {
		return fmt.Errorf("store: update product %d: %w", p.ID, err)
	}
	return nil
}

// SoftDeleteProduct sets deleted_at, hiding the product from ListProducts/ProductByID.
// Products are never hard-deleted — old quote_lines may still reference them.
func (s *Store) SoftDeleteProduct(ctx context.Context, id int64) error {
	_, err := s.exec(ctx, `
		UPDATE products SET deleted_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?`, id,
	)
	if err != nil {
		return fmt.Errorf("store: soft-delete product %d: %w", id, err)
	}
	return nil
}
