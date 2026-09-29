package views

import (
	"net/url"
	"strconv"
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
	Pager   Pager  // the page shown; the zero value means one unpaged page
	Anchor  string // fragment the pager's links and form jump to, for lists inside a longer page
}

// DefaultPerPage is the rows per page of a list that doesn't ask for another size, and
// PerPageChoices are the sizes the pager offers.
const DefaultPerPage = 20

var PerPageChoices = []int{10, 20, 50, 100}

// Pager is where a paged list stands. Page and Pages start at 1; Total counts every
// matching row, not just this page's.
type Pager struct {
	Page, Pages, Total, PerPage int
}

// PageHref is the page's URL for page n, with search, sort and filters kept. Page 1
// carries no param.
func (lv ListView) PageHref(n int) string {
	v := lv.values("")
	if n > 1 {
		v.Set("page", strconv.Itoa(n))
	}
	return withQuery(lv.Base, v) + lv.fragment()
}

func (lv ListView) fragment() string {
	if lv.Anchor == "" {
		return ""
	}
	return "#" + lv.Anchor
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
	if lv.Pager.PerPage != 0 && lv.Pager.PerPage != DefaultPerPage && skip != "per" {
		v.Set("per", strconv.Itoa(lv.Pager.PerPage))
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

// choiceFilter is a text column that offers its known values as a dropdown, falling
// back to the plain text box when there are none (too many to list).
func choiceFilter(values []string) ColumnFilter {
	if len(values) == 0 {
		return textFilter
	}
	opts := make([]FilterOption, len(values))
	for i, v := range values {
		opts[i] = FilterOption{Value: v, Label: v}
	}
	return ColumnFilter{Kind: store.FilterText, Options: opts}
}

// Choice lists for the enum column filters that don't depend on the database.
var (
	quoteStatusFilter = enumFilter(
		FilterOption{"borrador", "Borrador"}, FilterOption{"emitida", "Emitida"}, FilterOption{"pipeline", "Pipeline"},
		FilterOption{"oc_emitida", "OC emitida"}, FilterOption{"entregada", "Entregada"},
		FilterOption{"cerrada", "Entregada y cerrada"}, FilterOption{"revisada", "Revisada"})
	userRoleFilter   = enumFilter(FilterOption{"admin", "Administrador"}, FilterOption{"vendedor", "Vendedor"})
	userStatusFilter = enumFilter(FilterOption{"activo", "Activo"}, FilterOption{"deshabilitado", "Deshabilitado"})
)
