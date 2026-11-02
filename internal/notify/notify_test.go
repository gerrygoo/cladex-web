package notify

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	cladex "github.com/gerrygoo/cladex-web"
	"github.com/gerrygoo/cladex-web/internal/jobs"
	"github.com/gerrygoo/cladex-web/internal/store"
)

type fixture struct {
	t     *testing.T
	store *store.Store
	fake  *Fake
	svc   *Service
	now   time.Time
}

// newFixture is a Service whose email goes to a Fake, in production or, with allow set,
// outside it.
func newFixture(t *testing.T, production bool, allow ...string) *fixture {
	t.Helper()
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"), cladex.MigrationsFS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	f := &fixture{t: t, store: s, fake: NewFake(), now: time.Now()}
	cfg := &Config{
		senders: map[string]Sender{store.ChannelEmail: f.fake},
		Status: Status{Production: production, Allow: allow, Channels: []ChannelStatus{
			{Channel: store.ChannelEmail, Enabled: true, Provider: "fake"},
			{Channel: store.ChannelWhatsApp, Problem: "todavía no está disponible"},
		}},
	}
	f.svc = New(s, cfg, nil)
	f.svc.now = func() time.Time { return f.now }
	return f
}

func (f *fixture) queue(channel, address string) int64 {
	f.t.Helper()
	subject, text, html := Prueba("Ana")
	id, err := f.store.QueueNotification(context.Background(), store.NewNotification{
		Kind: PruebaKind, Channel: channel, Address: address, Subject: subject, BodyText: text, BodyHTML: html,
	}, f.now)
	if err != nil {
		f.t.Fatal(err)
	}
	return id
}

// run claims the next notification job, runs it through Handle and records its outcome
// the way the runner does.
func (f *fixture) run() (string, error) {
	f.t.Helper()
	ctx := context.Background()
	job, err := f.store.ClaimJob(ctx, []string{store.NotificationJobKind}, f.now, time.Minute)
	if err != nil || job == nil {
		f.t.Fatalf("claim: %v %v", job, err)
	}
	result, err := f.svc.Handle(ctx, *job)
	if err != nil {
		if ferr := f.store.FailJob(ctx, job.ID, err.Error(), jobs.IsPermanent(err), f.now, f.now); ferr != nil {
			f.t.Fatal(ferr)
		}
		return result, err
	}
	if err := f.store.CompleteJob(ctx, job.ID, result, f.now); err != nil {
		f.t.Fatal(err)
	}
	return result, nil
}

func (f *fixture) notification(id int64) store.Notification {
	f.t.Helper()
	n, err := f.store.NotificationByID(context.Background(), id)
	if err != nil || n == nil {
		f.t.Fatalf("notification %d: %v %v", id, n, err)
	}
	return *n
}

func events(n store.Notification) []string {
	var out []string
	for _, e := range n.Events {
		out = append(out, e.Event)
	}
	return out
}

func TestSendAccepted(t *testing.T) {
	f := newFixture(t, true)
	id := f.queue(store.ChannelEmail, "ana@example.com")

	result, err := f.run()
	if err != nil || result != "Enviada por fake." {
		t.Fatalf("run = %q, %v", result, err)
	}
	n := f.notification(id)
	if n.Status != store.NotifAceptada || n.Provider != "fake" || n.ProviderMessageID != "fake-1" || n.SentAt == "" {
		t.Errorf("notification: %+v", n)
	}
	if got := events(n); len(got) != 3 || got[0] != "en_cola" || got[1] != "enviando" || got[2] != "aceptada" {
		t.Errorf("events = %v", got)
	}
	sent := f.fake.Sent()
	if len(sent) != 1 || sent[0].To != "ana@example.com" || sent[0].Subject != "Prueba de avisos de Cladex" || sent[0].IdempotencyKey == "" {
		t.Errorf("sent = %+v", sent)
	}

	// Its job running again (at least once) doesn't send it twice, and neither does a job
	// that isn't the notification's own.
	n = f.notification(id)
	job, err := f.store.JobByID(context.Background(), n.JobID)
	if err != nil || job == nil {
		t.Fatal(err)
	}
	if result, err := f.svc.Handle(context.Background(), *job); err != nil || result != "La notificación 1 ya estaba aceptada." {
		t.Errorf("same job again = %q, %v", result, err)
	}
	if _, err := f.store.EnqueueJob(context.Background(), store.NewJob{Kind: store.NotificationJobKind, Payload: `{"id":1}`}, f.now); err != nil {
		t.Fatal(err)
	}
	if result, err := f.run(); err != nil || result != "La notificación 1 ya la envía el trabajo 1." {
		t.Errorf("stale job = %q, %v", result, err)
	}
	if len(f.fake.Sent()) != 1 {
		t.Errorf("sent %d times", len(f.fake.Sent()))
	}
}

