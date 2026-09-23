// Package guia renders the Spanish user guide (docs/guia/*.md) to HTML for the in-app
// help pages under /ayuda.
//
// The Markdown stays the source of truth; on top of plain CommonMark + GFM tables it
// understands three conventions:
//
//   - Links to sibling pages (`cotizaciones.md#emitir-la-cotización`) become /ayuda
//     URLs. Heading IDs are generated the way GitHub generates them, accents included,
//     so anchors written against the Markdown keep working.
//   - `[[término]]` or `[[término|texto]]` renders a glossary tooltip. The term must be
//     an H2 in glosario.md; the tooltip shows that entry's first paragraph.
//   - Lines between `:::admin` and `:::` are shown to administrators only.
//
// Every page is rendered twice at startup — once per role — and every internal link
// and anchor is checked in both variants, so a broken link fails New (and its test)
// instead of reaching a user.
package guia

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
)

// pageDef lists the guide's pages in navigation order. Every .md file in the guide
// directory must appear here.
type pageDef struct {
	slug      string // "" is the index, served at /ayuda
	file      string
	navTitle  string // short label for the guide's page list; the H1 is the page title
	adminOnly bool
}

var pageDefs = []pageDef{
	{"", "README.md", "Inicio", false},
	{"acceso", "acceso.md", "Acceso y cuenta", false},
	{"clientes", "clientes.md", "Clientes", false},
	{"cotizaciones", "cotizaciones.md", "Cotizaciones", false},
	{"productos", "productos.md", "Productos", false},
	{"administracion", "administracion.md", "Administración", true},
	{"glosario", "glosario.md", "Glosario", false},
}

const (
	glossaryFile = "glosario.md"
	glossarySlug = "glosario"
	// repoDocsURL resolves links that leave the guide (../NAS_OPERATIONS.md). Only
	// admins with repository access follow those.
	repoDocsURL = "https://github.com/gerrygoo/cladex-web/blob/main/docs/"
)

// Page is one rendered guide page.
type Page struct {
	Slug     string
	Title    string
	NavTitle string
	URL      string
	HTML     string

	ids map[string]bool // heading ids, for Resolves
}

// Guide holds every page rendered for both roles.
type Guide struct {
	pages map[bool][]*Page // keyed by isAdmin, in navigation order
}

// Page returns the page for slug as the given role sees it. Admin-only pages are
// reported as missing to non-admins.
func (g *Guide) Page(slug string, isAdmin bool) (*Page, bool) {
	for _, p := range g.pages[isAdmin] {
		if p.Slug == slug {
			return p, true
		}
	}
	return nil, false
}

// Pages returns the pages the given role can see, in navigation order.
func (g *Guide) Pages(isAdmin bool) []*Page {
	return g.pages[isAdmin]
}

// Resolves reports whether url (/ayuda/<página>[#<sección>]) names a page and heading
// the given role can see. The app's "?" help links are checked with it.
func (g *Guide) Resolves(url string, isAdmin bool) bool {
	pagePart, frag, _ := strings.Cut(url, "#")
	if pagePart != "/ayuda" && !strings.HasPrefix(pagePart, "/ayuda/") {
		return false
	}
	p, ok := g.Page(strings.TrimPrefix(strings.TrimPrefix(pagePart, "/ayuda"), "/"), isAdmin)
	return ok && (frag == "" || p.ids[frag])
}

func pageURL(slug string) string {
	if slug == "" {
		return "/ayuda"
	}
	return "/ayuda/" + slug
}

// New reads and renders the guide from fsys (the docs/guia directory).
func New(fsys fs.FS) (*Guide, error) {
	sources := map[string][]byte{}
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || path.Ext(e.Name()) != ".md" {
			continue
		}
		if !slices.ContainsFunc(pageDefs, func(d pageDef) bool { return d.file == e.Name() }) {
			return nil, fmt.Errorf("guia: %s is not listed in pageDefs", e.Name())
		}
	}
	for _, d := range pageDefs {
		src, err := fs.ReadFile(fsys, d.file)
		if err != nil {
			return nil, fmt.Errorf("guia: %w", err)
		}
		sources[d.file] = src
	}

	terms, err := loadGlossary(sources[glossaryFile])
	if err != nil {
		return nil, err
	}

	g := &Guide{pages: map[bool][]*Page{}}
	for _, isAdmin := range []bool{false, true} {
		pages, err := renderVariant(sources, terms, isAdmin)
		if err != nil {
			role := "vendedor"
			if isAdmin {
				role = "admin"
			}
			return nil, fmt.Errorf("guia (%s): %w", role, err)
		}
		g.pages[isAdmin] = pages
	}
	return g, nil
}

