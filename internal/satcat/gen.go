//go:build ignore

// gen.go rebuilds data/*.tsv.gz from the SAT's catCFDI spreadsheet, the official source of
// the CFDI 4.0 catalogs (Anexo 20, http://omawww.sat.gob.mx/tramitesyservicios/Paginas/anexo_20.htm).
// The spreadsheet is a legacy .xls; convert every sheet to UTF-8, tab-separated CSV first:
//
//	soffice --headless --convert-to 'csv:Text - txt - csv (StarCalc):9,34,76,1,,0,false,true,false,false,false,-1' catCFDI_V_4_YYYYMMDD.xls
//
// which writes one catCFDI_V_4_YYYYMMDD-<sheet>.csv per sheet. Then, from this directory:
//
//	go run gen.go <dir>/catCFDI_V_4_YYYYMMDD
//
// and update Version in satcat.go. The output is deterministic (gzip without a timestamp),
// so an unchanged catalog regenerates byte-identical files.
package main

import (
	"compress/gzip"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strings"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: go run gen.go <path>/catCFDI_V_4_YYYYMMDD")
	}
	prefix := os.Args[1]
	// c_ClaveProdServ: c_ClaveProdServ, Descripción, …, FechaFinVigencia (6), …, Palabras similares (8).
	n, err := convert(prefix+"-c_ClaveProdServ.csv", "data/claveprodserv.tsv.gz",
		regexp.MustCompile(`^[0-9]{8}$`), 1, 8, 6)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("c_ClaveProdServ:", n)
	// c_ClaveUnidad: c_ClaveUnidad, Nombre, Descripción, Nota, inicio, FechaFinVigencia (5), Símbolo.
	if n, err = convert(prefix+"-c_ClaveUnidad.csv", "data/claveunidad.tsv.gz",
		regexp.MustCompile(`^[A-Za-z0-9]{1,3}$`), 1, -1, 5); err != nil {
		log.Fatal(err)
	}
	fmt.Println("c_ClaveUnidad:", n)
}

// convert writes code, description and (when similarCol >= 0) the "palabras similares" of
// every row of in whose first column is a code, skipping the sheet's title and header rows
// and any code the SAT has given an end of validity.
func convert(in, out string, code *regexp.Regexp, descCol, similarCol, endCol int) (int, error) {
	f, err := os.Open(in)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comma = '\t'
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	o, err := os.Create(out)
	if err != nil {
		return 0, err
	}
	defer o.Close()
	z, err := gzip.NewWriterLevel(o, gzip.BestCompression)
	if err != nil {
		return 0, err
	}
	clean := strings.NewReplacer("\t", " ", "\r", " ", "\n", " ")
	seen := map[string]bool{}
	n := 0
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
		c := strings.TrimSpace(rec[0])
		if !code.MatchString(c) {
			continue
		}
		if endCol < len(rec) && strings.TrimSpace(rec[endCol]) != "" {
			continue
		}
		if seen[c] {
			return 0, fmt.Errorf("%s: duplicate code %s", in, c)
		}
		seen[c] = true
		line := c + "\t" + strings.Join(strings.Fields(clean.Replace(rec[descCol])), " ")
		if similarCol >= 0 && similarCol < len(rec) {
			if s := strings.Join(strings.Fields(clean.Replace(rec[similarCol])), " "); s != "" {
				line += "\t" + s
			}
		}
		if _, err := io.WriteString(z, line+"\n"); err != nil {
			return 0, err
		}
		n++
	}
	if err := z.Close(); err != nil {
		return 0, err
	}
	return n, o.Close()
}
