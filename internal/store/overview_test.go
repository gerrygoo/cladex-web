package store

import (
	"context"
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
	if len(empty.Stages) != 3 || empty.Stages[0].Count != 0 || len(empty.Vendedores) != 0 {
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

	borr := ov.Stages[0]
	if borr.Status != "borrador" || borr.Count != 4 || borr.Total != money.Centavos(1_100_00) {
		t.Errorf("borrador stage = %+v", borr)
	}
	var top []string
	for _, q := range borr.Top {
		top = append(top, q.Folio)
	}
	if len(top) != 3 || top[0] != "QA0002" || top[1] != "QA0003" || top[2] != "QA0004" {
		t.Errorf("borrador top = %v, want [QA0002 QA0003 QA0004]", top)
	}
	if ov.Stages[1].Count != 2 || ov.Stages[2].Count != 1 || len(ov.Stages[2].Top) != 1 {
		t.Errorf("emitida/revisada stages = %+v / %+v", ov.Stages[1], ov.Stages[2])
	}

	// The admin quoted too and issued more, so they lead; a user with no quotes is absent.
	if len(ov.Vendedores) != 2 {
		t.Fatalf("vendedores = %+v, want 2 rows", ov.Vendedores)
	}
	first, second := ov.Vendedores[0], ov.Vendedores[1]
	if first.UserID != admin || first.Emitidas != 1 || first.MontoEmitido != money.Centavos(5_000_00) ||
		first.Borradores != 2 || first.MontoBorrador != money.Centavos(500_00) {
		t.Errorf("first vendedor = %+v", first)
	}
	if second.UserID != vend || second.Borradores != 2 || second.Emitidas != 1 || second.Revisadas != 1 {
		t.Errorf("second vendedor = %+v", second)
	}
}
