package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Job statuses (migrations/0035_jobs.sql).
const (
	JobPendiente = "pendiente"
	JobEnCurso   = "en_curso"
	JobHecho     = "hecho"
	JobFallido   = "fallido"
	JobCancelado = "cancelado"
)

// JobStatusLabels are the words admins see for each status.
var JobStatusLabels = map[string]string{
	JobPendiente: "Pendiente",
	JobEnCurso:   "En curso",
	JobHecho:     "Hecho",
	JobFallido:   "Fallido",
	JobCancelado: "Cancelado",
}

// jobTimeLayout is strftime's '%Y-%m-%dT%H:%M:%fZ', the column defaults' shape. Every job
// time is written in it so run_at and lease_until compare correctly as text.
const jobTimeLayout = "2006-01-02T15:04:05.000Z"

func jobTime(t time.Time) string { return t.UTC().Format(jobTimeLayout) }

// ErrJobState is returned when a job isn't in a state for the action asked of it.
var ErrJobState = errors.New("store: job is not in a state for that")

// Job is a jobs row.
type Job struct {
	ID          int64
	Kind        string
	Payload     string
	DedupeKey   string
	Status      string
	RunAt       string
	Attempts    int
	MaxAttempts int
	LeaseUntil  string
	LastError   string
	Result      string
	CreatedAt   string
	FinishedAt  string
}

// NewJob is a job to enqueue. A zero RunAt means now; a zero MaxAttempts, the default 5.
// DedupeKey, when set, makes the enqueue a no-op while the kind already has a live
// (pendiente or en curso) job with that key.
type NewJob struct {
	Kind        string
	Payload     string
	DedupeKey   string
	RunAt       time.Time
	MaxAttempts int
}

const jobCols = `id, kind, payload, COALESCE(dedupe_key, ''), status, run_at, attempts, max_attempts,
	COALESCE(lease_until, ''), COALESCE(last_error, ''), COALESCE(result, ''), created_at,
	COALESCE(finished_at, '')`

func scanJob(row interface{ Scan(...any) error }) (Job, error) {
	var j Job
	err := row.Scan(&j.ID, &j.Kind, &j.Payload, &j.DedupeKey, &j.Status, &j.RunAt, &j.Attempts,
		&j.MaxAttempts, &j.LeaseUntil, &j.LastError, &j.Result, &j.CreatedAt, &j.FinishedAt)
	return j, err
}

// EnqueueJob queues j on its own. It returns the new job's id, or the live job's when the
// dedupe key is already queued.
func (s *Store) EnqueueJob(ctx context.Context, j NewJob, now time.Time) (int64, error) {
	var id int64
	err := s.inTx(ctx, "enqueue job", func(tx *sql.Tx) error {
		var err error
		id, err = enqueueJob(ctx, tx, j, now)
		return err
	})
	return id, err
}

// enqueueJob queues j inside tx, so a job is written in the same transaction as whatever
// called for it.
func enqueueJob(ctx context.Context, tx *sql.Tx, j NewJob, now time.Time) (int64, error) {
	if j.Payload == "" {
		j.Payload = "{}"
	}
	if j.MaxAttempts == 0 {
		j.MaxAttempts = 5
	}
	runAt := j.RunAt
	if runAt.IsZero() {
		runAt = now
	}
	var id int64
	err := tx.QueryRowContext(ctx, `
		INSERT INTO jobs (kind, payload, dedupe_key, run_at, max_attempts, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (kind, dedupe_key) WHERE dedupe_key IS NOT NULL AND status IN ('pendiente', 'en_curso')
		DO NOTHING
		RETURNING id`,
		j.Kind, j.Payload, nullIfEmpty(j.DedupeKey), jobTime(runAt), j.MaxAttempts, jobTime(now),
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, `
			SELECT id FROM jobs WHERE kind = ? AND dedupe_key = ? AND status IN ('pendiente', 'en_curso')`,
			j.Kind, j.DedupeKey).Scan(&id)
	}
	return id, err
}

// ReleaseExpiredJobs puts back the en curso jobs whose lease ran out before now: the
// process that claimed them died. One that has used up its attempts fails instead, so a
// job that kills the process can't loop forever.
func (s *Store) ReleaseExpiredJobs(ctx context.Context, now time.Time) error {
	const lost = "El proceso se detuvo antes de terminar el trabajo."
	ts := jobTime(now)
	return s.inTx(ctx, "release expired jobs", func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			UPDATE jobs SET status = 'fallido', lease_until = NULL, last_error = ?, finished_at = ?
			WHERE status = 'en_curso' AND lease_until <= ? AND attempts >= max_attempts`,
			lost, ts, ts); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			UPDATE jobs SET status = 'pendiente', lease_until = NULL, last_error = ?
			WHERE status = 'en_curso' AND lease_until <= ?`,
			lost, ts)
		return err
	})
}

