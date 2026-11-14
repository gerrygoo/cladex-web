package avisos

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gerrygoo/cladex-web/internal/jobs"
	"github.com/gerrygoo/cladex-web/internal/notify"
	"github.com/gerrygoo/cladex-web/internal/store"
)

// The avisos of a client O.C. received (#69 C and F). store.ReceiveOC queues an
// OCJobKind job in the transaction that takes the proyecto to Orden colocada 100%. That
// job emails everyone with oc_recibida on, at once, and queues the F reminders: a
// SupplierJobKind job at each of supplierSlots. Each reminder goes out only while the
// proyecto is still en curso from that same receipt and its supplier O.C. is still to be
// confirmed (store.SupplierOCPending), to everyone with oc_proveedor_pendiente on.

// OCJobKind is store.AvisoOCJobKind, the job that starts them.
const OCJobKind = store.AvisoOCJobKind

// SupplierJobKind is one F reminder.
const SupplierJobKind = "avisos_oc_proveedor"

// supplierSlots are the F reminders for a client O.C. received at t: one hour after it,
// two hours after that and at 18:00; received from 15:00 on, one and two hours after it.
// One received on a weekend, from 18:00 on or before 8:00 counts from 8:00 of the next
// weekday (or of that same day, before 8:00).
func supplierSlots(t time.Time, loc *time.Location) []time.Time {
	t = t.In(loc)
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
	start := t
	switch {
	case !weekday(day) || t.Hour() >= 18:
		day = day.AddDate(0, 0, 1)
		for !weekday(day) {
			day = day.AddDate(0, 0, 1)
		}
		start = day.Add(8 * time.Hour)
	case t.Hour() < 8:
		start = day.Add(8 * time.Hour)
	}
	if start.Hour() >= 15 {
		return []time.Time{start.Add(time.Hour), start.Add(2 * time.Hour)}
	}
	return []time.Time{start.Add(time.Hour), start.Add(3 * time.Hour), day.Add(18 * time.Hour)}
}

type ocPayload struct {
	ProjectID  int64  `json:"project_id"`
	ReceivedAt string `json:"received_at,omitempty"`
	Step       int    `json:"step,omitempty"`
}

// HandleOC is the OCJobKind job: aviso C, and the F reminders queued.
func (s *Service) HandleOC(ctx context.Context, job store.Job) (string, error) {
	var p ocPayload
	if err := json.Unmarshal([]byte(job.Payload), &p); err != nil || p.ProjectID == 0 {
		return "", jobs.Permanent(fmt.Errorf("payload %q: %v", job.Payload, err))
	}
	project, received, ok, err := s.receivedProject(ctx, p.ProjectID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "No se envió: el proyecto ya no está en curso.", nil
	}
	at := received.UTC().Format(time.RFC3339Nano)
	for i, slot := range supplierSlots(received, s.loc) {
		payload, _ := json.Marshal(ocPayload{ProjectID: p.ProjectID, ReceivedAt: at, Step: i + 1})
		if _, err := s.jobs.Enqueue(ctx, store.NewJob{Kind: SupplierJobKind, Payload: string(payload),
			DedupeKey: fmt.Sprintf("%d %s %d", p.ProjectID, at, i+1), RunAt: slot}); err != nil {
			return "", err
		}
	}

	recipients, err := s.store.AvisoRecipients(ctx, store.AvisoOCRecibida)
	if err != nil {
		return "", err
	}
	var ns []store.NewNotification
	for _, r := range recipients {
		subject, text, html := s.ocMessage(r, *project)
		ns = append(ns, emailTo(store.AvisoOCRecibida, r, *project, subject, text, html)...)
	}
	done, err := s.store.QueueAvisoRound(ctx, fmt.Sprintf("oc_recibida %d %s", p.ProjectID, at), ns, s.now())
	if err != nil {
		return "", err
	}
	if done {
		return "Este aviso ya se había enviado.", nil
	}
	return fmt.Sprintf("%d avisos en cola; recordatorios de la O.C. al proveedor programados.", len(ns)), nil
}

