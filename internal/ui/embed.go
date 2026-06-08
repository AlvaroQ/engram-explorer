package ui

import "embed"

//go:embed static
var StaticFS embed.FS

//go:embed locales
var localesFS embed.FS
