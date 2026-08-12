package store

import (
	"context"
	"testing"
)

func TestCustomerCRUDLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if c, err := s.CustomerByID(ctx, 1); err != nil || c != nil {
		t.Fatalf("CustomerByID(nonexistent) = %+v, %v; want nil, nil", c, err)
	}

	id, err := s.CreateCustomer(ctx, Customer{
		Name:        "Grupo PEME",
		RFC:         "PEM010101ABC",
		ContactName: "Juan Pérez",
		Phone:       "442-123-4567",
		Email:       "juan@grupopeme.mx",
	})
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}

	got, err := s.CustomerByID(ctx, id)
	if err != nil {
		t.Fatalf("CustomerByID: %v", err)
	}
	if got == nil {
		t.Fatal("CustomerByID = nil, want a customer")
	}
	if got.Name != "Grupo PEME" || got.RFC != "PEM010101ABC" || got.ContactName != "Juan Pérez" {
		t.Fatalf("CustomerByID = %+v", got)
	}
	if got.Address != "" || got.Notes != "" {
		t.Fatalf("unset fields should be empty strings, got Address=%q Notes=%q", got.Address, got.Notes)
	}

	// Update.
	got.Name = "Grupo PEME SA de CV"
	got.Notes = "Cliente frecuente"
	if err := s.UpdateCustomer(ctx, *got); err != nil {
		t.Fatalf("UpdateCustomer: %v", err)
	}
	updated, err := s.CustomerByID(ctx, id)
	if err != nil {
		t.Fatalf("CustomerByID after update: %v", err)
	}
	if updated.Name != "Grupo PEME SA de CV" || updated.Notes != "Cliente frecuente" {
		t.Fatalf("CustomerByID after update = %+v", updated)
	}

	// List + search.
	customers, err := s.ListCustomers(ctx, "")
	if err != nil {
		t.Fatalf("ListCustomers(\"\"): %v", err)
	}
	if len(customers) != 1 {
		t.Fatalf("ListCustomers(\"\") = %d customers, want 1", len(customers))
	}
	if customers, err = s.ListCustomers(ctx, "peme"); err != nil || len(customers) != 1 {
		t.Fatalf("ListCustomers(case-insensitive substring) = %d, %v; want 1, nil", len(customers), err)
	}
	if customers, err = s.ListCustomers(ctx, "juan"); err != nil || len(customers) != 1 {
		t.Fatalf("ListCustomers(match on contact_name) = %d, %v; want 1, nil", len(customers), err)
	}
	if customers, err = s.ListCustomers(ctx, "no existe"); err != nil || len(customers) != 0 {
		t.Fatalf("ListCustomers(no match) = %d, %v; want 0, nil", len(customers), err)
	}

	// Soft-delete.
	if err := s.SoftDeleteCustomer(ctx, id); err != nil {
		t.Fatalf("SoftDeleteCustomer: %v", err)
	}
	if c, err := s.CustomerByID(ctx, id); err != nil || c != nil {
		t.Fatalf("CustomerByID after delete = %+v, %v; want nil, nil", c, err)
	}
	if customers, err := s.ListCustomers(ctx, ""); err != nil || len(customers) != 0 {
		t.Fatalf("ListCustomers after delete = %d, %v; want 0, nil", len(customers), err)
	}
	var deletedAt *string
	if err := s.db.QueryRowContext(ctx, `SELECT deleted_at FROM customers WHERE id = ?`, id).Scan(&deletedAt); err != nil {
		t.Fatalf("check deleted_at: %v", err)
	}
	if deletedAt == nil {
		t.Fatal("deleted_at is still NULL after soft-delete")
	}
}
