package views

import (
	"strings"
	"testing"

	"github.com/gerrygoo/cladex-web/internal/store"
)

func TestQuotePDFName(t *testing.T) {
	snap := func(s string) *string { return &s }
	tests := []struct {
		name  string
		quote store.Quote
		want  string
	}{
		{"draft uses the current name", store.Quote{Folio: "QS0011", CustomerName: "SONEPAR"}, "QS0011_SONEPAR.pdf"},
		{"issued uses the printed name", store.Quote{Folio: "QS0011", CustomerName: "Sonepar México", CustomerNameSnapshot: snap("SONEPAR")}, "QS0011_SONEPAR.pdf"},
		{"revision folio, accents and spaces kept", store.Quote{Folio: "QS0025-R2", CustomerName: "José Camposeco"}, "QS0025-R2_José Camposeco.pdf"},
		{"illegal characters removed", store.Quote{Folio: "QA0003", CustomerName: `A/B: "C" <D>*?|\E`}, "QA0003_A B C D E.pdf"},
		{"whitespace and controls collapsed", store.Quote{Folio: "QI0004", CustomerName: "  Ferre\tteria\n  Sur  "}, "QI0004_Ferre teria Sur.pdf"},
		{"trailing dots trimmed", store.Quote{Folio: "QL0005", CustomerName: "Cables S.A."}, "QL0005_Cables S.A.pdf"},
		{"empty name", store.Quote{Folio: "QL0006", CustomerName: " /?: "}, "QL0006.pdf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := QuotePDFName(tt.quote); got != tt.want {
				t.Errorf("QuotePDFName = %q, want %q", got, tt.want)
			}
		})
	}

	long := QuotePDFName(store.Quote{Folio: "QA0007", CustomerName: strings.Repeat("ñ", 200)})
	if want := "QA0007_" + strings.Repeat("ñ", maxPDFNameCustomerRunes) + ".pdf"; long != want {
		t.Errorf("long name = %q, want it capped at %d runes", long, maxPDFNameCustomerRunes)
	}
}

func TestQuotePDFPath(t *testing.T) {
	got := QuotePDFPath(store.Quote{Folio: "QS0025-R2", CustomerName: "José Camposeco"})
	if want := "/cotizaciones/QS0025-R2/pdf/QS0025-R2_Jos%C3%A9%20Camposeco.pdf"; got != want {
		t.Errorf("QuotePDFPath = %q, want %q", got, want)
	}
}
