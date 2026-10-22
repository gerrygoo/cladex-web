package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Customer is a customers row. All fields but ID and Name are optional free text —
// see migrations/0001_init.sql.
type Customer struct {
	ID          int64
	Name        string
	RFC         string
	ContactName string
	Phone       string
	Email       string
	Address     string
	PostalCode  string
	TaxRegime   string // c_RegimenFiscal code (see TaxRegimes), "" when unknown
	Notes       string
	// LegalName is the razón social as registered with the SAT, without the corporate
	// regime; CFDIUse the default uso de CFDI (see CFDIUses). Issue #17.
	LegalName string
	CFDIUse   string
}

const customerSelectCols = `id, name, rfc, contact_name, phone, email, address, postal_code, tax_regime, notes, legal_name, cfdi_use`

func scanCustomer(row interface{ Scan(...any) error }) (*Customer, error) {
	var c Customer
	var rfc, contactName, phone, email, address, postalCode, taxRegime, notes, legalName, cfdiUse sql.NullString
	err := row.Scan(&c.ID, &c.Name, &rfc, &contactName, &phone, &email, &address, &postalCode, &taxRegime, &notes, &legalName, &cfdiUse)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.RFC = rfc.String
	c.ContactName = contactName.String
	c.Phone = phone.String
	c.Email = email.String
	c.Address = address.String
	c.PostalCode = postalCode.String
	c.TaxRegime = taxRegime.String
	c.Notes = notes.String
	c.LegalName = legalName.String
	c.CFDIUse = cfdiUse.String
	return &c, nil
}

// customerSortColumns is the sortable-column whitelist for ListCustomers; the first
// entry (name) is the default when sort doesn't match a known column. Text sorts
// ignore case, so "pruebas" lands between "Pruebas" and "SODIMAC", not after "ZASA"
// (issue #35).
var customerSortColumns = []sortColumn{
	{"name", "name COLLATE NOCASE"},
	{"rfc", "rfc COLLATE NOCASE"},
	{"contact_name", "contact_name COLLATE NOCASE"},
	{"phone", "phone"},
	{"email", "email COLLATE NOCASE"},
}

// customerFilterColumns are the per-column filters ListCustomers accepts.
var customerFilterColumns = []filterColumn{
	{name: "name", expr: "name", kind: FilterText},
	{name: "rfc", expr: "rfc", kind: FilterText},
	{name: "contact_name", expr: "contact_name", kind: FilterText},
	{name: "phone", expr: "phone", kind: FilterText},
	{name: "email", expr: "email", kind: FilterText},
}

// ListCustomers returns non-deleted customers, optionally filtered by a
// case-insensitive substring match on name, RFC, or contact name, and sorted per
// sort/dir (see customerSortColumns; dir is "asc" or "desc").
func (s *Store) ListCustomers(ctx context.Context, query, sort, dir string, filters Filters) ([]Customer, error) {
	like := "%" + escapeLike(query) + "%"
	extra, extraArgs := filterWhere(ctx, customerFilterColumns, filters)
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+customerSelectCols+`
		FROM customers
		WHERE deleted_at IS NULL
		  AND (? = '' OR name LIKE ? ESCAPE '\' COLLATE NOCASE
		           OR rfc LIKE ? ESCAPE '\' COLLATE NOCASE
		           OR contact_name LIKE ? ESCAPE '\' COLLATE NOCASE)`+extra+`
		`+orderByClause(customerSortColumns, sort, dir), append([]any{query, like, like, like}, extraArgs...)...,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list customers: %w", err)
	}
	defer rows.Close()

	var customers []Customer
	for rows.Next() {
		c, err := scanCustomer(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list customers: %w", err)
		}
		customers = append(customers, *c)
	}
	return customers, rows.Err()
}

