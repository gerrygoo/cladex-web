package appenv

import "testing"

func TestParse(t *testing.T) {
	for _, c := range []struct {
		in                     string
		prod, explicitProd, ok bool
		banner, watermark      bool
	}{
		{in: "", prod: true, ok: true},
		{in: "production", prod: true, explicitProd: true, ok: true},
		{in: "staging", ok: true, banner: true, watermark: true},
		{in: "development", ok: true, banner: true, watermark: true},
		{in: "prod"},
		{in: "Production"},
	} {
		e, err := Parse(c.in)
		if (err == nil) != c.ok {
			t.Errorf("Parse(%q) err = %v", c.in, err)
			continue
		}
		if !c.ok {
			continue
		}
		if e.IsProduction() != c.prod || e.IsExplicitProduction() != c.explicitProd ||
			(e.Banner() != "") != c.banner || (e.Watermark() != "") != c.watermark {
			t.Errorf("Parse(%q) = %+v: prod %v explicit %v banner %q watermark %q", c.in, e, e.IsProduction(), e.IsExplicitProduction(), e.Banner(), e.Watermark())
		}
	}
}
