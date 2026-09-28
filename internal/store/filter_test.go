package store

import (
	"context"
	"fmt"
	"testing"
)

func TestFilterWhereIgnoresUnknownAndInvalid(t *testing.T) {
	where, args := filterWhere(quoteFilterColumns, Filters{
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
