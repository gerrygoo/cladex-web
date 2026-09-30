package pdf

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestTypstStringLiteralEscaping(t *testing.T) {
	cases := map[string]string{
		`plain`:         `"plain"`,
		`with "quotes"`: `"with \"quotes\""`,
		`back\slash`:    `"back\\slash"`,
		"line\nbreak":   `"line\nbreak"`,
		`# * _ [ ] $ @`: `"# * _ [ ] $ @"`, // markup metacharacters need no escaping in a string literal
	}
	for in, want := range cases {
		if got := typstStringLiteral(in); got != want {
			t.Errorf("typstStringLiteral(%q) = %q, want %q", in, got, want)
		}
	}
}

func sampleDoc() QuoteDocument {
	return QuoteDocument{
		Folio:        "QA0001",
		CustomerName: "Grupo PEME - Mario Pelcastre",
		Vendedor:     "Rodolfo Flores",
		Fecha:        "08/08/2026",
		Lines: []QuoteLineDoc{
			{Description: "THW-2-LS CALIBRE 14 AWG", Qty: "1", UnitPrice: "$6.32", Total: "$6.32"},
			{Description: "THW-2-LS CALIBRE 12 AWG", Qty: "1", UnitPrice: "$8.86", Total: "$8.86"},
		},
		Subtotal: "$15.18",
		IVA:      "$2.43",
		Total:    "$17.61",
		Terms: []string{
			"Precios en pesos mexicanos (MXN), no incluyen IVA",
			"Precios sujetos a cambios sin previo aviso",
			"Pago por adelantado para colocar OC",
		},
	}
}

// TestRenderQuoteProducesValidPDF requires the `typst` CLI (part of the documented
// local dev toolchain, see docs/PLAN.md 0.0a) — skips cleanly if it isn't on PATH.
func TestRenderQuoteProducesValidPDF(t *testing.T) {
	bytes, err := RenderQuote(context.Background(), sampleDoc())
	if err != nil {
		t.Fatalf("RenderQuote: %v", err)
	}
	if !strings.HasPrefix(string(bytes), "%PDF") {
		t.Fatalf("output doesn't look like a PDF: %q", bytes[:min(20, len(bytes))])
	}
}

// TestRenderQuoteEscapesUntrustedContent proves customer/product text containing
// Typst markup metacharacters and control characters — none of it under this app's
// control — compiles cleanly instead of breaking the template or, worse, being
// interpreted as Typst code (e.g. a "#" followed by a function call).
func TestRenderQuoteEscapesUntrustedContent(t *testing.T) {
	doc := sampleDoc()
	doc.CustomerName = `Cliente "malicioso" #read("secrets") *bold* \n [bracket] $math$`
	doc.Lines = []QuoteLineDoc{
		{Description: "Línea con \"comillas\", #hash, *asterisco*, backslash \\", Qty: "1", UnitPrice: "$1.00", Total: "$1.00"},
	}
	bytes, err := RenderQuote(context.Background(), doc)
	if err != nil {
		t.Fatalf("RenderQuote with adversarial content: %v", err)
	}
	if !strings.HasPrefix(string(bytes), "%PDF") {
		t.Fatalf("output doesn't look like a PDF")
	}
}

// TestRenderQuoteIsDeterministic is what lets issued quotes be regenerated instead of
// stored: with Created pinned, two renders of the same document are byte-identical
// (Typst would otherwise stamp the render time into the PDF metadata).
func TestRenderQuoteIsDeterministic(t *testing.T) {
	doc := sampleDoc()
	doc.Created = time.Date(2026, 8, 2, 15, 4, 5, 0, time.UTC)
	first, err := RenderQuote(context.Background(), doc)
	if err != nil {
		t.Fatalf("RenderQuote: %v", err)
	}
	time.Sleep(1100 * time.Millisecond) // cross a second boundary, Typst's timestamp resolution
	second, err := RenderQuote(context.Background(), doc)
	if err != nil {
		t.Fatalf("RenderQuote: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("two renders of the same document with the same Created differ")
	}
}

// TestQuoteTypstNamesTheCurrency pins that a quote's currency labels the amounts, and
// that without one the layout is exactly what it was before currencies existed.
func TestQuoteTypstNamesTheCurrency(t *testing.T) {
	doc := QuoteDocument{Folio: "QL0001", Subtotal: "$1,000.00", IVA: "$160.00", Total: "$1,160.00"}
	plain := buildQuoteTypst(doc)
	for _, label := range []string{"[Precio unitario]", "[Subtotal]", "[IVA]", "[*Total*]"} {
		if !strings.Contains(plain, label) {
			t.Errorf("layout without a currency lost %q", label)
		}
	}
	doc.Currency = "USD"
	usd := buildQuoteTypst(doc)
	for _, label := range []string{"[Precio unitario (USD)]", "[Subtotal (USD)]", "[IVA (USD)]", "[*Total (USD)*]"} {
		if !strings.Contains(usd, label) {
			t.Errorf("USD layout is missing %q", label)
		}
	}
}
