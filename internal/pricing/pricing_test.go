package pricing

import (
	"testing"

	"github.com/gerrygoo/cladex-web/internal/money"
)

func micros(f float64) *money.Micros {
	m := money.MicrosFromFloat(f)
	return &m
}

// ccaSettings matches the workbook's own snapshot values at the time these reference
// prices were captured (TIPO DE CAMBIO!C1048562, "Margen CCA (%)").
var ccaSettings = Settings{DefaultMargin: money.MicrosFromFloat(0.1234)}

// ccsSettings matches TIPO DE CAMBIO!C1048566, "Precio por Kilo" — the CCS & AC family
// prices out to exactly kg/m * copper price; see basePrice's doc comment for why the
// workbook's own margin math reduces to that.
var ccsSettings = Settings{CopperPrice: money.MicrosFromFloat(220)}

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
			got, err := UnitPrice(p, money.MilliFromFloat(1), nil, ccaSettings)
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

// TestUnitPrice_CCSAndAC reproduces every price from the workbook's "CCS & AC" sheet
// (hidden P:S mirror block) exactly, from the weight the importer already stores in
// products.kg_per_m_micros.
func TestUnitPrice_CCSAndAC(t *testing.T) {
	cases := []struct {
		desc      string
		kgPerM    float64
		wantPrice float64
	}{
		{"ALAMBRE 4 (4 AWG)", 0.1723, 37.905999999999999},
		{"7#10 LC DSA (2 AWG)", 0.3031, 66.682000000000002},
		{"7#9 LC DSA (1 AWG)", 0.3823, 84.105999999999995},
		{"7#8 LC DSA (1/0 AWG)", 0.482, 106.03999999999999},
		{"7#7 LC DSA (2/0 AWG)", 0.6078, 133.71600000000001},
		{"7#6 LC DSA (3/0 AWG)", 0.7664, 168.608},
		{"7#5 LC DSA (4/0 AWG)", 0.9664, 212.608},
		{"19#9 LC DSA (3/0 AWG)", 1.0416, 229.15199999999999},
		{"19#8 LC DSA (4/0 AWG)", 1.3135, 288.96999999999997},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			p := Product{KgPerMMicros: micros(c.kgPerM)}
			got, err := UnitPrice(p, money.MilliFromFloat(1), nil, ccsSettings)
			if err != nil {
				t.Fatalf("UnitPrice: %v", err)
			}
			want := money.MicrosFromFloat(c.wantPrice)
			if got != want {
				t.Errorf("UnitPrice(kg/m=%v) = %v, want %v", c.kgPerM, got, want)
			}
		})
	}
}

// TestUnitPrice_FlatCatalog covers ABASTILUM-style products: unit_price_micros is
// already the final MXN price (baked in at import time, FX and margin already
// applied), so the engine must return it unchanged.
func TestUnitPrice_FlatCatalog(t *testing.T) {
	p := Product{UnitPriceMicros: micros(37800)} // "POSTE... 4 MTS", workbook's D7
	got, err := UnitPrice(p, money.MilliFromFloat(1), nil, Settings{})
	if err != nil {
		t.Fatalf("UnitPrice: %v", err)
	}
	if want := money.MicrosFromFloat(37800); got != want {
		t.Errorf("UnitPrice(flat) = %v, want %v", got, want)
	}
}

func TestUnitPrice_USDConvertsByFXRate(t *testing.T) {
	p := Product{UnitPriceMicros: micros(10), Currency: "USD"}
	s := Settings{FXRate: money.MicrosFromFloat(18.5)}
	got, err := UnitPrice(p, money.MilliFromFloat(1), nil, s)
	if err != nil {
		t.Fatalf("UnitPrice: %v", err)
	}
	if want := money.MicrosFromFloat(185); got != want {
		t.Errorf("UnitPrice(USD) = %v, want %v", got, want)
	}
}

func TestUnitPrice_USDWithoutFXRateErrors(t *testing.T) {
	p := Product{UnitPriceMicros: micros(10), Currency: "USD"}
	if _, err := UnitPrice(p, money.MilliFromFloat(1), nil, Settings{}); err == nil {
		t.Error("UnitPrice(USD, no FX rate) = nil error, want error")
	}
}

func TestUnitPrice_PriceBreaks(t *testing.T) {
	p := Product{UnitPriceMicros: micros(100)}
	breaks := []PriceBreak{
		{MinQty: money.MilliFromFloat(100), UnitPriceMicros: money.MicrosFromFloat(90)},
		{MinQty: money.MilliFromFloat(500), UnitPriceMicros: money.MicrosFromFloat(80)},
	}
	cases := []struct {
		qty  float64
		want float64
	}{
		{1, 100},   // below every tier: base price
		{99, 100},  // just below the first tier
		{100, 90},  // exactly the first tier's threshold
		{499, 90},  // between tiers
		{500, 80},  // exactly the second tier's threshold
		{1000, 80}, // above every tier: highest still applies
	}
	for _, c := range cases {
		got, err := UnitPrice(p, money.MilliFromFloat(c.qty), breaks, Settings{})
		if err != nil {
			t.Fatalf("UnitPrice(qty=%v): %v", c.qty, err)
		}
		if want := money.MicrosFromFloat(c.want); got != want {
			t.Errorf("UnitPrice(qty=%v) = %v, want %v", c.qty, got, want)
		}
	}
}

func TestUnitPrice_NoPricingDataErrors(t *testing.T) {
	if _, err := UnitPrice(Product{}, money.MilliFromFloat(1), nil, Settings{}); err == nil {
		t.Error("UnitPrice(no pricing data) = nil error, want error")
	}
}

func TestUnitPrice_MarginOutOfRangeErrors(t *testing.T) {
	p := Product{CostMicros: micros(10)}
	for _, margin := range []float64{1.0, 1.5, -0.1} {
		s := Settings{DefaultMargin: money.MicrosFromFloat(margin)}
		if _, err := UnitPrice(p, money.MilliFromFloat(1), nil, s); err == nil {
			t.Errorf("UnitPrice(margin=%v) = nil error, want error", margin)
		}
	}
}

func TestUnitPrice_ZeroCopperPriceErrors(t *testing.T) {
	p := Product{KgPerMMicros: micros(1)}
	if _, err := UnitPrice(p, money.MilliFromFloat(1), nil, Settings{}); err == nil {
		t.Error("UnitPrice(copper price=0) = nil error, want error")
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
