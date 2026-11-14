package store

import (
	"context"
	"testing"
	"time"
)

func TestUserAvisos(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ana, _ := s.CreateUser(ctx, "ana", "Ana", "hash", "vendedor")
	beto, _ := s.CreateUser(ctx, "beto", "Beto", "hash", "vendedor")
	ctxAdmin := WithActor(ctx, Actor{UserID: beto, Source: "web"})

	if err := s.SetUserAvisos(ctxAdmin, ana, RoleVendedor, []string{AvisoSeguimientoVencido, "no_existe"}); err != nil {
		t.Fatal(err)
	}
	// A vendedor can't turn on an aviso only admins may have (#69).
	s.SetUserAvisos(ctx, beto, RoleVendedor, []string{AvisoSeguimientoVencido, AvisoSeguimientoEscalado})
	if on, _ := s.UserAvisos(ctx, ana); len(on) != 1 || !on[AvisoSeguimientoVencido] {
		t.Errorf("Ana's avisos: %v", on)
	}
	if on, _ := s.UserAvisos(ctx, beto); on[AvisoSeguimientoEscalado] {
		t.Errorf("a vendedor turned on %s", AvisoSeguimientoEscalado)
	}
	// One an admin turned on for someone, as admin, stays when they save their own, and
	// only reaches them while they are an admin.
	s.SetUserAvisos(ctx, beto, RoleAdmin, []string{AvisoSeguimientoVencido, AvisoSeguimientoEscalado})
	s.SetUserAvisos(ctx, beto, RoleVendedor, []string{AvisoSeguimientoVencido})
	if on, _ := s.UserAvisos(ctx, beto); !on[AvisoSeguimientoEscalado] {
		t.Error("a vendedor's save turned off an admin-only aviso")
	}
	if rs, _ := s.AvisoRecipients(ctx, AvisoSeguimientoEscalado); len(rs) != 0 {
		t.Errorf("an admin-only aviso reaches a vendedor: %+v", rs)
	}
	s.db.ExecContext(ctx, `UPDATE users SET role = 'admin' WHERE id = ?`, beto)
	if rs, _ := s.AvisoRecipients(ctx, AvisoSeguimientoEscalado); len(rs) != 1 {
		t.Errorf("an admin-only aviso doesn't reach an admin: %+v", rs)
	}
	rs, _ := s.AvisoRecipients(ctx, AvisoSeguimientoVencido)
	if len(rs) != 2 || rs[0].Name != "Ana" {
		t.Errorf("recipients: %+v", rs)
	}
	// A disabled user gets nothing.
	if err := s.SetUserDisabled(ctx, ana, true); err != nil {
		t.Fatal(err)
	}
	if rs, _ := s.AvisoRecipients(ctx, AvisoSeguimientoVencido); len(rs) != 1 || rs[0].Name != "Beto" {
		t.Errorf("recipients with Ana disabled: %+v", rs)
	}
	// Turning one off is audited, with who did it.
	s.SetUserAvisos(ctxAdmin, ana, RoleVendedor, nil)
	var actor int64
	if err := s.db.QueryRowContext(ctx, `SELECT actor_id FROM audit_log WHERE table_name = 'user_avisos' AND op = 'delete'`).Scan(&actor); err != nil || actor != beto {
		t.Errorf("audit of the removal: actor %d, %v", actor, err)
	}

	now := time.Date(2026, 11, 12, 15, 0, 0, 0, time.UTC)
	n := NewNotification{Kind: AvisoSeguimientoVencido, Channel: ChannelEmail, UserID: beto, Address: "b@x.mx", Subject: "s", BodyText: "t"}
	if done, err := s.QueueAvisoRound(ctx, "r1", []NewNotification{n, n}, now); done || err != nil {
		t.Fatalf("first: %v %v", done, err)
	}
	if done, err := s.QueueAvisoRound(ctx, "r1", []NewNotification{n}, now); !done || err != nil {
		t.Fatalf("again: %v %v", done, err)
	}
	if c := rowCount(t, s, "notifications"); c != 2 {
		t.Errorf("notifications: %d", c)
	}
}