func newMarkdown(terms map[string]string) goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(extension.Table, &termExtension{terms: terms}),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		// The sources are our own files in the repo; raw HTML is allowed so a page can
		// carry a small hand-written diagram.
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)
}

// parsed is one page parsed for one role, before rendering.
type parsed struct {
	def      pageDef
	src      []byte
	doc      ast.Node
	title    string
	ids      map[string]bool
	links    []string // internal destinations, after rewriting, to validate
	termKeys []string
}

func renderVariant(sources map[string][]byte, terms map[string]string, isAdmin bool) ([]*Page, error) {
	md := newMarkdown(terms)
	visible := map[string]pageDef{} // by file
	for _, d := range pageDefs {
		if !d.adminOnly || isAdmin {
			visible[d.file] = d
		}
	}

	var all []*parsed
	for _, d := range pageDefs {
		if _, ok := visible[d.file]; !ok {
			continue
		}
		src, err := filterAdminBlocks(sources[d.file], isAdmin)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", d.file, err)
		}
		ids := newGithubIDs()
		doc := md.Parser().Parse(text.NewReader(src), parser.WithContext(parser.NewContext(parser.WithIDs(ids))))
		p := &parsed{def: d, src: src, doc: doc, ids: ids.used}
		if err := rewriteLinks(p, visible); err != nil {
			return nil, fmt.Errorf("%s: %w", d.file, err)
		}
		all = append(all, p)
	}

	bySlug := map[string]*parsed{}
	for _, p := range all {
		bySlug[p.def.slug] = p
	}
	var pages []*Page
	for _, p := range all {
		for _, link := range p.links {
			if err := checkLink(p, link, bySlug); err != nil {
				return nil, fmt.Errorf("%s: %w", p.def.file, err)
			}
		}
		for _, key := range p.termKeys {
			if _, ok := terms[key]; !ok {
				return nil, fmt.Errorf("%s: [[%s]] is not a heading in %s", p.def.file, key, glossaryFile)
			}
			if !bySlug[glossarySlug].ids[key] {
				return nil, fmt.Errorf("%s: glossary entry %q is not visible to this role", p.def.file, key)
			}
		}
		if p.title == "" {
			return nil, fmt.Errorf("%s: missing H1 title", p.def.file)
		}
		var buf bytes.Buffer
		if err := md.Renderer().Render(&buf, p.src, p.doc); err != nil {
			return nil, fmt.Errorf("%s: %w", p.def.file, err)
		}
		pages = append(pages, &Page{Slug: p.def.slug, Title: p.title, NavTitle: p.def.navTitle, URL: pageURL(p.def.slug), HTML: buf.String(), ids: p.ids})
	}
	return pages, nil
}

// filterAdminBlocks drops the `:::admin` / `:::` marker lines, and for non-admins
// everything between them.
func filterAdminBlocks(src []byte, isAdmin bool) ([]byte, error) {
	var out bytes.Buffer
	inBlock := false
	for i, line := range strings.SplitAfter(string(src), "\n") {
		switch strings.TrimSpace(line) {
		case ":::admin":
			if inBlock {
				return nil, fmt.Errorf("line %d: nested :::admin", i+1)
			}
			inBlock = true
			continue
		case ":::":
			if !inBlock {
				return nil, fmt.Errorf("line %d: ::: without :::admin", i+1)
			}
			inBlock = false
			continue
		}
		if !inBlock || isAdmin {
			out.WriteString(line)
		}
	}
	if inBlock {
		return nil, fmt.Errorf("unterminated :::admin block")
	}
	return out.Bytes(), nil
}

