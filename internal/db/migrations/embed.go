package migrations

import "embed"

// FS contains immutable product migrations shipped with the Hub binary.
//go:embed *.sql
var FS embed.FS
