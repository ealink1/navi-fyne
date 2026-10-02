package sqlworkbench

import (
	"strings"
	"unicode"
)

// Token is a display-only SQL token. It never changes text sent to the driver.
type Token struct{ Text, Kind string }

var keywords = func() map[string]bool {
	words := strings.Fields("SELECT FROM WHERE AS AND OR NOT NULL IS IN LIKE BETWEEN EXISTS JOIN LEFT RIGHT INNER OUTER FULL CROSS ON USING WITH UNION ALL DISTINCT ORDER BY ASC DESC GROUP HAVING LIMIT OFFSET FETCH ROWS NEXT ONLY INSERT INTO VALUES UPDATE SET DELETE CREATE ALTER DROP TABLE VIEW INDEX PRIMARY KEY REFERENCES DEFAULT RETURNING CASE WHEN THEN ELSE END TRUE FALSE CAST COALESCE COUNT SUM AVG MIN MAX BEGIN COMMIT ROLLBACK EXPLAIN SHOW DESCRIBE")
	result := make(map[string]bool, len(words))
	for _, word := range words {
		result[word] = true
	}
	return result
}()

// Lex preserves all whitespace and incomplete edits while bounding display work.
func Lex(text string) []Token {
	runes := []rune(text)
	tokens := make([]Token, 0, min(len(runes), 256))
	for i := 0; i < len(runes); {
		if len(tokens) >= 10000 {
			tokens = append(tokens, Token{Text: string(runes[i:]), Kind: "plain"})
			break
		}
		start := i
		kind := "plain"
		c := runes[i]
		i++
		switch {
		case c == '-' && i < len(runes) && runes[i] == '-':
			kind = "comment"
			for i < len(runes) && runes[i] != '\n' {
				i++
			}
		case c == '/' && i < len(runes) && runes[i] == '*':
			kind = "comment"
			i++
			depth := 1
			for i < len(runes) && depth > 0 {
				if runes[i] == '/' && i+1 < len(runes) && runes[i+1] == '*' {
					depth++
					i += 2
				} else if runes[i] == '*' && i+1 < len(runes) && runes[i+1] == '/' {
					depth--
					i += 2
				} else {
					i++
				}
			}
		case c == '\'' || c == '"' || c == '`':
			if c == '\'' {
				kind = "string"
			}
			for i < len(runes) {
				if runes[i] == '\\' && i+1 < len(runes) {
					i += 2
					continue
				}
				if runes[i] == c {
					i++
					if i < len(runes) && runes[i] == c {
						i++
						continue
					}
					break
				}
				i++
			}
		case unicode.IsLetter(c) || c == '_':
			for i < len(runes) && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) || runes[i] == '_') {
				i++
			}
			if keywords[strings.ToUpper(string(runes[start:i]))] {
				kind = "keyword"
			}
		case unicode.IsDigit(c):
			kind = "number"
			for i < len(runes) && (unicode.IsDigit(runes[i]) || runes[i] == '.') {
				i++
			}
		case unicode.IsSpace(c):
			for i < len(runes) && unicode.IsSpace(runes[i]) {
				i++
			}
		}
		tokens = append(tokens, Token{Text: string(runes[start:i]), Kind: kind})
	}
	return tokens
}
