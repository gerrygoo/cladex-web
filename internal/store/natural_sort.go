package store

import (
	"regexp"
	"strconv"
	"strings"
)

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

var awgRe = regexp.MustCompile(`(?i)(\d+)(/0)?\s*AWG`)

// awgKey extracts a product's wire gauge from its description ("... 14 AWG", "(1/0 AWG)")
// as a number that grows with conductor size: 14 AWG → -14, 1 AWG → -1, 1/0 → 0,
// 4/0 → 3 (AWG counts down as wires get thicker, then the aught sizes count up).
func awgKey(description string) (int, bool) {
	m := awgRe.FindStringSubmatch(description)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	if m[2] != "" {
		return n - 1, true
	}
	return -n, true
}

// isDesnudo reports whether the product is bare cable ("CABLE DESNUDO ..."), which lists
// after the insulated products.
func isDesnudo(description string) bool {
	return strings.Contains(strings.ToLower(description), "desnudo")
}

// awgLess orders insulated products before bare (desnudo) ones, each thinnest to thickest
// gauge (thickest first when desc), then by natural SKU. Within a group, products with no
// AWG in their description sort after the gauged ones in either direction.
func awgLess(a, b Product, desc bool) bool {
	da, db := isDesnudo(a.Description), isDesnudo(b.Description)
	if da != db {
		return db
	}
	ka, oka := awgKey(a.Description)
	kb, okb := awgKey(b.Description)
	if oka != okb {
		return oka
	}
	if oka && ka != kb {
		if desc {
			return ka > kb
		}
		return ka < kb
	}
	if desc {
		return naturalLess(b.SKU, a.SKU)
	}
	return naturalLess(a.SKU, b.SKU)
}
