package store

import (
	"context"
	"fmt"
	"strings"
)

// accentFolder drops accents and the dieresis, so "Galván" and "GALVAN" compare equal.
var accentFolder = strings.NewReplacer(
	"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n",
	"Á", "a", "É", "e", "Í", "i", "Ó", "o", "Ú", "u", "Ü", "u", "Ñ", "n",
)

// legalSuffixes are the endings of a razón social, as normalizeName leaves them (no dots
// or commas), longest first so "sa de cv" goes before "sa".
var legalSuffixes = []string{"sapi de cv", "s de rl de cv", "s de rl", "sa de cv", "sa", "sc"}

// normalizeName is a customer name as the duplicate check compares it (issue #29): lower
// case, no accents, dots or commas, single spaces, and without its razón social ending,
// so "SONEPAR S.A. de C.V." and "Sonepar" are both "sonepar".
func normalizeName(name string) string {
	n := accentFolder.Replace(strings.ToLower(name))
	n = strings.NewReplacer(".", "", ",", " ").Replace(n)
	n = strings.Join(strings.Fields(n), " ")
	for _, suf := range legalSuffixes {
		if trimmed, ok := strings.CutSuffix(n, " "+suf); ok {
			n = strings.TrimSpace(trimmed)
			break
		}
	}
	return n
}

// normalizeRFC is an RFC without spaces or hyphens, upper case.
func normalizeRFC(rfc string) string {
	return strings.ToUpper(strings.NewReplacer(" ", "", "-", "").Replace(rfc))
}

// minContainedName is the shortest normalized name that counts as contained in another:
// below it, a name like "a" would match half the catalog.
const minContainedName = 3

// SimilarCustomers lists the customers a new one with this name and RFC may duplicate
// (issue #29): the same RFC, ignoring spaces, hyphens and case, or a name that, normalized
// (see normalizeName), is equal to the new one or contains it whole, or is contained in
// it. It only warns; nothing stops creating the customer anyway.
func (s *Store) SimilarCustomers(ctx context.Context, name, rfc string) ([]Customer, error) {
	all, err := s.ListCustomers(ctx, "", "name", "asc", nil)
	if err != nil {
		return nil, fmt.Errorf("store: similar customers: %w", err)
	}
	wantName, wantRFC := normalizeName(name), normalizeRFC(rfc)
	var out []Customer
	for _, c := range all {
		if wantRFC != "" && normalizeRFC(c.RFC) == wantRFC {
			out = append(out, c)
			continue
		}
		have := normalizeName(c.Name)
		if wantName == "" || have == "" {
			continue
		}
		if have == wantName ||
			(len(wantName) >= minContainedName && strings.Contains(have, wantName)) ||
			(len(have) >= minContainedName && strings.Contains(wantName, have)) {
			out = append(out, c)
		}
	}
	return out, nil
}
