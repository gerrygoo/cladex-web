package store

import (
	"context"
	"testing"
)

func TestListUsers(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.CreateUser(ctx, "ana", "Ana", "hashedpw", "admin"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := s.CreateUser(ctx, "rodolfo", "Rodolfo", "hashedpw", "vendedor"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	users, err := s.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("ListUsers = %d users, want 2", len(users))
	}
	// Ordered by username: "ana" before "rodolfo".
	if users[0].Username != "ana" || users[1].Username != "rodolfo" {
		t.Fatalf("ListUsers order = %+v", users)
	}
}

func TestSetUserDisabledByID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, err := s.CreateUser(ctx, "rodolfo", "Rodolfo", "hashedpw", "vendedor")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if err := s.SetUserDisabled(ctx, id, true); err != nil {
		t.Fatalf("SetUserDisabled(true): %v", err)
	}
	u, err := s.UserByID(ctx, id)
	if err != nil || u == nil || u.DisabledAt == nil {
		t.Fatalf("UserByID after disable = %+v, %v; want disabled", u, err)
	}

	if err := s.SetUserDisabled(ctx, id, false); err != nil {
		t.Fatalf("SetUserDisabled(false): %v", err)
	}
	u, err = s.UserByID(ctx, id)
	if err != nil || u == nil || u.DisabledAt != nil {
		t.Fatalf("UserByID after re-enable = %+v, %v; want enabled", u, err)
	}
}

func TestSetUserRole(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, err := s.CreateUser(ctx, "rodolfo", "Rodolfo", "hashedpw", "vendedor")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if err := s.SetUserRole(ctx, id, "admin"); err != nil {
		t.Fatalf("SetUserRole: %v", err)
	}
	u, err := s.UserByID(ctx, id)
	if err != nil || u == nil || u.Role != "admin" {
		t.Fatalf("UserByID after role change = %+v, %v; want role admin", u, err)
	}
}
