package db

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

type vectorWhereExpr interface{}

type vectorWhereComparison struct {
	Field string
	Op    string
	Value interface{}
}

type vectorWhereLogical struct {
	Op          string
	Left, Right vectorWhereExpr
}

type vectorWhereParser struct {
	tokens []string
	pos    int
}

func parseVectorSQLWhere(sqlText string) (vectorWhereExpr, bool, error) {
	whereStart := findSQLKeyword(sqlText, "WHERE", 0)
	if whereStart < 0 {
		return nil, false, nil
	}
	end := len(sqlText)
	for _, keyword := range []string{"LIMIT", "OFFSET"} {
		if index := findSQLKeyword(sqlText, keyword, whereStart+len("WHERE")); index >= 0 && index < end {
			end = index
		}
	}
	if index := findSQLOrderBy(sqlText, whereStart+len("WHERE")); index >= 0 && index < end {
		end = index
	}
	clause := strings.TrimSpace(sqlText[whereStart+len("WHERE") : end])
	clause = strings.TrimSuffix(clause, ";")
	clause = strings.TrimSpace(clause)
	if clause == "" {
		return nil, true, fmt.Errorf("WHERE 条件不能为空")
	}
	tokens, err := tokenizeVectorWhere(clause)
	if err != nil {
		return nil, true, err
	}
	parser := vectorWhereParser{tokens: tokens}
	expr, err := parser.parseOr()
	if err != nil {
		return nil, true, err
	}
	if parser.pos != len(tokens) {
		return nil, true, fmt.Errorf("WHERE 包含不支持的语法：%s", tokens[parser.pos])
	}
	return expr, true, nil
}

func findSQLOrderBy(text string, start int) int {
	for searchFrom := start; searchFrom < len(text); {
		order := findSQLKeyword(text, "ORDER", searchFrom)
		if order < 0 {
			return -1
		}
		afterOrder := order + len("ORDER")
		for afterOrder < len(text) && unicode.IsSpace(rune(text[afterOrder])) {
			afterOrder++
		}
		if afterOrder+len("BY") <= len(text) && strings.EqualFold(text[afterOrder:afterOrder+len("BY")], "BY") &&
			(afterOrder+len("BY") == len(text) || !isSQLWordByte(text[afterOrder+len("BY")])) {
			return order
		}
		searchFrom = order + len("ORDER")
	}
	return -1
}

func validateQdrantWhereExpr(expr vectorWhereExpr) error {
	switch value := expr.(type) {
	case vectorWhereLogical:
		if err := validateQdrantWhereExpr(value.Left); err != nil {
			return err
		}
		return validateQdrantWhereExpr(value.Right)
	case vectorWhereComparison:
		field := strings.TrimPrefix(value.Field, "payload.")
		if strings.EqualFold(field, "id") && value.Op != "=" && value.Op != "!=" {
			return fmt.Errorf("Qdrant point ID 仅支持 = 和 != 条件")
		}
	}
	return nil
}

func findSQLKeyword(text, keyword string, start int) int {
	for i := start; i < len(text); {
		if next, ok := skipSQLQuotedLiteral(text, i); ok {
			i = next
			continue
		}
		if next, ok := skipSQLComment(text, i); ok {
			i = next
			continue
		}
		if sqlKeywordAt(text, keyword, i) {
			return i
		}
		i++
	}
	return -1
}

