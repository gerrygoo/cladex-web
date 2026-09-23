package guia

import (
	"bytes"
	"html"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// termNode is a `[[término|texto]]` glossary reference.
type termNode struct {
	ast.BaseInline
	key  string // glossary heading id
	text string // what the reader sees
}

var kindTerm = ast.NewNodeKind("GlossaryTerm")

func (n *termNode) Kind() ast.NodeKind { return kindTerm }

func (n *termNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"key": n.key, "text": n.text}, nil)
}

type termParser struct{}

func (termParser) Trigger() []byte { return []byte{'['} }

func (termParser) Parse(_ ast.Node, block text.Reader, _ parser.Context) ast.Node {
	line, _ := block.PeekLine()
	if !bytes.HasPrefix(line, []byte("[[")) {
		return nil
	}
	end := bytes.Index(line, []byte("]]"))
	if end < 0 {
		return nil
	}
	inner := string(line[2:end])
	if strings.TrimSpace(inner) == "" || strings.ContainsAny(inner, "[]") {
		return nil
	}
	key, display, found := strings.Cut(inner, "|")
	if !found {
		display = key
	}
	block.Advance(end + 2)
	return &termNode{key: slug(key), text: strings.TrimSpace(display)}
}

type termRenderer struct {
	terms map[string]string // nil while the glossary itself is being loaded
}

func (r *termRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindTerm, r.render)
}

func (r *termRenderer) render(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	t := n.(*termNode)
	def, ok := r.terms[t.key]
	if !ok {
		w.WriteString(html.EscapeString(t.text))
		return ast.WalkContinue, nil
	}
	// tabindex makes the term focusable, so the tooltip opens on tap and keyboard
	// focus as well as hover; the CSS keys off :hover and :focus-within.
	w.WriteString(`<span class="term" tabindex="0">`)
	w.WriteString(html.EscapeString(t.text))
	w.WriteString(`<span class="term-tip" role="tooltip">`)
	w.WriteString(def)
	w.WriteString(` <a href="` + pageURL(glossarySlug) + "#" + html.EscapeString(t.key) + `">Ver en el glosario</a>`)
	w.WriteString(`</span></span>`)
	return ast.WalkContinue, nil
}

type termExtension struct {
	terms map[string]string
}

func (e *termExtension) Extend(m goldmark.Markdown) {
	// Ahead of the link parser (priority 200), which also triggers on '['.
	m.Parser().AddOptions(parser.WithInlineParsers(util.Prioritized(termParser{}, 199)))
	m.Renderer().AddOptions(renderer.WithNodeRenderers(util.Prioritized(&termRenderer{terms: e.terms}, 500)))
}
