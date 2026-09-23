package guia

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/yuin/goldmark/ast"
)

// slug turns heading text into an anchor the way GitHub does: lowercase, drop
// punctuation, spaces to hyphens, keep accented letters. "Emitir la cotización" →
// "emitir-la-cotización".
func slug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case unicode.IsLetter(r), unicode.IsNumber(r), unicode.IsMark(r), r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		}
	}
	return b.String()
}

// githubIDs implements parser.IDs with GitHub's slugs and its -1, -2… suffixes for
// repeated headings.
type githubIDs struct {
	used map[string]bool
}

func newGithubIDs() *githubIDs {
	return &githubIDs{used: map[string]bool{}}
}

func (g *githubIDs) Generate(value []byte, _ ast.NodeKind) []byte {
	base := slug(string(value))
	id := base
	for i := 1; g.used[id]; i++ {
		id = fmt.Sprintf("%s-%d", base, i)
	}
	g.used[id] = true
	return []byte(id)
}

func (g *githubIDs) Put(value []byte) {
	g.used[string(value)] = true
}
