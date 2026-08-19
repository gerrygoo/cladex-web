package main

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/gerrygoo/cladex-web/internal/money"
)

// item is a parsed product, not yet written to the database.
type item struct {
	SKU             string
	Description     string
	CostMicros      *money.Micros
	UnitPriceMicros *money.Micros
	KgPerMMicros    *money.Micros
}

// family is one product_families row plus its parsed products.
type family struct {
	Name      string
	SheetName string
	Items     []item
}

// parseABASTILUM reads the hidden L3:M45 mirror block — the exact (Descripción, Precio)
// range the "Cotizador Alumbrado" quote form VLOOKUPs against. It's a clean, flattened
// list spanning three visually-separate categories in the raw sheet (postes, LEDVANCE,
// PHILIPS), so no category-boundary parsing is needed. These are flat catalog prices
// (no margin math at quote time), so they land in unit_price_micros, not cost_micros.
// There's no real SKU for this family, so one is synthesized from the description.
func parseABASTILUM(f *excelize.File) family {
	fam := family{Name: "ABASTILUM", SheetName: "ABASTILUM"}
	for r := 4; r <= 45; r++ {
		desc := cellStr(f, "ABASTILUM", fmt.Sprintf("L%d", r))
		if desc == "" || desc == "-" {
			continue
		}
		price, ok := cellFloat(f, "ABASTILUM", fmt.Sprintf("M%d", r))
		if !ok {
			log.Printf("ABASTILUM row %d: %q has no price, skipping", r, desc)
			continue
		}
		pm := money.MicrosFromFloat(price)
		fam.Items = append(fam.Items, item{
			SKU:             "abl-" + slugify(desc),
			Description:     desc,
			UnitPriceMicros: &pm,
		})
	}
	return fam
}

type gaugeCost struct {
	CostMicros   *money.Micros
	KgPerMMicros *money.Micros
}

// ccaLines maps a mirror description's line prefix (the text before " CALIBRE ") to the
// SKU prefix agreed for that line, and to the raw block's line label in column B — the
// two differ ("CABLE DESNUDO" in the description vs "DESNUDO" in the raw header).
var ccaLines = map[string]struct{ SKUPrefix, RawLine string }{
	"THW-2-LS":      {"cca", "THW-2-LS"},
	"CABLE DESNUDO": {"ccad", "DESNUDO"},
}

// parseCCA reads the CCA sheet. The raw data is two repeated blocks (THW-2-LS, then
// DESNUDO), each a header row ("Peso kg/m" in column C, product line name in B)
// followed by gauge rows (B=gauge, C=kg/m, E=cost before margin) until the next header
// or a blank row. The hidden X2:Y20 mirror is what the "Cotizador CCA" quote form
// VLOOKUPs against, and is walked as the authoritative product list; the raw blocks are
// only consulted to attach cost and kg/m, which the mirror doesn't carry.
//
// The mirror's price (already margin-adjusted) is deliberately not imported: CCA is a
// cost + margin family, margin is applied at quote time (M2), so only cost_micros is
// populated here.
func parseCCA(f *excelize.File) family {
	fam := family{Name: "CCA", SheetName: "CCA"}

	raw := map[string]map[string]gaugeCost{}
	var currentLine string
	for r := 1; r <= 200; r++ {
		b := cellStr(f, "CCA", fmt.Sprintf("B%d", r))
		c := cellStr(f, "CCA", fmt.Sprintf("C%d", r))
		if c == "Peso kg/m" {
			currentLine = strings.ToUpper(b)
			if raw[currentLine] == nil {
				raw[currentLine] = map[string]gaugeCost{}
			}
			continue
		}
		if currentLine == "" || b == "" {
			continue
		}
		var gc gaugeCost
		if cost, ok := cellFloat(f, "CCA", fmt.Sprintf("E%d", r)); ok {
			m := money.MicrosFromFloat(cost)
			gc.CostMicros = &m
		}
		if kgm, ok := cellFloat(f, "CCA", fmt.Sprintf("C%d", r)); ok {
			m := money.MicrosFromFloat(kgm)
			gc.KgPerMMicros = &m
		}
		raw[currentLine][strings.ToUpper(b)] = gc
	}

	for r := 2; r <= 200; r++ {
		desc := cellStr(f, "CCA", fmt.Sprintf("X%d", r))
		if desc == "" || desc == "-" {
			continue
		}
		linePrefix, gauge, ok := strings.Cut(desc, " CALIBRE ")
		if !ok {
			log.Printf("CCA row %d: could not split line/gauge from %q, skipping", r, desc)
			continue
		}
		gauge = strings.TrimSpace(gauge)
		info, ok := ccaLines[linePrefix]
		if !ok {
			log.Printf("CCA row %d: unknown product line %q, skipping", r, linePrefix)
			continue
		}
		gc, ok := raw[info.RawLine][strings.ToUpper(gauge)]
		if !ok {
			log.Printf("CCA row %d: no raw cost data for %s %s, skipping", r, info.RawLine, gauge)
			continue
		}
		fam.Items = append(fam.Items, item{
			SKU:          info.SKUPrefix + "-c" + gaugeSlug(gauge),
			Description:  desc,
			CostMicros:   gc.CostMicros,
			KgPerMMicros: gc.KgPerMMicros,
		})
	}
	return fam
}

