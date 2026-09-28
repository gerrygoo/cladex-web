package views

import (
	"fmt"
	"time"
)

// BuildAge phrases how long ago the build was made, e.g. "hace 3 horas".
func BuildAge(built, now time.Time) string {
	return haceCuanto(now.Sub(built))
}

// haceCuanto phrases an elapsed duration in Spanish at the coarsest sensible unit.
func haceCuanto(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "hace unos segundos"
	case d < time.Hour:
		return plural(int(d/time.Minute), "minuto", "minutos")
	case d < 24*time.Hour:
		return plural(int(d/time.Hour), "hora", "horas")
	case d < 30*24*time.Hour:
		return plural(int(d/(24*time.Hour)), "día", "días")
	case d < 365*24*time.Hour:
		return plural(int(d/(30*24*time.Hour)), "mes", "meses")
	default:
		return plural(int(d/(365*24*time.Hour)), "año", "años")
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "hace 1 " + one
	}
	return fmt.Sprintf("hace %d %s", n, many)
}
