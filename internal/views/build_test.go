package views

import (
	"testing"
	"time"
)

func TestBuildAge(t *testing.T) {
	built := time.Date(2026, 9, 22, 20, 3, 0, 0, time.UTC)
	cases := []struct {
		since time.Duration
		want  string
	}{
		{10 * time.Second, "hace unos segundos"},
		{time.Minute, "hace 1 minuto"},
		{45 * time.Minute, "hace 45 minutos"},
		{3*time.Hour + 59*time.Minute, "hace 3 horas"},
		{24 * time.Hour, "hace 1 día"},
		{40 * 24 * time.Hour, "hace 1 mes"},
		{800 * 24 * time.Hour, "hace 2 años"},
	}
	for _, c := range cases {
		if got := BuildAge(built, built.Add(c.since)); got != c.want {
			t.Errorf("BuildAge(+%v) = %q, want %q", c.since, got, c.want)
		}
	}
}

func TestTimestampFallback(t *testing.T) {
	if got := timestampFallback("2026-09-28T16:42:05.000Z"); got != "28/09/2026 - 16:42" {
		t.Errorf("timestampFallback = %q", got)
	}
	if got := timestampFallback("basura"); got != "basura" {
		t.Errorf("unparseable = %q, want it untouched", got)
	}
	if got := dateText("2026-10-27"); got != "27/10/2026" {
		t.Errorf("dateText = %q", got)
	}
}
