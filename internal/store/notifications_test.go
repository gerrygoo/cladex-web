package store

import (
	"context"
	"strings"
	"testing"
)

func TestSetUserContact(t *testing.T) {
	s := newTestStore(t)
	ctx := WithActor(context.Background(), Actor{Source: SourceCLI})
	id, err := s.CreateUser(ctx, "ana", "Ana", "hash", RoleVendedor)
	if err != nil {
		t.Fatal(err)
	}
	if c, err := s.UserContactByID(ctx, id); err != nil || c != (UserContact{}) {
		t.Fatalf("a new user's contact: %+v, %v", c, err)
	}

	// A channel without its address is off, whatever the checkbox said.
	if err := s.SetUserContact(ctx, id, UserContact{Email: "ana@cladex.com.mx", NotifyEmail: true, NotifyWhatsApp: true}); err != nil {
		t.Fatal(err)
	}
	if c, _ := s.UserContactByID(ctx, id); c != (UserContact{Email: "ana@cladex.com.mx", NotifyEmail: true}) {
		t.Errorf("saved %+v", c)
	}

	// The users audit triggers (recreated in 0032) record contact changes.
	var oldV, newV string
	if err := s.db.QueryRowContext(ctx, `
		SELECT old_values, new_values FROM audit_log
		WHERE table_name = 'users' AND row_key = ? AND op = 'update' ORDER BY id DESC LIMIT 1`, id,
	).Scan(&oldV, &newV); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(oldV, `"email":null`) || !strings.Contains(newV, `"email":"ana@cladex.com.mx"`) ||
		!strings.Contains(newV, `"notify_email":1`) || !strings.Contains(newV, `"password_changed":0`) {
		t.Errorf("audit: %s → %s", oldV, newV)
	}
}
