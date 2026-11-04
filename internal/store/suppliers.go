package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Supplier is a suppliers row with its contacts and the familias it supplies (issue #64;
// see migrations/0033_suppliers.sql). Name (nombre comercial), RFC, LegalName and the
// fiscal address are required; everything else is optional, "" or nil when not set.
type Supplier struct {
	ID           int64
	Name         string
	RFC          string
	LegalName    string
	Street       string
	ExtNumber    string
	Neighborhood string
	Municipality string
	State        string
	PostalCode   string
	TaxRegime    string // c_RegimenFiscal code (see TaxRegimes)
	Notes        string
	CreditDays   *int64
	Currency     string // "MXN", "USD" or ""; informational, nothing is converted
	PaymentTerms string
	LeadTime     string
	Bank         string
	CLABE        string
	Beneficiary  string
	Inactive     bool

	Contacts []SupplierContact
	// FamilyIDs are the familias whose default supplier this is.
	FamilyIDs []int64
}

// SupplierContact is one of a supplier's contacts. ReceivesPO marks who an orden de
// compra goes to; it's informative only. ID is 0 for a contact not saved yet.
type SupplierContact struct {
	ID         int64
	Name       string
	Role       string
	Phone      string
	Email      string
	ReceivesPO bool
}

// SupplierRow is a supplier as the Proveedores list shows it: Families are the names of
// the familias it supplies, ", "-separated.
type SupplierRow struct {
	ID        int64
	Name      string
	RFC       string
	LegalName string
	Families  string
	Inactive  bool
}

// ValidCLABE reports whether clabe is 18 digits whose last one is the check digit: the
// first 17 weighted 3, 7, 1, 3, 7, 1…, each product taken mod 10, and the check digit
// (10 − sum mod 10) mod 10.
func ValidCLABE(clabe string) bool {
	if len(clabe) != 18 {
		return false
	}
	weights := [3]int{3, 7, 1}
	sum := 0
	for i := 0; i < 18; i++ {
		c := clabe[i]
		if c < '0' || c > '9' {
			return false
		}
		if i < 17 {
			sum += int(c-'0') * weights[i%3] % 10
		}
	}
	return int(clabe[17]-'0') == (10-sum%10)%10
}

var supplierSortColumns = []sortColumn{
	{"name", "s.name COLLATE NOCASE"},
	{"rfc", "s.rfc COLLATE NOCASE"},
	{"legal_name", "s.legal_name COLLATE NOCASE"},
	{"families", "families COLLATE NOCASE"},
	{"status", "s.inactive"},
}

var supplierFilterColumns = []filterColumn{
	{name: "name", expr: "s.name", kind: FilterText},
	{name: "rfc", expr: "s.rfc", kind: FilterText},
	{name: "legal_name", expr: "s.legal_name", kind: FilterText},
	{name: "families", expr: "families", kind: FilterText},
	{name: "status", expr: "CASE WHEN s.inactive = 0 THEN 'activo' ELSE 'inactivo' END", kind: FilterEnum},
}

// ListSuppliers returns the suppliers, inactive ones included, optionally filtered by a
// case-insensitive substring of the nombre comercial, RFC, razón social or a contact's
// name, and sorted per sort/dir (see supplierSortColumns).
func (s *Store) ListSuppliers(ctx context.Context, query, sort, dir string, filters Filters) ([]SupplierRow, error) {
	like := "%" + escapeLike(query) + "%"
	extra, extraArgs := filterWhere(ctx, supplierFilterColumns, filters)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, rfc, legal_name, families, inactive FROM (
			SELECT s.id, s.name, s.rfc, s.legal_name, s.inactive,
				COALESCE((SELECT group_concat(name, ', ') FROM
					(SELECT f.name FROM product_families f WHERE f.default_supplier_id = s.id ORDER BY f.id)), '') AS families,
				COALESCE((SELECT group_concat(c.name, ' ') FROM supplier_contacts c WHERE c.supplier_id = s.id), '') AS contacts
			FROM suppliers s
		) s
		WHERE (? = '' OR s.name LIKE ? ESCAPE '\' COLLATE NOCASE
		           OR s.rfc LIKE ? ESCAPE '\' COLLATE NOCASE
		           OR s.legal_name LIKE ? ESCAPE '\' COLLATE NOCASE
		           OR s.contacts LIKE ? ESCAPE '\' COLLATE NOCASE)`+extra+`
		`+orderByClause(supplierSortColumns, sort, dir)+`, s.name COLLATE NOCASE`,
		append([]any{query, like, like, like, like}, extraArgs...)...)
	if err != nil {
		return nil, fmt.Errorf("store: list suppliers: %w", err)
	}
	defer rows.Close()
	var out []SupplierRow
	for rows.Next() {
		var r SupplierRow
		if err := rows.Scan(&r.ID, &r.Name, &r.RFC, &r.LegalName, &r.Families, &r.Inactive); err != nil {
			return nil, fmt.Errorf("store: list suppliers: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

const supplierSelectCols = `id, name, rfc, legal_name, street, ext_number, neighborhood,
	municipality, state, postal_code, COALESCE(tax_regime, ''), COALESCE(notes, ''),
	credit_days, COALESCE(currency, ''), COALESCE(payment_terms, ''), COALESCE(lead_time, ''),
	COALESCE(bank, ''), COALESCE(clabe, ''), COALESCE(beneficiary, ''), inactive`

// SupplierByID returns the supplier with its contacts (in the order they were added) and
// familias, or nil if there is none.
func (s *Store) SupplierByID(ctx context.Context, id int64) (*Supplier, error) {
	var sup Supplier
	var credit sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT `+supplierSelectCols+` FROM suppliers WHERE id = ?`, id).Scan(
		&sup.ID, &sup.Name, &sup.RFC, &sup.LegalName, &sup.Street, &sup.ExtNumber, &sup.Neighborhood,
		&sup.Municipality, &sup.State, &sup.PostalCode, &sup.TaxRegime, &sup.Notes,
		&credit, &sup.Currency, &sup.PaymentTerms, &sup.LeadTime,
		&sup.Bank, &sup.CLABE, &sup.Beneficiary, &sup.Inactive)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: supplier %d: %w", id, err)
	}
	if credit.Valid {
		sup.CreditDays = &credit.Int64
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, COALESCE(role, ''), COALESCE(phone, ''), COALESCE(email, ''), receives_po
		FROM supplier_contacts WHERE supplier_id = ? ORDER BY id`, id)
	if err != nil {
		return nil, fmt.Errorf("store: supplier %d contacts: %w", id, err)
	}
	defer rows.Close()
	for rows.Next() {
		var c SupplierContact
		if err := rows.Scan(&c.ID, &c.Name, &c.Role, &c.Phone, &c.Email, &c.ReceivesPO); err != nil {
			return nil, fmt.Errorf("store: supplier %d contacts: %w", id, err)
		}
		sup.Contacts = append(sup.Contacts, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: supplier %d contacts: %w", id, err)
	}

	frows, err := s.db.QueryContext(ctx, `SELECT id FROM product_families WHERE default_supplier_id = ? ORDER BY id`, id)
	if err != nil {
		return nil, fmt.Errorf("store: supplier %d familias: %w", id, err)
	}
	defer frows.Close()
	for frows.Next() {
		var fid int64
		if err := frows.Scan(&fid); err != nil {
			return nil, fmt.Errorf("store: supplier %d familias: %w", id, err)
		}
		sup.FamilyIDs = append(sup.FamilyIDs, fid)
	}
	return &sup, frows.Err()
}

// supplierArgs are the supplier's own columns, in the order of the INSERT and UPDATE
// below (?1..?19).
func supplierArgs(sup Supplier) []any {
	var credit any
	if sup.CreditDays != nil {
		credit = *sup.CreditDays
	}
	return []any{sup.Name, sup.RFC, sup.LegalName, sup.Street, sup.ExtNumber, sup.Neighborhood,
		sup.Municipality, sup.State, sup.PostalCode, nullIfEmpty(sup.TaxRegime), nullIfEmpty(sup.Notes),
		credit, nullIfEmpty(sup.Currency), nullIfEmpty(sup.PaymentTerms), nullIfEmpty(sup.LeadTime),
		nullIfEmpty(sup.Bank), nullIfEmpty(sup.CLABE), nullIfEmpty(sup.Beneficiary)}
}

// CreateSupplier inserts sup with its contacts and makes it the default supplier of
// sup.FamilyIDs (taking them from any other supplier), in one transaction, and returns
// its id.
func (s *Store) CreateSupplier(ctx context.Context, sup Supplier) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("store: begin: %w", err)
	}
	defer tx.Rollback()
	if err := stampActor(ctx, tx); err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO suppliers (name, rfc, legal_name, street, ext_number, neighborhood,
			municipality, state, postal_code, tax_regime, notes, credit_days, currency,
			payment_terms, lead_time, bank, clabe, beneficiary)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, supplierArgs(sup)...)
	if err != nil {
		return 0, fmt.Errorf("store: create supplier %q: %w", sup.Name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := saveSupplierContacts(ctx, tx, id, sup.Contacts); err != nil {
		return 0, err
	}
	if err := setSupplierFamilies(ctx, tx, id, sup.FamilyIDs); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("store: commit: %w", err)
	}
	return id, nil
}

// UpdateSupplier overwrites supplier sup.ID's fields, contacts and familias in one
// transaction. Contacts with an ID are updated, those without are added, and saved
// contacts missing from sup.Contacts are removed. Familias in sup.FamilyIDs become its
// own (taken from any other supplier); familias of its own not in the list are left
// without a supplier. Nothing unchanged is written, so the Historial gets no empty
// entry. It returns sql.ErrNoRows when there is no such supplier.
func (s *Store) UpdateSupplier(ctx context.Context, sup Supplier) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin: %w", err)
	}
	defer tx.Rollback()
	if err := stampActor(ctx, tx); err != nil {
		return err
	}
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM suppliers WHERE id = ?`, sup.ID).Scan(&exists); err != nil {
		return fmt.Errorf("store: update supplier %d: %w", sup.ID, err)
	}
	if exists == 0 {
		return sql.ErrNoRows
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE suppliers SET
			name = ?1, rfc = ?2, legal_name = ?3, street = ?4, ext_number = ?5,
			neighborhood = ?6, municipality = ?7, state = ?8, postal_code = ?9,
			tax_regime = ?10, notes = ?11, credit_days = ?12, currency = ?13,
			payment_terms = ?14, lead_time = ?15, bank = ?16, clabe = ?17, beneficiary = ?18,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?19
		  AND (name IS NOT ?1 OR rfc IS NOT ?2 OR legal_name IS NOT ?3 OR street IS NOT ?4
		       OR ext_number IS NOT ?5 OR neighborhood IS NOT ?6 OR municipality IS NOT ?7
		       OR state IS NOT ?8 OR postal_code IS NOT ?9 OR tax_regime IS NOT ?10
		       OR notes IS NOT ?11 OR credit_days IS NOT ?12 OR currency IS NOT ?13
		       OR payment_terms IS NOT ?14 OR lead_time IS NOT ?15 OR bank IS NOT ?16
		       OR clabe IS NOT ?17 OR beneficiary IS NOT ?18)`,
		append(supplierArgs(sup), sup.ID)...); err != nil {
		return fmt.Errorf("store: update supplier %d: %w", sup.ID, err)
	}

	// Contacts: remove the saved ones the form no longer has, then update or add.
	keep := map[int64]bool{}
	for _, c := range sup.Contacts {
		if c.ID != 0 {
			keep[c.ID] = true
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM supplier_contacts WHERE supplier_id = ?`, sup.ID)
	if err != nil {
		return fmt.Errorf("store: supplier %d contacts: %w", sup.ID, err)
	}
	var drop []int64
	for rows.Next() {
		var cid int64
		if err := rows.Scan(&cid); err != nil {
			rows.Close()
			return fmt.Errorf("store: supplier %d contacts: %w", sup.ID, err)
		}
		if !keep[cid] {
			drop = append(drop, cid)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("store: supplier %d contacts: %w", sup.ID, err)
	}
	for _, cid := range drop {
		if _, err := tx.ExecContext(ctx, `DELETE FROM supplier_contacts WHERE id = ?`, cid); err != nil {
			return fmt.Errorf("store: delete contact %d: %w", cid, err)
		}
	}
	if err := saveSupplierContacts(ctx, tx, sup.ID, sup.Contacts); err != nil {
		return err
	}
	if err := setSupplierFamilies(ctx, tx, sup.ID, sup.FamilyIDs); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit: %w", err)
	}
	return nil
}

