package store

// naturalLess reports whether a sorts before b under "natural" order: runs of digits
// compare by numeric value rather than byte-by-byte, so "SKU-9" sorts before "SKU-10".
// Plain SQL ORDER BY can't express this, so sort columns that need it are fetched
// unsorted (or sorted by a stable tiebreaker) and re-sorted in Go with this — see
// ListProducts' handling of the "sku" column.
func naturalLess(a, b string) bool {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		ca, cb := a[i], b[j]
		if isDigit(ca) && isDigit(cb) {
			ai, bj := i, j
			for ai < len(a) && isDigit(a[ai]) {
				ai++
			}
			for bj < len(b) && isDigit(b[bj]) {
				bj++
			}
			na, nb := trimLeadingZeros(a[i:ai]), trimLeadingZeros(b[j:bj])
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			i, j = ai, bj
			continue
		}
		if ca != cb {
			return ca < cb
		}
		i++
		j++
	}
	return len(a)-i < len(b)-j
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func trimLeadingZeros(s string) string {
	i := 0
	for i < len(s)-1 && s[i] == '0' {
		i++
	}
	return s[i:]
}
