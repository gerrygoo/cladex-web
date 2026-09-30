package store

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"
)

// FilterKind is the type of a filterable column, which decides both the control the
// list page shows for it and how the value is matched.
type FilterKind int

const (
	FilterText   FilterKind = iota // case-insensitive substring
	FilterEnum                     // exact match against one of a fixed set of values
	FilterNumber                   // inclusive min/max range
	FilterDate                     // inclusive min/max range of calendar days
)

// Filters holds a list page's active column filters, keyed by column name. Text and
// enum columns use the bare name (text columns also accept "<name>.is" for an exact
// match); number and date columns use "<name>.min" and "<name>.max". Empty values are
// never stored. Columns that offer options (enum, and text with ".is") take several
// values, any of which matches; the other kinds use only the first.
type Filters map[string][]string

// first is the filter's first value, "" when it isn't set.
func (f Filters) first(key string) string {
	if v := f[key]; len(v) > 0 {
		return v[0]
	}
	return ""
}

type filterColumn struct {
	name  string
	expr  string
	kind  FilterKind
	scale float64 // FilterNumber only: multiplies the typed value into the column's stored unit
}

// where builds the " AND ..." fragment (empty when nothing applies) and its arguments
// for the active filters. Only columns in cols are honored, and values are always bound
// as parameters, so f can come straight from the URL. Unparseable numbers and dates are
// ignored rather than rejected.
func filterWhere(ctx context.Context, cols []filterColumn, f Filters) (string, []any) {
	var sb strings.Builder
	var args []any
	add := func(cond string, arg any) {
		sb.WriteString(" AND " + cond)
		args = append(args, arg)
	}
	// addAny matches any of vals exactly: "expr IN (?, ?)" plus a collation suffix.
	addAny := func(expr, collate string, vals []string) {
		var marks []string
		for _, v := range vals {
			if v != "" {
				marks = append(marks, "?")
				args = append(args, v)
			}
		}
		if len(marks) > 0 {
			sb.WriteString(" AND " + expr + collate + " IN (" + strings.Join(marks, ", ") + ")")
		}
	}
	for _, c := range cols {
		switch c.kind {
		case FilterText:
			// "<name>.is" is the exact-match form a dropdown of known values submits;
			// the bare name stays the typed substring.
			addAny(c.expr, " COLLATE NOCASE", f[c.name+".is"])
			if v := f.first(c.name); v != "" {
				add(c.expr+` LIKE ? ESCAPE '\' COLLATE NOCASE`, "%"+escapeLike(v)+"%")
			}
		case FilterEnum:
			addAny(c.expr, "", f[c.name])
		case FilterNumber:
			if n, ok := parseFilterNumber(f.first(c.name+".min"), c.scale); ok {
				add(c.expr+" >= ?", n)
			}
			if n, ok := parseFilterNumber(f.first(c.name+".max"), c.scale); ok {
				add(c.expr+" <= ?", n)
			}
		case FilterDate:
			// The typed days are the viewer's own, so they bound the column at local
			// midnights: from the start of the first day up to (not including) the start
			// of the day after the last.
			loc := LocationFromContext(ctx)
			if d, ok := parseFilterDate(f.first(c.name+".min"), loc); ok {
				add(c.expr+" >= ?", d.UTC().Format(storedTimeLayout))
			}
			if d, ok := parseFilterDate(f.first(c.name+".max"), loc); ok {
				add(c.expr+" < ?", d.AddDate(0, 0, 1).UTC().Format(storedTimeLayout))
			}
		}
	}
	return sb.String(), args
}

func parseFilterNumber(s string, scale float64) (int64, bool) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, false
	}
	return int64(math.Round(n * scale)), true
}

// storedTimeLayout is how the database writes its UTC timestamps, so bounds in this
// layout compare correctly as text.
const storedTimeLayout = "2006-01-02T15:04:05.000Z"

// parseFilterDate reads a typed YYYY-MM-DD as midnight starting that day in loc.
func parseFilterDate(s string, loc *time.Location) (time.Time, bool) {
	d, err := time.ParseInLocation("2006-01-02", s, loc)
	if err != nil {
		return time.Time{}, false
	}
	return d, true
}

type locationKey struct{}

// WithLocation returns a context whose date filters read their days in loc: the
// viewer's time zone, while the database itself stays in UTC.
func WithLocation(ctx context.Context, loc *time.Location) context.Context {
	return context.WithValue(ctx, locationKey{}, loc)
}

// LocationFromContext is the zone set by WithLocation, or UTC.
func LocationFromContext(ctx context.Context) *time.Location {
	if loc, ok := ctx.Value(locationKey{}).(*time.Location); ok {
		return loc
	}
	return time.UTC
}
