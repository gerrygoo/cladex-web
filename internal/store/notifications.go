package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Notification statuses (migrations/0036_notifications.sql).
const (
	NotifPorEnviar = "por_enviar"
	NotifEnviando  = "enviando"
	NotifAceptada  = "aceptada"
	NotifEntregada = "entregada"
	NotifFallida   = "fallida"
	NotifCancelada = "cancelada"

	ChannelEmail    = "email"
	ChannelWhatsApp = "whatsapp"

	// NotificationJobKind is the job that sends a notification; its payload is the id.
	NotificationJobKind = "notificacion"
)

// NotificationStatusLabels are the words admins see for each status.
var NotificationStatusLabels = map[string]string{
	NotifPorEnviar: "Por enviar",
	NotifEnviando:  "Enviando",
	NotifAceptada:  "Aceptada",
	NotifEntregada: "Entregada",
	NotifFallida:   "Fallida",
	NotifCancelada: "Cancelada",
}

// ChannelLabels name each channel.
var ChannelLabels = map[string]string{
	ChannelEmail:    "Correo",
	ChannelWhatsApp: "WhatsApp",
}

// ErrNotificationState is returned when a notification isn't in a state for the action.
var ErrNotificationState = errors.New("store: notification is not in a state for that")

// NewNotification is a notification to queue. Address is the email or phone to send to,
// copied as it is now.
type NewNotification struct {
	Kind       string
	Channel    string
	UserID     int64
	CustomerID int64
	Address    string
	ProjectID  int64
	Subject    string
	BodyText   string
	BodyHTML   string
	CreatedBy  int64
}

// Notification is a notifications row, with its recipient's name and, when read alone,
// its events.
type Notification struct {
	ID                int64
	Kind              string
	Channel           string
	UserID            int64
	UserName          string
	CustomerID        int64
	Address           string
	ProjectID         int64
	Subject           string
	BodyText          string
	BodyHTML          string
	Status            string
	Provider          string
	ProviderMessageID string
	Error             string
	JobID             int64
	CreatedAt         string
	SentAt            string
	Events            []NotificationEvent
}

// NotificationEvent is one entry of a notification's history.
type NotificationEvent struct {
	Event     string
	Detail    string
	UserName  string
	CreatedAt string
}

const notificationCols = `n.id, n.kind, n.channel, COALESCE(n.user_id, 0), COALESCE(u.name, ''),
	COALESCE(n.customer_id, 0), n.address, COALESCE(n.project_id, 0), COALESCE(n.subject, ''),
	COALESCE(n.body_text, ''), COALESCE(n.body_html, ''), n.status, COALESCE(n.provider, ''),
	COALESCE(n.provider_message_id, ''), COALESCE(n.error, ''), COALESCE(n.job_id, 0),
	n.created_at, COALESCE(n.sent_at, '')`

const notificationFrom = ` FROM notifications n LEFT JOIN users u ON u.id = n.user_id`

func scanNotification(row interface{ Scan(...any) error }) (Notification, error) {
	var n Notification
	err := row.Scan(&n.ID, &n.Kind, &n.Channel, &n.UserID, &n.UserName, &n.CustomerID, &n.Address,
		&n.ProjectID, &n.Subject, &n.BodyText, &n.BodyHTML, &n.Status, &n.Provider,
		&n.ProviderMessageID, &n.Error, &n.JobID, &n.CreatedAt, &n.SentAt)
	return n, err
}

// QueueNotification writes n and the job that sends it, in one transaction.
func (s *Store) QueueNotification(ctx context.Context, n NewNotification, now time.Time) (int64, error) {
	var id int64
	err := s.inTx(ctx, "queue notification", func(tx *sql.Tx) error {
		var err error
		id, err = queueNotification(ctx, tx, n, now)
		return err
	})
	return id, err
}

