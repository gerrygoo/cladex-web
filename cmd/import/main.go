// Command import parses the Cladex pricing workbook's catalog sheets into
// product_families and products. See docs/PLAN.md, slice 1.2.
//
// Run with -dry-run first and read the report — SKUs, descriptions, and cost/price
// figures are exactly what would be written, so it doubles as the "spot-check 5 SKUs by
// hand" verification step. The workbook itself is never committed to the repo; point
// -xlsx at wherever it actually lives.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/xuri/excelize/v2"

	cladex "github.com/gerrygoo/cladex-web"
	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/store"
)

func main() {
	xlsxPath := flag.String("xlsx", "", "path to the Cladex catalog workbook (.xlsx)")
	dbPath := flag.String("db", "", "path to the SQLite database; omit for -dry-run")
	dryRun := flag.Bool("dry-run", false, "print what would be imported without writing to the database")
	flag.Parse()

	if *xlsxPath == "" {
		fmt.Fprintln(os.Stderr, "usage: import -xlsx <path> [-db <path>] [-dry-run]")
		os.Exit(1)
	}
	if !*dryRun && *dbPath == "" {
		fmt.Fprintln(os.Stderr, "-db is required unless -dry-run is set")
		os.Exit(1)
	}

	f, err := excelize.OpenFile(*xlsxPath)
	if err != nil {
		log.Fatalf("open workbook: %v", err)
	}
	defer f.Close()

	families := []family{
		parseABASTILUM(f),
		parseCCA(f),
		parseCCSAC(f),
	}

	dupes := printReport(families)
	if dupes > 0 {
		log.Fatalf("%d duplicate SKU(s) found — fix before importing", dupes)
	}
	if *dryRun {
		return
	}

	// Catalog rewrites are audited as coming from the importer, so a price that moved
	// during a re-import is distinguishable from one a person edited.
	ctx := store.WithActor(context.Background(), store.Actor{Source: store.SourceImport})
	db, err := store.Open(ctx, *dbPath, cladex.MigrationsFS)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer db.Close()

	for _, fam := range families {
		familyID, err := db.UpsertFamily(ctx, fam.Name, fam.SheetName)
		if err != nil {
			log.Fatal(err)
		}
		for _, it := range fam.Items {
			err := db.UpsertProduct(ctx, store.Product{
				FamilyID:        familyID,
				SKU:             it.SKU,
				Description:     it.Description,
				KgPerMMicros:    it.KgPerMMicros,
				UnitPriceMicros: it.UnitPriceMicros,
				CostMicros:      it.CostMicros,
			})
			if err != nil {
				log.Fatal(err)
			}
		}
	}
	fmt.Println("import complete")
}

// printReport prints every parsed product and returns the number of SKU collisions
// found across families (sku is globally unique in the schema, not just per family).
func printReport(families []family) int {
	seen := map[string]string{}
	dupes := 0
	total := 0
	for _, fam := range families {
		fmt.Printf("== %s (%d products) ==\n", fam.Name, len(fam.Items))
		for _, it := range fam.Items {
			total++
			if prev, dup := seen[it.SKU]; dup {
				fmt.Printf("  !! DUPLICATE SKU %s: %q vs %q\n", it.SKU, prev, it.Description)
				dupes++
			}
			seen[it.SKU] = it.Description
			fmt.Printf("  %-22s %-55s cost=%-10s price=%-10s kg/m=%s\n",
				it.SKU, truncate(it.Description, 55),
				microsStr(it.CostMicros), microsStr(it.UnitPriceMicros), microsStr(it.KgPerMMicros))
		}
	}
	fmt.Printf("== total: %d products across %d families ==\n", total, len(families))
	return dupes
}

func microsStr(m *money.Micros) string {
	if m == nil {
		return "-"
	}
	return fmt.Sprintf("%.4f", float64(*m)/1_000_000)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
