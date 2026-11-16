// Package studio embeds the compiled React frontend. Rebuild with npm run build.
package studio

import "embed"

//go:embed dist
var Assets embed.FS
