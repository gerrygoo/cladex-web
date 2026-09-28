package pricing

import (
	"errors"
	"testing"

	"github.com/gerrygoo/cladex-web/internal/money"
)

func micros(f float64) *money.Micros {
	m := money.MicrosFromFloat(f)
	return &m
}

// ccaMargin matches the workbook's own snapshot values at the time these reference
// prices were captured (TIPO DE CAMBIO!C1048562, "Margen CCA (%)").
var ccaMargin = marginPtr(money.MicrosFromFloat(0.1234))

func marginPtr(m money.Micros) *money.Micros { return &m }

// ccsMargin is the 29.55% margin option that stands in for CCS & AC's old
// copper-derived margin ((220 - 155) / 220 ≈ 29.5455%) once the family is costed from
// its CCS 30% content at $155/kg ('CCS & AC'!D4) instead of priced at kg/m × 220.
var ccsMargin = marginPtr(money.MicrosFromFloat(0.2955))

// ccsMaterial is 'CCS & AC'!D4: the family's material cost per kg.
var ccsMaterial = money.MicrosFromFloat(155)

// TestUnitPrice_CCA reproduces every THW-2-LS and CABLE DESNUDO price from the
// workbook's "CCA" sheet (hidden X:Y mirror block, the same range the "Cotizador CCA"
// quote form VLOOKUPs against) exactly, from the underlying cost the importer already
// stores in products.cost_micros.
func TestUnitPrice_CCA(t *testing.T) {
	cases := []struct {
		desc      string
		costMXN   float64
		wantPrice float64
	}{
		{"THW-2-LS 14 AWG", 5.54, 6.3198722336299333},
		{"THW-2-LS 12 AWG", 7.77, 8.8637919233401767},
		{"THW-2-LS 10 AWG", 12.3, 14.031485284052019},
		{"THW-2-LS 8 AWG", 19.75, 22.530230435774584},
		{"THW-2-LS 6 AWG", 31.52, 35.957107004334929},
		{"THW-2-LS 4 AWG", 48.98, 55.874971480720959},
		{"THW-2-LS 2 AWG", 76.64, 87.428701802418431},
		{"THW-2-LS 1/0 AWG", 121.92, 139.08281998631074},
		{"THW-2-LS 2/0 AWG", 152.71, 174.207164042893},
		{"THW-2-LS 3/0 AWG", 191.24, 218.16107688797626},
		{"THW-2-LS 4/0 AWG", 239.59, 273.31736253707504},
		{"CABLE DESNUDO 14 AWG", 4.71, 5.3730321697467485},
		{"CABLE DESNUDO 12 AWG", 6.62, 7.551905087839379},
		{"CABLE DESNUDO 10 AWG", 10.47, 11.943874058863791},
		{"CABLE DESNUDO 8 AWG", 16.78, 19.142140086698607},
		{"CABLE DESNUDO 6 AWG", 26.77, 30.538443988135977},
		{"CABLE DESNUDO 4 AWG", 41.61, 47.467488021902803},
		{"CABLE DESNUDO 2 AWG", 65.14, 74.30983344741044},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			p := Product{CostMicros: micros(c.costMXN)}
			got, err := UnitPrice(p, ccaMargin)
			if err != nil {
				t.Fatalf("UnitPrice: %v", err)
			}
			want := money.MicrosFromFloat(c.wantPrice)
			if got != want {
				t.Errorf("UnitPrice(cost=%v) = %v, want %v", c.costMXN, got, want)
			}
		})
	}
}

