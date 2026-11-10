package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPendientes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ana, _ := s.CreateUser(ctx, "ana", "Ana", "hash", "vendedor")
	beto, _ := s.CreateUser(ctx, "beto", "Beto", "hash", "admin")

	mail, err := s.AddPendiente(ctx, ana, "Mandarle mail a Sonepar", PriorityMedia)
	if err != nil {
		t.Fatal(err)
	}
	contador, _ := s.AddPendiente(ctx, ana, "Mandarle al contador los estados de cuenta", PriorityBaja)
	betos, _ := s.AddPendiente(ctx, beto, "Revisar el pronóstico", PriorityAlta)
	if _, err := s.AddPendiente(ctx, ana, "x", "urgente"); err == nil {
		t.Error("AddPendiente took a priority that isn't one")
	}
	if _, err := s.AddPendiente(ctx, ana, "   ", PriorityMedia); err == nil {
		t.Error("AddPendiente took an empty description")
	}

	open, err := s.OpenPendientes(ctx, ana)
	if err != nil || len(open) != 2 || open[0].ID != mail || open[1].ID != contador {
		t.Fatalf("Ana's open = %+v, %v", open, err)
	}

	// Nothing of Ana's can be touched as Beto, nor Beto's as Ana, whatever the id.
	for _, err := range []error{
		s.SetPendientePriority(ctx, beto, mail, PriorityMuyAlta),
		s.CompletePendiente(ctx, beto, mail, time.Now()),
		s.ReopenPendiente(ctx, beto, mail),
		s.CompletePendiente(ctx, ana, betos, time.Now()),
		s.SetPendientePriority(ctx, ana, 9999, PriorityAlta),
	} {
		if !errors.Is(err, ErrPendienteNotFound) {
			t.Errorf("cross-user write = %v, want ErrPendienteNotFound", err)
		}
	}
	if open, _ := s.OpenPendientes(ctx, beto); len(open) != 1 || open[0].ID != betos {
		t.Errorf("Beto's open = %+v", open)
	}

	if err := s.SetPendientePriority(ctx, ana, mail, PriorityMuyAlta); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := s.CompletePendiente(ctx, ana, mail, now); err != nil {
		t.Fatal(err)
	}
	if err := s.CompletePendiente(ctx, ana, mail, now); !errors.Is(err, ErrPendienteNotFound) {
		t.Errorf("completing twice = %v", err)
	}
	if err := s.SetPendientePriority(ctx, ana, mail, PriorityBaja); !errors.Is(err, ErrPendienteNotFound) {
		t.Errorf("changing a completed one's priority = %v", err)
	}
	done, err := s.CompletedPendientes(ctx, ana, now.AddDate(0, 0, -30))
	if err != nil || len(done) != 1 || done[0].ID != mail || done[0].Priority != PriorityMuyAlta || done[0].CompletedAt == "" {
		t.Fatalf("Ana's completed = %+v, %v", done, err)
	}
	if done, _ := s.CompletedPendientes(ctx, ana, now.Add(time.Minute)); len(done) != 0 {
		t.Errorf("completed since after it = %+v", done)
	}
	if done, _ := s.CompletedPendientes(ctx, beto, now.AddDate(0, 0, -30)); len(done) != 0 {
		t.Errorf("Beto sees completed %+v", done)
	}

	if err := s.ReopenPendiente(ctx, ana, mail); err != nil {
		t.Fatal(err)
	}
	if open, _ := s.OpenPendientes(ctx, ana); len(open) != 2 {
		t.Errorf("after reopening, open = %+v", open)
	}

	// The text never reaches the audit log, which admins can read.
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE COALESCE(new_values, '') || COALESCE(old_values, '') LIKE '%Sonepar%'`).Scan(&n)
	if n != 0 {
		t.Errorf("%d audit rows carry a pendiente's text", n)
	}
}
