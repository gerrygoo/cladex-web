package store

import (
	"context"
	"fmt"

	"github.com/gerrygoo/cladex-web/internal/money"
)

// QuoteStatuses is the quote lifecycle in order: a draft is issued, and an issued quote
// becomes revisada once a revision supersedes it.
var QuoteStatuses = []string{"borrador", "emitida", "revisada"}

// StageSummary is one lifecycle stage on the home overview: how many quotes sit in it,
// what they add up to, and the largest few by total.
type StageSummary struct {
	Status string
	Count  int
	Total  money.Centavos
	Top    []Quote // only Folio, CustomerName, UserName, Total, CreatedAt are set
}

// VendedorSummary is one salesperson's row on the home overview. A "vendedor" here is
// whoever created the quote (quotes.user_id), whatever their role: admins who quote
// count the same as vendedores.
type VendedorSummary struct {
	UserID        int64
	Name          string
	Borradores    int
	Emitidas      int
	Revisadas     int
	MontoEmitido  money.Centavos // sum of totals of quotes currently emitida
	MontoBorrador money.Centavos // sum of totals of open drafts
}

// Overview is the home page's snapshot of the quote pipeline. Every quote is MXN today
// (CreateDraftQuote never sets another currency), so totals are summed as-is.
type Overview struct {
	Stages     []StageSummary    // one per QuoteStatuses entry, in order
	Vendedores []VendedorSummary // ranked by MontoEmitido, then Emitidas, then name
}

// topPerStage is how many of each stage's largest quotes the overview lists.
const topPerStage = 3

// QuoteOverview summarizes quotes by lifecycle stage and by vendedor.
func (s *Store) QuoteOverview(ctx context.Context) (*Overview, error) {
	ov := &Overview{}
	byStatus := map[string]*StageSummary{}
	for _, st := range QuoteStatuses {
		ov.Stages = append(ov.Stages, StageSummary{Status: st})
	}
	for i := range ov.Stages {
		byStatus[ov.Stages[i].Status] = &ov.Stages[i]
	}

	rows, err := s.db.QueryContext(ctx, `SELECT status, count(*), coalesce(sum(total), 0) FROM quotes GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("store: overview stages: %w", err)
	}
	for rows.Next() {
		var status string
		var count int
		var total money.Centavos
		if err := rows.Scan(&status, &count, &total); err != nil {
			rows.Close()
			return nil, fmt.Errorf("store: overview stages: %w", err)
		}
		if st := byStatus[status]; st != nil {
			st.Count, st.Total = count, total
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: overview stages: %w", err)
	}

	rows, err = s.db.QueryContext(ctx, `
		SELECT status, folio, customer, vendedor, total, created_at FROM (
			SELECT q.status, q.folio, c.name AS customer, u.name AS vendedor, q.total, q.created_at,
				row_number() OVER (PARTITION BY q.status ORDER BY q.total DESC, q.created_at DESC) AS rn
			FROM quotes q
			JOIN customers c ON c.id = q.customer_id
			JOIN users u ON u.id = q.user_id
		) WHERE rn <= ? ORDER BY status, rn`, topPerStage)
	if err != nil {
		return nil, fmt.Errorf("store: overview top quotes: %w", err)
	}
	for rows.Next() {
		var status string
		var q Quote
		if err := rows.Scan(&status, &q.Folio, &q.CustomerName, &q.UserName, &q.Total, &q.CreatedAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("store: overview top quotes: %w", err)
		}
		q.Status = status
		if st := byStatus[status]; st != nil {
			st.Top = append(st.Top, q)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: overview top quotes: %w", err)
	}

	rows, err = s.db.QueryContext(ctx, `
		SELECT u.id, u.name,
			sum(q.status = 'borrador'), sum(q.status = 'emitida'), sum(q.status = 'revisada'),
			coalesce(sum(CASE WHEN q.status = 'emitida' THEN q.total END), 0),
			coalesce(sum(CASE WHEN q.status = 'borrador' THEN q.total END), 0)
		FROM quotes q
		JOIN users u ON u.id = q.user_id
		GROUP BY u.id
		ORDER BY 6 DESC, 4 DESC, u.name COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("store: overview vendedores: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var v VendedorSummary
		if err := rows.Scan(&v.UserID, &v.Name, &v.Borradores, &v.Emitidas, &v.Revisadas, &v.MontoEmitido, &v.MontoBorrador); err != nil {
			return nil, fmt.Errorf("store: overview vendedores: %w", err)
		}
		ov.Vendedores = append(ov.Vendedores, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: overview vendedores: %w", err)
	}
	return ov, nil
}
