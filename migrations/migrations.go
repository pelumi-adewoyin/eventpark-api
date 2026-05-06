// Package migrations embeds all SQL migration files so they are compiled
// directly into the binary. Import this package and pass FS to db.RunMigrations.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
