package main

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/ealink1/navi-fyne/internal/upstream/db"
)

func decodeAgentRequest(line string, req *agentRequest) error {
	*req = agentRequest{}
	decoder := json.NewDecoder(strings.NewReader(line))
	decoder.UseNumber()
	if err := decoder.Decode(req); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return db.ErrInvalidAgentArguments
	}
	args, err := db.DecodeAgentArguments(req.Args, req.ArgTypes)
	if err != nil {
		return err
	}
	req.Args = args
	return nil
}
