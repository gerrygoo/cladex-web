package store

import (
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
// enum columns use the bare name; number and date columns use "<name>.min" and
// "<name>.max". Empty values are never stored.
type Filters map[string]string

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
func filterWhere(cols []filterColumn, f Filters) (string, []any) {
	var sb strings.Builder
	var args []any
	add := func(cond string, arg any) {
		sb.WriteString(" AND " + cond)
		args = append(args, arg)
	}
	for _, c := range cols {
		switch c.kind {
		case FilterText:
			if v := f[c.name]; v != "" {
				add(c.expr+` LIKE ? ESCAPE '\' COLLATE NOCASE`, "%"+escapeLike(v)+"%")
			}
		case FilterEnum:
			if v := f[c.name]; v != "" {
				add(c.expr+" = ?", v)
			}
		case FilterNumber:
			if n, ok := parseFilterNumber(f[c.name+".min"], c.scale); ok {
				add(c.expr+" >= ?", n)
			}
			if n, ok := parseFilterNumber(f[c.name+".max"], c.scale); ok {
				add(c.expr+" <= ?", n)
			}
		case FilterDate:
			if d, ok := parseFilterDate(f[c.name+".min"]); ok {
				add("substr("+c.expr+", 1, 10) >= ?", d)
			}
			if d, ok := parseFilterDate(f[c.name+".max"]); ok {
				add("substr("+c.expr+", 1, 10) <= ?", d)
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

func parseFilterDate(s string) (string, bool) {
	if _, err := time.Parse("2006-01-02", s); err != nil {
		return "", false
	}
	return s, true
}
