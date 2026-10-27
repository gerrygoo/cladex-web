package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

var jobsT0 = time.Date(2026, 10, 26, 15, 0, 0, 0, time.UTC)

func mustEnqueue(t *testing.T, s *Store, j NewJob, now time.Time) int64 {
	t.Helper()
	id, err := s.EnqueueJob(context.Background(), j, now)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	return id
}

func mustJob(t *testing.T, s *Store, id int64) Job {
	t.Helper()
	j, err := s.JobByID(context.Background(), id)
	if err != nil || j == nil {
		t.Fatalf("job %d: %v, %v", id, j, err)
	}
	return *j
}

func mustClaim(t *testing.T, s *Store, kinds []string, now time.Time) *Job {
	t.Helper()
	j, err := s.ClaimJob(context.Background(), kinds, now, 5*time.Minute)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	return j
}

func TestEnqueueJobDedupe(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	a := mustEnqueue(t, s, NewJob{Kind: "aviso", DedupeKey: "p:1"}, jobsT0)
	if b := mustEnqueue(t, s, NewJob{Kind: "aviso", DedupeKey: "p:1"}, jobsT0); b != a {
		t.Errorf("same live key queued twice: %d and %d", a, b)
	}
	if c := mustEnqueue(t, s, NewJob{Kind: "otro", DedupeKey: "p:1"}, jobsT0); c == a {
		t.Error("the key should be unique per kind, not across kinds")
	}
	// Keyless jobs never collide.
	if x, y := mustEnqueue(t, s, NewJob{Kind: "aviso"}, jobsT0), mustEnqueue(t, s, NewJob{Kind: "aviso"}, jobsT0); x == y {
		t.Error("two keyless jobs got the same id")
	}

	j := mustJob(t, s, a)
	if j.Status != JobPendiente || j.Payload != "{}" || j.MaxAttempts != 5 || j.RunAt != "2026-10-26T15:00:00.000Z" {
		t.Errorf("defaults: %+v", j)
	}

	// Once finished, the key is free again.
	claimed := mustClaim(t, s, []string{"aviso"}, jobsT0)
	if claimed == nil || claimed.ID != a {
		t.Fatalf("claimed %+v, want job %d", claimed, a)
	}
	if err := s.CompleteJob(ctx, a, "", jobsT0); err != nil {
		t.Fatal(err)
	}
	if d := mustEnqueue(t, s, NewJob{Kind: "aviso", DedupeKey: "p:1"}, jobsT0); d == a {
		t.Error("a finished job's key should be free to queue again")
	}
}

func TestClaimJob(t *testing.T) {
	s := newTestStore(t)
	kinds := []string{"a", "b"}

	later := mustEnqueue(t, s, NewJob{Kind: "a", RunAt: jobsT0.Add(time.Hour)}, jobsT0)
	unknown := mustEnqueue(t, s, NewJob{Kind: "z"}, jobsT0)
	second := mustEnqueue(t, s, NewJob{Kind: "b", RunAt: jobsT0.Add(-time.Minute)}, jobsT0)
	first := mustEnqueue(t, s, NewJob{Kind: "a", RunAt: jobsT0.Add(-2 * time.Minute)}, jobsT0)

	for _, want := range []int64{first, second} {
		j := mustClaim(t, s, kinds, jobsT0)
		if j == nil || j.ID != want {
			t.Fatalf("claimed %+v, want job %d", j, want)
		}
		if j.Status != JobEnCurso || j.Attempts != 1 || j.LeaseUntil != "2026-10-26T15:05:00.000Z" {
			t.Errorf("claimed job: %+v", j)
		}
	}
	// The future job isn't due, and kind z isn't this runner's.
	if j := mustClaim(t, s, kinds, jobsT0); j != nil {
		t.Errorf("claimed %+v; nothing else is due", j)
	}
	if j := mustClaim(t, s, kinds, jobsT0.Add(time.Hour)); j == nil || j.ID != later {
		t.Errorf("claimed %+v at its run_at, want job %d", j, later)
	}
	if mustJob(t, s, unknown).Status != JobPendiente {
		t.Error("a kind the runner doesn't handle was touched")
	}
	if j := mustClaim(t, s, nil, jobsT0.Add(time.Hour)); j != nil {
		t.Errorf("a runner with no kinds claimed %+v", j)
	}
}

func TestFailJob(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	retryAt := jobsT0.Add(time.Minute)

	// A temporary error with attempts left: back to pendiente at retryAt.
	id := mustEnqueue(t, s, NewJob{Kind: "a", MaxAttempts: 2}, jobsT0)
	mustClaim(t, s, []string{"a"}, jobsT0)
	if err := s.FailJob(ctx, id, "timeout", false, retryAt, jobsT0); err != nil {
		t.Fatal(err)
	}
	j := mustJob(t, s, id)
	if j.Status != JobPendiente || j.RunAt != "2026-10-26T15:01:00.000Z" || j.LastError != "timeout" || j.LeaseUntil != "" || j.FinishedAt != "" {
		t.Errorf("after a retryable failure: %+v", j)
	}
	if mustClaim(t, s, []string{"a"}, jobsT0) != nil {
		t.Error("claimed before its retry time")
	}

	// The last attempt: fallido, run_at kept.
	mustClaim(t, s, []string{"a"}, retryAt)
	if err := s.FailJob(ctx, id, "timeout otra vez", false, retryAt.Add(time.Hour), retryAt); err != nil {
		t.Fatal(err)
	}
	j = mustJob(t, s, id)
	if j.Status != JobFallido || j.Attempts != 2 || j.RunAt != "2026-10-26T15:01:00.000Z" || j.FinishedAt != "2026-10-26T15:01:00.000Z" {
		t.Errorf("out of attempts: %+v", j)
	}

	// A permanent error fails on the first attempt.
	id = mustEnqueue(t, s, NewJob{Kind: "a"}, jobsT0)
	mustClaim(t, s, []string{"a"}, jobsT0)
	if err := s.FailJob(ctx, id, "dirección inválida", true, retryAt, jobsT0); err != nil {
		t.Fatal(err)
	}
	if j := mustJob(t, s, id); j.Status != JobFallido || j.Attempts != 1 {
		t.Errorf("permanent failure: %+v", j)
	}

	// An outcome for a job that is no longer en curso is refused.
	if err := s.FailJob(ctx, id, "tarde", false, retryAt, jobsT0); !errors.Is(err, ErrJobState) {
		t.Errorf("late FailJob: %v, want ErrJobState", err)
	}
	if err := s.CompleteJob(ctx, id, "", jobsT0); !errors.Is(err, ErrJobState) {
		t.Errorf("late CompleteJob: %v, want ErrJobState", err)
	}
}

