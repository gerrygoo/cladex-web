package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Aviso kinds (migrations/0037_avisos.sql). Each user turns their own on or off in Mi
// cuenta; the AdminOnly ones only an admin may have.
const (
	// AvisoSeguimientoVencido goes to a proyecto's owner when its seguimiento is overdue.
	AvisoSeguimientoVencido = "seguimiento_vencido"
	// AvisoSeguimientoEscalado goes to everyone who has it on when an owner leaves an
	// overdue seguimiento unattended.
	AvisoSeguimientoEscalado = "seguimiento_escalado"
	// AvisoOCRecibida goes to everyone who has it on when a proyecto's client O.C. is
	// received: it passes to Orden colocada 100% (#69 C).
	AvisoOCRecibida = "oc_recibida"
	// AvisoOCProveedor reminds whoever has it on that a proyecto's supplier O.C. is still
	// to be confirmed and sent, in the hours after its client O.C. was received (#69 F).
	AvisoOCProveedor = "oc_proveedor_pendiente"
	// AvisoResumenPerdidos is the monthly email of the proyectos lost in the month before
	// (#69 D).
	AvisoResumenPerdidos = "resumen_perdidos"
)

// AvisoOCJobKind is the job ReceiveOC queues, in its own transaction, when a proyecto's
// client O.C. is received; internal/avisos handles it (C, and the F reminders).
const AvisoOCJobKind = "avisos_oc_recibida"

// AvisoKind describes an aviso for the screens where it is turned on. AdminOnly ones
// are about the team or about what only an admin can do (confirm a supplier O.C.): only
// admins see them, and only admins get them.
type AvisoKind struct {
	Key         string
	Label       string
	Description string
	AdminOnly   bool
}

// Offered reports whether a user of role may have the aviso.
func (k AvisoKind) Offered(role string) bool { return !k.AdminOnly || role == "admin" }

// AvisoKindByKey returns the aviso with key, and false when there's none.
func AvisoKindByKey(key string) (AvisoKind, bool) {
	for _, k := range AvisoKinds {
		if k.Key == key {
			return k, true
		}
	}
	return AvisoKind{}, false
}

// AvisoKinds lists every aviso, in the order the screens show them.
var AvisoKinds = []AvisoKind{
	{AvisoSeguimientoVencido, "Seguimiento vencido de sus proyectos",
		"Si pasa el día de seguimiento de un proyecto suyo sin que se registre nada: el día hábil siguiente a las 9:00, 12:00 y 18:00, y después cada día hábil a las 18:00.", false},
	{AvisoSeguimientoEscalado, "Seguimientos vencidos del equipo",
		"Si un vendedor no atiende un seguimiento vencido: el segundo día hábil a las 12:00, y después cada 3 días hábiles.", true},
	{AvisoOCRecibida, "Orden de compra del cliente recibida",
		"En cuanto un proyecto pasa a Orden colocada 100%.", false},
	{AvisoOCProveedor, "Orden de compra al proveedor sin enviar",
		"Si la orden de compra al proveedor no está confirmada: una hora después de recibir la del cliente, dos horas después de eso y a las 18:00. Si la del cliente llega después de las 15:00, solo una hora y dos horas después. Las que llegan de noche o en fin de semana cuentan desde las 8:00 del siguiente día hábil.", true},
	{AvisoResumenPerdidos, "Resumen mensual de proyectos perdidos",
		"El primer día hábil de cada mes a las 9:00: los proyectos perdidos el mes anterior, con su cliente, su valor sin IVA y el motivo.", false},
}

// IsAvisoKind reports whether kind is one of AvisoKinds.
func IsAvisoKind(kind string) bool {
	_, ok := AvisoKindByKey(kind)
	return ok
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

// SetUserAvisos makes kinds the avisos a user has on among those offered to role (see
// AvisoKind.Offered), turning the rest of those off. Unknown kinds are ignored, and the
// ones not offered to role are left as they are.
func (s *Store) SetUserAvisos(ctx context.Context, userID int64, role string, kinds []string) error {
	want := map[string]bool{}
	for _, k := range kinds {
		if IsAvisoKind(k) {
			want[k] = true
		}
	}
	return s.inTx(ctx, "set user avisos", func(tx *sql.Tx) error {
		for _, k := range AvisoKinds {
			if !k.Offered(role) {
				continue
			}
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

// AvisoRecipients returns the active users who have kind on, by name. An AdminOnly kind
// goes only to admins, so someone who stops being one stops getting it.
func (s *Store) AvisoRecipients(ctx context.Context, kind string) ([]AvisoRecipient, error) {
	k, _ := AvisoKindByKey(kind)
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.name, COALESCE(u.email, ''), COALESCE(u.whatsapp, ''), u.notify_email, u.notify_whatsapp
		FROM user_avisos a JOIN users u ON u.id = a.user_id
		WHERE a.kind = ? AND u.disabled_at IS NULL AND (? OR u.role = 'admin')
		ORDER BY u.name COLLATE NOCASE, u.id`, kind, !k.AdminOnly)
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

// ProjectByID returns the proyecto with id, or nil if none exists.
func (s *Store) ProjectByID(ctx context.Context, id int64) (*Project, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+projectSelectCols+projectFrom+` WHERE p.id = ?`, id)
	p, err := scanProject(row)
	if err != nil {
		return nil, fmt.Errorf("store: project %d: %w", id, err)
	}
	return p, nil
}

// OCReceivedAt is when the proyecto last went from prospecto to en curso (its client
// O.C. was received), from the audit trail; zero if it never did.
func (s *Store) OCReceivedAt(ctx context.Context, projectID int64) (time.Time, error) {
	var at sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT at FROM audit_log
		WHERE table_name = 'projects' AND row_key = ? AND op = 'update'
			AND json_extract(old_values, '$.status') = 'prospecto'
			AND json_extract(new_values, '$.status') = 'en_curso'
		ORDER BY id DESC LIMIT 1`, projectID).Scan(&at)
	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	if err != nil || !at.Valid {
		return time.Time{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, at.String)
	if err != nil {
		return time.Time{}, fmt.Errorf("store: O.C. received at of project %d: %w", projectID, err)
	}
	return t, nil
}

// SupplierOCPending reports whether a proyecto's supplier O.C. is still to be sent: it has
// no confirmed one, or a draft is left (#69 F). Confirming is when the PDF is downloaded
// to send, so it counts as sent.
func (s *Store) SupplierOCPending(ctx context.Context, projectID int64) (bool, error) {
	var confirmed, drafts int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FILTER (WHERE status = 'confirmada'), COUNT(*) FILTER (WHERE status = 'borrador')
		FROM purchase_orders WHERE project_id = ?`, projectID).Scan(&confirmed, &drafts)
	if err != nil {
		return false, fmt.Errorf("store: supplier O.C. of project %d: %w", projectID, err)
	}
	return confirmed == 0 || drafts > 0, nil
}

// LostBetween returns the proyectos lost (and still lost) at a time in [from, to), in
// the order they were lost.
func (s *Store) LostBetween(ctx context.Context, from, to time.Time) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+projectSelectCols+projectFrom+`
		WHERE p.status = 'perdido' AND p.lost_at >= ? AND p.lost_at < ?
		ORDER BY p.lost_at, p.id`, jobTime(from), jobTime(to))
	if err != nil {
		return nil, fmt.Errorf("store: projects lost: %w", err)
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, fmt.Errorf("store: projects lost: %w", err)
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}
