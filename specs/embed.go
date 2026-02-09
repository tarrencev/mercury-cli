package specs

import "embed"

// FS contains the vendored OpenAPI specs embedded into the binary.
//
//go:embed *.json
var FS embed.FS
