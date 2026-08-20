package store

import "testing"

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
