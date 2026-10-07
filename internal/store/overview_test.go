package store

import (
	"context"
	"strings"
	"testing"

	"github.com/gerrygoo/cladex-web/internal/money"
)

func TestProjectOverview(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	empty, err := s.ProjectOverview(ctx, "2026-10-07")
	if err != nil {
		t.Fatalf("ProjectOverview(empty): %v", err)
	}
	if len(empty.Stages) != len(ProjectFlow) || empty.Stages[0].Count != 0 || len(empty.Vendedores) != 0 {
		t.Fatalf("empty overview = %+v", empty)
	}

	customerID, err := s.CreateCustomer(ctx, Customer{Name: "Grupo PEME"})
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	admin, err := s.CreateUser(ctx, "efs", "Emilio", "hash", "admin")
	if err != nil {
		t.Fatalf("CreateUser(admin): %v", err)
	}
	vend, err := s.CreateUser(ctx, "rfm", "Rodolfo", "hash", "vendedor")
	if err != nil {
		t.Fatalf("CreateUser(vendedor): %v", err)
	}
	if _, err := s.CreateUser(ctx, "ggr", "Sin cotizaciones", "hash", "vendedor"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// stage "" is a quote with no proyecto: a draft that was never issued.
	quotes := []struct {
		folio       string
		user        int64
		stage       string
		probability int
		total       int64
	}{
		{"QA0001", vend, "", 0, 100_00},
		{"QA0002", admin, "", 0, 300_00},
		{"QA0005", vend, "prospecto", 10, 1_000_00},
		{"QA0006", admin, "prospecto", 50, 5_000_00},
		{"QA0008", vend, "prospecto", 75, 2_000_00},
		{"QA0009", admin, "prospecto", 90, 700_00},
		{"QA0010", admin, "oc_recibida", 75, 900_00},
		{"QA0011", vend, "en_entrega", 10, 400_00},
		{"QA0012", vend, "cerrado", 10, 600_00},
	}
	for _, q := range quotes {
		if q.stage == "" {
			if _, err := s.db.ExecContext(ctx,
				`INSERT INTO quotes (folio, prefix, customer_id, user_id, status, total) VALUES (?, 'QA', ?, ?, 'borrador', ?)`,
				q.folio, customerID, q.user, q.total); err != nil {
				t.Fatalf("insert %s: %v", q.folio, err)
			}
			continue
		}
		insertProject(t, s, q.folio, customerID, q.user, q.stage, q.probability, q.total)
	}
	// QA0005 was revised: the proyecto's amount is the draft revision's, and the
	// superseded quote counts nowhere.
	if _, err := s.db.ExecContext(ctx, `UPDATE quotes SET status = 'revisada' WHERE folio = 'QA0005'`); err != nil {
		t.Fatalf("supersede: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO quotes (folio, prefix, customer_id, user_id, status, total, supersedes_quote_id, project_id)
		SELECT 'QA0005-R1', 'QA', customer_id, user_id, 'borrador', 1500_00, id, project_id FROM quotes WHERE folio = 'QA0005'`); err != nil {
		t.Fatalf("revision: %v", err)
	}

	ov, err := s.ProjectOverview(ctx, "2026-10-07")
	if err != nil {
		t.Fatalf("ProjectOverview: %v", err)
	}

	var got []string
	for _, st := range ov.Stages {
		got = append(got, st.Status)
	}
	want := []string{"prospecto", "oc_recibida", "facturado", "en_entrega", "cerrado"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("stages = %v, want %v", got, want)
	}
	wantCounts := []int{4, 1, 0, 1, 1}
	for i, st := range ov.Stages {
		if st.Count != wantCounts[i] {
			t.Errorf("%s count = %d, want %d", st.Status, st.Count, wantCounts[i])
		}
	}
	pros := ov.Stages[0]
	if pros.Total != money.Centavos(9_200_00) || len(pros.Top) != 3 || pros.Top[0].Folio != "QA0006" || pros.Top[2].Folio != "QA0005" || pros.Top[2].Total != money.Centavos(1_500_00) {
		t.Errorf("prospecto stage = %+v", pros)
	}
	if pros.ForecastCount != 2 || pros.ForecastTotal != money.Centavos(2_700_00) {
		t.Errorf("prospecto forecast = %d, %v; want 2 proyectos, $2,700", pros.ForecastCount, pros.ForecastTotal)
	}
	// A proyecto past prospecto is not a forecast, whatever probability it kept.
	if oc := ov.Stages[1]; oc.ForecastCount != 0 || oc.ForecastTotal != 0 {
		t.Errorf("oc_recibida forecast = %d, %v; want none", oc.ForecastCount, oc.ForecastTotal)
	}

	// Ranked by forecast amount: the vendedor's $2,000 beats the admin's $700 even though
	// the admin has the larger prospecto amount; a user with no proyectos is absent.
	if len(ov.Vendedores) != 2 {
		t.Fatalf("vendedores = %+v, want 2 rows", ov.Vendedores)
	}
	first, second := ov.Vendedores[0], ov.Vendedores[1]
	if first.UserID != vend || first.Forecast != money.Centavos(2_000_00) ||
		first.Count["prospecto"] != 2 || first.Count["en_entrega"] != 1 || first.Count["cerrado"] != 1 {
		t.Errorf("first vendedor = %+v", first)
	}
	if second.UserID != admin || second.Forecast != money.Centavos(700_00) ||
		second.Total["prospecto"] != money.Centavos(5_700_00) || second.Count["oc_recibida"] != 1 {
		t.Errorf("second vendedor = %+v", second)
	}
}

// insertProject seeds a proyecto in the given stage with one issued quote of the same
// folio, bypassing IssueQuote, and returns the quote's id.
func insertProject(t *testing.T, s *Store, folio string, customerID, userID int64, stage string, probability int, total int64) int64 {
	t.Helper()
	ctx := context.Background()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO projects (folio, customer_id, user_id, status, probability) VALUES (?, ?, ?, ?, ?)`,
		folio, customerID, userID, stage, probability)
	if err != nil {
		t.Fatalf("insert project %s: %v", folio, err)
	}
	projectID, _ := res.LastInsertId()
	res, err = s.db.ExecContext(ctx,
		`INSERT INTO quotes (folio, prefix, customer_id, user_id, status, total, project_id) VALUES (?, ?, ?, ?, 'emitida', ?, ?)`,
		folio, folio[:2], customerID, userID, total, projectID)
	if err != nil {
		t.Fatalf("insert quote %s: %v", folio, err)
	}
	id, _ := res.LastInsertId()
	return id
}
