package store

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gerrygoo/cladex-web/internal/money"
)

// StageSummary is one proyecto stage on the home overview: how many proyectos sit in it,
// what their current quotes add up to, and the largest few by total. Forecast is the
// part of a prospecto stage that is relevante para pronóstico (see ForecastRelevant);
// it is zero for every other stage.
type StageSummary struct {
	Status        string
	Count         int
	Total         money.Centavos
	ForecastCount int
	ForecastTotal money.Centavos
	Top           []Project // only Folio, CustomerName, UserName, Total, CreatedAt, Status, Probability are set
}

// VendedorSummary is one salesperson's row on the home overview. A "vendedor" here is
// whoever owns the proyecto (projects.user_id, the user who created the quote that
// opened it), whatever their role: admins who quote count the same as vendedores.
type VendedorSummary struct {
	UserID   int64
	Name     string
	Count    map[string]int            // proyectos per stage (ProjectFlow only)
	Total    map[string]money.Centavos // their sum, per stage
	Forecast money.Centavos            // the sum of their prospectos that are relevante para pronóstico
}

// Overview is the home page's snapshot of the proyectos. A proyecto's amount is its
// current quote's total, and amounts are summed as-is.
type Overview struct {
	Stages     []StageSummary    // one per ProjectFlow entry, in order
	Lost       StageSummary      // the perdido proyectos: count and total only, they are off the board
	Vendedores []VendedorSummary // ranked by forecast amount, then prospecto amount, then name
}

// topPerStage is how many of each stage's largest proyectos the overview lists.
const topPerStage = 3

// overviewFrom joins each proyecto to its current quote, the one that isn't revisada.
// Drafts that were never issued have no proyecto, so they are left out.
const overviewFrom = `
	FROM projects p
	JOIN quotes q ON q.project_id = p.id AND q.status != 'revisada'`

// ProjectOverview summarizes proyectos by stage and by vendedor.
func (s *Store) ProjectOverview(ctx context.Context) (*Overview, error) {
	ov := &Overview{}
	byStatus := map[string]*StageSummary{}
	for _, st := range ProjectFlow {
		ov.Stages = append(ov.Stages, StageSummary{Status: st})
	}
	for i := range ov.Stages {
		byStatus[ov.Stages[i].Status] = &ov.Stages[i]
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT p.status, count(*), coalesce(sum(q.total), 0),
			coalesce(sum(p.status = 'prospecto' AND p.probability >= ?), 0),
			coalesce(sum(CASE WHEN p.status = 'prospecto' AND p.probability >= ? THEN q.total END), 0)
		`+overviewFrom+` GROUP BY p.status`, ForecastThreshold, ForecastThreshold)
	if err != nil {
		return nil, fmt.Errorf("store: overview stages: %w", err)
	}
	for rows.Next() {
		var sum StageSummary
		if err := rows.Scan(&sum.Status, &sum.Count, &sum.Total, &sum.ForecastCount, &sum.ForecastTotal); err != nil {
			rows.Close()
			return nil, fmt.Errorf("store: overview stages: %w", err)
		}
		if st := byStatus[sum.Status]; st != nil {
			*st = sum
		} else if sum.Status == "perdido" {
			ov.Lost = sum
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: overview stages: %w", err)
	}

	rows, err = s.db.QueryContext(ctx, `
		SELECT status, folio, customer, vendedor, total, created_at, probability FROM (
			SELECT p.status, p.folio, c.name AS customer, u.name AS vendedor, q.total, p.created_at, p.probability,
				row_number() OVER (PARTITION BY p.status ORDER BY q.total DESC, p.created_at DESC) AS rn
			`+overviewFrom+`
			JOIN customers c ON c.id = p.customer_id
			JOIN users u ON u.id = p.user_id
		) WHERE rn <= ? ORDER BY status, rn`, topPerStage)
	if err != nil {
		return nil, fmt.Errorf("store: overview top projects: %w", err)
	}
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.Status, &p.Folio, &p.CustomerName, &p.UserName, &p.Total, &p.CreatedAt, &p.Probability); err != nil {
			rows.Close()
			return nil, fmt.Errorf("store: overview top projects: %w", err)
		}
		if st := byStatus[p.Status]; st != nil {
			st.Top = append(st.Top, p)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: overview top projects: %w", err)
	}

	rows, err = s.db.QueryContext(ctx, `
		SELECT u.id, u.name, p.status, count(*), coalesce(sum(q.total), 0),
			coalesce(sum(CASE WHEN p.status = 'prospecto' AND p.probability >= ? THEN q.total END), 0)
		`+overviewFrom+`
		JOIN users u ON u.id = p.user_id
		GROUP BY u.id, p.status
		ORDER BY u.id`, ForecastThreshold)
	if err != nil {
		return nil, fmt.Errorf("store: overview vendedores: %w", err)
	}
	defer rows.Close()
	byUser := map[int64]*VendedorSummary{}
	var order []int64
	for rows.Next() {
		var id int64
		var name, status string
		var count int
		var total, forecast money.Centavos
		if err := rows.Scan(&id, &name, &status, &count, &total, &forecast); err != nil {
			return nil, fmt.Errorf("store: overview vendedores: %w", err)
		}
		if byStatus[status] == nil {
			continue // lost proyectos are off the board
		}
		v := byUser[id]
		if v == nil {
			v = &VendedorSummary{UserID: id, Name: name, Count: map[string]int{}, Total: map[string]money.Centavos{}}
			byUser[id] = v
			order = append(order, id)
		}
		v.Count[status], v.Total[status] = count, total
		v.Forecast += forecast
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: overview vendedores: %w", err)
	}
	for _, id := range order {
		ov.Vendedores = append(ov.Vendedores, *byUser[id])
	}
	sort.SliceStable(ov.Vendedores, func(i, j int) bool {
		a, b := ov.Vendedores[i], ov.Vendedores[j]
		if a.Forecast != b.Forecast {
			return a.Forecast > b.Forecast
		}
		if a.Total["prospecto"] != b.Total["prospecto"] {
			return a.Total["prospecto"] > b.Total["prospecto"]
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	return ov, nil
}
