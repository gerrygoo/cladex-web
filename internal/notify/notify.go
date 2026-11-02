// Package notify sends the app's notifications (#18; docs/planning/notificaciones-y-trabajos.md
// §2). A notification is a row the store writes together with the job that sends it;
// Service.Handle is that job, and a Sender per channel does the sending.
//
// Email goes through Resend or an SMTP mailbox (EMAIL_PROVIDER), WhatsApp comes in N4.
// A channel without its configuration is off, not fatal: Status says why, in the startup
// log and in Configuración del sistema.
package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gerrygoo/cladex-web/internal/jobs"
	"github.com/gerrygoo/cladex-web/internal/store"
)

// Message is one message to send on a channel.
type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
	// IdempotencyKey is the same on every retry of one send job, so a provider that
	// supports it doesn't send twice when we didn't record its answer.
	IdempotencyKey string
}

// Sender sends on one channel through one provider.
type Sender interface {
	// Provider names it on the notification: "resend", "smtp", "fake".
	Provider() string
	// Send returns the provider's id for the message. An error a retry can't fix is
	// wrapped with jobs.Permanent.
	Send(ctx context.Context, m Message) (providerID string, err error)
}

// Service is the notification job's handler and what it needs.
type Service struct {
	store   *store.Store
	logger  *slog.Logger
	senders map[string]Sender // by channel; a missing one is off
	status  Status
	now     func() time.Time
}

func New(s *store.Store, cfg *Config, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Service{store: s, logger: logger, senders: cfg.senders, status: cfg.Status, now: time.Now}
}

// Handle is the store.NotificationJobKind job: it sends one notification. It re-reads
// the notification and does nothing when it is no longer por enviar, so running twice is
// harmless.
func (s *Service) Handle(ctx context.Context, job store.Job) (string, error) {
	var p struct{ ID int64 }
	if err := json.Unmarshal([]byte(job.Payload), &p); err != nil {
		return "", jobs.Permanent(fmt.Errorf("payload: %w", err))
	}
	n, err := s.store.NotificationByID(ctx, p.ID)
	if err != nil {
		return "", err
	}
	if n == nil {
		return "", jobs.Permanent(fmt.Errorf("no existe la notificación %d", p.ID))
	}
	if n.JobID != job.ID {
		return fmt.Sprintf("La notificación %d ya la envía el trabajo %d.", n.ID, n.JobID), nil
	}
	if n.Status != store.NotifPorEnviar && n.Status != store.NotifEnviando {
		return fmt.Sprintf("La notificación %d ya estaba %s.", n.ID, strings.ToLower(store.NotificationStatusLabels[n.Status])), nil
	}
	if !s.status.Allowed(n.Address) {
		reason := fmt.Sprintf("Fuera de producción solo se envía a NOTIFY_ALLOW; %s no está.", n.Address)
		if err := s.store.CancelNotification(ctx, n.ID, 0, reason, s.now()); err != nil {
			return "", err
		}
		return reason, nil
	}
	sender := s.senders[n.Channel]
	if sender == nil {
		err := jobs.Permanent(fmt.Errorf("el canal %s está apagado: %s",
			strings.ToLower(store.ChannelLabels[n.Channel]), s.status.Channel(n.Channel).Problem))
		return "", s.recordError(ctx, n.ID, err, true)
	}

	if err := s.store.StartSending(ctx, n.ID, sender.Provider(),
		fmt.Sprintf("Intento %d de %d por %s.", job.Attempts, job.MaxAttempts, sender.Provider()), s.now()); err != nil {
		return "", err
	}
	id, err := sender.Send(ctx, Message{
		To: n.Address, Subject: n.Subject, Text: n.BodyText, HTML: n.BodyHTML,
		IdempotencyKey: fmt.Sprintf("cladex-notificacion-%d-trabajo-%d", n.ID, job.ID),
	})
	if err != nil {
		return "", s.recordError(ctx, n.ID, err, jobs.IsPermanent(err) || job.Attempts >= job.MaxAttempts)
	}
	if err := s.store.MarkNotificationAccepted(ctx, n.ID, id, s.now()); err != nil {
		// Sent, but not recorded: a retry would send it again. Say so loudly.
		s.logger.Error("notify: sent but not recorded", slog.Int64("notification", n.ID), slog.String("provider_id", id), slog.Any("err", err))
		return "", jobs.Permanent(fmt.Errorf("se envió (id %s) pero no se pudo registrar: %w", id, err))
	}
	return fmt.Sprintf("Enviada por %s.", sender.Provider()), nil
}

// recordError writes a failed send on the notification and returns the job's error.
func (s *Service) recordError(ctx context.Context, id int64, sendErr error, final bool) error {
	if err := s.store.MarkNotificationError(ctx, id, sendErr.Error(), final, s.now()); err != nil {
		return errors.Join(sendErr, err)
	}
	return sendErr
}
