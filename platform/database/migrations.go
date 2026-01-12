package database

import "embed"

//go:embed migrations/*.sql
var CreditMigrations embed.FS