// queueNotification is QueueNotification inside the caller's transaction, so a
// notification is written together with whatever caused it.
func queueNotification(ctx context.Context, tx *sql.Tx, n NewNotification, now time.Time) (int64, error) {
	res, err := tx.ExecContext(ctx, `
		INSERT INTO notifications (kind, channel, user_id, customer_id, address, project_id,
			subject, body_text, body_html, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.Kind, n.Channel, nullIfZero(n.UserID), nullIfZero(n.CustomerID), n.Address,
		nullIfZero(n.ProjectID), nullIfEmpty(n.Subject), nullIfEmpty(n.BodyText),
		nullIfEmpty(n.BodyHTML), nullIfZero(n.CreatedBy), jobTime(now))
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := addNotificationEvent(ctx, tx, id, n.CreatedBy, "en_cola", "", now); err != nil {
		return 0, err
	}
	return id, enqueueSend(ctx, tx, id, now)
}

// enqueueSend queues the job that sends notification id, now, and points the
// notification at it. The notification's job_id is its one current job: a job it no
// longer points at (one still finishing when the notification was retried by hand) does
// nothing, so two jobs never send it at once.
func enqueueSend(ctx context.Context, tx *sql.Tx, id int64, now time.Time) error {
	jobID, err := enqueueJob(ctx, tx, NewJob{
		Kind:    NotificationJobKind,
		Payload: fmt.Sprintf(`{"id":%d}`, id),
	}, now)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE notifications SET job_id = ? WHERE id = ?`, jobID, id)
	return err
}

func addNotificationEvent(ctx context.Context, tx *sql.Tx, id, userID int64, event, detail string, now time.Time) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO notification_events (notification_id, event, detail, user_id, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		id, event, nullIfEmpty(detail), nullIfZero(userID), jobTime(now))
	return err
}

// NotificationByID reads one notification with its events, or nil when there's none.
func (s *Store) NotificationByID(ctx context.Context, id int64) (*Notification, error) {
	n, err := scanNotification(s.db.QueryRowContext(ctx, `SELECT `+notificationCols+notificationFrom+` WHERE n.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: notification: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.event, COALESCE(e.detail, ''), COALESCE(u.name, ''), e.created_at
		FROM notification_events e LEFT JOIN users u ON u.id = e.user_id
		WHERE e.notification_id = ? ORDER BY e.id`, id)
	if err != nil {
		return nil, fmt.Errorf("store: notification events: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var e NotificationEvent
		if err := rows.Scan(&e.Event, &e.Detail, &e.UserName, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: notification events: %w", err)
		}
		n.Events = append(n.Events, e)
	}
	return &n, rows.Err()
}

// NotificationFilter narrows ListNotifications; an empty field matches everything.
type NotificationFilter struct {
	Status  string
	Channel string
	Kind    string
}

// ListNotifications reads notifications newest first.
func (s *Store) ListNotifications(ctx context.Context, f NotificationFilter) ([]Notification, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+notificationCols+notificationFrom+`
		WHERE (? = '' OR n.status = ?) AND (? = '' OR n.channel = ?) AND (? = '' OR n.kind = ?)
		ORDER BY n.id DESC`,
		f.Status, f.Status, f.Channel, f.Channel, f.Kind, f.Kind)
	if err != nil {
		return nil, fmt.Errorf("store: list notifications: %w", err)
	}
	defer rows.Close()
	var out []Notification
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list notifications: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// NotificationKinds lists the kinds there are notifications of, for the list's filter.
func (s *Store) NotificationKinds(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT kind FROM notifications ORDER BY kind`)
	if err != nil {
		return nil, fmt.Errorf("store: notification kinds: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// notificationTransition moves notification id from one of from to the status set by set
// (an UPDATE's SET list and its args) and records event. It returns ErrNotificationState
// when the notification wasn't in one of from.
func (s *Store) notificationTransition(ctx context.Context, what string, id, userID int64, from []string,
	event, detail string, now time.Time, set string, args ...any) error {
	return s.inTx(ctx, what, func(tx *sql.Tx) error {
		return transitionNotification(ctx, tx, id, userID, from, event, detail, now, set, args...)
	})
}

func transitionNotification(ctx context.Context, tx *sql.Tx, id, userID int64, from []string,
	event, detail string, now time.Time, set string, args ...any) error {
	q := `UPDATE notifications SET ` + set + ` WHERE id = ? AND status IN (?` + strings.Repeat(", ?", len(from)-1) + `)`
	args = append(args, id)
	for _, f := range from {
		args = append(args, f)
	}
	res, err := tx.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("notification %d: %w", id, ErrNotificationState)
	}
	return addNotificationEvent(ctx, tx, id, userID, event, detail, now)
}

// StartSending marks a notification enviando before its send is attempted. A
// notification found enviando already was cut off mid-send by a dead process, and is
// sent again.
func (s *Store) StartSending(ctx context.Context, id int64, provider, detail string, now time.Time) error {
	return s.notificationTransition(ctx, "start sending", id, 0, []string{NotifPorEnviar, NotifEnviando},
		"enviando", detail, now, `status = 'enviando', provider = ?`, provider)
}

// MarkNotificationAccepted records that the provider took the message, with its id.
func (s *Store) MarkNotificationAccepted(ctx context.Context, id int64, providerMessageID string, now time.Time) error {
	return s.notificationTransition(ctx, "accept notification", id, 0, []string{NotifEnviando},
		"aceptada", providerMessageID, now,
		`status = 'aceptada', provider_message_id = ?, error = NULL, sent_at = ?`,
		nullIfEmpty(providerMessageID), jobTime(now))
}

// MarkNotificationError records a failed send. A final one is fallida; otherwise the
// notification waits por enviar for its job's retry.
func (s *Store) MarkNotificationError(ctx context.Context, id int64, errMsg string, final bool, now time.Time) error {
	status, event := NotifPorEnviar, "error"
	if final {
		status, event = NotifFallida, "fallida"
	}
	return s.notificationTransition(ctx, "notification error", id, 0, []string{NotifPorEnviar, NotifEnviando},
		event, errMsg, now, `status = ?, error = ?`, status, errMsg)
}

// CancelNotification cancels a notification still por enviar, and the job that would
// send it. userID is 0 when the app cancels it (as the staging guard does).
func (s *Store) CancelNotification(ctx context.Context, id, userID int64, reason string, now time.Time) error {
	return s.inTx(ctx, "cancel notification", func(tx *sql.Tx) error {
		if err := transitionNotification(ctx, tx, id, userID, []string{NotifPorEnviar}, "cancelada", reason, now,
			`status = 'cancelada', error = ?`, nullIfEmpty(reason)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			UPDATE jobs SET status = 'cancelado', result = 'Se canceló la notificación.', finished_at = ?
			WHERE id = (SELECT job_id FROM notifications WHERE id = ?) AND status = 'pendiente'`,
			jobTime(now), id)
		return err
	})
}

