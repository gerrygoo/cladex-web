package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gerrygoo/cladex-web/internal/money"
)

// Material is a materials row joined with its unit's code: a raw material with a
// pure-cost price per unit (e.g. CCS 30% at $155/kg). UpdatedAt is when the price or
// name last changed, for the "actualizado hace N días" hint. See
// migrations/0008_materials.sql.
type Material struct {
	ID          int64
	Name        string
	UnitID      int64
	UnitCode    string
	PriceMicros money.Micros
	UpdatedAt   string
}

// ProductMaterial is one product_materials row joined with its material: how much of
// the material (QtyPerUnitMicros, in the material's unit) one unit of the product holds.
type ProductMaterial struct {
	ID               int64
	ProductID        int64
	QtyPerUnitMicros money.Micros
	Material         Material
}

var (
	// ErrDuplicateMaterialName is returned when another material already has the name.
	ErrDuplicateMaterialName = errors.New("store: a material with this name already exists")
	// ErrMaterialNotFound is returned when the id matches no material.
	ErrMaterialNotFound = errors.New("store: material not found")
	// ErrDuplicateProductMaterial is returned when the product already lists the material.
	ErrDuplicateProductMaterial = errors.New("store: product already has this material")
)

const materialCols = `m.id, m.name, m.unit_id, u.code, m.price_micros, m.updated_at`

const materialFrom = ` FROM materials m JOIN units u ON u.id = m.unit_id`

// ListMaterials returns every material ordered by name.
func (s *Store) ListMaterials(ctx context.Context) ([]Material, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+materialCols+materialFrom+` ORDER BY m.name`)
	if err != nil {
		return nil, fmt.Errorf("store: list materials: %w", err)
	}
	defer rows.Close()

	var materials []Material
	for rows.Next() {
		var m Material
		if err := rows.Scan(&m.ID, &m.Name, &m.UnitID, &m.UnitCode, &m.PriceMicros, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: list materials: %w", err)
		}
		materials = append(materials, m)
	}
	return materials, rows.Err()
}

func isUniqueViolation(err error, column string) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed: "+column)
}

// CreateMaterial inserts a material and returns its id. Its unit is fixed from here on:
// changing it would silently change the meaning of every product's quantity of it.
func (s *Store) CreateMaterial(ctx context.Context, name string, unitID int64, price money.Micros, updatedBy int64) (int64, error) {
	res, err := s.exec(ctx, `
		INSERT INTO materials (name, unit_id, price_micros, updated_by) VALUES (?, ?, ?, ?)`,
		name, unitID, int64(price), nullIfZero(updatedBy))
	if isUniqueViolation(err, "materials.name") {
		return 0, ErrDuplicateMaterialName
	}
	if err != nil {
		return 0, fmt.Errorf("store: create material %q: %w", name, err)
	}
	return res.LastInsertId()
}

// UpdateMaterial renames and/or reprices a material. Every draft quoting a product made
// of it reprices the next time it's opened.
func (s *Store) UpdateMaterial(ctx context.Context, id int64, name string, price money.Micros, updatedBy int64) error {
	res, err := s.exec(ctx, `
		UPDATE materials SET
			name = ?, price_micros = ?, updated_by = ?,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?`,
		name, int64(price), nullIfZero(updatedBy), id)
	if isUniqueViolation(err, "materials.name") {
		return ErrDuplicateMaterialName
	}
	if err != nil {
		return fmt.Errorf("store: update material %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrMaterialNotFound
	}
	return nil
}

// ListProductMaterials returns what one unit of a product is made of, ordered by
// material name.
func (s *Store) ListProductMaterials(ctx context.Context, productID int64) ([]ProductMaterial, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT pm.id, pm.product_id, pm.qty_per_unit_micros, `+materialCols+`
		FROM product_materials pm
		JOIN materials m ON m.id = pm.material_id
		JOIN units u ON u.id = m.unit_id
		WHERE pm.product_id = ?
		ORDER BY m.name`, productID)
	if err != nil {
		return nil, fmt.Errorf("store: list materials of product %d: %w", productID, err)
	}
	defer rows.Close()

	var out []ProductMaterial
	for rows.Next() {
		var pm ProductMaterial
		m := &pm.Material
		if err := rows.Scan(&pm.ID, &pm.ProductID, &pm.QtyPerUnitMicros,
			&m.ID, &m.Name, &m.UnitID, &m.UnitCode, &m.PriceMicros, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: list materials of product %d: %w", productID, err)
		}
		out = append(out, pm)
	}
	return out, rows.Err()
}

// AddProductMaterial records that one unit of a product holds qty of a material.
func (s *Store) AddProductMaterial(ctx context.Context, productID, materialID int64, qty money.Micros) (int64, error) {
	res, err := s.exec(ctx, `
		INSERT INTO product_materials (product_id, material_id, qty_per_unit_micros) VALUES (?, ?, ?)`,
		productID, materialID, int64(qty))
	if isUniqueViolation(err, "product_materials.product_id") {
		return 0, ErrDuplicateProductMaterial
	}
	if err != nil {
		return 0, fmt.Errorf("store: add material %d to product %d: %w", materialID, productID, err)
	}
	return res.LastInsertId()
}

// DeleteProductMaterial removes one of a product's materials. productID scopes the
// delete so a stale or forged URL can't remove another product's row.
func (s *Store) DeleteProductMaterial(ctx context.Context, productID, id int64) error {
	if _, err := s.exec(ctx, `DELETE FROM product_materials WHERE id = ? AND product_id = ?`, id, productID); err != nil {
		return fmt.Errorf("store: delete product material %d: %w", id, err)
	}
	return nil
}
