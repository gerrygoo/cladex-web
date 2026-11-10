package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Aviso kinds (migrations/0037_avisos.sql). Each is turned on per user, by an admin.
const (
	// AvisoSeguimientoVencido goes to a proyecto's owner when its seguimiento is overdue.
	AvisoSeguimientoVencido = "seguimiento_vencido"
	// AvisoSeguimientoEscalado goes to everyone who has it on when an owner leaves an
	// overdue seguimiento unattended.
	AvisoSeguimientoEscalado = "seguimiento_escalado"
)

// AvisoKind describes an aviso for the screens where an admin turns it on.
type AvisoKind struct {
	Key         string
	Label       string
	Description string
}

// AvisoKinds lists every aviso, in the order the screens show them.
var AvisoKinds = []AvisoKind{
	{AvisoSeguimientoVencido, "Seguimiento vencido de sus proyectos",
		"Si pasa el día de seguimiento de un proyecto suyo sin que se registre nada: el día hábil siguiente a las 9:00, 12:00 y 18:00, y después cada día hábil a las 18:00."},
	{AvisoSeguimientoEscalado, "Seguimientos vencidos del equipo",
		"Si un vendedor no atiende un seguimiento vencido: el segundo día hábil a las 12:00, y después cada 3 días hábiles."},
}

// IsAvisoKind reports whether kind is one of AvisoKinds.
func IsAvisoKind(kind string) bool {
	for _, k := range AvisoKinds {
		if k.Key == kind {
			return true
		}
	}
	return false
}

// UserAvisos returns the avisos a user has on.
func (s *Store) UserAvisos(ctx context.Context, userID int64) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT kind FROM user_avisos WHERE user_id = ?`, userID)
	if err != nil {
		return nil, fmt.Errorf("store: avisos of user %d: %w", userID, err)
	}
	defer rows.Close()
	on := map[string]bool{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, fmt.Errorf("store: avisos of user %d: %w", userID, err)
		}
		on[k] = true
	}
	return on, rows.Err()
}

// SetUserAvisos makes kinds the avisos a user has on, turning the rest off. Unknown kinds
// are ignored.
func (s *Store) SetUserAvisos(ctx context.Context, userID int64, kinds []string) error {
	want := map[string]bool{}
	for _, k := range kinds {
		if IsAvisoKind(k) {
			want[k] = true
		}
	}
	return s.inTx(ctx, "set user avisos", func(tx *sql.Tx) error {
		for _, k := range AvisoKinds {
			var err error
			if want[k.Key] {
				_, err = tx.ExecContext(ctx, `INSERT INTO user_avisos (user_id, kind) VALUES (?, ?) ON CONFLICT DO NOTHING`, userID, k.Key)
			} else {
				_, err = tx.ExecContext(ctx, `DELETE FROM user_avisos WHERE user_id = ? AND kind = ?`, userID, k.Key)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// AvisoRecipient is an active user with an aviso on, and where to reach them.
type AvisoRecipient struct {
	UserID int64
	Name   string
	UserContact
}

// AvisoRecipients returns the active users who have kind on, by name.
func (s *Store) AvisoRecipients(ctx context.Context, kind string) ([]AvisoRecipient, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.name, COALESCE(u.email, ''), COALESCE(u.whatsapp, ''), u.notify_email, u.notify_whatsapp
		FROM user_avisos a JOIN users u ON u.id = a.user_id
		WHERE a.kind = ? AND u.disabled_at IS NULL
		ORDER BY u.name COLLATE NOCASE, u.id`, kind)
	if err != nil {
		return nil, fmt.Errorf("store: recipients of %s: %w", kind, err)
	}
	defer rows.Close()
	var out []AvisoRecipient
	for rows.Next() {
		var r AvisoRecipient
		if err := rows.Scan(&r.UserID, &r.Name, &r.Email, &r.WhatsApp, &r.NotifyEmail, &r.NotifyWhatsApp); err != nil {
			return nil, fmt.Errorf("store: recipients of %s: %w", kind, err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ProspectosFollowUpBefore returns the prospectos whose next seguimiento is a day before
// day (YYYY-MM-DD), oldest seguimiento first.
func (s *Store) ProspectosFollowUpBefore(ctx context.Context, day string) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+projectSelectCols+projectFrom+`
		WHERE p.status = 'prospecto' AND p.next_followup_date IS NOT NULL AND p.next_followup_date < ?
		ORDER BY p.next_followup_date, p.id`, day)
	if err != nil {
		return nil, fmt.Errorf("store: prospectos with seguimiento before %s: %w", day, err)
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, fmt.Errorf("store: prospectos with seguimiento before %s: %w", day, err)
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// QueueAvisoRound queues a round's notifications, once: it records round in the same
// transaction, and a round already on record queues nothing and reports done.
func (s *Store) QueueAvisoRound(ctx context.Context, round string, ns []NewNotification, now time.Time) (done bool, err error) {
	err = s.inTx(ctx, "queue aviso round", func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `INSERT INTO aviso_rounds (round, queued, created_at) VALUES (?, ?, ?)
			ON CONFLICT (round) DO NOTHING`, round, len(ns), jobTime(now))
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n == 0 {
			done = true
			return nil
		}
		for _, n := range ns {
			if _, err := queueNotification(ctx, tx, n, now); err != nil {
				return err
			}
		}
		return nil
	})
	return done, err
}
