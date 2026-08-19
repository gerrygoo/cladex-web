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

// UserByID returns the user with the given id, or nil if none exists.
func (s *Store) UserByID(ctx context.Context, id int64) (*User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `
		SELECT id, username, name, password_hash, role, disabled_at
		FROM users WHERE id = ?`, id,
	).Scan(&u.ID, &u.Username, &u.Name, &u.PasswordHash, &u.Role, &u.DisabledAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: user by id %d: %w", id, err)
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

// UpdatePassword sets a user's password hash — used both by admin-driven resets
// (cladexctl user passwd) and self-service change (POST /mi-cuenta).
func (s *Store) UpdatePassword(ctx context.Context, userID int64, passwordHash string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, userID)
	if err != nil {
		return fmt.Errorf("store: update password for user %d: %w", userID, err)
	}
	return nil
}

// DisableUser sets disabled_at on the named user, returning false if no such user
// exists. Disabling is immediate: SessionUser already excludes disabled users, so no
// separate session revocation is needed.
func (s *Store) DisableUser(ctx context.Context, username string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE users SET disabled_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE username = ?`, username,
	)
	if err != nil {
		return false, fmt.Errorf("store: disable user %q: %w", username, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: disable user %q: %w", username, err)
	}
	return n > 0, nil
}

// userSortColumns is the sortable-column whitelist for ListUsers; the first entry
// (username) is the default when sort doesn't match a known column.
var userSortColumns = []sortColumn{
	{"username", "username"},
	{"name", "name"},
	{"role", "role"},
	{"status", "disabled_at"},
}

// ListUsers returns all users, sorted per sort/dir (see userSortColumns; dir is "asc"
// or "desc"), for the admin /usuarios page.
func (s *Store) ListUsers(ctx context.Context, sort, dir string) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, username, name, password_hash, role, disabled_at
		FROM users `+orderByClause(userSortColumns, sort, dir),
	)
	if err != nil {
		return nil, fmt.Errorf("store: list users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Name, &u.PasswordHash, &u.Role, &u.DisabledAt); err != nil {
			return nil, fmt.Errorf("store: list users: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// SetUserDisabled sets or clears disabled_at for the given user id, by id (unlike
// DisableUser, which the CLI uses by username) — the admin /usuarios page already has
// the row's id from ListUsers.
func (s *Store) SetUserDisabled(ctx context.Context, id int64, disabled bool) error {
	var err error
	if disabled {
		_, err = s.db.ExecContext(ctx, `
			UPDATE users SET disabled_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
			WHERE id = ?`, id)
	} else {
		_, err = s.db.ExecContext(ctx, `UPDATE users SET disabled_at = NULL WHERE id = ?`, id)
	}
	if err != nil {
		return fmt.Errorf("store: set user %d disabled=%v: %w", id, disabled, err)
	}
	return nil
}

// SetUserRole updates a user's role. role must be "admin" or "vendedor" (enforced by
// the users.role CHECK constraint).
func (s *Store) SetUserRole(ctx context.Context, id int64, role string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET role = ? WHERE id = ?`, role, id)
	if err != nil {
		return fmt.Errorf("store: set user %d role %q: %w", id, role, err)
	}
	return nil
}
