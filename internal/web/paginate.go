package web

import (
	"net/http"
	"slices"
	"strconv"

	"github.com/gerrygoo/cladex-web/internal/views"
)

// perPage reads the "per" query param: one of views.PerPageChoices, else the default.
func perPage(r *http.Request) int {
	n, _ := strconv.Atoi(r.URL.Query().Get("per"))
	if slices.Contains(views.PerPageChoices, n) {
		return n
	}
	return views.DefaultPerPage
}

// paginate returns the rows of the page the "page" query param asks for, at the size
// the "per" param asks for (clamped to the last page, and to the first for anything
// unparseable), and where that page stands. Every list handler passes its full, already sorted and filtered rows.
func paginate[T any](r *http.Request, rows []T) ([]T, views.Pager) {
	pageSize := perPage(r)
	pages := max(1, (len(rows)+pageSize-1)/pageSize)
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil {
		page = 1
	}
	page = min(max(page, 1), pages)
	start := (page - 1) * pageSize
	end := min(start+pageSize, len(rows))
	return rows[start:end], views.Pager{Page: page, Pages: pages, Total: len(rows), PerPage: pageSize}
}
