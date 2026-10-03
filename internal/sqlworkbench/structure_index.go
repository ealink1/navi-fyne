package sqlworkbench

import (
	"errors"
	"strings"

	"github.com/ealink1/super-link/internal/domain"
	"github.com/ealink1/super-link/internal/upstream/connection"
)

func buildIndexChange(kind, table string, object domain.Object, change domain.StructureChange, columns map[string]connection.ColumnDefinition, existing []connection.IndexDefinition) ([]string, error) {
	if change.Kind == "dropIndex" {
		found := false
		for _, index := range existing {
			if index.Name == change.OriginalName {
				found = true
			}
		}
		if !found {
			return nil, errors.New("original index is no longer present")
		}
		if strings.EqualFold(change.OriginalName, "PRIMARY") || strings.HasPrefix(change.OriginalName, "sqlite_autoindex_") {
			return nil, errors.New("constraint indexes must be changed through explicit DDL")
		}
		name, err := qualifiedIndex(kind, object, change.OriginalName)
		if err != nil {
			return nil, err
		}
		statement := "DROP INDEX " + name
		if kind == "mysql" {
			statement += " ON " + table
		}
		return []string{statement}, nil
	}
	index := change.Index
	name, err := Quote(kind, index.Name)
	if err != nil {
		return nil, err
	}
	for _, old := range existing {
		if old.Name == index.Name {
			return nil, errors.New("duplicate index name")
		}
	}
	if len(index.Columns) == 0 || len(index.Columns) > 32 {
		return nil, errors.New("index requires 1–32 columns")
	}
	parts := []string{}
	seen := map[string]bool{}
	for _, column := range index.Columns {
		if _, ok := columns[column.Name]; !ok || seen[column.Name] {
			return nil, errors.New("unknown or duplicate index column")
		}
		seen[column.Name] = true
		q, err := Quote(kind, column.Name)
		if err != nil {
			return nil, err
		}
		if column.Descending {
			q += " DESC"
		}
		parts = append(parts, q)
	}
	method := strings.ToUpper(index.Method)
	if method == "" {
		method = "BTREE"
	}
	allowed := method == "BTREE" || kind != "sqlite" && method == "HASH" || kind == "postgres" && (method == "GIN" || method == "GIST" || method == "BRIN" || method == "SPGIST")
	if !allowed {
		return nil, errors.New("unsupported index method")
	}
	unique := ""
	if index.Unique {
		unique = "UNIQUE "
	}
	statement := "CREATE " + unique + "INDEX " + name + " ON " + table
	if kind == "postgres" {
		statement += " USING " + method
	}
	statement += " (" + strings.Join(parts, ", ") + ")"
	if kind == "mysql" {
		statement += " USING " + method
	}
	return []string{statement}, nil
}

func updateIndexes(indexes []connection.IndexDefinition, change domain.StructureChange) []connection.IndexDefinition {
	if change.Kind == "addIndex" {
		return append(indexes, connection.IndexDefinition{Name: change.Index.Name})
	}
	kept := indexes[:0]
	for _, index := range indexes {
		if index.Name != change.OriginalName {
			kept = append(kept, index)
		}
	}
	return kept
}
