// Package avisos decides which avisos go out and when (issue #69;
// docs/planning/notificaciones-y-trabajos.md §3), and hands them to the notifications
// outbox. It owns the seguimiento rounds: a job per round, at 9:00, 12:00 and 18:00
// (Mexico City) on weekdays, each queueing the next.
//
// A round looks at every prospecto whose seguimiento day D has passed with nothing on its
// history since D began, and counts n, the weekdays after D up to today:
//
//   - its owner, with seguimiento_vencido on: on day 1 at 9:00, 12:00 and 18:00, and
//     every day after at 18:00;
//   - everyone with seguimiento_escalado on: on day 2 at 12:00, and every 3 weekdays
//     after (5, 8, …), also at 12:00.
//
// Any comment on the proyecto (a follow-up, which a new date also writes, or a note), or
// its leaving prospecto, stops them: the next round no longer finds it.
package avisos

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gerrygoo/cladex-web/internal/jobs"
	"github.com/gerrygoo/cladex-web/internal/notify"
	"github.com/gerrygoo/cladex-web/internal/store"
)

// JobKind is the seguimiento round's job; its payload names the round's time.
const JobKind = "avisos_seguimiento"

// Hours are the rounds' times of day.
var Hours = []int{9, 12, 18}

// lateLimit is how late a round may still run. One that missed it (the server was down)
// sends nothing: the next one picks the proyectos up.
const lateLimit = 2 * time.Hour

const slotLayout = "2006-01-02T15:04"

// Enqueuer queues a job; *jobs.Runner is one.
type Enqueuer interface {
	Enqueue(ctx context.Context, j store.NewJob) (int64, error)
}

// Service runs the seguimiento rounds.
type Service struct {
	store   *store.Store
	jobs    Enqueuer
	logger  *slog.Logger
	baseURL string
	loc     *time.Location
	now     func() time.Time
}

// New returns the service; baseURL is the app's address, for links in the avisos.
func New(s *store.Store, j Enqueuer, baseURL string, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Service{store: s, jobs: j, logger: logger, baseURL: strings.TrimRight(baseURL, "/"), loc: MexicoCity(), now: time.Now}
}

// MexicoCity is the rounds' time zone. Mexico has kept UTC-6 all year since 2022.
func MexicoCity() *time.Location {
	if l, err := time.LoadLocation("America/Mexico_City"); err == nil {
		return l
	}
	return time.FixedZone("CST", -6*60*60)
}

func weekday(t time.Time) bool { return t.Weekday() != time.Saturday && t.Weekday() != time.Sunday }

// nextSlot is the first round time after t, in loc.
func nextSlot(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
	for {
		if weekday(day) {
			for _, h := range Hours {
				if s := day.Add(time.Duration(h) * time.Hour); s.After(t) {
					return s
				}
			}
		}
		day = day.AddDate(0, 0, 1)
	}
}

// Schedule queues the round due next. At startup it starts from lateLimit ago, so a round
// missed by a restart still runs (a round that already went out sends nothing twice).
func (s *Service) Schedule(ctx context.Context) error {
	return s.schedule(ctx, nextSlot(s.now().Add(-lateLimit), s.loc))
}

func (s *Service) schedule(ctx context.Context, slot time.Time) error {
	key := slot.In(s.loc).Format(slotLayout)
	_, err := s.jobs.Enqueue(ctx, store.NewJob{
		Kind:      JobKind,
		Payload:   fmt.Sprintf(`{"slot":%q}`, key),
		DedupeKey: key,
		RunAt:     slot,
	})
	return err
}

// Handle is a round's job. It queues the next round first, so a failing round never
// stops the ones after it.
func (s *Service) Handle(ctx context.Context, job store.Job) (string, error) {
	var p struct{ Slot string }
	if err := json.Unmarshal([]byte(job.Payload), &p); err != nil {
		return "", jobs.Permanent(fmt.Errorf("payload: %w", err))
	}
	slot, err := time.ParseInLocation(slotLayout, p.Slot, s.loc)
	if err != nil {
		return "", jobs.Permanent(fmt.Errorf("payload: %w", err))
	}
	if err := s.schedule(ctx, nextSlot(slot, s.loc)); err != nil {
		return "", err
	}
	if late := s.now().Sub(slot); late > lateLimit {
		return fmt.Sprintf("No se envió: la ronda de las %s corrió %s tarde.", slot.Format("15:04"), late.Round(time.Minute)), nil
	}

	ns, err := s.round(ctx, slot)
	if err != nil {
		return "", err
	}
	done, err := s.store.QueueAvisoRound(ctx, "seguimiento "+p.Slot, ns, s.now())
	if err != nil {
		return "", err
	}
	if done {
		return "Esta ronda ya se había enviado.", nil
	}
	if len(ns) == 0 {
		return "Sin seguimientos vencidos que avisar.", nil
	}
	return fmt.Sprintf("%d avisos en cola.", len(ns)), nil
}

// weekdaysAfter counts the weekdays after day d up to and including today.
func weekdaysAfter(d, today time.Time) int {
	n := 0
	for t := d.AddDate(0, 0, 1); !t.After(today); t = t.AddDate(0, 0, 1) {
		if weekday(t) {
			n++
		}
	}
	return n
}

// ownerDue and escalationDue say whether a round at hour h on weekday n after D sends to
// the owner and to the team.
func ownerDue(n, h int) bool {
	return (n == 1 && (h == 9 || h == 12 || h == 18)) || (n >= 2 && h == 18)
}
func escalationDue(n, h int) bool { return n >= 2 && h == 12 && (n-2)%3 == 0 }

