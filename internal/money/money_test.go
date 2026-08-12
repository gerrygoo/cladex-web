package money

import "testing"

func TestToCentavosHalfUp(t *testing.T) {
	cases := []struct {
		micros Micros
		want   Centavos
	}{
		{0, 0},
		{10_000, 1},        // exact: 1.00 centavo
		{15_000, 2},        // exactly half -> rounds up
		{14_999, 1},        // just under half -> rounds down
		{1_249_999, 125},   // 124.9999 -> 125
		{63_198_722, 6320}, // 6.319872233629933 MXN/m style value, truncated to micros
		{-15_000, -2},      // negative: half rounds away from zero
		{-14_999, -1},
	}
	for _, c := range cases {
		if got := c.micros.ToCentavosHalfUp(); got != c.want {
			t.Errorf("Micros(%d).ToCentavosHalfUp() = %d, want %d", c.micros, got, c.want)
		}
	}
}

func TestCentavosString(t *testing.T) {
	cases := []struct {
		c    Centavos
		want string
	}{
		{0, "$0.00"},
		{5, "$0.05"},
		{100, "$1.00"},
		{123456, "$1,234.56"},
		{100000000, "$1,000,000.00"},
		{-500, "-$5.00"},
	}
	for _, c := range cases {
		if got := c.c.String(); got != c.want {
			t.Errorf("Centavos(%d).String() = %q, want %q", c.c, got, c.want)
		}
	}
}

func TestMicrosFromFloat(t *testing.T) {
	cases := []struct {
		f    float64
		want Micros
	}{
		{0, 0},
		{6.319872233629933, 6_319_872}, // catalog's own per-metre price, rounds down
		{5.54, 5_540_000},
		{0.1723, 172_300},
		{37800, 37_800_000_000},
		{-1.5, -1_500_000},
	}
	for _, c := range cases {
		if got := MicrosFromFloat(c.f); got != c.want {
			t.Errorf("MicrosFromFloat(%v) = %d, want %d", c.f, got, c.want)
		}
	}
}

func TestRoundOnceInvariant(t *testing.T) {
	// Sanity check on the domain's own numbers: the spreadsheet's per-metre price.
	price := Micros(6_319_872) // truncated from 6.319872233629933
	got := price.ToCentavosHalfUp()
	want := Centavos(632)
	if got != want {
		t.Errorf("got %d centavos, want %d", got, want)
	}
}
