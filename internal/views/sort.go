package views

import (
	"net/url"
	"strings"

	"github.com/gerrygoo/cladex-web/internal/store"
)

// ListView is a list page's URL state: where it lives, the search text, the active
// sort and the active column filters. Every link and form on the page rebuilds its
// URL from it, so sorting, searching and filtering never drop each other.
type ListView struct {
	Base    string // the page's path, e.g. "/productos"
	Query   string // the "q" search text, may be empty
	Sort    string
	Dir     string
	Filters store.Filters
}

// filterParam is the URL param name for a Filters key ("sku" -> "f.sku").
func filterParam(key string) string { return "f." + key }

// values builds the query string for the page: search, sort and every active filter
// except those of column skip ("" keeps them all; "q" drops the search text instead).
func (lv ListView) values(skip string) url.Values {
	v := url.Values{}
	if lv.Query != "" && skip != "q" {
		v.Set("q", lv.Query)
	}
	if lv.Sort != "" {
		v.Set("sort", lv.Sort)
		v.Set("dir", lv.Dir)
	}
	for k, val := range lv.Filters {
		if skip != "" && columnOfFilterKey(k) == skip {
			continue
		}
		v.Set(filterParam(k), val)
	}
	return v
}

// columnOfFilterKey maps "total.min" to "total".
func columnOfFilterKey(key string) string {
	col, _, _ := strings.Cut(key, ".")
	return col
}

// HasFilters reports whether any column filter is active.
func (lv ListView) HasFilters() bool { return len(lv.Filters) > 0 }

// FilterActive reports whether column col has an active filter.
func (lv ListView) FilterActive(col string) bool {
	for k := range lv.Filters {
		if columnOfFilterKey(k) == col {
			return true
		}
	}
	return false
}

// FilterValue returns the active filter value for key ("" if none).
func (lv ListView) FilterValue(key string) string { return lv.Filters[key] }

// ClearHref is the page's URL without column col's filter ("" clears all filters).
func (lv ListView) ClearHref(col string) string {
	v := lv.values(col)
	if col == "" {
		for k := range v {
			if strings.HasPrefix(k, "f.") {
				v.Del(k)
			}
		}
	}
	return withQuery(lv.Base, v)
}

// SortHref builds the URL for a sortable table header cell. Clicking a column that's
// already the active sort toggles its direction; clicking any other column sorts by it
// ascending. Search and filters are kept.
func (lv ListView) SortHref(col string) string {
	dir := "asc"
	if lv.Sort == col && lv.Dir != "desc" {
		dir = "desc"
	}
	v := lv.values("")
	v.Set("sort", col)
	v.Set("dir", dir)
	return withQuery(lv.Base, v)
}

// SortIndicator returns the arrow to show next to a column header if it's the active
// sort ("" otherwise).
func (lv ListView) SortIndicator(col string) string {
	if lv.Sort != col {
		return ""
	}
	if lv.Dir == "desc" {
		return "▼"
	}
	return "▲"
}

func withQuery(base string, v url.Values) string {
	if len(v) == 0 {
		return base
	}
	return base + "?" + v.Encode()
}

// ColumnFilter describes the filter control a column header offers. Its Kind matches
// the store's filter for that column.
type ColumnFilter struct {
	Kind    store.FilterKind
	Options []FilterOption // FilterEnum only
}

// FilterOption is one choice of an enum column filter.
type FilterOption struct {
	Value, Label string
}

var (
	textFilter   = ColumnFilter{Kind: store.FilterText}
	numberFilter = ColumnFilter{Kind: store.FilterNumber}
	dateFilter   = ColumnFilter{Kind: store.FilterDate}
)

func enumFilter(opts ...FilterOption) ColumnFilter {
	return ColumnFilter{Kind: store.FilterEnum, Options: opts}
}

// Choice lists for the enum column filters that don't depend on the database.
var (
	quoteStatusFilter = enumFilter(
		FilterOption{"borrador", "Borrador"}, FilterOption{"emitida", "Emitida"}, FilterOption{"revisada", "Revisada"})
	userRoleFilter   = enumFilter(FilterOption{"admin", "Administrador"}, FilterOption{"vendedor", "Vendedor"})
	userStatusFilter = enumFilter(FilterOption{"activo", "Activo"}, FilterOption{"deshabilitado", "Deshabilitado"})
)
