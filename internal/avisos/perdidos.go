package avisos

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gerrygoo/cladex-web/internal/jobs"
	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/notify"
	"github.com/gerrygoo/cladex-web/internal/store"
)

// The monthly summary of lost proyectos (#69 D): on the first weekday of each month at
// 9:00, everyone with resumen_perdidos on gets the proyectos lost in the month before
// (and still lost), each with its client, vendedor, value before IVA and motivo. A month
// with none says so. One job per month, each queueing the next; one that runs more than
// perdidosLateLimit late sends nothing.

// PerdidosJobKind is the monthly summary's job; its payload names the month it is sent in.
const PerdidosJobKind = "avisos_resumen_perdidos"

const perdidosLateLimit = 3 * 24 * time.Hour

const monthLayout = "2006-01"

var monthNames = [...]string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio",
	"agosto", "septiembre", "octubre", "noviembre", "diciembre"}

func monthName(t time.Time) string {
	return fmt.Sprintf("%s de %d", monthNames[t.Month()-1], t.Year())
}

// perdidosSlot is when the summary of the month before month goes out: its first weekday,
// at 9:00.
func perdidosSlot(month time.Time, loc *time.Location) time.Time {
	day := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, loc)
	for !weekday(day) {
		day = day.AddDate(0, 0, 1)
	}
	return day.Add(9 * time.Hour)
}

// SchedulePerdidos queues the summary due next: this month's, unless it is more than
// perdidosLateLimit past, then next month's. A month already sent sends nothing twice.
func (s *Service) SchedulePerdidos(ctx context.Context) error {
	now := s.now().In(s.loc)
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, s.loc)
	if now.Sub(perdidosSlot(month, s.loc)) > perdidosLateLimit {
		month = month.AddDate(0, 1, 0)
	}
	return s.schedulePerdidos(ctx, month)
}

func (s *Service) schedulePerdidos(ctx context.Context, month time.Time) error {
	key := month.Format(monthLayout)
	_, err := s.jobs.Enqueue(ctx, store.NewJob{
		Kind:      PerdidosJobKind,
		Payload:   fmt.Sprintf(`{"month":%q}`, key),
		DedupeKey: key,
		RunAt:     perdidosSlot(month, s.loc),
	})
	return err
}

// HandlePerdidos is a month's summary. It queues next month's first.
func (s *Service) HandlePerdidos(ctx context.Context, job store.Job) (string, error) {
	var p struct{ Month string }
	if err := json.Unmarshal([]byte(job.Payload), &p); err != nil {
		return "", jobs.Permanent(fmt.Errorf("payload: %w", err))
	}
	month, err := time.ParseInLocation(monthLayout, p.Month, s.loc)
	if err != nil {
		return "", jobs.Permanent(fmt.Errorf("payload: %w", err))
	}
	if err := s.schedulePerdidos(ctx, month.AddDate(0, 1, 0)); err != nil {
		return "", err
	}
	if late := s.now().Sub(perdidosSlot(month, s.loc)); late > perdidosLateLimit {
		return fmt.Sprintf("No se envió: el resumen corrió %s tarde.", late.Round(time.Hour)), nil
	}

	from := month.AddDate(0, -1, 0)
	lost, err := s.store.LostBetween(ctx, from, month)
	if err != nil {
		return "", err
	}
	recipients, err := s.store.AvisoRecipients(ctx, store.AvisoResumenPerdidos)
	if err != nil {
		return "", err
	}
	var ns []store.NewNotification
	for _, r := range recipients {
		if !r.NotifyEmail || r.Email == "" {
			continue
		}
		subject, text, html := s.perdidosMessage(r, from, lost)
		ns = append(ns, store.NewNotification{Kind: store.AvisoResumenPerdidos, Channel: store.ChannelEmail,
			UserID: r.UserID, Address: r.Email, Subject: subject, BodyText: text, BodyHTML: html})
	}
	done, err := s.store.QueueAvisoRound(ctx, "perdidos "+from.Format(monthLayout), ns, s.now())
	if err != nil {
		return "", err
	}
	if done {
		return "Este resumen ya se había enviado.", nil
	}
	return fmt.Sprintf("%d avisos en cola (%d proyectos perdidos en %s).", len(ns), len(lost), monthName(from)), nil
}

func (s *Service) perdidosMessage(r store.AvisoRecipient, month time.Time, lost []store.Project) (subject, text, html string) {
	name := monthName(month)
	paras := []string{"Hola, " + r.Name + "."}
	if len(lost) == 0 {
		paras = append(paras, "En "+name+" no se perdió ningún proyecto.")
	} else {
		var total money.Centavos
		for _, p := range lost {
			total += p.Subtotal
		}
		count := "1 proyecto"
		if len(lost) > 1 {
			count = fmt.Sprintf("%d proyectos", len(lost))
		}
		paras = append(paras, fmt.Sprintf("En %s se perdieron %s, con valor de %s sin IVA:", name, count, total))
		for _, p := range lost {
			paras = append(paras, fmt.Sprintf("%s · %s · vendedor %s · %s sin IVA. Motivo: %s\n%s/proyectos/%s",
				p.Label(), p.CustomerName, p.UserName, p.Subtotal, p.LostReason, s.baseURL, p.Folio))
		}
	}
	paras = append(paras, footer)
	text, html = notify.Email(paras...)
	return "Proyectos perdidos en " + name, text, html
}