func isSQLWordByte(value byte) bool {
	return value == '_' || value >= '0' && value <= '9' || value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func sqlKeywordAt(text, keyword string, i int) bool {
	return i+len(keyword) <= len(text) && strings.EqualFold(text[i:i+len(keyword)], keyword) &&
		(i == 0 || !isSQLWordByte(text[i-1])) && (i+len(keyword) == len(text) || !isSQLWordByte(text[i+len(keyword)]))
}

func skipSQLQuotedLiteral(text string, i int) (int, bool) {
	if i >= len(text) {
		return i, false
	}
	quote := text[i]
	if quote != '\'' && quote != '"' && quote != '`' {
		return i, false
	}
	i++
	for i < len(text) {
		if text[i] == '\\' && quote == '\'' && i+1 < len(text) {
			i += 2
			continue
		}
		if text[i] == quote {
			if i+1 < len(text) && text[i+1] == quote {
				i += 2
				continue
			}
			return i + 1, true
		}
		i++
	}
	return len(text), true
}

func skipSQLComment(text string, i int) (int, bool) {
	if i+1 >= len(text) {
		return i, false
	}
	if text[i] == '-' && text[i+1] == '-' {
		i += 2
		for i < len(text) && text[i] != '\n' {
			i++
		}
		return i, true
	}
	if text[i] == '/' && text[i+1] == '*' {
		i += 2
		for i+1 < len(text) && !(text[i] == '*' && text[i+1] == '/') {
			i++
		}
		if i+1 < len(text) {
			return i + 2, true
		}
		return len(text), true
	}
	return i, false
}

func skipSQLSpaceAndComments(text string, i int) int {
	for i < len(text) {
		if unicode.IsSpace(rune(text[i])) {
			i++
			continue
		}
		if next, ok := skipSQLComment(text, i); ok {
			i = next
			continue
		}
		return i
	}
	return i
}

func sqlSelectProjection(text string) string {
	selectAt := findSQLKeyword(text, "SELECT", 0)
	if selectAt < 0 {
		return ""
	}
	fromAt := findSQLKeyword(text, "FROM", selectAt+len("SELECT"))
	if fromAt < 0 {
		return ""
	}
	return strings.TrimSpace(text[selectAt+len("SELECT") : fromAt])
}

func sqlContainsFunctionCall(text, name string) bool {
	for i := 0; i < len(text); {
		if next, ok := skipSQLQuotedLiteral(text, i); ok {
			i = next
			continue
		}
		if next, ok := skipSQLComment(text, i); ok {
			i = next
			continue
		}
		if sqlKeywordAt(text, name, i) {
			j := skipSQLSpaceAndComments(text, i+len(name))
			if j < len(text) && text[j] == '(' {
				return true
			}
			i += len(name)
			continue
		}
		i++
	}
	return false
}

func parseSQLFromName(text string) string {
	fromAt := findSQLKeyword(text, "FROM", 0)
	if fromAt < 0 {
		return ""
	}
	i := skipSQLSpaceAndComments(text, fromAt+len("FROM"))
	if i >= len(text) {
		return ""
	}
	if text[i] == '"' || text[i] == '`' {
		quote := text[i]
		i++
		start := i
		for i < len(text) && text[i] != quote {
			i++
		}
		if i >= len(text) {
			return ""
		}
		return strings.TrimSpace(text[start:i])
	}
	start := i
	for i < len(text) && isSQLFromNameByte(text[i]) {
		i++
	}
	return strings.TrimSpace(text[start:i])
}

func isSQLFromNameByte(value byte) bool {
	return value == '_' || value == '.' || value == '-' ||
		value >= '0' && value <= '9' || value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func parseSQLLimitClause(text string) (int, bool) {
	return parseSQLUnsignedKeywordValue(text, "LIMIT")
}

func parseSQLUnsignedOffset(text string) (int, bool) {
	return parseSQLUnsignedKeywordValue(text, "OFFSET")
}

func parseSQLUnsignedKeywordValue(text, keyword string) (int, bool) {
	for searchFrom := 0; searchFrom < len(text); {
		idx := findSQLKeyword(text, keyword, searchFrom)
		if idx < 0 {
			return 0, false
		}
		i := skipSQLSpaceAndComments(text, idx+len(keyword))
		if n, ok := parseSQLUnsignedInt(text[i:]); ok {
			return n, true
		}
		searchFrom = idx + len(keyword)
	}
	return 0, false
}

func parseSQLUnsignedInt(text string) (int, bool) {
	if text == "" || text[0] < '0' || text[0] > '9' {
		return 0, false
	}
	end := 1
	for end < len(text) && text[end] >= '0' && text[end] <= '9' {
		end++
	}
	n, err := strconv.Atoi(text[:end])
	if err != nil {
		return 0, false
	}
	return n, true
}

func parseSQLOffsetToken(text string) (string, bool) {
	for searchFrom := 0; searchFrom < len(text); {
		idx := findSQLKeyword(text, "OFFSET", searchFrom)
		if idx < 0 {
			return "", false
		}
		i := skipSQLSpaceAndComments(text, idx+len("OFFSET"))
		start := i
		for i < len(text) && isSQLFromNameByte(text[i]) {
			i++
		}
		if i > start {
			return text[start:i], true
		}
		searchFrom = idx + len("OFFSET")
	}
	return "", false
}

func tokenizeVectorWhere(text string) ([]string, error) {
	var tokens []string
	for i := 0; i < len(text); {
		if unicode.IsSpace(rune(text[i])) {
			i++
			continue
		}
		switch text[i] {
		case '(', ')':
			tokens = append(tokens, text[i:i+1])
			i++
		case '=', '!', '>', '<':
			start := i
			i++
			if i < len(text) && text[i] == '=' {
				i++
			}
			op := text[start:i]
			if op == "!" {
				return nil, fmt.Errorf("WHERE 包含不支持的操作符：%s", op)
			}
			tokens = append(tokens, op)
		case '\'', '"', '`':
			quote, start := text[i], i
			i++
			for i < len(text) {
				if text[i] == quote {
					if i+1 < len(text) && text[i+1] == quote {
						i += 2
						continue
					}
					break
				}
				if text[i] == '\\' && quote == '\'' && i+1 < len(text) {
					i += 2
				} else {
					i++
				}
			}
			if i == len(text) {
				return nil, fmt.Errorf("WHERE 包含未闭合的引号")
			}
			i++
			tokens = append(tokens, text[start:i])
		default:
			start := i
			for i < len(text) && !unicode.IsSpace(rune(text[i])) && !strings.ContainsRune("()=!><", rune(text[i])) {
				i++
			}
			tokens = append(tokens, text[start:i])
		}
	}
	return tokens, nil
}

func (p *vectorWhereParser) parseOr() (vectorWhereExpr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.consumeKeyword("OR") {
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = vectorWhereLogical{Op: "OR", Left: left, Right: right}
	}
	return left, nil
}
func (p *vectorWhereParser) parseAnd() (vectorWhereExpr, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for p.consumeKeyword("AND") {
		right, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		left = vectorWhereLogical{Op: "AND", Left: left, Right: right}
	}
	return left, nil
}
func (p *vectorWhereParser) parsePrimary() (vectorWhereExpr, error) {
	if p.consume("(") {
		expr, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if !p.consume(")") {
			return nil, fmt.Errorf("WHERE 缺少右括号")
		}
		return expr, nil
	}
	if p.pos+2 >= len(p.tokens) {
		return nil, fmt.Errorf("WHERE 比较表达式不完整")
	}
	field, op, raw := p.tokens[p.pos], p.tokens[p.pos+1], p.tokens[p.pos+2]
	p.pos += 3
	if strings.HasPrefix(field, "'") {
		return nil, fmt.Errorf("WHERE 左侧必须是字段名")
	}
	if op != "=" && op != "!=" && op != ">" && op != ">=" && op != "<" && op != "<=" {
		return nil, fmt.Errorf("WHERE 包含不支持的操作符：%s", op)
	}
	value, err := parseVectorWhereValue(raw)
	if err != nil {
		return nil, err
	}
	return vectorWhereComparison{Field: strings.Trim(field, "\"`"), Op: op, Value: value}, nil
}
func (p *vectorWhereParser) consume(token string) bool {
	if p.pos < len(p.tokens) && p.tokens[p.pos] == token {
		p.pos++
		return true
	}
	return false
}
func (p *vectorWhereParser) consumeKeyword(token string) bool {
	if p.pos < len(p.tokens) && strings.EqualFold(p.tokens[p.pos], token) {
		p.pos++
		return true
	}
	return false
}

func parseVectorWhereValue(raw string) (interface{}, error) {
	if len(raw) >= 2 && raw[0] == '\'' && raw[len(raw)-1] == '\'' {
		value := strings.ReplaceAll(raw[1:len(raw)-1], "''", "'")
		return strings.ReplaceAll(value, "\\'", "'"), nil
	}
	if strings.EqualFold(raw, "true") {
		return true, nil
	}
	if strings.EqualFold(raw, "false") {
		return false, nil
	}
	if number, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return number, nil
	}
	if number, err := strconv.ParseFloat(raw, 64); err == nil {
		return number, nil
	}
	return nil, fmt.Errorf("WHERE 值必须是字符串、数字或布尔值：%s", raw)
}

func chromaWhereFromExpr(expr vectorWhereExpr) interface{} {
	switch value := expr.(type) {
	case vectorWhereLogical:
		return map[string]interface{}{"$" + strings.ToLower(value.Op): []interface{}{chromaWhereFromExpr(value.Left), chromaWhereFromExpr(value.Right)}}
	case vectorWhereComparison:
		op := map[string]string{"=": "$eq", "!=": "$ne", ">": "$gt", ">=": "$gte", "<": "$lt", "<=": "$lte"}[value.Op]
		return map[string]interface{}{strings.TrimPrefix(value.Field, "metadata."): map[string]interface{}{op: value.Value}}
	}
	return nil
}

func qdrantFilterFromExpr(expr vectorWhereExpr) interface{} {
	return qdrantEnsureRootFilter(qdrantConditionFromExpr(expr))
}

func qdrantEnsureRootFilter(value interface{}) interface{} {
	m, ok := value.(map[string]interface{})
	if !ok {
		return value
	}
	if _, hasKey := m["key"]; hasKey {
		return map[string]interface{}{"must": []interface{}{m}}
	}
	if _, hasID := m["has_id"]; hasID {
		return map[string]interface{}{"must": []interface{}{m}}
	}
	return m
}

func qdrantConditionFromExpr(expr vectorWhereExpr) interface{} {
	switch value := expr.(type) {
	case vectorWhereLogical:
		key := "must"
		if value.Op == "OR" {
			key = "should"
		}
		return map[string]interface{}{key: []interface{}{qdrantConditionFromExpr(value.Left), qdrantConditionFromExpr(value.Right)}}
	case vectorWhereComparison:
		field := strings.TrimPrefix(value.Field, "payload.")
		if strings.EqualFold(field, "id") && (value.Op == "=" || value.Op == "!=") {
			condition := map[string]interface{}{"has_id": []interface{}{value.Value}}
			if value.Op == "!=" {
				return map[string]interface{}{"must_not": []interface{}{condition}}
			}
			return condition
		}
		if value.Op == "!=" {
			return map[string]interface{}{"must_not": []interface{}{map[string]interface{}{"key": field, "match": map[string]interface{}{"value": value.Value}}}}
		}
		if value.Op == "=" {
			return map[string]interface{}{"key": field, "match": map[string]interface{}{"value": value.Value}}
		}
		op := map[string]string{">": "gt", ">=": "gte", "<": "lt", "<=": "lte"}[value.Op]
		return map[string]interface{}{"key": field, "range": map[string]interface{}{op: value.Value}}
	}
	return nil
}