func TestSendTemporaryErrorRetriesThenFails(t *testing.T) {
	f := newFixture(t, true)
	id := f.queue(store.ChannelEmail, "ana@example.com")
	f.fake.Err = errors.New("resend 500: oops")

	// While attempts remain, the notification waits por enviar with the error.
	if _, err := f.run(); err == nil || jobs.IsPermanent(err) {
		t.Fatalf("first attempt: %v", err)
	}
	if n := f.notification(id); n.Status != store.NotifPorEnviar || n.Error != "resend 500: oops" {
		t.Errorf("after a temporary error: %+v", n)
	}
	// A retry that works sends it.
	f.fake.Err = nil
	if _, err := f.run(); err != nil {
		t.Fatal(err)
	}
	if n := f.notification(id); n.Status != store.NotifAceptada || n.Error != "" {
		t.Errorf("after the retry: %+v", n)
	}

	// The last attempt's error is final.
	id = f.queue(store.ChannelEmail, "beto@example.com")
	f.fake.Err = errors.New("timeout")
	for range 5 {
		f.run()
	}
	if n := f.notification(id); n.Status != store.NotifFallida || n.Error != "timeout" {
		t.Errorf("out of attempts: %+v", n)
	}
}

func TestSendPermanentErrorFailsAtOnce(t *testing.T) {
	f := newFixture(t, true)
	id := f.queue(store.ChannelEmail, "ana@example.com")
	f.fake.Err = jobs.Permanent(errors.New("resend 422 validation_error: Invalid to field."))
	if _, err := f.run(); !jobs.IsPermanent(err) {
		t.Fatalf("run: %v", err)
	}
	n := f.notification(id)
	if n.Status != store.NotifFallida || n.Error != "resend 422 validation_error: Invalid to field." {
		t.Errorf("notification: %+v", n)
	}

	// Retried by hand, it gets a new job and is sent.
	f.fake.Err = nil
	if err := f.store.RetryNotification(context.Background(), id, 0, f.now); err != nil {
		t.Fatal(err)
	}
	if _, err := f.run(); err != nil {
		t.Fatal(err)
	}
	if n := f.notification(id); n.Status != store.NotifAceptada || n.JobID == 0 {
		t.Errorf("after retrying by hand: %+v", n)
	}

	// A channel that's off fails for good, saying why.
	id = f.queue(store.ChannelWhatsApp, "+523312345678")
	if _, err := f.run(); !jobs.IsPermanent(err) {
		t.Fatalf("whatsapp: %v", err)
	}
	if n := f.notification(id); n.Status != store.NotifFallida || n.Error != "el canal whatsapp está apagado: todavía no está disponible" {
		t.Errorf("whatsapp: %+v", n)
	}
}

// Outside production only NOTIFY_ALLOW gets anything; the rest is cancelled, saying why.
func TestStagingOnlySendsToAllowlist(t *testing.T) {
	f := newFixture(t, false, "gerry@example.com")
	allowed := f.queue(store.ChannelEmail, "gerry@example.com")
	other := f.queue(store.ChannelEmail, "cliente@example.com")

	f.run()
	result, err := f.run()
	if err != nil || result != "Fuera de producción solo se envía a NOTIFY_ALLOW; cliente@example.com no está." {
		t.Errorf("run = %q, %v", result, err)
	}
	if n := f.notification(allowed); n.Status != store.NotifAceptada {
		t.Errorf("allowed: %+v", n)
	}
	if n := f.notification(other); n.Status != store.NotifCancelada || n.Error == "" {
		t.Errorf("other: %+v", n)
	}
	if sent := f.fake.Sent(); len(sent) != 1 || sent[0].To != "gerry@example.com" {
		t.Errorf("sent = %+v", sent)
	}
}

func TestCancelNotification(t *testing.T) {
	f := newFixture(t, true)
	ctx := context.Background()
	id := f.queue(store.ChannelEmail, "ana@example.com")
	if err := f.store.CancelNotification(ctx, id, 0, "ya no aplica", f.now); err != nil {
		t.Fatal(err)
	}
	n := f.notification(id)
	if n.Status != store.NotifCancelada {
		t.Errorf("notification: %+v", n)
	}
	if j, _ := f.store.JobByID(ctx, n.JobID); j == nil || j.Status != store.JobCancelado {
		t.Errorf("its job: %+v", j)
	}
	// Only por enviar can be cancelled, only fallida retried.
	if err := f.store.CancelNotification(ctx, id, 0, "", f.now); !errors.Is(err, store.ErrNotificationState) {
		t.Errorf("cancel twice: %v", err)
	}
	if err := f.store.RetryNotification(ctx, id, 0, f.now); !errors.Is(err, store.ErrNotificationState) {
		t.Errorf("retry a cancelled one: %v", err)
	}
}

// The whole path through a running Runner: queued, woken, sent.
func TestRunnerSendsNotification(t *testing.T) {
	f := newFixture(t, true)
	r := jobs.New(f.store, nil)
	r.Register(store.NotificationJobKind, f.svc.Handle)
	f.svc.now = time.Now
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.Run(ctx)

	id := f.queue(store.ChannelEmail, "ana@example.com")
	r.Wake()
	deadline := time.Now().Add(5 * time.Second)
	for f.notification(id).Status != store.NotifAceptada {
		if time.Now().After(deadline) {
			t.Fatalf("not sent: %+v", f.notification(id))
		}
		time.Sleep(20 * time.Millisecond)
	}
}