// round builds the notifications for the round at slot.
func (s *Service) round(ctx context.Context, slot time.Time) ([]store.NewNotification, error) {
	today := time.Date(slot.Year(), slot.Month(), slot.Day(), 0, 0, 0, 0, s.loc)
	if !weekday(today) {
		return nil, nil
	}
	projects, err := s.store.ProspectosFollowUpBefore(ctx, today.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	owners, err := s.store.AvisoRecipients(ctx, store.AvisoSeguimientoVencido)
	if err != nil {
		return nil, err
	}
	team, err := s.store.AvisoRecipients(ctx, store.AvisoSeguimientoEscalado)
	if err != nil {
		return nil, err
	}
	ownerByID := map[int64]store.AvisoRecipient{}
	for _, r := range owners {
		ownerByID[r.UserID] = r
	}

	var out []store.NewNotification
	for _, p := range projects {
		d, err := time.ParseInLocation("2006-01-02", p.NextFollowUpDate, s.loc)
		if err != nil {
			continue
		}
		if s.attendedSince(p, d) {
			continue
		}
		n := weekdaysAfter(d, today)
		if r, ok := ownerByID[p.UserID]; ok && ownerDue(n, slot.Hour()) {
			subject, text, html := s.ownerMessage(r, p, d, n)
			out = append(out, emailTo(store.AvisoSeguimientoVencido, r, p, subject, text, html)...)
		}
		if escalationDue(n, slot.Hour()) {
			for _, r := range team {
				subject, text, html := s.teamMessage(r, p, d, n)
				out = append(out, emailTo(store.AvisoSeguimientoEscalado, r, p, subject, text, html)...)
			}
		}
	}
	return out, nil
}

// attendedSince reports whether the proyecto's history has an entry from day d on.
func (s *Service) attendedSince(p store.Project, d time.Time) bool {
	if p.LastAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339Nano, p.LastAt)
	if err != nil {
		return false
	}
	return !t.In(s.loc).Before(d)
}

// emailTo is r's email notification, or none when r has email off. WhatsApp joins the
// steps that call for it (day 1 at 18:00 and the team's) once its sender and templates
// are in (N4).
func emailTo(kind string, r store.AvisoRecipient, p store.Project, subject, text, html string) []store.NewNotification {
	if !r.NotifyEmail || r.Email == "" {
		return nil
	}
	return []store.NewNotification{{
		Kind: kind, Channel: store.ChannelEmail, UserID: r.UserID, Address: r.Email,
		ProjectID: p.ID, Subject: subject, BodyText: text, BodyHTML: html,
	}}
}

func day(t time.Time) string { return t.Format("02/01/2006") }

// details are the paragraphs every seguimiento aviso carries: the proyecto, its money,
// its last comment and its link.
func (s *Service) details(p store.Project) []string {
	contact := p.ContactName
	for _, c := range []string{p.ContactPhone, p.ContactEmail} {
		if c != "" {
			if contact != "" {
				contact += " · "
			}
			contact += c
		}
	}
	if contact == "" {
		contact = "sin contacto registrado"
	}
	last := "Último comentario de seguimiento: no hay ninguno."
	if p.LastBody != "" {
		when := p.LastAt
		if t, err := time.Parse(time.RFC3339Nano, p.LastAt); err == nil {
			when = day(t.In(s.loc))
		}
		last = fmt.Sprintf("Último comentario de seguimiento, de %s el %s: %s", p.LastUserName, when, p.LastBody)
	}
	return []string{
		fmt.Sprintf("Proyecto %s · cotización %s · cliente %s · contacto: %s.", p.Folio, p.CurrentQuoteFolio, p.CustomerName, contact),
		fmt.Sprintf("Total: %s. Antes de IVA: venta %s, costo %s, utilidad %s, margen %.1f%%.",
			p.Total, p.Subtotal, p.Cost(), p.Profit, p.MarginPercent()),
		last,
	}
}

func (s *Service) link(p store.Project) string {
	return "Ver el proyecto: " + s.baseURL + "/proyectos/" + p.Folio
}

const footer = "— Cladex. Este correo es automático; no lo respondas."

func (s *Service) ownerMessage(r store.AvisoRecipient, p store.Project, d time.Time, n int) (subject, text, html string) {
	what := fmt.Sprintf("El seguimiento del proyecto %s de %s era el %s y no se ha registrado nada desde entonces.", p.Folio, p.CustomerName, day(d))
	if n > 1 {
		what += fmt.Sprintf(" Van %d días hábiles.", n)
	}
	paras := append([]string{"Hola, " + r.Name + ".", what}, s.details(p)...)
	paras = append(paras,
		"Siguiente paso: registra el seguimiento en el proyecto, con un comentario o una nueva fecha. En cuanto lo hagas, dejan de llegar estos avisos.",
		s.link(p), footer)
	text, html = notify.Email(paras...)
	return fmt.Sprintf("Seguimiento vencido: %s · %s", p.Folio, p.CustomerName), text, html
}

func (s *Service) teamMessage(r store.AvisoRecipient, p store.Project, d time.Time, n int) (subject, text, html string) {
	what := fmt.Sprintf("%s no le ha dado seguimiento al proyecto %s de %s, con valor de %s. Su seguimiento era el %s; van %d días hábiles sin que se registre nada.",
		p.UserName, p.Folio, p.CustomerName, p.Total, day(d), n)
	paras := append([]string{"Hola, " + r.Name + ".", what}, s.details(p)...)
	paras = append(paras,
		"Siguiente paso: revisar el proyecto con "+p.UserName+".",
		s.link(p), footer)
	text, html = notify.Email(paras...)
	return fmt.Sprintf("%s no ha dado seguimiento a %s · %s", p.UserName, p.Folio, p.CustomerName), text, html
}
