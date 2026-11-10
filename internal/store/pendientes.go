package store

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Priorities of Mis pendientes (issue #72), lowest first. A pendiente libre takes any of
// them; a seguimiento starts at media (SeguimientoPeriod.Priority).
const (
	PriorityBaja    = "baja"
	PriorityMedia   = "media"
	PriorityAlta    = "alta"
	PriorityMuyAlta = "muy_alta"
)

// Priorities lists the priorities, lowest first, with their labels.
var Priorities = []struct{ Key, Label string }{
	{PriorityBaja, "Baja"}, {PriorityMedia, "Media"}, {PriorityAlta, "Alta"}, {PriorityMuyAlta, "Muy alta"},
}

// PriorityRank orders the priorities: 1 for baja up to 4 for muy alta, 0 for anything else.
func PriorityRank(p string) int {
	for i, x := range Priorities {
		if x.Key == p {
			return i + 1
		}
	}
	return 0
}

// PriorityLabel is the word for a priority, or "" for one that isn't.
func PriorityLabel(p string) string {
	for _, x := range Priorities {
		if x.Key == p {
			return x.Label
		}
	}
	return ""
}

// MaxPendienteLen is the longest description a pendiente takes, in characters.
const MaxPendienteLen = 500

// ErrPendienteNotFound is what every pendiente write returns for an id that isn't one of
// the user's own: someone else's is indistinguishable from one that doesn't exist.
var ErrPendienteNotFound = errors.New("store: pendiente not found")

// Pendiente is a to-do a user wrote on Mis pendientes. CompletedAt is empty while open.
type Pendiente struct {
	ID          int64
	Body        string
	Priority    string
	CreatedAt   string
	CompletedAt string
}

// AddPendiente adds an open pendiente for userID.
func (s *Store) AddPendiente(ctx context.Context, userID int64, body, priority string) (int64, error) {
	if PriorityRank(priority) == 0 {
		return 0, fmt.Errorf("store: add pendiente: priority %q", priority)
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO pendientes (user_id, body, priority) VALUES (?, ?, ?)`, userID, body, priority)
	if err != nil {
		return 0, fmt.Errorf("store: add pendiente: %w", err)
	}
	return res.LastInsertId()
}

// OpenPendientes returns userID's open pendientes, oldest first.
func (s *Store) OpenPendientes(ctx context.Context, userID int64) ([]Pendiente, error) {
	return s.pendientes(ctx, `WHERE user_id = ? AND completed_at IS NULL ORDER BY created_at, id`, userID)
}

// CompletedPendientes returns userID's pendientes completed at or after since, newest first.
func (s *Store) CompletedPendientes(ctx context.Context, userID int64, since time.Time) ([]Pendiente, error) {
	return s.pendientes(ctx, `WHERE user_id = ? AND completed_at >= ? ORDER BY completed_at DESC, id DESC`, userID, jobTime(since))
}

func (s *Store) pendientes(ctx context.Context, where string, args ...any) ([]Pendiente, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, body, priority, created_at, COALESCE(completed_at, '') FROM pendientes `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("store: pendientes: %w", err)
	}
	defer rows.Close()
	var out []Pendiente
	for rows.Next() {
		var p Pendiente
		if err := rows.Scan(&p.ID, &p.Body, &p.Priority, &p.CreatedAt, &p.CompletedAt); err != nil {
			return nil, fmt.Errorf("store: pendientes: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SetPendientePriority changes the priority of one of userID's open pendientes.
func (s *Store) SetPendientePriority(ctx context.Context, userID, id int64, priority string) error {
	if PriorityRank(priority) == 0 {
		return fmt.Errorf("store: pendiente %d: priority %q", id, priority)
	}
	return s.pendienteWrite(ctx, id, `UPDATE pendientes SET priority = ? WHERE id = ? AND user_id = ? AND completed_at IS NULL`, priority, id, userID)
}

// CompletePendiente checks off one of userID's open pendientes.
func (s *Store) CompletePendiente(ctx context.Context, userID, id int64, now time.Time) error {
	return s.pendienteWrite(ctx, id, `UPDATE pendientes SET completed_at = ? WHERE id = ? AND user_id = ? AND completed_at IS NULL`, jobTime(now), id, userID)
}

// ReopenPendiente puts one of userID's completed pendientes back on the list.
func (s *Store) ReopenPendiente(ctx context.Context, userID, id int64) error {
	return s.pendienteWrite(ctx, id, `UPDATE pendientes SET completed_at = NULL WHERE id = ? AND user_id = ? AND completed_at IS NOT NULL`, id, userID)
}

// pendienteWrite runs a one-row write scoped to its user, and reports
// ErrPendienteNotFound when it changes nothing.
func (s *Store) pendienteWrite(ctx context.Context, id int64, query string, args ...any) error {
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("store: pendiente %d: %w", id, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("store: pendiente %d: %w", id, err)
	} else if n != 1 {
		return ErrPendienteNotFound
	}
	return nil
}
