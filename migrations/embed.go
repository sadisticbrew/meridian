package migrations

import "embed"

// FS holds the embedded *.sql migrations, applied in numeric filename order.
// embed.FS cannot reach across directories, hence this tiny package.
//
//go:embed *.sql
var FS embed.FS