// TestUnitPrice_CCSAndAC prices every CCS & AC product from its CCS 30% content and the
// 29.55% margin option, and checks it against the workbook's "CCS & AC" sheet (hidden
// P:S mirror block, kg/m × 220). 29.55% is a rounding of the workbook's repeating
// 65/220, so the unit prices come out a hair high: equal to the centavo for three
// products and 1–2 centavos above for the other six, which docs/PLAN.md (M3) accepts.
func TestUnitPrice_CCSAndAC(t *testing.T) {
	cases := []struct {
		desc       string
		kgPerM     float64
		wantMicros money.Micros // exact, from cost / (1 - 0.2955)
		workbook   float64      // the workbook's own price, kg/m × 220
	}{
		{"ALAMBRE 4 (4 AWG)", 0.1723, 37_908_446, 37.906},
		{"7#10 LC DSA (2 AWG)", 0.3031, 66_686_302, 66.682},
		{"7#9 LC DSA (1 AWG)", 0.3823, 84_111_427, 84.106},
		{"7#8 LC DSA (1/0 AWG)", 0.482, 106_046_842, 106.04},
		{"7#7 LC DSA (2/0 AWG)", 0.6078, 133_724_627, 133.716},
		{"7#6 LC DSA (3/0 AWG)", 0.7664, 168_618_879, 168.608},
		{"7#5 LC DSA (4/0 AWG)", 0.9664, 212_621_718, 212.608},
		{"19#9 LC DSA (3/0 AWG)", 1.0416, 229_166_785, 229.152},
		{"19#8 LC DSA (4/0 AWG)", 1.3135, 288_988_644, 288.97},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			p := Product{Materials: []MaterialContent{{QtyPerUnit: money.MicrosFromFloat(c.kgPerM), Price: ccsMaterial}}}
			got, err := UnitPrice(p, ccsMargin)
			if err != nil {
				t.Fatalf("UnitPrice: %v", err)
			}
			if got != c.wantMicros {
				t.Errorf("UnitPrice(kg/m=%v) = %v, want %v", c.kgPerM, got, c.wantMicros)
			}
			drift := got.ToCentavosHalfUp() - money.MicrosFromFloat(c.workbook).ToCentavosHalfUp()
			if drift < 0 || drift > 2 {
				t.Errorf("UnitPrice(kg/m=%v) = %v, %d centavos from the workbook's %v; want 0-2", c.kgPerM, got, drift, c.workbook)
			}
		})
	}
}

// A product's cost is its flat cost plus its materials, and the margin applies to the
// whole.
func TestUnitPrice_FlatCostPlusMaterials(t *testing.T) {
	p := Product{
		CostMicros: micros(10),
		Materials: []MaterialContent{
			{QtyPerUnit: money.MicrosFromFloat(0.5), Price: money.MicrosFromFloat(20)}, // $10
			{QtyPerUnit: money.MicrosFromFloat(2), Price: money.MicrosFromFloat(15)},   // $30
		},
	}
	got, err := UnitPrice(p, marginPtr(money.MicrosFromFloat(0.2)))
	if err != nil {
		t.Fatalf("UnitPrice: %v", err)
	}
	if want := money.MicrosFromFloat(62.5); got != want { // $50 / 0.80
		t.Errorf("UnitPrice = %v, want %v", got, want)
	}
}

func TestUnitPrice_NoPricingDataErrors(t *testing.T) {
	if _, err := UnitPrice(Product{}, ccaMargin); !errors.Is(err, ErrNoCost) {
		t.Errorf("UnitPrice(no pricing data) error = %v, want ErrNoCost", err)
	}
}

func TestUnitPrice_MarginOutOfRangeErrors(t *testing.T) {
	p := Product{CostMicros: micros(10)}
	for _, margin := range []float64{1.0, 1.5, -0.1} {
		if _, err := UnitPrice(p, marginPtr(money.MicrosFromFloat(margin))); err == nil {
			t.Errorf("UnitPrice(margin=%v) = nil error, want error", margin)
		}
	}
}

// A quote whose margin option is gone can't price anything catalog-priced.
func TestUnitPrice_NoMargin(t *testing.T) {
	if _, err := UnitPrice(Product{CostMicros: micros(10)}, nil); !errors.Is(err, ErrNoMargin) {
		t.Errorf("UnitPrice(cost, no margin) error = %v, want ErrNoMargin", err)
	}
}

