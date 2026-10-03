package db

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ealink1/super-link/internal/upstream/connection"
)

func TestAgentBinaryMetadataPreservesPrintableBinaryAndDoesNotReinterpretText(t *testing.T) {
	rows := []map[string]any{{"data": []byte("0x12"), "text": "0x12", "empty": []byte{}, "null": nil}}
	cells := AgentBinaryCells(rows)
	raw, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	var decoded []map[string]any
	if err := decodeJSONWithUseNumber(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := RestoreAgentBinaryCells(&decoded, cells); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rows, decoded) {
		t.Fatal("binary/text/empty/null distinction lost")
	}
	sets := []connection.ResultSetData{{Rows: rows}, {Rows: []map[string]any{{"body": []byte{0, 255}}}}}
	raw, _ = json.Marshal(sets)
	var decodedSets []connection.ResultSetData
	if err := decodeJSONWithUseNumber(raw, &decodedSets); err != nil {
		t.Fatal(err)
	}
	if err := RestoreAgentBinaryCells(&decodedSets, AgentBinaryCells(sets)); err != nil || !reflect.DeepEqual(sets, decodedSets) {
		t.Fatal("multi-result binary transport failed", err)
	}
}

func TestAgentBinaryMetadataRejectsOutOfRangeAndDuplicateCells(t *testing.T) {
	for _, cells := range [][]AgentBinaryCell{
		{{Result: -1, Row: -1, Column: "data"}}, {{Result: -1, Row: 1, Column: "data"}},
		{{Result: 0, Row: 0, Column: "data"}}, {{Result: -1, Row: 0, Column: "absent"}},
		{{Result: -1, Row: 0, Column: "data"}, {Result: -1, Row: 0, Column: "data"}},
	} {
		rows := []map[string]any{{"data": "AA=="}}
		if RestoreAgentBinaryCells(&rows, cells) == nil {
			t.Fatal("invalid binary metadata accepted", cells)
		}
	}
}

func TestNativeBinaryDoesNotGuessFromPrintableContent(t *testing.T) {
	for _, kind := range []string{"BLOB", "VARBINARY(20)", "BYTEA", "RAW", "OCIBlobLocator"} {
		value, ok := nativeBinaryValue([]byte("0x12"), kind, true)
		if !ok || !bytes.Equal(value, []byte("0x12")) {
			t.Fatal(kind, value, ok)
		}
	}
	if _, ok := nativeBinaryValue([]byte("text"), "VARCHAR", true); ok {
		t.Fatal("text was interpreted as binary")
	}
	if _, ok := nativeBinaryValue([]byte{0xff}, "BLOB", false); ok {
		t.Fatal("legacy display normalization changed")
	}
}
