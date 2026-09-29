package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestFilterWhereIgnoresUnknownAndInvalid(t *testing.T) {
	where, args := filterWhere(context.Background(), quoteFilterColumns, Filters{
		"nope":      "x",
		"total.min": "abc",
		"fecha.max": "31/12/2026",
		"folio":     "QA",
	})
	if where != ` AND q.folio LIKE ? ESCAPE '\' COLLATE NOCASE` || fmt.Sprint(args) != "[%QA%]" {
		t.Errorf("where = %q, args = %v; want only the folio filter", where, args)
	}
}

func TestListQuotesColumnFilters(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	customerID, err := s.CreateCustomer(ctx, Customer{Name: "Grupo PEME"})
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	ana, _ := s.CreateUser(ctx, "ana", "Ana", "hash", "vendedor")
	beto, _ := s.CreateUser(ctx, "beto", "Beto", "hash", "vendedor")
	rows := []struct {
		folio, prefix, status, created string
		user, total                    int64
	}{
		{"QA0001", "QA", "borrador", "2026-01-10T10:00:00Z", ana, 100_00},
		{"QA0002", "QA", "emitida", "2026-02-10T10:00:00Z", ana, 500_00},
		{"QI0001", "QI", "emitida", "2026-03-10T10:00:00Z", beto, 900_00},
	}
	for _, r := range rows {
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO quotes (folio, prefix, customer_id, user_id, status, total, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			r.folio, r.prefix, customerID, r.user, r.status, r.total, r.created); err != nil {
			t.Fatalf("insert %s: %v", r.folio, err)
		}
	}

	tests := []struct {
		name    string
		filters Filters
		want    string
	}{
		{"none", nil, "[QA0002 QA0001 QI0001]"},
		{"estado", Filters{"estado": "emitida"}, "[QA0002 QI0001]"},
		{"autor contains", Filters{"autor": "bet"}, "[QI0001]"},
		{"autor exact", Filters{"autor.is": "ana"}, "[QA0002 QA0001]"},
		{"autor exact is not a substring", Filters{"autor.is": "an"}, "[]"},
		{"total range", Filters{"total.min": "200", "total.max": "600"}, "[QA0002]"},
		{"total min only", Filters{"total.min": "500.00"}, "[QA0002 QI0001]"},
		{"fecha range", Filters{"fecha.min": "2026-02-01", "fecha.max": "2026-02-28"}, "[QA0002]"},
		{"combined", Filters{"estado": "emitida", "autor": "ana"}, "[QA0002]"},
		{"folio literal wildcard", Filters{"folio": "%"}, "[]"},
	}
	for _, tt := range tests {
		quotes, err := s.ListQuotes(ctx, "", "", "", tt.filters)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		got := []string{}
		for _, q := range quotes {
			got = append(got, q.Folio)
		}
		if fmt.Sprint(got) != tt.want {
			t.Errorf("%s: got %v, want %s", tt.name, got, tt.want)
		}
	}
}

func TestListQuotesDateFilterUsesViewerZone(t *testing.T) {
	s := newTestStore(t)
	customerID, _ := s.CreateCustomer(context.Background(), Customer{Name: "Grupo PEME"})
	ana, _ := s.CreateUser(context.Background(), "ana", "Ana", "hash", "vendedor")
	// 03:00 UTC on the 28th is 21:00 on the 27th in Mexico City (UTC-6).
	if _, err := s.db.Exec(`INSERT INTO quotes (folio, prefix, customer_id, user_id, status, created_at) VALUES ('QA0001', 'QA', ?, ?, 'emitida', '2026-09-28T03:00:00.000Z')`, customerID, ana); err != nil {
		t.Fatal(err)
	}
	mexico := time.FixedZone("UTC-6", -6*3600)
	count := func(ctx context.Context, f Filters) int {
		qs, err := s.ListQuotes(ctx, "", "", "", f)
		if err != nil {
			t.Fatal(err)
		}
		return len(qs)
	}
	on27 := Filters{"fecha.min": "2026-09-27", "fecha.max": "2026-09-27"}
	on28 := Filters{"fecha.min": "2026-09-28", "fecha.max": "2026-09-28"}
	local := WithLocation(context.Background(), mexico)
	if count(local, on27) != 1 || count(local, on28) != 0 {
		t.Error("in Mexico City the quote belongs to the 27th")
	}
	if count(context.Background(), on27) != 0 || count(context.Background(), on28) != 1 {
		t.Error("with no zone the quote belongs to its UTC day, the 28th")
	}
}

func TestListUsersStatusFilter(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if _, err := s.CreateUser(ctx, "ana", "Ana", "hash", "vendedor"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser(ctx, "beto", "Beto", "hash", "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DisableUser(ctx, "beto"); err != nil {
		t.Fatal(err)
	}
	for filters, want := range map[string]string{"activo": "ana", "deshabilitado": "beto"} {
		users, err := s.ListUsers(ctx, "", "", Filters{"status": filters})
		if err != nil || len(users) != 1 || users[0].Username != want {
			t.Errorf("status=%s: got %+v, %v; want just %s", filters, users, err, want)
		}
	}
	if users, _ := s.ListUsers(ctx, "", "", Filters{"role": "admin"}); len(users) != 1 || users[0].Username != "beto" {
		t.Errorf("role=admin: got %+v", users)
	}
}

func TestQuoteFilterChoices(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	acme, _ := s.CreateCustomer(ctx, Customer{Name: "acme"})
	zeta, _ := s.CreateCustomer(ctx, Customer{Name: "Zeta"})
	_, _ = s.CreateCustomer(ctx, Customer{Name: "Sin cotizaciones"})
	ana, _ := s.CreateUser(ctx, "ana", "Ana", "hash", "vendedor")
	for i, c := range []int64{zeta, acme, acme} {
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO quotes (folio, prefix, customer_id, user_id, status) VALUES (?, 'QA', ?, ?, 'borrador')`,
			fmt.Sprintf("QA%04d", i+1), c, ana); err != nil {
			t.Fatal(err)
		}
	}
	customers, authors, err := s.QuoteFilterChoices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(customers) != "[acme Zeta]" || fmt.Sprint(authors) != "[Ana]" {
		t.Errorf("customers = %v, authors = %v", customers, authors)
	}

	// Past the cap a column falls back to free text (nil choices).
	for i := 0; i <= MaxFilterChoices; i++ {
		id, _ := s.CreateCustomer(ctx, Customer{Name: fmt.Sprintf("Cliente %03d", i)})
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO quotes (folio, prefix, customer_id, user_id, status) VALUES (?, 'QS', ?, ?, 'borrador')`,
			fmt.Sprintf("QS%04d", i+1), id, ana); err != nil {
			t.Fatal(err)
		}
	}
	customers, authors, err = s.QuoteFilterChoices(ctx)
	if err != nil || customers != nil || fmt.Sprint(authors) != "[Ana]" {
		t.Errorf("over the cap: customers = %v, authors = %v, err = %v", customers, authors, err)
	}
}
