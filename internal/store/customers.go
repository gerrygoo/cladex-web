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
	Notes       string
}

const customerSelectCols = `id, name, rfc, contact_name, phone, email, address, notes`

func scanCustomer(row interface{ Scan(...any) error }) (*Customer, error) {
	var c Customer
	var rfc, contactName, phone, email, address, notes sql.NullString
	err := row.Scan(&c.ID, &c.Name, &rfc, &contactName, &phone, &email, &address, &notes)
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
	c.Notes = notes.String
	return &c, nil
}

// customerSortColumns is the sortable-column whitelist for ListCustomers; the first
// entry (name) is the default when sort doesn't match a known column.
var customerSortColumns = []sortColumn{
	{"name", "name"},
	{"rfc", "rfc"},
	{"contact_name", "contact_name"},
	{"phone", "phone"},
	{"email", "email"},
}

// ListCustomers returns non-deleted customers, optionally filtered by a
// case-insensitive substring match on name, RFC, or contact name, and sorted per
// sort/dir (see customerSortColumns; dir is "asc" or "desc").
func (s *Store) ListCustomers(ctx context.Context, query, sort, dir string) ([]Customer, error) {
	like := "%" + escapeLike(query) + "%"
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+customerSelectCols+`
		FROM customers
		WHERE deleted_at IS NULL
		  AND (? = '' OR name LIKE ? ESCAPE '\' COLLATE NOCASE
		           OR rfc LIKE ? ESCAPE '\' COLLATE NOCASE
		           OR contact_name LIKE ? ESCAPE '\' COLLATE NOCASE)
		`+orderByClause(customerSortColumns, sort, dir), query, like, like, like,
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
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO customers (name, rfc, contact_name, phone, email, address, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.Name, nullIfEmpty(c.RFC), nullIfEmpty(c.ContactName), nullIfEmpty(c.Phone),
		nullIfEmpty(c.Email), nullIfEmpty(c.Address), nullIfEmpty(c.Notes),
	)
	if err != nil {
		return 0, fmt.Errorf("store: create customer %q: %w", c.Name, err)
	}
	return res.LastInsertId()
}

// UpdateCustomer overwrites an existing customer's editable fields, identified by c.ID.
func (s *Store) UpdateCustomer(ctx context.Context, c Customer) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE customers SET
			name         = ?,
			rfc          = ?,
			contact_name = ?,
			phone        = ?,
			email        = ?,
			address      = ?,
			notes        = ?,
			updated_at   = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?`,
		c.Name, nullIfEmpty(c.RFC), nullIfEmpty(c.ContactName), nullIfEmpty(c.Phone),
		nullIfEmpty(c.Email), nullIfEmpty(c.Address), nullIfEmpty(c.Notes), c.ID,
	)
	if err != nil {
		return fmt.Errorf("store: update customer %d: %w", c.ID, err)
	}
	return nil
}

// SoftDeleteCustomer sets deleted_at, hiding the customer from
// ListCustomers/CustomerByID. Customers are never hard-deleted — old quotes may still
// reference them.
func (s *Store) SoftDeleteCustomer(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `
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
