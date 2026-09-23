package views

import (
	"testing"
	"time"
)

func TestBuildLabel(t *testing.T) {
	built := time.Date(2026, 9, 22, 20, 3, 0, 0, time.UTC) // 14:03 in Mexico City (UTC-6)
	cases := []struct {
		since time.Duration
		want  string
	}{
		{10 * time.Second, "Compilado el 22/09/2026 14:03 (hace unos segundos)"},
		{time.Minute, "Compilado el 22/09/2026 14:03 (hace 1 minuto)"},
		{45 * time.Minute, "Compilado el 22/09/2026 14:03 (hace 45 minutos)"},
		{3*time.Hour + 59*time.Minute, "Compilado el 22/09/2026 14:03 (hace 3 horas)"},
		{24 * time.Hour, "Compilado el 22/09/2026 14:03 (hace 1 día)"},
		{40 * 24 * time.Hour, "Compilado el 22/09/2026 14:03 (hace 1 mes)"},
		{800 * 24 * time.Hour, "Compilado el 22/09/2026 14:03 (hace 2 años)"},
	}
	for _, c := range cases {
		if got := BuildLabel(built, built.Add(c.since)); got != c.want {
			t.Errorf("BuildLabel(+%v) = %q, want %q", c.since, got, c.want)
		}
	}
	if got := BuildLabel(time.Time{}, built); got != "Compilación local (dev)" {
		t.Errorf("zero build time = %q", got)
	}
}
