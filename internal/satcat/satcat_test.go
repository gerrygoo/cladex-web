package satcat

import (
	"slices"
	"testing"
	"time"
)

func TestCatalogsLoadWhole(t *testing.T) {
	start := time.Now()
	ps, u := Count()
	t.Logf("loaded in %v", time.Since(start))
	// The full official catalogs, not a sample: catCFDI_V_4_20261029 has 52,513 product
	// codes and 2,419 unit codes in force.
	if ps != 52513 || u != 2419 {
		t.Errorf("Count() = %d, %d; want 52513, 2419", ps, u)
	}
}

func TestLookup(t *testing.T) {
	for _, tc := range []struct {
		code, want string
		ok         bool
	}{
		{"26121600", "Cables eléctricos y accesorios", true},
		{"01010101", "No existe en el catálogo", true},
		{" 39111500 ", "Iluminación de interiores y artefactos", true},
		{"26121699", "", false},
		{"2612160", "", false},
		{"", "", false},
	} {
		c, ok := ProdServ(tc.code)
		if ok != tc.ok || c.Description != tc.want {
			t.Errorf("ProdServ(%q) = %q, %v; want %q, %v", tc.code, c.Description, ok, tc.want, tc.ok)
		}
	}
	if u, ok := Unit("mtr"); !ok || u.Code != "MTR" || u.Label() != "MTR · Metro" {
		t.Errorf("Unit(mtr) = %+v, %v", u, ok)
	}
	if u, ok := Unit("ohm"); !ok || u.Code != "Ohm" {
		t.Errorf("Unit(ohm) = %+v, %v; want the SAT's own spelling, Ohm", u, ok)
	}
	if _, ok := Unit("QQQ"); ok {
		t.Error("Unit(QQQ) should not exist")
	}
}

func TestSearchProdServ(t *testing.T) {
	codes := func(cs []Code) []string {
		var out []string
		for _, c := range cs {
			out = append(out, c.Code)
		}
		return out
	}
	// By words, accents and case ignored; the general entry first.
	got := SearchProdServ("CABLES electricos", 10)
	if len(got) == 0 || got[0].Code != "26121600" {
		t.Errorf("SearchProdServ(cables electricos) = %v; want 26121600 first", codes(got))
	}
	// By code prefix.
	got = SearchProdServ("261216", 50)
	if len(got) < 10 || got[0].Code != "26121600" {
		t.Errorf("SearchProdServ(261216) = %v", codes(got))
	}
	for _, c := range got {
		if c.Code[:6] != "261216" {
			t.Errorf("SearchProdServ(261216) returned %s", c.Code)
		}
	}
	// The limit holds, and nothing comes back for nothing.
	if got := SearchProdServ("cable", 5); len(got) != 5 {
		t.Errorf("SearchProdServ(cable, 5) returned %d", len(got))
	}
	if got := SearchProdServ("  ", 5); got != nil {
		t.Errorf("SearchProdServ(blank) = %v", codes(got))
	}
	if got := SearchProdServ("zzzzqqq", 5); len(got) != 0 {
		t.Errorf("SearchProdServ(zzzzqqq) = %v", codes(got))
	}
}

func TestSearchUnitsAndCommon(t *testing.T) {
	got := SearchUnits("carrete", 10)
	found := slices.ContainsFunc(got, func(c Code) bool { return c.Code == "XRL" })
	if !found {
		t.Errorf("SearchUnits(carrete) = %v; want XRL among them", got)
	}
	if got := SearchUnits("metro", 3); len(got) == 0 || got[0].Code != "MTR" {
		t.Errorf("SearchUnits(metro) = %v; want MTR first", got)
	}
	if got := SearchUnits("kgm", 3); len(got) == 0 || got[0].Code != "KGM" {
		t.Errorf("SearchUnits(kgm) = %v; want KGM first", got)
	}
	common := CommonUnits()
	if len(common) != len(CommonUnitCodes) {
		t.Fatalf("CommonUnits() has %d of %d: a listed code isn't in the catalog", len(common), len(CommonUnitCodes))
	}
	want := map[string]string{"H87": "Pieza", "KGM": "Kilogramo", "MTR": "Metro", "XBX": "Caja", "XRO": "Rollo",
		"XRL": "Carrete", "XPK": "Paquete", "KT": "Kit", "PR": "Par", "LTR": "Litro", "E48": "Unidad de servicio", "ACT": "Actividad"}
	for i, c := range common {
		if c.Code != CommonUnitCodes[i] || want[c.Code] != c.Description {
			t.Errorf("CommonUnits()[%d] = %s; want %s · %s", i, c.Label(), CommonUnitCodes[i], want[CommonUnitCodes[i]])
		}
	}
}