// TestComputeTotals reproduces the "Cotizador CCA" sample quote's own subtotal/IVA/
// total (QA0105, workbook cells E43:E45): 11 THW-2-LS lines plus 7 CABLE DESNUDO lines,
// each qty=1. The workbook sums unrounded float prices before rounding once at the
// end; this engine rounds each line to centavos first (internal/money's documented
// design), so the expected total here is computed the engine's way, not copied
// verbatim from the workbook's own (unrounded-until-the-end) total.
func TestComputeTotals(t *testing.T) {
	prices := []float64{
		6.3198722336299333, 8.8637919233401767, 14.031485284052019,
		22.530230435774584, 35.957107004334929, 55.874971480720959,
		87.428701802418431, 139.08281998631074, 174.207164042893,
		218.16107688797626, 273.31736253707504,
		5.3730321697467485, 7.551905087839379, 11.943874058863791,
		19.142140086698607, 30.538443988135977, 47.467488021902803,
		74.30983344741044,
	}
	var lines []Line
	var wantSubtotal money.Centavos
	for _, p := range prices {
		unitPrice := money.MicrosFromFloat(p)
		lines = append(lines, Line{UnitPriceMicros: unitPrice, QtyMilli: money.MilliFromFloat(1)})
		wantSubtotal += unitPrice.ToCentavosHalfUp()
	}

	got := ComputeTotals(lines)
	if got.Subtotal != wantSubtotal {
		t.Errorf("Subtotal = %v, want %v", got.Subtotal, wantSubtotal)
	}
	wantIVA := money.ApplyRate(wantSubtotal, IVARate)
	if got.IVA != wantIVA {
		t.Errorf("IVA = %v, want %v", got.IVA, wantIVA)
	}
	if got.Total != wantSubtotal+wantIVA {
		t.Errorf("Total = %v, want %v", got.Total, wantSubtotal+wantIVA)
	}
}

// TestConvertQty checks the fixed-point arithmetic directly: qty (Milli, 1e-3) times
// rate (Micros, 1e-6) should reduce to a Milli result via the same RoundHalfUp rule
// used everywhere else in this package.
func TestConvertQty(t *testing.T) {
	cases := []struct {
		desc    string
		qty     float64
		rate    float64
		wantQty float64
	}{
		{"3 rollos at 100 m/rollo -> 300 m", 3, 100, 300},
		{"2.5 kg at 3.937 m/kg -> 9.8425 m, rounds to 9.843", 2.5, 3.937, 9.843},
		{"1 unit at 1:1 rate is unchanged", 1, 1, 1},
		{"0 qty converts to 0", 0, 100, 0},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			got := ConvertQty(money.MilliFromFloat(c.qty), money.MicrosFromFloat(c.rate))
			want := money.MilliFromFloat(c.wantQty)
			if got != want {
				t.Errorf("ConvertQty(%v, %v) = %v, want %v", c.qty, c.rate, got, want)
			}
		})
	}
}

func TestPriceMarginRoundTrip(t *testing.T) {
	cost := money.Micros(160_000_000)
	price, err := PriceFromMargin(cost, 295_500)
	if err != nil {
		t.Fatal(err)
	}
	if price != 227_111_427 { // 160 / (1 - 0.2955)
		t.Errorf("PriceFromMargin = %v, want 227.111427", price)
	}
	m, err := MarginFromPrice(cost, 220_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if m != 272_727 { // 1 - 160/220
		t.Errorf("MarginFromPrice = %v, want 0.272727", m)
	}
	if m, err := MarginFromPrice(cost, cost); err != nil || m != 0 {
		t.Errorf("MarginFromPrice(cost, cost) = %v, %v; want 0, nil", m, err)
	}
	if _, err := MarginFromPrice(cost, cost-1); err == nil {
		t.Error("MarginFromPrice below cost: want error")
	}
	if _, err := PriceFromMargin(cost, 1_000_000); err == nil {
		t.Error("PriceFromMargin at 100%: want error")
	}
}
