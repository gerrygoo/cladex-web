package store

import (
	"sort"
	"testing"
)

func TestNaturalLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"SKU-9", "SKU-10", true},
		{"SKU-10", "SKU-9", false},
		{"SKU-2", "SKU-10", true},
		{"a", "a", false},
		{"a", "b", true},
		{"SKU-01", "SKU-1", false}, // equal numeric value (leading zero ignored) => neither less
		{"SKU-1", "SKU-01", false},
		{"SKU-100", "SKU-100-A", true}, // shorter prefix sorts first
		{"", "a", true},
		{"a", "", false},
		{"", "", false},
		{"item2", "item10", true},
		{"item10", "item2", false},
	}
	for _, c := range cases {
		if got := naturalLess(c.a, c.b); got != c.want {
			t.Errorf("naturalLess(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestAWGLessOrdersThinToThickThenUngauged(t *testing.T) {
	descs := []string{"x", "THW 4/0 AWG", "THW 1/0 AWG", "cable (1 AWG)", "THW 14 AWG", "THW 2 AWG", "THW 12 AWG", "CABLE DESNUDO 14 AWG", "CABLE DESNUDO 2 AWG"}
	var ps []Product
	for _, d := range descs {
		ps = append(ps, Product{SKU: d, Description: d})
	}
	sort.SliceStable(ps, func(i, j int) bool { return awgLess(ps[i], ps[j], false) })
	want := []string{"THW 14 AWG", "THW 12 AWG", "THW 2 AWG", "cable (1 AWG)", "THW 1/0 AWG", "THW 4/0 AWG", "CABLE DESNUDO 14 AWG", "CABLE DESNUDO 2 AWG", "x"}
	for i, p := range ps {
		if p.Description != want[i] {
			t.Fatalf("asc[%d] = %q, want %q", i, p.Description, want[i])
		}
	}
}
