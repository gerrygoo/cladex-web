package store

import (
	"context"
	"fmt"
)

// RecordLoginAttempt logs one login attempt for rate limiting and audit.
func (s *Store) RecordLoginAttempt(ctx context.Context, username, ip string, success bool) error {
	successInt := 0
	if success {
		successInt = 1
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO login_attempts (username, ip, success) VALUES (?, ?, ?)`,
		username, ip, successInt,
	)
	if err != nil {
		return fmt.Errorf("store: record login attempt: %w", err)
	}
	return nil
}

// UsernameFailureStreak returns the count and most recent timestamp (ISO-8601 UTC, or
// "" if count is 0) of failed login attempts for username since its last success — a
// successful login resets the streak.
func (s *Store) UsernameFailureStreak(ctx context.Context, username string) (count int, lastAttempt string, err error) {
	err = s.db.QueryRowContext(ctx, `
		SELECT count(*), COALESCE(max(attempted_at), '')
		FROM login_attempts
		WHERE username = ? AND success = 0
		  AND attempted_at > COALESCE(
		        (SELECT max(attempted_at) FROM login_attempts WHERE username = ? AND success = 1),
		        '')`,
		username, username,
	).Scan(&count, &lastAttempt)
	if err != nil {
		return 0, "", fmt.Errorf("store: username failure streak: %w", err)
	}
	return count, lastAttempt, nil
}

// IPFailureStreak is UsernameFailureStreak's per-IP counterpart.
func (s *Store) IPFailureStreak(ctx context.Context, ip string) (count int, lastAttempt string, err error) {
	err = s.db.QueryRowContext(ctx, `
		SELECT count(*), COALESCE(max(attempted_at), '')
		FROM login_attempts
		WHERE ip = ? AND success = 0
		  AND attempted_at > COALESCE(
		        (SELECT max(attempted_at) FROM login_attempts WHERE ip = ? AND success = 1),
		        '')`,
		ip, ip,
	).Scan(&count, &lastAttempt)
	if err != nil {
		return 0, "", fmt.Errorf("store: ip failure streak: %w", err)
	}
	return count, lastAttempt, nil
}
