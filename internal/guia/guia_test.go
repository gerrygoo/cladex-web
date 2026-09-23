package guia

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

// TestRealGuideBuilds renders docs/guia for both roles, which checks every internal
// link, anchor and [[término]] in the shipped guide.
func TestRealGuideBuilds(t *testing.T) {
	g, err := New(os.DirFS("../../docs/guia"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := g.Page("administracion", false); ok {
		t.Error("vendedor can see the administración page")
	}
	if _, ok := g.Page("administracion", true); !ok {
		t.Error("admin cannot see the administración page")
	}
	idx, _ := g.Page("", false)
	if strings.Contains(idx.HTML, "/ayuda/administracion") {
		t.Error("vendedor index links to the admin page")
	}
	for url, want := range map[string]bool{
		"/ayuda": true,
		"/ayuda/cotizaciones#emitir-la-cotización": true,
		"/ayuda/cotizaciones#no-existe":            false,
		"/ayuda/administracion":                    false, // vendedor
		"/clientes":                                false,
	} {
		if got := g.Resolves(url, false); got != want {
			t.Errorf("Resolves(%q, vendedor) = %v, want %v", url, got, want)
		}
	}
	if !g.Resolves("/ayuda/administracion#unidades", true) {
		t.Error("admin cannot resolve /ayuda/administracion#unidades")
	}
	cot, _ := g.Page("cotizaciones", false)
	if !strings.Contains(cot.HTML, `id="emitir-la-cotización"`) {
		t.Error("cotizaciones is missing GitHub-style accented heading ids")
	}
	if !strings.Contains(cot.HTML, `class="term"`) {
		t.Error("cotizaciones renders no glossary tooltips")
	}
}

func guideFS(files map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for _, d := range pageDefs {
		fsys[d.file] = &fstest.MapFile{Data: []byte("# " + d.file + "\n")}
	}
	fsys[glossaryFile] = &fstest.MapFile{Data: []byte("# Glosario\n\n## Folio\n\nIdentificador *único*.\n")}
	for name, body := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(body)}
	}
	return fsys
}

func TestLinksAndAnchors(t *testing.T) {
	g, err := New(guideFS(map[string]string{
		"README.md":       "# Índice\n\n[emitir](cotizaciones.md#emitir-la-cotización) [aquí](#índice) [nas](../NAS_OPERATIONS.md)\n",
		"cotizaciones.md": "# Cotizaciones\n\n## Emitir la cotización\n",
	}))
	if err != nil {
		t.Fatal(err)
	}
	idx, _ := g.Page("", false)
	// Goldmark percent-encodes non-ASCII fragments; browsers decode them before matching ids.
	for _, want := range []string{
		`href="/ayuda/cotizaciones#emitir-la-cotizaci%C3%B3n"`,
		`href="#%C3%ADndice"`,
		`href="https://github.com/gerrygoo/cladex-web/blob/main/docs/NAS_OPERATIONS.md"`,
	} {
		if !strings.Contains(idx.HTML, want) {
			t.Errorf("missing %s in:\n%s", want, idx.HTML)
		}
	}
	if idx.Title != "Índice" || idx.URL != "/ayuda" {
		t.Errorf("title/url = %q %q", idx.Title, idx.URL)
	}
}

func TestBrokenReferencesFail(t *testing.T) {
	for name, readme := range map[string]string{
		"missing anchor":        "# I\n\n[x](cotizaciones.md#no-existe)\n",
		"missing page":          "# I\n\n[x](nada.md)\n",
		"missing term":          "# I\n\n[[nada]]\n",
		"term inside link":      "# I\n\n[ver [[folio]]](acceso.md)\n",
		"unterminated admin":    "# I\n\n:::admin\nsecreto\n",
		"anchor hidden by role": "# I\n\n[x](#secreto)\n\n:::admin\n## Secreto\n:::\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := New(guideFS(map[string]string{"README.md": readme})); err == nil {
				t.Error("New succeeded, want error")
			}
		})
	}
}

func TestUnlistedFileFails(t *testing.T) {
	if _, err := New(guideFS(map[string]string{"extra.md": "# Extra\n"})); err == nil {
		t.Error("New succeeded with a page missing from pageDefs")
	}
}

func TestAdminBlocksAndLinks(t *testing.T) {
	g, err := New(guideFS(map[string]string{
		"README.md": "# I\n\nPara todos.\n\n:::admin\nSolo admins.\n:::\n\nVer [Ajustes](administracion.md).\n",
	}))
	if err != nil {
		t.Fatal(err)
	}
	vend, _ := g.Page("", false)
	admin, _ := g.Page("", true)
	if strings.Contains(vend.HTML, "Solo admins") || !strings.Contains(admin.HTML, "Solo admins") {
		t.Errorf("admin block: vendedor=%q admin=%q", vend.HTML, admin.HTML)
	}
	if strings.Contains(vend.HTML, "<a") || !strings.Contains(vend.HTML, "Ver Ajustes.") {
		t.Errorf("vendedor link to admin page should be plain text: %q", vend.HTML)
	}
	if !strings.Contains(admin.HTML, `<a href="/ayuda/administracion">Ajustes</a>`) {
		t.Errorf("admin link missing: %q", admin.HTML)
	}
}

func TestTermTooltip(t *testing.T) {
	g, err := New(guideFS(map[string]string{"README.md": "# I\n\nEl [[folio|folio QA0001]] no cambia.\n"}))
	if err != nil {
		t.Fatal(err)
	}
	idx, _ := g.Page("", false)
	want := `<span class="term" tabindex="0">folio QA0001<span class="term-tip" role="tooltip">Identificador <em>único</em>. <a href="/ayuda/glosario#folio">Ver en el glosario</a></span></span>`
	if !strings.Contains(idx.HTML, want) {
		t.Errorf("got:\n%s\nwant substring:\n%s", idx.HTML, want)
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{
		"Emitir la cotización":                                 "emitir-la-cotización",
		"Actualizar tipo de cambio, precio del cobre y margen": "actualizar-tipo-de-cambio-precio-del-cobre-y-margen",
		"¿Qué serie elijo?":                                    "qué-serie-elijo",
		"IVA":                                                  "iva",
	} {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
}