// CustomerByID returns the non-deleted customer with the given id, or nil if none
// exists.
func (s *Store) CustomerByID(ctx context.Context, id int64) (*Customer, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+customerSelectCols+`
		FROM customers WHERE id = ? AND deleted_at IS NULL`, id,
	)
	c, err := scanCustomer(row)
	if err != nil {
		return nil, fmt.Errorf("store: customer by id %d: %w", id, err)
	}
	return c, nil
}

// CreateCustomer inserts a new customer, returning its id.
func (s *Store) CreateCustomer(ctx context.Context, c Customer) (int64, error) {
	res, err := s.exec(ctx, `
		INSERT INTO customers (name, rfc, contact_name, phone, email, address, postal_code, tax_regime, notes, legal_name, cfdi_use)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Name, nullIfEmpty(c.RFC), nullIfEmpty(c.ContactName), nullIfEmpty(c.Phone),
		nullIfEmpty(c.Email), nullIfEmpty(c.Address), nullIfEmpty(c.PostalCode),
		nullIfEmpty(c.TaxRegime), nullIfEmpty(c.Notes), nullIfEmpty(c.LegalName), nullIfEmpty(c.CFDIUse),
	)
	if err != nil {
		return 0, fmt.Errorf("store: create customer %q: %w", c.Name, err)
	}
	return res.LastInsertId()
}

// UpdateCustomer overwrites an existing customer's editable fields, identified by c.ID.
// Saving without changing anything writes nothing, so the customer's Historial (its
// audit trail) gets no empty entry (issue #41).
func (s *Store) UpdateCustomer(ctx context.Context, c Customer) error {
	_, err := s.exec(ctx, `
		UPDATE customers SET
			name         = ?1,
			rfc          = ?2,
			contact_name = ?3,
			phone        = ?4,
			email        = ?5,
			address      = ?6,
			postal_code  = ?7,
			tax_regime   = ?8,
			notes        = ?9,
			legal_name   = ?10,
			cfdi_use     = ?11,
			updated_at   = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?12
		  AND (name IS NOT ?1 OR rfc IS NOT ?2 OR contact_name IS NOT ?3 OR phone IS NOT ?4
		       OR email IS NOT ?5 OR address IS NOT ?6 OR postal_code IS NOT ?7
		       OR tax_regime IS NOT ?8 OR notes IS NOT ?9 OR legal_name IS NOT ?10
		       OR cfdi_use IS NOT ?11)`,
		c.Name, nullIfEmpty(c.RFC), nullIfEmpty(c.ContactName), nullIfEmpty(c.Phone),
		nullIfEmpty(c.Email), nullIfEmpty(c.Address), nullIfEmpty(c.PostalCode),
		nullIfEmpty(c.TaxRegime), nullIfEmpty(c.Notes), nullIfEmpty(c.LegalName), nullIfEmpty(c.CFDIUse), c.ID,
	)
	if err != nil {
		return fmt.Errorf("store: update customer %d: %w", c.ID, err)
	}
	return nil
}

// CustomerCreatedAt is when the customer row was created (ISO-8601 UTC), for a
// Historial whose audit trail has no creation entry: customers from before the trail.
func (s *Store) CustomerCreatedAt(ctx context.Context, id int64) (string, error) {
	var at string
	err := s.db.QueryRowContext(ctx, `SELECT created_at FROM customers WHERE id = ?`, id).Scan(&at)
	if err != nil {
		return "", fmt.Errorf("store: customer %d created_at: %w", id, err)
	}
	return at, nil
}

// SoftDeleteCustomer sets deleted_at, hiding the customer from
// ListCustomers/CustomerByID. Customers are never hard-deleted — old quotes may still
// reference them.
func (s *Store) SoftDeleteCustomer(ctx context.Context, id int64) error {
	_, err := s.exec(ctx, `
		UPDATE customers SET deleted_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?`, id,
	)
	if err != nil {
		return fmt.Errorf("store: soft-delete customer %d: %w", id, err)
	}
	return nil
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
