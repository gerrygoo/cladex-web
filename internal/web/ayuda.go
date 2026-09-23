package web

import (
	"net/http"

	"github.com/gerrygoo/cladex-web/internal/guia"
	"github.com/gerrygoo/cladex-web/internal/views"
)

// Help serves the user guide under /ayuda. Pages are rendered once at startup (see
// internal/guia); each request only picks the variant for the user's role, so
// vendedores never see admin-only pages or sections.
type Help struct {
	guide *guia.Guide
}

func NewHelp(g *guia.Guide) *Help {
	return &Help{guide: g}
}

func (h *Help) Page(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	isAdmin := user.Role == "admin"
	page, ok := h.guide.Page(r.PathValue("pagina"), isAdmin)
	if !ok {
		http.NotFound(w, r)
		return
	}
	nav := make([]views.GuiaNavItem, 0, len(h.guide.Pages(isAdmin)))
	for _, p := range h.guide.Pages(isAdmin) {
		nav = append(nav, views.GuiaNavItem{Title: p.NavTitle, URL: p.URL, Current: p == page})
	}
	views.Guia(page.Title, page.HTML, nav, navUserView(user)).Render(r.Context(), w)
}
