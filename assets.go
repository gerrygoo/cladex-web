// Package cladex embeds the static asset, migration and user-guide trees so the built
// binary is self-contained.
package cladex

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
)

//go:embed static
var StaticFS embed.FS

//go:embed migrations
var MigrationsFS embed.FS

// GuiaFS is the Spanish user guide, served to logged-in users under /ayuda.
//
//go:embed docs/guia
var GuiaFS embed.FS

// AssetTag is the content tag of a static file: the first 8 bytes of its SHA-256, in
// hex. The static handler sends it as the ETag, and views.Asset puts it in the URL.
func AssetTag(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:8])
}
