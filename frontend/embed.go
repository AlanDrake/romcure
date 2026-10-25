// Package frontend embeds the built Svelte+TS assets (frontend/dist) so the
// Wails entry point in /cmd/romcure can serve them without importing across
// the frontend/backend boundary via a relative go:embed path (not permitted).
package frontend

import "embed"

//go:embed all:dist
var Assets embed.FS
