// Package migrations содержит SQL-миграции, встроенные в бинарник.
package migrations

import "embed"

// MigrationsFS — файловая система со всеми SQL-миграциями проекта.
//
//go:embed *.sql
var MigrationsFS embed.FS
