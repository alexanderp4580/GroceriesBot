package migrations

import "embed"

//go:embed *.sql defaults.json
var Files embed.FS
