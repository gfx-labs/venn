package cqlmigrations

import "github.com/scylladb/gocqlx/v2/table"

var RawBlockMetadata = table.Metadata{
	Name:    "blocks",
	Columns: []string{"hash", "parent_hash", "number", "chain", "flags", "header", "receipts", "logs"},
	PartKey: []string{"chain", "number"},
}
