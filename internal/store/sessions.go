// Session and login-attempt writes deliberately bypass the audit-stamping exec in
// audit.go: they are not audited (see migrations/0003_audit_log.sql), they run on every
// authenticated request, and wrapping each one in a transaction would buy nothing.
package store

import (
	"context"
	"database/sql"
	"fmt"
)

// AuthenticatedUser is the trimmed view of a user attached to the request context by
// auth middleware — no password hash.
type AuthenticatedUser struct {
	ID       int64
	Username string
	Name     string
	Role     string // "admin" or "vendedor"
}

// CreateSession inserts a new session for userID, expiring 14 days from now. tokenHash
// is the SHA-256 hex digest of the cookie token — the raw token never reaches the DB.
func (s *Store) CreateSession(ctx context.Context, userID int64, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (user_id, token_hash, expires_at)
		VALUES (?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now', '+14 days'))`,
		userID, tokenHash,
	)
	if err != nil {
		return fmt.Errorf("store: create session: %w", err)
	}
	return nil
}

// SessionUser returns the user attached to a live (unexpired) session with the given
// token hash, or nil if the session doesn't exist, has expired, or the user has since
// been disabled.
func (s *Store) SessionUser(ctx context.Context, tokenHash string) (*AuthenticatedUser, error) {
	var u AuthenticatedUser
	err := s.db.QueryRowContext(ctx, `
		SELECT u.id, u.username, u.name, u.role
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ?
		  AND s.expires_at > strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		  AND u.disabled_at IS NULL`,
		tokenHash,
	).Scan(&u.ID, &u.Username, &u.Name, &u.Role)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: session user: %w", err)
	}
	return &u, nil
}

// RenewSession implements sliding expiry: it pushes expires_at out to 14 days from now,
// but only if the session isn't already fresh (more than a day old worth of validity
// left), so an active user doesn't write to the sessions table on every request.
func (s *Store) RenewSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE sessions
		SET expires_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now', '+14 days')
		WHERE token_hash = ?
		  AND expires_at < strftime('%Y-%m-%dT%H:%M:%fZ', 'now', '+13 days')`,
		tokenHash,
	)
	if err != nil {
		return fmt.Errorf("store: renew session: %w", err)
	}
	return nil
}

// DeleteSession removes a session by token hash (logout). Deleting a token hash that
// doesn't exist is not an error.
func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	if err != nil {
		return fmt.Errorf("store: delete session: %w", err)
	}
	return nil
}

// DeleteSessionsByUserID removes every session belonging to a user — used after an
// admin-driven password reset (cladexctl user passwd) so a stale cookie can't outlive
// the reset.
func (s *Store) DeleteSessionsByUserID(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("store: delete sessions for user %d: %w", userID, err)
	}
	return nil
}
