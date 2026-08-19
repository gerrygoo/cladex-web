package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/gerrygoo/cladex-web/internal/money"
)

// Unit is a units row — a shared unit of measure (m, kg, pza, rollo, ...) that
// products and their conversion rates reference. Seeded with a starter set in
// migrations/0003_add_units_and_conversions.sql; admins can add more via /unidades.
type Unit struct {
	ID   int64
	Code string
	Name string
}

// ListUnits returns all units ordered by code.
func (s *Store) ListUnits(ctx context.Context) ([]Unit, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, code, name FROM units ORDER BY code`)
	if err != nil {
		return nil, fmt.Errorf("store: list units: %w", err)
	}
	defer rows.Close()

	var units []Unit
	for rows.Next() {
		var u Unit
		if err := rows.Scan(&u.ID, &u.Code, &u.Name); err != nil {
			return nil, fmt.Errorf("store: list units: %w", err)
		}
		units = append(units, u)
	}
	return units, rows.Err()
}

// ErrDuplicateUnitCode is returned by CreateUnit when a unit with the same code
// already exists.
var ErrDuplicateUnitCode = errors.New("store: a unit with this code already exists")

// CreateUnit inserts a new unit, returning its id. Returns ErrDuplicateUnitCode if
// code is already taken.
func (s *Store) CreateUnit(ctx context.Context, code, name string) (int64, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM units WHERE code = ?`, code).Scan(&exists); err != nil {
		return 0, fmt.Errorf("store: check existing unit %q: %w", code, err)
	}
	if exists > 0 {
		return 0, ErrDuplicateUnitCode
	}

	res, err := s.db.ExecContext(ctx, `INSERT INTO units (code, name) VALUES (?, ?)`, code, name)
	if err != nil {
		return 0, fmt.Errorf("store: create unit %q: %w", code, err)
	}
	return res.LastInsertId()
}

// ProductUnitConversion is a product_unit_conversions row, joined with both units'
// codes for display.
type ProductUnitConversion struct {
	ID           int64
	ProductID    int64
	FromUnitID   int64
	FromUnitCode string
	ToUnitID     int64
	ToUnitCode   string
	RateMicros   money.Micros
}

// ErrDuplicateUnitPair is returned by CreateConversion when the product already has a
// conversion between the same two units, in either direction — see
// migrations/0003_add_units_and_conversions.sql's comment on why the DB's own unique
// index doesn't catch the reverse-direction case.
var ErrDuplicateUnitPair = errors.New("store: product already has a conversion between these units")

// ListConversionsByProduct returns a product's unit conversions, ordered by id.
func (s *Store) ListConversionsByProduct(ctx context.Context, productID int64) ([]ProductUnitConversion, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.product_id, c.from_unit_id, fu.code, c.to_unit_id, tu.code, c.rate_micros
		FROM product_unit_conversions c
		JOIN units fu ON fu.id = c.from_unit_id
		JOIN units tu ON tu.id = c.to_unit_id
		WHERE c.product_id = ?
		ORDER BY c.id`, productID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list conversions for product %d: %w", productID, err)
	}
	defer rows.Close()

	var conversions []ProductUnitConversion
	for rows.Next() {
		var c ProductUnitConversion
		var rate int64
		if err := rows.Scan(&c.ID, &c.ProductID, &c.FromUnitID, &c.FromUnitCode,
			&c.ToUnitID, &c.ToUnitCode, &rate); err != nil {
			return nil, fmt.Errorf("store: list conversions for product %d: %w", productID, err)
		}
		c.RateMicros = money.Micros(rate)
		conversions = append(conversions, c)
	}
	return conversions, rows.Err()
}

// CreateConversion inserts a conversion rate between two units for a product. Returns
// ErrDuplicateUnitPair if a conversion already exists between the same pair (in either
// direction) — for now each unordered {from, to} pair is unique per product.
func (s *Store) CreateConversion(ctx context.Context, productID, fromUnitID, toUnitID int64, rateMicros money.Micros) (int64, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, `
		SELECT count(*) FROM product_unit_conversions
		WHERE product_id = ? AND (
			(from_unit_id = ? AND to_unit_id = ?) OR (from_unit_id = ? AND to_unit_id = ?)
		)`, productID, fromUnitID, toUnitID, toUnitID, fromUnitID,
	).Scan(&exists)
	if err != nil {
		return 0, fmt.Errorf("store: check existing conversion: %w", err)
	}
	if exists > 0 {
		return 0, ErrDuplicateUnitPair
	}

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO product_unit_conversions (product_id, from_unit_id, to_unit_id, rate_micros)
		VALUES (?, ?, ?, ?)`,
		productID, fromUnitID, toUnitID, int64(rateMicros),
	)
	if err != nil {
		return 0, fmt.Errorf("store: create conversion for product %d: %w", productID, err)
	}
	return res.LastInsertId()
}

// DeleteConversion removes a conversion rate by id.
func (s *Store) DeleteConversion(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM product_unit_conversions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete conversion %d: %w", id, err)
	}
	return nil
}
