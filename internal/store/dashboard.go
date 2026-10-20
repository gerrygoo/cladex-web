package store

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gerrygoo/cladex-web/internal/money"
)

// DashboardSellers are the people the home page's Vendedores table always lists, even
// with nothing to show, matched by username. Emilio also sells, though he is an admin;
// the development account (ggo) is deliberately not here. Name is shown when the user
// doesn't exist yet.
var DashboardSellers = []struct{ Username, Name string }{
	{"efs", "Emilio Flores"},
	{"rfm", "Rodolfo Flores"},
	{"ggr", "Gerardo Galván Rosillo"},
}

// SellerRow is one salesperson's line on the home page. Only active proyectos count: the
// ones still being followed up (prospecto). Once the purchase order is in ("Orden
// colocada 100%") or it is lost, a proyecto is no longer part of the pipeline.
type SellerRow struct {
	Name string
	// Projects are all their active proyectos (issue #36); len(Projects) is the count.
	Projects []DashboardProject
	Pipeline money.Centavos // their subtotals (before IVA) added up
	Profit   money.Centavos // their profits added up; Profit / Pipeline is the pipeline margin
	// Forecast is the active proyectos at Alta (75%) or more: how many, and their subtotal.
	ForecastCount  int
	ForecastAmount money.Centavos
	// Untracked are the active proyectos without follow-up: no next follow-up day, or one
	// that is today or past. UntrackedAmount adds up their subtotals.
	Untracked       []DashboardProject
	UntrackedAmount money.Centavos
	// OverdueOC are the active proyectos whose expected purchase order day has passed and
	// whose order hasn't arrived.
	OverdueOC []DashboardProject
}

// DashboardProject is one proyecto in a SellerRow list. The lists run from the biggest
// Amount (subtotal, before IVA) down.
type DashboardProject struct {
	Folio, Customer string
	Amount          money.Centavos
}

// Dashboard is the home page's Vendedores table.
type Dashboard struct {
	Sellers []SellerRow // ranked by UntrackedAmount, the biggest first
}

// SalesDashboard builds the home page's table. Every amount is before IVA (issue #24),
// as in the Pronóstico: a proyecto's amount is its current quote's subtotal and its profit the same figure the Pronóstico shows (see quoteProfitSQL).
// today (YYYY-MM-DD, in the viewer's zone) decides which follow-ups and purchase orders
// are overdue.
func (s *Store) SalesDashboard(ctx context.Context, today string) (*Dashboard, error) {
	usernames := make([]any, len(DashboardSellers))
	marks := make([]string, len(DashboardSellers))
	for i, sl := range DashboardSellers {
		usernames[i], marks[i] = sl.Username, "?"
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.username, u.name, p.folio, c.name, q.subtotal, `+quoteProfitSQL+`, p.probability,
			COALESCE(p.next_followup_date, ''), COALESCE(p.expected_oc_date, '')
		FROM projects p
		JOIN quotes q ON q.project_id = p.id AND q.status != 'revisada'
		JOIN customers c ON c.id = p.customer_id
		JOIN users u ON u.id = p.user_id
		WHERE p.status = 'prospecto' AND u.username IN (`+strings.Join(marks, ", ")+`)
		ORDER BY p.folio`, usernames...)
	if err != nil {
		return nil, fmt.Errorf("store: sales dashboard: %w", err)
	}
	defer rows.Close()

	byUser := map[string]*SellerRow{}
	d := &Dashboard{Sellers: make([]SellerRow, len(DashboardSellers))}
	for i, sl := range DashboardSellers {
		d.Sellers[i].Name = sl.Name
		byUser[sl.Username] = &d.Sellers[i]
	}
	seenName := map[string]bool{}
	for rows.Next() {
		var username, name, folio, customer, followUp, expectedOC string
		var total, profit money.Centavos
		var probability int
		if err := rows.Scan(&username, &name, &folio, &customer, &total, &profit, &probability, &followUp, &expectedOC); err != nil {
			return nil, fmt.Errorf("store: sales dashboard: %w", err)
		}
		row := byUser[username]
		if !seenName[username] {
			row.Name, seenName[username] = name, true // the user's own name wins over the fallback
		}
		dp := DashboardProject{Folio: folio, Customer: customer, Amount: total}
		row.Projects = append(row.Projects, dp)
		row.Pipeline += total
		row.Profit += profit
		if probability >= ForecastThreshold {
			row.ForecastCount++
			row.ForecastAmount += total
		}
		if followUp == "" || followUp <= today {
			row.Untracked = append(row.Untracked, dp)
			row.UntrackedAmount += total
		}
		if expectedOC != "" && expectedOC < today {
			row.OverdueOC = append(row.OverdueOC, dp)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: sales dashboard: %w", err)
	}
	biggestFirst := func(ps []DashboardProject) {
		sort.SliceStable(ps, func(i, j int) bool { return ps[i].Amount > ps[j].Amount })
	}
	for i := range d.Sellers {
		biggestFirst(d.Sellers[i].Projects)
		biggestFirst(d.Sellers[i].Untracked)
		biggestFirst(d.Sellers[i].OverdueOC)
	}
	sort.SliceStable(d.Sellers, func(i, j int) bool { return d.Sellers[i].UntrackedAmount > d.Sellers[j].UntrackedAmount })
	return d, nil
}