// HandleSupplier is one F reminder.
func (s *Service) HandleSupplier(ctx context.Context, job store.Job) (string, error) {
	var p ocPayload
	if err := json.Unmarshal([]byte(job.Payload), &p); err != nil || p.ProjectID == 0 || p.Step == 0 {
		return "", jobs.Permanent(fmt.Errorf("payload %q: %v", job.Payload, err))
	}
	want, err := time.Parse(time.RFC3339Nano, p.ReceivedAt)
	if err != nil {
		return "", jobs.Permanent(fmt.Errorf("payload: %w", err))
	}
	if runAt, err := time.Parse(time.RFC3339Nano, job.RunAt); err == nil {
		if late := s.now().Sub(runAt); late > lateLimit {
			return fmt.Sprintf("No se envió: el recordatorio corrió %s tarde.", late.Round(time.Minute)), nil
		}
	}
	project, received, ok, err := s.receivedProject(ctx, p.ProjectID)
	if err != nil {
		return "", err
	}
	// A receipt undone (Deshacer O.C. recibida) and done again starts its own reminders.
	if !ok || !received.Equal(want) {
		return "No se envió: el proyecto ya no está en curso con esa orden de compra.", nil
	}
	pending, err := s.store.SupplierOCPending(ctx, p.ProjectID)
	if err != nil {
		return "", err
	}
	if !pending {
		return "No se envió: la orden de compra al proveedor ya está confirmada.", nil
	}
	recipients, err := s.store.AvisoRecipients(ctx, store.AvisoOCProveedor)
	if err != nil {
		return "", err
	}
	var ns []store.NewNotification
	for _, r := range recipients {
		subject, text, html := s.supplierMessage(r, *project, received)
		ns = append(ns, emailTo(store.AvisoOCProveedor, r, *project, subject, text, html)...)
	}
	done, err := s.store.QueueAvisoRound(ctx, fmt.Sprintf("oc_proveedor %d %s %d", p.ProjectID, p.ReceivedAt, p.Step), ns, s.now())
	if err != nil {
		return "", err
	}
	if done {
		return "Este recordatorio ya se había enviado.", nil
	}
	return fmt.Sprintf("%d avisos en cola.", len(ns)), nil
}

// receivedProject is the proyecto and when its client O.C. was received, and false when
// it no longer exists or isn't en curso.
func (s *Service) receivedProject(ctx context.Context, id int64) (*store.Project, time.Time, bool, error) {
	project, err := s.store.ProjectByID(ctx, id)
	if err != nil || project == nil || project.Status != store.StageEnCurso {
		return nil, time.Time{}, false, err
	}
	received, err := s.store.OCReceivedAt(ctx, id)
	if err != nil || received.IsZero() {
		return nil, time.Time{}, false, err
	}
	return project, received, true, nil
}

// ocSummary is the proyecto, its client O.C. and its money, for C and F.
func (s *Service) ocSummary(p store.Project) []string {
	oc := "O.C. del cliente " + p.OC.Number
	if t, err := time.Parse("2006-01-02", p.OC.Date); err == nil {
		oc += " del " + day(t)
	}
	return []string{
		fmt.Sprintf("Proyecto %s · cliente %s · vendedor %s · %s.", p.Label(), p.CustomerName, p.UserName, oc),
		fmt.Sprintf("Total: %s. Antes de IVA: venta %s, costo %s, utilidad %s, margen %.1f%%.",
			p.Total, p.Subtotal, p.Cost(), p.Profit, p.MarginPercent()),
	}
}

func (s *Service) ocMessage(r store.AvisoRecipient, p store.Project) (subject, text, html string) {
	paras := []string{"Hola, " + r.Name + ".",
		fmt.Sprintf("Proyecto %s de %s: orden colocada al 100%%.", p.Label(), p.CustomerName)}
	paras = append(paras, s.ocSummary(p)...)
	paras = append(paras, "Siguiente paso: generar y mandar la orden de compra al proveedor.", s.link(p), footer)
	text, html = notify.Email(paras...)
	return fmt.Sprintf("Orden colocada 100%%: %s · %s", p.Label(), p.CustomerName), text, html
}

func (s *Service) supplierMessage(r store.AvisoRecipient, p store.Project, received time.Time) (subject, text, html string) {
	paras := []string{"Hola, " + r.Name + ".",
		fmt.Sprintf("Proyecto %s de %s: orden colocada al 100%% (recibida el %s a las %s), orden de compra al proveedor sin enviar.",
			p.Label(), p.CustomerName, day(received.In(s.loc)), received.In(s.loc).Format("15:04"))}
	paras = append(paras, s.ocSummary(p)...)
	paras = append(paras,
		"Siguiente paso: confirmar y descargar la orden de compra al proveedor y mandársela. En cuanto esté confirmada, dejan de llegar estos avisos.",
		s.link(p), footer)
	text, html = notify.Email(paras...)
	return fmt.Sprintf("O.C. al proveedor sin enviar: %s · %s", p.Label(), strings.TrimSpace(p.CustomerName)), text, html
}
