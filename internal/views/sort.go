package views

import "net/url"

// SortHref builds the URL for a sortable table header cell. base is the list page's
// path (e.g. "/productos"); query is the current search text (the "q" param, may be
// empty); col is this column's sort key; currentSort/currentDir are the list's active
// sort state, straight from the request's own query params. Clicking a column that's
// already the active sort toggles its direction; clicking any other column sorts by it
// ascending.
func SortHref(base, query, col, currentSort, currentDir string) string {
	dir := "asc"
	if currentSort == col && currentDir != "desc" {
		dir = "desc"
	}
	v := url.Values{}
	if query != "" {
		v.Set("q", query)
	}
	v.Set("sort", col)
	v.Set("dir", dir)
	return base + "?" + v.Encode()
}

// SortIndicator returns the arrow to show next to a column header if it's the active
// sort ("" otherwise).
func SortIndicator(col, currentSort, currentDir string) string {
	if currentSort != col {
		return ""
	}
	if currentDir == "desc" {
		return "▼"
	}
	return "▲"
}
