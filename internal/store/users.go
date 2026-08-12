package store

import (
	"context"
	"database/sql"
	"fmt"
)

// User is a full users row, including the password hash — only for auth-path code
// (login, cladexctl). Everything else should use the trimmed AuthenticatedUser.
type User struct {
	ID           int64
	Username     string
	Name         string
	PasswordHash string
	Role         string // "admin" or "vendedor"
	DisabledAt   *string
}

// UserByUsername returns the user with the given username, or nil if none exists.
func (s *Store) UserByUsername(ctx context.Context, username string) (*User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `
		SELECT id, username, name, password_hash, role, disabled_at
		FROM users WHERE username = ?`, username,
	).Scan(&u.ID, &u.Username, &u.Name, &u.PasswordHash, &u.Role, &u.DisabledAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: user by username %q: %w", username, err)
	}
	return &u, nil
}

// CreateUser inserts a new user with an already-hashed password, returning its id.
// role must be "admin" or "vendedor" (enforced by the users.role CHECK constraint).
func (s *Store) CreateUser(ctx context.Context, username, name, passwordHash, role string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO users (username, name, password_hash, role) VALUES (?, ?, ?, ?)`,
		username, name, passwordHash, role,
	)
	if err != nil {
		return 0, fmt.Errorf("store: create user %q: %w", username, err)
	}
	return res.LastInsertId()
}
