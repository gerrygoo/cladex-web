package store

import (
	"context"
	"strings"
	"testing"

	"github.com/gerrygoo/cladex-web/internal/money"
)

func TestQuoteOverview(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	empty, err := s.QuoteOverview(ctx)
	if err != nil {
		t.Fatalf("QuoteOverview(empty): %v", err)
	}
	if len(empty.Stages) != len(QuoteStatuses) || empty.Stages[0].Count != 0 || len(empty.Vendedores) != 0 {
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

	quotes := []struct {
		folio  string
		user   int64
		status string
		total  int64
	}{
		{"QA0001", vend, "borrador", 100_00},
		{"QA0002", vend, "borrador", 500_00},
		{"QA0003", admin, "borrador", 300_00},
		{"QA0004", admin, "borrador", 200_00},
		{"QA0005", vend, "emitida", 1_000_00},
		{"QA0006", admin, "emitida", 5_000_00},
		{"QA0007", vend, "revisada", 800_00},
		{"QA0008", vend, "pipeline", 2_000_00},
		{"QA0009", admin, "pipeline", 700_00},
		{"QA0010", admin, "oc_emitida", 900_00},
		{"QA0011", vend, "entregada", 400_00},
		{"QA0012", vend, "cerrada", 600_00},
	}
	for _, q := range quotes {
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO quotes (folio, prefix, customer_id, user_id, status, total) VALUES (?, 'QA', ?, ?, ?, ?)`,
			q.folio, customerID, q.user, q.status, q.total); err != nil {
			t.Fatalf("insert %s: %v", q.folio, err)
		}
	}

	ov, err := s.QuoteOverview(ctx)
	if err != nil {
		t.Fatalf("QuoteOverview: %v", err)
	}

	var got []string
	for _, st := range ov.Stages {
		got = append(got, st.Status)
	}
	want := []string{"emitida", "pipeline", "oc_emitida", "entregada", "cerrada", "revisada"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("stages = %v, want %v (no borrador)", got, want)
	}
	wantCounts := []int{2, 2, 1, 1, 1, 1}
	for i, st := range ov.Stages {
		if st.Count != wantCounts[i] {
			t.Errorf("%s count = %d, want %d", st.Status, st.Count, wantCounts[i])
		}
	}
	if pipe := ov.Stages[1]; pipe.Total != money.Centavos(2_700_00) || len(pipe.Top) != 2 || pipe.Top[0].Folio != "QA0008" {
		t.Errorf("pipeline stage = %+v", pipe)
	}

	// Ranked by pipeline amount: the vendedor's $2,000 beats the admin's $700 even though
	// the admin has the larger issued amount; a user with no quotes is absent.
	if len(ov.Vendedores) != 2 {
		t.Fatalf("vendedores = %+v, want 2 rows", ov.Vendedores)
	}
	first, second := ov.Vendedores[0], ov.Vendedores[1]
	if first.UserID != vend || first.Total["pipeline"] != money.Centavos(2_000_00) ||
		first.Count["emitida"] != 1 || first.Count["revisada"] != 1 || first.Count["entregada"] != 1 || first.Count["cerrada"] != 1 {
		t.Errorf("first vendedor = %+v", first)
	}
	if second.UserID != admin || second.Total["emitida"] != money.Centavos(5_000_00) || second.Count["oc_emitida"] != 1 {
		t.Errorf("second vendedor = %+v", second)
	}
}