// RetryNotification puts a fallida notification back por enviar with a new job, now.
func (s *Store) RetryNotification(ctx context.Context, id, userID int64, now time.Time) error {
	return s.inTx(ctx, "retry notification", func(tx *sql.Tx) error {
		if err := transitionNotification(ctx, tx, id, userID, []string{NotifFallida}, "reintento", "", now,
			`status = 'por_enviar', error = NULL`); err != nil {
			return err
		}
		return enqueueSend(ctx, tx, id, now)
	})
}

// UserContact is where a user wants to be told things.
type UserContact struct {
	Email          string
	WhatsApp       string // E.164, "+52…"
	NotifyEmail    bool
	NotifyWhatsApp bool
}

// UserContactByID reads a user's contact data.
func (s *Store) UserContactByID(ctx context.Context, id int64) (UserContact, error) {
	var c UserContact
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(email, ''), COALESCE(whatsapp, ''), notify_email, notify_whatsapp
		FROM users WHERE id = ?`, id).Scan(&c.Email, &c.WhatsApp, &c.NotifyEmail, &c.NotifyWhatsApp)
	if err != nil {
		return c, fmt.Errorf("store: user contact %d: %w", id, err)
	}
	return c, nil
}

// SetUserContact saves a user's contact data. A channel without its address is off.
func (s *Store) SetUserContact(ctx context.Context, id int64, c UserContact) error {
	return s.inTx(ctx, "set user contact", func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			UPDATE users SET email = ?, whatsapp = ?, notify_email = ?, notify_whatsapp = ? WHERE id = ?`,
			nullIfEmpty(c.Email), nullIfEmpty(c.WhatsApp), c.NotifyEmail && c.Email != "",
			c.NotifyWhatsApp && c.WhatsApp != "", id)
		return err
	})
}
