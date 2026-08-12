package store

import (
	"context"
	"path/filepath"
	"testing"

	cladex "github.com/gerrygoo/cladex-web"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(context.Background(), dsn, cladex.MigrationsFS)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestUserByUsername(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if u, err := s.UserByUsername(ctx, "nadie"); err != nil || u != nil {
		t.Fatalf("UserByUsername(nonexistent) = %+v, %v; want nil, nil", u, err)
	}

	id, err := s.CreateUser(ctx, "rodolfo", "Rodolfo Flores", "hashedpw", "vendedor")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	u, err := s.UserByUsername(ctx, "rodolfo")
	if err != nil {
		t.Fatalf("UserByUsername: %v", err)
	}
	if u == nil || u.ID != id || u.Name != "Rodolfo Flores" || u.Role != "vendedor" || u.PasswordHash != "hashedpw" {
		t.Fatalf("UserByUsername = %+v", u)
	}
	if u.DisabledAt != nil {
		t.Fatalf("DisabledAt = %v, want nil", u.DisabledAt)
	}
}

func TestSessionLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userID, err := s.CreateUser(ctx, "ana", "Ana", "hashedpw", "admin")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if u, err := s.SessionUser(ctx, "no-such-hash"); err != nil || u != nil {
		t.Fatalf("SessionUser(missing) = %+v, %v; want nil, nil", u, err)
	}

	if err := s.CreateSession(ctx, userID, "tok-hash-1"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	u, err := s.SessionUser(ctx, "tok-hash-1")
	if err != nil {
		t.Fatalf("SessionUser: %v", err)
	}
	if u == nil || u.ID != userID || u.Username != "ana" || u.Role != "admin" {
		t.Fatalf("SessionUser = %+v", u)
	}

	if err := s.RenewSession(ctx, "tok-hash-1"); err != nil {
		t.Fatalf("RenewSession: %v", err)
	}

	if err := s.DeleteSession(ctx, "tok-hash-1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if u, err := s.SessionUser(ctx, "tok-hash-1"); err != nil || u != nil {
		t.Fatalf("SessionUser(deleted) = %+v, %v; want nil, nil", u, err)
	}
}

func TestSessionUserIgnoresDisabledUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userID, err := s.CreateUser(ctx, "emilio", "Emilio", "hashedpw", "vendedor")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := s.CreateSession(ctx, userID, "tok-hash-2"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE users SET disabled_at = '2026-01-01T00:00:00.000Z' WHERE id = ?`, userID); err != nil {
		t.Fatalf("disable user: %v", err)
	}

	if u, err := s.SessionUser(ctx, "tok-hash-2"); err != nil || u != nil {
		t.Fatalf("SessionUser(disabled) = %+v, %v; want nil, nil", u, err)
	}
}

func TestLoginAttemptFailureStreak(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if count, last, err := s.UsernameFailureStreak(ctx, "rodolfo"); err != nil || count != 0 || last != "" {
		t.Fatalf("UsernameFailureStreak(none) = %d, %q, %v", count, last, err)
	}

	for i := 0; i < 3; i++ {
		if err := s.RecordLoginAttempt(ctx, "rodolfo", "10.0.0.1", false); err != nil {
			t.Fatalf("RecordLoginAttempt: %v", err)
		}
	}
	count, last, err := s.UsernameFailureStreak(ctx, "rodolfo")
	if err != nil {
		t.Fatalf("UsernameFailureStreak: %v", err)
	}
	if count != 3 || last == "" {
		t.Fatalf("UsernameFailureStreak = %d, %q; want 3, non-empty", count, last)
	}

	ipCount, _, err := s.IPFailureStreak(ctx, "10.0.0.1")
	if err != nil {
		t.Fatalf("IPFailureStreak: %v", err)
	}
	if ipCount != 3 {
		t.Fatalf("IPFailureStreak = %d, want 3", ipCount)
	}

	// A success resets the username's streak but not an unrelated IP's.
	if err := s.RecordLoginAttempt(ctx, "rodolfo", "10.0.0.2", true); err != nil {
		t.Fatalf("RecordLoginAttempt(success): %v", err)
	}
	count, _, err = s.UsernameFailureStreak(ctx, "rodolfo")
	if err != nil {
		t.Fatalf("UsernameFailureStreak: %v", err)
	}
	if count != 0 {
		t.Fatalf("UsernameFailureStreak after success = %d, want 0", count)
	}
	ipCount, _, err = s.IPFailureStreak(ctx, "10.0.0.1")
	if err != nil {
		t.Fatalf("IPFailureStreak: %v", err)
	}
	if ipCount != 3 {
		t.Fatalf("IPFailureStreak after unrelated success = %d, want 3", ipCount)
	}
}
