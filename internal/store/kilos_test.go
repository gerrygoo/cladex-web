package store

import (
	"testing"

	"github.com/gerrygoo/cladex-web/internal/money"
)

// #63 point 5's example: CCS 30% 7#7, 1,000 m at 0.6078 kg/m for $123,811.11 is 607.80 KGM
// at $203.70… a kilo, and the importe doesn't move.
func TestKilosAndUnitPrice(t *testing.T) {
	kilos := KilosOf(1_000_000, 607_800)
	if kilos != 607_800 {
		t.Fatalf("KilosOf = %v, want 607.800", kilos)
	}
	price := UnitPriceFor(12_381_111, kilos)
	if price != 203_703_702 {
		t.Errorf("UnitPriceFor = %v, want 203.703702", price)
	}
	// Quantity × unit price comes back to the importe to the centavo.
	if got := money.Centavos(money.RoundHalfUp(int64(kilos)*int64(price), 10_000_000)); got != 12_381_111 {
		t.Errorf("kilos × price = %v, want $123,811.11", got)
	}
	// Kilos round to the hundredth, as the quote shows them: 111 m × 0.3896 = 43.2456 → 43.25.
	if k := KilosOf(111_000, 389_600); k != 43_250 {
		t.Errorf("KilosOf(111 m, 0.3896) = %v, want 43.250", k)
	}
	if UnitPriceFor(100, 0) != 0 {
		t.Error("UnitPriceFor(0 kilos) isn't 0")
	}
}

// The c_UsoCFDI catalog's régimen column: business usos for a persona moral or
// actividad empresarial, deducciones personales for a sueldos y salarios receptor.
func TestCFDIUseAllowed(t *testing.T) {
	for _, c := range []struct {
		use, regime string
		want        bool
	}{
		{"G01", "601", true}, {"G03", "626", true}, {"G03", "605", false}, {"I04", "616", false},
		{"D01", "605", true}, {"D01", "601", false}, {"S01", "616", true}, {"CN01", "605", true},
		{"CN01", "601", false}, {"G03", "", true},
	} {
		if got := CFDIUseAllowed(c.use, c.regime); got != c.want {
			t.Errorf("CFDIUseAllowed(%s, %s) = %v, want %v", c.use, c.regime, got, c.want)
		}
	}
	for _, u := range CFDIUsesFor("605") {
		if u.Code == "G01" || u.Code == "CP01" {
			t.Errorf("CFDIUsesFor(605) offers %s", u.Code)
		}
	}
	if len(CFDIUsesFor("")) != len(CFDIUses)-1 {
		t.Error("an unknown régimen doesn't offer every uso but CP01")
	}
}
