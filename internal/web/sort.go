package web

import (
	"net/http"
	"strings"

	"github.com/gerrygoo/cladex-web/internal/store"
)

// sortParams reads the "sort" and "dir" query params shared by every sortable list
// handler. dir is normalized to exactly "asc" or "desc" (anything else, including
// absent, becomes "asc"); sort is passed through as-is — each store List* method
// whitelists it against its own allowed columns.
func sortParams(r *http.Request) (sort, dir string) {
	sort = r.URL.Query().Get("sort")
	dir = r.URL.Query().Get("dir")
	if dir != "desc" {
		dir = "asc"
	}
	return sort, dir
}

// filterParams reads the column filters from the "f.<column>" query params (text and
// choice columns) and "f.<column>.min" / "f.<column>.max" (number and date columns). Choice columns may
// repeat their param to select several values. Blank values are dropped; each store List* method ignores columns it doesn't know.
func filterParams(r *http.Request) store.Filters {
	f := store.Filters{}
	for k, vals := range r.URL.Query() {
		key, ok := strings.CutPrefix(k, "f.")
		if !ok || key == "" || len(vals) == 0 {
			continue
		}
		for _, v := range vals {
			if v = strings.TrimSpace(v); v != "" {
				f[key] = append(f[key], v)
			}
		}
	}
	return f
}
