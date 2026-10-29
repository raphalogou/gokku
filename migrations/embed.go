// Package migrations embeds the goose SQL migrations. They are applied on start.
package migrations

import "embed"

// FS holds every numbered migration file.
//
//go:embed *.sql
var FS embed.FS
