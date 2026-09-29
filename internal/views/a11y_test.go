package views

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/gerrygoo/cladex-web/internal/pricing"
	"github.com/gerrygoo/cladex-web/internal/store"
)

// These pin the WCAG fixes tracked in docs/UX_QUALITY.md to the rendered HTML, so a
// template change can't quietly undo them.

func render(t *testing.T, c templ.Component) string {
	t.Helper()
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func assertContains(t *testing.T, html string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(html, w) {
			t.Errorf("missing %q", w)
		}
	}
}

func TestLayoutHasSkipLinkAndLiveRegion(t *testing.T) {
	html := render(t, Login("", ""))
	assertContains(t, html,
		`<html lang="es">`,
		`<a class="skip-link" href="#contenido">`,
		`<main class="app-main" id="contenido" tabindex="-1">`,
		`<div id="live-status" class="visually-hidden" role="status">`,
	)
}

func TestFieldErrorsAreTiedToTheirFields(t *testing.T) {
	html := render(t, CustomerForm("Nuevo cliente", "/clientes/nuevo", CustomerFormValues{},
		map[string]string{"name": "El nombre es obligatorio."}, "", &NavUser{Username: "ana"}))
	assertContains(t, html,
		`name="name" value="" required aria-describedby="error-name" aria-invalid="true"`,
		`<p class="error" id="error-name">El nombre es obligatorio.</p>`,
	)
	if strings.Contains(html, `name="rfc" value="" aria-invalid`) {
		t.Error("a field without an error is marked invalid")
	}
}

func TestLoginDeclaresAutocomplete(t *testing.T) {
	html := render(t, Login("Usuario o contraseña incorrectos.", ""))
	assertContains(t, html,
		`autocomplete="username"`,
		`autocomplete="current-password"`,
		`<p class="error" role="alert">`,
	)
}

func TestQuoteLinesAreLabelledWithStableIDs(t *testing.T) {
	lines := []QuoteLineView{{Key: "3", ProductLabel: "cca-4 — Cable CCA 4", QtyRaw: "2"}}
	html := render(t, QuoteLinesFragment(store.Quote{Folio: "QA0001"}, lines, pricing.Totals{}, ""))
	assertContains(t, html,
		`id="linea-3-qty"`,
		`aria-label="Cantidad: cca-4 — Cable CCA 4"`,
		`id="linea-3-remove"`,
		`<span class="visually-hidden"> cca-4 — Cable CCA 4</span>`,
		`<div id="quote-lines-fragment" tabindex="-1" data-announce="Total: `,
		`<th><span class="visually-hidden">Acciones</span></th>`,
	)
}

func TestListsUseLinksNotButtonsInsideLinks(t *testing.T) {
	html := render(t, CustomersList(nil, ListView{Base: "/clientes"}, "", &NavUser{Username: "ana"}))
	if strings.Contains(html, "<a href=\"/clientes/nuevo\"><button") {
		t.Error("button nested in a link")
	}
	assertContains(t, html,
		`<a class="button" href="/clientes/nuevo">Nuevo cliente</a>`,
		`aria-label="Buscar por nombre, RFC o contacto"`,
		`data-announce="Ningún resultado"`,
	)
}

func TestRowActionsNameTheirRow(t *testing.T) {
	html := render(t, CustomersTableBody([]store.Customer{{ID: 7, Name: "Constructora Bajío"}}))
	assertContains(t, html,
		`Editar<span class="visually-hidden"> Constructora Bajío</span>`,
		`Eliminar<span class="visually-hidden"> Constructora Bajío</span>`,
		`data-confirm="¿Eliminar este cliente?"`,
	)
	if strings.Contains(html, "hx-confirm") {
		t.Error("hx-confirm does nothing on a plain form post; use data-confirm")
	}
}
