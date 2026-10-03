// Package sqlworkbench builds dialect-aware table operations outside the UI.
package sqlworkbench

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ealink1/super-link/internal/domain"
)

// Quote quotes one literal metadata identifier; it never treats dots as SQL.
func Quote(kind, name string) (string, error) {
	if name == "" || strings.ContainsRune(name, 0) {
		return "", errors.New("invalid empty identifier")
	}
	switch kind {
	case "mysql", "goldendb", "mariadb", "oceanbase", "diros", "starrocks", "sphinx", "clickhouse", "tdengine", "iotdb":
		return "`" + strings.ReplaceAll(name, "`", "``") + "`", nil
	case "sqlserver":
		return "[" + strings.ReplaceAll(name, "]", "]]") + "]", nil
	default:
		return "\"" + strings.ReplaceAll(name, "\"", "\"\"") + "\"", nil
	}
}

// ObjectName quotes canonical components returned by metadata.
func ObjectName(kind string, object domain.Object) (string, error) {
	parts := []string{object.Name}
	if object.Schema != "" {
		parts = []string{object.Schema, object.Name}
	} else if object.Scope != "" {
		switch kind {
		case "mysql", "goldendb", "mariadb", "oceanbase", "diros", "starrocks", "sphinx":
			parts = []string{object.Scope, object.Name}
		}
	}
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		q, err := Quote(kind, part)
		if err != nil {
			return "", err
		}
		quoted = append(quoted, q)
	}
	return strings.Join(quoted, "."), nil
}

// BuildPage returns a page statement and a matching count statement.
func BuildPage(kind string, request domain.TableRequest, info domain.TableInfo) (string, string, error) {
	if request.Page < 1 || request.Size < 1 || request.Size > 1000 || request.Page > 10000000 {
		return "", "", errors.New("invalid page or page size")
	}
	name, err := ObjectName(kind, request.Object)
	if err != nil {
		return "", "", err
	}
	columns := make(map[string]bool, len(info.Columns))
	for _, column := range info.Columns {
		columns[column.Name] = true
	}
	where, err := conditions(kind, request, columns)
	if err != nil {
		return "", "", err
	}
	orders := make([]string, 0, len(request.Sorts))
	for _, sort := range request.Sorts {
		if !columns[sort.Column] {
			return "", "", errors.New("sort column is not in table metadata")
		}
		q, _ := Quote(kind, sort.Column)
		direction := " ASC"
		if sort.Descending {
			direction = " DESC"
		}
		orders = append(orders, q+direction)
	}
	if len(orders) == 0 {
		for _, column := range info.Columns {
			if column.Key == "PRI" {
				q, _ := Quote(kind, column.Name)
				orders = append(orders, q+" ASC")
			}
		}
	}
	order := ""
	if len(orders) > 0 {
		order = " ORDER BY " + strings.Join(orders, ", ")
	}
	offset := int64(request.Page-1) * int64(request.Size)
	base := "SELECT * FROM " + name + where
	count := "SELECT COUNT(*) AS navi_total FROM " + name + where
	switch kind {
	case "sqlserver":
		if order == "" {
			order = " ORDER BY (SELECT NULL)"
		}
		return fmt.Sprintf("%s%s OFFSET %d ROWS FETCH NEXT %d ROWS ONLY", base, order, offset, request.Size), count, nil
	case "oracle", "dameng":
		return fmt.Sprintf("%s%s OFFSET %d ROWS FETCH NEXT %d ROWS ONLY", base, order, offset, request.Size), count, nil
	case "iris", "cache":
		if request.Page != 1 {
			return "", "", errors.New("this driver supports the first TOP preview page only")
		}
		return fmt.Sprintf("SELECT TOP %d * FROM %s%s%s", request.Size, name, where, order), count, nil
	default:
		return fmt.Sprintf("%s%s LIMIT %d OFFSET %d", base, order, request.Size, offset), count, nil
	}
}

// BuildExportQuery keeps current filters and ordering while removing pagination.
func BuildExportQuery(kind string, request domain.TableRequest, info domain.TableInfo) (string, error) {
	request.Page = 1
	query, _, err := BuildPage(kind, request, info)
	if err != nil {
		return "", err
	}
	switch kind {
	case "oracle", "dameng", "sqlserver":
		query = strings.TrimSuffix(query, fmt.Sprintf(" OFFSET 0 ROWS FETCH NEXT %d ROWS ONLY", request.Size))
	case "iris", "cache":
		query = strings.Replace(query, fmt.Sprintf("SELECT TOP %d ", request.Size), "SELECT ", 1)
	default:
		query = strings.TrimSuffix(query, fmt.Sprintf(" LIMIT %d OFFSET 0", request.Size))
	}
	return query, nil
}

func conditions(kind string, request domain.TableRequest, columns map[string]bool) (string, error) {
	parts := make([]string, 0, len(request.Filters)+1)
	if text := strings.TrimSpace(request.Condition); text != "" {
		parts = append(parts, "("+text+")")
	}
	for _, filter := range request.Filters {
		if !columns[filter.Column] {
			return "", errors.New("filter column is not in table metadata")
		}
		column, _ := Quote(kind, filter.Column)
		operator := strings.ToUpper(strings.TrimSpace(filter.Operator))
		join := "AND"
		if filter.Join == "OR" {
			join = "OR"
		} else if filter.Join != "" && filter.Join != "AND" {
			return "", errors.New("invalid condition join")
		}
		value := TextLiteral(kind, filter.Value)
		switch operator {
		case "=", "<>", ">", ">=", "<", "<=", "LIKE", "NOT LIKE":
		case "IS NULL", "IS NOT NULL":
			value = ""
		default:
			return "", errors.New("invalid filter operator")
		}
		predicate := "(" + column + " " + operator
		if value != "" {
			predicate += " " + value
		}
		predicate += ")"
		if len(parts) > 0 {
			predicate = join + " " + predicate
		}
		parts = append(parts, predicate)
	}
	if len(parts) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(parts, " "), nil
}
