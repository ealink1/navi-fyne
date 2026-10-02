package sqlworkbench

import (
	"strings"
	"testing"
)

func TestLexerPreservesIncompleteSourceAndWhitespace(t *testing.T) {
	for _, source := range []string{"SELECT 1 AS id, 'Fyne parity' AS name, NULL;", "-- 中文\nSELECT \"a.b\" FROM \"表\";", "/* nested /* comment */ */\nSELECT 'unclosed", "SELECT " + strings.Repeat("a,", 15000)} {
		var rebuilt strings.Builder
		for _, token := range Lex(source) {
			rebuilt.WriteString(token.Text)
		}
		if rebuilt.String() != source {
			t.Fatal("display lexer changed source")
		}
	}
	got := Lex("SELECT 'a' 42 -- end")
	counts := map[string]int{}
	for _, token := range got {
		counts[token.Kind]++
	}
	for _, kind := range []string{"keyword", "string", "number", "comment"} {
		if counts[kind] != 1 {
			t.Fatal(got)
		}
	}
}