// rewriteLinks maps .md destinations to /ayuda URLs, unwraps links to pages this role
// can't see, and records the title, internal links and glossary terms for validation.
func rewriteLinks(p *parsed, visible map[string]pageDef) error {
	var walkErr error
	var unwrap []*ast.Link
	ast.Walk(p.doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Heading:
			if n.Level == 1 && p.title == "" {
				p.title = plainText(n, p.src)
			}
		case *termNode:
			for a := n.Parent(); a != nil; a = a.Parent() {
				if _, ok := a.(*ast.Link); ok {
					// The tooltip carries its own link; nested <a> is invalid HTML.
					walkErr = fmt.Errorf("[[%s]] inside a link", n.text)
					return ast.WalkStop, nil
				}
			}
			p.termKeys = append(p.termKeys, n.key)
		case *ast.Link:
			dest := string(n.Destination)
			switch {
			case strings.HasPrefix(dest, "http://"), strings.HasPrefix(dest, "https://"), strings.HasPrefix(dest, "mailto:"):
			case strings.HasPrefix(dest, "#"):
				p.links = append(p.links, pageURL(p.def.slug)+dest)
			case strings.HasPrefix(dest, "../"):
				n.Destination = []byte(repoDocsURL + strings.TrimPrefix(dest, "../"))
			default:
				file, frag, _ := strings.Cut(dest, "#")
				def, ok := visible[file]
				if !ok {
					if slices.ContainsFunc(pageDefs, func(d pageDef) bool { return d.file == file }) {
						unwrap = append(unwrap, n)
						return ast.WalkSkipChildren, nil
					}
					walkErr = fmt.Errorf("link to unknown page %q", dest)
					return ast.WalkStop, nil
				}
				url := pageURL(def.slug)
				if frag != "" {
					url += "#" + frag
				}
				n.Destination = []byte(url)
				p.links = append(p.links, url)
			}
		}
		return ast.WalkContinue, nil
	})
	for _, link := range unwrap {
		parent := link.Parent()
		for c := link.FirstChild(); c != nil; {
			next := c.NextSibling()
			parent.InsertBefore(parent, link, c)
			c = next
		}
		parent.RemoveChild(parent, link)
	}
	return walkErr
}

func checkLink(from *parsed, url string, bySlug map[string]*parsed) error {
	pagePart, frag, _ := strings.Cut(url, "#")
	target, ok := bySlug[strings.TrimPrefix(strings.TrimPrefix(pagePart, "/ayuda"), "/")]
	if !ok {
		return fmt.Errorf("link %q: no such page", url)
	}
	if frag != "" && !target.ids[frag] {
		return fmt.Errorf("link %q: %s has no heading with that id", url, target.def.file)
	}
	return nil
}

// loadGlossary maps each H2 id in glosario.md to the rendered HTML of the paragraph
// that follows it, which becomes the tooltip text.
func loadGlossary(src []byte) (map[string]string, error) {
	src, err := filterAdminBlocks(src, true)
	if err != nil {
		return nil, fmt.Errorf("guia: %s: %w", glossaryFile, err)
	}
	// Terms inside definitions render as plain text here (nil map), so tooltips never nest.
	md := newMarkdown(nil)
	ids := newGithubIDs()
	doc := md.Parser().Parse(text.NewReader(src), parser.WithContext(parser.NewContext(parser.WithIDs(ids))))

	terms := map[string]string{}
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		h, ok := n.(*ast.Heading)
		if !ok || h.Level != 2 {
			continue
		}
		id, _ := h.AttributeString("id")
		para, ok := h.NextSibling().(*ast.Paragraph)
		if !ok {
			return nil, fmt.Errorf("guia: %s: %q must be followed by a paragraph", glossaryFile, plainText(h, src))
		}
		var buf bytes.Buffer
		for c := para.FirstChild(); c != nil; c = c.NextSibling() {
			if err := md.Renderer().Render(&buf, src, c); err != nil {
				return nil, err
			}
		}
		terms[string(id.([]byte))] = strings.TrimSpace(buf.String())
	}
	return terms, nil
}

func plainText(n ast.Node, src []byte) string {
	var b strings.Builder
	ast.Walk(n, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			switch t := n.(type) {
			case *ast.Text:
				b.Write(t.Segment.Value(src))
			case *ast.String:
				b.Write(t.Value)
			}
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}
