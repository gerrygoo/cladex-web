package jobs

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	cladex "github.com/gerrygoo/cladex-web"
	"github.com/gerrygoo/cladex-web/internal/store"
)

func openStore(t *testing.T, dsn string) *store.Store {
	t.Helper()
	s, err := store.Open(context.Background(), dsn, cladex.MigrationsFS)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// clock is a runner's time, moved by hand.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newRunner(s *store.Store, c *clock) *Runner {
	r := New(s, nil)
	r.now = c.now
	return r
}

func job(t *testing.T, s *store.Store, id int64) store.Job {
	t.Helper()
	j, err := s.JobByID(context.Background(), id)
	if err != nil || j == nil {
		t.Fatalf("job %d: %v, %v", id, j, err)
	}
	return *j
}

func parse(t *testing.T, ts string) time.Time {
	t.Helper()
	v, err := time.Parse("2006-01-02T15:04:05.000Z", ts)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// A job queued before a restart runs after it: the queue is the database, not memory.
func TestJobSurvivesRestart(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "test.db")
	c := &clock{time.Date(2026, 10, 26, 15, 0, 0, 0, time.UTC)}
	ctx := context.Background()

	before := openStore(t, dsn)
	id, err := newRunner(before, c).Enqueue(ctx, store.NewJob{Kind: KindPrueba, RunAt: c.t.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	before.Close()

	after := openStore(t, dsn)
	r := newRunner(after, c)
	r.Register(KindPrueba, Prueba)
	if r.runOne(ctx, []string{KindPrueba}) {
		t.Fatal("ran a job before its time")
	}
	c.t = c.t.Add(time.Hour)
	if !r.runOne(ctx, []string{KindPrueba}) {
		t.Fatal("the job queued before the restart didn't run")
	}
	if j := job(t, after, id); j.Status != store.JobHecho || j.Result != "El trabajo de prueba se ejecutó." {
		t.Errorf("after the run: %+v", j)
	}
}

// A failing job retries on the backoff schedule, shows its error, and fails for good
// after its last attempt.
func TestFailingJobRetriesOnSchedule(t *testing.T) {
	s := openStore(t, filepath.Join(t.TempDir(), "test.db"))
	c := &clock{time.Date(2026, 10, 26, 15, 0, 0, 0, time.UTC)}
	ctx := context.Background()
	r := newRunner(s, c)
	calls := 0
	r.Register("falla", func(context.Context, store.Job) (string, error) {
		calls++
		return "", errors.New("el proveedor no responde")
	})
	kinds := []string{"falla"}

	id, err := r.Enqueue(ctx, store.NewJob{Kind: "falla"})
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= 5; attempt++ {
		if !r.runOne(ctx, kinds) {
			t.Fatalf("attempt %d didn't run", attempt)
		}
		j := job(t, s, id)
		if j.LastError != "el proveedor no responde" || j.Attempts != attempt {
			t.Fatalf("after attempt %d: %+v", attempt, j)
		}
		if attempt == 5 {
			if j.Status != store.JobFallido {
				t.Fatalf("after the last attempt: %+v", j)
			}
			break
		}
		wait := parse(t, j.RunAt).Sub(c.t)
		want := backoff[attempt-1]
		if j.Status != store.JobPendiente || wait < want*9/10 || wait > want*11/10 {
			t.Fatalf("after attempt %d: status %s, retry in %v, want %v ±10%%", attempt, j.Status, wait, want)
		}
		// Not before its time.
		if r.runOne(ctx, kinds) {
			t.Fatalf("attempt %d's retry ran early", attempt)
		}
		c.t = parse(t, j.RunAt)
	}
	if calls != 5 {
		t.Errorf("handler ran %d times, want 5", calls)
	}
	c.t = c.t.Add(24 * time.Hour)
	if r.runOne(ctx, kinds) {
		t.Error("a failed job ran again by itself")
	}
}

func TestPermanentErrorsAndPanicsDontRetry(t *testing.T) {
	s := openStore(t, filepath.Join(t.TempDir(), "test.db"))
	c := &clock{time.Date(2026, 10, 26, 15, 0, 0, 0, time.UTC)}
	ctx := context.Background()
	r := newRunner(s, c)
	r.Register("rechazo", func(context.Context, store.Job) (string, error) {
		return "", Permanent(errors.New("plantilla no aprobada"))
	})
	r.Register("bug", func(context.Context, store.Job) (string, error) {
		panic("índice fuera de rango")
	})

	for kind, wantErr := range map[string]string{
		"rechazo": "plantilla no aprobada",
		"bug":     "panic: índice fuera de rango",
	} {
		id, err := r.Enqueue(ctx, store.NewJob{Kind: kind})
		if err != nil {
			t.Fatal(err)
		}
		r.runOne(ctx, []string{kind})
		if j := job(t, s, id); j.Status != store.JobFallido || j.Attempts != 1 || j.LastError != wantErr {
			t.Errorf("%s: %+v", kind, j)
		}
	}
}

func TestRunWakesOnEnqueueAndStops(t *testing.T) {
	s := openStore(t, filepath.Join(t.TempDir(), "test.db"))
	r := New(s, nil)
	ran := make(chan int64, 1)
	r.Register("eco", func(_ context.Context, j store.Job) (string, error) {
		ran <- j.ID
		return "", nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Run(ctx)
		close(done)
	}()

	// Well within the 30 s poll, so it's the wake that ran it.
	id, err := r.Enqueue(context.Background(), store.NewJob{Kind: "eco"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-ran:
		if got != id {
			t.Errorf("ran job %d, want %d", got, id)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the queued job didn't run")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run didn't return after cancel")
	}
}

func TestRetryDelay(t *testing.T) {
	for attempts, want := range map[int]time.Duration{0: time.Minute, 1: time.Minute, 3: 30 * time.Minute, 5: 12 * time.Hour, 9: 12 * time.Hour} {
		for range 50 {
			if d := retryDelay(attempts); d < want*9/10 || d > want*11/10 {
				t.Fatalf("retryDelay(%d) = %v, want %v ±10%%", attempts, d, want)
			}
		}
	}
}