// saveSupplierContacts adds the contacts without an ID to supplier id and updates those
// with one (only the ones that belong to it, and only if something changed).
func saveSupplierContacts(ctx context.Context, tx *sql.Tx, id int64, contacts []SupplierContact) error {
	for _, c := range contacts {
		args := []any{c.Name, nullIfEmpty(c.Role), nullIfEmpty(c.Phone), nullIfEmpty(c.Email), c.ReceivesPO, id}
		if c.ID == 0 {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO supplier_contacts (name, role, phone, email, receives_po, supplier_id)
				VALUES (?, ?, ?, ?, ?, ?)`, args...); err != nil {
				return fmt.Errorf("store: add contact %q: %w", c.Name, err)
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE supplier_contacts SET name = ?1, role = ?2, phone = ?3, email = ?4, receives_po = ?5
			WHERE supplier_id = ?6 AND id = ?7
			  AND (name IS NOT ?1 OR role IS NOT ?2 OR phone IS NOT ?3 OR email IS NOT ?4
			       OR receives_po IS NOT ?5)`, append(args, c.ID)...); err != nil {
			return fmt.Errorf("store: update contact %d: %w", c.ID, err)
		}
	}
	return nil
}

// setSupplierFamilies makes supplier id the default of exactly familyIDs: those become
// its own (free-lines-only familias never do), and its others are left without one.
func setSupplierFamilies(ctx context.Context, tx *sql.Tx, id int64, familyIDs []int64) error {
	want := map[int64]bool{}
	for _, fid := range familyIDs {
		want[fid] = true
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM product_families WHERE default_supplier_id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: supplier %d familias: %w", id, err)
	}
	var drop []int64
	for rows.Next() {
		var fid int64
		if err := rows.Scan(&fid); err != nil {
			rows.Close()
			return fmt.Errorf("store: supplier %d familias: %w", id, err)
		}
		if !want[fid] {
			drop = append(drop, fid)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("store: supplier %d familias: %w", id, err)
	}
	for _, fid := range drop {
		if _, err := tx.ExecContext(ctx, `UPDATE product_families SET default_supplier_id = NULL WHERE id = ?`, fid); err != nil {
			return fmt.Errorf("store: unassign familia %d: %w", fid, err)
		}
	}
	for fid := range want {
		if _, err := tx.ExecContext(ctx, `
			UPDATE product_families SET default_supplier_id = ?1
			WHERE id = ?2 AND free_lines_only = 0 AND default_supplier_id IS NOT ?1`, id, fid); err != nil {
			return fmt.Errorf("store: assign familia %d: %w", fid, err)
		}
	}
	return nil
}

// SetSupplierInactive marks supplier id inactive, or active again. It keeps its
// familias. It returns sql.ErrNoRows when there is no such supplier.
func (s *Store) SetSupplierInactive(ctx context.Context, id int64, inactive bool) error {
	res, err := s.exec(ctx, `
		UPDATE suppliers SET inactive = ?1, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?2`, inactive, id)
	if err != nil {
		return fmt.Errorf("store: supplier %d inactive=%v: %w", id, inactive, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SupplierInUseError is DeleteSupplier's refusal: the supplier still supplies
// Familias (names), or has órdenes de compra (OrdenesCompra, their numbers; issue #65).
type SupplierInUseError struct {
	Familias      []string
	OrdenesCompra []string
}

func (e *SupplierInUseError) Error() string {
	return "store: supplier in use by familias " + strings.Join(e.Familias, ", ") +
		"; purchase orders " + strings.Join(e.OrdenesCompra, ", ")
}

// DeleteSupplier deletes supplier id and its contacts for good, unless it is the default
// supplier of some familia or has órdenes de compra (in any state): then it returns a
// *SupplierInUseError naming them. It returns sql.ErrNoRows when there is no such
// supplier.
func (s *Store) DeleteSupplier(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin: %w", err)
	}
	defer tx.Rollback()
	if err := stampActor(ctx, tx); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT name FROM product_families WHERE default_supplier_id = ? ORDER BY id`, id)
	if err != nil {
		return fmt.Errorf("store: supplier %d familias: %w", id, err)
	}
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return fmt.Errorf("store: supplier %d familias: %w", id, err)
		}
		names = append(names, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("store: supplier %d familias: %w", id, err)
	}
	orders, err := s.SupplierPurchaseOrders(ctx, id)
	if err != nil {
		return err
	}
	if len(names) > 0 || len(orders) > 0 {
		return &SupplierInUseError{Familias: names, OrdenesCompra: orders}
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM suppliers WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete supplier %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit: %w", err)
	}
	return nil
}

// SimilarSuppliers lists the suppliers a new one with this nombre comercial, razón
// social and RFC may duplicate, by the customers' criterion (SimilarCustomers): the same
// RFC, or a nombre comercial or razón social equal to or containing, normalized, the new
// one's nombre comercial or razón social. Inactive ones count. It only warns.
func (s *Store) SimilarSuppliers(ctx context.Context, name, legalName, rfc string) ([]SupplierRow, error) {
	all, err := s.ListSuppliers(ctx, "", "name", "asc", nil)
	if err != nil {
		return nil, fmt.Errorf("store: similar suppliers: %w", err)
	}
	wantRFC := normalizeRFC(rfc)
	wantNames := []string{normalizeName(name), normalizeName(legalName)}
	var out []SupplierRow
	for _, sup := range all {
		if wantRFC != "" && normalizeRFC(sup.RFC) == wantRFC {
			out = append(out, sup)
			continue
		}
	match:
		for _, have := range []string{normalizeName(sup.Name), normalizeName(sup.LegalName)} {
			for _, want := range wantNames {
				if similarNormalizedName(want, have) {
					out = append(out, sup)
					break match
				}
			}
		}
	}
	return out, nil
}

// SupplierFamilyOption is a familia offered in a supplier's "Familias que surte": every
// familia but the free-lines-only ones, with the quote types that quote it (so "CCA"
// reads "CCA — Cable CCA (QA)") and its current default supplier, if any.
type SupplierFamilyOption struct {
	ID           int64
	Name         string
	QuoteTypes   []FamilyQuoteType
	SupplierID   int64
	SupplierName string
}

// FamilyQuoteType is a quote type as a familia's label shows it.
type FamilyQuoteType struct {
	Name, Prefix string
}

// SupplierFamilyOptions lists the familias a supplier can supply, in creation order.
func (s *Store) SupplierFamilyOptions(ctx context.Context) ([]SupplierFamilyOption, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT f.id, f.name, COALESCE(sup.id, 0), COALESCE(sup.name, ''),
			COALESCE(t.name, ''), COALESCE(t.prefix, '')
		FROM product_families f
		LEFT JOIN suppliers sup ON sup.id = f.default_supplier_id
		LEFT JOIN quote_types t ON t.family_id = f.id
		WHERE f.free_lines_only = 0
		ORDER BY f.id, t.id`)
	if err != nil {
		return nil, fmt.Errorf("store: supplier familia options: %w", err)
	}
	defer rows.Close()
	var out []SupplierFamilyOption
	for rows.Next() {
		var o SupplierFamilyOption
		var t FamilyQuoteType
		if err := rows.Scan(&o.ID, &o.Name, &o.SupplierID, &o.SupplierName, &t.Name, &t.Prefix); err != nil {
			return nil, fmt.Errorf("store: supplier familia options: %w", err)
		}
		if n := len(out); n == 0 || out[n-1].ID != o.ID {
			out = append(out, o)
		}
		if t.Prefix != "" {
			last := &out[len(out)-1]
			last.QuoteTypes = append(last.QuoteTypes, t)
		}
	}
	return out, rows.Err()
}

// SupplierCreatedAt is when supplier id was created (ISO-8601 UTC).
func (s *Store) SupplierCreatedAt(ctx context.Context, id int64) (string, error) {
	var at string
	err := s.db.QueryRowContext(ctx, `SELECT created_at FROM suppliers WHERE id = ?`, id).Scan(&at)
	if err != nil {
		return "", fmt.Errorf("store: supplier %d created_at: %w", id, err)
	}
	return at, nil
}

// SupplierAuditLog is the trail behind supplier id's Historial, newest first: its own
// row, its contacts (by supplier_id), and the familias that became or stopped being
// its own (by default_supplier_id, before or after).
func (s *Store) SupplierAuditLog(ctx context.Context, id int64) ([]AuditEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+auditSelect+`
		FROM audit_log a
		LEFT JOIN users u ON u.id = a.actor_id
		WHERE (a.table_name = 'suppliers' AND a.row_key = ?1)
		   OR (a.table_name = 'supplier_contacts'
		       AND ?2 IN (json_extract(a.new_values, '$.supplier_id'), json_extract(a.old_values, '$.supplier_id')))
		   OR (a.table_name = 'product_families'
		       AND ?2 IN (json_extract(a.new_values, '$.default_supplier_id'), json_extract(a.old_values, '$.default_supplier_id')))
		ORDER BY a.id DESC
		LIMIT 1000`, strconv.FormatInt(id, 10), id)
	if err != nil {
		return nil, fmt.Errorf("store: supplier %d audit log: %w", id, err)
	}
	return scanAuditEntries(rows)
}
