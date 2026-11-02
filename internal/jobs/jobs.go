// Package jobs is the background job runner (#19; docs/planning/notificaciones-y-trabajos.md
// §1): one goroutine inside the server that runs the jobs queued in SQLite, one at a time,
// with leases, retries and backoff. A job with a future run_at is a timer that survives
// restarts.
//
// A job runs at least once: a handler may run again after it did its work (the process
// died before recording it), so every handler re-reads the state it acts on and does
// nothing when the work is already done.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/gerrygoo/cladex-web/internal/store"
)

// Handler runs one job. result is short text kept on the job, mainly to say why it did
// nothing. An error retries the job with backoff, unless it is Permanent.
type Handler func(ctx context.Context, job store.Job) (result string, err error)

type permanentError struct{ err error }

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

// Permanent marks err as one a retry can't fix (a bad address, a rejected template): the
// job fails at once instead of retrying.
func Permanent(err error) error { return permanentError{err} }

// IsPermanent reports whether err, or an error it wraps, was marked Permanent.
func IsPermanent(err error) bool {
	var p permanentError
	return errors.As(err, &p)
}

const (
	// pollInterval is how often the runner looks for due jobs when nothing wakes it.
	pollInterval = 30 * time.Second
	// lease is how long a claimed job is the runner's. A handler that needs longer is a bug.
	lease = 5 * time.Minute
)

// backoff is the wait before each retry: after the first failure, 1 minute, and so on.
var backoff = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 12 * time.Hour}

// retryDelay is the wait after a job's attempts-th failure, ±10% so jobs that failed
// together don't all retry together.
func retryDelay(attempts int) time.Duration {
	d := backoff[min(max(attempts, 1), len(backoff))-1]
	return d + time.Duration((rand.Float64()*0.2-0.1)*float64(d))
}

// Runner runs the queued jobs of the kinds registered on it.
type Runner struct {
	store    *store.Store
	logger   *slog.Logger
	handlers map[string]Handler
	wake     chan struct{}
	now      func() time.Time
}

func New(s *store.Store, logger *slog.Logger) *Runner {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Runner{store: s, logger: logger, handlers: map[string]Handler{},
		wake: make(chan struct{}, 1), now: time.Now}
}

// Register sets kind's handler. Call it before Run.
func (r *Runner) Register(kind string, h Handler) { r.handlers[kind] = h }

// Enqueue queues a job and wakes the runner, so a job due now runs without waiting for
// the next poll.
func (r *Runner) Enqueue(ctx context.Context, j store.NewJob) (int64, error) {
	id, err := r.store.EnqueueJob(ctx, j, r.now())
	if err == nil {
		r.Wake()
	}
	return id, err
}

// Wake makes the runner look for due jobs now, as after a job is queued or retried
// outside Enqueue.
func (r *Runner) Wake() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// Run runs due jobs until ctx is cancelled, then returns once the job in flight, if any,
// has finished. Shutting down, the caller waits for it as long as it waits for in-flight
// requests; a job that takes longer is picked up again when its lease runs out.
func (r *Runner) Run(ctx context.Context) {
	kinds := make([]string, 0, len(r.handlers))
	for k := range r.handlers {
		kinds = append(kinds, k)
	}
	slices.Sort(kinds)
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-r.wake:
		}
		// Run everything that's due before sleeping again.
		for ctx.Err() == nil && r.runOne(ctx, kinds) {
		}
		timer.Reset(pollInterval)
	}
}

// runOne claims and runs one due job, and reports whether there was one.
func (r *Runner) runOne(ctx context.Context, kinds []string) bool {
	if err := r.store.ReleaseExpiredJobs(ctx, r.now()); err != nil {
		r.logger.Error("jobs: release expired", slog.Any("err", err))
		return false
	}
	job, err := r.store.ClaimJob(ctx, kinds, r.now(), lease)
	if err != nil {
		r.logger.Error("jobs: claim", slog.Any("err", err))
		return false
	}
	if job == nil {
		return false
	}

	// The job finishes even if ctx is cancelled meanwhile, within its lease.
	jobCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), lease)
	defer cancel()
	jobCtx = store.WithActor(jobCtx, store.Actor{Source: store.SourceJob})
	log := r.logger.With(slog.Int64("job", job.ID), slog.String("kind", job.Kind), slog.Int("attempt", job.Attempts))
	start := r.now()
	result, err := r.call(jobCtx, *job)
	if err == nil {
		log.Info("job done", slog.Duration("took", r.now().Sub(start)), slog.String("result", result))
		if err := r.store.CompleteJob(jobCtx, job.ID, result, r.now()); err != nil {
			log.Error("jobs: complete", slog.Any("err", err))
		}
		return true
	}
	permanent := IsPermanent(err)
	log.Warn("job failed", slog.Any("err", err), slog.Bool("permanent", permanent))
	now := r.now()
	if err := r.store.FailJob(jobCtx, job.ID, err.Error(), permanent, now.Add(retryDelay(job.Attempts)), now); err != nil {
		log.Error("jobs: fail", slog.Any("err", err))
	}
	return true
}

// call runs job's handler, turning a panic into a permanent error: a bug won't fix itself
// on retry.
func (r *Runner) call(ctx context.Context, job store.Job) (result string, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = Permanent(fmt.Errorf("panic: %v", p))
		}
	}()
	return r.handlers[job.Kind](ctx, job)
}

// KindPrueba is the job the Trabajos page queues to check that the runner is running.
const KindPrueba = "prueba"

// Prueba is KindPrueba's handler: it does nothing but finish.
func Prueba(context.Context, store.Job) (string, error) {
	return "El trabajo de prueba se ejecutó.", nil
}
