package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
)

// QuoteType is a quote_types row (migrations/0029_quote_types.sql): a kind of quote,
// with how it is numbered (Prefix, NextNumber), printed (Terms, Letterhead,
// HideAddress) and built (the other flags). FamilyID is the familia whose products it
// quotes, nil for none; FamilyName is that familia's name, "" without one.
type QuoteType struct {
	ID         int64
	Prefix     string
	Name       string
	Terms      string // one term per line
	NextNumber int64
	FamilyID   *int64
	FamilyName string
	QuoteTypeBehavior
}

// QuoteTypeBehavior is what a quote type does differently from a plain one.
// Letterhead is one of Letterheads; "" is the plain Cladex one.
type QuoteTypeBehavior struct {
	FreeLinesOnly     bool
	HideAddress       bool
	SortLast          bool
	KgPriced          bool
	CopperPriceMargin bool
	Letterhead        string
}

// Letterheads are quote_types.letterhead's values, in the order the type form offers
// them, with what each prints.
var Letterheads = []struct{ Value, Label string }{
	{"cladex", "Cladex"},
	{"cladex_large", "Cladex grande, sin dirección"},
	{"conduccasa", "ConduCCAsa"},
	{"copperclad", "Copperclad"},
	{"alumoclad", "Alumoclad"},
}

// ValidLetterhead reports whether v is one of Letterheads.
func ValidLetterhead(v string) bool {
	for _, l := range Letterheads {
		if l.Value == v {
			return true
		}
	}
	return false
}

// PickerLabel is the type as the new-quote form lists it: "QA — Cable CCA", or just
// "QA — <familia>" when the type has no name of its own.
func (t QuoteType) PickerLabel() string {
	label := t.Name
	if label == "" {
		label = t.FamilyName
	}
	return t.Prefix + " — " + label
}

// NextFolioText is the folio the type's next quote will get.
func (t QuoteType) NextFolioText() string {
	return FolioText(t.Prefix, t.NextNumber)
}

// PrefixPattern is what a prefix looks like: Q and one to five more capital letters
// (QA, QCCA, QCCSA). Decided 2026-10-28 (issue #53).
var PrefixPattern = regexp.MustCompile(`^Q[A-Z]{1,5}$`)

var (
	// ErrBadPrefix means the prefix doesn't match PrefixPattern.
	ErrBadPrefix = errors.New("store: a prefix is Q and one to five capital letters")
	// ErrDuplicateSeries means another quote type has the prefix now, or had it when it
	// issued a folio: reusing it would hand out folios that exist.
	ErrDuplicateSeries = errors.New("store: another quote type has or had this prefix")
	// ErrBadLetterhead means the letterhead isn't one of Letterheads.
	ErrBadLetterhead = errors.New("store: unknown letterhead")
)

const quoteTypeCols = `t.id, t.prefix, t.name, t.terms, t.next_number, t.family_id, COALESCE(f.name, ''),
	t.free_lines_only, t.hide_address, t.sort_last, t.kg_priced, t.copper_price_margin, t.letterhead`

const quoteTypeFrom = ` FROM quote_types t LEFT JOIN product_families f ON f.id = t.family_id`

func scanQuoteType(row interface{ Scan(...any) error }) (QuoteType, error) {
	var t QuoteType
	var familyID sql.NullInt64
	err := row.Scan(&t.ID, &t.Prefix, &t.Name, &t.Terms, &t.NextNumber, &familyID, &t.FamilyName,
		&t.FreeLinesOnly, &t.HideAddress, &t.SortLast, &t.KgPriced, &t.CopperPriceMargin, &t.Letterhead)
	if familyID.Valid {
		t.FamilyID = &familyID.Int64
	}
	return t, err
}

// ListQuoteTypes returns every quote type in creation order, which is also the order
// of the new-quote form.
func (s *Store) ListQuoteTypes(ctx context.Context) ([]QuoteType, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+quoteTypeCols+quoteTypeFrom+` ORDER BY t.id`)
	if err != nil {
		return nil, fmt.Errorf("store: list quote types: %w", err)
	}
	defer rows.Close()
	var types []QuoteType
	for rows.Next() {
		t, err := scanQuoteType(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list quote types: %w", err)
		}
		types = append(types, t)
	}
	return types, rows.Err()
}

// QuoteTypeByID returns the quote type with id, or nil if there is none.
func (s *Store) QuoteTypeByID(ctx context.Context, id int64) (*QuoteType, error) {
	t, err := scanQuoteType(s.db.QueryRowContext(ctx, `SELECT `+quoteTypeCols+quoteTypeFrom+` WHERE t.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: quote type %d: %w", id, err)
	}
	return &t, nil
}

// ErrUnknownQuoteType means no quote type has the prefix a new quote asked for.
var ErrUnknownQuoteType = errors.New("store: no quote type has that prefix")

// QuoteTypeIDByPrefix returns the id of the quote type whose current prefix is prefix,
// or ErrUnknownQuoteType.
func (s *Store) QuoteTypeIDByPrefix(ctx context.Context, prefix string) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM quote_types WHERE prefix = ?`, prefix).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrUnknownQuoteType
	}
	if err != nil {
		return 0, fmt.Errorf("store: quote type %q: %w", prefix, err)
	}
	return id, nil
}

