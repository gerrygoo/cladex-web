// Package satcat holds the SAT's full CFDI 4.0 catalogs of product/service codes
// (c_ClaveProdServ, ~52,500) and unit codes (c_ClaveUnidad, ~2,400): what a product's
// "Datos para facturar (SAT)" are checked against and searched in (issue #62).
//
// They are too big for a Go literal like the small catalogs in internal/store/sat.go, and
// they're reference data, not Cladex's records, so they don't go into the database: the
// official spreadsheet is reduced to two gzipped TSV files embedded in the binary (~620
// KB) and read into memory on first use. Updating them is regenerating those files from a
// newer spreadsheet (gen.go) and changing Version.
package satcat

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"sort"
	"strings"
	"sync"
)

// Version is the SAT spreadsheet the embedded catalogs come from: catCFDI_V_4_20261029.xls,
// downloaded on 2026-11-03 from the SAT's Anexo 20 page. Its c_ClaveProdServ is version 2,
// revision 2 (published 2025-12-31); its c_ClaveUnidad, version 1, revision 0.
const Version = "catCFDI_V_4_20261029"

//go:embed data/claveprodserv.tsv.gz
var prodServGz []byte

//go:embed data/claveunidad.tsv.gz
var unitGz []byte

// Code is one catalog entry: the code and the SAT's description (for a unit, its name).
// A product code's search key also holds the SAT's "palabras similares", never shown.
type Code struct {
	Code, Description string
	desc, key         string // folded description, and with the similar words: for search
}

// Label is how a code is shown: "MTR · Metro".
func (c Code) Label() string {
	return c.Code + " · " + c.Description
}

type catalog struct {
	once   sync.Once
	gz     []byte
	codes  []Code
	byCode map[string]int // upper-cased code → index
}

var (
	prodServ = &catalog{gz: prodServGz}
	units    = &catalog{gz: unitGz}
)

func (c *catalog) load() {
	c.once.Do(func() {
		zr, err := gzip.NewReader(bytes.NewReader(c.gz))
		if err != nil {
			panic("satcat: " + err.Error()) // embedded at build time; can't happen
		}
		c.byCode = map[string]int{}
		sc := bufio.NewScanner(zr)
		for sc.Scan() {
			f := strings.Split(sc.Text(), "\t")
			if len(f) < 2 {
				continue
			}
			e := Code{Code: f[0], Description: f[1]}
			e.desc = fold(e.Description)
			e.key = e.desc
			if len(f) > 2 {
				e.key += " " + fold(f[2])
			}
			c.byCode[strings.ToUpper(e.Code)] = len(c.codes)
			c.codes = append(c.codes, e)
		}
		if err := sc.Err(); err != nil {
			panic("satcat: " + err.Error())
		}
	})
}

func (c *catalog) lookup(code string) (Code, bool) {
	c.load()
	i, ok := c.byCode[strings.ToUpper(strings.TrimSpace(code))]
	if !ok {
		return Code{}, false
	}
	return c.codes[i], true
}

// search returns up to limit entries, best first: those whose code starts with q (the
// shortest code first), then those whose description begins with q, then those whose
// description holds q as a phrase (in those two, shortest description first), then every
// word of q, and last those that match only through the SAT's similar words. Case and
// accents are ignored; otherwise the catalog's own order (by code) holds.
func (c *catalog) search(q string, limit int) []Code {
	c.load()
	q = strings.TrimSpace(q)
	words := strings.Fields(fold(q))
	if len(words) == 0 {
		return nil
	}
	phrase := strings.Join(words, " ")
	upper := strings.ToUpper(q)
	var tiers [5][]Code
	for _, e := range c.codes {
		switch {
		case strings.HasPrefix(strings.ToUpper(e.Code), upper):
			tiers[0] = append(tiers[0], e)
		case !containsAll(e.key, words):
		case strings.HasPrefix(e.desc, phrase):
			tiers[1] = append(tiers[1], e)
		case strings.Contains(e.desc, phrase):
			tiers[2] = append(tiers[2], e)
		case containsAll(e.desc, words):
			tiers[3] = append(tiers[3], e)
		default:
			tiers[4] = append(tiers[4], e)
		}
	}
	sort.SliceStable(tiers[0], func(i, j int) bool { return len(tiers[0][i].Code) < len(tiers[0][j].Code) })
	// Among descriptions that begin with or hold the phrase, the shortest first: "Metro"
	// before "Metro cúbico por kilogramo".
	for _, t := range tiers[1:3] {
		sort.SliceStable(t, func(i, j int) bool { return len(t[i].desc) < len(t[j].desc) })
	}
	var out []Code
	for _, t := range tiers {
		out = append(out, t...)
		if len(out) >= limit {
			return out[:limit]
		}
	}
	return out
}

func containsAll(s string, words []string) bool {
	for _, w := range words {
		if !strings.Contains(s, w) {
			return false
		}
	}
	return true
}

// accents maps the accented letters the catalogs use to their plain form.
var accents = strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u",
	"ñ", "n", "à", "a", "è", "e", "ù", "u", "ë", "e")

// fold lower-cases s and strips its accents, so "electrico" finds "Eléctrico".
func fold(s string) string {
	return accents.Replace(strings.ToLower(s))
}

// ProdServ looks up a c_ClaveProdServ code.
func ProdServ(code string) (Code, bool) { return prodServ.lookup(code) }

// Unit looks up a c_ClaveUnidad code, ignoring case; the result carries the code as the
// SAT writes it.
func Unit(code string) (Code, bool) { return units.lookup(code) }

// SearchProdServ finds product/service codes by code prefix or by words of their
// description.
func SearchProdServ(q string, limit int) []Code { return prodServ.search(q, limit) }

// SearchUnits finds unit codes by code prefix or by words of their name.
func SearchUnits(q string, limit int) []Code { return units.search(q, limit) }

// CommonUnitCodes are the units Cladex invoices in, listed first in the product form's
// unit picker (issue #62), in this order.
var CommonUnitCodes = []string{"H87", "KGM", "MTR", "XBX", "XRO", "XRL", "XPK", "KT", "PR", "LTR", "E48", "ACT"}

// CommonUnits is CommonUnitCodes with their names.
func CommonUnits() []Code {
	out := make([]Code, 0, len(CommonUnitCodes))
	for _, c := range CommonUnitCodes {
		if u, ok := Unit(c); ok {
			out = append(out, u)
		}
	}
	return out
}

// Count is the number of codes in each catalog.
func Count() (prodServCodes, unitCodes int) {
	prodServ.load()
	units.load()
	return len(prodServ.codes), len(units.codes)
}
