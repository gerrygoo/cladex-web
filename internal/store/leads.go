package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
)

// Lead statuses (migrations/0048_leads.sql).
const (
	LeadAbierto    = "abierto"
	LeadPerdido    = "perdido"
	LeadConvertido = "convertido"
)

// ErrLeadState is returned for a change a lead can't take in its status: following up,
// losing or converting a lead that is no longer abierto.
var ErrLeadState = errors.New("store: lead is not open")

// LeadInput is a lead's own data, as the new and edit forms post it.
type LeadInput struct {
	ContactName  string
	CustomerID   *int64 // an existing customer the contact works for
	Company      string // the company, when it isn't a customer yet
	Phone        string
	Email        string
	UserID       int64  // the vendedor in charge
	NextFollowUp string // YYYY-MM-DD, "" for none
}

// Errors returns the form's field errors, by field name, or nil when it's valid.
func (in LeadInput) Errors() map[string]string {
	errs := map[string]string{}
	if strings.TrimSpace(in.ContactName) == "" {
		errs["contact_name"] = "Escribe el nombre del contacto."
	}
	if in.Phone == "" && in.Email == "" {
		errs["phone"] = "Escribe un teléfono o un correo para poder darle seguimiento."
	}
	if in.Email != "" {
		if a, err := mail.ParseAddress(in.Email); err != nil || a.Address != in.Email {
			errs["email"] = "Escribe un correo válido, p. ej. juan@empresa.com."
		}
	}
	if in.UserID == 0 {
		errs["user_id"] = "Elige el vendedor responsable."
	}
	if in.NextFollowUp != "" {
		if _, err := time.Parse("2006-01-02", in.NextFollowUp); err != nil {
			errs["next_followup"] = "Escribe la fecha como día/mes/año."
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

// Lead is a leads row with its customer's, vendedor's and quote's names and the last
// entry of its history.
type Lead struct {
	ID           int64
	ContactName  string
	CustomerID   *int64
	CustomerName string
	Company      string
	Phone        string
	Email        string
	UserID       int64
	UserName     string
	NextFollowUp string // YYYY-MM-DD, "" for none
	Status       string
	LostReason   string
	QuoteID      *int64
	QuoteFolio   string
	// ProjectFolio is the proyecto the converted lead's quote became, once issued.
	ProjectFolio string
	CreatedAt    string
	ClosedAt     string
	LastAt       string
	LastBody     string
	LastUserName string
}

// CompanyName is who the contact works for: the customer, else the company typed in.
func (l Lead) CompanyName() string {
	if l.CustomerName != "" {
		return l.CustomerName
	}
	return l.Company
}

// Label names the lead in lists and avisos: "Juan Pérez · Sonepar".
func (l Lead) Label() string {
	if c := l.CompanyName(); c != "" {
		return l.ContactName + " · " + c
	}
	return l.ContactName
}

// Open reports whether the lead is still abierto.
func (l Lead) Open() bool { return l.Status == LeadAbierto }

// FollowUpDue reports whether an open lead's seguimiento day is today or past.
func (l Lead) FollowUpDue(today string) bool {
	return l.Open() && l.NextFollowUp != "" && l.NextFollowUp <= today
}

const leadSelect = `
	SELECT l.id, l.contact_name, l.customer_id, COALESCE(c.name, ''), l.company, l.phone,
		l.email, l.user_id, u.name, COALESCE(l.next_followup_date, ''), l.status,
		COALESCE(l.lost_reason, ''), l.quote_id, COALESCE(q.folio, ''),
		COALESCE((SELECT p.folio FROM projects p WHERE p.id = q.project_id), ''),
		l.created_at, COALESCE(l.closed_at, ''),
		COALESCE((SELECT lc.created_at FROM lead_comments lc WHERE lc.lead_id = l.id ORDER BY lc.created_at DESC, lc.id DESC LIMIT 1), ''),
		COALESCE((SELECT lc.body FROM lead_comments lc WHERE lc.lead_id = l.id ORDER BY lc.created_at DESC, lc.id DESC LIMIT 1), ''),
		COALESCE((SELECT lu.name FROM lead_comments lc JOIN users lu ON lu.id = lc.user_id WHERE lc.lead_id = l.id ORDER BY lc.created_at DESC, lc.id DESC LIMIT 1), '')
	FROM leads l
	JOIN users u ON u.id = l.user_id
	LEFT JOIN customers c ON c.id = l.customer_id
	LEFT JOIN quotes q ON q.id = l.quote_id`

func scanLead(row interface{ Scan(...any) error }) (*Lead, error) {
	var l Lead
	var customerID, quoteID sql.NullInt64
	if err := row.Scan(&l.ID, &l.ContactName, &customerID, &l.CustomerName, &l.Company, &l.Phone,
		&l.Email, &l.UserID, &l.UserName, &l.NextFollowUp, &l.Status,
		&l.LostReason, &quoteID, &l.QuoteFolio, &l.ProjectFolio,
		&l.CreatedAt, &l.ClosedAt, &l.LastAt, &l.LastBody, &l.LastUserName); err != nil {
		return nil, err
	}
	if customerID.Valid {
		l.CustomerID = &customerID.Int64
	}
	if quoteID.Valid {
		l.QuoteID = &quoteID.Int64
	}
	return &l, nil
}

func (s *Store) queryLeads(ctx context.Context, where string, args ...any) ([]Lead, error) {
	rows, err := s.db.QueryContext(ctx, leadSelect+` WHERE `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list leads: %w", err)
	}
	defer rows.Close()
	var out []Lead
	for rows.Next() {
		l, err := scanLead(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list leads: %w", err)
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}

// LeadByID returns the lead, or nil if there's none.
func (s *Store) LeadByID(ctx context.Context, id int64) (*Lead, error) {
	l, err := scanLead(s.db.QueryRowContext(ctx, leadSelect+` WHERE l.id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: lead %d: %w", id, err)
	}
	return l, nil
}

// ListLeads returns the leads of status ("" for every one) whose vendedor is userID (0
// for anyone's): open ones by seguimiento day, the soonest (most overdue) first and those
// without one last; closed ones newest first.
func (s *Store) ListLeads(ctx context.Context, status string, userID int64) ([]Lead, error) {
	return s.queryLeads(ctx, `(? = '' OR l.status = ?) AND (? = 0 OR l.user_id = ?)
		ORDER BY l.status != 'abierto', l.status = 'abierto' AND l.next_followup_date IS NULL,
			CASE WHEN l.status = 'abierto' THEN l.next_followup_date END,
			COALESCE(l.closed_at, l.created_at) DESC, l.id DESC`,
		status, status, userID, userID)
}

// LeadsFollowUpBefore returns the open leads whose seguimiento day is before day
// (YYYY-MM-DD): the ones the seguimiento avisos look at.
func (s *Store) LeadsFollowUpBefore(ctx context.Context, day string) ([]Lead, error) {
	return s.queryLeads(ctx, `l.status = 'abierto' AND l.next_followup_date < ? ORDER BY l.next_followup_date, l.id`, day)
}

// LeadOfProject returns the lead a proyecto came from (its quote chain starts at the
// quote the lead's conversion created), or nil.
func (s *Store) LeadOfProject(ctx context.Context, projectID int64) (*Lead, error) {
	l, err := scanLead(s.db.QueryRowContext(ctx, leadSelect+`
		WHERE l.quote_id IN (SELECT id FROM quotes WHERE project_id = ?) LIMIT 1`, projectID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: lead of project %d: %w", projectID, err)
	}
	return l, nil
}

// LeadComment is one entry of a lead's history.
type LeadComment struct {
	ID        int64
	UserName  string
	Body      string
	CreatedAt string
}

// ListLeadComments returns the lead's history, newest first.
func (s *Store) ListLeadComments(ctx context.Context, leadID int64) ([]LeadComment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, u.name, c.body, c.created_at FROM lead_comments c JOIN users u ON u.id = c.user_id
		WHERE c.lead_id = ? ORDER BY c.created_at DESC, c.id DESC`, leadID)
	if err != nil {
		return nil, fmt.Errorf("store: comments of lead %d: %w", leadID, err)
	}
	defer rows.Close()
	var out []LeadComment
	for rows.Next() {
		var c LeadComment
		if err := rows.Scan(&c.ID, &c.UserName, &c.Body, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: comments of lead %d: %w", leadID, err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func addLeadComment(ctx context.Context, tx *sql.Tx, leadID, userID int64, body string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO lead_comments (lead_id, user_id, body) VALUES (?, ?, ?)`, leadID, userID, body)
	return err
}

// CreateLead saves a new, open lead and, when note isn't empty, it as the first entry of
// its history. It returns the lead's id.
func (s *Store) CreateLead(ctx context.Context, in LeadInput, note string, createdBy int64) (int64, error) {
	var id int64
	err := s.inTx(ctx, "create lead", func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO leads (contact_name, customer_id, company, phone, email, user_id, next_followup_date, created_by)
			VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?)`,
			strings.TrimSpace(in.ContactName), in.CustomerID, companyOf(in), in.Phone, in.Email, in.UserID,
			in.NextFollowUp, nullIfZero(createdBy))
		if err != nil {
			return err
		}
		if id, err = res.LastInsertId(); err != nil {
			return err
		}
		if note = strings.TrimSpace(note); note != "" {
			return addLeadComment(ctx, tx, id, createdBy, note)
		}
		return nil
	})
	return id, err
}

// companyOf is the company to store: none when the lead names a customer.
func companyOf(in LeadInput) string {
	if in.CustomerID != nil {
		return ""
	}
	return strings.TrimSpace(in.Company)
}

// UpdateLead saves a lead's own data. Its seguimiento day changes through FollowUpLead,
// so the history says so; in.NextFollowUp is ignored here. A change of vendedor is
// written to the history.
func (s *Store) UpdateLead(ctx context.Context, id int64, in LeadInput, userID int64) error {
	return s.inTx(ctx, "update lead", func(tx *sql.Tx) error {
		var oldUser int64
		var oldName string
		err := tx.QueryRowContext(ctx, `SELECT l.user_id, u.name FROM leads l JOIN users u ON u.id = l.user_id WHERE l.id = ?`, id).Scan(&oldUser, &oldName)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE leads SET contact_name = ?, customer_id = ?, company = ?, phone = ?, email = ?, user_id = ?
			WHERE id = ?`,
			strings.TrimSpace(in.ContactName), in.CustomerID, companyOf(in), in.Phone, in.Email, in.UserID, id); err != nil {
			return err
		}
		if in.UserID != oldUser {
			var newName string
			if err := tx.QueryRowContext(ctx, `SELECT name FROM users WHERE id = ?`, in.UserID).Scan(&newName); err != nil {
				return err
			}
			return addLeadComment(ctx, tx, id, userID, fmt.Sprintf("Vendedor responsable: %s → %s.", oldName, newName))
		}
		return nil
	})
}

// openLead locks in that the lead is abierto inside tx, or returns ErrLeadState.
func openLead(ctx context.Context, tx *sql.Tx, id int64) (Lead, error) {
	var l Lead
	var customerID sql.NullInt64
	err := tx.QueryRowContext(ctx, `
		SELECT contact_name, customer_id, company, phone, email, user_id, COALESCE(next_followup_date, '')
		FROM leads WHERE id = ? AND status = 'abierto'`, id).Scan(
		&l.ContactName, &customerID, &l.Company, &l.Phone, &l.Email, &l.UserID, &l.NextFollowUp)
	if err == sql.ErrNoRows {
		return l, ErrLeadState
	}
	if customerID.Valid {
		l.CustomerID = &customerID.Int64
	}
	l.ID = id
	return l, err
}

// FollowUpLead records a follow-up on an open lead: the next seguimiento day (nil
// leaves it, "" clears it) and a note, as one entry of its history. Like a proyecto's,
// any entry stops that day's avisos.
func (s *Store) FollowUpLead(ctx context.Context, id, userID int64, next *string, note string) error {
	if next != nil && *next != "" {
		if _, err := time.Parse("2006-01-02", *next); err != nil {
			return ErrBadTransition
		}
	}
	note = strings.TrimSpace(note)
	return s.inTx(ctx, "follow up lead", func(tx *sql.Tx) error {
		l, err := openLead(ctx, tx, id)
		if err != nil {
			return err
		}
		var lines []string
		if next != nil && *next != l.NextFollowUp {
			if _, err := tx.ExecContext(ctx, `UPDATE leads SET next_followup_date = NULLIF(?, '') WHERE id = ?`, *next, id); err != nil {
				return err
			}
			lines = append(lines, fmt.Sprintf("Próximo seguimiento: %s.", dayText(*next)))
		}
		if note != "" {
			lines = append(lines, note)
		}
		if len(lines) == 0 {
			return nil
		}
		return addLeadComment(ctx, tx, id, userID, strings.Join(lines, "\n"))
	})
}

// LoseLead marks an open lead perdido, with why. Its seguimiento ends.
func (s *Store) LoseLead(ctx context.Context, id, userID int64, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ErrBadTransition
	}
	return s.inTx(ctx, "lose lead", func(tx *sql.Tx) error {
		if _, err := openLead(ctx, tx, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE leads SET status = 'perdido', lost_reason = ?, next_followup_date = NULL,
				closed_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
			WHERE id = ?`, reason, id); err != nil {
			return err
		}
		return addLeadComment(ctx, tx, id, userID, "Perdido: "+reason)
	})
}

// ReopenLead makes a perdido lead abierto again.
func (s *Store) ReopenLead(ctx context.Context, id, userID int64) error {
	return s.inTx(ctx, "reopen lead", func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE leads SET status = 'abierto', lost_reason = NULL, closed_at = NULL
			WHERE id = ? AND status = 'perdido'`, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return ErrLeadState
		}
		return addLeadComment(ctx, tx, id, userID, "Reabierto.")
	})
}

// ConvertLead turns an open lead into the start of a proyecto: its customer (created
// from the lead's company and contact when it has none) gets a new draft quote of type
// typeID, owned by the lead's vendedor, and the lead is closed as convertido pointing at
// it. The proyecto itself opens when that quote is issued, as any other.
func (s *Store) ConvertLead(ctx context.Context, id, userID, typeID int64) (*Quote, error) {
	l, err := s.LeadByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if l == nil || !l.Open() {
		return nil, ErrLeadState
	}
	customerID := int64(0)
	if l.CustomerID != nil {
		customerID = *l.CustomerID
	} else {
		name := l.Company
		if name == "" {
			name = l.ContactName
		}
		customerID, err = s.CreateCustomer(ctx, Customer{Name: name, ContactName: l.ContactName, Phone: l.Phone, Email: l.Email})
		if err != nil {
			return nil, fmt.Errorf("store: convert lead %d: %w", id, err)
		}
	}
	q, err := s.CreateDraftQuoteOfType(ctx, customerID, l.UserID, typeID)
	if err != nil {
		return nil, fmt.Errorf("store: convert lead %d: %w", id, err)
	}
	err = s.inTx(ctx, "convert lead", func(tx *sql.Tx) error {
		if _, err := openLead(ctx, tx, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE leads SET status = 'convertido', customer_id = ?, quote_id = ?, next_followup_date = NULL,
				closed_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
			WHERE id = ?`, customerID, q.ID, id); err != nil {
			return err
		}
		return addLeadComment(ctx, tx, id, userID, "Convertido en proyecto: cotización "+q.Folio+".")
	})
	if err != nil {
		return nil, err
	}
	return q, nil
}
