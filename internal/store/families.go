package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Family is a product_families row with the quote series it owns (migrations/
// 0013_familia_series.sql). Series is empty for a familia created without one, which
// then can't start quotes.
type Family struct {
	ID            int64
	Name          string
	Series        string
	SeriesLabel   string
	Terms         string // the series' terms block, one term per line
	FreeLinesOnly bool   // holds no catalog products: quoted with líneas libres only
}

// Series is a quote series a new quote can start in: the folio prefix, how the picker
// labels it, and the familia that owns it.
type Series struct {
	Prefix        string
	Label         string
	FamilyName    string
	FreeLinesOnly bool
}

// PickerLabel is the series as the new-quote form lists it: "QA — Cable CCA", or just
// "QA — <familia>" when the series has no label of its own.
func (s Series) PickerLabel() string {
	label := s.Label
	if label == "" {
		label = s.FamilyName
	}
	return s.Prefix + " — " + label
}

const familyCols = `id, name, COALESCE(series, ''), COALESCE(series_label, ''), terms, free_lines_only`

func scanFamily(row interface{ Scan(...any) error }) (Family, error) {
	var f Family
	err := row.Scan(&f.ID, &f.Name, &f.Series, &f.SeriesLabel, &f.Terms, &f.FreeLinesOnly)
	return f, err
}

// ListAllFamilies returns every familia, including free-lines-only ones, in creation
// order (which is also the order of their series in the new-quote form).
func (s *Store) ListAllFamilies(ctx context.Context) ([]Family, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+familyCols+` FROM product_families ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("store: list all families: %w", err)
	}
	defer rows.Close()

	var families []Family
	for rows.Next() {
		f, err := scanFamily(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list all families: %w", err)
		}
		families = append(families, f)
	}
	return families, rows.Err()
}

// ListSeries returns the quote series a new quote can start in, in creation order.
func (s *Store) ListSeries(ctx context.Context) ([]Series, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT series, COALESCE(series_label, ''), name, free_lines_only
		FROM product_families WHERE series IS NOT NULL ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("store: list series: %w", err)
	}
	defer rows.Close()

	var series []Series
	for rows.Next() {
		var sr Series
		if err := rows.Scan(&sr.Prefix, &sr.Label, &sr.FamilyName, &sr.FreeLinesOnly); err != nil {
			return nil, fmt.Errorf("store: list series: %w", err)
		}
		series = append(series, sr)
	}
	return series, rows.Err()
}

// SeriesExists reports whether prefix is the series of some familia.
func (s *Store) SeriesExists(ctx context.Context, prefix string) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM product_families WHERE series = ?`, prefix).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: series %q: %w", prefix, err)
	}
	return true, nil
}

// ErrDuplicateFamilyName and ErrDuplicateSeries are returned by CreateFamily when the
// name or the series prefix is already taken.
var (
	ErrDuplicateFamilyName = errors.New("store: a familia with this name already exists")
	ErrDuplicateSeries     = errors.New("store: a familia with this series already exists")
)

// CreateFamily inserts a familia together with its quote series and returns its id.
// Name and series are unique; terms is the series' terms block, one term per line.
func (s *Store) CreateFamily(ctx context.Context, f Family) (int64, error) {
	var taken int
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM product_families WHERE name = ? COLLATE NOCASE`, f.Name).Scan(&taken); err != nil {
		return 0, fmt.Errorf("store: check familia %q: %w", f.Name, err)
	}
	if taken > 0 {
		return 0, ErrDuplicateFamilyName
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM product_families WHERE series = ?`, f.Series).Scan(&taken); err != nil {
		return 0, fmt.Errorf("store: check series %q: %w", f.Series, err)
	}
	if taken > 0 {
		return 0, ErrDuplicateSeries
	}

	res, err := s.exec(ctx, `
		INSERT INTO product_families (name, series, series_label, terms, free_lines_only)
		VALUES (?, ?, ?, ?, ?)`,
		f.Name, f.Series, f.SeriesLabel, f.Terms, f.FreeLinesOnly)
	if err != nil {
		return 0, fmt.Errorf("store: create familia %q: %w", f.Name, err)
	}
	return res.LastInsertId()
}

// UpdateFamilyTerms replaces a familia's terms block and series label. It never
// touches the series prefix — folios already carry it — and never reaches issued
// quotes, which print the terms frozen at issue time.
func (s *Store) UpdateFamilyTerms(ctx context.Context, id int64, seriesLabel, terms string) error {
	res, err := s.exec(ctx,
		`UPDATE product_families SET series_label = ?, terms = ? WHERE id = ?`, seriesLabel, terms, id)
	if err != nil {
		return fmt.Errorf("store: update familia %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SplitTerms turns a terms block into its non-empty, trimmed lines.
func SplitTerms(terms string) []string {
	var out []string
	for _, line := range strings.Split(terms, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}
