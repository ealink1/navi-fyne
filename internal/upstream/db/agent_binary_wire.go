package db

import (
	"encoding/base64"
	"errors"

	"github.com/ealink1/navi-fyne/internal/upstream/connection"
)

// AgentBinaryCell is out-of-band type information. No magic value in a user's
// text or JSON field can be mistaken for an encoded binary value.
type AgentBinaryCell struct {
	Result int    `json:"result"` // -1 for a single result, otherwise the result-set index.
	Row    int    `json:"row"`
	Column string `json:"column"`
}

func AgentBinaryCells(data any) []AgentBinaryCell {
	var cells []AgentBinaryCell
	appendRows := func(result int, rows []map[string]any) {
		for row, values := range rows {
			for column, value := range values {
				if bytes, ok := value.([]byte); ok && bytes != nil {
					cells = append(cells, AgentBinaryCell{Result: result, Row: row, Column: column})
				}
			}
		}
	}
	switch value := data.(type) {
	case []map[string]any:
		appendRows(-1, value)
	case []connection.ResultSetData:
		for i, result := range value {
			appendRows(i, result.Rows)
		}
	}
	return cells
}

func RestoreAgentBinaryCells(output any, cells []AgentBinaryCell) error {
	if len(cells) == 0 {
		return nil
	}
	invalid := errors.New("invalid driver-agent binary cell metadata")
	seen := make(map[AgentBinaryCell]bool, len(cells))
	for _, cell := range cells {
		if seen[cell] || cell.Row < 0 {
			return invalid
		}
		seen[cell] = true
		var rows []map[string]any
		switch value := output.(type) {
		case *[]map[string]any:
			if value == nil || cell.Result != -1 {
				return invalid
			}
			rows = *value
		case *[]connection.ResultSetData:
			if value == nil || cell.Result < 0 || cell.Result >= len(*value) {
				return invalid
			}
			rows = (*value)[cell.Result].Rows
		default:
			return invalid
		}
		if cell.Row >= len(rows) {
			return invalid
		}
		text, ok := rows[cell.Row][cell.Column].(string)
		if !ok {
			return invalid
		}
		data, err := base64.StdEncoding.DecodeString(text)
		if err != nil {
			return invalid
		}
		rows[cell.Row][cell.Column] = data
	}
	return nil
}