// ClaimJob takes the oldest pendiente job of one of kinds that is due at now, marks it en
// curso until now+lease and counts the attempt. It returns nil when nothing is due. Only
// the kinds a runner can handle are claimed: during a blue/green deploy the old build's
// runner shares the queue with the new one's, and must leave alone kinds it doesn't know.
func (s *Store) ClaimJob(ctx context.Context, kinds []string, now time.Time, lease time.Duration) (*Job, error) {
	if len(kinds) == 0 {
		return nil, nil
	}
	args := []any{jobTime(now.Add(lease)), jobTime(now)}
	for _, k := range kinds {
		args = append(args, k)
	}
	var job *Job
	err := s.inTx(ctx, "claim job", func(tx *sql.Tx) error {
		j, err := scanJob(tx.QueryRowContext(ctx, `
			UPDATE jobs SET status = 'en_curso', lease_until = ?, attempts = attempts + 1
			WHERE id = (
				SELECT id FROM jobs
				WHERE status = 'pendiente' AND run_at <= ? AND kind IN (?`+strings.Repeat(", ?", len(kinds)-1)+`)
				ORDER BY run_at, id LIMIT 1)
			RETURNING `+jobCols, args...))
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		job = &j
		return nil
	})
	return job, err
}

// CompleteJob marks a claimed job hecho, with the handler's result.
func (s *Store) CompleteJob(ctx context.Context, id int64, result string, now time.Time) error {
	return s.updateJob(ctx, "complete job", id, `
		UPDATE jobs SET status = 'hecho', lease_until = NULL, result = ?, finished_at = ?
		WHERE id = ? AND status = 'en_curso'`,
		nullIfEmpty(result), jobTime(now), id)
}

// FailJob records a claimed job's error. It goes back to pendiente at retryAt, unless
// permanent is set or it has used up its attempts, in which case it is fallido.
func (s *Store) FailJob(ctx context.Context, id int64, errMsg string, permanent bool, retryAt, now time.Time) error {
	return s.updateJob(ctx, "fail job", id, `
		UPDATE jobs SET
			status      = CASE WHEN ? OR attempts >= max_attempts THEN 'fallido' ELSE 'pendiente' END,
			run_at      = CASE WHEN ? OR attempts >= max_attempts THEN run_at ELSE ? END,
			finished_at = CASE WHEN ? OR attempts >= max_attempts THEN ? END,
			lease_until = NULL, last_error = ?
		WHERE id = ? AND status = 'en_curso'`,
		permanent, permanent, jobTime(retryAt), permanent, jobTime(now), errMsg, id)
}

// updateJob runs a statement guarded by the job's status, and returns ErrJobState when the
// job wasn't in it. For a claim's outcome, a job no longer en curso means its lease ran
// out and another runner may have it, so the late outcome is refused.
func (s *Store) updateJob(ctx context.Context, what string, id int64, query string, args ...any) error {
	return s.inTx(ctx, what, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("job %d: %w", id, ErrJobState)
		}
		return nil
	})
}

// RetryJobNow makes a pendiente job due now, or puts a fallido one back in the queue with
// its attempts reset, as if just enqueued.
func (s *Store) RetryJobNow(ctx context.Context, id int64, now time.Time) error {
	return s.updateJob(ctx, "retry job", id, `
		UPDATE jobs SET
			attempts    = CASE WHEN status = 'fallido' THEN 0 ELSE attempts END,
			status      = 'pendiente', run_at = ?, finished_at = NULL
		WHERE id = ? AND status IN ('pendiente', 'fallido')`,
		jobTime(now), id)
}

// JobByID reads one job, or nil when there's none.
func (s *Store) JobByID(ctx context.Context, id int64) (*Job, error) {
	j, err := scanJob(s.db.QueryRowContext(ctx, `SELECT `+jobCols+` FROM jobs WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: job: %w", err)
	}
	return &j, nil
}

// JobsOverview is what the Trabajos page lists: every job still to run, running or failed,
// soonest first, then the most recently finished ones.
type JobsOverview struct {
	Open   []Job
	Recent []Job
}

// ListJobs reads the Trabajos page: all open jobs (pendiente, en curso, fallido) and the
// last recentLimit hecho or cancelado ones.
func (s *Store) ListJobs(ctx context.Context, recentLimit int) (JobsOverview, error) {
	var o JobsOverview
	var err error
	o.Open, err = s.queryJobs(ctx, `SELECT `+jobCols+` FROM jobs
		WHERE status IN ('pendiente', 'en_curso', 'fallido')
		ORDER BY CASE status WHEN 'fallido' THEN 0 WHEN 'en_curso' THEN 1 ELSE 2 END, run_at, id`)
	if err != nil {
		return o, err
	}
	o.Recent, err = s.queryJobs(ctx, `SELECT `+jobCols+` FROM jobs
		WHERE status IN ('hecho', 'cancelado')
		ORDER BY finished_at DESC, id DESC LIMIT ?`, recentLimit)
	return o, err
}

func (s *Store) queryJobs(ctx context.Context, query string, args ...any) ([]Job, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list jobs: %w", err)
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list jobs: %w", err)
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
