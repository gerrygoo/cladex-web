// Package cladex embeds the static asset and migration trees so the built binary is
// self-contained.
package cladex

import "embed"

//go:embed static
var StaticFS embed.FS

//go:embed migrations
var MigrationsFS embed.FS
