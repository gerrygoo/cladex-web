package web

import (
	"fmt"
	"io/fs"
	"net/http"
	"time"

	"github.com/gerrygoo/cladex-web/internal/views"
)

// NewMux builds the application router. buildSHA is surfaced on /healthz.
func NewMux(buildSHA string, staticFS fs.FS) *http.ServeMux {
	mux := http.NewServeMux()

	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticFS)))

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "ok %s", buildSHA)
	})

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		views.Home().Render(r.Context(), w)
	})

	mux.HandleFunc("GET /saludo", func(w http.ResponseWriter, r *http.Request) {
		mensaje := fmt.Sprintf("Hola — htmx funciona. Hora del servidor: %s", time.Now().Format(time.TimeOnly))
		views.Saludo(mensaje).Render(r.Context(), w)
	})

	return mux
}
