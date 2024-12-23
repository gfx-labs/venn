package cqlmigrations

import "embed"

// Files contains *.cql schema migration files.

//go:embed *.cql
var Files embed.FS
