package cli

import (
	"context"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/bcrypt"

	cladex "github.com/gerrygoo/cladex-web"
	"github.com/gerrygoo/cladex-web/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dsn, cladex.MigrationsFS)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestUserAdd(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := Run(ctx, s, []string{"user", "add", "rodolfo", "--name", "Rodolfo Flores", "--role", "vendedor"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	u, err := s.UserByUsername(ctx, "rodolfo")
	if err != nil {
		t.Fatalf("UserByUsername: %v", err)
	}
	if u == nil || u.Name != "Rodolfo Flores" || u.Role != "vendedor" {
		t.Fatalf("user = %+v", u)
	}
	cost, err := bcrypt.Cost([]byte(u.PasswordHash))
	if err != nil {
		t.Fatalf("bcrypt.Cost: %v", err)
	}
	if cost != bcryptCost {
		t.Fatalf("bcrypt cost = %d, want %d", cost, bcryptCost)
	}
}

func TestUserAddRejectsBadRole(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	err := Run(ctx, s, []string{"user", "add", "rodolfo", "--name", "Rodolfo", "--role", "gerente"})
	if err == nil {
		t.Fatal("Run: expected error for bad role, got nil")
	}
}

func TestUserAddRequiresName(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	err := Run(ctx, s, []string{"user", "add", "rodolfo"})
	if err == nil {
		t.Fatal("Run: expected error for missing --name, got nil")
	}
}

func TestUserPasswd(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := Run(ctx, s, []string{"user", "add", "rodolfo", "--name", "Rodolfo", "--role", "vendedor"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	before, err := s.UserByUsername(ctx, "rodolfo")
	if err != nil {
		t.Fatalf("UserByUsername: %v", err)
	}
	if err := s.CreateSession(ctx, before.ID, "some-hash"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if err := Run(ctx, s, []string{"user", "passwd", "rodolfo"}); err != nil {
		t.Fatalf("passwd: %v", err)
	}

	after, err := s.UserByUsername(ctx, "rodolfo")
	if err != nil {
		t.Fatalf("UserByUsername: %v", err)
	}
	if after.PasswordHash == before.PasswordHash {
		t.Fatal("password hash unchanged after passwd")
	}

	// The password reset must invalidate existing sessions.
	if u, err := s.SessionUser(ctx, "some-hash"); err != nil || u != nil {
		t.Fatalf("SessionUser after passwd = %+v, %v; want nil, nil", u, err)
	}
}

func TestUserPasswdUnknownUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := Run(ctx, s, []string{"user", "passwd", "nadie"}); err == nil {
		t.Fatal("Run: expected error for unknown user, got nil")
	}
}

func TestUserDisable(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := Run(ctx, s, []string{"user", "add", "emilio", "--name", "Emilio", "--role", "vendedor"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := Run(ctx, s, []string{"user", "disable", "emilio"}); err != nil {
		t.Fatalf("disable: %v", err)
	}
	u, err := s.UserByUsername(ctx, "emilio")
	if err != nil {
		t.Fatalf("UserByUsername: %v", err)
	}
	if u.DisabledAt == nil {
		t.Fatal("DisabledAt is nil after disable")
	}
}

func TestUserDisableUnknownUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := Run(ctx, s, []string{"user", "disable", "nadie"}); err == nil {
		t.Fatal("Run: expected error for unknown user, got nil")
	}
}
