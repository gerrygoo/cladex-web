package web

import "net/http"

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
