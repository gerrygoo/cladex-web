package views

import (
	"strings"
	"testing"
	"time"

	"github.com/gerrygoo/cladex-web/internal/facturoporti"
	"github.com/gerrygoo/cladex-web/internal/store"
)

func TestEnvironmentBanner(t *testing.T) {
	t.Cleanup(func() { EnvironmentBanner = "" })
	if page := render(t, Layout("Inicio", nil)); strings.Contains(page, "env-banner") {
		t.Error("production shows an environment banner")
	}
	EnvironmentBanner = "Ambiente de pruebas"
	assertContains(t, render(t, Layout("Inicio", nil)), `<p class="env-banner" role="note">Ambiente de pruebas</p>`)
}

func TestAjustesFacturacion(t *testing.T) {
	user := &NavUser{Username: "gerry", IsAdmin: true}
	off := render(t, Ajustes(nil, nil, nil, facturoporti.Status{Sandbox: true, Problem: "no está configurada"}, store.Emisor{SerieFactura: "F"}, nil, "", "", user))
	assertContains(t, off, "<h2>Facturación</h2>", "Pruebas (los timbres no son válidos)", "Apagada: no está configurada")
	on := render(t, Ajustes(nil, nil, nil, facturoporti.Status{Enabled: true, RFC: "EKU9003173C9", Number: "30001000000500003416",
		Expires: time.Date(2027, 5, 18, 0, 0, 0, 0, time.UTC)}, store.Emisor{RFC: "EKU9003173C9"}, nil, "", "", user))
	assertContains(t, on, "Producción", "Lista para timbrar", "RFC EKU9003173C9 · No. 30001000000500003416 · vence el 2027-05-18")
}
