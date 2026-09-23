package views

import (
	"testing"
	"time"
)

func TestMaterialAge(t *testing.T) {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct{ updatedAt, want string }{
		{"2026-09-22T08:00:00.000Z", "actualizado hoy"},
		{"2026-09-21T11:00:00.000Z", "actualizado hace 1 día"},
		{"2026-09-12T12:00:00.000Z", "actualizado hace 10 días"},
		{"no es fecha", ""},
	} {
		if got := MaterialAge(c.updatedAt, now); got != c.want {
			t.Errorf("MaterialAge(%q) = %q, want %q", c.updatedAt, got, c.want)
		}
	}
}
