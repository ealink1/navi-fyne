package db

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestAgentArgumentsPreserveScalarTypes(t *testing.T) {
	stamp := time.Date(2026, 10, 2, 15, 22, 33, 123456789, time.FixedZone("", 8*3600))
	want := []any{int64(9223372036854775807), []byte{0, 255, 13}, stamp, nil, "NULL", false, 0.5}
	args, kinds, err := EncodeAgentArguments(want)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	var wire []any
	if err = json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	got, err := DecodeAgentArguments(wire, kinds)
	if err != nil {
		t.Fatal(err)
	}
	if !got[2].(time.Time).Equal(stamp) {
		t.Fatal("datetime changed instant")
	}
	got[2] = stamp
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("lost scalar types: %#v %v", got, err)
	}
	legacy, err := DecodeAgentArguments([]any{json.Number("9223372036854775807")}, nil)
	if err != nil || legacy[0] != want[0] {
		t.Fatalf("legacy integer lost precision: %#v %v", legacy, err)
	}
}

func TestAgentArgumentsRejectUnsupportedValuesAndLegacyAgents(t *testing.T) {
	for _, kind := range []string{"integer", "binary", "datetime", "unexpected"} {
		_, err := DecodeAgentArguments([]any{"invalid secret value"}, []string{kind})
		if !errors.Is(err, ErrInvalidAgentArguments) {
			t.Fatal(kind, err)
		}
	}
	client, stdin := newParamsTestAgentClient(t, OptionalDriverAgentProtocolSchemaV2)
	client.setConnectionCapabilities(optionalAgentConnectionInfo{TypedArguments: false})
	driver := &OptionalDriverAgentDB{driverType: "sqlite", client: client}
	_, err := driver.ExecContextWithArgs(t.Context(), "INSERT INTO t VALUES (?)", []any{int64(9223372036854775807)})
	if !errors.Is(err, ErrOptionalDriverAgentParamsUnsupported) || stdin.String() != "" {
		t.Fatal("legacy agent received unsafe bound write", err)
	}
}
