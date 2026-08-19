package main

import (
	"testing"

	"github.com/xuri/excelize/v2"
)

// TestParseCCSACOmitsCost is a regression test for a real pricing bug caught during
// M2.3's PDF verification: CCS & AC's cost-before-margin column (F) used to be
// imported into CostMicros alongside KgPerMMicros, and internal/pricing.basePrice's
// field-presence dispatch silently picked CCA's cost/margin formula instead of CCS's
// correct kg_per_m * copper_price formula — every CCS/AC line was mispriced. See the
// parseCCSAC doc comment.
func TestParseCCSACOmitsCost(t *testing.T) {
	f := excelize.NewFile()
	const sheet = "CCS & AC"
	f.NewSheet(sheet)
	f.SetCellValue(sheet, "B4", "Cable CCS 30% ALAMBRE 4 (4 AWG)")
	f.SetCellValue(sheet, "E4", 0.1723)
	f.SetCellValue(sheet, "F4", 26.7065) // cost-before-margin — must NOT be imported
	f.SetCellValue(sheet, "P4", "Cable CCS 30% ALAMBRE 4 (4 AWG)")

	fam := parseCCSAC(f)
	if len(fam.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1", len(fam.Items))
	}
	it := fam.Items[0]
	if it.CostMicros != nil {
		t.Errorf("CostMicros = %v, want nil (CCS & AC has no independent margin — see parseCCSAC doc)", *it.CostMicros)
	}
	if it.KgPerMMicros == nil {
		t.Fatal("KgPerMMicros = nil, want set (the actual pricing input for this family)")
	}
	if got := it.KgPerMMicros.String(); got != "0.1723" {
		t.Errorf("KgPerMMicros = %q, want \"0.1723\"", got)
	}
}
