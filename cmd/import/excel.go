package main

import (
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// cellStr reads a cell's raw (unformatted) value, normalizing non-breaking spaces —
// the catalog workbook has them scattered through gauge labels — and trimming.
func cellStr(f *excelize.File, sheet, ref string) string {
	v, err := f.GetCellValue(sheet, ref, excelize.Options{RawCellValue: true})
	if err != nil {
		return ""
	}
	v = strings.ReplaceAll(v, " ", " ")
	return strings.TrimSpace(v)
}

// cellFloat reads a cell as a number. ok is false for blank or non-numeric cells.
func cellFloat(f *excelize.File, sheet, ref string) (v float64, ok bool) {
	s := cellStr(f, sheet, ref)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}
