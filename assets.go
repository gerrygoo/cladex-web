// Package cladex embeds the static asset, migration and user-guide trees so the built
// binary is self-contained.
package cladex

import "embed"

//go:embed static
var StaticFS embed.FS

//go:embed migrations
var MigrationsFS embed.FS

// GuiaFS is the Spanish user guide, served to logged-in users under /ayuda.
//
//go:embed docs/guia
var GuiaFS embed.FS