// parseCCSAC reads the 'CCS & AC' sheet, rows 4-12, where the raw data (B=name, E=kg/m)
// and the hidden mirror (P=description) sit on the same rows, so no join is needed.
// Unlike CCA, this family's margin is copper-price-derived and cancels out entirely
// (see internal/pricing's package doc / docs/PLAN.md footnote 8): the real formula is
// exactly kg_per_m * copper_price, with no independent margin term, so only
// kg_per_m_micros is populated — column F ("cost before margin") is deliberately not
// imported. Populating both would make pricing.basePrice's field-presence dispatch
// silently pick CCA's cost/margin formula instead (its only real bug, caught during
// M2.3's PDF verification: it priced every CCS/AC line using the CCA margin).
//
// SKU deviates from the "ccs-c<awg>" scheme used for CCA: the raw AWG code (column C)
// isn't unique here — two products ("7#6 LC DSA" and "19#9 LC DSA") both carry "3/0" —
// so the SKU is built from the product name instead.
func parseCCSAC(f *excelize.File) family {
	const sheet = "CCS & AC"
	fam := family{Name: sheet, SheetName: sheet}
	for r := 4; r <= 12; r++ {
		name := cellStr(f, sheet, fmt.Sprintf("B%d", r))
		if name == "" {
			continue
		}
		desc := cellStr(f, sheet, fmt.Sprintf("P%d", r))
		if desc == "" || desc == "-" {
			log.Printf("CCS & AC row %d: %q has no mirror description, skipping", r, name)
			continue
		}
		it := item{SKU: "ccs-" + slugify(name), Description: desc}
		if kgm, ok := cellFloat(f, sheet, fmt.Sprintf("E%d", r)); ok {
			m := money.MicrosFromFloat(kgm)
			it.KgPerMMicros = &m
		}
		fam.Items = append(fam.Items, it)
	}
	return fam
}

// gaugeSlug turns a gauge label ("14 AWG", "1/0 AWG") into a SKU-safe suffix ("14",
// "1-0").
func gaugeSlug(gauge string) string {
	g := strings.TrimSpace(strings.TrimSuffix(gauge, "AWG"))
	g = strings.ReplaceAll(g, "/", "-")
	return strings.ToLower(g)
}

var accentReplacer = strings.NewReplacer(
	"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ñ", "n", "ü", "u",
	"Á", "a", "É", "e", "Í", "i", "Ó", "o", "Ú", "u", "Ñ", "n", "Ü", "u",
)

var nonSlugRun = regexp.MustCompile(`[^a-z0-9]+`)

// slugify turns a free-text description into a SKU-safe, deterministic slug — used for
// families that have no real manufacturer SKU, so re-running the import produces the
// same SKU for the same product.
func slugify(s string) string {
	s = accentReplacer.Replace(s)
	s = strings.ToLower(s)
	s = nonSlugRun.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}