// checkQuoteType validates what CreateQuoteType and UpdateQuoteType write. self is the
// type being updated, 0 for a new one: a type may go back to a prefix it had itself,
// since its counter only moves forward, but never take one another type has or issued.
func (s *Store) checkQuoteType(ctx context.Context, t QuoteType, self int64) error {
	if !PrefixPattern.MatchString(t.Prefix) {
		return ErrBadPrefix
	}
	if t.Letterhead != "" && !ValidLetterhead(t.Letterhead) {
		return ErrBadLetterhead
	}
	var taken int
	if err := s.db.QueryRowContext(ctx, `
		SELECT (SELECT count(*) FROM quote_types WHERE prefix = ?1 AND id != ?2)
			+ (SELECT count(*) FROM quotes WHERE prefix = ?1 AND quote_type_id != ?2)`,
		t.Prefix, self).Scan(&taken); err != nil {
		return fmt.Errorf("store: check prefix %q: %w", t.Prefix, err)
	}
	if taken > 0 {
		return ErrDuplicateSeries
	}
	return nil
}

// CreateQuoteType inserts a quote type, numbering from 1, and returns its id. It
// returns ErrBadPrefix, ErrDuplicateSeries or ErrBadLetterhead without writing.
func (s *Store) CreateQuoteType(ctx context.Context, t QuoteType) (int64, error) {
	if err := s.checkQuoteType(ctx, t, 0); err != nil {
		return 0, err
	}
	if t.Letterhead == "" {
		t.Letterhead = "cladex"
	}
	res, err := s.exec(ctx, `
		INSERT INTO quote_types (prefix, name, terms, family_id, free_lines_only, hide_address,
			sort_last, kg_priced, copper_price_margin, letterhead)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.Prefix, t.Name, t.Terms, t.FamilyID, t.FreeLinesOnly, t.HideAddress,
		t.SortLast, t.KgPriced, t.CopperPriceMargin, t.Letterhead)
	if err != nil {
		return 0, fmt.Errorf("store: create quote type %q: %w", t.Prefix, err)
	}
	return res.LastInsertId()
}

// UpdateQuoteType replaces a quote type's prefix, name, terms, familia and behavior.
// FreeLinesOnly can't change once the type exists (its drafts are built one way or the
// other) and NextNumber never goes back, so neither is written. A new prefix only
// reaches folios handed out from now on; drafts follow the new terms and behavior,
// issued quotes keep what they were issued with. It returns ErrBadPrefix,
// ErrDuplicateSeries or ErrBadLetterhead without writing, and sql.ErrNoRows when there
// is no such type.
func (s *Store) UpdateQuoteType(ctx context.Context, t QuoteType) error {
	if err := s.checkQuoteType(ctx, t, t.ID); err != nil {
		return err
	}
	if t.Letterhead == "" {
		t.Letterhead = "cladex"
	}
	res, err := s.exec(ctx, `
		UPDATE quote_types SET prefix = ?, name = ?, terms = ?, family_id = ?, hide_address = ?,
			sort_last = ?, kg_priced = ?, copper_price_margin = ?, letterhead = ?
		WHERE id = ?`,
		t.Prefix, t.Name, t.Terms, t.FamilyID, t.HideAddress,
		t.SortLast, t.KgPriced, t.CopperPriceMargin, t.Letterhead, t.ID)
	if err != nil {
		return fmt.Errorf("store: update quote type %d: %w", t.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SetQuoteTypeBehavior replaces a quote type's behavior flags, FreeLinesOnly included.
func (s *Store) SetQuoteTypeBehavior(ctx context.Context, quoteTypeID int64, b QuoteTypeBehavior) error {
	if b.Letterhead == "" {
		b.Letterhead = "cladex"
	}
	if !ValidLetterhead(b.Letterhead) {
		return ErrBadLetterhead
	}
	res, err := s.exec(ctx, `
		UPDATE quote_types SET free_lines_only = ?, hide_address = ?, sort_last = ?, kg_priced = ?,
			copper_price_margin = ?, letterhead = ?
		WHERE id = ?`,
		b.FreeLinesOnly, b.HideAddress, b.SortLast, b.KgPriced, b.CopperPriceMargin, b.Letterhead, quoteTypeID)
	if err != nil {
		return fmt.Errorf("store: quote type %d behavior: %w", quoteTypeID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
