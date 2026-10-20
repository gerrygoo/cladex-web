// Package appenv says which instance of the app is running: production, the staging
// instance on the NAS, or a developer's machine. It is read once from APP_ENV at startup.
//
// Unset means production, so the production container keeps working before its compose
// file names it. Anything that could reach the real world, such as stamping CFDIs with
// Facturoporti's production host, asks for Explicit production instead: an instance
// only gets there when its compose file says APP_ENV=production.
package appenv

import "fmt"

// Env is the running instance.
type Env struct {
	Name     string // "production", "staging" or "development"
	Explicit bool   // APP_ENV was set, not defaulted
}

const (
	Production  = "production"
	Staging     = "staging"
	Development = "development"
)

// Parse reads APP_ENV's value. Empty is production by default; an unknown name is an
// error, so a typo can't silently run as production.
func Parse(v string) (Env, error) {
	switch v {
	case "":
		return Env{Name: Production}, nil
	case Production, Staging, Development:
		return Env{Name: v, Explicit: true}, nil
	}
	return Env{}, fmt.Errorf("APP_ENV %q: want production, staging or development", v)
}

// IsProduction reports whether this is production, explicitly or by default.
func (e Env) IsProduction() bool { return e.Name == Production }

// IsExplicitProduction reports whether the instance was told it is production. Only then
// may it act on the real world (see the package comment).
func (e Env) IsExplicitProduction() bool { return e.IsProduction() && e.Explicit }

// Banner is the notice every page of a non-production instance shows, empty in
// production.
func (e Env) Banner() string {
	switch e.Name {
	case Staging:
		return "Ambiente de pruebas: los datos de aquí no son los reales y nada de lo que hagas aquí cuenta."
	case Development:
		return "Ambiente de desarrollo."
	}
	return ""
}

// Watermark is the word stamped across a non-production instance's PDFs, so one can't
// pass for a real quote; empty in production.
func (e Env) Watermark() string {
	if e.IsProduction() {
		return ""
	}
	return "PRUEBA"
}