func TestReleaseExpiredJobs(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	again := mustEnqueue(t, s, NewJob{Kind: "a"}, jobsT0)
	spent := mustEnqueue(t, s, NewJob{Kind: "a", MaxAttempts: 1}, jobsT0)
	mustClaim(t, s, []string{"a"}, jobsT0)
	mustClaim(t, s, []string{"a"}, jobsT0)

	// Still within the lease: nothing moves.
	if err := s.ReleaseExpiredJobs(ctx, jobsT0.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if mustJob(t, s, again).Status != JobEnCurso {
		t.Fatal("released a job whose lease hadn't run out")
	}

	if err := s.ReleaseExpiredJobs(ctx, jobsT0.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if j := mustJob(t, s, again); j.Status != JobPendiente || j.LeaseUntil != "" || j.LastError == "" {
		t.Errorf("expired lease with attempts left: %+v", j)
	}
	if j := mustJob(t, s, spent); j.Status != JobFallido || j.FinishedAt == "" {
		t.Errorf("expired lease out of attempts: %+v", j)
	}
	// The released job runs again, as its second attempt.
	if j := mustClaim(t, s, []string{"a"}, jobsT0.Add(5*time.Minute)); j == nil || j.ID != again || j.Attempts != 2 {
		t.Errorf("reclaimed %+v", j)
	}
}

func TestRetryJobNow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := jobsT0.Add(10 * time.Minute)

	// A failed job goes back in the queue with its attempts reset.
	failed := mustEnqueue(t, s, NewJob{Kind: "a"}, jobsT0)
	mustClaim(t, s, []string{"a"}, jobsT0)
	if err := s.FailJob(ctx, failed, "no", true, jobsT0, jobsT0); err != nil {
		t.Fatal(err)
	}
	if err := s.RetryJobNow(ctx, failed, now); err != nil {
		t.Fatal(err)
	}
	if j := mustJob(t, s, failed); j.Status != JobPendiente || j.Attempts != 0 || j.FinishedAt != "" || j.RunAt != "2026-10-26T15:10:00.000Z" {
		t.Errorf("retried failed job: %+v", j)
	}

	// A pending one waiting out its backoff becomes due now, keeping its attempts.
	waiting := mustEnqueue(t, s, NewJob{Kind: "b"}, jobsT0)
	mustClaim(t, s, []string{"b"}, jobsT0)
	if err := s.FailJob(ctx, waiting, "no", false, jobsT0.Add(2*time.Hour), jobsT0); err != nil {
		t.Fatal(err)
	}
	if err := s.RetryJobNow(ctx, waiting, now); err != nil {
		t.Fatal(err)
	}
	if j := mustJob(t, s, waiting); j.Status != JobPendiente || j.Attempts != 1 || j.RunAt != "2026-10-26T15:10:00.000Z" {
		t.Errorf("retried waiting job: %+v", j)
	}

	// Running, done and missing jobs can't be retried.
	mustClaim(t, s, []string{"a"}, now)
	if err := s.RetryJobNow(ctx, failed, now); !errors.Is(err, ErrJobState) {
		t.Errorf("retry en curso: %v", err)
	}
	if err := s.CompleteJob(ctx, failed, "", now); err != nil {
		t.Fatal(err)
	}
	if err := s.RetryJobNow(ctx, failed, now); !errors.Is(err, ErrJobState) {
		t.Errorf("retry hecho: %v", err)
	}
	if err := s.RetryJobNow(ctx, 999, now); !errors.Is(err, ErrJobState) {
		t.Errorf("retry missing: %v", err)
	}
}

func TestListJobs(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	pending := mustEnqueue(t, s, NewJob{Kind: "a", RunAt: jobsT0.Add(time.Hour)}, jobsT0)
	failed := mustEnqueue(t, s, NewJob{Kind: "b"}, jobsT0)
	mustClaim(t, s, []string{"b"}, jobsT0)
	if err := s.FailJob(ctx, failed, "no", true, jobsT0, jobsT0); err != nil {
		t.Fatal(err)
	}
	var done []int64
	for i := range 3 {
		id := mustEnqueue(t, s, NewJob{Kind: "c"}, jobsT0)
		mustClaim(t, s, []string{"c"}, jobsT0)
		if err := s.CompleteJob(ctx, id, "listo", jobsT0.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
		done = append(done, id)
	}

	o, err := s.ListJobs(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(o.Open) != 2 || o.Open[0].ID != failed || o.Open[1].ID != pending {
		t.Errorf("open = %+v, want the failed job then the pending one", o.Open)
	}
	if len(o.Recent) != 2 || o.Recent[0].ID != done[2] || o.Recent[1].ID != done[1] || o.Recent[0].Result != "listo" {
		t.Errorf("recent = %+v, want the last two finished, newest first", o.Recent)
	}
}
