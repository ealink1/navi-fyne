package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/ealink1/super-link/internal/upstream/connection"
)

type IndexColumn struct {
	Name       string
	Descending bool
}
type NewIndex struct {
	Name, Method string
	Columns      []IndexColumn
	Unique       bool
}
type StructureChange struct {
	Kind, OriginalName string
	Column             connection.ColumnDefinition
	Index              NewIndex
}
type StructureRequest struct {
	Object       Object
	Revision     int64
	BeforeHash   string
	Changes      []StructureChange
	Confirmation string
}
type StructureResult struct {
	Applied           int
	Atomic, Attempted bool
}

// StructureHash ignores transient metadata warnings and binds the visible schema.
func StructureHash(info TableInfo) string {
	info.Warnings = nil
	raw, _ := json.Marshal(info)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
