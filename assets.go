// Package cladex embeds the static asset tree so the built binary is self-contained.
package cladex

import "embed"

//go:embed static
var StaticFS embed.FS
