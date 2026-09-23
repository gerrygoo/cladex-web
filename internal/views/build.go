package views

import (
	"fmt"
	"time"
)

// mexicoCity is where every user of the app is; build times render in its wall clock.
// Falls back to UTC if the zone database is somehow missing.
var mexicoCity = func() *time.Location {
	if loc, err := time.LoadLocation("America/Mexico_City"); err == nil {
		return loc
	}
	return time.UTC
}()

// BuildLabel renders the home page's build footer, e.g.
// "Compilado el 22/09/2026 14:03 (hace 3 horas)". A zero built means a local build
// with no timestamp stamped in.
func BuildLabel(built, now time.Time) string {
	if built.IsZero() {
		return "Compilación local (dev)"
	}
	return fmt.Sprintf("Compilado el %s (%s)", built.In(mexicoCity).Format("02/01/2006 15:04"), haceCuanto(now.Sub(built)))
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
