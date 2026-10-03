package main

import (
	"bytes"
	"encoding/json"
	"math"
	"testing"

	"github.com/ealink1/super-link/internal/upstream/db"
)

func TestAgentRequestDecodePreservesIntegerAndBinary(t *testing.T) {
	values, kinds, err := db.EncodeAgentArguments([]any{int64(math.MaxInt64), []byte{0, 255}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(agentRequest{ID: 1, Method: agentMethodExec, Args: values, ArgTypes: kinds})
	if err != nil {
		t.Fatal(err)
	}
	var request agentRequest
	if err = decodeAgentRequest(string(raw), &request); err != nil || request.Args[0] != int64(math.MaxInt64) || !bytes.Equal(request.Args[1].([]byte), []byte{0, 255}) {
		t.Fatal(request.Args, err)
	}
	if err = decodeAgentRequest(`{"id":1,"args":[9223372036854775807]}`, &request); err != nil || request.Args[0] != int64(math.MaxInt64) {
		t.Fatal(request.Args, err)
	}
}

func TestAgentRequestRejectsMalformedParametersAndTrailingData(t *testing.T) {
	for _, input := range []string{`{"id":1} {"id":2}`, `{"id":1,"args":[[1]]}`, `{"id":1,"args":["x"],"argTypes":["integer"]}`, `{"id":1,"args":["x"],"argTypes":["string","null"]}`} {
		var request agentRequest
		if decodeAgentRequest(input, &request) == nil {
			t.Fatal("invalid IPC request accepted")
		}
	}
}
