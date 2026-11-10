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

	if err := s.SetUserAvisos(ctxAdmin, ana, []string{AvisoSeguimientoVencido, "no_existe"}); err != nil {
		t.Fatal(err)
	}
	s.SetUserAvisos(ctx, beto, []string{AvisoSeguimientoVencido, AvisoSeguimientoEscalado})
	if on, _ := s.UserAvisos(ctx, ana); len(on) != 1 || !on[AvisoSeguimientoVencido] {
		t.Errorf("Ana's avisos: %v", on)
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
	s.SetUserAvisos(ctxAdmin, ana, nil)
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
