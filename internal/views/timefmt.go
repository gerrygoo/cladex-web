package views

import "time"

// timestampLayout is how every date and time reads in the app: 28/09/2026 - 16:42.
const timestampLayout = "02/01/2006 - 15:04"

// storedTimeLayouts are the shapes timestamps take in the database.
var storedTimeLayouts = []string{"2006-01-02T15:04:05.000Z", time.RFC3339Nano, time.RFC3339}

// parseStoredTime reads a database timestamp, which is always UTC.
func parseStoredTime(iso string) (time.Time, bool) {
	for _, layout := range storedTimeLayouts {
		if t, err := time.Parse(layout, iso); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// timestampFallback is the text a timestamp shows until /static/time.js swaps in the
// browser's own time zone (or for good when JavaScript is off): the same format, in UTC.
// A value that doesn't parse is shown as stored rather than hidden.
func timestampFallback(iso string) string {
	if t, ok := parseStoredTime(iso); ok {
		return t.UTC().Format(timestampLayout)
	}
	return iso
}

// dateText formats a plain calendar date ("2026-10-27", such as a quote's vigencia) as
// 27/10/2026. It has no time of day, so it is never shifted to another time zone.
func dateText(isoDate string) string {
	if t, err := time.Parse("2006-01-02", isoDate); err == nil {
		return t.Format("02/01/2006")
	}
	return isoDate
}

// isoTime is t as the <time datetime> value the browser script reads.
func isoTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}
