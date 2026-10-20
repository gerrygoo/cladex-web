package store

import (
	"context"
	"testing"
)

func TestNormalizeName(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"SONEPAR S.A. de C.V.", "sonepar"},
		{"Sonepar", "sonepar"},
		{"  Zasa  ", "zasa"},
		{"Construcciones Galván, S. de R.L. de C.V.", "construcciones galvan"},
		{"Grupo Peñoles S.A.P.I. de C.V.", "grupo penoles"},
		{"Despacho Ruiz S.C.", "despacho ruiz"},
		{"Industrias SA", "industrias"},
		{"Casa", "casa"}, // a name that merely ends in "sa" keeps it
	} {
		if got := normalizeName(c.in); got != c.want {
			t.Errorf("normalizeName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestSimilarCustomers pins issue #29's duplicate check: the same RFC (spaces, hyphens and
// case aside), or a normalized name equal to, containing or contained in another.
func TestSimilarCustomers(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for _, c := range []Customer{
		{Name: "SONEPAR", ContactName: "Alfonso Felix"},
		{Name: "Grupo PEME", RFC: "GPE-010101-AB1"},
		{Name: "ZASA"},
		{Name: "Pruebas"},
	} {
		if _, err := s.CreateCustomer(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	names := func(cs []Customer) []string {
		var out []string
		for _, c := range cs {
			out = append(out, c.Name)
		}
		return out
	}
	for _, c := range []struct {
		name, rfc string
		want      []string
	}{
		{"Sonepar S.A. de C.V.", "", []string{"SONEPAR"}},
		{"zasa", "", []string{"ZASA"}},
		{"Otra empresa", "gpe 010101 ab1", []string{"Grupo PEME"}},
		{"Pruebas Industriales del Norte", "", []string{"Pruebas"}},
		{"Nadie Parecido", "XAXX010101000", nil},
		{"Su", "", nil}, // too short to count as contained in anything
	} {
		got, err := s.SimilarCustomers(ctx, c.name, c.rfc)
		if err != nil {
			t.Fatal(err)
		}
		if g := names(got); len(g) != len(c.want) || (len(g) > 0 && g[0] != c.want[0]) {
			t.Errorf("SimilarCustomers(%q, %q) = %v, want %v", c.name, c.rfc, g, c.want)
		}
	}
}
